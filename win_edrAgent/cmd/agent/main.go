// Package main provides the entry point for the EDR Windows Agent.
// The agent runs as a Windows Service with SYSTEM privileges and collects
// security events via ETW/WMI, streaming them to Connection Manager via gRPC/mTLS.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/edr-platform/win-agent/internal/agent"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/enrollment"
	"github.com/edr-platform/win-agent/internal/logging"
	"github.com/edr-platform/win-agent/internal/service"
)

// Version information (set during build)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Parse command-line flags
	var (
		configPath     = flag.String("config", "C:\\ProgramData\\EDR\\config\\config.yaml", "Path to configuration file")
		showVersion    = flag.Bool("version", false, "Show version information")
		installService = flag.Bool("install", false, "Install Windows Service")
		removeService  = flag.Bool("uninstall", false, "Remove Windows Service")
		runAsService   = flag.Bool("service", false, "Run as Windows Service (internal)")
		debugMode      = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	// Show version
	if *showVersion {
		fmt.Printf("EDR Windows Agent\n")
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Build Time: %s\n", BuildTime)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		os.Exit(0)
	}

	// Initialize logger
	logLevel := "INFO"
	if *debugMode {
		logLevel = "DEBUG"
	}
	logger := logging.NewLogger(logging.Config{
		Level:      logLevel,
		FilePath:   "C:\\ProgramData\\EDR\\logs\\agent.log",
		MaxSizeMB:  100,
		MaxAgeDays: 7,
	})
	defer logger.Close()

	// Service management commands
	if *installService {
		if err := service.Install(); err != nil {
			logger.Errorf("Failed to install service: %v", err)
			fmt.Fprintf(os.Stderr, "Failed to install service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("EDR Agent service installed successfully")
		fmt.Println("Start with: net start EDRAgent")
		os.Exit(0)
	}

	if *removeService {
		if err := service.Uninstall(); err != nil {
			logger.Errorf("Failed to uninstall service: %v", err)
			fmt.Fprintf(os.Stderr, "Failed to uninstall service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("EDR Agent service removed successfully")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Errorf("Failed to load configuration: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Update log level from config
	logger.SetLevel(cfg.Logging.Level)

	logger.Infof("========================================")
	logger.Infof("EDR Windows Agent v%s", Version)
	logger.Infof("========================================")
	logger.Infof("Config: %s", *configPath)
	logger.Infof("Server: %s", cfg.Server.Address)
	logger.Infof("Log Level: %s", cfg.Logging.Level)

	// Auto-bootstrap CA certificate: fetch from Connection Manager if missing
	if !cfg.Server.Insecure && cfg.Certs.CAPath != "" {
		if err := enrollment.FetchCACertificate(cfg.Server.Address, cfg.Certs.CAPath, logger); err != nil {
			logger.Warnf("CA auto-bootstrap failed (will try existing cert): %v", err)
		}
	}

	// Ensure enrolled (cert/key exist or bootstrap registration) before any agent or gRPC use
	if err := enrollment.EnsureEnrolled(cfg, logger, *configPath); err != nil {
		logger.Errorf("Agent enrollment failed: %v", err)
		os.Exit(1)
	}

	// Run as Windows Service or standalone
	if *runAsService {
		// Running as Windows Service
		logger.Info("Starting as Windows Service...")
		if err := service.Run(cfg, logger); err != nil {
			logger.Errorf("Service error: %v", err)
			os.Exit(1)
		}
	} else {
		// Running standalone (for development/testing)
		logger.Info("Starting in standalone mode (Ctrl+C to stop)...")
		runStandalone(cfg, logger, *configPath)
	}
}

// runStandalone runs the agent in standalone mode (for development/testing)
func runStandalone(cfg *config.Config, logger *logging.Logger, configPath string) {
	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Infof("Received signal: %v, shutting down...", sig)
		cancel()
	}()

	// Create and start agent
	ag, err := agent.New(cfg, logger)
	if err != nil {
		logger.Errorf("Failed to create agent: %v", err)
		os.Exit(1)
	}

	// Set config path for self-healing re-enrollment
	ag.SetConfigFilePath(configPath)

	// Start agent
	if err := ag.Start(ctx); err != nil {
		logger.Errorf("Failed to start agent: %v", err)
		os.Exit(1)
	}

	// Wait for shutdown
	<-ctx.Done()

	// Graceful shutdown
	logger.Info("Initiating graceful shutdown...")
	if err := ag.Stop(); err != nil {
		logger.Errorf("Error during shutdown: %v", err)
	}

	logger.Info("Agent stopped")
}
