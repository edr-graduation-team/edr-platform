//go:build windows
// +build windows

package collectors

import (
	"context"

	"github.com/edr-platform/win-agent/internal/event"
)

// FileAutoResponse optionally performs local hash-match quarantine on high-risk file paths.
type FileAutoResponse interface {
	EvaluateAndAct(ctx context.Context, filePath string, opcode uint8, pid uint32, base map[string]interface{}) (*event.Event, bool)
}

// ProcessAutoResponse optionally performs local process enforcement on process creation events.
type ProcessAutoResponse interface {
	EvaluateAndAct(ctx context.Context, base map[string]interface{}) (*event.Event, bool)
}
