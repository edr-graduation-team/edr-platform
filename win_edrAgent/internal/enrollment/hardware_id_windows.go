//go:build windows

package enrollment

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// GetHardwareID returns a stable device fingerprint for idempotent enrollment.
// On Windows we use MachineGuid (best-effort). If unavailable, returns an error.
func GetHardwareID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open MachineGuid key: %w", err)
	}
	defer k.Close()

	v, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return "", fmt.Errorf("read MachineGuid: %w", err)
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("MachineGuid empty")
	}
	return v, nil
}

