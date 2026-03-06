package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

// EnhancedJSONLOutput writes enhanced alerts in JSONL format.
type EnhancedJSONLOutput struct {
	file  *os.File
	mu    sync.Mutex
	stats OutputStats
}

// NewEnhancedJSONLOutput creates a new enhanced JSONL output writer.
// Creates the file and directory if they don't exist.
func NewEnhancedJSONLOutput(filePath string) (*EnhancedJSONLOutput, error) {
	// Get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for output file: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %s: %w", dir, err)
	}

	// Create file if it doesn't exist
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		file, err := os.Create(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file %s: %w", absPath, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("failed to close output file %s: %w", absPath, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat output file %s: %w", absPath, err)
	}

	// Open file for appending
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", absPath, err)
	}

	return &EnhancedJSONLOutput{
		file: file,
		stats: OutputStats{},
	}, nil
}

// WriteEnhancedAlert writes an enhanced alert as a single JSON line.
func (ejlo *EnhancedJSONLOutput) WriteEnhancedAlert(alert *domain.EnhancedAlert) error {
	if alert == nil {
		return fmt.Errorf("alert is nil")
	}

	ejlo.mu.Lock()
	defer ejlo.mu.Unlock()

	data, err := json.Marshal(alert)
	if err != nil {
		ejlo.stats.Errors++
		return fmt.Errorf("json marshaling failed: %w", err)
	}

	if _, err := ejlo.file.Write(data); err != nil {
		ejlo.stats.Errors++
		return fmt.Errorf("write failed: %w", err)
	}

	if _, err := ejlo.file.WriteString("\n"); err != nil {
		ejlo.stats.Errors++
		return fmt.Errorf("write failed: %w", err)
	}

	ejlo.stats.AlertsWritten++
	return nil
}

// WriteAlert implements OutputWriter interface (for compatibility).
// Converts regular Alert to EnhancedAlert if needed.
func (ejlo *EnhancedJSONLOutput) WriteAlert(alert *domain.Alert) error {
	if alert == nil {
		return fmt.Errorf("alert is nil")
	}

	// Convert to EnhancedAlert
	enhanced := domain.NewEnhancedAlert(alert)
	return ejlo.WriteEnhancedAlert(enhanced)
}

// Close closes the output file.
func (ejlo *EnhancedJSONLOutput) Close() error {
	ejlo.mu.Lock()
	defer ejlo.mu.Unlock()

	if ejlo.file != nil {
		return ejlo.file.Close()
	}
	return nil
}

// Stats returns output statistics.
func (ejlo *EnhancedJSONLOutput) Stats() OutputStats {
	ejlo.mu.Lock()
	defer ejlo.mu.Unlock()

	return OutputStats{
		AlertsWritten: ejlo.stats.AlertsWritten,
		Errors:        ejlo.stats.Errors,
	}
}

