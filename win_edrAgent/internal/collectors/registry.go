// Package collectors provides registry monitoring.
//go:build windows
// +build windows

package collectors

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// RegistryCollector monitors registry changes.
type RegistryCollector struct {
	logger    *logging.Logger
	eventChan chan<- *event.Event
	filter    *Filter

	// Keys to monitor (persistence mechanisms)
	watchKeys []RegistryWatchKey

	// State
	running atomic.Bool
	mu      sync.Mutex

	// Value cache for change detection
	valueCache map[string]string
	cacheMu    sync.RWMutex

	// Metrics
	keysMonitored   int
	changesDetected atomic.Uint64
	eventsGenerated atomic.Uint64
}

// RegistryWatchKey defines a registry key to monitor.
type RegistryWatchKey struct {
	Hive registry.Key
	Path string
	Name string // Human-readable name
}

// DefaultWatchKeys returns the default registry keys to monitor.
func DefaultWatchKeys() []RegistryWatchKey {
	return []RegistryWatchKey{
		// Run keys (persistence)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKLM Run"},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, "HKLM RunOnce"},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKCU Run"},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, "HKCU RunOnce"},

		// Services
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services`, "Services"},

		// Startup folder
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, "Shell Folders"},

		// Winlogon
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon`, "Winlogon"},

		// Image File Execution Options (debugger hijack)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`, "IFEO"},

		// AppInit DLLs
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Windows`, "AppInit"},

		// Scheduled Tasks
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Schedule\TaskCache\Tasks`, "Tasks"},
	}
}

// NewRegistryCollector creates a new registry collector.
func NewRegistryCollector(eventChan chan<- *event.Event, filter *Filter, logger *logging.Logger) *RegistryCollector {
	return &RegistryCollector{
		logger:     logger,
		eventChan:  eventChan,
		filter:     filter,
		watchKeys:  DefaultWatchKeys(),
		valueCache: make(map[string]string),
	}
}

// Start begins registry monitoring.
func (c *RegistryCollector) Start(ctx context.Context) error {
	if c.running.Load() {
		return nil
	}

	c.logger.Info("Starting registry collector...")
	c.logger.Infof("Monitoring %d registry keys", len(c.watchKeys))

	c.running.Store(true)
	c.keysMonitored = len(c.watchKeys)

	// Initial baseline
	c.collectBaseline()

	// Start monitoring loop
	go c.monitorLoop(ctx)

	c.logger.Info("Registry collector started")
	return nil
}

// Stop stops registry monitoring.
func (c *RegistryCollector) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.logger.Info("Stopping registry collector...")
	c.running.Store(false)

	c.logger.Infof("Registry stats: changes=%d events=%d",
		c.changesDetected.Load(),
		c.eventsGenerated.Load())

	return nil
}

// collectBaseline reads initial values for all monitored keys.
func (c *RegistryCollector) collectBaseline() {
	for _, wk := range c.watchKeys {
		c.readKeyValues(wk, true)
	}
}

// monitorLoop periodically checks for registry changes.
func (c *RegistryCollector) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.running.Load() {
				return
			}
			c.checkForChanges()
		}
	}
}

// checkForChanges scans all monitored keys for changes.
func (c *RegistryCollector) checkForChanges() {
	for _, wk := range c.watchKeys {
		c.readKeyValues(wk, false)
	}
}

// readKeyValues reads and optionally compares registry values.
func (c *RegistryCollector) readKeyValues(wk RegistryWatchKey, baseline bool) {
	key, err := registry.OpenKey(wk.Hive, wk.Path, registry.READ)
	if err != nil {
		return // Key doesn't exist or access denied
	}
	defer key.Close()

	valueNames, err := key.ReadValueNames(-1)
	if err != nil {
		return
	}

	for _, valueName := range valueNames {
		value, _, err := key.GetStringValue(valueName)
		if err != nil {
			continue
		}

		cacheKey := wk.Name + "\\" + valueName

		c.cacheMu.RLock()
		oldValue, exists := c.valueCache[cacheKey]
		c.cacheMu.RUnlock()

		if baseline {
			// Just store baseline
			c.cacheMu.Lock()
			c.valueCache[cacheKey] = value
			c.cacheMu.Unlock()
		} else if !exists {
			// New value
			c.changesDetected.Add(1)
			c.generateEvent("created", wk, valueName, "", value)

			c.cacheMu.Lock()
			c.valueCache[cacheKey] = value
			c.cacheMu.Unlock()
		} else if oldValue != value {
			// Value changed
			c.changesDetected.Add(1)
			c.generateEvent("modified", wk, valueName, oldValue, value)

			c.cacheMu.Lock()
			c.valueCache[cacheKey] = value
			c.cacheMu.Unlock()
		}
	}
}

// generateEvent creates a registry event.
func (c *RegistryCollector) generateEvent(action string, wk RegistryWatchKey, valueName, oldValue, newValue string) {
	hivePrefix := "HKLM"
	if wk.Hive == registry.CURRENT_USER {
		hivePrefix = "HKCU"
	}

	keyPath := hivePrefix + "\\" + wk.Path

	evt := event.NewEvent(event.EventTypeRegistry, event.SeverityMedium, map[string]interface{}{
		"action":        action,
		"key_path":      keyPath,
		"value_name":    valueName,
		"value_data":    newValue,
		"previous_data": oldValue,
		"watch_name":    wk.Name,
	})

	// Apply filter
	if c.filter != nil && c.filter.ShouldFilter(evt) {
		return
	}

	c.sendEvent(evt)
}

// sendEvent sends an event to the channel.
func (c *RegistryCollector) sendEvent(evt *event.Event) {
	select {
	case c.eventChan <- evt:
		c.eventsGenerated.Add(1)
	default:
		// Buffer full
	}
}

// Stats returns collector statistics.
func (c *RegistryCollector) Stats() RegistryStats {
	return RegistryStats{
		Running:         c.running.Load(),
		KeysMonitored:   c.keysMonitored,
		ChangesDetected: c.changesDetected.Load(),
		EventsGenerated: c.eventsGenerated.Load(),
	}
}

// RegistryStats holds registry collector statistics.
type RegistryStats struct {
	Running         bool
	KeysMonitored   int
	ChangesDetected uint64
	EventsGenerated uint64
}
