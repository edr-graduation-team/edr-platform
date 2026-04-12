// Package security — Agent self-protection (anti-tampering).
//
//go:build windows
// +build windows

package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/edr-platform/win-agent/internal/logging"
)

// ProtectProcess hardens the current process against tampering.
//
// It modifies the process DACL so that non-SYSTEM/non-Admin users lose the
// ability to terminate, suspend, or inject into the agent process.
// SYSTEM and Administrators retain all access (necessary for SCM control).
//
// This is not foolproof against kernel-level attacks or a user running as
// SYSTEM, but it prevents standard-user malware from trivially killing
// the EDR agent via taskkill or TerminateProcess.
func ProtectProcess(logger *logging.Logger) error {
	handle, err := windows.GetCurrentProcess()
	if err != nil {
		return fmt.Errorf("GetCurrentProcess: %w", err)
	}

	// Build a restricted DACL for the process object.
	// SYSTEM: full control
	// Administrators: only PROCESS_QUERY_LIMITED_INFORMATION + SYNCHRONIZE
	// Everyone: only PROCESS_QUERY_LIMITED_INFORMATION + SYNCHRONIZE
	// (This denies PROCESS_TERMINATE, PROCESS_SUSPEND_RESUME, PROCESS_VM_WRITE
	// to ANY user other than SYSTEM.)
	sidEveryone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return fmt.Errorf("create Everyone SID: %w", err)
	}

	const processFullControl = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0xFFFF
	// PROCESS_QUERY_LIMITED_INFORMATION(0x1000) | SYNCHRONIZE(0x00100000)
	const processLimited = 0x1000 | windows.SYNCHRONIZE

	aces := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: processFullControl,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       0,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidSystem),
			},
		},
		{
			AccessPermissions: processLimited,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       0,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidAdministrators),
			},
		},
		{
			AccessPermissions: processLimited,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       0,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(sidEveryone),
			},
		},
	}

	dacl, err := windows.ACLFromEntries(aces, nil)
	if err != nil {
		return fmt.Errorf("ACLFromEntries: %w", err)
	}

	const secInfo = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION

	ret := setSecurityInfoByHandle(
		handle,
		windows.SE_KERNEL_OBJECT,
		secInfo,
		nil, nil,
		dacl, nil,
	)
	if ret != 0 {
		return fmt.Errorf("SetSecurityInfo on process: error code %d", ret)
	}

	if logger != nil {
		logger.Info("[Security] Process DACL hardened — non-admin users cannot terminate agent")
	}
	return nil
}

// StartFileWatchdog launches a goroutine that checks the integrity of critical
// agent files every 30 seconds. If any file is modified after the initial hash
// is computed, it logs a HIGH severity tamper alert.
//
// Monitored files typically include:
//   - The agent executable itself
//   - The config file
//   - The CA / client certificates
func StartFileWatchdog(ctx context.Context, paths []string, logger *logging.Logger) {
	if len(paths) == 0 || logger == nil {
		return
	}

	// Compute initial baseline hashes.
	baseline := make(map[string]string, len(paths))
	for _, p := range paths {
		if h, err := hashFile(p); err == nil {
			baseline[p] = h
		}
	}

	logger.Infof("[Watchdog] File integrity watchdog started — monitoring %d files", len(baseline))
	go fileWatchdogLoop(ctx, baseline, logger)
}

func fileWatchdogLoop(ctx context.Context, baseline map[string]string, logger *logging.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var alertOnce sync.Map // path → struct{}: prevent repeated alerts for same file

	for {
		select {
		case <-ctx.Done():
			logger.Info("[Watchdog] File integrity watchdog stopped")
			return
		case <-ticker.C:
			for path, expected := range baseline {
				current, err := hashFile(path)
				if err != nil {
					// File deleted or inaccessible — treat as tamper.
					if _, alerted := alertOnce.LoadOrStore(path, struct{}{}); !alerted {
						logger.Errorf("[TAMPER ALERT] File inaccessible or deleted: %s (error: %v)", path, err)
					}
					continue
				}
				if current != expected {
					if _, alerted := alertOnce.LoadOrStore(path, struct{}{}); !alerted {
						logger.Errorf("[TAMPER ALERT] File modified: %s (expected %s, got %s)", path, expected[:16], current[:16])
					}
				}
			}
		}
	}
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// setSecurityInfoByHandle wraps advapi32!SetSecurityInfo (handle-based variant).
var procSetSecurityInfo = modAdvapi32.NewProc("SetSecurityInfo")

func setSecurityInfoByHandle(
	handle windows.Handle,
	objectType windows.SE_OBJECT_TYPE,
	secInfo windows.SECURITY_INFORMATION,
	owner, group *windows.SID,
	dacl, sacl *windows.ACL,
) uint32 {
	r, _, _ := procSetSecurityInfo.Call(
		uintptr(handle),
		uintptr(objectType),
		uintptr(secInfo),
		uintptr(unsafe.Pointer(owner)),
		uintptr(unsafe.Pointer(group)),
		uintptr(unsafe.Pointer(dacl)),
		uintptr(unsafe.Pointer(sacl)),
	)
	return uint32(r)
}
