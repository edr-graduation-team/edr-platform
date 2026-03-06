package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration.
type Config struct {
	FileMonitoring FileMonitoringConfig `yaml:"file_monitoring"`
	EventCounting  EventCountingConfig  `yaml:"event_counting"`
	Escalation     EscalationConfig     `yaml:"escalation"`
	Detection      DetectionConfig      `yaml:"detection"`
	Filtering      FilteringConfig      `yaml:"filtering"`
	Output         OutputConfig         `yaml:"output"`
	Rules          RulesConfig          `yaml:"rules"`
}

// FileMonitoringConfig configures file monitoring.
type FileMonitoringConfig struct {
	WatchDirectory            string `yaml:"watch_directory"`
	FilePattern               string `yaml:"file_pattern"`
	PollIntervalMS            int    `yaml:"poll_interval_ms"`
	MaxFileSizeGB             int    `yaml:"max_file_size_gb"`
	CheckpointFile            string `yaml:"checkpoint_file"`
	CheckpointIntervalSeconds int    `yaml:"checkpoint_interval_seconds"`
}

// EventCountingConfig configures event counting.
type EventCountingConfig struct {
	WindowSizeMinutes      int     `yaml:"window_size_minutes"`
	AlertThreshold         int     `yaml:"alert_threshold"`
	RateThresholdPerMinute float64 `yaml:"rate_threshold_per_minute"`
}

// EscalationConfig configures alert escalation.
type EscalationConfig struct {
	CountThreshold           int     `yaml:"count_threshold"`
	RateThresholdPerMinute   float64 `yaml:"rate_threshold_per_minute"`
	EnableCriticalEscalation bool    `yaml:"enable_critical_escalation"`
}

// DetectionConfig configures the detection engine.
type DetectionConfig struct {
	Workers                 int     `yaml:"workers"`
	BatchSize               int     `yaml:"batch_size"`
	CacheSize               int     `yaml:"cache_size"`
	MinConfidence           float64 `yaml:"min_confidence"`
	EnableFilters           *bool   `yaml:"enable_filters"`
	EnableContextValidation *bool   `yaml:"enable_context_validation"`
}

// FilteringConfig configures global whitelisting to reduce false positives.
type FilteringConfig struct {
	Enable *bool `yaml:"enable"`

	WhitelistedProcesses       []string `yaml:"whitelisted_processes"`
	WhitelistedUsers           []string `yaml:"whitelisted_users"`
	WhitelistedParentProcesses []string `yaml:"whitelisted_parent_processes"`
}

// OutputConfig configures output settings.
type OutputConfig struct {
	OutputFile string `yaml:"output_file"`
	LogLevel   string `yaml:"log_level"`
}

// RulesConfig configures rules loading.
type RulesConfig struct {
	RulesDirectory   string   `yaml:"rules_directory"`
	ProductWhitelist []string `yaml:"product_whitelist,omitempty"` // Products to load (empty = all)
	CacheFile        string   `yaml:"cache_file"`
	CacheMaxAgeHours int      `yaml:"cache_max_age_hours"`
	ParallelWorkers  int      `yaml:"parallel_workers"`

	// Quality filters
	MinLevel         string   `yaml:"min_level"`
	AllowedStatus    []string `yaml:"allowed_status"`
	SkipExperimental *bool    `yaml:"skip_experimental"`
}

// FiltersEnabled returns whether rule filters should be applied.
// Default: true.
func (dc DetectionConfig) FiltersEnabled() bool {
	if dc.EnableFilters == nil {
		return true
	}
	return *dc.EnableFilters
}

// ContextValidationEnabled returns whether context validation should be applied.
// Default: false.
func (dc DetectionConfig) ContextValidationEnabled() bool {
	if dc.EnableContextValidation == nil {
		return false
	}
	return *dc.EnableContextValidation
}

// FilteringEnabled returns whether global whitelisting is enabled.
// Default: false.
func (fc FilteringConfig) FilteringEnabled() bool {
	if fc.Enable == nil {
		return false
	}
	return *fc.Enable
}

// SkipExperimentalEnabled returns whether experimental rules should be skipped.
// Default: true.
func (rc RulesConfig) SkipExperimentalEnabled() bool {
	if rc.SkipExperimental == nil {
		return true
	}
	return *rc.SkipExperimental
}

// LoadConfig loads configuration from a YAML file.
// If the file doesn't exist, returns default configuration.
// If the file exists but is invalid, returns an error.
func LoadConfig(configPath string) (*Config, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		logger.Infof("Config file not found at %s, using defaults", configPath)
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Validate and set defaults
	config.ValidateAndSetDefaults()

	return &config, nil
}

// ValidateAndSetDefaults validates configuration and sets default values where needed.
func (c *Config) ValidateAndSetDefaults() {
	// File monitoring defaults
	if c.FileMonitoring.WatchDirectory == "" {
		c.FileMonitoring.WatchDirectory = "data/agent_ecs-events"
	}
	if c.FileMonitoring.FilePattern == "" {
		c.FileMonitoring.FilePattern = "*.jsonl"
	}
	if c.FileMonitoring.PollIntervalMS <= 0 {
		c.FileMonitoring.PollIntervalMS = 100
	}
	if c.FileMonitoring.MaxFileSizeGB <= 0 {
		c.FileMonitoring.MaxFileSizeGB = 1
	}
	if c.FileMonitoring.CheckpointFile == "" {
		c.FileMonitoring.CheckpointFile = "data/checkpoint.json"
	}
	if c.FileMonitoring.CheckpointIntervalSeconds <= 0 {
		c.FileMonitoring.CheckpointIntervalSeconds = 30
	}

	// Event counting defaults
	if c.EventCounting.WindowSizeMinutes <= 0 {
		c.EventCounting.WindowSizeMinutes = 5
	}
	if c.EventCounting.AlertThreshold <= 0 {
		c.EventCounting.AlertThreshold = 10
	}
	if c.EventCounting.RateThresholdPerMinute <= 0 {
		c.EventCounting.RateThresholdPerMinute = 5.0
	}

	// Escalation defaults
	if c.Escalation.CountThreshold <= 0 {
		c.Escalation.CountThreshold = 100
	}
	if c.Escalation.RateThresholdPerMinute <= 0 {
		c.Escalation.RateThresholdPerMinute = 10.0
	}

	// Detection defaults
	if c.Detection.Workers <= 0 {
		c.Detection.Workers = runtime.NumCPU()
		if c.Detection.Workers < 1 {
			c.Detection.Workers = 4
		}
	}
	if c.Detection.BatchSize <= 0 {
		c.Detection.BatchSize = 100
	}
	if c.Detection.CacheSize <= 0 {
		c.Detection.CacheSize = 10000
	}
	if c.Detection.MinConfidence <= 0 {
		c.Detection.MinConfidence = 0.6
	}
	if c.Detection.EnableFilters == nil {
		v := true
		c.Detection.EnableFilters = &v
	}
	if c.Detection.EnableContextValidation == nil {
		v := false
		c.Detection.EnableContextValidation = &v
	}

	// Filtering defaults
	if c.Filtering.Enable == nil {
		v := false
		c.Filtering.Enable = &v
	}

	// Output defaults
	if c.Output.OutputFile == "" {
		c.Output.OutputFile = "data/alerts.jsonl"
	}
	if c.Output.LogLevel == "" {
		c.Output.LogLevel = "info"
	}

	// Rules defaults
	if c.Rules.RulesDirectory == "" {
		c.Rules.RulesDirectory = "sigma_rules/rules"
	}
	// Default product whitelist: only Windows (for EDR agent)
	if len(c.Rules.ProductWhitelist) == 0 {
		c.Rules.ProductWhitelist = []string{"windows"}
	}
	// Rule cache defaults
	if c.Rules.CacheFile == "" {
		c.Rules.CacheFile = "cache/sigma_rules.cache"
	}
	if c.Rules.CacheMaxAgeHours <= 0 {
		c.Rules.CacheMaxAgeHours = 24
	}
	// Rule parsing is IO-bound; default to CPU*4 with a cap.
	if c.Rules.ParallelWorkers <= 0 {
		pw := runtime.NumCPU() * 4
		if pw < 4 {
			pw = 4
		}
		if pw > 32 {
			pw = 32
		}
		c.Rules.ParallelWorkers = pw
	}
	// Rule quality defaults
	if c.Rules.MinLevel == "" {
		c.Rules.MinLevel = "medium"
	}
	if len(c.Rules.AllowedStatus) == 0 {
		c.Rules.AllowedStatus = []string{"stable", "test"}
	}
	if c.Rules.SkipExperimental == nil {
		v := true
		c.Rules.SkipExperimental = &v
	}
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 4
	}
	enableFilters := true
	enableContextValidation := false
	enableFiltering := false
	skipExperimental := true

	return &Config{
		FileMonitoring: FileMonitoringConfig{
			WatchDirectory:            "data/agent_ecs-events",
			FilePattern:               "*.jsonl",
			PollIntervalMS:            100,
			MaxFileSizeGB:             1,
			CheckpointFile:            "data/checkpoint.json",
			CheckpointIntervalSeconds: 30,
		},
		EventCounting: EventCountingConfig{
			WindowSizeMinutes:      5,
			AlertThreshold:         10,
			RateThresholdPerMinute: 5.0,
		},
		Escalation: EscalationConfig{
			CountThreshold:           100,
			RateThresholdPerMinute:   10.0,
			EnableCriticalEscalation: true,
		},
		Detection: DetectionConfig{
			Workers:                 workers,
			BatchSize:               100,
			CacheSize:               10000,
			MinConfidence:           0.6,
			EnableFilters:           &enableFilters,
			EnableContextValidation: &enableContextValidation,
		},
		Filtering: FilteringConfig{
			Enable: &enableFiltering,
		},
		Output: OutputConfig{
			OutputFile: "data/alerts.jsonl",
			LogLevel:   "info",
		},
		Rules: RulesConfig{
			RulesDirectory:   "sigma_rules/rules",
			ProductWhitelist: []string{"windows"}, // Default: only Windows rules
			CacheFile:        "cache/sigma_rules.cache",
			CacheMaxAgeHours: 24,
			ParallelWorkers:  32,
			MinLevel:         "medium",
			AllowedStatus:    []string{"stable", "test"},
			SkipExperimental: &skipExperimental,
		},
	}
}

// PollInterval returns the poll interval as a time.Duration.
func (fmc *FileMonitoringConfig) PollInterval() time.Duration {
	return time.Duration(fmc.PollIntervalMS) * time.Millisecond
}

// CheckpointInterval returns the checkpoint interval as a time.Duration.
func (fmc *FileMonitoringConfig) CheckpointInterval() time.Duration {
	return time.Duration(fmc.CheckpointIntervalSeconds) * time.Second
}

// WindowSize returns the window size as a time.Duration.
func (ecc *EventCountingConfig) WindowSize() time.Duration {
	return time.Duration(ecc.WindowSizeMinutes) * time.Minute
}

// EnsureOutputFile ensures the output file and its directory exist.
// Creates the directory if it doesn't exist and creates an empty file if needed.
func (oc *OutputConfig) EnsureOutputFile() error {
	// Get absolute path
	absPath, err := filepath.Abs(oc.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for output file: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", dir, err)
	}

	// Create file if it doesn't exist
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		file, err := os.Create(absPath)
		if err != nil {
			return fmt.Errorf("failed to create output file %s: %w", absPath, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close output file %s: %w", absPath, err)
		}
		logger.Infof("Created output file: %s", absPath)
	} else if err != nil {
		return fmt.Errorf("failed to stat output file %s: %w", absPath, err)
	}

	// Verify file is writable
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("output file %s is not writable: %w", absPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close output file %s: %w", absPath, err)
	}

	return nil
}

// EnsureWatchDirectory ensures the watch directory exists.
func (fmc *FileMonitoringConfig) EnsureWatchDirectory() error {
	// Get absolute path
	absPath, err := filepath.Abs(fmc.WatchDirectory)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for watch directory: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return fmt.Errorf("failed to create watch directory %s: %w", absPath, err)
		}
		logger.Infof("Created watch directory: %s", absPath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat watch directory %s: %w", absPath, err)
	}

	// Verify it's a directory
	if !info.IsDir() {
		return fmt.Errorf("watch path %s is not a directory", absPath)
	}

	// Verify it's readable
	if info.Mode().Perm()&0444 == 0 {
		return fmt.Errorf("watch directory %s is not readable", absPath)
	}

	return nil
}

// Validate validates the configuration and returns any errors.
func (c *Config) Validate() error {
	// Validate file monitoring
	if err := c.FileMonitoring.EnsureWatchDirectory(); err != nil {
		return fmt.Errorf("file monitoring validation failed: %w", err)
	}

	// Validate output
	if err := c.Output.EnsureOutputFile(); err != nil {
		return fmt.Errorf("output validation failed: %w", err)
	}

	// Validate rules directory
	if c.Rules.RulesDirectory != "" {
		info, err := os.Stat(c.Rules.RulesDirectory)
		if os.IsNotExist(err) {
			return fmt.Errorf("rules directory does not exist: %s", c.Rules.RulesDirectory)
		}
		if err != nil {
			return fmt.Errorf("failed to stat rules directory %s: %w", c.Rules.RulesDirectory, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("rules path is not a directory: %s", c.Rules.RulesDirectory)
		}
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": false,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Output.LogLevel] {
		return fmt.Errorf("invalid log level: %s (must be: debug, info, warn, error)", c.Output.LogLevel)
	}

	return nil
}
