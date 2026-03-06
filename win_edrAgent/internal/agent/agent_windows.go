//go:build windows
// +build windows

package agent

import (
	"context"

	"github.com/edr-platform/win-agent/internal/collectors"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// startPlatformCollectors starts Windows-specific collectors (ETW, Registry, Network, WMI) when enabled.
// Each collector runs in its own goroutine and respects ctx.Done() for graceful shutdown.
// A shared telemetry filter is built from cfg.Filtering to reduce noise (ExcludePaths, ExcludeIPs, etc.).
// Panics in collector goroutines are recovered so one failing collector cannot crash the agent.
func startPlatformCollectors(ctx context.Context, cfg *config.Config, eventChan chan<- *event.Event, logger *logging.Logger) {
	if cfg == nil || logger == nil || eventChan == nil {
		return
	}

	// Global telemetry filter from config (ExcludeProcesses, ExcludeIPs, ExcludeRegistry, ExcludePaths, IncludePaths)
	evtFilter := collectors.NewFilter(collectors.FilterConfig{
		ExcludeProcesses: cfg.Filtering.ExcludeProcesses,
		ExcludeIPs:       cfg.Filtering.ExcludeIPs,
		ExcludeRegistry:  cfg.Filtering.ExcludeRegistry,
		ExcludePaths:     cfg.Filtering.ExcludePaths,
		IncludePaths:     cfg.Filtering.IncludePaths,
	}, logger)

	// ETW collector (no filter parameter in current API)
	if cfg.Collectors.ETWEnabled {
		sessionName := cfg.Collectors.ETWSessionName
		if sessionName == "" {
			sessionName = "EDRAgentSession"
		}
		etw := collectors.NewETWCollector(sessionName, eventChan, logger)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("ETW collector panicked and was safely recovered: %v", r)
				}
			}()
			if err := etw.Start(ctx); err != nil {
				logger.Warnf("ETW collector failed to start: %v", err)
			} else {
				logger.Info("ETW collector started")
			}
		}()
	} else {
		logger.Debug("ETW collector disabled by config")
	}

	// Registry collector (persistence / config change monitoring)
	if cfg.Collectors.RegistryEnabled {
		reg := collectors.NewRegistryCollector(eventChan, evtFilter, logger)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Registry collector panicked and was safely recovered: %v", r)
				}
			}()
			if err := reg.Start(ctx); err != nil {
				logger.Warnf("Registry collector failed to start: %v", err)
			} else {
				logger.Info("Registry collector started")
			}
		}()
	} else {
		logger.Debug("Registry collector disabled by config")
	}

	// Network collector (connection monitoring)
	if cfg.Collectors.NetworkEnabled {
		net := collectors.NewNetworkCollector(eventChan, evtFilter, logger)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Network collector panicked and was safely recovered: %v", r)
				}
			}()
			if err := net.Start(ctx); err != nil {
				logger.Warnf("Network collector failed to start: %v", err)
			} else {
				logger.Info("Network collector started")
			}
		}()
	} else {
		logger.Debug("Network collector disabled by config")
	}

	// WMI collector (inventory / periodic system events)
	if cfg.Collectors.WMIEnabled {
		interval := cfg.Collectors.WMIInterval
		wmi := collectors.NewWMICollector(interval, eventChan, logger)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("WMI collector panicked and was safely recovered: %v", r)
				}
			}()
			if err := wmi.Start(ctx); err != nil {
				logger.Warnf("WMI collector failed to start: %v", err)
			} else {
				logger.Info("WMI collector started")
			}
		}()
	} else {
		logger.Debug("WMI collector disabled by config")
	}
}
