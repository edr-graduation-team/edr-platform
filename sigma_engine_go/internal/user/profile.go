// Package user provides user alert profiles and preferences.
package user

import (
	"fmt"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// AlertProfile defines user-specific alert preferences.
type AlertProfile struct {
	ID                 string             `json:"id"`
	UserID             string             `json:"user_id"`
	Name               string             `json:"name"`
	Default            bool               `json:"default"`
	Preferences        AlertPreferences   `json:"preferences"`
	NotificationConfig NotificationConfig `json:"notification_config"`
	AlertRouting       []RoutingRule      `json:"alert_routing"`
	QuietHours         *QuietHours        `json:"quiet_hours,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// AlertPreferences defines which alerts a user wants to see.
type AlertPreferences struct {
	MinSeverity     string   `json:"min_severity"` // critical, high, medium, low
	WatchRules      []string `json:"watch_rules"`
	WatchAgents     []string `json:"watch_agents"`
	ExcludeRules    []string `json:"exclude_rules"`
	ExcludeAgents   []string `json:"exclude_agents"`
	IncludeKeywords []string `json:"include_keywords"`
	ExcludeKeywords []string `json:"exclude_keywords"`
}

// NotificationConfig defines notification channels for a profile.
type NotificationConfig struct {
	SlackChannel string `json:"slack_channel,omitempty"`
	SlackDM      bool   `json:"slack_dm"`
	EmailAddress string `json:"email_address,omitempty"`
	TeamsChannel string `json:"teams_channel,omitempty"`
	WebhookURL   string `json:"webhook_url,omitempty"`
}

// RoutingRule defines how alerts are routed.
type RoutingRule struct {
	Condition string `json:"condition"` // severity = critical, rule = malware-*
	Action    string `json:"action"`    // immediate, batch_5m, batch_1h, daily_digest
	Channel   string `json:"channel,omitempty"`
}

// QuietHours defines do-not-disturb periods.
type QuietHours struct {
	Enabled      bool `json:"enabled"`
	StartHour    int  `json:"start_hour"`    // 0-23
	EndHour      int  `json:"end_hour"`      // 0-23
	Weekends     bool `json:"weekends"`      // Apply to weekends
	BufferAlerts bool `json:"buffer_alerts"` // Queue and send after quiet hours
}

// UserAlert tracks alerts sent to a user.
type UserAlert struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	ProfileID string     `json:"profile_id"`
	AlertID   string     `json:"alert_id"`
	Channel   string     `json:"channel"`
	Status    string     `json:"status"` // pending, sent, failed, buffered
	SentAt    *time.Time `json:"sent_at,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ProfileManager manages user alert profiles.
type ProfileManager struct {
	mu         sync.RWMutex
	profiles   map[string]*AlertProfile // key: profile ID
	userAlerts []UserAlert
}

// NewProfileManager creates a new profile manager.
func NewProfileManager() *ProfileManager {
	return &ProfileManager{
		profiles:   make(map[string]*AlertProfile),
		userAlerts: make([]UserAlert, 0),
	}
}

// CreateProfile creates a new alert profile.
func (m *ProfileManager) CreateProfile(profile AlertProfile) (*AlertProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}
	profile.CreatedAt = time.Now()
	profile.UpdatedAt = time.Now()

	m.profiles[profile.ID] = &profile
	logger.Infof("Created profile: %s for user %s", profile.Name, profile.UserID)
	return &profile, nil
}

// GetProfile retrieves a profile by ID.
func (m *ProfileManager) GetProfile(id string) (*AlertProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profile, exists := m.profiles[id]
	if !exists {
		return nil, fmt.Errorf("profile not found: %s", id)
	}
	return profile, nil
}

// GetUserProfiles returns all profiles for a user.
func (m *ProfileManager) GetUserProfiles(userID string) []*AlertProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profiles := make([]*AlertProfile, 0)
	for _, p := range m.profiles {
		if p.UserID == userID {
			profiles = append(profiles, p)
		}
	}
	return profiles
}

// GetDefaultProfile returns user's default profile.
func (m *ProfileManager) GetDefaultProfile(userID string) *AlertProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.profiles {
		if p.UserID == userID && p.Default {
			return p
		}
	}
	return nil
}

// UpdateProfile updates an existing profile.
func (m *ProfileManager) UpdateProfile(id string, profile AlertProfile) (*AlertProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.profiles[id]
	if !exists {
		return nil, fmt.Errorf("profile not found: %s", id)
	}

	profile.ID = id
	profile.UserID = existing.UserID
	profile.CreatedAt = existing.CreatedAt
	profile.UpdatedAt = time.Now()
	m.profiles[id] = &profile
	return &profile, nil
}

// DeleteProfile removes a profile.
func (m *ProfileManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.profiles[id]; !exists {
		return fmt.Errorf("profile not found: %s", id)
	}
	delete(m.profiles, id)
	return nil
}

// ShouldDeliverAlert checks if an alert should be delivered based on profile.
func (m *ProfileManager) ShouldDeliverAlert(profile *AlertProfile, alert *domain.Alert) bool {
	// Check severity
	if !m.meetsSeverityThreshold(profile.Preferences.MinSeverity, alert.Severity.String()) {
		return false
	}

	// Check watch rules
	if len(profile.Preferences.WatchRules) > 0 {
		found := false
		for _, r := range profile.Preferences.WatchRules {
			if r == alert.RuleID || r == "*" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check exclude rules
	for _, r := range profile.Preferences.ExcludeRules {
		if r == alert.RuleID {
			return false
		}
	}

	// Check quiet hours
	if profile.QuietHours != nil && profile.QuietHours.Enabled {
		if m.isQuietHours(profile.QuietHours) && !profile.QuietHours.BufferAlerts {
			return false
		}
	}

	return true
}

// meetsSeverityThreshold checks if alert meets minimum severity.
func (m *ProfileManager) meetsSeverityThreshold(minSev, alertSev string) bool {
	severityOrder := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
	}

	minLevel := severityOrder[minSev]
	alertLevel := severityOrder[alertSev]

	return alertLevel >= minLevel
}

// isQuietHours checks if current time is within quiet hours.
func (m *ProfileManager) isQuietHours(qh *QuietHours) bool {
	now := time.Now()
	hour := now.Hour()
	weekday := now.Weekday()

	// Check weekends
	if qh.Weekends && (weekday == time.Saturday || weekday == time.Sunday) {
		return true
	}

	// Check hour range
	if qh.StartHour < qh.EndHour {
		return hour >= qh.StartHour && hour < qh.EndHour
	} else {
		// Overnight quiet hours (e.g., 22:00 - 06:00)
		return hour >= qh.StartHour || hour < qh.EndHour
	}
}

// GetRoutingAction determines notification action for an alert.
func (m *ProfileManager) GetRoutingAction(profile *AlertProfile, alert *domain.Alert) string {
	// Check routing rules
	for _, rule := range profile.AlertRouting {
		if m.matchesRoutingCondition(rule.Condition, alert) {
			return rule.Action
		}
	}

	// Default based on severity
	switch alert.Severity.String() {
	case "critical", "high":
		return "immediate"
	case "medium":
		return "batch_5m"
	default:
		return "batch_1h"
	}
}

// matchesRoutingCondition checks if alert matches routing condition.
func (m *ProfileManager) matchesRoutingCondition(condition string, alert *domain.Alert) bool {
	// Simple parsing: "severity = critical" or "rule = malware-*"
	// In production, use proper parser
	if condition == "severity = critical" && alert.Severity == domain.SeverityCritical {
		return true
	}
	if condition == "severity = high" && alert.Severity == domain.SeverityHigh {
		return true
	}
	return false
}

// RecordUserAlert records that an alert was sent to a user.
func (m *ProfileManager) RecordUserAlert(userID, profileID, alertID, channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	ua := UserAlert{
		ID:        uuid.New().String(),
		UserID:    userID,
		ProfileID: profileID,
		AlertID:   alertID,
		Channel:   channel,
		Status:    "sent",
		SentAt:    &now,
		CreatedAt: now,
	}

	m.userAlerts = append(m.userAlerts, ua)
	if len(m.userAlerts) > 10000 {
		m.userAlerts = m.userAlerts[len(m.userAlerts)-10000:]
	}
}

// GetUserAlerts returns alerts delivered to a user.
func (m *ProfileManager) GetUserAlerts(userID string, limit int) []UserAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]UserAlert, 0)
	for i := len(m.userAlerts) - 1; i >= 0 && len(result) < limit; i-- {
		if m.userAlerts[i].UserID == userID {
			result = append(result, m.userAlerts[i])
		}
	}
	return result
}

// GetProfileStats returns profile statistics.
func (m *ProfileManager) GetProfileStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"total_profiles": len(m.profiles),
		"total_alerts":   len(m.userAlerts),
	}
}
