// Package models defines the domain models for the connection-manager.
package models

import (
	"time"

	"github.com/google/uuid"
)

// Agent represents an EDR agent registered with the server.
type Agent struct {
	ID       uuid.UUID `db:"id" json:"id"`
	Hostname string    `db:"hostname" json:"hostname"`
	Status   string    `db:"status" json:"status"` // pending, online, offline, degraded, suspended

	// Device information
	OSType    string `db:"os_type" json:"os_type"`       // windows, linux, macos
	OSVersion string `db:"os_version" json:"os_version"` // e.g., "Windows 10 21H2"
	CPUCount  int    `db:"cpu_count" json:"cpu_count"`
	MemoryMB  int64  `db:"memory_mb" json:"memory_mb"`

	// Agent metadata
	AgentVersion  string     `db:"agent_version" json:"agent_version"`
	InstalledDate *time.Time `db:"installed_date" json:"installed_date"`
	LastSeen      time.Time  `db:"last_seen" json:"last_seen"`

	// Metrics
	EventsCollected int64   `db:"events_collected" json:"events_collected"`
	EventsDelivered int64   `db:"events_delivered" json:"events_delivered"`
	EventsDropped   int64   `db:"events_dropped" json:"events_dropped"`
	QueueDepth      int     `db:"queue_depth" json:"queue_depth"`
	CPUUsage        float64 `db:"cpu_usage" json:"cpu_usage"`
	MemoryUsedMB    int64   `db:"memory_used_mb" json:"memory_used_mb"`
	HealthScore     float64 `db:"health_score" json:"health_score"`

	// Network telemetry
	IPAddresses []string `db:"ip_addresses" json:"ip_addresses"`
	IsIsolated  bool     `db:"is_isolated" json:"is_isolated"` // Smart network isolation active

	// Certificate
	CurrentCertID *uuid.UUID `db:"current_cert_id" json:"current_cert_id"`
	CertExpiresAt *time.Time `db:"cert_expires_at" json:"cert_expires_at"`

	// Metadata
	Tags     map[string]string `db:"tags" json:"tags"`
	Metadata map[string]string `db:"metadata" json:"metadata"`

	// Timestamps
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// AgentStatus constants
const (
	AgentStatusPending   = "pending"
	AgentStatusOnline    = "online"
	AgentStatusOffline   = "offline"
	AgentStatusDegraded  = "degraded"
	AgentStatusSuspended = "suspended"
)

// IsOnline returns true if the agent is currently online.
func (a *Agent) IsOnline() bool {
	return a.Status == AgentStatusOnline
}

// IsApproved returns true if the agent has been approved (not pending).
func (a *Agent) IsApproved() bool {
	return a.Status != AgentStatusPending
}

// NeedsCertRenewal returns true if the certificate expires within the given duration.
func (a *Agent) NeedsCertRenewal(within time.Duration) bool {
	if a.CertExpiresAt == nil || a.CertExpiresAt.IsZero() {
		return true
	}
	return time.Until(*a.CertExpiresAt) < within
}

// UpdateMetrics updates the agent's metrics from a heartbeat.
func (a *Agent) UpdateMetrics(cpuUsage float64, memoryUsedMB int64, queueDepth int, eventsGenerated, eventsSent, eventsDropped int64) {
	a.CPUUsage = cpuUsage
	a.MemoryUsedMB = memoryUsedMB
	a.QueueDepth = queueDepth
	a.EventsCollected = eventsGenerated
	a.EventsDelivered = eventsSent
	a.EventsDropped = eventsDropped
	a.LastSeen = time.Now()
	a.UpdatedAt = time.Now()
}

// CalculateHealthScore calculates the agent's health score based on metrics.
// Factors:
//   - Delivery ratio (50% weight): events_delivered / events_collected
//   - Status score (30% weight): online=100, degraded=80, offline=50, suspended=0
//   - Drop rate factor (20% weight): penalizes high drop rates as a potential blinding indicator
func (a *Agent) CalculateHealthScore() float64 {
	// Delivery ratio (50% weight)
	var deliveryRatio float64 = 100.0
	if a.EventsCollected > 0 {
		deliveryRatio = float64(a.EventsDelivered) / float64(a.EventsCollected) * 100
	}

	// Status score (30% weight)
	statusScore := 100.0
	switch a.Status {
	case AgentStatusOnline:
		statusScore = 100.0
	case AgentStatusDegraded:
		statusScore = 80.0
	case AgentStatusOffline:
		statusScore = 50.0
	case AgentStatusSuspended:
		statusScore = 0.0
	}

	// Drop rate factor (20% weight) — high drops degrade health
	// >20% drop rate = full penalty, <5% = no penalty, linear between
	dropScore := 100.0
	if a.EventsCollected > 0 {
		dropRate := float64(a.EventsDropped) / float64(a.EventsCollected)
		switch {
		case dropRate > 0.20:
			dropScore = 0.0 // Severe: potential blinding attack
		case dropRate > 0.05:
			// Linear degradation from 100 to 0 between 5% and 20%
			dropScore = (0.20 - dropRate) / 0.15 * 100
		}
	}

	// Combined score
	a.HealthScore = (deliveryRatio * 0.5) + (statusScore * 0.3) + (dropScore * 0.2)
	return a.HealthScore
}
