// Package main provides the entry point for the EDR Windows Agent.
//
// # Execution Modes
//
//	-install  Zero-touch setup: patch hosts file, write config.yaml, register SCM service,
//	           start the service. Requires -server-ip, -server-domain, and -token.
//	-uninstall  Remove the Windows Service registration.
//	CLI / standalone  Run interactively (development / testing).
//	SCM-managed  Detected automatically via svc.IsWindowsService(); no flag required.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"

	"github.com/edr-platform/win-agent/internal/agent"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/enrollment"
	"github.com/edr-platform/win-agent/internal/installer"
	"github.com/edr-platform/win-agent/internal/logging"
	"github.com/edr-platform/win-agent/internal/service"
)

// Version information (injected at build time via -ldflags).
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// ── CLI flags ──────────────────────────────────────────────────────────────
	var (
		configPath = flag.String(
			"config",
			installer.DefaultConfigPath,
			"Path to configuration YAML file",
		)
		showVersion = flag.Bool("version", false, "Show version information and exit")

		// ── Installation flags ─────────────────────────────────────────────────
		doInstall    = flag.Bool("install", false, "Zero-touch install: patch hosts, write config, register and start Windows Service")
		doUninstall  = flag.Bool("uninstall", false, "Remove the EDRAgent Windows Service")
		serverIP     = flag.String("server-ip", "", "C2 server IP address (used with -install for hosts file injection)")
		serverDomain = flag.String("server-domain", "", "C2 server FQDN/hostname (used with -install)")
		serverPort   = flag.String("server-port", "50051", "C2 gRPC port (used with -install, default 50051)")
		token        = flag.String("token", "", "Bootstrap enrollment token (install) or uninstall authorization token")

		// ── Runtime flags ──────────────────────────────────────────────────────
		debugMode = flag.Bool("debug", false, "Enable DEBUG-level logging")

		// Deprecated: kept for backward compatibility. SCM detection is now automatic.
		_ = flag.Bool("service", false, "[DEPRECATED] Run as Windows Service — now detected automatically")
	)
	flag.Parse()

	// ── Version ────────────────────────────────────────────────────────────────
	if *showVersion {
		fmt.Printf("EDR Windows Agent\n")
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Build Time: %s\n", BuildTime)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		os.Exit(0)
	}

	// ── Bootstrap logger (written to disk so SCM-managed starts have a log) ──
	logLevel := "INFO"
	if *debugMode {
		logLevel = "DEBUG"
	}
	logger := logging.NewLogger(logging.Config{
		Level:      logLevel,
		FilePath:   `C:\ProgramData\EDR\logs\agent.log`,
		MaxSizeMB:  100,
		MaxAgeDays: 7,
	})
	defer logger.Close()

	// ══════════════════════════════════════════════════════════════════════════
	// INSTALL PATH
	// ══════════════════════════════════════════════════════════════════════════
	if *doInstall {
		runInstall(logger, *serverIP, *serverDomain, *serverPort, *token, *configPath)
		// runInstall calls os.Exit internally.
	}

	// ══════════════════════════════════════════════════════════════════════════
	// UNINSTALL PATH
	// ══════════════════════════════════════════════════════════════════════════
	if *doUninstall {
		if err := service.Uninstall(*token); err != nil {
			logger.Errorf("Failed to uninstall service: %v", err)
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("EDR Agent service removed successfully.")
		os.Exit(0)
	}

	// ══════════════════════════════════════════════════════════════════════════
	// RUNTIME PATH — load config and start the agent
	// ══════════════════════════════════════════════════════════════════════════
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Errorf("Failed to load configuration: %v", err)
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	logger.SetLevel(cfg.Logging.Level)
	logger.Infof("════════════════════════════════════════")
	logger.Infof("EDR Windows Agent v%s", Version)
	logger.Infof("════════════════════════════════════════")
	logger.Infof("Config:    %s", *configPath)
	logger.Infof("Server:    %s", cfg.Server.Address)
	logger.Infof("Agent ID:  %s", cfg.Agent.ID)

	// ── Execution mode detection (BEFORE enrollment) ──────────────────────────
	// CRITICAL: svc.IsWindowsService() MUST be checked before any blocking
	// network call. When the SCM starts this process, enrollment happens inside
	// service.Execute() AFTER svc.Running is reported. Running os.Exit before
	// that causes error 1053 ("did not respond in a timely fashion").
	isScm, err := svc.IsWindowsService()
	if err != nil {
		logger.Warnf("IsWindowsService check failed (%v); assuming standalone mode", err)
		isScm = false
	}

	if isScm {
		// SCM path: hand off directly to service.Run() — Execute() will
		// perform CA fetch → enrollment → agent.Start() asynchronously.
		logger.Info("Execution context: Windows Service Control Manager")
		if err := service.Run(cfg, logger); err != nil {
			logger.Errorf("Service execution error: %v", err)
			os.Exit(1)
		}
		return
	}

	// Standalone path: enrollment runs synchronously here.
	logger.Info("Execution context: Interactive / standalone (Ctrl+C to stop)")

	// Auto-bootstrap CA certificate from C2 if missing.
	if !cfg.Server.Insecure && cfg.Certs.CAPath != "" {
		if err := enrollment.FetchCACertificate(cfg.Server.Address, cfg.Certs.CAPath, logger); err != nil {
			logger.Warnf("CA auto-bootstrap failed (will try existing cert): %v", err)
		}
	}

	// Ensure enrolled (cert/key present or bootstrap registration) before starting.
	if err := enrollment.EnsureEnrolled(cfg, logger, *configPath); err != nil {
		logger.Errorf("Agent enrollment failed: %v", err)
		os.Exit(1)
	}

	runStandalone(cfg, logger, *configPath)
}

// runInstall implements the zero-touch installation flow:
//
//  1. Validate required flags.
//  2. Create all EDR directories.
//  3. Patch the Windows hosts file (idempotent).
//  4. Generate and save config.yaml with injected server/token/UUID.
//  5. Register the Windows Service via the SCM.
//  6. Start the service and poll until it reaches Running state.
func runInstall(
	logger *logging.Logger,
	serverIP, serverDomain, serverPort, token, configPath string,
) {
	// Validate required parameters.
	if serverIP == "" || serverDomain == "" || token == "" {
		msg := "Error: -install requires -server-ip, -server-domain, and -token\n\n" +
			"Example:\n" +
			"  agent.exe -install \\\n" +
			"    -server-ip 192.168.1.10 \\\n" +
			"    -server-domain edr.internal \\\n" +
			"    -server-port 50051 \\\n" +
			"    -token <bootstrap-token>\n"
		fmt.Fprint(os.Stderr, msg)
		os.Exit(1)
	}
	if serverPort == "" {
		serverPort = "50051"
	}

	fmt.Println("[1/5] Creating EDR directories...")
	if err := installer.EnsureDirectories(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[2/5] Patching hosts file: %s → %s ...\n", serverIP, serverDomain)
	if err := installer.PatchHostsFile(serverIP, serverDomain); err != nil {
		// Non-fatal: DNS might already be configured centrally. Warn and continue.
		fmt.Fprintf(os.Stderr, "Warning: hosts file patch failed (continuing): %v\n", err)
		logger.Warnf("Hosts file patch failed (non-fatal): %v", err)
	} else {
		fmt.Println("         → Done.")
		logger.Infof("Hosts file patched: %s %s", serverIP, serverDomain)
	}

	fmt.Printf("[3/5] Generating config.yaml → %s ...\n", configPath)
	opts := installer.Options{
		ServerIP:     serverIP,
		ServerDomain: serverDomain,
		ServerPort:   serverPort,
		Token:        token,
		ConfigPath:   configPath,
	}
	if err := installer.GenerateConfig(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating config: %v\n", err)
		logger.Errorf("Config generation failed: %v", err)
		os.Exit(1)
	}
	fmt.Println("         → Done.")
	logger.Infof("Config written to %s (server=%s:%s)", configPath, serverDomain, serverPort)

	fmt.Println("[4/5] Registering Windows Service (EDRAgent)...")
	// If the service already exists, uninstall it first for a clean re-install.
	if err := service.Install(); err != nil {
		if isAlreadyExistsErr(err) {
			fmt.Println("         → Service exists; re-registering...")
			_ = service.ForceUninstall()
			if err2 := service.Install(); err2 != nil {
				fmt.Fprintf(os.Stderr, "Error installing service: %v\n", err2)
				logger.Errorf("Service install failed: %v", err2)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error installing service: %v\n", err)
			logger.Errorf("Service install failed: %v", err)
			os.Exit(1)
		}
	}
	fmt.Println("         → Done.")
	logger.Info("Service registered in SCM")

	fmt.Println("[5/5] Starting EDRAgent service...")
	if err := service.StartService(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting service: %v\n", err)
		logger.Errorf("Service start failed: %v", err)
		os.Exit(1)
	}

	fmt.Println("\n✓ EDR Agent installed and running successfully.")
	fmt.Printf("  Server:    %s:%s\n", serverDomain, serverPort)
	fmt.Printf("  Config:    %s\n", configPath)
	fmt.Println("  Service:   EDRAgent (Automatic, LocalSystem)")
	fmt.Println("\n  To check status:   sc query EDRAgent")
	fmt.Println("  To view logs:      Get-Content C:\\ProgramData\\EDR\\logs\\agent.log -Tail 50")
	fmt.Println("  To uninstall:      agent.exe -uninstall -token <secret>")
	logger.Infof("Zero-touch installation complete: server=%s:%s", serverDomain, serverPort)
	os.Exit(0)
}

// isAlreadyExistsErr returns true when the error from service.Install() indicates
// the service name already exists in the SCM.
func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "already exists") || contains(msg, "1073")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// runStandalone runs the agent in interactive/development mode.
func runStandalone(cfg *config.Config, logger *logging.Logger, configPath string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Infof("Received signal: %v — shutting down...", sig)
		cancel()
	}()

	ag, err := agent.New(cfg, logger)
	if err != nil {
		logger.Errorf("Failed to create agent: %v", err)
		os.Exit(1)
	}

	ag.SetConfigFilePath(configPath)
	ag.SetRestartInfo(configPath)

	// Wire the hot-reload callback so C2 UPDATE_CONFIG commands are live:
	// command.Handler.updateConfig() → agent.UpdateConfig() → validate + save + swap.
	ag.SetConfigUpdateHandler(ag.UpdateConfig)

	if err := ag.Start(ctx); err != nil {
		logger.Errorf("Failed to start agent: %v", err)
		os.Exit(1)
	}

	<-ctx.Done()

	logger.Info("Initiating graceful shutdown...")
	if err := ag.Stop(); err != nil {
		logger.Errorf("Error during shutdown: %v", err)
	}
	logger.Info("Agent stopped.")
}
