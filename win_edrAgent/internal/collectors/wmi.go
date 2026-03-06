// Package collectors provides WMI-based event collection and system inventory.
//go:build windows
// +build windows

package collectors

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// WMICollector collects system inventory and events via WMI.
type WMICollector struct {
	logger    *logging.Logger
	eventChan chan<- *event.Event
	interval  time.Duration

	// State
	running atomic.Bool
	mu      sync.Mutex

	// Cache
	processCache map[uint32]string // PID -> process name
	cacheMu      sync.RWMutex

	// Metrics
	queriesRun  atomic.Uint64
	eventsFound atomic.Uint64
	errors      atomic.Uint64
}

// NewWMICollector creates a new WMI collector.
func NewWMICollector(interval time.Duration, eventChan chan<- *event.Event, logger *logging.Logger) *WMICollector {
	if interval <= 0 {
		interval = 60 * time.Minute
	}

	return &WMICollector{
		logger:       logger,
		eventChan:    eventChan,
		interval:     interval,
		processCache: make(map[uint32]string),
	}
}

// Start begins WMI event collection.
func (c *WMICollector) Start(ctx context.Context) error {
	if c.running.Load() {
		return fmt.Errorf("WMI collector already running")
	}

	c.logger.Info("Starting WMI collector...")
	c.logger.Infof("Inventory interval: %v", c.interval)

	c.running.Store(true)

	// Initial inventory
	go c.collectInventory()

	// Start periodic collection
	go c.collectLoop(ctx)

	c.logger.Info("WMI collector started")
	return nil
}

// Stop stops WMI collection.
func (c *WMICollector) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.logger.Info("Stopping WMI collector...")
	c.running.Store(false)

	c.logger.Infof("WMI stats: queries=%d events=%d errors=%d",
		c.queriesRun.Load(),
		c.eventsFound.Load(),
		c.errors.Load())

	return nil
}

// collectLoop runs periodic inventory collection.
func (c *WMICollector) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.running.Load() {
				return
			}
			c.collectInventory()
		}
	}
}

// collectInventory gathers system information via WMI.
func (c *WMICollector) collectInventory() {
	c.logger.Debug("Collecting system inventory...")

	// Collect running processes
	c.collectProcesses()

	// Collect system info
	c.collectSystemInfo()

	// Collect network adapters
	c.collectNetworkAdapters()

	c.logger.Debug("Inventory collection complete")
}

// collectProcesses gets running processes via PowerShell/WMI.
func (c *WMICollector) collectProcesses() {
	c.queriesRun.Add(1)

	// Using PowerShell for WMI query (Go WMI packages require CGO)
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine,CreationDate | ConvertTo-Json -Compress`)

	output, err := cmd.Output()
	if err != nil {
		c.errors.Add(1)
		c.logger.Debugf("Failed to query processes: %v", err)
		return
	}

	// Parse JSON output (simplified - in production use proper JSON parsing)
	processes := c.parseProcessOutput(string(output))

	c.cacheMu.Lock()
	newCache := make(map[uint32]string)
	c.cacheMu.Unlock()

	for _, proc := range processes {
		pid, _ := strconv.ParseUint(proc["ProcessId"], 10, 32)
		ppid, _ := strconv.ParseUint(proc["ParentProcessId"], 10, 32)
		name := proc["Name"]

		// Check if this is a new process
		c.cacheMu.RLock()
		_, exists := c.processCache[uint32(pid)]
		c.cacheMu.RUnlock()

		if !exists && pid > 4 { // Skip System and Idle
			evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
				"action":       "created",
				"pid":          pid,
				"ppid":         ppid,
				"name":         name,
				"executable":   proc["ExecutablePath"],
				"command_line": proc["CommandLine"],
			})
			c.sendEvent(evt)
		}

		newCache[uint32(pid)] = name
	}

	// Update cache
	c.cacheMu.Lock()
	c.processCache = newCache
	c.cacheMu.Unlock()
}

// parseProcessOutput parses PowerShell JSON output.
func (c *WMICollector) parseProcessOutput(output string) []map[string]string {
	// Simplified parsing - in production use encoding/json
	var results []map[string]string

	output = strings.TrimSpace(output)
	if output == "" || output == "null" {
		return results
	}

	// Very basic parsing for demo
	// In production, unmarshal JSON properly
	lines := strings.Split(output, "},{")
	for _, line := range lines {
		proc := make(map[string]string)
		line = strings.Trim(line, "[]{}")

		parts := strings.Split(line, ",")
		for _, part := range parts {
			kv := strings.SplitN(part, ":", 2)
			if len(kv) == 2 {
				key := strings.Trim(kv[0], `" `)
				value := strings.Trim(kv[1], `" `)
				proc[key] = value
			}
		}

		if proc["ProcessId"] != "" {
			results = append(results, proc)
		}
	}

	return results
}

// collectSystemInfo gathers system information.
func (c *WMICollector) collectSystemInfo() {
	c.queriesRun.Add(1)

	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_ComputerSystem | Select-Object Name,Domain,Manufacturer,Model,TotalPhysicalMemory | ConvertTo-Json -Compress`)

	output, err := cmd.Output()
	if err != nil {
		c.errors.Add(1)
		c.logger.Debugf("Failed to query system info: %v", err)
		return
	}

	c.logger.Debugf("System info collected: %d bytes", len(output))
}

// collectNetworkAdapters gathers network adapter information.
func (c *WMICollector) collectNetworkAdapters() {
	c.queriesRun.Add(1)

	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-NetAdapter | Where-Object {$_.Status -eq 'Up'} | Select-Object Name,InterfaceDescription,MacAddress,LinkSpeed | ConvertTo-Json -Compress`)

	output, err := cmd.Output()
	if err != nil {
		c.errors.Add(1)
		c.logger.Debugf("Failed to query network adapters: %v", err)
		return
	}

	c.logger.Debugf("Network adapters collected: %d bytes", len(output))
}

// sendEvent sends an event to the channel.
func (c *WMICollector) sendEvent(evt *event.Event) {
	select {
	case c.eventChan <- evt:
		c.eventsFound.Add(1)
	default:
		c.errors.Add(1)
	}
}

// Stats returns collector statistics.
func (c *WMICollector) Stats() WMIStats {
	return WMIStats{
		Running:     c.running.Load(),
		QueriesRun:  c.queriesRun.Load(),
		EventsFound: c.eventsFound.Load(),
		Errors:      c.errors.Load(),
	}
}

// WMIStats holds WMI collector statistics.
type WMIStats struct {
	Running     bool
	QueriesRun  uint64
	EventsFound uint64
	Errors      uint64
}

// IsRunning returns whether the collector is running.
func (c *WMICollector) IsRunning() bool {
	return c.running.Load()
}

// ForceInventory triggers an immediate inventory collection.
func (c *WMICollector) ForceInventory() {
	go c.collectInventory()
}
