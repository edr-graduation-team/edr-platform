//go:build !windows
// +build !windows

package agent

import (
	"context"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// startPlatformCollectors is a no-op on non-Windows platforms.
func startPlatformCollectors(ctx context.Context, cfg *config.Config, eventChan chan<- *event.Event, logger *logging.Logger) {
	_ = ctx
	_ = cfg
	_ = eventChan
	_ = logger
}
