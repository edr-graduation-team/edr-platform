// Package service provides Windows Service integration for the EDR Agent.
//go:build windows
// +build windows

package service

import (
	"context"
	"fmt"
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
	// Config loading happens HERE (inside Execute) rather than in main() to
	// guarantee that svc.Run() has already registered the service handler.
	// If config loading happened in main() and failed, os.Exit() would kill
	// the process before the SCM received a handler → "Error 1: Incorrect function".
	cfg, err := config.Load(s.configPath)
	if err != nil {
		s.logger.Errorf("[SCM] Failed to load configuration from %s: %v", s.configPath, err)
		changes <- svc.Status{State: svc.StopPending}
		return false, 2 // service-specific error code 2 = config load failure
	}
	s.cfg = cfg
	s.logger.SetLevel(cfg.Logging.Level)
	s.logger.Infof("[SCM] Config loaded successfully: server=%s agent=%s", cfg.Server.Address, cfg.Agent.ID)

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
	// Protects HKLM\SYSTEM\CurrentControlSet\Services\EDRAgent from direct
	// registry deletion (reg delete) by Administrators.
	if err := protection.HardenServiceRegistryKey(ServiceName); err != nil {
		s.logger.Warnf("[SCM] Registry key hardening failed (non-fatal): %v", err)
	} else {
		s.logger.Info("[SCM] Service registry key hardened — reg delete blocked")
	}

	// ── Uninstall File Watcher ─────────────────────────────────────────
	// Polls for C:\ProgramData\EDR\uninstall.dat every 2 seconds.
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
			if err := enrollment.EnsureCACertificate(s.cfg.Server.Address, s.cfg.Certs.CAPath, s.logger); err != nil {
				s.logger.Warnf("[SCM] CA provisioning failed (using existing cert if present): %v", err)
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
				s.logger.Info("[UNINSTALL] Registry key DACL restored")
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

// serviceExists checks if EDRAgent is registered in the SCM using minimal
// permissions (SERVICE_QUERY_STATUS). This works even when the service DACL
// is hardened, unlike mgr.OpenService() which requires SERVICE_ALL_ACCESS.
func serviceExists() bool {
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
func Install() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Connect to service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists (using minimal permissions that work with hardened DACL)
	if serviceExists() {
		return fmt.Errorf("service %s already exists", ServiceName)
	}

	// Create service
	s, err := m.CreateService(ServiceName, exePath, mgr.Config{
		DisplayName:      ServiceDisplayName,
		Description:      ServiceDescription,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "LocalSystem",
	},
		"-service",
		"-config", "C:\\ProgramData\\EDR\\config\\config.yaml",
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
		// Non-fatal, just log
		fmt.Printf("Warning: failed to set recovery actions: %v\n", err)
	}

	// Create event log source
	if err := eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		// Non-fatal error
		fmt.Printf("Warning: failed to create event log source: %v\n", err)
	}

	// Create required directories
	// NOTE: installer.EnsureDirectories() is called before Install() during zero-touch
	// setup, but we keep directory creation here as a safety net for manual installs.
	dirs := []string{
		"C:\\ProgramData\\EDR",
		"C:\\ProgramData\\EDR\\config",
		"C:\\ProgramData\\EDR\\certs",
		"C:\\ProgramData\\EDR\\logs",
		"C:\\ProgramData\\EDR\\queue",
		"C:\\ProgramData\\EDR\\quarantine",
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
	}

	// NOTE: config.yaml is intentionally NOT written here. During a zero-touch
	// install, installer.GenerateConfig() has already written the dynamically
	// parameterised config before Install() was called. During a manual install
	// (legacy -install without flags), the agent will load defaults at startup
	// and the operator must edit config.yaml afterwards.


	return nil
}

// Uninstall removes the Windows service. Requires a valid anti-tamper token.
// embeddedTokenHash is the SHA-256 hash of the enrollment token, compiled into the binary.
//
// Flow:
//  1. Verify the plaintext token against the embedded hash (first gate).
//  2. Write the token's SHA-256 hash to C:\ProgramData\EDR\uninstall.dat.
//  3. The running service (SYSTEM) detects the file via its watcher goroutine,
//     verifies the hash, restores DACL + registry permissions, and stops.
//  4. This function polls until the service stops, then deletes it from the SCM.
func Uninstall(token, embeddedTokenHash string) error {
	// ── Anti-Tamper: Verify uninstall token ──────────────────────────────────
	if err := protection.VerifyUninstallToken(token, embeddedTokenHash); err != nil {
		return fmt.Errorf("uninstall blocked: %w", err)
	}

	// Write the token hash to signal the running service
	providedHash := protection.HashUninstallToken(token)
	hashFilePath := filepath.Join("C:\\ProgramData\\EDR", "uninstall.dat")
	if err := os.WriteFile(hashFilePath, []byte(providedHash), 0600); err != nil {
		return fmt.Errorf("failed to write uninstall signal: %w", err)
	}

	fmt.Println("  Token verified. Signaling service to release protections...")

	// Poll until the service stops (the watcher goroutine inside the service
	// will detect uninstall.dat, verify the hash, restore DACL, and stop).
	// Re-write uninstall.dat every 5 seconds to handle race conditions
	// where a previous service instance consumed the file before stopping.
	stopped := false
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		state, err := Status()
		if err != nil || state == svc.Stopped {
			stopped = true
			break
		}
		// Re-write the signal file every 5 seconds in case it was consumed
		// by a restarting service instance (before recovery was disabled).
		if i%5 == 4 {
			_ = os.WriteFile(hashFilePath, []byte(providedHash), 0600)
			fmt.Printf("  Waiting for service to stop... (%d/30s)\n", i+1)
		}
	}
	if stopped {
		fmt.Println("  Service stopped. Removing service registration...")
	} else {
		fmt.Println("  Service did not stop within 30s. Attempting forced removal...")
	}

	// Clean up signal and residual files
	_ = os.Remove(hashFilePath)
	cleanupResidualFiles()

	return forceRemoveService()
}

// ForceUninstall removes the service WITHOUT token verification.
// This is ONLY called from runInstall() during a re-install, which is already
// a privileged operation (requires admin + enrollment token).
// It is NOT exposed via any CLI flag.
//
// Uses the embedded token hash to signal the running service via the
// uninstall.dat file mechanism, then waits for the service to stop.
func ForceUninstall(embeddedTokenHash string) error {
	// Write the embedded hash to signal the running service
	hashFilePath := filepath.Join("C:\\ProgramData\\EDR", "uninstall.dat")
	hash := embeddedTokenHash
	if hash == "" {
		hash = protection.HashUninstallToken("EDR-Uninstall-2026!")
	}
	_ = os.WriteFile(hashFilePath, []byte(hash), 0600)

	fmt.Println("      Signaling running service to release protections...")
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		state, err := Status()
		if err != nil || state == svc.Stopped {
			break
		}
		// Re-write signal every 5 seconds
		if i%5 == 4 {
			_ = os.WriteFile(hashFilePath, []byte(hash), 0600)
		}
	}

	// Clean up signal and residual files
	_ = os.Remove(hashFilePath)
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
// Called after the running service should have restored its own DACL.
// Includes retries to handle race conditions between DACL restoration and
// this function's attempt to open the service with full permissions.
func forceRemoveService() error {
	// Best-effort DACL restore from Admin context
	_ = protection.RestoreServiceDACL(ServiceName)
	_ = protection.RestoreServiceRegistryKey(ServiceName)

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Retry opening the service — DACL restoration by the service (SYSTEM)
	// may still be in progress.
	var s *mgr.Service
	for attempt := 0; attempt < 10; attempt++ {
		s, err = m.OpenService(ServiceName)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		// If the service no longer exists, it was already removed (e.g.,
		// it was marked for deletion and auto-deleted when it stopped).
		if strings.Contains(err.Error(), "not exist") || strings.Contains(err.Error(), "1060") {
			return nil // Already gone — success
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
