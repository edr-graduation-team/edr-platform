// Package security provides NTFS ACL hardening, encryption, retention, and
// self-protection for the EDR Agent on Windows.
//
//go:build windows
// +build windows

package security

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/edr-platform/win-agent/internal/logging"
)

// Well-known SIDs used in our DACLs.
var (
	sidSystem         *windows.SID // S-1-5-18  (NT AUTHORITY\SYSTEM)
	sidAdministrators *windows.SID // S-1-5-32-544  (BUILTIN\Administrators)
)

func init() {
	var err error
	sidSystem, err = windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		panic("security: cannot create SYSTEM SID: " + err.Error())
	}
	sidAdministrators, err = windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		panic("security: cannot create Administrators SID: " + err.Error())
	}
}

// HardenDirectories applies restrictive NTFS ACLs to the given directories.
// After this call only SYSTEM and BUILTIN\Administrators can read, write, or
// list the contents. Standard users will receive "Access Denied".
//
// The function:
//  1. Creates the directory if it does not exist.
//  2. Builds a new DACL with two ALLOW ACEs (SYSTEM + Administrators, Full Control).
//  3. Applies the DACL via SetNamedSecurityInfo with PROTECTED_DACL so
//     inherited permissions from parent directories are stripped.
func HardenDirectories(dirs []string, logger *logging.Logger) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("security: create dir %s: %w", dir, err)
		}
		if err := setRestrictedDACL(dir); err != nil {
			if logger != nil {
				logger.Warnf("[Security] Failed to harden %s: %v", dir, err)
			}
			return fmt.Errorf("security: harden %s: %w", dir, err)
		}
		if logger != nil {
			logger.Infof("[Security] ACL hardened: %s (SYSTEM + Administrators only)", dir)
		}
	}
	return nil
}

// setRestrictedDACL builds a DACL containing exactly two ACEs:
//   - SYSTEM       → GENERIC_ALL (Full Control) with Object Inherit + Container Inherit
//   - Administrators → GENERIC_ALL (Full Control) with Object Inherit + Container Inherit
//
// The DACL is applied with PROTECTED_DACL_SECURITY_INFORMATION so that any
// inherited ACEs from parent folders are removed.
func setRestrictedDACL(path string) error {
	// ── Build explicit ACEs ──────────────────────────────────────────────────
	// ACCESS_MASK for full control over files and subdirectories.
	const fullControl = windows.GENERIC_ALL

	// Inheritance flags: apply to this directory, all subdirectories, and all files.
	const inheritFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE

	aces := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: fullControl,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       inheritFlags,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidSystem),
			},
		},
		{
			AccessPermissions: fullControl,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       inheritFlags,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidAdministrators),
			},
		},
	}

	// ── Create the DACL ─────────────────────────────────────────────────────
	newDACL, err := windows.ACLFromEntries(aces, nil)
	if err != nil {
		return fmt.Errorf("ACLFromEntries: %w", err)
	}

	// ── Apply to the path ───────────────────────────────────────────────────
	// PROTECTED_DACL_SECURITY_INFORMATION prevents ACE inheritance from the
	// parent directory, ensuring our explicit ACEs are the ONLY ones.
	const secInfo = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("UTF16PtrFromString: %w", err)
	}

	ret := setNamedSecurityInfoW(
		pathPtr,
		windows.SE_FILE_OBJECT,
		secInfo,
		nil,  // owner (unchanged)
		nil,  // group (unchanged)
		newDACL,
		nil, // SACL (unchanged)
	)
	if ret != 0 {
		return fmt.Errorf("SetNamedSecurityInfo: error code %d", ret)
	}

	return nil
}

// ApplyRestrictedDACL applies a protected DACL that grants Full Control to
// SYSTEM and Administrators. This is used for directories that must remain
// uninstallable by an elevated Administrator (e.g., C:\ProgramData\EDR root),
// even while subdirectories may be hardened to SYSTEM-only.
func ApplyRestrictedDACL(path string) error {
	return setRestrictedDACL(path)
}

// setNamedSecurityInfoW is a thin wrapper calling advapi32!SetNamedSecurityInfoW
// which is not yet exposed by golang.org/x/sys/windows.
var (
	modAdvapi32             = windows.NewLazyDLL("advapi32.dll")
	procSetNamedSecurityInf = modAdvapi32.NewProc("SetNamedSecurityInfoW")
)

func setNamedSecurityInfoW(
	objectName *uint16,
	objectType windows.SE_OBJECT_TYPE,
	secInfo windows.SECURITY_INFORMATION,
	owner, group *windows.SID,
	dacl, sacl *windows.ACL,
) uint32 {
	r, _, _ := procSetNamedSecurityInf.Call(
		uintptr(unsafe.Pointer(objectName)),
		uintptr(objectType),
		uintptr(secInfo),
		uintptr(unsafe.Pointer(owner)),
		uintptr(unsafe.Pointer(group)),
		uintptr(unsafe.Pointer(dacl)),
		uintptr(unsafe.Pointer(sacl)),
	)
	return uint32(r)
}

func enableTakeOwnershipPrivilegeLocal() error {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return err
	}
	defer token.Close()
	privName, _ := windows.UTF16PtrFromString("SeTakeOwnershipPrivilege")
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, privName, &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0].Luid = luid
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
	return windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
}

func setSystemOwnedExclusiveDACL(path string) error {
	const inheritFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	aces := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritFlags,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(sidSystem),
		},
	}}
	newDACL, err := windows.ACLFromEntries(aces, nil)
	if err != nil {
		return fmt.Errorf("ACLFromEntries: %w", err)
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("UTF16PtrFromString: %w", err)
	}
	const secInfo = windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	ret := setNamedSecurityInfoW(pathPtr, windows.SE_FILE_OBJECT, secInfo, sidSystem, nil, newDACL, nil)
	if ret != 0 {
		return fmt.Errorf("SetNamedSecurityInfo owner+dacl: error %d", ret)
	}
	return nil
}

// setSystemOwnedExclusiveFile applies the same SYSTEM-only model to a single file (no inheritance ACEs).
func setSystemOwnedExclusiveFile(path string) error {
	aces := []windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       0,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(sidSystem),
		},
	}}
	newDACL, err := windows.ACLFromEntries(aces, nil)
	if err != nil {
		return fmt.Errorf("ACLFromEntries: %w", err)
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("UTF16PtrFromString: %w", err)
	}
	const secInfo = windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	ret := setNamedSecurityInfoW(pathPtr, windows.SE_FILE_OBJECT, secInfo, sidSystem, nil, newDACL, nil)
	if ret != 0 {
		return fmt.Errorf("SetNamedSecurityInfo file: error %d", ret)
	}
	return nil
}

// HardenAgentDirectoriesExclusive sets OWNER=SYSTEM and a DACL that grants only
// SYSTEM full control (inheriting to subfolders). Administrators cannot change
// ACLs or modify contents until RestoreAgentDirectoriesACL during authorized
// uninstall. C:\ProgramData\EDR itself is not in the list so uninstall.dat
// can still be created by an elevated admin.
func HardenAgentDirectoriesExclusive(dirs []string, logger *logging.Logger) error {
	var first error
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if err := os.MkdirAll(dir, 0700); err != nil {
			err = fmt.Errorf("security: create dir %s: %w", dir, err)
			if logger != nil {
				logger.Warnf("[Security] %v", err)
			}
			if first == nil {
				first = err
			}
			continue
		}
		if err := setSystemOwnedExclusiveDACL(dir); err != nil {
			wrapped := fmt.Errorf("security: exclusive ACL %s: %w", dir, err)
			if logger != nil {
				logger.Warnf("[Security] %v", wrapped)
			}
			if first == nil {
				first = wrapped
			}
			continue
		}
		if filepath.Base(dir) == "bin" {
			exe := filepath.Join(dir, "edr-agent.exe")
			if st, err := os.Stat(exe); err == nil && !st.IsDir() {
				if err := setSystemOwnedExclusiveFile(exe); err != nil && logger != nil {
					logger.Warnf("[Security] exclusive ACL on binary: %v", err)
				}
			}
		}
		if logger != nil {
			logger.Infof("[Security] Directory locked (SYSTEM-only): %s", dir)
		}
	}
	return first
}

// RestoreAgentDirectoriesACL restores Administrators + SYSTEM full control and
// owner Administrators on each directory (and typical uninstall layout).
func RestoreAgentDirectoriesACL(dirs []string) error {
	_ = enableTakeOwnershipPrivilegeLocal()
	var last error
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		st, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			last = err
			continue
		}
		if !st.IsDir() {
			continue
		}
		pathPtr, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			last = err
			continue
		}
		ret := setNamedSecurityInfoW(pathPtr, windows.SE_FILE_OBJECT,
			windows.OWNER_SECURITY_INFORMATION, sidAdministrators, nil, nil, nil)
		if ret != 0 {
			last = fmt.Errorf("SetNamedSecurityInfo owner on %s: error %d", dir, ret)
			continue
		}
		if err := setRestrictedDACL(dir); err != nil {
			last = fmt.Errorf("restore DACL %s: %w", dir, err)
			continue
		}
		if filepath.Base(dir) == "bin" {
			exe := filepath.Join(dir, "edr-agent.exe")
			if st2, err := os.Stat(exe); err == nil && !st2.IsDir() {
				_ = enableTakeOwnershipPrivilegeLocal()
				exePtr, _ := windows.UTF16PtrFromString(exe)
				ret := setNamedSecurityInfoW(exePtr, windows.SE_FILE_OBJECT,
					windows.OWNER_SECURITY_INFORMATION, sidAdministrators, nil, nil, nil)
				if ret == 0 {
					_ = setRestrictedDACLOnFile(exe)
				}
			}
		}
	}
	return last
}

// setRestrictedDACLOnFile applies SYSTEM+Administrators full control to a file (no inheritance flags).
func setRestrictedDACLOnFile(path string) error {
	const fullControl = windows.GENERIC_ALL
	aces := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: fullControl,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       0,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidSystem),
			},
		},
		{
			AccessPermissions: fullControl,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       0,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidAdministrators),
			},
		},
	}
	newDACL, err := windows.ACLFromEntries(aces, nil)
	if err != nil {
		return err
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const secInfo = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	ret := setNamedSecurityInfoW(pathPtr, windows.SE_FILE_OBJECT, secInfo, nil, nil, newDACL, nil)
	if ret != 0 {
		return fmt.Errorf("SetNamedSecurityInfo: error %d", ret)
	}
	return nil
}
