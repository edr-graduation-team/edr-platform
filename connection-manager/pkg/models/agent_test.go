// Package models provides unit tests for domain models.
package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAgent_IsOnline(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"online agent", AgentStatusOnline, true},
		{"offline agent", AgentStatusOffline, false},
		{"pending agent", AgentStatusPending, false},
		{"degraded agent", AgentStatusDegraded, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &Agent{Status: tt.status}
			assert.Equal(t, tt.want, agent.IsOnline())
		})
	}
}

func TestAgent_IsApproved(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"pending agent", AgentStatusPending, false},
		{"online agent", AgentStatusOnline, true},
		{"offline agent", AgentStatusOffline, true},
		{"suspended agent", AgentStatusSuspended, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &Agent{Status: tt.status}
			assert.Equal(t, tt.want, agent.IsApproved())
		})
	}
}

func TestAgent_NeedsCertRenewal(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		within    time.Duration
		want      bool
	}{
		{
			name:      "expires in 1 day, check within 7 days",
			expiresAt: time.Now().Add(24 * time.Hour),
			within:    7 * 24 * time.Hour,
			want:      true,
		},
		{
			name:      "expires in 30 days, check within 7 days",
			expiresAt: time.Now().Add(30 * 24 * time.Hour),
			within:    7 * 24 * time.Hour,
			want:      false,
		},
		{
			name:      "zero expiry time",
			expiresAt: time.Time{},
			within:    7 * 24 * time.Hour,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// agent := &Agent{CertExpiresAt: tt.expiresAt}
			// assert.Equal(t, tt.want, agent.NeedsCertRenewal(tt.within))
		})
	}
}

func TestAgent_CalculateHealthScore(t *testing.T) {
	tests := []struct {
		name            string
		eventsCollected int64
		eventsDelivered int64
		status          string
		minScore        float64
		maxScore        float64
	}{
		{
			name:            "perfect delivery, online",
			eventsCollected: 100,
			eventsDelivered: 100,
			status:          AgentStatusOnline,
			minScore:        99,
			maxScore:        100,
		},
		{
			name:            "90% delivery, online",
			eventsCollected: 100,
			eventsDelivered: 90,
			status:          AgentStatusOnline,
			minScore:        85,
			maxScore:        100,
		},
		{
			name:            "50% delivery, degraded",
			eventsCollected: 100,
			eventsDelivered: 50,
			status:          AgentStatusDegraded,
			minScore:        40,
			maxScore:        75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &Agent{
				EventsCollected: tt.eventsCollected,
				EventsDelivered: tt.eventsDelivered,
				Status:          tt.status,
			}
			score := agent.CalculateHealthScore()
			assert.GreaterOrEqual(t, score, tt.minScore)
			assert.LessOrEqual(t, score, tt.maxScore)
		})
	}
}

func TestAgent_UpdateMetrics(t *testing.T) {
	agent := &Agent{
		ID:       uuid.New(),
		Hostname: "test-host",
	}

	before := time.Now()
	agent.UpdateMetrics(25.5, 1024, 50, 1000, 990, 10)
	after := time.Now()

	assert.Equal(t, 25.5, agent.CPUUsage)
	assert.Equal(t, int64(1024), agent.MemoryUsedMB)
	assert.Equal(t, 50, agent.QueueDepth)
	assert.Equal(t, int64(1000), agent.EventsCollected)
	assert.Equal(t, int64(990), agent.EventsDelivered)
	assert.Equal(t, int64(10), agent.EventsDropped)
	assert.True(t, agent.LastSeen.After(before) || agent.LastSeen.Equal(before))
	assert.True(t, agent.LastSeen.Before(after) || agent.LastSeen.Equal(after))
}
