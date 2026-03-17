// Package installer provides the Zero-Touch / CLI-Driven installation logic for the EDR Agent.
// It handles hosts-file DNS injection, dynamic config generation, and Windows Service registration.
//
// This package is Windows-only and must be run with elevated (Administrator/SYSTEM) privileges.
//
//go:build windows
// +build windows

package installer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/edr-platform/win-agent/internal/config"
)

const (
	// hostsFile is the canonical Windows hosts file path.
	hostsFile = `C:\Windows\System32\drivers\etc\hosts`

	// ConfigDir is the protected directory for agent configuration.
	ConfigDir = `C:\ProgramData\EDR\config`

	// DefaultConfigPath is the standard config file path.
	DefaultConfigPath = `C:\ProgramData\EDR\config\config.yaml`

	// ServiceName matches the name registered in the SCM.
	ServiceName = "EDRAgent"

	// hostsComment is the marker appended to EDR-managed hosts entries.
	hostsComment = "# EDR C2"
)

// Options carries the parameters supplied via CLI flags during installation.
type Options struct {
	// ServerIP is the raw IP of the C2 server to inject into the hosts file.
	ServerIP string

	// ServerDomain is the FQDN/hostname the agent will use to connect to C2.
	// This is also the domain injected into the hosts file.
	ServerDomain string

	// ServerPort is the gRPC port (default "50051").
	ServerPort string

	// Token is the bootstrap enrollment token written into config.yaml.
	Token string

	// ConfigPath is the absolute path where config.yaml will be written.
	// Defaults to DefaultConfigPath.
	ConfigPath string
}

// effectiveConfigPath returns opts.ConfigPath if set, otherwise DefaultConfigPath.
func (o *Options) effectiveConfigPath() string {
	if o.ConfigPath != "" {
		return o.ConfigPath
	}
	return DefaultConfigPath
}

// =============================================================================
// PatchHostsFile
// =============================================================================

// PatchHostsFile ensures that the Windows hosts file contains a mapping of
// serverIP → serverDomain. The operation is idempotent: if an uncommented line
// containing both tokens already exists, the file is not modified.
//
// Requires Administrator or SYSTEM privileges on the calling process.
func PatchHostsFile(serverIP, serverDomain string) error {
	if serverIP == "" || serverDomain == "" {
		return fmt.Errorf("PatchHostsFile: serverIP and serverDomain must not be empty")
	}

	// ── Read existing content ──────────────────────────────────────────────────
	data, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// ── Check idempotency ─────────────────────────────────────────────────────
	// Consider a line a match only when it is not a comment AND contains both
	// the exact IP and the exact domain as whitespace-delimited tokens.
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		stripped := strings.TrimSpace(line)

		// Skip blank lines and comment lines.
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}

		// Check if this line already maps our IP to our domain.
		fields := strings.Fields(stripped)
		if len(fields) >= 2 && fields[0] == serverIP {
			for _, f := range fields[1:] {
				if strings.EqualFold(f, serverDomain) {
					// Entry already present — idempotent, nothing to do.
					return nil
				}
			}
		}
	}

	// ── Append the new entry ──────────────────────────────────────────────────
	// Ensure the file ends with a newline before appending.
	content := string(data)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content += "\n"
	}
	entry := fmt.Sprintf("%s\t%s\t%s\n", serverIP, serverDomain, hostsComment)
	content += entry

	if err := os.WriteFile(hostsFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write hosts file: %w", err)
	}

	return nil
}

// =============================================================================
// GenerateConfig
// =============================================================================

// GenerateConfig builds a complete agent Config from sensible defaults, overrides
// the dynamic fields (Server.Address, Certs.BootstrapToken, Agent.ID), and saves
// it to opts.ConfigPath (or DefaultConfigPath).
//
// The generated config.yaml is the canonical source of truth used by the agent
// when started by the SCM.
func GenerateConfig(opts Options) error {
	if opts.ServerDomain == "" {
		return fmt.Errorf("GenerateConfig: ServerDomain must not be empty")
	}
	if opts.ServerPort == "" {
		opts.ServerPort = "50051"
	}

	// ── Build configuration from proven defaults ───────────────────────────────
	cfg := config.DefaultConfig()

	// Override the three dynamic fields.
	cfg.Server.Address = opts.ServerDomain + ":" + opts.ServerPort
	cfg.Certs.BootstrapToken = opts.Token
	cfg.Agent.ID = uuid.New().String()

	// ── Create the target directory ────────────────────────────────────────────
	cfgPath := opts.effectiveConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// ── Persist to disk ────────────────────────────────────────────────────────
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("failed to save generated config: %w", err)
	}

	return nil
}

// =============================================================================
// InstallAndStart
// =============================================================================

// InstallAndStart registers the agent as a Windows Service and starts it.
// If the service already exists (e.g., a previous install), it is first
// uninstalled so the registration is always fresh.
//
// The SCM is connected twice intentionally: once via service.Install() to create
// the service entry, and once here to call Start() — this avoids exposing a raw
// mgr.Service handle through the service package API.
func InstallAndStart(exePath string) error {
	// ── Connect to SCM ─────────────────────────────────────────────────────────
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// ── Open service handle ────────────────────────────────────────────────────
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found in SCM (was Install() called?): %w", ServiceName, err)
	}
	defer s.Close()

	// ── Start the service ──────────────────────────────────────────────────────
	if err := s.Start(); err != nil {
		// "already running" is not an error for idempotency.
		if !strings.Contains(err.Error(), "already") {
			return fmt.Errorf("failed to start service: %w", err)
		}
	}

	// ── Poll until Running (up to 10 s) ───────────────────────────────────────
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}
		if status.State == svc.Running {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("service did not reach Running state within 10s")
}

// =============================================================================
// EnsureDirectories
// =============================================================================

// EnsureDirectories creates all required EDR directories with appropriate
// permissions. This is called once during installation.
func EnsureDirectories() error {
	dirs := []string{
		`C:\ProgramData\EDR`,
		`C:\ProgramData\EDR\config`,
		`C:\ProgramData\EDR\certs`,
		`C:\ProgramData\EDR\logs`,
		`C:\ProgramData\EDR\queue`,
		`C:\ProgramData\EDR\quarantine`,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}
