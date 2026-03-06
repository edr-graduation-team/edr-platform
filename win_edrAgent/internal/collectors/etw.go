// Package collectors provides event collection from Windows sources.
//go:build windows
// +build windows

package collectors

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// ETW Provider GUIDs for security-relevant events
var (
	// Microsoft-Windows-Kernel-Process
	KernelProcessGUID = windows.GUID{
		Data1: 0x22fb2cd6,
		Data2: 0x0fe7,
		Data3: 0x4212,
		Data4: [8]byte{0xa2, 0x96, 0x1f, 0x7f, 0x7d, 0x3b, 0x40, 0x0c},
	}

	// Microsoft-Windows-Kernel-Network
	KernelNetworkGUID = windows.GUID{
		Data1: 0x7dd42a49,
		Data2: 0x5329,
		Data3: 0x4832,
		Data4: [8]byte{0x8a, 0x15, 0xfb, 0x9b, 0x24, 0xe8, 0x4d, 0xd8},
	}

	// Microsoft-Windows-Kernel-Registry
	KernelRegistryGUID = windows.GUID{
		Data1: 0x70eb4f03,
		Data2: 0xc1de,
		Data3: 0x4f73,
		Data4: [8]byte{0xa0, 0x51, 0x33, 0xd1, 0x3d, 0x54, 0x13, 0xbd},
	}

	// Microsoft-Windows-Kernel-File
	KernelFileGUID = windows.GUID{
		Data1: 0xedd08927,
		Data2: 0x9cc4,
		Data3: 0x4e65,
		Data4: [8]byte{0xb9, 0x70, 0xc2, 0x56, 0x0f, 0xb5, 0xc2, 0x89},
	}
)

// ETWCollector collects events using Event Tracing for Windows.
type ETWCollector struct {
	logger      *logging.Logger
	eventChan   chan<- *event.Event
	sessionName string

	// State
	running atomic.Bool
	mu      sync.Mutex

	// Metrics
	eventsCollected atomic.Uint64
	eventsDropped   atomic.Uint64
	errors          atomic.Uint64
}

// NewETWCollector creates a new ETW collector.
func NewETWCollector(sessionName string, eventChan chan<- *event.Event, logger *logging.Logger) *ETWCollector {
	if sessionName == "" {
		sessionName = "EDRAgentSession"
	}

	return &ETWCollector{
		logger:      logger,
		eventChan:   eventChan,
		sessionName: sessionName,
	}
}

// Start begins ETW event collection.
func (c *ETWCollector) Start(ctx context.Context) error {
	if c.running.Load() {
		return fmt.Errorf("ETW collector already running")
	}

	c.logger.Info("Starting ETW collector...")
	c.logger.Infof("Session name: %s", c.sessionName)

	c.running.Store(true)

	// Start collection goroutine
	go c.collectLoop(ctx)

	c.logger.Info("ETW collector started")
	return nil
}

// Stop stops ETW event collection.
func (c *ETWCollector) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.logger.Info("Stopping ETW collector...")
	c.running.Store(false)

	c.logger.Infof("ETW stats: collected=%d dropped=%d errors=%d",
		c.eventsCollected.Load(),
		c.eventsDropped.Load(),
		c.errors.Load())

	return nil
}

// collectLoop is the main ETW event collection loop.
// It uses a process snapshot (Toolhelp32) for process events and emits simulated
// network events for pipeline validation until full ETW trace subscription is available.
func (c *ETWCollector) collectLoop(ctx context.Context) {
	c.logger.Debug("ETW collection loop started")

	// Process snapshot interval (avoids flooding; real ETW would be event-driven)
	processTicker := time.NewTicker(2 * time.Second)
	defer processTicker.Stop()

	// Simulated network event interval for pipeline validation
	simTicker := time.NewTicker(10 * time.Second)
	defer simTicker.Stop()

	tickCount := 0
	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("ETW collection loop stopped (context)")
			return
		case <-processTicker.C:
			if !c.running.Load() {
				c.logger.Debug("ETW collection loop stopped (flag)")
				return
			}
			c.collectProcessEvents()
		case <-simTicker.C:
			if !c.running.Load() {
				return
			}
			tickCount++
			c.emitSimulatedNetworkEvent(tickCount)
		}
	}
}

// collectProcessEvents collects process events using Windows API.
// Enriches each process with full executable path and command line
// so Sigma rules can match on CommandLine and Image fields.
func (c *ETWCollector) collectProcessEvents() {
	// Using CreateToolhelp32Snapshot for process enumeration
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		c.errors.Add(1)
		c.logger.Debugf("Failed to create process snapshot: %v", err)
		return
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(snapshot, &entry)
	if err != nil {
		c.errors.Add(1)
		return
	}

	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		pid := entry.ProcessID

		// Enrich with full executable path via OpenProcess + QueryFullProcessImageNameW
		executable := c.getProcessImagePath(pid)
		if executable == "" {
			// Fallback: construct path from name (common system processes)
			executable = name
		}

		// Construct a synthetic command line from the executable/name
		// Real ETW (process start) would have the actual command line;
		// for snapshot mode we use the executable as the command line.
		commandLine := executable

		evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
			"action":       "snapshot",
			"pid":          pid,
			"ppid":         entry.ParentProcessID,
			"name":         name,
			"executable":   executable,
			"command_line": commandLine,
			"threads":      entry.Threads,
		})

		c.sendEvent(evt)

		err = windows.Process32Next(snapshot, &entry)
		if err != nil {
			break
		}
	}
}

// getProcessImagePath retrieves the full executable path for a process via Windows API.
// Returns empty string if the process cannot be queried (e.g., access denied, system process).
func (c *ETWCollector) getProcessImagePath(pid uint32) string {
	if pid == 0 || pid == 4 {
		return "" // System Idle Process and System
	}

	// PROCESS_QUERY_LIMITED_INFORMATION = 0x1000 (works even for elevated processes)
	handle, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)

	// QueryFullProcessImageNameW
	var buf [windows.MAX_PATH]uint16
	bufSize := uint32(len(buf))
	err = windows.QueryFullProcessImageName(handle, 0, &buf[0], &bufSize)
	if err != nil {
		return ""
	}

	return windows.UTF16ToString(buf[:bufSize])
}

// emitSimulatedNetworkEvent pushes a synthetic network event for pipeline validation
// when full ETW kernel network tracing is not yet enabled.
func (c *ETWCollector) emitSimulatedNetworkEvent(seq int) {
	evt := event.NewEvent(event.EventTypeNetwork, event.SeverityLow, map[string]interface{}{
		"action":           "connection_established",
		"direction":        "outbound",
		"protocol":         "tcp",
		"source_ip":        "127.0.0.1",
		"source_port":      0,
		"destination_ip":   "0.0.0.0",
		"destination_port": 0,
		"pid":              int64(0),
		"process_name":     "",
		"simulated":        true,
		"seq":              seq,
	})
	c.sendEvent(evt)
}

// sendEvent sends an event to the channel.
func (c *ETWCollector) sendEvent(evt *event.Event) {
	select {
	case c.eventChan <- evt:
		c.eventsCollected.Add(1)
	default:
		c.eventsDropped.Add(1)
	}
}

// Stats returns collector statistics.
func (c *ETWCollector) Stats() ETWStats {
	return ETWStats{
		Running:         c.running.Load(),
		EventsCollected: c.eventsCollected.Load(),
		EventsDropped:   c.eventsDropped.Load(),
		Errors:          c.errors.Load(),
	}
}

// ETWStats holds ETW collector statistics.
type ETWStats struct {
	Running         bool
	EventsCollected uint64
	EventsDropped   uint64
	Errors          uint64
}

// IsRunning returns whether the collector is running.
func (c *ETWCollector) IsRunning() bool {
	return c.running.Load()
}
