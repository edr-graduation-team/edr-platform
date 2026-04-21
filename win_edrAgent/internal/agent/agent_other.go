//go:build !windows
// +build !windows

package agent

import (
	"context"
)

// startPlatformCollectors is a no-op on non-Windows platforms.
func startPlatformCollectors(ctx context.Context, a *Agent) {
	_ = ctx
	_ = a
}
