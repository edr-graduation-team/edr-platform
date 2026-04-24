// agent_security_windows.go — Windows-specific security bootstrap for the EDR Agent.
//
//go:build windows
// +build windows

package agent

import (
	"os"
	"time"

	"github.com/edr-platform/win-agent/internal/protection"
	"github.com/edr-platform/win-agent/internal/security"
)

// initSecurity runs all security hardening steps:
//  1. NTFS ACL lockdown on all EDR data directories
//  2. Process self-protection (SYSTEM-only DACL — same as service.Execute)
//  3. Encryption key initialisation
//  4. Data retention cleaner (48 h)
//  5. File integrity watchdog
//
// Errors are logged but NOT fatal — the agent should still run even if
// a security hardening step fails (e.g. running outside SYSTEM context
// during development).
func (a *Agent) initSecurity() {
	// ── 1. Harden directories (SYSTEM-only; matches service.Execute layer 5) ─
	// NOTE: certs/ and config/ are not on disk after enrollment; paths come
	// from config (queue_dir, log file parent, bin, quarantine, EncryptKey).
	if err := security.HardenAgentDirectoriesExclusive(a.cfg.DataDirectoriesToHarden(), a.logger); err != nil {
		a.logger.Warnf("[Security] Directory hardening failed (agent continues): %v", err)
	}

	// ── 2. Process self-protection ──────────────────────────────────────────
	// Use protection.ProtectProcess (not security.ProtectProcess) so the service
	// path and agent.Start share one DACL: SYSTEM full control only; avoids
	// initSecurity overwriting the stricter descriptor applied in service.Execute.
	if err := protection.ProtectProcess(); err != nil {
		a.logger.Warnf("[Security] Process protection failed (agent continues): %v", err)
	} else {
		a.logger.Info("[Security] Process DACL hardened — SYSTEM-only (aligned with service tamper layer)")
	}

	// ── 3. Encryption key ───────────────────────────────────────────────────
	keyPath := `C:\ProgramData\EDR\EncryptKey\.agent.key`
	enc, err := security.NewEncryptor(keyPath, a.logger)
	if err != nil {
		a.logger.Warnf("[Security] Encryption init failed (data-at-rest NOT encrypted): %v", err)
	} else {
		// Wire encryptor into disk queue.
		// Logs remain plaintext by design so Administrators can read them
		// (read-only) for operations while preserving tamper resistance via ACLs.
		a.diskQueue.SetEncryptor(enc)
		a.logger.Info("[Security] Data-at-rest encryption ACTIVE (queue)")
	}

	// Req 4: Start 24h log rotation (truncates log file every 24 hours).
	a.logger.StartLogRotation(a.ctx)
	a.logger.Info("[Security] 24h log rotation ACTIVE")

	// ── 4. Retention cleaner (48 h) ─────────────────────────────────────────
	retentionDirs := []string{
		`C:\ProgramData\EDR\queue`,
		`C:\ProgramData\EDR\logs`,
	}
	security.StartRetentionCleaner(a.ctx, retentionDirs, 48*time.Hour, a.logger)

	// ── 5. File integrity watchdog ──────────────────────────────────────────
	// Only watch the agent binary — cert files and config.yaml no longer
	// exist on disk (migrated to Registry).
	var watchPaths []string
	if exe, err := os.Executable(); err == nil {
		watchPaths = append(watchPaths, exe)
	}
	security.StartFileWatchdog(a.ctx, watchPaths, a.logger)
}
