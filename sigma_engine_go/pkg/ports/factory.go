package ports

import (
	"context"
	"fmt"
)

// EngineFactory creates DetectionEngine instances.
// This factory pattern allows for centralized engine creation with configuration.
//
// Usage:
//
//	config := ports.DefaultEngineConfig()
//	factory := ports.NewEngineFactory(config)
//	engine, err := factory.Create(ctx)
type EngineFactory struct {
	config EngineConfig
}

// NewEngineFactory creates a new factory with the given configuration.
func NewEngineFactory(config EngineConfig) *EngineFactory {
	// Validate and normalize config
	_ = config.Validate()
	return &EngineFactory{config: config}
}

// Config returns the factory's configuration.
func (f *EngineFactory) Config() EngineConfig {
	return f.config
}

// CreateFunc is the function type that actually creates an engine.
// This is set by the internal package to avoid circular imports.
var CreateFunc func(ctx context.Context, config EngineConfig) (DetectionEngine, error)

// Create creates a new DetectionEngine instance.
// The actual creation logic is provided by the internal package via CreateFunc.
//
// Returns error if CreateFunc is not set (internal package not imported)
// or if engine creation fails.
func (f *EngineFactory) Create(ctx context.Context) (DetectionEngine, error) {
	if CreateFunc == nil {
		return nil, fmt.Errorf("engine factory not initialized: import the detection package to register the factory")
	}
	return CreateFunc(ctx, f.config)
}
