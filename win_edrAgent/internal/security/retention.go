// Package security — Automated data retention (48-hour cleanup).
//
//go:build windows
// +build windows

package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// StartRetentionCleaner launches a background goroutine that scans the listed
// directories every 15 minutes and deletes any file older than maxAge.
//
// Only files matching known EDR data patterns are removed:
//   - *.bin / *.bin.tmp   (queue files)
//   - agent.log.*         (rotated log files)
//
// The primary (current) log file `agent.log` is never deleted.
func StartRetentionCleaner(ctx context.Context, dirs []string, maxAge time.Duration, logger *logging.Logger) {
	if maxAge <= 0 {
		maxAge = 48 * time.Hour
	}
	if logger != nil {
		logger.Infof("[Retention] Cleaner started — scanning every 15 min, max age %v", maxAge)
	}

	go retentionLoop(ctx, dirs, maxAge, logger)
}

func retentionLoop(ctx context.Context, dirs []string, maxAge time.Duration, logger *logging.Logger) {
	// Run immediately on startup, then every 15 minutes.
	cleanDirs(dirs, maxAge, logger)

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if logger != nil {
				logger.Info("[Retention] Cleaner stopped")
			}
			return
		case <-ticker.C:
			cleanDirs(dirs, maxAge, logger)
		}
	}
}

func cleanDirs(dirs []string, maxAge time.Duration, logger *logging.Logger) {
	cutoff := time.Now().Add(-maxAge)
	var totalRemoved int

	for _, dir := range dirs {
		removed, err := cleanDir(dir, cutoff)
		if err != nil && logger != nil {
			logger.Warnf("[Retention] Error scanning %s: %v", dir, err)
		}
		totalRemoved += removed
	}

	if totalRemoved > 0 && logger != nil {
		logger.Infof("[Retention] Cleaned up %d expired files (older than %v)", totalRemoved, maxAge)
	}
}

func cleanDir(dir string, cutoff time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Only remove EDR data files — never touch unknown files.
		if !isRetentionCandidate(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}

// isRetentionCandidate returns true if the file matches a known EDR data
// pattern that should be subject to the retention policy.
func isRetentionCandidate(name string) bool {
	lower := strings.ToLower(name)

	// Queue files: <timestamp>_<batchid>.bin or .bin.tmp
	if strings.HasSuffix(lower, ".bin") || strings.HasSuffix(lower, ".bin.tmp") {
		return true
	}

	// Rotated log files: agent.log.<timestamp>
	// We never delete the primary log file itself ("agent.log").
	if strings.HasPrefix(lower, "agent.log.") {
		return true
	}

	return false
}
