//go:build windows
// +build windows

package responder

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func volumeSerialForPath(filePath string) string {
	vol := filepath.VolumeName(filePath)
	if vol == "" || len(vol) < 2 {
		return ""
	}
	deviceID := strings.ToUpper(vol[:2]) // D:

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ps := `(Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='` + deviceID + `'").VolumeSerialNumber`
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
