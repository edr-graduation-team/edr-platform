//go:build windows
// +build windows

package collectors

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// removableRootsLister is implemented by *responder.Engine.
type removableRootsLister interface {
	RegisterRemovableRoot(root string)
}

// StartUSBVolumeWatcher polls for removable drives and registers them with the responder.
func StartUSBVolumeWatcher(ctx context.Context, eng removableRootsLister, logger *logging.Logger) {
	if eng == nil || logger == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	prev := map[string]struct{}{}
	syncRoots := func() {
		cur := map[string]struct{}{}
		for _, d := range listRemovableDrives(ctx) {
			cur[d] = struct{}{}
			if _, ok := prev[d]; !ok {
				eng.RegisterRemovableRoot(d)
				logger.Infof("[USB] Removable volume detected: %s", d)
			}
		}
		prev = cur
	}

	syncRoots()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncRoots()
		}
	}
}

func listRemovableDrives(ctx context.Context) []string {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-CimInstance Win32_LogicalDisk | Where-Object { $_.DriveType -eq 2 -and $_.DeviceID } | ForEach-Object { $_.DeviceID }").Output()
	if err != nil {
		return nil
	}
	var drives []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if len(line) >= 2 && line[1] == ':' {
			drives = append(drives, strings.ToUpper(line[:2]))
		}
	}
	return drives
}
