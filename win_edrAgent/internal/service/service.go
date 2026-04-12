// Package service provides Windows Service integration for the EDR Agent.
//go:build windows
// +build windows

package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/edr-platform/win-agent/internal/agent"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/enrollment"
	"github.com/edr-platform/win-agent/internal/logging"
	"github.com/edr-platform/win-agent/internal/protection"
	"github.com/edr-platform/win-agent/internal/security"
)

const (
	ServiceName        = "EDRAgent"
	ServiceDisplayName = "EDR Agent Service"
	ServiceDescription = "Endpoint Detection and Response Agent - Collects security events and provides threat protection"

)

// edrService implements the Windows service interface.
type edrService struct {
	configPath        string         // path to config.yaml — loaded inside Execute()
	cfg               *config.Config // populated during Execute() after StartPending
	logger            *logging.Logger
	agent             *agent.Agent
	embeddedTokenHash string         // passed down from main() for runtime uninstall verification
}

// Execute is the Windows Service control handler.
//
// SCM Contract (critical):
//   - StartPending must be sent within ~30 s of Execute() being called.
//   - Running must be sent before any blocking I/O that could time out (TLS
//     handshakes, network calls, enrollment).
//   - If the agent cannot start after reporting Running, transition to
//     StopPending → return(false, non-zero) so the SCM logs a clean failure
//     rather than a 1053 "did not respond in a timely fashion" error.
//
// Startup sequence:
//  1. StartPending — acknowledge the SCM immediately.
//  2. Prepare the agent struct and wire callbacks (pure in-memory, cannot fail).
//  3. Running — satisfy the SCM contract BEFORE any network / TLS work.
//  4. Async goroutine:  CA fetch → enrollment → agent.Start()
//     On any failure: signal agentErrCh so the control loop can shut down cleanly.
func (s *edrService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	// ── Panic recovery: ensure SCM always gets a clean status transition ────
	// Without this, an unrecovered panic in Execute() would crash the service
	// process, causing an ungraceful termination visible as Error 1.
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Errorf("[SCM] PANIC recovered in Execute: %v", rec)
		}
	}()

	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	// ── 1. StartPending — satisfy SCM within the startup timeout window ────────
	changes <- svc.Status{State: svc.StartPending}

	// ── 1b. Load configuration ─────────────────────────────────────────────────
	// Priority: Registry (secure, SYSTEM-only) > YAML file (fallback, first boot only)
	//
	// After the first successful boot, config.yaml is migrated to the
	// protected Registry key and DELETED from disk. This ensures:
	//   - No plaintext config file visible to Administrators
	//   - Config survives file system tampering
	//   - Only SYSTEM can read the configuration
	var (
		cfg *config.Config
		err error
	)

	// Try Registry first (primary source after initial migration)
	cfg, err = config.LoadFromRegistry()
	if err != nil {
		s.logger.Warnf("[SCM] Registry config corrupted, falling back to YAML: %v", err)
		cfg = nil
	}
	if cfg != nil {
		s.logger.Info("[SCM] Config loaded from protected Registry (no YAML needed)")
		// Clean up any orphaned YAML from a crashed/aborted install
		if err := config.DeleteConfigFile(s.configPath); err == nil {
			s.logger.Info("[SCM] Cleaned up orphaned config.yaml from disk")
		}
		
		// Clean up any orphaned certificate files from an overlaid install
		_ = os.Remove(`C:\ProgramData\EDR\client.crt`)
		_ = os.Remove(`C:\ProgramData\EDR\private.key`)
		_ = os.Remove(`C:\ProgramData\EDR\ca-chain.crt`)
	} else {
		// Fallback: load from YAML file (first boot / fresh install)
		cfg, err = config.Load(s.configPath)
		if err != nil {
			s.logger.Errorf("[SCM] Failed to load configuration from %s: %v", s.configPath, err)
			changes <- svc.Status{State: svc.StopPending}
			return false, 2 // service-specific error code 2 = config load failure
		}
		s.logger.Infof("[SCM] Config loaded from YAML file: %s", s.configPath)

		// Migrate to Registry and delete YAML file
		if err := cfg.SaveToRegistry(); err != nil {
			s.logger.Warnf("[SCM] Failed to migrate config to Registry: %v", err)
		} else {
			s.logger.Info("[SCM] Config migrated to protected Registry")
			// Delete the plaintext YAML file — no longer needed
			if err := config.DeleteConfigFile(s.configPath); err != nil {
				s.logger.Warnf("[SCM] Failed to delete config YAML: %v", err)
			} else {
				s.logger.Info("[SCM] config.yaml deleted from disk (migrated to Registry)")
			}
		}
	}

	s.cfg = cfg
	s.logger.SetLevel(cfg.Logging.Level)
	s.logger.Infof("[SCM] Config active: server=%s agent=%s", cfg.Server.Address, cfg.Agent.ID)

	// ── 2. Prepare agent (pure in-memory, cannot fail) ────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.agent, err = agent.New(s.cfg, s.logger)
	if err != nil {
		s.logger.Errorf("[SCM] Failed to construct agent: %v", err)
		changes <- svc.Status{State: svc.StopPending}
		return false, 3 // service-specific error code 3 = agent init failure
	}

	s.agent.SetConfigFilePath(s.configPath)
	s.agent.SetRestartInfo(s.configPath)
	s.agent.SetConfigUpdateHandler(s.agent.UpdateConfig)

	// ── 3. Running — must be sent BEFORE any network / TLS / enrollment work ──
	// This satisfies the SCM contract. All subsequent work is async.
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	s.logger.Info("[SCM] Service reported Running — starting async enrollment + agent")
	s.logger.Infof("[SCM] Config path: %s", s.configPath)

	// ── Anti-Tamper Layer 1: Process DACL ─────────────────────────────────
	// Prevents taskkill, Task Manager, TerminateProcess from non-SYSTEM.
	// Must be called AFTER Running is reported (so SCM doesn't timeout).
	if err := protection.ProtectProcess(); err != nil {
		s.logger.Warnf("[SCM] Process self-protection failed (non-fatal): %v", err)
	} else {
		s.logger.Info("[SCM] Process self-protection enabled — tamper-resistant")
	}

	// ── Anti-Tamper Layer 2: Service DACL Hardening ────────────────────────
	// Prevents Administrators from stopping or deleting the service via:
	//   - sc stop EDRAgent      → ACCESS DENIED
	//   - sc delete EDRAgent    → ACCESS DENIED
	//   - Stop-Service EDRAgent → ACCESS DENIED
	//   - net stop EDRAgent     → ACCESS DENIED
	// Only SYSTEM retains full control. This is restored during uninstall
	// via RestoreServiceDACL() before the service can be stopped.
	if err := protection.HardenServiceDACL(ServiceName); err != nil {
		s.logger.Warnf("[SCM] Service DACL hardening failed (non-fatal): %v", err)
	} else {
		s.logger.Info("[SCM] Service DACL hardened — only SYSTEM can stop/delete")
	}

	// ── Anti-Tamper Layer 3: Registry Key Protection ───────────────────
	// Protects HKLM\SYSTEM\CurrentControlSet\Services\EDRAgent AND all
	// numbered ControlSets (ControlSet001, 002...) from direct registry
	// deletion by Administrators via regedit, reg.exe, or PowerShell.
	if err := protection.HardenServiceRegistryKey(ServiceName); err != nil {
		s.logger.Warnf("[SCM] Registry key hardening failed (non-fatal): %v", err)
	} else {
		s.logger.Info("[SCM] Service registry keys hardened (all ControlSets) — reg delete blocked")
	}

	// ── Anti-Tamper Layer 4: Agent Config Registry Protection ─────────
	// Protects HKLM\SOFTWARE\EDR\Agent (token hash, critical config backup)
	if err := protection.HardenAgentRegistryKey(); err != nil {
		s.logger.Warnf("[SCM] Agent registry key hardening failed (non-fatal): %v", err)
	} else {
		s.logger.Info("[SCM] Agent config registry hardened — SYSTEM full, Admin read-only")
	}

	// ── Anti-Tamper Layer 5: NTFS — SYSTEM-only on agent data dirs + binary ─
	if s.cfg != nil {
		if err := security.HardenAgentDirectoriesExclusive(s.cfg.DataDirectoriesToHarden(), s.logger); err != nil {
			s.logger.Warnf("[SCM] Agent directory hardening failed (non-fatal): %v", err)
		} else {
			s.logger.Info("[SCM] Agent data directories hardened (SYSTEM-only, uninstall restores ACLs)")
		}
	}

	// ── Continuous Protection Watchdog: Process DACL ──────────────────
	// Re-applies process DACL every 10 seconds. If an attacker manages to
	// remove the DACL (e.g., via a SYSTEM-level tool), this goroutine
	// will restore it within 10 seconds.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := protection.ProtectProcess(); err != nil {
					s.logger.Warnf("[WATCHDOG] Process DACL re-apply failed: %v", err)
				}
			}
		}
	}()
	s.logger.Info("[SCM] Process DACL watchdog started (re-apply every 10s)")

	// ── Continuous Protection Watchdog: Registry Integrity ────────────
	// Checks every 5 seconds that the service registry key AND agent config
	// key still exist and are hardened. Re-applies if tampered.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Re-harden service registry keys (all ControlSets)
				_ = protection.HardenServiceRegistryKey(ServiceName)
				// Re-harden agent config registry key
				_ = protection.HardenAgentRegistryKey()
				// Re-apply NTFS ACLs if an admin reset icacls / ownership
				if s.cfg != nil {
					_ = security.HardenAgentDirectoriesExclusive(s.cfg.DataDirectoriesToHarden(), nil)
				}
			}
		}
	}()
	s.logger.Info("[SCM] Registry + directory integrity watchdog started (every 5s)")

	// ── Save critical config to protected Registry ───────────────────
	// This runs after config is loaded, providing a tamper-proof backup.
	go func() {
		// Wait for config to be fully loaded (after enrollment)
		time.Sleep(15 * time.Second)
		if s.cfg != nil {
			if err := protection.SaveCriticalConfig(
				s.cfg.Server.Address,
				s.cfg.Agent.ID,
				s.cfg.Certs.CAPath,
				s.cfg.Certs.CertPath,
				s.cfg.Certs.KeyPath,
			); err != nil {
				s.logger.Warnf("[SCM] Failed to save critical config to registry: %v", err)
			} else {
				s.logger.Info("[SCM] Critical config backed up to protected registry")
				_ = protection.HardenAgentRegistryKey()
			}
		}
	}()

	// ── Uninstall File Watcher ─────────────────────────────────────────
	// Polls for C:\ProgramData\EDR\uninstall.dat every second.
	// When a valid token hash is found, the service (SYSTEM) restores DACL
	// and registry permissions, then signals the control loop to stop.
	uninstallCh := make(chan struct{}, 1)
	go s.watchUninstallFile(ctx, uninstallCh)

	// agentErrCh carries the outcome of the async startup sequence.
	// A nil value means the agent is running; a non-nil value means it failed.
	agentErrCh := make(chan error, 1)

	// ── 4. Async startup goroutine ─────────────────────────────────────────────
	go func() {
		// 4a. Provision CA certificate — embedded if available, else HTTP fetch
		if !s.cfg.Server.Insecure && s.cfg.Certs.CAPath != "" {
			// Skip if CA cert is already loaded from the Registry
			if len(s.cfg.Certs.CACertPEM) == 0 {
				if err := enrollment.EnsureCACertificate(s.cfg.Server.Address, s.cfg.Certs.CAPath, s.logger); err != nil {
					s.logger.Warnf("[SCM] CA provisioning failed (using existing cert if present): %v", err)
				}
			} else {
				s.logger.Info("[SCM] CA certificate already loaded from Registry (skipping disk file provision)")
			}
		}

		// 4b. Enrollment — this is where TLS handshakes happen.
		// On failure we log the error and signal the control loop to stop cleanly.
		if err := ensureEnrolled(s.cfg, s.logger, s.configPath); err != nil {
			s.logger.Errorf("[SCM] Enrollment failed — service will stop: %v", err)
			agentErrCh <- err
			cancel()
			return
		}

		// 4c. Start the agent collectors and gRPC pipeline.
		if err := s.agent.Start(ctx); err != nil {
			s.logger.Errorf("[SCM] Agent start failed — service will stop: %v", err)
			agentErrCh <- err
			cancel()
			return
		}

		agentErrCh <- nil // signal: agent is live
	}()

	// ── 5. Control loop — handle SCM commands + startup failures ───────────────
	var startupDone bool
	for {
		select {
		case err := <-agentErrCh:
			startupDone = true
			if err != nil {
				// Startup failed: tell SCM we are stopping with an error code
				// so it records a proper "service terminated unexpectedly" event
				// instead of a confusing 1053 timeout.
				s.logger.Errorf("[SCM] Async startup failed: %v — transitioning to Stopped", err)
				changes <- svc.Status{State: svc.StopPending}
				// errno 1 → service-specific error, visible in Windows Event Log
				return false, 1
			}
			s.logger.Info("[SCM] Agent startup complete — fully operational")

		case <-uninstallCh:
			s.logger.Info("[SCM] Uninstall confirmed — stopping service")
			changes <- svc.Status{State: svc.StopPending}
			cancel()
			if startupDone && s.agent != nil {
				done := make(chan struct{})
				go func() {
					s.agent.Stop()
					close(done)
				}()
				select {
				case <-done:
					s.logger.Info("[SCM] Agent stopped gracefully for uninstall")
				case <-time.After(30 * time.Second):
					s.logger.Warn("[SCM] Agent stop timed out after 30s")
				}
			}
			return false, 0

		case c, ok := <-r:
			if !ok {
				return false, 0
			}
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus

			case svc.Stop, svc.Shutdown:
				s.logger.Info("[SCM] Stop/Shutdown requested")
				changes <- svc.Status{State: svc.StopPending}
				cancel()

				// Only try to stop the agent if it actually started.
				if startupDone && s.agent != nil {
					done := make(chan struct{})
					go func() {
						s.agent.Stop()
						close(done)
					}()
					select {
					case <-done:
						s.logger.Info("[SCM] Agent stopped gracefully")
					case <-time.After(30 * time.Second):
						s.logger.Warn("[SCM] Agent stop timed out after 30s")
					}
				}
				return false, 0

			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
				s.logger.Info("[SCM] Service paused")

			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
				s.logger.Info("[SCM] Service resumed")

			default:
				s.logger.Warnf("[SCM] Unexpected control request: %d", c.Cmd)
			}
		}
	}

	// This is structurally unreachable (the for-select loop returns internally),
	// but required by Go's compiler for the function signature.
	return false, 0
}

// Run starts the service.
//
// configPath is loaded inside Execute() (not here) so that svc.Run()
// registers the service handler with the SCM before any config I/O.
// This prevents "Error 1: Incorrect function" if config loading fails.
func Run(configPath string, logger *logging.Logger, embeddedTokenHash string) error {
	logger.Infof("Initializing Windows Service (config=%s)...", configPath)

	// Check if running as a service
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to check if running as service: %w", err)
	}

	if !isService {
		return fmt.Errorf("not running as a Windows service")
	}

	// Run the service — config loading is deferred to Execute().
	err = svc.Run(ServiceName, &edrService{
		configPath:        configPath,
		logger:            logger,
		embeddedTokenHash: embeddedTokenHash,
	})
	if err != nil {
		return fmt.Errorf("service run failed: %w", err)
	}

	return nil
}

// ensureEnrolled is a thin wrapper over enrollment.EnsureEnrolled used by Execute()
// to perform full certificate bootstrap inside the async startup goroutine,
// AFTER the SCM has already received svc.Running.
func ensureEnrolled(cfg *config.Config, logger *logging.Logger, configPath string) error {
	return enrollment.EnsureEnrolled(cfg, logger, configPath)
}

// watchUninstallFile polls for C:\ProgramData\EDR\uninstall.dat every 2 seconds.
// When a file is found containing a valid SHA-256 token hash, the service
// (running as SYSTEM) restores the service DACL and registry key permissions,
// then signals the main control loop to initiate a clean shutdown.
//
// This file-based IPC approach is used instead of Custom Control Codes because
// Go's svc package does not reliably forward user-defined control codes (128+)
// to the Execute handler.
func (s *edrService) watchUninstallFile(ctx context.Context, uninstallCh chan<- struct{}) {
	const uninstallFile = `C:\ProgramData\EDR\uninstall.dat`
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(uninstallFile)
			if err != nil {
				continue // file doesn't exist — normal operation
			}
			hash := strings.TrimSpace(string(data))
			if err := protection.VerifyUninstallHash(hash, s.embeddedTokenHash); err != nil {
				s.logger.Warnf("[UNINSTALL] Invalid hash in uninstall.dat: %v", err)
				_ = os.Remove(uninstallFile)
				continue
			}

			// ── Valid hash — authorize uninstall ──────────────────────────
			s.logger.Info("[UNINSTALL] Valid uninstall token hash detected")

			// CRITICAL: Disable recovery actions FIRST so the SCM does not
			// automatically restart the service after we exit Execute().
			// Without this, the service stops → SCM restarts it immediately
			// (Delay: 0) → new instance re-hardens DACL → appears as if
			// the service never stopped.
			if scm, mgrErr := mgr.Connect(); mgrErr == nil {
				if svcHandle, svcErr := scm.OpenService(ServiceName); svcErr == nil {
					noRestart := []mgr.RecoveryAction{
						{Type: mgr.NoAction, Delay: 0},
						{Type: mgr.NoAction, Delay: 0},
						{Type: mgr.NoAction, Delay: 0},
					}
					_ = svcHandle.SetRecoveryActions(noRestart, 0)
					svcHandle.Close()
					s.logger.Info("[UNINSTALL] Recovery actions disabled — no auto-restart")
				}
				scm.Disconnect()
			}

			// Restore service DACL (allow Administrators to stop/delete)
			if err := protection.RestoreServiceDACL(ServiceName); err != nil {
				s.logger.Errorf("[UNINSTALL] RestoreServiceDACL failed: %v", err)
			} else {
				s.logger.Info("[UNINSTALL] Service DACL restored — Admin access re-enabled")
			}

			// Restore registry key permissions
			if err := protection.RestoreServiceRegistryKey(ServiceName); err != nil {
				s.logger.Errorf("[UNINSTALL] RestoreServiceRegistryKey failed: %v", err)
			} else {
				s.logger.Info("[UNINSTALL] Service registry key DACL restored")
			}

			// Restore agent config registry key (allow Admin to delete during cleanup)
			if err := protection.RestoreAgentRegistryKey(); err != nil {
				s.logger.Errorf("[UNINSTALL] RestoreAgentRegistryKey failed: %v", err)
			} else {
				s.logger.Info("[UNINSTALL] Agent config registry key DACL restored")
			}

			if s.cfg != nil {
				if err := security.RestoreAgentDirectoriesACL(s.cfg.DataDirectoriesToHarden()); err != nil {
					s.logger.Warnf("[UNINSTALL] RestoreAgentDirectoriesACL: %v", err)
				} else {
					s.logger.Info("[UNINSTALL] Agent data directory ACLs restored for cleanup")
				}
			}

			_ = os.Remove(uninstallFile)
			// Clean up any leftover debug/temp files
			_ = os.Remove(`C:\ProgramData\EDR\uninstall_debug.txt`)
			select {
			case uninstallCh <- struct{}{}:
			default:
			}
			return
		}
	}
}

// ServiceExists checks if EDRAgent is registered in the SCM using minimal
// permissions (SERVICE_QUERY_STATUS). This works even when the service DACL
// is hardened, unlike mgr.OpenService() which requires SERVICE_ALL_ACCESS.
func ServiceExists() bool {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(scm)
	svcNamePtr, _ := windows.UTF16PtrFromString(ServiceName)
	h, err := windows.OpenService(scm, svcNamePtr, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	windows.CloseServiceHandle(h)
	return true
}

// Install installs the Windows service.
// embeddedTokenHash is saved to a protected registry key for uninstall verification.
func Install(embeddedTokenHash string) error {
	// ── Pre-flight Check ────────────────────────────────────────────────
	// Check if service already exists BEFORE attempting any file operations.
	// If it does, we return an 'already exists' error immediately so the caller
	// (runInstall) can cleanly stop and uninstall it. This prevents "Access Denied"
	// and "File in use" errors when trying to overwrite a running agent binary.
	if ServiceExists() {
		return fmt.Errorf("service %s already exists", ServiceName)
	}
	srcPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// ── Create required directories (including secure bin dir) ────────────
	dirs := []string{
		"C:\\ProgramData\\EDR",
		"C:\\ProgramData\\EDR\\bin",
		"C:\\ProgramData\\EDR\\logs",
		"C:\\ProgramData\\EDR\\queue",
		"C:\\ProgramData\\EDR\\quarantine",
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0700)
	}

	// ── Relocate EXE to secure path ──────────────────────────────────────
	// Copy the agent binary to C:\ProgramData\EDR\bin\ which will be
	// protected by SYSTEM-only DACL. The service is registered to run from
	// this secure path, so even if the original download is deleted, the
	// agent survives reboots.
	dstPath := filepath.Join("C:\\ProgramData\\EDR\\bin", "edr-agent.exe")
	if !strings.EqualFold(srcPath, dstPath) {
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy agent to secure path: %w", err)
		}
		fmt.Printf("      Agent binary secured: %s\n", dstPath)
	}

	// Connect to service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// ── Register service from the SECURE path (not from Downloads!) ──────
	s, err := m.CreateService(ServiceName, dstPath, mgr.Config{
		DisplayName:      ServiceDisplayName,
		Description:      ServiceDescription,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "LocalSystem",
	},
		"-service",
		"-config", "C:\\ProgramData\\EDR\\config.yaml",
	)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Configure recovery options (restart on failure)
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 0},
		{Type: mgr.ServiceRestart, Delay: 0},
		{Type: mgr.ServiceRestart, Delay: 0},
	}
	if err := s.SetRecoveryActions(recoveryActions, 24*60*60); err != nil {
		fmt.Printf("Warning: failed to set recovery actions: %v\n", err)
	}

	// Create event log source
	if err := eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		fmt.Printf("Warning: failed to create event log source: %v\n", err)
	}

	// ── Save token hash to protected Registry ────────────────────────────
	// This ensures uninstall verification works even if the EXE is replaced
	// or the embedded hash is somehow lost.
	if err := protection.SaveTokenHashToRegistry(embeddedTokenHash); err != nil {
		fmt.Printf("Warning: failed to save token hash to registry: %v\n", err)
	}

	// NOTE: HardenAgentRegistryKey is NOT called here because Install()
	// runs as Administrator, which cannot set OWNER=SYSTEM. The hardening
	// is applied in Execute() which runs as SYSTEM.

	return nil
}

// copyFile copies a file from src to dst, preserving content.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// Uninstall removes the Windows service. Requires a valid anti-tamper token.
// embeddedTokenHash is the SHA-256 hash of the enrollment token, compiled into
// the binary. If the registry also contains a stored hash, it is used as the
// primary source (more tamper-resistant than the embedded value).
//
// Flow:
//  1. Determine the authoritative token hash (registry > embedded).
//  2. Verify the plaintext token against the authoritative hash.
//  3. Check if the service is running:
//     a. Running  → write uninstall.dat, wait for the SYSTEM watcher to restore DACL.
//     b. Stopped  → restore DACL directly from Administrator context (no watcher available).
//  4. Delete the service from the SCM.
func Uninstall(token, embeddedTokenHash string) error {
	// ── Resolve authoritative token hash ────────────────────────────────────
	// Priority: Registry (tamper-proof) > Embedded in EXE > legacy default
	authoritativeHash := protection.ReadTokenHashFromRegistry()
	if authoritativeHash == "" {
		authoritativeHash = embeddedTokenHash
	}

	// ── Anti-Tamper: Verify uninstall token ──────────────────────────────────
	if err := protection.VerifyUninstallToken(token, authoritativeHash); err != nil {
		return fmt.Errorf("uninstall blocked: %w", err)
	}

	fmt.Println("  Token verified. Checking service state...")

	// ── Check if the service is actually running ─────────────────────────────
	// If the service is stopped (e.g., enrollment failed on first boot), there
	// is no watcher goroutine to detect uninstall.dat. In that case, we must
	// restore DACL/registry permissions DIRECTLY from Administrator context.
	state, stateErr := Status()
	serviceIsStopped := stateErr != nil || state == svc.Stopped

	if serviceIsStopped {
		// ── Stopped-service path: restore protections directly ──────────
		fmt.Println("  Service is stopped — restoring protections directly...")
		if err := protection.RestoreServiceDACL(ServiceName); err != nil {
			fmt.Printf("  Warning: DACL restore: %v\n", err)
		} else {
			fmt.Println("  Service DACL restored.")
		}
		if err := protection.RestoreServiceRegistryKey(ServiceName); err != nil {
			fmt.Printf("  Warning: Registry restore: %v\n", err)
		} else {
			fmt.Println("  Service registry keys restored.")
		}
		_ = protection.RestoreAgentRegistryKey()
		if err := security.RestoreAgentDirectoriesACL(config.DefaultConfig().DataDirectoriesToHarden()); err != nil {
			fmt.Printf("  Warning: directory ACL restore: %v\n", err)
		}
	} else {
		// ── Running-service path: signal via uninstall.dat ───────────────
		providedHash := protection.HashUninstallToken(token)
		hashFilePath := filepath.Join("C:\\ProgramData\\EDR", "uninstall.dat")
		if err := os.WriteFile(hashFilePath, []byte(providedHash), 0600); err != nil {
			return fmt.Errorf("failed to write uninstall signal: %w", err)
		}

		fmt.Println("  Signaling running service to release protections...")

		// Poll until the service stops
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			st, err := Status()
			if err != nil || st == svc.Stopped {
				break
			}
			if i%5 == 4 {
				_ = os.WriteFile(hashFilePath, []byte(providedHash), 0600)
				fmt.Printf("  Waiting for service to stop... (%d/30s)\n", i+1)
			}
		}
		_ = os.Remove(hashFilePath)

		// After signaling, also attempt direct DACL restore in case the
		// watcher didn't fully complete before the service stopped.
		_ = protection.RestoreServiceDACL(ServiceName)
		_ = protection.RestoreServiceRegistryKey(ServiceName)
		_ = protection.RestoreAgentRegistryKey()
		_ = security.RestoreAgentDirectoriesACL(config.DefaultConfig().DataDirectoriesToHarden())
	}

	fmt.Println("  Removing service registration...")

	// Clean up residual files and agent registry key
	cleanupResidualFiles()
	protection.CleanAgentRegistryKey()

	return forceRemoveService()
}

// ForceUninstall removes the service WITHOUT token verification.
// This is ONLY called from runInstall() during a re-install, which is already
// a privileged operation (requires admin + enrollment token).
// It is NOT exposed via any CLI flag.
//
// Handles both service states:
//   - Running: signals via uninstall.dat, waits for watcher to restore DACL.
//   - Stopped: restores DACL directly from Administrator context.
func ForceUninstall(embeddedTokenHash string) error {
	// ── Check if the service is actually running ─────────────────────────────
	state, stateErr := Status()
	serviceIsStopped := stateErr != nil || state == svc.Stopped

	if serviceIsStopped {
		// ── Stopped-service path: restore protections directly ──────────
		fmt.Println("      Service is stopped — restoring protections directly...")
		_ = protection.RestoreServiceDACL(ServiceName)
		_ = protection.RestoreServiceRegistryKey(ServiceName)
		_ = protection.RestoreAgentRegistryKey()
		_ = security.RestoreAgentDirectoriesACL(config.DefaultConfig().DataDirectoriesToHarden())
	} else {
		// ── Running-service path: signal via uninstall.dat ───────────────
		hashFilePath := filepath.Join("C:\\ProgramData\\EDR", "uninstall.dat")
		hash := embeddedTokenHash
		if hash == "" {
			hash = protection.HashUninstallToken("EDR-Uninstall-2026!")
		}
		_ = os.WriteFile(hashFilePath, []byte(hash), 0600)

		fmt.Println("      Signaling running service to release protections...")
		for i := 0; i < 15; i++ {
			time.Sleep(1 * time.Second)
			st, err := Status()
			if err != nil || st == svc.Stopped {
				break
			}
			if i%5 == 4 {
				_ = os.WriteFile(hashFilePath, []byte(hash), 0600)
			}
		}
		_ = os.Remove(hashFilePath)

		// Belt-and-suspenders: also restore DACL in case the watcher did
		// not fully complete.
		_ = protection.RestoreServiceDACL(ServiceName)
		_ = protection.RestoreServiceRegistryKey(ServiceName)
		_ = protection.RestoreAgentRegistryKey()
		_ = security.RestoreAgentDirectoriesACL(config.DefaultConfig().DataDirectoriesToHarden())
	}

	// Clean up residual files
	cleanupResidualFiles()

	return forceRemoveService()
}

// cleanupResidualFiles removes temporary files that may be left behind
// by the uninstall process or by user errors (e.g., PowerShell's sc alias
// creates files named 'stop' and 'delete' instead of calling sc.exe).
func cleanupResidualFiles() {
	residual := []string{
		`C:\ProgramData\EDR\uninstall.dat`,
		`C:\ProgramData\EDR\uninstall_debug.txt`,
		`C:\ProgramData\EDR\stop`,
		`C:\ProgramData\EDR\delete`,
	}
	for _, f := range residual {
		_ = os.Remove(f)
	}
}

// forceRemoveService stops and deletes the service.
// Called after DACL restoration has been attempted by the caller.
//
// This function includes multiple retry strategies:
//  1. Retry opening the service with full permissions (DACL restore may be async)
//  2. If Access Denied persists, attempt DACL restore between retries
//  3. If the service was already deleted (e.g., marked-for-delete), return success
func forceRemoveService() error {
	// Best-effort DACL restore from Admin context (belt-and-suspenders)
	_ = protection.RestoreServiceDACL(ServiceName)
	_ = protection.RestoreServiceRegistryKey(ServiceName)

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Retry opening the service with escalating recovery attempts.
	var s *mgr.Service
	for attempt := 0; attempt < 10; attempt++ {
		s, err = m.OpenService(ServiceName)
		if err == nil {
			break
		}

		// If service no longer exists, consider it success.
		if strings.Contains(err.Error(), "not exist") || strings.Contains(err.Error(), "1060") {
			return nil
		}

		// If Access Denied, re-attempt DACL restore before retrying.
		// This handles the race condition where DACL restore is still propagating.
		if attempt%3 == 2 {
			_ = protection.RestoreServiceDACL(ServiceName)
			_ = protection.RestoreServiceRegistryKey(ServiceName)
		}

		time.Sleep(1 * time.Second)
	}
	if err != nil {
		// Final check: service may have been deleted between retries.
		if strings.Contains(err.Error(), "not exist") || strings.Contains(err.Error(), "1060") {
			return nil
		}
		return fmt.Errorf("service %s: %w", ServiceName, err)
	}
	defer s.Close()

	// Disable recovery actions so the service doesn't auto-restart after stop.
	noRestart := []mgr.RecoveryAction{
		{Type: mgr.NoAction, Delay: 0},
		{Type: mgr.NoAction, Delay: 0},
		{Type: mgr.NoAction, Delay: 0},
	}
	_ = s.SetRecoveryActions(noRestart, 0)

	// Stop the service if running.
	status, err := s.Query()
	if err == nil && status.State != svc.Stopped {
		_, _ = s.Control(svc.Stop)
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			status, err = s.Query()
			if err != nil || status.State == svc.Stopped {
				break
			}
		}
	}

	// Delete the service.
	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	eventlog.Remove(ServiceName)
	return nil
}

// StartService opens a handle to the named service and issues a Start call.
// Blocks up to 10 s polling until the service reaches the Running state.
// This is used by the zero-touch installer after Install() to bring the
// service online without requiring a separate "net start" invocation.
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", ServiceName, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		// "already running" is acceptable — caller may retry.
		if !isAlreadyRunning(err) {
			return fmt.Errorf("failed to start service: %w", err)
		}
		return nil
	}

	// Poll until Running (up to 30 s — first boot after install needs extra time
	// for config loading and agent construction inside Execute()).
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}
		if status.State == svc.Running {
			return nil
		}
		// If the service has already stopped, don't keep waiting
		if status.State == svc.Stopped {
			return fmt.Errorf("service stopped unexpectedly — check C:\\ProgramData\\EDR\\logs\\agent.log for details")
		}
	}

	return fmt.Errorf("service did not reach Running state within 30s — check C:\\ProgramData\\EDR\\logs\\agent.log")
}

// isAlreadyRunning returns true when the error message indicates the service
// is already in a running state (Windows error 1056).
func isAlreadyRunning(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already") ||
		strings.Contains(err.Error(), "1056")
}

// Status returns the current service status using minimal permissions
// (SERVICE_QUERY_STATUS). This works even when the service DACL is hardened,
// unlike mgr.OpenService() which requires SERVICE_ALL_ACCESS.
func Status() (svc.State, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to SCM: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	svcNamePtr, _ := windows.UTF16PtrFromString(ServiceName)
	h, err := windows.OpenService(scm, svcNamePtr, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, fmt.Errorf("service not found: %w", err)
	}
	defer windows.CloseServiceHandle(h)

	var needed uint32
	var buf [256]byte
	err = windows.QueryServiceStatusEx(h, windows.SC_STATUS_PROCESS_INFO, &buf[0], uint32(len(buf)), &needed)
	if err != nil {
		return 0, fmt.Errorf("failed to query status: %w", err)
	}

	ssp := (*windows.SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0]))
	return svc.State(ssp.CurrentState), nil
}
