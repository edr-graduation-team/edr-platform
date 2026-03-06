// Package service provides Windows Service integration for the EDR Agent.
//go:build windows
// +build windows

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/edr-platform/win-agent/internal/agent"
	"github.com/edr-platform/win-agent/internal/config"
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

// Execute is the main service control handler required by Windows Service API.
func (s *edrService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	// Report service is starting
	changes <- svc.Status{State: svc.StartPending}

	// Create context for agent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create agent
	var err error
	s.agent, err = agent.New(s.cfg, s.logger)
	if err != nil {
		s.logger.Errorf("Failed to create agent: %v", err)
		return false, 1
	}

	// Start agent in background
	go func() {
		if err := s.agent.Start(ctx); err != nil {
			s.logger.Errorf("Agent start error: %v", err)
		}
	}()

	// Report service is running
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	s.logger.Info("Service is running")

	// Main service loop - handle control requests
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus

		case svc.Stop, svc.Shutdown:
			s.logger.Info("Service stop requested")
			changes <- svc.Status{State: svc.StopPending}

			// Cancel context to trigger graceful shutdown
			cancel()

			// Wait for agent to stop (with timeout)
			done := make(chan struct{})
			go func() {
				s.agent.Stop()
				close(done)
			}()

			select {
			case <-done:
				s.logger.Info("Agent stopped gracefully")
			case <-time.After(30 * time.Second):
				s.logger.Warn("Agent stop timed out")
			}

			return false, 0

		case svc.Pause:
			changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			s.logger.Info("Service paused")

		case svc.Continue:
			changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			s.logger.Info("Service resumed")

		default:
			s.logger.Warnf("Unexpected control request: %d", c.Cmd)
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
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
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
	dirs := []string{
		"C:\\ProgramData\\EDR",
		"C:\\ProgramData\\EDR\\config",
		"C:\\ProgramData\\EDR\\certs",
		"C:\\ProgramData\\EDR\\logs",
		"C:\\ProgramData\\EDR\\quarantine",
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
	}

	// Create default config if not exists
	configPath := "C:\\ProgramData\\EDR\\config\\config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultCfg := config.DefaultConfig()
		if err := defaultCfg.Save(configPath); err != nil {
			fmt.Printf("Warning: failed to create default config: %v\n", err)
		}
	}

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
