// agent_security_windows.go — Windows-specific security bootstrap for the EDR Agent.
//
//go:build windows
// +build windows

package agent

import (
	"os"
	"path/filepath"
	"time"

	"github.com/edr-platform/win-agent/internal/security"
)

// initSecurity runs all security hardening steps:
//  1. NTFS ACL lockdown on all EDR data directories
//  2. Process self-protection (DACL hardening)
//  3. Encryption key initialisation
//  4. Data retention cleaner (48 h)
//  5. File integrity watchdog
//
// Errors are logged but NOT fatal — the agent should still run even if
// a security hardening step fails (e.g. running outside SYSTEM context
// during development).
func (a *Agent) initSecurity() {
	// ── 1. Harden directories ───────────────────────────────────────────────
	dirs := []string{
		`C:\ProgramData\EDR\queue`,
		`C:\ProgramData\EDR\logs`,
		`C:\ProgramData\EDR\certs`,
		`C:\ProgramData\EDR\config`,
		`C:\ProgramData\EDR\quarantine`,
	}
	if err := security.HardenDirectories(dirs, a.logger); err != nil {
		a.logger.Warnf("[Security] Directory hardening failed (agent continues): %v", err)
	}

	// ── 2. Process self-protection ──────────────────────────────────────────
	if err := security.ProtectProcess(a.logger); err != nil {
		a.logger.Warnf("[Security] Process protection failed (agent continues): %v", err)
	}

	// ── 3. Encryption key ───────────────────────────────────────────────────
	keyPath := `C:\ProgramData\EDR\config\.agent.key`
	enc, err := security.NewEncryptor(keyPath, a.logger)
	if err != nil {
		a.logger.Warnf("[Security] Encryption init failed (data-at-rest NOT encrypted): %v", err)
	} else {
		// Wire encryptor into disk queue and logger.
		a.diskQueue.SetEncryptor(enc)
		a.logger.SetEncryptor(enc)
		a.logger.Info("[Security] Data-at-rest encryption ACTIVE")

		// Req 5: Retroactively encrypt any plaintext log lines written before
		// the encryption key was ready (early startup logs).
		if err := a.logger.RetroEncryptExistingLog(); err != nil {
			a.logger.Warnf("[Security] Retroactive log encryption failed (non-fatal): %v", err)
		} else {
			a.logger.Info("[Security] Retroactive log encryption applied — startup logs secured")
		}
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
	var watchPaths []string
	if exe, err := os.Executable(); err == nil {
		watchPaths = append(watchPaths, exe)
	}
	if a.cfg.Certs.CertPath != "" {
		watchPaths = append(watchPaths, a.cfg.Certs.CertPath)
	}
	if a.cfg.Certs.CAPath != "" {
		watchPaths = append(watchPaths, a.cfg.Certs.CAPath)
	}
	configPath := filepath.Join(`C:\ProgramData\EDR\config`, "config.yaml")
	watchPaths = append(watchPaths, configPath)
	security.StartFileWatchdog(a.ctx, watchPaths, a.logger)
}
