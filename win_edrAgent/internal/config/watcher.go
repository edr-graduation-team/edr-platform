// Package config provides runtime configuration watching and hot-reload.
package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// Watcher monitors configuration file for changes.
type Watcher struct {
	logger        *logging.Logger
	configPath    string
	checkInterval time.Duration

	// State
	running     bool
	mu          sync.Mutex
	lastHash    string
	lastModTime time.Time

	// Callbacks
	onConfigChange func(*Config)
}

// NewWatcher creates a new configuration watcher.
func NewWatcher(configPath string, logger *logging.Logger) *Watcher {
	return &Watcher{
		logger:        logger,
		configPath:    configPath,
		checkInterval: 30 * time.Second,
	}
}

// SetCallback sets the function to call when config changes.
func (w *Watcher) SetCallback(callback func(*Config)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onConfigChange = callback
}

// Start begins watching the configuration file.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	// Get initial hash
	w.lastHash = w.getFileHash()

	w.logger.Info("Configuration watcher started")
	go w.watchLoop(ctx)
}

// Stop stops the configuration watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running = false
	w.logger.Info("Configuration watcher stopped")
}

// watchLoop periodically checks for configuration changes.
func (w *Watcher) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			running := w.running
			w.mu.Unlock()

			if !running {
				return
			}

			w.checkForChanges()
		}
	}
}

// checkForChanges checks if the config file has been modified.
func (w *Watcher) checkForChanges() {
	currentHash := w.getFileHash()
	if currentHash == "" || currentHash == w.lastHash {
		return
	}

	w.logger.Info("Configuration change detected, reloading...")

	// Load new config
	cfg, err := Load(w.configPath)
	if err != nil {
		w.logger.Errorf("Failed to reload configuration: %v", err)
		return
	}

	// Update hash
	w.lastHash = currentHash

	// Call callback
	w.mu.Lock()
	callback := w.onConfigChange
	w.mu.Unlock()

	if callback != nil {
		callback(cfg)
		w.logger.Info("Configuration reloaded successfully")
	}
}

// getFileHash returns SHA256 hash of the config file.
func (w *Watcher) getFileHash() string {
	data, err := os.ReadFile(w.configPath)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// RuntimeConfig holds runtime-adjustable settings.
type RuntimeConfig struct {
	mu sync.RWMutex

	// Logging
	LogLevel string

	// Batching
	BatchSize     int
	BatchInterval time.Duration

	// Filtering
	ExcludeProcesses []string
	ExcludeIPs       []string

	// Server
	ServerAddress string
}

// NewRuntimeConfig creates runtime config from static config.
func NewRuntimeConfig(cfg *Config) *RuntimeConfig {
	return &RuntimeConfig{
		LogLevel:         cfg.Logging.Level,
		BatchSize:        cfg.Agent.BatchSize,
		BatchInterval:    cfg.Agent.BatchInterval,
		ExcludeProcesses: cfg.Filtering.ExcludeProcesses,
		ExcludeIPs:       cfg.Filtering.ExcludeIPs,
		ServerAddress:    cfg.Server.Address,
	}
}

// Update applies new settings from a config update.
func (r *RuntimeConfig) Update(updates map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if level, ok := updates["log_level"].(string); ok {
		r.LogLevel = level
	}
	if size, ok := updates["batch_size"].(int); ok {
		if size >= 1 && size <= 10000 {
			r.BatchSize = size
		}
	}
	if interval, ok := updates["batch_interval"].(string); ok {
		if d, err := time.ParseDuration(interval); err == nil {
			r.BatchInterval = d
		}
	}
}

// GetLogLevel returns current log level.
func (r *RuntimeConfig) GetLogLevel() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.LogLevel
}

// GetBatchSize returns current batch size.
func (r *RuntimeConfig) GetBatchSize() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.BatchSize
}

// GetBatchInterval returns current batch interval.
func (r *RuntimeConfig) GetBatchInterval() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.BatchInterval
}

// ServerConfigUpdate represents a configuration update from server.
type ServerConfigUpdate struct {
	UpdateID        string                 `json:"update_id"`
	Timestamp       time.Time              `json:"timestamp"`
	Settings        map[string]interface{} `json:"settings"`
	RequiresRestart bool                   `json:"requires_restart"`
}

// ApplyServerUpdate applies a configuration update from the server.
func (r *RuntimeConfig) ApplyServerUpdate(update *ServerConfigUpdate, logger *logging.Logger) error {
	if update == nil {
		return nil
	}

	logger.Infof("Applying server config update: %s", update.UpdateID)

	r.Update(update.Settings)

	if update.RequiresRestart {
		logger.Warn("Configuration update requires restart")
	}

	return nil
}
