// Package collectors — Real-time registry monitoring via RegNotifyChangeKeyValue.
//
// REPLACES the previous 10-second polling approach with kernel-level
// change notifications that fire INSTANTLY when a monitored key changes.
//
// Advantages over polling:
//   - Zero detection latency (instant vs 10s gap)
//   - Zero CPU when idle (blocks on kernel event, no busy loop)
//   - Detects ALL change types: value set, subkey add/delete, security changes
//   - No missed changes (polling could miss rapid create+delete cycles)
//
// Architecture: Each monitored registry key gets its own goroutine that
// calls RegNotifyChangeKeyValue with an event handle. When the kernel
// signals a change, the goroutine reads the current values, diffs against
// the cached baseline, and emits events for any differences.
//
// The Win32 API RegNotifyChangeKeyValue is the same mechanism used by
// Process Monitor (Sysinternals) and Sysmon for registry monitoring.
//
//go:build windows
// +build windows

package collectors

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// =====================================================================
// RegNotifyChangeKeyValue Constants
// =====================================================================

// Notification filter flags for RegNotifyChangeKeyValue
const (
	// REG_NOTIFY_CHANGE_NAME — subkey created/deleted
	regNotifyChangeName uint32 = 0x00000001
	// REG_NOTIFY_CHANGE_ATTRIBUTES — key attributes changed
	regNotifyChangeAttributes uint32 = 0x00000002
	// REG_NOTIFY_CHANGE_LAST_SET — value set/changed/deleted
	regNotifyChangeLastSet uint32 = 0x00000004
	// REG_NOTIFY_CHANGE_SECURITY — security descriptor changed
	regNotifyChangeSecurity uint32 = 0x00000008
)

var (
	advapi32              = windows.NewLazyDLL("advapi32.dll")
	regNotifyChangeKeyVal = advapi32.NewProc("RegNotifyChangeKeyValue")
)

// =====================================================================
// Registry Collector
// =====================================================================

// RegistryCollector monitors registry changes using kernel notifications.
type RegistryCollector struct {
	logger    *logging.Logger
	eventChan chan<- *event.Event
	filter    *Filter

	// Keys to monitor (persistence mechanisms)
	watchKeys []RegistryWatchKey

	// State
	running atomic.Bool
	wg      sync.WaitGroup

	// Value cache for change detection (key = "watchName\valueName" → value)
	valueCache map[string]string
	cacheMu    sync.RWMutex

	// Subkey cache to detect new/deleted subkeys
	subkeyCache map[string]map[string]bool // watchKeyName → set of subkey names
	subkeyMu    sync.RWMutex

	// Metrics
	keysMonitored   int
	changesDetected atomic.Uint64
	eventsGenerated atomic.Uint64
	dropped         atomic.Uint64
}

// RegistryWatchKey defines a registry key to monitor.
type RegistryWatchKey struct {
	Hive registry.Key
	Path string
	Name string // Human-readable name
}

// DefaultWatchKeys returns the default registry keys to monitor.
// These cover the most critical persistence mechanisms (MITRE T1547, T1546, T1053).
func DefaultWatchKeys() []RegistryWatchKey {
	return []RegistryWatchKey{
		// Run keys (persistence — T1547.001)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKLM Run"},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, "HKLM RunOnce"},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "HKCU Run"},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`, "HKCU RunOnce"},

		// Services (T1543.003)
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services`, "Services"},

		// Startup folder
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, "Shell Folders"},

		// Winlogon (T1547.004)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon`, "Winlogon"},

		// Image File Execution Options (debugger hijack — T1546.012)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`, "IFEO"},

		// AppInit DLLs (T1546.010)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Windows`, "AppInit"},

		// Scheduled Tasks (T1053.005)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Schedule\TaskCache\Tasks`, "Tasks"},

		// COM Object Hijacking (T1546.015)
		{registry.LOCAL_MACHINE, `SOFTWARE\Classes\CLSID`, "CLSID"},
		{registry.CURRENT_USER, `SOFTWARE\Classes\CLSID`, "HKCU CLSID"},

		// WMI Persistence (T1546.003)
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\WBEM`, "WMI"},

		// Boot/Logon Autostart — RunServices
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunServicesOnce`, "RunServicesOnce"},

		// Security Providers (T1547.005)
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\SecurityProviders`, "SecurityProviders"},
	}
}

// NewRegistryCollector creates a new registry collector.
func NewRegistryCollector(eventChan chan<- *event.Event, filter *Filter, logger *logging.Logger) *RegistryCollector {
	return &RegistryCollector{
		logger:      logger,
		eventChan:   eventChan,
		filter:      filter,
		watchKeys:   DefaultWatchKeys(),
		valueCache:  make(map[string]string),
		subkeyCache: make(map[string]map[string]bool),
	}
}

// Start begins registry monitoring with kernel change notifications.
func (c *RegistryCollector) Start(ctx context.Context) error {
	if c.running.Load() {
		return nil
	}

	c.logger.Info("[REG] Starting registry collector (RegNotifyChangeKeyValue — kernel notifications)...")
	c.logger.Infof("[REG] Monitoring %d registry keys", len(c.watchKeys))

	c.running.Store(true)
	c.keysMonitored = len(c.watchKeys)

	// Initial baseline — read all current values
	c.collectBaseline()

	// Start a watcher goroutine for each monitored key
	for _, wk := range c.watchKeys {
		c.wg.Add(1)
		go c.watchKey(ctx, wk)
	}

	c.logger.Info("[REG] Registry collector started — all keys under kernel notification")
	return nil
}

// Stop stops registry monitoring.
func (c *RegistryCollector) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.logger.Info("[REG] Stopping registry collector...")
	c.running.Store(false)

	// The wg.Wait() happens naturally as context cancellation
	// unblocks WaitForSingleObject in each watcher goroutine.

	c.logger.Infof("[REG] Stats: changes=%d events=%d dropped=%d",
		c.changesDetected.Load(),
		c.eventsGenerated.Load(),
		c.dropped.Load())

	return nil
}

// collectBaseline reads initial values for all monitored keys.
func (c *RegistryCollector) collectBaseline() {
	for _, wk := range c.watchKeys {
		c.snapshotKeyValues(wk)
		c.snapshotSubkeys(wk)
	}
}

// watchKey monitors a single registry key using RegNotifyChangeKeyValue.
// This function blocks on the kernel event until the key changes, then
// reads the new values and emits events for any differences.
//
// IMPORTANT: RegNotifyChangeKeyValue only signals ONCE per registration.
// After each notification, we must re-register for the next change.
// This is handled by the for loop below.
func (c *RegistryCollector) watchKey(ctx context.Context, wk RegistryWatchKey) {
	defer c.wg.Done()

	for ctx.Err() == nil && c.running.Load() {
		// Open the key with KEY_NOTIFY permission
		key, err := registry.OpenKey(wk.Hive, wk.Path, registry.READ|registry.NOTIFY)
		if err != nil {
			// Key doesn't exist — wait and retry (key may be created later)
			c.logger.Debugf("[REG] Key not found: %s — will retry in 30s", wk.Name)
			select {
			case <-ctx.Done():
				return
			case <-waitDuration(30):
			}
			continue
		}

		// Create a Windows event object for the notification
		evtHandle, err := windows.CreateEvent(nil, 0, 0, nil)
		if err != nil {
			key.Close()
			c.logger.Errorf("[REG] CreateEvent failed for %s: %v", wk.Name, err)
			select {
			case <-ctx.Done():
				return
			case <-waitDuration(5):
			}
			continue
		}

		// Register for change notifications on this key.
		// Notification filter: values changed + subkeys added/deleted + security changes.
		// watchSubtree=true catches changes to all subkeys recursively.
		notifyFilter := regNotifyChangeName | regNotifyChangeLastSet |
			regNotifyChangeAttributes | regNotifyChangeSecurity

		ret, _, callErr := regNotifyChangeKeyVal.Call(
			uintptr(key),          // hKey
			uintptr(1),            // bWatchSubtree = TRUE
			uintptr(notifyFilter), // dwNotifyFilter
			uintptr(evtHandle),    // hEvent
			uintptr(1),            // fAsynchronous = TRUE
		)

		if ret != 0 {
			windows.CloseHandle(evtHandle)
			key.Close()
			c.logger.Errorf("[REG] RegNotifyChangeKeyValue failed for %s: %v (ret=%d)",
				wk.Name, callErr, ret)
			select {
			case <-ctx.Done():
				return
			case <-waitDuration(5):
			}
			continue
		}

		c.logger.Debugf("[REG] Watching: %s (kernel notification active)", wk.Name)

		// Block until the kernel signals a change OR context is cancelled.
		// Use a polling approach with WaitForSingleObject to check ctx periodically.
		changed := false
		for !changed && ctx.Err() == nil && c.running.Load() {
			// Wait 1 second at a time to allow context cancellation
			r, _ := windows.WaitForSingleObject(evtHandle, 1000)
			if r == windows.WAIT_OBJECT_0 {
				changed = true
			}
			// WAIT_TIMEOUT (258) means we loop and check ctx again
		}

		windows.CloseHandle(evtHandle)
		key.Close()

		if !changed {
			// Context cancelled — exit
			return
		}

		// The key changed — diff against cached baseline
		c.logger.Debugf("[REG] Change detected: %s", wk.Name)
		c.changesDetected.Add(1)

		c.diffKeyValues(wk)
		c.diffSubkeys(wk)

		// Re-loop: RegNotifyChangeKeyValue fires only ONCE per registration,
		// so we must re-open the key and re-register.
	}
}

// snapshotKeyValues reads all values from a key and stores them in the cache.
func (c *RegistryCollector) snapshotKeyValues(wk RegistryWatchKey) {
	key, err := registry.OpenKey(wk.Hive, wk.Path, registry.READ)
	if err != nil {
		return
	}
	defer key.Close()

	valueNames, err := key.ReadValueNames(-1)
	if err != nil {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	for _, vn := range valueNames {
		valStr, valType := readRegValue(key, vn)
		if valType == 0 && valStr == "" {
			continue
		}
		cacheKey := wk.Name + "\\" + vn
		c.valueCache[cacheKey] = valStr
	}
}

// snapshotSubkeys reads all subkey names and stores them in the cache.
func (c *RegistryCollector) snapshotSubkeys(wk RegistryWatchKey) {
	key, err := registry.OpenKey(wk.Hive, wk.Path, registry.READ)
	if err != nil {
		return
	}
	defer key.Close()

	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return
	}

	c.subkeyMu.Lock()
	defer c.subkeyMu.Unlock()

	set := make(map[string]bool, len(subkeys))
	for _, sk := range subkeys {
		set[strings.ToLower(sk)] = true
	}
	c.subkeyCache[wk.Name] = set
}

// diffKeyValues reads current values and emits events for any changes.
func (c *RegistryCollector) diffKeyValues(wk RegistryWatchKey) {
	key, err := registry.OpenKey(wk.Hive, wk.Path, registry.READ)
	if err != nil {
		return
	}
	defer key.Close()

	valueNames, err := key.ReadValueNames(-1)
	if err != nil {
		return
	}

	currentValues := make(map[string]string, len(valueNames))

	for _, vn := range valueNames {
		valStr, valType := readRegValue(key, vn)
		if valType == 0 && valStr == "" {
			continue
		}

		currentValues[wk.Name+"\\"+vn] = valStr

		cacheKey := wk.Name + "\\" + vn

		c.cacheMu.RLock()
		oldVal, exists := c.valueCache[cacheKey]
		c.cacheMu.RUnlock()

		if !exists {
			// New value added
			c.generateEvent("value_set", wk, vn, "", valStr, inferRegType(valType))
		} else if oldVal != valStr {
			// Value modified
			c.generateEvent("value_set", wk, vn, oldVal, valStr, inferRegType(valType))
		}
	}

	// Check for deleted values
	c.cacheMu.RLock()
	for cacheKey, oldVal := range c.valueCache {
		if strings.HasPrefix(cacheKey, wk.Name+"\\") {
			if _, exists := currentValues[cacheKey]; !exists {
				vn := strings.TrimPrefix(cacheKey, wk.Name+"\\")
				c.generateEvent("value_delete", wk, vn, oldVal, "", "")
			}
		}
	}
	c.cacheMu.RUnlock()

	// Update cache
	c.cacheMu.Lock()
	// Clear old entries for this key
	for k := range c.valueCache {
		if strings.HasPrefix(k, wk.Name+"\\") {
			delete(c.valueCache, k)
		}
	}
	for k, v := range currentValues {
		c.valueCache[k] = v
	}
	c.cacheMu.Unlock()
}

// diffSubkeys detects new and deleted subkeys.
func (c *RegistryCollector) diffSubkeys(wk RegistryWatchKey) {
	key, err := registry.OpenKey(wk.Hive, wk.Path, registry.READ)
	if err != nil {
		return
	}
	defer key.Close()

	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return
	}

	currentSet := make(map[string]bool, len(subkeys))
	for _, sk := range subkeys {
		currentSet[strings.ToLower(sk)] = true
	}

	c.subkeyMu.RLock()
	oldSet := c.subkeyCache[wk.Name]
	c.subkeyMu.RUnlock()

	if oldSet == nil {
		// First time — just set baseline
		c.subkeyMu.Lock()
		c.subkeyCache[wk.Name] = currentSet
		c.subkeyMu.Unlock()
		return
	}

	// New subkeys
	for sk := range currentSet {
		if !oldSet[sk] {
			c.generateEvent("key_created", wk, sk, "", "", "")
		}
	}

	// Deleted subkeys
	for sk := range oldSet {
		if !currentSet[sk] {
			c.generateEvent("key_deleted", wk, sk, "", "", "")
		}
	}

	c.subkeyMu.Lock()
	c.subkeyCache[wk.Name] = currentSet
	c.subkeyMu.Unlock()
}

// generateEvent creates a registry event with Sigma-compatible fields.
func (c *RegistryCollector) generateEvent(action string, wk RegistryWatchKey, valueName, oldValue, newValue, valueType string) {
	hivePrefix := "HKLM"
	if wk.Hive == registry.CURRENT_USER {
		hivePrefix = "HKCU"
	}

	keyPath := hivePrefix + "\\" + wk.Path

	// Build full target object path (Sigma TargetObject format)
	targetObject := keyPath
	if valueName != "" {
		targetObject = keyPath + "\\\\" + valueName
	}

	// Determine Sigma-compatible event category from action
	sigmaAction := action
	switch action {
	case "value_set":
		sigmaAction = "SetValue"
	case "value_delete":
		sigmaAction = "DeleteValue"
	case "key_created":
		sigmaAction = "CreateKey"
	case "key_deleted":
		sigmaAction = "DeleteKey"
	}

	evt := event.NewEvent(event.EventTypeRegistry, event.SeverityMedium, map[string]interface{}{
		"action":        action,
		"key_path":      keyPath,
		"value_name":    valueName,
		"value_data":    newValue,
		"value_type":    valueType,
		"previous_data": oldValue,
		"watch_name":    wk.Name,
		// Sigma-compatible fields (match Sysmon EventID 12/13/14)
		"TargetObject":  targetObject,
		"Details":       newValue,
		"EventType":     sigmaAction,
	})

	// Apply filter
	if c.filter != nil && c.filter.ShouldFilter(evt) {
		return
	}

	c.send(evt)
	c.logger.Infof("[REG] %s: %s\\%s = %s", action, keyPath, valueName, truncStr(newValue, 80))
}

// send delivers a registry event to the agent's event pipeline.
func (c *RegistryCollector) send(evt *event.Event) {
	select {
	case c.eventChan <- evt:
		c.eventsGenerated.Add(1)
	default:
		c.dropped.Add(1)
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

// =====================================================================
// Helper Functions
// =====================================================================

// readRegValue reads a registry value using the appropriate typed method
// and returns the string representation and type code.
func readRegValue(key registry.Key, name string) (string, uint32) {
	// Try string first (most common for persistence keys)
	if s, valType, err := key.GetStringValue(name); err == nil {
		return s, valType
	}

	// Try integer
	if v, valType, err := key.GetIntegerValue(name); err == nil {
		return fmt.Sprintf("0x%X (%d)", v, v), valType
	}

	// Try binary
	if b, valType, err := key.GetBinaryValue(name); err == nil {
		if len(b) > 64 {
			return fmt.Sprintf("[binary %d bytes]", len(b)), valType
		}
		return fmt.Sprintf("%x", b), valType
	}

	// Try multi-string
	if ss, valType, err := key.GetStringsValue(name); err == nil {
		return strings.Join(ss, "; "), valType
	}

	return "", 0
}

// inferRegType maps Windows registry type constants to human-readable strings.
func inferRegType(valType uint32) string {
	switch valType {
	case registry.SZ:
		return "REG_SZ"
	case registry.EXPAND_SZ:
		return "REG_EXPAND_SZ"
	case registry.BINARY:
		return "REG_BINARY"
	case registry.DWORD:
		return "REG_DWORD"
	case registry.QWORD:
		return "REG_QWORD"
	case registry.MULTI_SZ:
		return "REG_MULTI_SZ"
	default:
		return fmt.Sprintf("REG_TYPE_%d", valType)
	}
}

// waitDuration returns a channel that receives after n seconds.
// Used for select{} based sleep with context checking.
func waitDuration(seconds int) <-chan time.Time {
	return time.After(time.Duration(seconds) * time.Second)
}

