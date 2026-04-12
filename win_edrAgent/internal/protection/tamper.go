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
// Works from BOTH contexts:
//   - SYSTEM: opens with WRITE_DAC directly (fast path)
//   - Administrator: uses SeTakeOwnershipPrivilege to take ownership first,
//     then reopens with WRITE_DAC (same pattern as restoreRegistryKeyByPath).
//     This solves the chicken-and-egg problem where the hardened DACL only
//     grants WRITE_DAC to SYSTEM, but the service is already stopped so the
//     SYSTEM-level watcher cannot run.
func RestoreServiceDACL(serviceName string) error {
	// Enable privileges needed for Administrator context.
	// Harmless for SYSTEM which already has them implicitly.
	_ = enableTakeOwnershipPrivilege()
	_ = enablePrivilege("SeRestorePrivilege")

	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("tamper: OpenSCManager: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	svcNamePtr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return err
	}

	// Fast path: try WRITE_DAC directly (succeeds from SYSTEM context).
	svc, err := windows.OpenService(scm, svcNamePtr, 0x40000|0x20000) // WRITE_DAC | READ_CONTROL
	if err != nil {
		// WRITE_DAC denied — we are Administrator, not SYSTEM.
		// Use SeTakeOwnershipPrivilege to take ownership first.

		// Step 1: Open with WRITE_OWNER (SeTakeOwnershipPrivilege grants this
		// regardless of the current DACL).
		svc, err = windows.OpenService(scm, svcNamePtr, 0x80000) // WRITE_OWNER
		if err != nil {
			return fmt.Errorf("tamper: cannot open service for ownership takeover: %w", err)
		}

		// Step 2: Set owner to Built-in Administrators (BA).
		// Once we own the service, we implicitly gain READ_CONTROL and WRITE_DAC.
		ownerSD, _, ownerErr := convertSDDLToSD("O:BA")
		if ownerErr != nil {
			windows.CloseServiceHandle(svc)
			return fmt.Errorf("tamper: convert owner SDDL: %w", ownerErr)
		}
		ret, _, callErr := procSetServiceObjectSecurity.Call(
			uintptr(svc),
			uintptr(1), // OWNER_SECURITY_INFORMATION
			uintptr(ownerSD),
		)
		windows.CloseServiceHandle(svc)
		if ret == 0 {
			return fmt.Errorf("tamper: set service owner to BA: %w", callErr)
		}

		// Step 3: Reopen with WRITE_DAC (now allowed because we are the owner).
		svc, err = windows.OpenService(scm, svcNamePtr, 0x40000|0x20000)
		if err != nil {
			return fmt.Errorf("tamper: reopen service for DACL restore after ownership: %w", err)
		}
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

// serviceRegistryPaths returns all registry paths that contain the service
// definition. Windows uses ControlSet001/002 as the actual storage and
// CurrentControlSet as a symlink. We must harden ALL of them to prevent
// an attacker from bypassing via direct ControlSet access.
func serviceRegistryPaths(serviceName string) []string {
	paths := []string{
		`SYSTEM\CurrentControlSet\Services\` + serviceName,
	}
	// Enumerate numbered ControlSets (001, 002, 003...)
	for i := 1; i <= 12; i++ {
		csPath := fmt.Sprintf(`SYSTEM\ControlSet%03d\Services\%s`, i, serviceName)
		// Only add if the key actually exists
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, csPath, registry.QUERY_VALUE)
		if err == nil {
			k.Close()
			paths = append(paths, csPath)
		}
	}
	return paths
}

// SDDL for service registry keys: SYSTEM full, Administrators read-only (query / enumerate).
const sddlServiceRegistryHardened = "O:SYD:P(A;CI;KA;;;SY)(A;CI;KR;;;BA)"

// SDDL for HKLM\SOFTWARE\EDR subtree: SYSTEM full, Administrators KEY_READ only.
// Owner SYSTEM blocks changing DACL via regedit without take-ownership; KR allows
// reading values (e.g. uninstall hash lookup) but not SET_VALUE / WRITE_DAC.
const sddlAgentRegistryHardened = "O:SYD:P(A;CI;KA;;;SY)(A;CI;KR;;;BA)"

// applyRegistrySDDL sets owner + protected DACL from an SDDL string on an opened HKLM key path.
func applyRegistrySDDL(keyPath, sddl string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, 0x20000|0x40000|0x80000) // READ_CONTROL | WRITE_DAC | WRITE_OWNER
	if err != nil {
		return fmt.Errorf("tamper: open registry key %s: %w", keyPath, err)
	}
	defer k.Close()

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("tamper: parse registry SDDL: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("tamper: extract owner SID: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("tamper: extract registry DACL: %w", err)
	}

	err = windows.SetSecurityInfo(
		windows.Handle(k),
		windows.SE_REGISTRY_KEY,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil,
	)
	if err != nil {
		return fmt.Errorf("tamper: set registry key security on %s: %w", keyPath, err)
	}
	return nil
}

// hardenRegistryKeyByPath applies a restrictive DACL on a single registry key
// AND sets the OWNER to SYSTEM. This is critical because:
//   - DACL alone is NOT enough: the key OWNER can always change the DACL
//   - By default, service registry keys are owned by Administrators
//   - Setting O:SY (Owner=SYSTEM) prevents Administrators from modifying
//     permissions via regedit, reg.exe, or PowerShell
func hardenRegistryKeyByPath(keyPath string) error {
	return applyRegistrySDDL(keyPath, sddlServiceRegistryHardened)
}

// enableTakeOwnershipPrivilege enables the SeTakeOwnershipPrivilege for the
// current process token. This allows an Administrator to take ownership of
// registry keys and service objects that are owned by SYSTEM.
func enableTakeOwnershipPrivilege() error {
	return enablePrivilege("SeTakeOwnershipPrivilege")
}

// enablePrivilege enables a named privilege (e.g. "SeTakeOwnershipPrivilege",
// "SeRestorePrivilege") on the current process token. This is a generic helper
// used by multiple tamper-resistance functions.
//
// Key privileges used by the agent:
//   - SeTakeOwnershipPrivilege: take ownership of objects regardless of DACL
//   - SeRestorePrivilege: set any valid owner/DACL regardless of current perms
func enablePrivilege(privilegeName string) error {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("tamper: open process token: %w", err)
	}
	defer token.Close()

	privName, _ := windows.UTF16PtrFromString(privilegeName)
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, privName, &luid); err != nil {
		return fmt.Errorf("tamper: lookup %s: %w", privilegeName, err)
	}

	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0].Luid = luid
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

	return windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
}

// restoreRegistryKeyByPath restores the default DACL and OWNER on a registry
// key, giving Administrators full control again.
//
// Works from BOTH contexts:
//   - SYSTEM: can always set any owner/DACL (used in uninstall watcher)
//   - Administrator: uses SeTakeOwnershipPrivilege (used in re-install)
func restoreRegistryKeyByPath(keyPath string) error {
	// Step 1: Enable SeTakeOwnershipPrivilege (needed for Admin context,
	// harmless for SYSTEM which already has it implicitly).
	_ = enableTakeOwnershipPrivilege()

	// Step 2: Take ownership — set OWNER back to Administrators (BA).
	// WRITE_OWNER (0x80000) is the only access right that SeTakeOwnershipPrivilege
	// grants regardless of the current DACL.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, 0x80000) // WRITE_OWNER
	if err != nil {
		// Key doesn't exist or truly inaccessible — not an error for fresh installs
		return nil
	}

	ownerSD, _ := windows.SecurityDescriptorFromString("O:BA")
	ownerSID, _, _ := ownerSD.Owner()
	err = windows.SetSecurityInfo(
		windows.Handle(k),
		windows.SE_REGISTRY_KEY,
		windows.OWNER_SECURITY_INFORMATION,
		ownerSID, nil, nil, nil,
	)
	k.Close()
	if err != nil {
		return fmt.Errorf("tamper: restore owner on %s: %w", keyPath, err)
	}

	// Step 3: Now that we own the key, restore full DACL.
	k, err = registry.OpenKey(registry.LOCAL_MACHINE, keyPath, 0x40000) // WRITE_DAC
	if err != nil {
		return fmt.Errorf("tamper: reopen for DACL restore %s: %w", keyPath, err)
	}
	defer k.Close()

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
		return fmt.Errorf("tamper: restore registry key security on %s: %w", keyPath, err)
	}
	return nil
}

// HardenServiceRegistryKey sets a restrictive DACL on ALL copies of the
// service's registry key (CurrentControlSet + every ControlSetXXX),
// INCLUDING their subkeys like \Security which block inheritance.
// This prevents deletion via regedit UI, CLI, or programmatic access.
func HardenServiceRegistryKey(serviceName string) error {
	paths := serviceRegistryPaths(serviceName)
	var lastErr error
	for _, p := range paths {
		// Harden the root service key
		if err := hardenRegistryKeyByPath(p); err != nil {
			lastErr = err
		}
		// CRITICAL: The SCM creates the \Security subkey with a DACL that blocks
		// inheritance (D:PAI). If we don't explicitly harden this subkey,
		// Administrators retain Full Control over it and can overwrite the
		// service's operational DACL, bypassing all our protections.
		if err := hardenRegistryKeyByPath(p + `\Security`); err != nil {
			// Not all services have this immediately, don't fail if not found
		}
		if err := hardenRegistryKeyByPath(p + `\Parameters`); err != nil {
			// Logically protect Parameters if it exists
		}
	}
	return lastErr
}

// RestoreServiceRegistryKey restores the default DACL on ALL copies of the
// service's registry key and its subkeys. Must be called from SYSTEM context.
func RestoreServiceRegistryKey(serviceName string) error {
	paths := serviceRegistryPaths(serviceName)
	var lastErr error
	for _, p := range paths {
		// Restore subkeys first before the parent
		_ = restoreRegistryKeyByPath(p + `\Security`)
		_ = restoreRegistryKeyByPath(p + `\Parameters`)

		if err := restoreRegistryKeyByPath(p); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// =========================================================================
// Layer 4: Agent Configuration Registry Protection
// =========================================================================

const (
	edrSoftwareRoot   = `SOFTWARE\EDR`
	agentRegistryPath = `SOFTWARE\EDR\Agent`
)

// HardenAgentRegistryKey protects HKLM\SOFTWARE\EDR and HKLM\SOFTWARE\EDR\Agent.
// SYSTEM has full control; Administrators may read (for uninstall hash resolution)
// but cannot change values or permissions without taking ownership.
func HardenAgentRegistryKey() error {
	// Ensure keys exist (install may have created only the leaf).
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, edrSoftwareRoot, registry.ALL_ACCESS)
	if err == nil {
		_ = k.Close()
	}
	k2, _, err2 := registry.CreateKey(registry.LOCAL_MACHINE, agentRegistryPath, registry.ALL_ACCESS)
	if err2 == nil {
		_ = k2.Close()
	}
	var lastErr error
	if err := applyRegistrySDDL(edrSoftwareRoot, sddlAgentRegistryHardened); err != nil {
		lastErr = err
	}
	if err := applyRegistrySDDL(agentRegistryPath, sddlAgentRegistryHardened); err != nil {
		lastErr = err
	}
	return lastErr
}

// RestoreAgentRegistryKey restores default DACL on HKLM\SOFTWARE\EDR\Agent and
// its parent SOFTWARE\EDR so Administrators can clean up during uninstall.
func RestoreAgentRegistryKey() error {
	var lastErr error
	if err := restoreRegistryKeyByPath(agentRegistryPath); err != nil {
		lastErr = err
	}
	if err := restoreRegistryKeyByPath(edrSoftwareRoot); err != nil {
		lastErr = err
	}
	return lastErr
}

// SaveTokenHashToRegistry stores the uninstall token hash in a protected
// registry key. This provides a backup source for uninstall verification
// even if the EXE file is deleted or replaced.
func SaveTokenHashToRegistry(tokenHash string) error {
	if tokenHash == "" {
		return nil
	}
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, agentRegistryPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("tamper: create agent registry key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue("UninstallTokenHash", tokenHash); err != nil {
		return fmt.Errorf("tamper: write token hash: %w", err)
	}
	return nil
}

// ReadTokenHashFromRegistry reads the stored uninstall token hash.
// Returns empty string if not found (not an error — fallback to embedded).
func ReadTokenHashFromRegistry() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, agentRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	val, _, err := k.GetStringValue("UninstallTokenHash")
	if err != nil {
		return ""
	}
	return val
}

// SaveCriticalConfig stores critical agent configuration values in the
// protected registry key. These serve as a tamper-proof backup that the
// agent can fall back to if config.yaml is deleted or corrupted.
func SaveCriticalConfig(serverAddress, agentID, caPath, certPath, keyPath string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, agentRegistryPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("tamper: create agent registry key: %w", err)
	}
	defer k.Close()

	if serverAddress != "" {
		_ = k.SetStringValue("ServerAddress", serverAddress)
	}
	if agentID != "" {
		_ = k.SetStringValue("AgentID", agentID)
	}
	if caPath != "" {
		_ = k.SetStringValue("CAPath", caPath)
	}
	if certPath != "" {
		_ = k.SetStringValue("CertPath", certPath)
	}
	if keyPath != "" {
		_ = k.SetStringValue("KeyPath", keyPath)
	}
	return nil
}

// ReadCriticalConfig reads critical config values from the protected registry.
// Returns empty strings for missing values (not errors).
func ReadCriticalConfig() (serverAddress, agentID, caPath, certPath, keyPath string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, agentRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	serverAddress, _, _ = k.GetStringValue("ServerAddress")
	agentID, _, _ = k.GetStringValue("AgentID")
	caPath, _, _ = k.GetStringValue("CAPath")
	certPath, _, _ = k.GetStringValue("CertPath")
	keyPath, _, _ = k.GetStringValue("KeyPath")
	return
}

// CleanAgentRegistryKey removes the agent's config registry key.
// Called during uninstall to clean up.
func CleanAgentRegistryKey() {
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, agentRegistryPath)
	// Also try parent if empty
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\EDR`)
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
