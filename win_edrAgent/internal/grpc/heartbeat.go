// Package grpcclient provides heartbeat mechanism for agent health reporting.
package grpcclient

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/logging"
)

// HeartbeatStatus represents the current agent status.
type HeartbeatStatus string

const (
	StatusHealthy  HeartbeatStatus = "healthy"
	StatusDegraded HeartbeatStatus = "degraded"
	StatusCritical HeartbeatStatus = "critical"
	StatusUpdating HeartbeatStatus = "updating"
	StatusIsolated HeartbeatStatus = "isolated"
)

// Heartbeat manages periodic health reporting.
// The status field is protected by statusMu because SetStatus() can be
// called from any goroutine (e.g., isolation trigger, update trigger),
// while buildRequest() reads status from the heartbeat loop goroutine.
// Without the mutex, this is a data race on string assignment.
type Heartbeat struct {
	logger   *logging.Logger
	cfg      *config.Config
	interval time.Duration

	// State — guarded by statusMu for thread safety
	running  atomic.Bool
	status   HeartbeatStatus
	statusMu sync.RWMutex // protects status field

	// Metrics collectors
	getEventsGenerated func() uint64
	getEventsSent      func() uint64
	getQueueDepth      func() int
	getEventsDropped   func() uint64 // filter + rate limiter drops

	// Config update callback — called when the server pushes a new config.
	// The callback receives the raw JSON bytes and is responsible for
	// parsing, validating, saving to Registry, and applying the new config.
	onConfigUpdate func(newConfig []byte)

	// Heartbeat counters
	heartbeatsSent atomic.Uint64
	lastHeartbeat  time.Time

	// Device context — refreshed every 5 min by background goroutine
	deviceProfile string
	loggedInUser  string
	deviceInfoMu  sync.RWMutex
}

// HeartbeatRequest represents data sent in heartbeat.
type HeartbeatRequest struct {
	AgentID         string          `json:"agent_id"`
	Timestamp       time.Time       `json:"timestamp"`
	Status          HeartbeatStatus `json:"status"`
	Version         string          `json:"version"`
	Hostname        string          `json:"hostname"`
	OsVersion       string          `json:"os_version,omitempty"`
	CPUUsage        float64         `json:"cpu_usage"`
	MemoryUsedMB    uint64          `json:"memory_used_mb"`
	MemoryTotalMB   uint64          `json:"memory_total_mb"`
	EventsGenerated uint64          `json:"events_generated"`
	EventsSent      uint64          `json:"events_sent"`
	QueueDepth      int             `json:"queue_depth"`
	CertExpiresAt   int64           `json:"cert_expires_at,omitempty"`
	CPUCount        int             `json:"cpu_count"`

	// Telemetry enrichment — dropped events visibility for SOC
	EventsDropped uint64   `json:"events_dropped"`
	IPAddresses   []string `json:"ip_addresses,omitempty"`

	// Sysmon telemetry
	SysmonInstalled bool `json:"sysmon_installed"`
	SysmonRunning   bool `json:"sysmon_running"`

	// Device context — detected via WMI and sent as gRPC metadata
	Profile      string `json:"profile,omitempty"`
	LoggedInUser string `json:"logged_in_user,omitempty"`
}

// HeartbeatResponse represents server response.
type HeartbeatResponse struct {
	AckTimestamp          time.Time `json:"ack_timestamp"`
	ServerStatus          string    `json:"server_status"`
	HasPendingCommands    bool      `json:"has_pending_commands"`
	CertRenewalRequired   bool      `json:"cert_renewal_required"`
	ConfigUpdateAvailable bool      `json:"config_update_available"`
	RecommendedBatchSize  int       `json:"recommended_batch_size,omitempty"`
	RecommendedIntervalMs int       `json:"recommended_interval_ms,omitempty"`

	// NewConfig holds the updated configuration bytes (JSON-encoded)
	// sent by the server when ConfigUpdateAvailable is true.
	NewConfig []byte `json:"new_config,omitempty"`
}

// NewHeartbeat creates a new heartbeat manager.
func NewHeartbeat(cfg *config.Config, logger *logging.Logger) *Heartbeat {
	interval := cfg.Server.HeartbeatInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	return &Heartbeat{
		logger:   logger,
		cfg:      cfg,
		interval: interval,
		status:   StatusHealthy,
	}
}

// SetOnConfigUpdate registers a callback that is invoked when the server
// pushes a new configuration via the heartbeat response.
// The callback receives raw JSON bytes of the new config.
func (h *Heartbeat) SetOnConfigUpdate(fn func(newConfig []byte)) {
	h.onConfigUpdate = fn
}

// SetMetricsCollectors sets the functions used to collect metrics.
func (h *Heartbeat) SetMetricsCollectors(
	eventsGenerated func() uint64,
	eventsSent func() uint64,
	queueDepth func() int,
	eventsDropped func() uint64,
) {
	h.getEventsGenerated = eventsGenerated
	h.getEventsSent = eventsSent
	h.getQueueDepth = queueDepth
	h.getEventsDropped = eventsDropped
}

// Start begins the heartbeat loop.
func (h *Heartbeat) Start(ctx context.Context, sendFunc func(*HeartbeatRequest) (*HeartbeatResponse, error)) {
	if h.running.Load() {
		return
	}

	h.running.Store(true)
	h.logger.Infof("Starting heartbeat (interval: %v)", h.interval)

	go h.heartbeatLoop(ctx, sendFunc)
}

// Stop stops the heartbeat.
func (h *Heartbeat) Stop() {
	h.running.Store(false)
	h.logger.Infof("Heartbeat stopped (sent: %d)", h.heartbeatsSent.Load())
}

// heartbeatLoop runs the periodic heartbeat.
func (h *Heartbeat) heartbeatLoop(ctx context.Context, sendFunc func(*HeartbeatRequest) (*HeartbeatResponse, error)) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	// Kick off background device-info refresh (profile + logged-in user).
	// Runs async so slow WMI calls never block the heartbeat RPC timeout.
	go h.refreshDeviceInfoLoop(ctx)

	// Send initial heartbeat
	h.sendHeartbeat(sendFunc)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !h.running.Load() {
				return
			}
			h.sendHeartbeat(sendFunc)
		}
	}
}

// sendHeartbeat sends a single heartbeat.
func (h *Heartbeat) sendHeartbeat(sendFunc func(*HeartbeatRequest) (*HeartbeatResponse, error)) {
	req := h.buildRequest()

	resp, err := sendFunc(req)
	if err != nil {
		h.logger.Warnf("[Heartbeat] FAILED (server unreachable?): %v", err)
		return
	}

	h.heartbeatsSent.Add(1)
	h.lastHeartbeat = time.Now()

	// Process response
	h.processResponse(resp)

	h.logger.Infof("[Heartbeat] OK: status=%s sent=%d events=%d/%d queue=%d sysmon=%v/%v",
		req.Status, h.heartbeatsSent.Load(), req.EventsSent, req.EventsGenerated, req.QueueDepth,
		req.SysmonInstalled, req.SysmonRunning)
}

// refreshDeviceInfoLoop fetches device profile and logged-in user every 5 minutes.
// Runs as a background goroutine to avoid blocking the heartbeat timeout.
func (h *Heartbeat) refreshDeviceInfoLoop(ctx context.Context) {
	h.refreshDeviceInfo()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.refreshDeviceInfo()
		}
	}
}

// refreshDeviceInfo fetches profile and logged-in user via WMI and caches them.
func (h *Heartbeat) refreshDeviceInfo() {
	profile := getDeviceProfile()
	user := getLoggedInUser()
	h.deviceInfoMu.Lock()
	if profile != "" {
		h.deviceProfile = profile
	}
	if user != "" {
		h.loggedInUser = user
	}
	h.deviceInfoMu.Unlock()
}

// buildRequest creates a heartbeat request with current metrics.
func (h *Heartbeat) buildRequest() *HeartbeatRequest {
	// Get ACTUAL system memory (not Go runtime memory)
	totalMB, usedMB := getSystemMemoryMB()
	cpuCount := getSystemCPUCount()

	// Read status under RLock — concurrent with SetStatus() writes
	h.statusMu.RLock()
	currentStatus := h.status
	h.statusMu.RUnlock()

	sysmonInstalled, sysmonRunning := checkSysmonStatus()

	h.deviceInfoMu.RLock()
	profile := h.deviceProfile
	loggedInUser := h.loggedInUser
	h.deviceInfoMu.RUnlock()

	req := &HeartbeatRequest{
		AgentID:         h.cfg.Agent.ID,
		Timestamp:       time.Now().UTC(),
		Status:          currentStatus,
		Version:         "1.0.0", // TODO: Get from build
		Hostname:        h.cfg.Agent.Hostname,
		OsVersion:       getOSVersion(),
		MemoryUsedMB:    usedMB,
		MemoryTotalMB:   totalMB,
		IPAddresses:     getLocalIPAddresses(),
		CPUCount:        cpuCount,
		SysmonInstalled: sysmonInstalled,
		SysmonRunning:   sysmonRunning,
		Profile:         profile,
		LoggedInUser:    loggedInUser,
	}

	// Get metrics from collectors
	if h.getEventsGenerated != nil {
		req.EventsGenerated = h.getEventsGenerated()
	}
	if h.getEventsSent != nil {
		req.EventsSent = h.getEventsSent()
	}
	if h.getQueueDepth != nil {
		req.QueueDepth = h.getQueueDepth()
	}
	if h.getEventsDropped != nil {
		req.EventsDropped = h.getEventsDropped()
	}

	// Calculate status based on metrics — but NEVER override manually-set
	// operational statuses (Isolated, Updating). These are set by command
	// handlers and reflect deliberate agent state, not metric thresholds.
	if currentStatus != StatusIsolated && currentStatus != StatusUpdating {
		req.Status = h.calculateStatus(req)
	}

	return req
}

// getLocalIPAddresses returns the agent's non-loopback, active IP addresses.
// Uses defensive programming: recovers from panics in OS-level network calls
// to ensure the heartbeat loop is never disrupted by transient system errors.
func getLocalIPAddresses() (addrs []string) {
	// Zero-panic guard: recover from any panic in net.Interfaces()
	defer func() {
		if r := recover(); r != nil {
			// Return empty list rather than crashing the heartbeat
			addrs = nil
		}
	}()

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, iface := range ifaces {
		// Skip down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range ifAddrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			// Skip loopback and link-local addresses
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}

			addrs = append(addrs, ip.String())
		}
	}

	return addrs
}

// calculateStatus determines agent status from metrics.
// Uses percentage-based thresholds for system memory (not Go process memory).
// Queue depth threshold is dynamic: 80% of buffer capacity (avoids false
// "degraded" alerts when the event channel is only partially filled).
func (h *Heartbeat) calculateStatus(req *HeartbeatRequest) HeartbeatStatus {
	// Check for critical: >90% system memory usage
	if req.MemoryTotalMB > 0 {
		memPct := float64(req.MemoryUsedMB) / float64(req.MemoryTotalMB)
		if memPct > 0.90 {
			return StatusCritical
		}
	}

	// Check for degraded conditions
	// Queue depth: trigger only when buffer is >80% full (backpressure).
	// Default buffer is 5000, so threshold = 4000.
	// Old hard-coded threshold of 1000 caused false "degraded" status because
	// ETW routinely fills 1000-2000 events during normal batching.
	queueThreshold := int(float64(h.cfg.Agent.BufferSize) * 0.80)
	if queueThreshold < 500 {
		queueThreshold = 500 // safety floor
	}
	if req.QueueDepth > queueThreshold {
		return StatusDegraded
	}
	if req.CPUUsage > 80 {
		return StatusDegraded
	}

	return StatusHealthy
}

// processResponse handles the server's heartbeat response.
func (h *Heartbeat) processResponse(resp *HeartbeatResponse) {
	if resp == nil {
		return
	}

	// Handle pending commands
	if resp.HasPendingCommands {
		h.logger.Debug("Server has pending commands")
		// TODO: Trigger command fetch
	}

	// Handle certificate renewal
	if resp.CertRenewalRequired {
		h.logger.Warn("Certificate renewal required")
		// TODO: Trigger certificate renewal
	}

	// Handle config update from server
	if resp.ConfigUpdateAvailable && len(resp.NewConfig) > 0 {
		h.logger.Info("[Heartbeat] Server pushed configuration update — applying...")
		if h.onConfigUpdate != nil {
			h.onConfigUpdate(resp.NewConfig)
			h.logger.Info("[Heartbeat] Configuration update applied and saved to Registry")
		} else {
			h.logger.Warn("[Heartbeat] Config update received but no handler registered")
		}
	} else if resp.ConfigUpdateAvailable {
		h.logger.Info("[Heartbeat] Config update flag set but no config payload received")
	}

	// Handle rate adjustment
	if resp.RecommendedBatchSize > 0 {
		h.logger.Infof("Server recommends batch size: %d", resp.RecommendedBatchSize)
		// TODO: Apply to batcher
	}
}

// SetStatus manually sets the agent status.
// Thread-safe: uses RWMutex because this can be called from any goroutine
// (e.g., main thread responding to an isolation command) while the heartbeat
// loop goroutine reads the status in buildRequest().
func (h *Heartbeat) SetStatus(status HeartbeatStatus) {
	h.statusMu.Lock()
	h.status = status
	h.statusMu.Unlock()
	h.logger.Infof("Agent status changed: %s", status)
}

// GetLastHeartbeat returns time of last successful heartbeat.
func (h *Heartbeat) GetLastHeartbeat() time.Time {
	return h.lastHeartbeat
}

// Stats returns heartbeat statistics.
func (h *Heartbeat) Stats() HeartbeatStats {
	h.statusMu.RLock()
	currentStatus := h.status
	h.statusMu.RUnlock()

	return HeartbeatStats{
		Running:        h.running.Load(),
		HeartbeatsSent: h.heartbeatsSent.Load(),
		LastHeartbeat:  h.lastHeartbeat,
		CurrentStatus:  currentStatus,
	}
}

// HeartbeatStats holds heartbeat statistics.
type HeartbeatStats struct {
	Running        bool
	HeartbeatsSent uint64
	LastHeartbeat  time.Time
	CurrentStatus  HeartbeatStatus
}

// checkSysmonStatus queries the Windows service control manager to determine
// whether Sysmon64 is installed and currently running.
// Returns (installed, running). Both are false on any query error.
func checkSysmonStatus() (installed bool, running bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sc.exe", "query", "Sysmon64")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, false
	}
	s := strings.ToUpper(string(out))
	installed = strings.Contains(s, "SERVICE_NAME")
	running = installed && strings.Contains(s, "RUNNING")
	return installed, running
}
