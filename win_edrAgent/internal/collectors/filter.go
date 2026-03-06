// Package collectors provides event filtering to reduce noise.
package collectors

import (
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// Filter applies exclusion rules to events.
// Thread safety: all shared state is protected by sync.RWMutex.
// Metrics counters use sync/atomic for zero-lock reads from the heartbeat goroutine.
type Filter struct {
	logger *logging.Logger
	mu     sync.RWMutex

	// Exclusion lists
	excludeProcesses map[string]bool
	excludeIPs       []*net.IPNet
	excludeRegistry  []string
	excludePaths     []*regexp.Regexp

	// Advanced exclusion — O(1) map lookups
	excludeEventIDs map[int]bool    // Sysmon Event IDs to drop
	trustedHashes   map[string]bool // SHA256 hashes of known-good binaries

	// Include lists (override exclude)
	includePaths []*regexp.Regexp

	// Metrics — atomic for lock-free reads from heartbeat goroutine
	totalEvents    atomic.Uint64
	filteredEvents atomic.Uint64
}

// FilterConfig holds filter configuration.
type FilterConfig struct {
	ExcludeProcesses []string
	ExcludeIPs       []string
	ExcludeRegistry  []string
	ExcludePaths     []string
	IncludePaths     []string
	ExcludeEventIDs  []int
	TrustedHashes    []string
}

// NewFilter creates a new event filter.
func NewFilter(cfg FilterConfig, logger *logging.Logger) *Filter {
	f := &Filter{
		logger:           logger,
		excludeProcesses: make(map[string]bool),
		excludeEventIDs:  make(map[int]bool),
		trustedHashes:    make(map[string]bool),
	}

	// Parse exclude processes
	for _, p := range cfg.ExcludeProcesses {
		f.excludeProcesses[strings.ToLower(p)] = true
	}

	// Parse exclude IPs
	for _, ip := range cfg.ExcludeIPs {
		if strings.Contains(ip, "/") {
			_, network, err := net.ParseCIDR(ip)
			if err == nil {
				f.excludeIPs = append(f.excludeIPs, network)
			}
		} else {
			// Single IP - convert to /32
			parsed := net.ParseIP(ip)
			if parsed != nil {
				var mask net.IPMask
				if parsed.To4() != nil {
					mask = net.CIDRMask(32, 32)
				} else {
					mask = net.CIDRMask(128, 128)
				}
				f.excludeIPs = append(f.excludeIPs, &net.IPNet{IP: parsed, Mask: mask})
			}
		}
	}

	// Parse registry patterns
	f.excludeRegistry = cfg.ExcludeRegistry

	// Parse path patterns
	for _, p := range cfg.ExcludePaths {
		pattern := pathToRegex(p)
		if re, err := regexp.Compile(pattern); err == nil {
			f.excludePaths = append(f.excludePaths, re)
		}
	}

	for _, p := range cfg.IncludePaths {
		pattern := pathToRegex(p)
		if re, err := regexp.Compile(pattern); err == nil {
			f.includePaths = append(f.includePaths, re)
		}
	}

	// Parse excluded Sysmon Event IDs — O(1) map lookup
	for _, id := range cfg.ExcludeEventIDs {
		f.excludeEventIDs[id] = true
	}

	// Parse trusted hashes — normalized to lowercase for case-insensitive matching
	for _, h := range cfg.TrustedHashes {
		f.trustedHashes[strings.ToLower(h)] = true
	}

	if logger != nil {
		logger.Infof("Filter initialized: %d processes, %d IPs, %d registry, %d paths, %d event_ids, %d trusted_hashes excluded",
			len(f.excludeProcesses), len(f.excludeIPs), len(f.excludeRegistry),
			len(f.excludePaths), len(f.excludeEventIDs), len(f.trustedHashes))
	}

	return f
}

// pathToRegex converts a glob pattern to a regex with directory-prefix matching.
// The generated regex matches the exact path or any path under it (subpaths/files).
// Requiring a separator (\ or /) after the pattern prevents false matches on same-prefix paths (e.g. Temp vs Temporary).
func pathToRegex(pattern string) string {
	// Escape regex special characters except * and ?
	pattern = regexp.QuoteMeta(pattern)
	// Convert glob wildcards to regex
	pattern = strings.ReplaceAll(pattern, `\*`, `.*`)
	pattern = strings.ReplaceAll(pattern, `\?`, `.`)
	// Match exact path or path + separator + anything (directory-prefix / subpath matching)
	return "(?i)^" + pattern + `($|[\\/].*)`
}

// ShouldFilter returns true if the event should be filtered out.
// Check order is optimized: cheapest checks first (O(1) map lookups for
// event ID and hash), then type-specific filtering.
func (f *Filter) ShouldFilter(evt *event.Event) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.totalEvents.Add(1)

	// Fast path: Event ID exclusion — O(1) map lookup, checked before anything else
	if f.filterByEventID(evt) {
		f.filteredEvents.Add(1)
		return true
	}

	// Fast path: Trusted hash exclusion — O(1) map lookup
	if f.filterByTrustedHash(evt) {
		f.filteredEvents.Add(1)
		return true
	}

	// Type-specific filtering
	switch evt.Type {
	case event.EventTypeProcess:
		if f.filterProcess(evt) {
			f.filteredEvents.Add(1)
			return true
		}

	case event.EventTypeNetwork:
		if f.filterNetwork(evt) {
			f.filteredEvents.Add(1)
			return true
		}

	case event.EventTypeRegistry:
		if f.filterRegistry(evt) {
			f.filteredEvents.Add(1)
			return true
		}

	case event.EventTypeFile:
		if f.filterFile(evt) {
			f.filteredEvents.Add(1)
			return true
		}
	}

	return false
}

// filterProcess checks if a process event should be filtered.
func (f *Filter) filterProcess(evt *event.Event) bool {
	name, ok := evt.Data["name"].(string)
	if !ok {
		return false
	}

	// Check exclusion list
	if f.excludeProcesses[strings.ToLower(name)] {
		return true
	}

	// Check executable path
	if exe, ok := evt.Data["executable"].(string); ok {
		if f.matchesExcludePath(exe) && !f.matchesIncludePath(exe) {
			return true
		}
	}

	return false
}

// filterNetwork checks if a network event should be filtered.
func (f *Filter) filterNetwork(evt *event.Event) bool {
	// Check source IP
	if srcIP, ok := evt.Data["source_ip"].(string); ok {
		if f.isExcludedIP(srcIP) {
			return true
		}
	}

	// Check destination IP
	if dstIP, ok := evt.Data["destination_ip"].(string); ok {
		if f.isExcludedIP(dstIP) {
			return true
		}
	}

	// Filter localhost connections
	if srcIP, _ := evt.Data["source_ip"].(string); srcIP == "127.0.0.1" || srcIP == "::1" {
		return true
	}

	return false
}

// filterRegistry checks if a registry event should be filtered.
func (f *Filter) filterRegistry(evt *event.Event) bool {
	keyPath, ok := evt.Data["key_path"].(string)
	if !ok {
		return false
	}

	keyPath = strings.ToLower(keyPath)
	for _, pattern := range f.excludeRegistry {
		if strings.Contains(keyPath, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// filterFile checks if a file event should be filtered.
func (f *Filter) filterFile(evt *event.Event) bool {
	path, ok := evt.Data["path"].(string)
	if !ok {
		return false
	}

	// Check if in include paths (override exclude)
	if f.matchesIncludePath(path) {
		return false
	}

	// Check if in exclude paths
	if f.matchesExcludePath(path) {
		return true
	}

	return false
}

// isExcludedIP checks if an IP is in the exclusion list.
func (f *Filter) isExcludedIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, network := range f.excludeIPs {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// matchesExcludePath checks if a path matches any exclude pattern.
func (f *Filter) matchesExcludePath(path string) bool {
	// Normalize path
	path = filepath.Clean(path)

	for _, re := range f.excludePaths {
		if re.MatchString(path) {
			return true
		}
	}

	return false
}

// matchesIncludePath checks if a path matches any include pattern.
func (f *Filter) matchesIncludePath(path string) bool {
	path = filepath.Clean(path)

	for _, re := range f.includePaths {
		if re.MatchString(path) {
			return true
		}
	}

	return false
}

// filterByEventID checks if the event's Sysmon Event ID is in the exclusion set.
// O(1) map lookup — the cheapest filter check, run first.
func (f *Filter) filterByEventID(evt *event.Event) bool {
	if len(f.excludeEventIDs) == 0 {
		return false
	}

	// Check "event_id" field in event data (integer or string)
	if rawID, ok := evt.Data["event_id"]; ok {
		switch v := rawID.(type) {
		case int:
			return f.excludeEventIDs[v]
		case int64:
			return f.excludeEventIDs[int(v)]
		case float64:
			return f.excludeEventIDs[int(v)]
		case string:
			if id, err := strconv.Atoi(v); err == nil {
				return f.excludeEventIDs[id]
			}
		}
	}

	return false
}

// filterByTrustedHash checks if the event contains a SHA256 hash that is trusted.
// O(1) map lookup — checked before expensive type-specific filtering.
func (f *Filter) filterByTrustedHash(evt *event.Event) bool {
	if len(f.trustedHashes) == 0 {
		return false
	}

	// Check "hash_sha256" field — present in ProcessEvent, FileEvent, DriverEvent, ImageLoadEvent
	if hash, ok := evt.Data["hash_sha256"].(string); ok && hash != "" {
		return f.trustedHashes[strings.ToLower(hash)]
	}

	return false
}

// UpdateExclusions updates the exclusion lists at runtime.
// Thread-safe: acquires write lock. Can be called from the config update goroutine.
func (f *Filter) UpdateExclusions(cfg FilterConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Re-initialize with new config
	f.excludeProcesses = make(map[string]bool)
	for _, p := range cfg.ExcludeProcesses {
		f.excludeProcesses[strings.ToLower(p)] = true
	}

	f.excludeRegistry = cfg.ExcludeRegistry

	// Update Event ID exclusions
	f.excludeEventIDs = make(map[int]bool)
	for _, id := range cfg.ExcludeEventIDs {
		f.excludeEventIDs[id] = true
	}

	// Update trusted hashes
	f.trustedHashes = make(map[string]bool)
	for _, h := range cfg.TrustedHashes {
		f.trustedHashes[strings.ToLower(h)] = true
	}

	if f.logger != nil {
		f.logger.Infof("Filter exclusions updated: %d processes, %d event_ids, %d trusted_hashes",
			len(f.excludeProcesses), len(f.excludeEventIDs), len(f.trustedHashes))
	}
}

// Stats returns filter statistics.
// Uses atomic loads — safe to call from any goroutine without locking.
func (f *Filter) Stats() FilterStats {
	total := f.totalEvents.Load()
	filtered := f.filteredEvents.Load()

	ratio := float64(0)
	if total > 0 {
		ratio = float64(filtered) / float64(total) * 100
	}

	return FilterStats{
		TotalEvents:    total,
		FilteredEvents: filtered,
		PassedEvents:   total - filtered,
		FilterRatio:    ratio,
	}
}

// DroppedCount returns the total number of events filtered out.
// This is used by the heartbeat to report dropped_events_count to the server.
// Uses atomic load — safe to call from any goroutine without locking.
func (f *Filter) DroppedCount() uint64 {
	return f.filteredEvents.Load()
}

// FilterStats holds filter statistics.
type FilterStats struct {
	TotalEvents    uint64
	FilteredEvents uint64
	PassedEvents   uint64
	FilterRatio    float64 // Percentage filtered
}
