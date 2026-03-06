package output

import (
	"fmt"
	"sync"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// OutputWriter defines the interface for alert output writers.
type OutputWriter interface {
	WriteAlert(alert *domain.Alert) error
	Close() error
	Stats() OutputStats
}

// OutputStats represents statistics for an output writer.
type OutputStats struct {
	AlertsWritten uint64
	Errors       uint64
	LastWrite    string
}

// OutputManager manages multiple output writers.
type OutputManager struct {
	outputs map[string]OutputWriter
	mu      sync.RWMutex
	stats   *OutputManagerStats
}

// OutputManagerStats tracks output manager statistics.
type OutputManagerStats struct {
	TotalWrites    uint64
	SuccessfulWrites uint64
	FailedWrites   uint64
	mu             sync.RWMutex
}

// NewOutputManager creates a new output manager.
func NewOutputManager() *OutputManager {
	return &OutputManager{
		outputs: make(map[string]OutputWriter),
		stats:   &OutputManagerStats{},
	}
}

// RegisterOutput registers an output writer.
func (om *OutputManager) RegisterOutput(name string, writer OutputWriter) {
	om.mu.Lock()
	defer om.mu.Unlock()

	om.outputs[name] = writer
	logger.Infof("Registered output: %s", name)
}

// WriteAlert writes an alert to all registered outputs.
func (om *OutputManager) WriteAlert(alert *domain.Alert) error {
	if alert == nil {
		return fmt.Errorf("alert is nil")
	}

	om.mu.RLock()
	defer om.mu.RUnlock()

	var errs []error
	successCount := 0

	for name, writer := range om.outputs {
		if err := writer.WriteAlert(alert); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			om.stats.mu.Lock()
			om.stats.FailedWrites++
			om.stats.mu.Unlock()
		} else {
			successCount++
			om.stats.mu.Lock()
			om.stats.SuccessfulWrites++
			om.stats.mu.Unlock()
		}
	}

	om.stats.mu.Lock()
	om.stats.TotalWrites++
	om.stats.mu.Unlock()

	if len(errs) > 0 && successCount == 0 {
		return fmt.Errorf("all outputs failed: %v", errs)
	}

	if len(errs) > 0 {
		logger.Warnf("Some outputs failed: %v", errs)
	}

	return nil
}

// Close closes all registered outputs.
func (om *OutputManager) Close() error {
	om.mu.Lock()
	defer om.mu.Unlock()

	var errs []error
	for name, writer := range om.outputs {
		if err := writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		logger.Infof("Closed output: %s", name)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing outputs: %v", errs)
	}

	return nil
}

// Stats returns output manager statistics.
func (om *OutputManager) Stats() OutputManagerStats {
	om.stats.mu.RLock()
	defer om.stats.mu.RUnlock()

	return OutputManagerStats{
		TotalWrites:      om.stats.TotalWrites,
		SuccessfulWrites: om.stats.SuccessfulWrites,
		FailedWrites:     om.stats.FailedWrites,
	}
}

// GetOutput returns an output writer by name.
func (om *OutputManager) GetOutput(name string) (OutputWriter, bool) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	writer, ok := om.outputs[name]
	return writer, ok
}

