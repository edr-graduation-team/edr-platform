package output

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

// JSONLOutput writes alerts in JSONL format (one JSON object per line).
type JSONLOutput struct {
	file  *os.File
	mu    sync.Mutex
	stats OutputStats
}

// NewJSONLOutput creates a new JSONL output writer.
func NewJSONLOutput(filePath string) (*JSONLOutput, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}

	return &JSONLOutput{
		file: file,
		stats: OutputStats{},
	}, nil
}

// WriteAlert writes an alert as a single JSON line.
func (jlo *JSONLOutput) WriteAlert(alert *domain.Alert) error {
	jlo.mu.Lock()
	defer jlo.mu.Unlock()

	data, err := json.Marshal(alert)
	if err != nil {
		jlo.stats.Errors++
		return fmt.Errorf("json marshaling failed: %w", err)
	}

	if _, err := jlo.file.Write(data); err != nil {
		jlo.stats.Errors++
		return fmt.Errorf("write failed: %w", err)
	}

	if _, err := jlo.file.WriteString("\n"); err != nil {
		jlo.stats.Errors++
		return fmt.Errorf("write failed: %w", err)
	}

	jlo.stats.AlertsWritten++
	return nil
}

// Close closes the output file.
func (jlo *JSONLOutput) Close() error {
	jlo.mu.Lock()
	defer jlo.mu.Unlock()

	if jlo.file != nil {
		return jlo.file.Close()
	}
	return nil
}

// Stats returns output statistics.
func (jlo *JSONLOutput) Stats() OutputStats {
	jlo.mu.Lock()
	defer jlo.mu.Unlock()

	return OutputStats{
		AlertsWritten: jlo.stats.AlertsWritten,
		Errors:        jlo.stats.Errors,
	}
}

