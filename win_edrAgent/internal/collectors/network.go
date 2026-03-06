// Package collectors provides network connection monitoring.
//go:build windows
// +build windows

package collectors

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// NetworkCollector monitors network connections.
type NetworkCollector struct {
	logger    *logging.Logger
	eventChan chan<- *event.Event
	filter    *Filter

	// State
	running atomic.Bool

	// Connection cache to detect new connections
	connCache map[string]bool
	cacheMu   sync.RWMutex

	// Metrics
	connectionsFound atomic.Uint64
	eventsGenerated  atomic.Uint64
}

// NewNetworkCollector creates a new network collector.
func NewNetworkCollector(eventChan chan<- *event.Event, filter *Filter, logger *logging.Logger) *NetworkCollector {
	return &NetworkCollector{
		logger:    logger,
		eventChan: eventChan,
		filter:    filter,
		connCache: make(map[string]bool),
	}
}

// Start begins network monitoring.
func (c *NetworkCollector) Start(ctx context.Context) error {
	if c.running.Load() {
		return nil
	}

	c.logger.Info("Starting network collector...")
	c.running.Store(true)

	go c.monitorLoop(ctx)

	c.logger.Info("Network collector started")
	return nil
}

// Stop stops network monitoring.
func (c *NetworkCollector) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.logger.Info("Stopping network collector...")
	c.running.Store(false)

	c.logger.Infof("Network stats: found=%d generated=%d",
		c.connectionsFound.Load(),
		c.eventsGenerated.Load())

	return nil
}

// monitorLoop polls for network connections.
func (c *NetworkCollector) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.running.Load() {
				return
			}
			c.collectConnections()
		}
	}
}

// collectConnections gets current network connections.
func (c *NetworkCollector) collectConnections() {
	// Use netstat via PowerShell
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-NetTCPConnection -State Established,Listen | Select-Object LocalAddress,LocalPort,RemoteAddress,RemotePort,OwningProcess,State | ConvertTo-Csv -NoTypeInformation`)

	output, err := cmd.Output()
	if err != nil {
		c.logger.Debugf("Failed to get connections: %v", err)
		return
	}

	lines := strings.Split(string(output), "\n")
	newCache := make(map[string]bool)

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // Skip header
		}

		parts := parseCSVLine(line)
		if len(parts) < 6 {
			continue
		}

		localAddr := strings.Trim(parts[0], `"`)
		localPort := strings.Trim(parts[1], `"`)
		remoteAddr := strings.Trim(parts[2], `"`)
		remotePort := strings.Trim(parts[3], `"`)
		pid := strings.Trim(parts[4], `"`)
		state := strings.Trim(parts[5], `"`)

		// Create connection key
		connKey := localAddr + ":" + localPort + "->" + remoteAddr + ":" + remotePort

		// Check if new connection
		c.cacheMu.RLock()
		_, exists := c.connCache[connKey]
		c.cacheMu.RUnlock()

		if !exists && remoteAddr != "" && remoteAddr != "0.0.0.0" && remoteAddr != "::" {
			c.connectionsFound.Add(1)

			localPortInt, _ := strconv.Atoi(localPort)
			remotePortInt, _ := strconv.Atoi(remotePort)
			pidInt, _ := strconv.ParseInt(pid, 10, 64)

			evt := event.NewEvent(event.EventTypeNetwork, event.SeverityLow, map[string]interface{}{
				"action":           "connection_established",
				"direction":        "outbound",
				"protocol":         "tcp",
				"source_ip":        localAddr,
				"source_port":      localPortInt,
				"destination_ip":   remoteAddr,
				"destination_port": remotePortInt,
				"pid":              pidInt,
				"state":            state,
			})

			// Apply filter
			if c.filter == nil || !c.filter.ShouldFilter(evt) {
				c.sendEvent(evt)
			}
		}

		newCache[connKey] = true
	}

	// Update cache
	c.cacheMu.Lock()
	c.connCache = newCache
	c.cacheMu.Unlock()
}

// parseCSVLine splits a CSV line into fields.
func parseCSVLine(line string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false

	for _, char := range line {
		switch char {
		case '"':
			inQuotes = !inQuotes
			current.WriteRune(char)
		case ',':
			if inQuotes {
				current.WriteRune(char)
			} else {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// sendEvent sends an event to the channel.
func (c *NetworkCollector) sendEvent(evt *event.Event) {
	select {
	case c.eventChan <- evt:
		c.eventsGenerated.Add(1)
	default:
		// Buffer full
	}
}

// Stats returns collector statistics.
func (c *NetworkCollector) Stats() NetworkStats {
	return NetworkStats{
		Running:          c.running.Load(),
		ConnectionsFound: c.connectionsFound.Load(),
		EventsGenerated:  c.eventsGenerated.Load(),
	}
}

// NetworkStats holds network collector statistics.
type NetworkStats struct {
	Running          bool
	ConnectionsFound uint64
	EventsGenerated  uint64
}
