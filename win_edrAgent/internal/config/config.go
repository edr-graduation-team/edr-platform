// Package config handles configuration loading and management for the EDR Agent.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Config represents the complete agent configuration.
type Config struct {
	Server     ServerConfig    `yaml:"server"`
	Agent      AgentConfig     `yaml:"agent"`
	Collectors CollectorConfig `yaml:"collectors"`
	Filtering  FilteringConfig `yaml:"filtering"`
	Logging    LoggingConfig   `yaml:"logging"`
	Certs      CertConfig      `yaml:"certs"`
}

// ServerConfig defines Connection Manager connection settings.
type ServerConfig struct {
	Address           string        `yaml:"address"`
	Insecure          bool          `yaml:"insecure"` // if true, use plaintext gRPC (no TLS) for debugging / Host-VM connectivity
	Timeout           time.Duration `yaml:"timeout"`
	ReconnectDelay    time.Duration `yaml:"reconnect_delay"`
	MaxReconnectDelay time.Duration `yaml:"max_reconnect_delay"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`

	// TLSServerName overrides the hostname used for TLS certificate SAN validation.
	// Use this when the server cert is issued for an internal service name
	// (e.g. "edr-connection-manager") but the agent connects via a custom deployment
	// domain (e.g. "edr.internal" or a raw IP). Leaving this empty uses the hostname
	// from Server.Address as-is.
	TLSServerName string `yaml:"tls_server_name"`
}

// AgentConfig defines agent behavior settings.
type AgentConfig struct {
	ID             string        `yaml:"id"`
	Hostname       string        `yaml:"hostname"`
	BatchSize      int           `yaml:"batch_size"`
	BatchInterval  time.Duration `yaml:"batch_interval"`
	BufferSize     int           `yaml:"buffer_size"`
	Compression    string        `yaml:"compression"`       // "snappy", "gzip", "none"
	QueueDir       string        `yaml:"queue_dir"`         // Offline disk queue directory (WAL)
	MaxQueueSizeMB int           `yaml:"max_queue_size_mb"` // Max disk queue size in MB
}

// CollectorConfig defines event collection settings.
type CollectorConfig struct {
	ETWEnabled       bool          `yaml:"etw_enabled"`
	ETWSessionName   string        `yaml:"etw_session_name"`
	WMIEnabled       bool          `yaml:"wmi_enabled"`
	WMIInterval      time.Duration `yaml:"wmi_interval"`
	RegistryEnabled  bool          `yaml:"registry_enabled"`
	FileEnabled      bool          `yaml:"file_enabled"`
	ImageLoadEnabled bool          `yaml:"imageload_enabled"`
	NetworkEnabled   bool          `yaml:"network_enabled"`
}

// FilteringConfig defines event filtering rules.
type FilteringConfig struct {
	ExcludeProcesses []string `yaml:"exclude_processes"`
	ExcludeIPs       []string `yaml:"exclude_ips"`
	ExcludeRegistry  []string `yaml:"exclude_registry"`
	ExcludePaths     []string `yaml:"exclude_paths"`
	IncludePaths     []string `yaml:"include_paths"`

	// Advanced filtering — Sysmon Event IDs to drop at the edge before serialization.
	// Common noisy IDs: 4 (Sysmon service state), 7 (ImageLoad), 15 (FileCreateStreamHash),
	// 22 (DNSEvent), 23 (FileDelete).
	ExcludeEventIDs []int `yaml:"exclude_event_ids"`

	// SHA256 hashes of known-good, trusted binaries whose events can be safely dropped.
	// Reduces noise from OS-native processes and verified third-party software.
	TrustedHashes []string `yaml:"trusted_hashes"`

	// QoS rate limiting configuration for noisy event types.
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// RateLimitConfig defines per-event-type rate limiting using a Token Bucket algorithm.
type RateLimitConfig struct {
	// Enabled toggles rate limiting. When false, all events pass through unrestricted.
	Enabled bool `yaml:"enabled"`

	// DefaultMaxEPS is the default maximum Events Per Second for any event type
	// not explicitly listed in PerEventType. 0 means unlimited.
	DefaultMaxEPS int `yaml:"default_max_eps"`

	// CriticalBypass, when true, ensures events with Critical or High severity
	// are never rate-limited — they always pass through regardless of token state.
	CriticalBypass bool `yaml:"critical_bypass"`

	// PerEventType allows fine-grained EPS limits per event type.
	// Keys are event type strings: "dns", "network", "file", "image_load", etc.
	// Values are the max EPS allowed for that type.
	PerEventType map[string]int `yaml:"per_event_type"`
}

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	Level      string `yaml:"level"`
	FilePath   string `yaml:"file_path"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxAgeDays int    `yaml:"max_age_days"`
}

// CertConfig defines certificate paths.
type CertConfig struct {
	CertPath       string `yaml:"cert_path"`
	KeyPath        string `yaml:"key_path"`
	CAPath         string `yaml:"ca_path"`
	BootstrapToken string `yaml:"bootstrap_token"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	return &Config{
		Server: ServerConfig{
			Address:           "localhost:50051",
			Insecure:          false,
			Timeout:           30 * time.Second,
			ReconnectDelay:    1 * time.Second,
			MaxReconnectDelay: 30 * time.Second,
			HeartbeatInterval: 30 * time.Second,
			TLSServerName:     "edr-connection-manager",
		},
		Agent: AgentConfig{
			ID:             uuid.New().String(),
			Hostname:       hostname,
			BatchSize:      50,
			BatchInterval:  1 * time.Second,
			BufferSize:     5000,
			Compression:    "snappy",
			QueueDir:       "C:\\ProgramData\\EDR\\queue",
			MaxQueueSizeMB: 500,
		},
		Collectors: CollectorConfig{
			ETWEnabled:      true,
			ETWSessionName:  "EDRAgentSession",
			WMIEnabled:      true,
			WMIInterval:     60 * time.Minute,
			RegistryEnabled: true,
			FileEnabled:     true,
			NetworkEnabled:  true,
		},
		Filtering: FilteringConfig{
			ExcludeProcesses: []string{
				"svchost.exe",
				"csrss.exe",
				"services.exe",
				"smss.exe",
				"wininit.exe",
				"winlogon.exe",
				"dwm.exe",
				"taskhostw.exe",
				"RuntimeBroker.exe",
				"SearchIndexer.exe",
				"MsMpEng.exe", // Windows Defender
				"agent.exe",   // Self
			},
			ExcludeIPs: []string{
				"127.0.0.1",
				"::1",
				"0.0.0.0",
				"169.254.0.0/16", // Link-local
			},
			ExcludeRegistry: []string{
				"HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing",
				"HKLM\\SYSTEM\\CurrentControlSet\\Services\\bam\\State",
			},
			ExcludePaths: []string{
				"C:\\Windows\\Temp",
				"C:\\Users\\*\\AppData\\Local\\Temp",
				"C:\\Windows\\SoftwareDistribution",
			},
			IncludePaths: []string{
				"C:\\Windows\\System32",
				"C:\\Program Files",
				"C:\\Program Files (x86)",
			},
		},
		Logging: LoggingConfig{
			Level:      "INFO",
			FilePath:   "C:\\ProgramData\\EDR\\logs\\agent.log",
			MaxSizeMB:  100,
			MaxAgeDays: 7,
		},
		Certs: CertConfig{
			CertPath:       "C:\\ProgramData\\EDR\\certs\\client.crt",
			KeyPath:        "C:\\ProgramData\\EDR\\certs\\private.key",
			CAPath:         "C:\\ProgramData\\EDR\\certs\\ca-chain.crt",
			BootstrapToken: "",
		},
	}
}

// Load reads configuration from a YAML file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return defaults if file doesn't exist
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Server.Address == "" {
		return fmt.Errorf("server.address is required")
	}
	if c.Agent.BatchSize < 1 || c.Agent.BatchSize > 10000 {
		return fmt.Errorf("agent.batch_size must be between 1 and 10000")
	}
	if c.Agent.BatchInterval < 100*time.Millisecond || c.Agent.BatchInterval > 60*time.Second {
		return fmt.Errorf("agent.batch_interval must be between 100ms and 60s")
	}
	if c.Agent.BufferSize < 100 || c.Agent.BufferSize > 100000 {
		return fmt.Errorf("agent.buffer_size must be between 100 and 100000")
	}
	if c.Agent.QueueDir == "" {
		c.Agent.QueueDir = "C:\\ProgramData\\EDR\\queue"
	}
	if c.Agent.MaxQueueSizeMB < 1 || c.Agent.MaxQueueSizeMB > 2000 {
		return fmt.Errorf("agent.max_queue_size_mb must be between 1 and 2000")
	}
	return nil
}

// Save writes configuration to a YAML file.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Clone creates a deep copy of the configuration.
func (c *Config) Clone() *Config {
	clone := *c
	clone.Filtering.ExcludeProcesses = append([]string{}, c.Filtering.ExcludeProcesses...)
	clone.Filtering.ExcludeIPs = append([]string{}, c.Filtering.ExcludeIPs...)
	clone.Filtering.ExcludeRegistry = append([]string{}, c.Filtering.ExcludeRegistry...)
	clone.Filtering.ExcludePaths = append([]string{}, c.Filtering.ExcludePaths...)
	clone.Filtering.IncludePaths = append([]string{}, c.Filtering.IncludePaths...)
	return &clone
}
