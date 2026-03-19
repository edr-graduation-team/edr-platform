// Package security provides NTFS ACL hardening, encryption, retention, and
// self-protection for the EDR Agent on Windows.
//
//go:build windows
// +build windows

package security

import (
	"fmt"
	"os"
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
