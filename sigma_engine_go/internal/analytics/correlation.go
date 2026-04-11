// Package analytics provides alert correlation and incident grouping.
package analytics

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// CorrelationType defines relationship types.
type CorrelationType string

const (
	CorrSameAgent CorrelationType = "same_agent"
	CorrSameRule  CorrelationType = "same_rule"
	CorrSameUser  CorrelationType = "same_user"
	CorrTimeBased CorrelationType = "time_based"
)

// AlertRelationship links two related alerts.
type AlertRelationship struct {
	ID               string          `json:"id"`
	Alert1ID         string          `json:"alert1_id"`
	Alert2ID         string          `json:"alert2_id"`
	RelationType     CorrelationType `json:"relationship_type"`
	CorrelationScore float64         `json:"correlation_score"`
	CreatedAt        time.Time       `json:"created_at"`
}

// Incident groups related alerts.
type Incident struct {
	ID          string     `json:"id"`
	AlertIDs    []string   `json:"alert_ids"`
	Status      string     `json:"status"` // open, investigating, resolved
	Severity    string     `json:"severity"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Notes       []Note     `json:"notes"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

// Note represents an incident note.
type Note struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// CorrelationManager handles alert correlation.
type CorrelationManager struct {
	mu            sync.RWMutex
	relationships []AlertRelationship
	incidents     map[string]*Incident
	alertCache    map[string]*domain.Alert // Recent alerts for correlation
}

// NewCorrelationManager creates a new correlation manager.
func NewCorrelationManager() *CorrelationManager {
	return &CorrelationManager{
		relationships: make([]AlertRelationship, 0),
		incidents:     make(map[string]*Incident),
		alertCache:    make(map[string]*domain.Alert),
	}
}

// CorrelateAlert finds correlations for a new alert.
func (m *CorrelationManager) CorrelateAlert(alert *domain.Alert) []AlertRelationship {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cache alert
	m.alertCache[alert.ID] = alert

	// Find correlations
	correlations := make([]AlertRelationship, 0)

	for id, cached := range m.alertCache {
		if id == alert.ID {
			continue
		}

		timeDelta := alert.Timestamp.Sub(cached.Timestamp).Abs()

		// FIX ISSUE-09: Time-decayed correlation scoring.
		// Reference: MITRE ATT&CK Kill Chain — alerts closer in time are
		// more likely to be part of the same attack chain.
		// Formula: score = baseScore × exp(-timeDelta / halfLife)
		//   halfLife = 2.5 minutes → alerts 10s apart ≈ 0.97×, 4 min apart ≈ 0.20×
		timeDecay := math.Exp(-timeDelta.Seconds() / 150.0) // 150s = 2.5min half-life

		// Check for same rule
		if cached.RuleID == alert.RuleID {
			// Same rule correlation: base 0.85 × time decay
			// (higher base than before because same-rule is very strong signal)
			score := 0.85 * timeDecay
			if score > 0.1 { // Only record if meaningful
				rel := AlertRelationship{
					ID:               uuid.New().String(),
					Alert1ID:         cached.ID,
					Alert2ID:         alert.ID,
					RelationType:     CorrSameRule,
					CorrelationScore: score,
					CreatedAt:        time.Now(),
				}
				correlations = append(correlations, rel)
				m.relationships = append(m.relationships, rel)
			}
		}

		// Check for time-based correlation (within 10 minutes window)
		if timeDelta < 10*time.Minute {
			// Time-based correlation: base 0.6 × time decay
			score := 0.6 * timeDecay
			if score > 0.1 {
				rel := AlertRelationship{
					ID:               uuid.New().String(),
					Alert1ID:         cached.ID,
					Alert2ID:         alert.ID,
					RelationType:     CorrTimeBased,
					CorrelationScore: score,
					CreatedAt:        time.Now(),
				}
				correlations = append(correlations, rel)
				m.relationships = append(m.relationships, rel)
			}
		}
	}

	// Clean old cached alerts (keep last hour)
	oneHourAgo := time.Now().Add(-time.Hour)
	for id, cached := range m.alertCache {
		if cached.Timestamp.Before(oneHourAgo) {
			delete(m.alertCache, id)
		}
	}

	// Limit relationships
	if len(m.relationships) > 10000 {
		m.relationships = m.relationships[len(m.relationships)-10000:]
	}

	return correlations
}

// GetAlertCorrelations returns correlations for an alert.
func (m *CorrelationManager) GetAlertCorrelations(alertID string) []AlertRelationship {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AlertRelationship, 0)
	for _, rel := range m.relationships {
		if rel.Alert1ID == alertID || rel.Alert2ID == alertID {
			result = append(result, rel)
		}
	}
	return result
}

// CreateIncident creates a new incident from alerts.
func (m *CorrelationManager) CreateIncident(alertIDs []string, title string) (*Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	incident := &Incident{
		ID:        uuid.New().String(),
		AlertIDs:  alertIDs,
		Status:    "open",
		Title:     title,
		Notes:     make([]Note, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Determine severity from alerts
	incident.Severity = "medium" // Default

	m.incidents[incident.ID] = incident
	logger.Infof("Created incident %s with %d alerts", incident.ID, len(alertIDs))
	return incident, nil
}

// GetIncident retrieves an incident.
func (m *CorrelationManager) GetIncident(id string) (*Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	incident, exists := m.incidents[id]
	if !exists {
		return nil, fmt.Errorf("incident not found: %s", id)
	}
	return incident, nil
}

// ListIncidents returns all incidents.
func (m *CorrelationManager) ListIncidents(status string) []*Incident {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Incident, 0)
	for _, inc := range m.incidents {
		if status == "" || inc.Status == status {
			result = append(result, inc)
		}
	}
	return result
}

// UpdateIncident updates an incident.
func (m *CorrelationManager) UpdateIncident(id string, status string) (*Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	incident, exists := m.incidents[id]
	if !exists {
		return nil, fmt.Errorf("incident not found: %s", id)
	}

	incident.Status = status
	incident.UpdatedAt = time.Now()
	if status == "resolved" {
		now := time.Now()
		incident.ClosedAt = &now
	}

	return incident, nil
}

// AddNote adds a note to an incident.
func (m *CorrelationManager) AddNote(incidentID, authorID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	incident, exists := m.incidents[incidentID]
	if !exists {
		return fmt.Errorf("incident not found: %s", incidentID)
	}

	note := Note{
		ID:        uuid.New().String(),
		AuthorID:  authorID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	incident.Notes = append(incident.Notes, note)
	incident.UpdatedAt = time.Now()
	return nil
}

// GetAnalytics returns correlation analytics.
func (m *CorrelationManager) GetAnalytics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	openIncidents := 0
	for _, inc := range m.incidents {
		if inc.Status == "open" {
			openIncidents++
		}
	}

	return map[string]interface{}{
		"total_relationships": len(m.relationships),
		"total_incidents":     len(m.incidents),
		"open_incidents":      openIncidents,
		"cached_alerts":       len(m.alertCache),
	}
}
