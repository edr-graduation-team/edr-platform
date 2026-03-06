// Package grpcclient provides automatic reconnection with exponential backoff.
package grpcclient

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/logging"
)

// ReconnectManager handles automatic reconnection with exponential backoff.
type ReconnectManager struct {
	logger      *logging.Logger
	cfg         *config.Config
	connectFunc func(context.Context) error

	// State
	running      atomic.Bool
	connected    atomic.Bool
	reconnecting atomic.Bool

	// Backoff settings
	baseDelay    time.Duration
	maxDelay     time.Duration
	currentDelay time.Duration
	mu           sync.Mutex

	// Metrics
	reconnectAttempts atomic.Uint64
	reconnectSuccess  atomic.Uint64
	reconnectFailed   atomic.Uint64
	lastConnected     time.Time
}

// NewReconnectManager creates a new reconnection manager.
func NewReconnectManager(cfg *config.Config, connectFunc func(context.Context) error, logger *logging.Logger) *ReconnectManager {
	baseDelay := cfg.Server.ReconnectDelay
	if baseDelay <= 0 {
		baseDelay = time.Second
	}

	maxDelay := cfg.Server.MaxReconnectDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	return &ReconnectManager{
		logger:       logger,
		cfg:          cfg,
		connectFunc:  connectFunc,
		baseDelay:    baseDelay,
		maxDelay:     maxDelay,
		currentDelay: baseDelay,
	}
}

// Start begins the reconnection monitoring loop.
func (r *ReconnectManager) Start(ctx context.Context) {
	if r.running.Load() {
		return
	}

	r.running.Store(true)
	r.logger.Info("Reconnection manager started")

	go r.monitorLoop(ctx)
}

// Stop stops the reconnection manager.
func (r *ReconnectManager) Stop() {
	r.running.Store(false)
	r.logger.Info("Reconnection manager stopped")
}

// monitorLoop monitors connection and triggers reconnection.
func (r *ReconnectManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !r.running.Load() {
				return
			}

			if !r.connected.Load() && !r.reconnecting.Load() {
				r.attemptReconnect(ctx)
			}
		}
	}
}

// attemptReconnect tries to reconnect with exponential backoff.
func (r *ReconnectManager) attemptReconnect(ctx context.Context) {
	r.reconnecting.Store(true)
	defer r.reconnecting.Store(false)

	r.mu.Lock()
	delay := r.currentDelay
	r.mu.Unlock()

	r.logger.Infof("Attempting reconnection in %v...", delay)

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	r.reconnectAttempts.Add(1)

	// Create timeout context for connection attempt
	connectCtx, cancel := context.WithTimeout(ctx, r.cfg.Server.Timeout)
	defer cancel()

	err := r.connectFunc(connectCtx)
	if err != nil {
		r.reconnectFailed.Add(1)
		r.logger.Warnf("Reconnection failed: %v", err)

		// Increase delay (exponential backoff)
		r.mu.Lock()
		r.currentDelay = r.currentDelay * 2
		if r.currentDelay > r.maxDelay {
			r.currentDelay = r.maxDelay
		}
		r.mu.Unlock()

		return
	}

	// Success
	r.reconnectSuccess.Add(1)
	r.connected.Store(true)
	r.lastConnected = time.Now()

	// Reset delay on success
	r.mu.Lock()
	r.currentDelay = r.baseDelay
	r.mu.Unlock()

	r.logger.Info("Reconnection successful")
}

// SetConnected updates connection status.
func (r *ReconnectManager) SetConnected(connected bool) {
	r.connected.Store(connected)
	if connected {
		r.lastConnected = time.Now()
	}
}

// IsConnected returns current connection status.
func (r *ReconnectManager) IsConnected() bool {
	return r.connected.Load()
}

// IsReconnecting returns whether currently attempting reconnection.
func (r *ReconnectManager) IsReconnecting() bool {
	return r.reconnecting.Load()
}

// GetCurrentDelay returns the current backoff delay.
func (r *ReconnectManager) GetCurrentDelay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentDelay
}

// ResetBackoff resets the backoff delay to base.
func (r *ReconnectManager) ResetBackoff() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentDelay = r.baseDelay
}

// Stats returns reconnection statistics.
func (r *ReconnectManager) Stats() ReconnectStats {
	r.mu.Lock()
	delay := r.currentDelay
	r.mu.Unlock()

	return ReconnectStats{
		Running:           r.running.Load(),
		Connected:         r.connected.Load(),
		Reconnecting:      r.reconnecting.Load(),
		CurrentDelay:      delay,
		ReconnectAttempts: r.reconnectAttempts.Load(),
		ReconnectSuccess:  r.reconnectSuccess.Load(),
		ReconnectFailed:   r.reconnectFailed.Load(),
		LastConnected:     r.lastConnected,
	}
}

// ReconnectStats holds reconnection statistics.
type ReconnectStats struct {
	Running           bool
	Connected         bool
	Reconnecting      bool
	CurrentDelay      time.Duration
	ReconnectAttempts uint64
	ReconnectSuccess  uint64
	ReconnectFailed   uint64
	LastConnected     time.Time
}

// Uptime returns time since last successful connection.
func (r *ReconnectManager) Uptime() time.Duration {
	if !r.connected.Load() || r.lastConnected.IsZero() {
		return 0
	}
	return time.Since(r.lastConnected)
}
