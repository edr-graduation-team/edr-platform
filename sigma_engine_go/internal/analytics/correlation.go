// Package analytics provides alert correlation and incident grouping.
package analytics

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/database"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// EdgePersistence persists undirected edges (optional).
type EdgePersistence interface {
	UpsertEdge(ctx context.Context, low, high string, relType string, score float64) error
	ListRecentEdges(ctx context.Context, limit int) ([]PersistedEdge, error)
	LoadRecentAlerts(ctx context.Context, limit int) ([]*domain.Alert, error)
	LoadCandidates(ctx context.Context, seed *domain.Alert, window time.Duration, limit int) ([]*domain.Alert, error)
	PruneOldEdges(ctx context.Context, ttl time.Duration) (int64, error)
}

// PersistedEdge is a row-shaped edge for bootstrap.
type PersistedEdge struct {
	AlertLowID   string
	AlertHighID  string
	RelationType string
	Score        float64
	CreatedAt    time.Time
}

// postgresEdgePersistence adapts database.CorrelationRepository to EdgePersistence.
type postgresEdgePersistence struct {
	repo *database.CorrelationRepository
}

// NewPostgresEdgePersistence returns an EdgePersistence backed by PostgreSQL, or nil if repo is nil.
func NewPostgresEdgePersistence(repo *database.CorrelationRepository) EdgePersistence {
	if repo == nil {
		return nil
	}
	return &postgresEdgePersistence{repo: repo}
}

func (p *postgresEdgePersistence) UpsertEdge(ctx context.Context, low, high, relType string, score float64) error {
	return p.repo.UpsertEdge(ctx, low, high, relType, score)
}

func (p *postgresEdgePersistence) ListRecentEdges(ctx context.Context, limit int) ([]PersistedEdge, error) {
	rows, err := p.repo.ListRecentEdges(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]PersistedEdge, 0, len(rows))
	for _, r := range rows {
		out = append(out, PersistedEdge{
			AlertLowID:   r.AlertLowID,
			AlertHighID:  r.AlertHighID,
			RelationType: r.RelationType,
			Score:        r.CorrelationScore,
			CreatedAt:    r.CreatedAt,
		})
	}
	return out, nil
}

func (p *postgresEdgePersistence) LoadRecentAlerts(ctx context.Context, limit int) ([]*domain.Alert, error) {
	return p.repo.LoadRecentAlertsForCorrelation(ctx, limit)
}

func (p *postgresEdgePersistence) LoadCandidates(ctx context.Context, seed *domain.Alert, window time.Duration, limit int) ([]*domain.Alert, error) {
	return p.repo.LoadCandidateAlertsForCorrelation(ctx, seed, window, limit)
}

func (p *postgresEdgePersistence) PruneOldEdges(ctx context.Context, ttl time.Duration) (int64, error) {
	return p.repo.PruneEdgesOlderThan(ctx, ttl)
}

// CorrelationType defines relationship types.
type CorrelationType string

const (
	CorrSameAgent CorrelationType = "same_agent"
	CorrSameRule  CorrelationType = "same_rule"
	CorrSameUser  CorrelationType = "same_user"
	CorrTimeBased CorrelationType = "time_based"
)

const (
	timeDecayHalfLifeSec = 150.0 // 2.5 min half-life
	correlationWindow    = 10 * time.Minute
	minEdgeScore         = 0.1
	maxRelationships     = 10000
	cacheRetention       = time.Hour
	edgeRetention        = 7 * 24 * time.Hour
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
	Status      string     `json:"status"`
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

// CorrelationManager handles alert correlation (in-memory + optional PostgreSQL edges).
type CorrelationManager struct {
	mu            sync.RWMutex
	relationships []AlertRelationship
	incidents     map[string]*Incident
	alertCache    map[string]*domain.Alert

	byRule  map[string]map[string]struct{}
	byAgent map[string]map[string]struct{}
	byUser  map[string]map[string]struct{}

	// seenPairs prevents duplicate undirected edges in memory (bootstrap + live).
	seenPairs map[string]struct{}

	persistence EdgePersistence
}

// NewCorrelationManager creates a new correlation manager.
func NewCorrelationManager() *CorrelationManager {
	return &CorrelationManager{
		relationships: make([]AlertRelationship, 0),
		incidents:     make(map[string]*Incident),
		alertCache:    make(map[string]*domain.Alert),
		byRule:        make(map[string]map[string]struct{}),
		byAgent:       make(map[string]map[string]struct{}),
		byUser:        make(map[string]map[string]struct{}),
		seenPairs:     make(map[string]struct{}),
	}
}

// SetPersistence enables async PostgreSQL persistence of new edges. Nil disables.
func (m *CorrelationManager) SetPersistence(p EdgePersistence) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistence = p
}

// Bootstrap loads recent edges and warms the alert cache from PostgreSQL (call before processing).
func (m *CorrelationManager) Bootstrap(ctx context.Context) error {
	m.mu.Lock()
	p := m.persistence
	m.mu.Unlock()
	if p == nil {
		return nil
	}
	edges, err := p.ListRecentEdges(ctx, maxRelationships)
	if err != nil {
		return fmt.Errorf("list correlation edges: %w", err)
	}
	alerts, err := p.LoadRecentAlerts(ctx, 500)
	if err != nil {
		return fmt.Errorf("load recent alerts for correlation cache: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range edges {
		pk := pairKey(e.AlertLowID, e.AlertHighID)
		if _, dup := m.seenPairs[pk]; dup {
			continue
		}
		m.seenPairs[pk] = struct{}{}
		m.relationships = append(m.relationships, AlertRelationship{
			ID:               uuid.New().String(),
			Alert1ID:         e.AlertLowID,
			Alert2ID:         e.AlertHighID,
			RelationType:     CorrelationType(e.RelationType),
			CorrelationScore: e.Score,
			CreatedAt:        e.CreatedAt,
		})
	}
	if len(m.relationships) > maxRelationships {
		m.relationships = m.relationships[len(m.relationships)-maxRelationships:]
	}

	for _, a := range alerts {
		if a == nil || a.ID == "" {
			continue
		}
		if _, ok := m.alertCache[a.ID]; ok {
			continue
		}
		m.alertCache[a.ID] = a
		m.addToIndexesLocked(a)
	}
	logger.Infof("Correlation bootstrap: %d edges, %d cached alerts", len(edges), len(m.alertCache))
	if pruned, err := p.PruneOldEdges(ctx, edgeRetention); err != nil {
		logger.Warnf("Correlation prune warning: %v", err)
	} else if pruned > 0 {
		logger.Infof("Correlation prune: removed %d stale edges", pruned)
	}
	return nil
}

// CorrelateAlert finds at most one deduplicated edge per peer alert, updates caches, and sets correlation fields on alert.
func (m *CorrelationManager) CorrelateAlert(alert *domain.Alert) []AlertRelationship {
	if alert == nil || alert.ID == "" {
		return nil
	}

	m.mu.RLock()
	p := m.persistence
	m.mu.RUnlock()

	// Cross-instance enhancement: query recent likely candidates from shared DB.
	var externalCandidates []*domain.Alert
	if p != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		ext, err := p.LoadCandidates(ctx, alert, correlationWindow, 200)
		cancel()
		if err != nil {
			logger.Debugf("Correlation candidate lookup skipped: %v", err)
		} else {
			externalCandidates = ext
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ext := range externalCandidates {
		if ext == nil || ext.ID == "" || ext.ID == alert.ID {
			continue
		}
		if _, exists := m.alertCache[ext.ID]; exists {
			continue
		}
		m.alertCache[ext.ID] = ext
		m.addToIndexesLocked(ext)
	}

	candidates := m.collectCandidateIDsLocked(alert)
	newRels := make([]AlertRelationship, 0)

	for _, candID := range candidates {
		cached := m.alertCache[candID]
		if cached == nil {
			continue
		}
		older, newer := orderedByTimeThenID(cached, alert)
		ct, score, ok := bestEdge(older, newer)
		if !ok {
			continue
		}
		pk := pairKey(older.ID, newer.ID)
		if _, dup := m.seenPairs[pk]; dup {
			continue
		}
		m.seenPairs[pk] = struct{}{}

		rel := AlertRelationship{
			ID:               uuid.New().String(),
			Alert1ID:         older.ID,
			Alert2ID:         newer.ID,
			RelationType:     ct,
			CorrelationScore: score,
			CreatedAt:        time.Now(),
		}
		newRels = append(newRels, rel)
		m.relationships = append(m.relationships, rel)
		m.persistEdgeLocked(rel)
	}

	m.alertCache[alert.ID] = alert
	m.addToIndexesLocked(alert)

	m.evictStaleFromCacheLocked()
	if len(m.relationships) > maxRelationships {
		m.relationships = m.relationships[len(m.relationships)-maxRelationships:]
	}

	applyCorrelationToAlert(alert, newRels)
	return newRels
}

func (m *CorrelationManager) persistEdgeLocked(rel AlertRelationship) {
	p := m.persistence
	if p == nil {
		return
	}
	low, high := canonicalPair(rel.Alert1ID, rel.Alert2ID)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.UpsertEdge(ctx, low, high, string(rel.RelationType), rel.CorrelationScore); err != nil {
			logger.Warnf("correlation edge persist failed: %v", err)
		}
	}()
}

func (m *CorrelationManager) collectCandidateIDsLocked(alert *domain.Alert) []string {
	seen := make(map[string]struct{})

	add := func(id string) {
		if id != "" && id != alert.ID {
			seen[id] = struct{}{}
		}
	}

	if alert.RuleID != "" {
		if bucket, ok := m.byRule[alert.RuleID]; ok {
			for id := range bucket {
				add(id)
			}
		}
	}
	if ag := agentIDFromAlert(alert); ag != "" {
		if bucket, ok := m.byAgent[ag]; ok {
			for id := range bucket {
				add(id)
			}
		}
	}
	if uk := userKeyFromAlert(alert); uk != "" {
		if bucket, ok := m.byUser[uk]; ok {
			for id := range bucket {
				add(id)
			}
		}
	}
	for id, ca := range m.alertCache {
		if id == alert.ID {
			continue
		}
		if alert.Timestamp.Sub(ca.Timestamp).Abs() < correlationWindow {
			add(id)
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func (m *CorrelationManager) addToIndexesLocked(a *domain.Alert) {
	if a.RuleID != "" {
		if m.byRule[a.RuleID] == nil {
			m.byRule[a.RuleID] = make(map[string]struct{})
		}
		m.byRule[a.RuleID][a.ID] = struct{}{}
	}
	if ag := agentIDFromAlert(a); ag != "" {
		if m.byAgent[ag] == nil {
			m.byAgent[ag] = make(map[string]struct{})
		}
		m.byAgent[ag][a.ID] = struct{}{}
	}
	if uk := userKeyFromAlert(a); uk != "" {
		if m.byUser[uk] == nil {
			m.byUser[uk] = make(map[string]struct{})
		}
		m.byUser[uk][a.ID] = struct{}{}
	}
}

func (m *CorrelationManager) removeFromIndexesLocked(a *domain.Alert) {
	if a.RuleID != "" && m.byRule[a.RuleID] != nil {
		delete(m.byRule[a.RuleID], a.ID)
		if len(m.byRule[a.RuleID]) == 0 {
			delete(m.byRule, a.RuleID)
		}
	}
	if ag := agentIDFromAlert(a); ag != "" && m.byAgent[ag] != nil {
		delete(m.byAgent[ag], a.ID)
		if len(m.byAgent[ag]) == 0 {
			delete(m.byAgent, ag)
		}
	}
	if uk := userKeyFromAlert(a); uk != "" && m.byUser[uk] != nil {
		delete(m.byUser[uk], a.ID)
		if len(m.byUser[uk]) == 0 {
			delete(m.byUser, uk)
		}
	}
}

func (m *CorrelationManager) evictStaleFromCacheLocked() {
	cutoff := time.Now().Add(-cacheRetention)
	for id, ca := range m.alertCache {
		if ca.Timestamp.Before(cutoff) {
			m.removeFromIndexesLocked(ca)
			delete(m.alertCache, id)
		}
	}
}

func orderedByTimeThenID(a, b *domain.Alert) (older, newer *domain.Alert) {
	if a.Timestamp.Before(b.Timestamp) {
		return a, b
	}
	if b.Timestamp.Before(a.Timestamp) {
		return b, a
	}
	if a.ID < b.ID {
		return a, b
	}
	return b, a
}

func bestEdge(older, newer *domain.Alert) (CorrelationType, float64, bool) {
	timeDelta := newer.Timestamp.Sub(older.Timestamp).Abs()
	timeDecay := math.Exp(-timeDelta.Seconds() / timeDecayHalfLifeSec)

	// 1) Same rule — strongest signal; no 10m cap (time decay only).
	if older.RuleID != "" && older.RuleID == newer.RuleID {
		s := 0.85 * timeDecay
		if s > minEdgeScore {
			return CorrSameRule, s, true
		}
	}

	// 2) Same agent (within window), typically different rules.
	if agO, agN := agentIDFromAlert(older), agentIDFromAlert(newer); agO != "" && agO == agN && timeDelta < correlationWindow {
		s := 0.78 * timeDecay
		if s > minEdgeScore {
			return CorrSameAgent, s, true
		}
	}

	// 3) Same user principal (within window).
	if uO, uN := userKeyFromAlert(older), userKeyFromAlert(newer); uO != "" && uO == uN && timeDelta < correlationWindow {
		s := 0.65 * timeDecay
		if s > minEdgeScore {
			return CorrSameUser, s, true
		}
	}

	// 4) Pure time proximity.
	if timeDelta < correlationWindow {
		s := 0.6 * timeDecay
		if s > minEdgeScore {
			return CorrTimeBased, s, true
		}
	}
	return "", 0, false
}

func canonicalPair(idA, idB string) (low, high string) {
	if idA < idB {
		return idA, idB
	}
	return idB, idA
}

func pairKey(idA, idB string) string {
	l, h := canonicalPair(idA, idB)
	return l + "|" + h
}

func agentIDFromAlert(a *domain.Alert) string {
	if a == nil || a.EventData == nil {
		return ""
	}
	v, ok := a.EventData["agent_id"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func userKeyFromAlert(a *domain.Alert) string {
	if a == nil || a.EventData == nil {
		return ""
	}
	if v, ok := a.EventData["user_sid"]; ok && v != nil {
		s := fmt.Sprint(v)
		if s != "" {
			return "sid:" + s
		}
	}
	if v, ok := a.EventData["user_name"]; ok && v != nil {
		s := fmt.Sprint(v)
		if s != "" {
			return "user:" + s
		}
	}
	return ""
}

func applyCorrelationToAlert(a *domain.Alert, rels []AlertRelationship) {
	if a == nil || len(rels) == 0 {
		return
	}
	maxScore := rels[0].CorrelationScore
	for _, r := range rels[1:] {
		if r.CorrelationScore > maxScore {
			maxScore = r.CorrelationScore
		}
	}
	pt := primaryCorrelationType(rels)
	a.CorrelationSummary = &domain.CorrelationSummary{
		EdgesAdded:     len(rels),
		PrimaryType:    pt,
		StrongestScore: maxScore,
	}
	if a.ContextSnapshot == nil {
		a.ContextSnapshot = make(map[string]any)
	}
	a.ContextSnapshot["correlation"] = map[string]any{
		"edges_added":     len(rels),
		"primary_type":    pt,
		"strongest_score": maxScore,
	}
}

func primaryCorrelationType(rels []AlertRelationship) string {
	order := []CorrelationType{CorrSameRule, CorrSameAgent, CorrSameUser, CorrTimeBased}
	for _, want := range order {
		for _, r := range rels {
			if r.RelationType == want {
				return string(want)
			}
		}
	}
	if len(rels) > 0 {
		return string(rels[0].RelationType)
	}
	return ""
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
	incident.Severity = "medium"
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
