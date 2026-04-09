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

	// Global telemetry filter from config
	evtFilter := collectors.NewFilter(collectors.FilterConfig{
		ExcludeProcesses: cfg.Filtering.ExcludeProcesses,
		ExcludeIPs:       cfg.Filtering.ExcludeIPs,
		ExcludeRegistry:  cfg.Filtering.ExcludeRegistry,
		ExcludePaths:     cfg.Filtering.ExcludePaths,
		IncludePaths:     cfg.Filtering.IncludePaths,
		ExcludeEventIDs:  cfg.Filtering.ExcludeEventIDs,
		TrustedHashes:    cfg.Filtering.TrustedHashes,
	}, logger)

	// ETW collector (process events + optional file I/O + image load)
	// All three event types share a single kernel session for minimal overhead.
	if cfg.Collectors.ETWEnabled {
		sessionName := cfg.Collectors.ETWSessionName
		if sessionName == "" {
			sessionName = "EDRAgentSession"
		}
		etw := collectors.NewETWCollector(
			sessionName, eventChan, logger, evtFilter,
			cfg.Collectors.FileEnabled,
			cfg.Collectors.ImageLoadEnabled,
		)
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
		net := collectors.NewNetworkCollector(eventChan, evtFilter, cfg.Filtering.FilterPrivateNetworks, logger)
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

	// NOTE: File monitoring and Image Load detection are now handled by
	// the ETW collector above (EVENT_TRACE_FLAG_FILE_IO_INIT and
	// EVENT_TRACE_FLAG_IMAGE_LOAD). They fire real-time from the kernel
	// with exact PID attribution — no separate polling collectors needed.
}
