package ports

import "time"

// EngineConfig configures the detection engine.
// Use DefaultEngineConfig() for production defaults.
type EngineConfig struct {
	// WorkerCount is the number of parallel workers for event processing.
	// Default: runtime.NumCPU()
	WorkerCount int `json:"worker_count"`

	// MaxQueueSize is the maximum number of events that can be queued.
	// Default: 1000
	MaxQueueSize int `json:"max_queue_size"`

	// EnableRegex enables regex pattern matching in rules.
	// Default: true
	EnableRegex bool `json:"enable_regex"`

	// RegexTimeoutMs is the timeout for regex evaluation in milliseconds.
	// Default: 100
	RegexTimeoutMs int `json:"regex_timeout_ms"`

	// EnableCaching enables field resolution caching for performance.
	// Default: true
	EnableCaching bool `json:"enable_caching"`

	// CacheSize is the number of entries in the field resolution cache.
	// Default: 10000
	CacheSize int `json:"cache_size"`

	// MinConfidence is the minimum confidence score for detections.
	// Detections below this threshold are filtered out.
	// Default: 0.6 (60%)
	MinConfidence float64 `json:"min_confidence"`

	// EnableFilters enables filter selection suppression.
	// Default: true
	EnableFilters bool `json:"enable_filters"`

	// EnableContextValidation enables context-based confidence adjustments.
	// Default: false
	EnableContextValidation bool `json:"enable_context_validation"`

	// ShutdownTimeout is the maximum time to wait for graceful shutdown.
	// Default: 30 seconds
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`

	// LogLevel controls logging verbosity.
	// Default: "info"
	LogLevel string `json:"log_level"`
}

// DefaultEngineConfig returns production-ready default configuration.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		WorkerCount:             4,
		MaxQueueSize:            1000,
		EnableRegex:             true,
		RegexTimeoutMs:          100,
		EnableCaching:           true,
		CacheSize:               10000,
		MinConfidence:           0.6,
		EnableFilters:           true,
		EnableContextValidation: false,
		ShutdownTimeout:         30 * time.Second,
		LogLevel:                "info",
	}
}

// Validate validates the configuration and returns an error if invalid.
func (c *EngineConfig) Validate() error {
	if c.WorkerCount < 1 {
		c.WorkerCount = 1
	}
	if c.MaxQueueSize < 1 {
		c.MaxQueueSize = 100
	}
	if c.CacheSize < 0 {
		c.CacheSize = 0
	}
	if c.MinConfidence < 0 {
		c.MinConfidence = 0
	}
	if c.MinConfidence > 1 {
		c.MinConfidence = 1
	}
	if c.ShutdownTimeout < time.Second {
		c.ShutdownTimeout = time.Second
	}
	return nil
}
