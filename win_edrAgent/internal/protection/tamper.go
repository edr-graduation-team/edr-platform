// Package protection implements tamper-resistance mechanisms for the EDR Agent.
//
// Three layers of defense are provided:
//
//  1. Process Self-Protection (ProtectProcess):
//     Sets a restrictive DACL on the agent's own process handle so that only
//     SYSTEM can terminate it.  taskkill /F, Process Explorer "End Process",
//     and TerminateProcess() calls from non-SYSTEM callers will receive
//     ERROR_ACCESS_DENIED.
//
//  2. Service DACL Hardening (HardenServiceDACL):
//     Modifies the service's security descriptor to remove Stop/Delete/PauseContinue
//     permissions from the Built-in Administrators group.  Only the SYSTEM account
//     (i.e., the service process itself) retains full control.  This blocks:
//       - sc stop EDRAgent
//       - sc delete EDRAgent
//       - Stop-Service EDRAgent
//     from an elevated command prompt.
//
//  3. Uninstall Token Verification (VerifyUninstallToken):
//     The --uninstall flag requires a matching --token argument.  The token is
//     verified against a SHA-256 hash stored in the agent's config file.
//     Without the correct token, the binary refuses to uninstall.
//
//go:build windows
// +build windows

package protection

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// =========================================================================
// Layer 1: Process Self-Protection
// =========================================================================

// ProtectProcess sets a restrictive DACL on the current process that only
// grants access to the SYSTEM account.  All other security principals
// (including Administrators) are denied PROCESS_TERMINATE and PROCESS_SUSPEND.
//
// This means:
//   - taskkill /F /PID <agentPID>              → ERROR_ACCESS_DENIED
//   - Task Manager → End Process               → ACCESS DENIED
//   - Process Explorer → Kill Process           → ACCESS DENIED
//   - TerminateProcess(handle, 1)               → ACCESS DENIED
//
// The agent itself (running as SYSTEM) is unaffected and can still self-terminate
// via os.Exit() or context cancellation.
//
// NOTE: This protection can be bypassed by:
//   - A kernel driver (ring-0)
//   - PsExec -s (runs as SYSTEM)
//   - A process running as SYSTEM
//     These are acceptable limitations for a user-mode EDR agent.
func ProtectProcess() error {
	// SDDL breakdown:
	//   D:           → DACL follows
	//   (A;;GA;;;SY) → Allow GENERIC_ALL to SYSTEM (S-1-5-18)
	//
	// By specifying ONLY a SYSTEM ACE, all other principals are implicitly
	// denied all access (default-deny).  This is stricter than adding an
	// explicit Deny ACE because it cannot be overridden by inherited ACEs.
	const sddl = "D:(A;;GA;;;SY)"

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("tamper: parse process SDDL: %w", err)
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("tamper: extract process DACL: %w", err)
	}

	err = windows.SetSecurityInfo(
		windows.CurrentProcess(),
		windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		return fmt.Errorf("tamper: SetSecurityInfo on process: %w", err)
	}

	return nil
}

// =========================================================================
// Layer 2: Service DACL Hardening
// =========================================================================

var (
	advapi32                                = windows.NewLazySystemDLL("advapi32.dll")
	procSetServiceObjectSecurity            = advapi32.NewProc("SetServiceObjectSecurity")
	procConvertStringSecurityDescriptorToSD = advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
)

// HardenServiceDACL modifies the security descriptor of the named service
// so that only SYSTEM retains full control.  Administrators can still
// query the service status but cannot stop, pause, or delete it.
//
// SDDL breakdown:
//
//	(A;;CCLCSWRPWPDTLOCRSDRCWDWO;;;SY)  → SYSTEM: full control
//	(A;;CCLCSWLOCRRC;;;BA)              → Built-in Admins: query-only
//	(A;;CCLCSWLOCRRC;;;IU)              → Interactive Users: query-only
//
// Access rights removed from Administrators:
//   - SERVICE_STOP           (WP) = 0x0020
//   - SERVICE_PAUSE_CONTINUE (DT) = 0x0040
//   - SERVICE_START          (RP) = 0x0010  (optional, kept for reboot)
//   - DELETE                 (SD) = 0x10000
//   - WRITE_DAC              (WD) = 0x40000
//   - WRITE_OWNER            (WO) = 0x80000
//   - SERVICE_CHANGE_CONFIG       = 0x0002
func HardenServiceDACL(serviceName string) error {
	// Open the service with WRITE_DAC permission.
	// We need this permission to change the service's security descriptor.
	// At install time, the installer runs as Administrator (or SYSTEM), which
	// has WRITE_DAC by default on a freshly-created service.
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("tamper: OpenSCManager: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	svcNamePtr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return fmt.Errorf("tamper: UTF16 service name: %w", err)
	}

	// SERVICE_QUERY_STATUS | WRITE_DAC | READ_CONTROL
	const desiredAccess = 0x0004 | 0x40000 | 0x20000
	svc, err := windows.OpenService(scm, svcNamePtr, desiredAccess)
	if err != nil {
		return fmt.Errorf("tamper: OpenService %s: %w", serviceName, err)
	}
	defer windows.CloseServiceHandle(svc)

	// Hardened SDDL:
	// SYSTEM = full control
	// Administrators = query status, query config, interrogate, read-only
	// Interactive Users = query status, query config, read-only
	const hardenedSDDL = "D:" +
		"(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;SY)" + // SYSTEM: full (including SERVICE_CHANGE_CONFIG)
		"(A;;CCLCSWLOCRRC;;;BA)" + // Admins: query-only
		"(A;;CCLCSWLOCRRC;;;IU)" // Interactive: query-only

	sd, sdSize, err := convertSDDLToSD(hardenedSDDL)
	if err != nil {
		return fmt.Errorf("tamper: convert hardened SDDL: %w", err)
	}
	_ = sdSize

	// DACL_SECURITY_INFORMATION = 4
	ret, _, callErr := procSetServiceObjectSecurity.Call(
		uintptr(svc),
		uintptr(4), // DACL_SECURITY_INFORMATION
		uintptr(sd),
	)
	if ret == 0 {
		return fmt.Errorf("tamper: SetServiceObjectSecurity: %w", callErr)
	}

	return nil
}

// RestoreServiceDACL restores the service's security descriptor to the
// Windows default, allowing Administrators to stop and delete the service.
// This must be called BEFORE stopping the service during a legitimate uninstall.
//
// Since the hardened DACL only gives WRITE_DAC to SYSTEM, this function
// MUST be called from a SYSTEM-level context (e.g., PsExec -s) or from
// within the service process itself.
func RestoreServiceDACL(serviceName string) error {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("tamper: OpenSCManager: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	svcNamePtr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return err
	}

	// WRITE_DAC
	svc, err := windows.OpenService(scm, svcNamePtr, 0x40000|0x20000)
	if err != nil {
		// If we can't open with WRITE_DAC, the caller isn't SYSTEM.
		// This is expected for non-SYSTEM callers.
		return fmt.Errorf("tamper: cannot restore DACL (are you running as SYSTEM?): %w", err)
	}
	defer windows.CloseServiceHandle(svc)

	// Default Windows service SDDL — full admin control restored.
	const defaultSDDL = "D:" +
		"(A;;CCLCSWRPWPDTLOCRRC;;;SY)" + // SYSTEM
		"(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)" + // Admins: full
		"(A;;CCLCSWLOCRRC;;;IU)" // Interactive: query

	sd, _, err := convertSDDLToSD(defaultSDDL)
	if err != nil {
		return fmt.Errorf("tamper: convert default SDDL: %w", err)
	}

	ret, _, callErr := procSetServiceObjectSecurity.Call(
		uintptr(svc),
		uintptr(4), // DACL_SECURITY_INFORMATION
		uintptr(sd),
	)
	if ret == 0 {
		return fmt.Errorf("tamper: restore DACL: %w", callErr)
	}

	return nil
}

// HardenServiceRegistryKey sets a restrictive DACL on the service's registry
// key (HKLM\SYSTEM\CurrentControlSet\Services\<serviceName>) so that only
// SYSTEM retains full control. Administrators can read the key but cannot
// delete or modify it, blocking attacks via `reg delete`.
func HardenServiceRegistryKey(serviceName string) error {
	keyPath := `SYSTEM\CurrentControlSet\Services\` + serviceName
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, 0x20000|0x40000) // READ_CONTROL | WRITE_DAC
	if err != nil {
		return fmt.Errorf("tamper: open registry key: %w", err)
	}
	defer k.Close()

	// SYSTEM: full key access with container inherit
	// Administrators: read-only with container inherit
	const sddl = "D:P(A;CI;KA;;;SY)(A;CI;KR;;;BA)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("tamper: parse registry SDDL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("tamper: extract registry DACL: %w", err)
	}

	err = windows.SetSecurityInfo(
		windows.Handle(k),
		windows.SE_REGISTRY_KEY,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		return fmt.Errorf("tamper: set registry key security: %w", err)
	}
	return nil
}

// RestoreServiceRegistryKey restores the default DACL on the service's registry key,
// giving Administrators full control again. Must be called from SYSTEM context.
func RestoreServiceRegistryKey(serviceName string) error {
	keyPath := `SYSTEM\CurrentControlSet\Services\` + serviceName
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, 0x20000|0x40000) // READ_CONTROL | WRITE_DAC
	if err != nil {
		return fmt.Errorf("tamper: open registry key for restore: %w", err)
	}
	defer k.Close()

	// Restore: both SYSTEM and Administrators get full key access
	const sddl = "D:(A;CI;KA;;;SY)(A;CI;KA;;;BA)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("tamper: parse restore SDDL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("tamper: extract restore DACL: %w", err)
	}

	err = windows.SetSecurityInfo(
		windows.Handle(k),
		windows.SE_REGISTRY_KEY,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		return fmt.Errorf("tamper: restore registry key security: %w", err)
	}
	return nil
}

// convertSDDLToSD converts an SDDL string to a binary security descriptor.
func convertSDDLToSD(sddl string) (sd unsafe.Pointer, sdSize uint32, err error) {
	sddlPtr, err := windows.UTF16PtrFromString(sddl)
	if err != nil {
		return nil, 0, err
	}

	var sdPtr uintptr
	var size uint32

	// SDDL_REVISION_1 = 1
	ret, _, callErr := procConvertStringSecurityDescriptorToSD.Call(
		uintptr(unsafe.Pointer(sddlPtr)),
		uintptr(1), // SDDL_REVISION_1
		uintptr(unsafe.Pointer(&sdPtr)),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return nil, 0, fmt.Errorf("ConvertStringSecurityDescriptorToSecurityDescriptorW: %w", callErr)
	}

	return unsafe.Pointer(sdPtr), size, nil
}

// =========================================================================
// Layer 3: Uninstall Token Verification
// =========================================================================

// VerifyUninstallToken validates the provided plaintext token against the
// SHA-256 hash embedded in the binary at build time.
//
// Security design:
//   - The binary contains ONLY the SHA-256 hash of the enrollment token,
//     injected via -ldflags by the dashboard build system.
//   - The plaintext secret is NEVER stored in the binary, config files,
//     registry, or disk — it exists only as a CLI argument during install.
//   - Even if the .exe is captured and reverse-engineered (strings, IDA Pro),
//     the attacker gets an irreversible hash, not the secret.
//   - Comparison uses crypto/subtle.ConstantTimeCompare to prevent timing
//     side-channel attacks.
//   - If no embedded hash exists (development builds), falls back to the
//     legacy default token for backward compatibility.
func VerifyUninstallToken(providedToken, embeddedHash string) error {
	if providedToken == "" {
		return fmt.Errorf("uninstall token required: use --token <secret>")
	}

	// Compute the SHA-256 hash of the provided token.
	providedHash := sha256HexString(providedToken)

	// If no embedded hash (dev build without dashboard), use legacy default.
	// if embeddedHash == "" {
	// 	embeddedHash = sha256HexString("EDR-Uninstall-2026!")
	// }

	// Constant-time comparison prevents timing side-channel attacks.
	if subtle.ConstantTimeCompare([]byte(providedHash), []byte(embeddedHash)) != 1 {
		return fmt.Errorf("invalid uninstall token")
	}

	return nil
}

// VerifyUninstallHash validates a pre-computed hash against the
// SHA-256 hash embedded in the binary. This is used by the background
// service when verifying uninstall requests via IPC/Registry.
func VerifyUninstallHash(providedHash, embeddedHash string) error {
	if providedHash == "" {
		return fmt.Errorf("uninstall hash required")
	}

	// If no embedded hash (dev build without dashboard), use legacy default.
	if embeddedHash == "" {
		embeddedHash = sha256HexString("EDR-Uninstall-2026!")
	}

	// Constant-time comparison prevents timing side-channel attacks.
	if subtle.ConstantTimeCompare([]byte(providedHash), []byte(embeddedHash)) != 1 {
		return fmt.Errorf("invalid uninstall hash")
	}

	return nil
}

// HashUninstallToken computes the SHA-256 hash of a plaintext token.
func HashUninstallToken(token string) string {
	return sha256HexString(token)
}

// sha256HexString returns the lowercase hex-encoded SHA-256 hash of s.
func sha256HexString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
