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

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/edr-platform/win-agent/internal/agent"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/enrollment"
	"github.com/edr-platform/win-agent/internal/logging"
)

const (
	ServiceName        = "EDRAgent"
	ServiceDisplayName = "EDR Agent Service"
	ServiceDescription = "Endpoint Detection and Response Agent - Collects security events and provides threat protection"
)

// edrService implements the Windows service interface.
type edrService struct {
	cfg    *config.Config
	logger *logging.Logger
	agent  *agent.Agent
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
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	// ── 1. StartPending — satisfy SCM within the startup timeout window ────────
	changes <- svc.Status{State: svc.StartPending}

	// ── 2. Prepare agent (pure in-memory, cannot fail) ────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	s.agent, err = agent.New(s.cfg, s.logger)
	if err != nil {
		// agent.New() failure is extremely unlikely (nil cfg/logger), but if it
		// happens before we report Running, we MUST still transition properly.
		s.logger.Errorf("[SCM] Failed to construct agent: %v", err)
		changes <- svc.Status{State: svc.StopPending}
		changes <- svc.Status{State: svc.Stopped}
		return false, 1
	}

	const svcConfigPath = `C:\ProgramData\EDR\config\config.yaml`
	s.agent.SetConfigFilePath(svcConfigPath)
	s.agent.SetRestartInfo(svcConfigPath)
	s.agent.SetConfigUpdateHandler(s.agent.UpdateConfig)

	// ── 3. Running — must be sent BEFORE any network / TLS / enrollment work ──
	// This satisfies the SCM contract. All subsequent work is async.
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	s.logger.Info("[SCM] Service reported Running — starting async enrollment + agent")

	// agentErrCh carries the outcome of the async startup sequence.
	// A nil value means the agent is running; a non-nil value means it failed.
	agentErrCh := make(chan error, 1)

	// ── 4. Async startup goroutine ─────────────────────────────────────────────
	go func() {
		// 4a. Fetch CA certificate (best-effort, non-fatal)
		if !s.cfg.Server.Insecure && s.cfg.Certs.CAPath != "" {
			if err := fetchCA(s.cfg, s.logger); err != nil {
				s.logger.Warnf("[SCM] CA auto-bootstrap failed (using existing cert if present): %v", err)
			}
		}

		// 4b. Enrollment — this is where TLS handshakes happen.
		// On failure we log the error and signal the control loop to stop cleanly.
		if err := ensureEnrolled(s.cfg, s.logger, svcConfigPath); err != nil {
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
func Run(cfg *config.Config, logger *logging.Logger) error {
	logger.Info("Initializing Windows Service...")

	// Check if running as a service
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to check if running as service: %w", err)
	}

	if !isService {
		return fmt.Errorf("not running as a Windows service")
	}

	// Run the service
	err = svc.Run(ServiceName, &edrService{
		cfg:    cfg,
		logger: logger,
	})
	if err != nil {
		return fmt.Errorf("service run failed: %w", err)
	}

	return nil
}

// fetchCA is a thin wrapper so Execute()'s async goroutine can call
// enrollment.FetchCACertificate without referencing the enrollment package
// at every call-site.
func fetchCA(cfg *config.Config, logger *logging.Logger) error {
	return enrollment.FetchCACertificate(cfg.Server.Address, cfg.Certs.CAPath, logger)
}

// ensureEnrolled is a thin wrapper over enrollment.EnsureEnrolled used by Execute()
// to perform full certificate bootstrap inside the async startup goroutine,
// AFTER the SCM has already received svc.Running.
func ensureEnrolled(cfg *config.Config, logger *logging.Logger, configPath string) error {
	return enrollment.EnsureEnrolled(cfg, logger, configPath)
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

	// Check if service already exists
	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", ServiceName)
	}

	// Create service
	s, err = m.CreateService(ServiceName, exePath, mgr.Config{
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

// Uninstall removes the Windows service.
func Uninstall() error {
	// Connect to service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Open service
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", ServiceName, err)
	}
	defer s.Close()

	// Stop service if running
	status, err := s.Query()
	if err == nil && status.State != svc.Stopped {
		s.Control(svc.Stop)
		// Wait for stop
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second)
			status, err = s.Query()
			if err != nil || status.State == svc.Stopped {
				break
			}
		}
	}

	// Delete service
	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Remove event log source
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

	// Poll until Running (up to 10 s).
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}
		if status.State == svc.Running {
			return nil
		}
	}

	return fmt.Errorf("service did not reach Running state within 10s")
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

// Status returns the current service status.
func Status() (svc.State, error) {
	m, err := mgr.Connect()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return 0, fmt.Errorf("service not found: %w", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return 0, fmt.Errorf("failed to query status: %w", err)
	}

	return status.State, nil
}
