//go:build windows

package enrollment

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
)

// GetHardwareID returns a stable device fingerprint for idempotent enrollment.
// On Windows we prefer MachineGuid. If unavailable (rare, but can happen under
// hardened registry policies), we fall back to a locally-persisted value under
// HKLM\SOFTWARE\EDR\Agent so the agent can still enroll reliably.
func GetHardwareID() (string, error) {
	// 1) Primary: MachineGuid
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("MachineGuid"); err == nil {
			v = strings.TrimSpace(v)
			if v != "" {
				return v, nil
			}
		}
	}

	// 2) Fallback: locally persisted hardware id (stable across reboots).
	const edrKeyPath = `SOFTWARE\EDR\Agent`
	const valueName = "HardwareID"
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, edrKeyPath, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue(valueName); err == nil {
			v = strings.TrimSpace(v)
			if v != "" {
				return v, nil
			}
		}
	}

	// 3) Last resort: generate + persist (requires SYSTEM/admin at install/runtime).
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, edrKeyPath, registry.SET_VALUE)
	if err != nil {
		return "", fmt.Errorf("create/open fallback hardware id key: %w", err)
	}
	defer k.Close()
	id := uuid.NewString()
	if err := k.SetStringValue(valueName, id); err != nil {
		return "", fmt.Errorf("persist fallback hardware id: %w", err)
	}
	return id, nil
}

