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

// rfc1918 defines the private IPv4 address ranges (RFC 1918).
// Used to filter internal LAN connections that generate high volume
// with near-zero security signal for EDR.
var rfc1918 = []struct{ prefix string; bits int }{
	{"10.", 8},
	{"172.16.", 12}, {"172.17.", 12}, {"172.18.", 12}, {"172.19.", 12},
	{"172.20.", 12}, {"172.21.", 12}, {"172.22.", 12}, {"172.23.", 12},
	{"172.24.", 12}, {"172.25.", 12}, {"172.26.", 12}, {"172.27.", 12},
	{"172.28.", 12}, {"172.29.", 12}, {"172.30.", 12}, {"172.31.", 12},
	{"192.168.", 16},
}

// NetworkCollector monitors network connections.
type NetworkCollector struct {
	logger    *logging.Logger
	eventChan chan<- *event.Event
	filter    *Filter

	// filterPrivateNetworks, when true, drops connections where BOTH
	// endpoints are RFC 1918 private IPs (e.g. 192.168.x ↔ 192.168.x).
	// Controlled by config.Filtering.FilterPrivateNetworks.
	filterPrivateNetworks bool

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
func NewNetworkCollector(eventChan chan<- *event.Event, filter *Filter, filterPrivateNetworks bool, logger *logging.Logger) *NetworkCollector {
	return &NetworkCollector{
		logger:                logger,
		eventChan:             eventChan,
		filter:                filter,
		filterPrivateNetworks: filterPrivateNetworks,
		connCache:             make(map[string]bool),
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
	ticker := time.NewTicker(30 * time.Second)
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
			// RFC 1918 filtering: skip private-to-private (LAN) connections.
			// These generate enormous volume with near-zero security signal.
			if c.filterPrivateNetworks && isPrivateIP(localAddr) && isPrivateIP(remoteAddr) {
				newCache[connKey] = true
				continue
			}

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

// isPrivateIP returns true if the IP address is in an RFC 1918 private range.
// Uses fast string-prefix matching (no net.ParseIP) for zero-allocation performance.
func isPrivateIP(ip string) bool {
	for _, r := range rfc1918 {
		if strings.HasPrefix(ip, r.prefix) {
			return true
		}
	}
	return false
}
