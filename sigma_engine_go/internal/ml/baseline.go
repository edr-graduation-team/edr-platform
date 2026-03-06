// Package ml provides machine learning for baseline learning and anomaly detection.
package ml

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/google/uuid"
)

// BaselineType defines the type of baseline.
type BaselineType string

const (
	BaselineProcess BaselineType = "process"
	BaselineNetwork BaselineType = "network"
	BaselineUser    BaselineType = "user"
	BaselineRule    BaselineType = "rule"
)

// Baseline represents learned normal behavior patterns.
type Baseline struct {
	ID          string                 `json:"id"`
	AgentID     string                 `json:"agent_id"`
	Type        BaselineType           `json:"type"`
	Patterns    map[string]interface{} `json:"patterns"`
	LearnedFrom int                    `json:"learned_from"`
	Confidence  float64                `json:"confidence"`
	LastUpdated time.Time              `json:"last_updated"`
	CreatedAt   time.Time              `json:"created_at"`
}

// ProcessPattern tracks normal process behavior.
type ProcessPattern struct {
	ProcessName   string   `json:"process_name"`
	AvgDaily      float64  `json:"avg_daily"`
	StdDev        float64  `json:"std_dev"`
	MaxObserved   int      `json:"max_observed"`
	CommonParents []string `json:"common_parents"`
	Confidence    float64  `json:"confidence"`
}

// NetworkPattern tracks normal network behavior.
type NetworkPattern struct {
	DestinationIP   string  `json:"destination_ip"`
	DestinationPort int     `json:"destination_port"`
	Protocol        string  `json:"protocol"`
	AvgBytesPerDay  float64 `json:"avg_bytes_per_day"`
	AvgConnPerDay   float64 `json:"avg_conn_per_day"`
	Confidence      float64 `json:"confidence"`
}

// AnomalyScore represents an anomaly detection result.
type AnomalyScore struct {
	ID           string    `json:"id"`
	AlertID      string    `json:"alert_id"`
	BaselineID   string    `json:"baseline_id"`
	Score        float64   `json:"score"`     // 0-100
	Deviation    float64   `json:"deviation"` // Z-score
	Confidence   float64   `json:"confidence"`
	Explanation  string    `json:"explanation"`
	CalculatedAt time.Time `json:"calculated_at"`
}

// MLModel represents a trained ML model.
type MLModel struct {
	ID           string                 `json:"id"`
	ModelType    string                 `json:"model_type"`
	AgentID      string                 `json:"agent_id"`
	ModelData    map[string]interface{} `json:"model_data"`
	Accuracy     float64                `json:"accuracy"`
	TrainedCount int                    `json:"trained_count"`
	LastTrained  time.Time              `json:"last_trained"`
	CreatedAt    time.Time              `json:"created_at"`
}

// BaselineManager manages baseline learning and anomaly detection.
type BaselineManager struct {
	mu        sync.RWMutex
	baselines map[string]*Baseline // key: agentID:type
	models    map[string]*MLModel
	scores    []AnomalyScore
}

// NewBaselineManager creates a new baseline manager.
func NewBaselineManager() *BaselineManager {
	return &BaselineManager{
		baselines: make(map[string]*Baseline),
		models:    make(map[string]*MLModel),
		scores:    make([]AnomalyScore, 0),
	}
}

// LearnBaseline learns or updates baseline from events.
func (m *BaselineManager) LearnBaseline(agentID string, bType BaselineType, events []map[string]interface{}) (*Baseline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", agentID, bType)
	baseline, exists := m.baselines[key]

	if !exists {
		baseline = &Baseline{
			ID:        uuid.New().String(),
			AgentID:   agentID,
			Type:      bType,
			Patterns:  make(map[string]interface{}),
			CreatedAt: time.Now(),
		}
	}

	// Learn patterns from events
	switch bType {
	case BaselineProcess:
		baseline.Patterns = m.learnProcessPatterns(events, baseline.Patterns)
	case BaselineNetwork:
		baseline.Patterns = m.learnNetworkPatterns(events, baseline.Patterns)
	case BaselineUser:
		baseline.Patterns = m.learnUserPatterns(events, baseline.Patterns)
	}

	// Update metadata
	baseline.LearnedFrom += len(events)
	baseline.Confidence = m.calculateConfidence(baseline.LearnedFrom)
	baseline.LastUpdated = time.Now()

	m.baselines[key] = baseline
	logger.Infof("Updated baseline %s for agent %s (learned from %d events)", bType, agentID, baseline.LearnedFrom)
	return baseline, nil
}

// learnProcessPatterns extracts process patterns from events.
func (m *BaselineManager) learnProcessPatterns(events []map[string]interface{}, existing map[string]interface{}) map[string]interface{} {
	processCounts := make(map[string]float64)
	processParents := make(map[string]map[string]int)

	// Count processes from events
	for _, event := range events {
		procName, _ := event["process_name"].(string)
		if procName == "" {
			procName, _ = event["ProcessName"].(string)
		}
		if procName != "" {
			processCounts[procName]++

			// Track parent processes
			if processParents[procName] == nil {
				processParents[procName] = make(map[string]int)
			}
			if parent, ok := event["parent_process"].(string); ok {
				processParents[procName][parent]++
			}
		}
	}

	// Create patterns
	patterns := make(map[string]interface{})
	for name, count := range processCounts {
		// Get common parents
		parents := make([]string, 0)
		if parentMap, ok := processParents[name]; ok {
			for parent := range parentMap {
				parents = append(parents, parent)
			}
		}

		patterns[name] = ProcessPattern{
			ProcessName:   name,
			AvgDaily:      count / float64(len(events)),
			StdDev:        math.Sqrt(count), // Simplified
			MaxObserved:   int(count),
			CommonParents: parents,
			Confidence:    0.8,
		}
	}

	return patterns
}

// learnNetworkPatterns extracts network patterns from events.
func (m *BaselineManager) learnNetworkPatterns(events []map[string]interface{}, existing map[string]interface{}) map[string]interface{} {
	patterns := make(map[string]interface{})

	connCounts := make(map[string]int)
	for _, event := range events {
		destIP, _ := event["destination_ip"].(string)
		if destIP == "" {
			destIP, _ = event["DestinationIp"].(string)
		}
		if destIP != "" {
			connCounts[destIP]++
		}
	}

	for ip, count := range connCounts {
		patterns[ip] = NetworkPattern{
			DestinationIP: ip,
			AvgConnPerDay: float64(count),
			Confidence:    0.7,
		}
	}

	return patterns
}

// learnUserPatterns extracts user behavior patterns.
func (m *BaselineManager) learnUserPatterns(events []map[string]interface{}, existing map[string]interface{}) map[string]interface{} {
	patterns := make(map[string]interface{})

	loginHours := make(map[int]int)
	for _, event := range events {
		if ts, ok := event["timestamp"].(time.Time); ok {
			loginHours[ts.Hour()]++
		}
	}

	patterns["login_hours"] = loginHours
	return patterns
}

// calculateConfidence calculates confidence based on sample size.
func (m *BaselineManager) calculateConfidence(sampleSize int) float64 {
	if sampleSize < 10 {
		return 0.3
	} else if sampleSize < 50 {
		return 0.5
	} else if sampleSize < 200 {
		return 0.7
	} else if sampleSize < 1000 {
		return 0.85
	}
	return 0.95
}

// GetBaseline retrieves an agent's baseline.
func (m *BaselineManager) GetBaseline(agentID string, bType BaselineType) (*Baseline, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", agentID, bType)
	baseline, exists := m.baselines[key]
	if !exists {
		return nil, fmt.Errorf("baseline not found: %s", key)
	}
	return baseline, nil
}

// ListBaselines returns all baselines.
func (m *BaselineManager) ListBaselines() []*Baseline {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Baseline, 0, len(m.baselines))
	for _, b := range m.baselines {
		list = append(list, b)
	}
	return list
}

// CalculateAnomalyScore scores an alert against baselines.
func (m *BaselineManager) CalculateAnomalyScore(alert *domain.Alert) (*AnomalyScore, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find relevant baseline
	key := fmt.Sprintf("%s:%s", "", BaselineProcess) // Would use agent ID
	baseline, exists := m.baselines[key]

	score := &AnomalyScore{
		ID:           uuid.New().String(),
		AlertID:      alert.ID,
		CalculatedAt: time.Now(),
	}

	if !exists {
		// No baseline, assign neutral score
		score.Score = 50
		score.Confidence = 0.3
		score.Explanation = "No baseline available for comparison"
		return score, nil
	}

	score.BaselineID = baseline.ID

	// Calculate deviation from baseline
	deviation := m.calculateDeviation(alert, baseline)
	score.Deviation = deviation

	// Convert Z-score to 0-100 scale
	// Z-score of 0 = 50, Z-score of 3+ = 100
	score.Score = math.Min(100, 50+(deviation*16.67))
	if score.Score < 0 {
		score.Score = 0
	}

	score.Confidence = baseline.Confidence
	score.Explanation = m.generateExplanation(deviation, baseline)

	// Store score
	m.mu.RUnlock()
	m.mu.Lock()
	m.scores = append(m.scores, *score)
	if len(m.scores) > 10000 {
		m.scores = m.scores[len(m.scores)-10000:]
	}
	m.mu.Unlock()
	m.mu.RLock()

	return score, nil
}

// calculateDeviation calculates Z-score deviation from baseline.
func (m *BaselineManager) calculateDeviation(alert *domain.Alert, baseline *Baseline) float64 {
	// Extract features from alert
	processName := ""
	if data := alert.EventData; data != nil {
		if pn, ok := data["process_name"].(string); ok {
			processName = pn
		}
	}

	if processName == "" {
		return 0 // Can't calculate
	}

	// Check if process is in baseline
	if pattern, ok := baseline.Patterns[processName]; ok {
		if pp, ok := pattern.(ProcessPattern); ok {
			// Process exists in baseline - low deviation
			if pp.Confidence > 0.8 {
				return 0.5 // Low anomaly
			}
			return 1.0
		}
	}

	// Process not in baseline - high deviation
	return 3.0 // High anomaly
}

// generateExplanation creates human-readable explanation.
func (m *BaselineManager) generateExplanation(deviation float64, baseline *Baseline) string {
	if deviation < 1 {
		return "Behavior matches baseline patterns"
	} else if deviation < 2 {
		return "Slight deviation from normal behavior"
	} else if deviation < 3 {
		return "Significant deviation from baseline"
	}
	return "Highly anomalous behavior detected"
}

// GetAnomalyScores retrieves anomaly scores for an alert.
func (m *BaselineManager) GetAnomalyScores(alertID string) []AnomalyScore {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AnomalyScore, 0)
	for _, s := range m.scores {
		if s.AlertID == alertID {
			result = append(result, s)
		}
	}
	return result
}

// GetMLStatus returns ML system status.
func (m *BaselineManager) GetMLStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgConfidence := 0.0
	for _, b := range m.baselines {
		avgConfidence += b.Confidence
	}
	if len(m.baselines) > 0 {
		avgConfidence /= float64(len(m.baselines))
	}

	return map[string]interface{}{
		"total_baselines": len(m.baselines),
		"total_models":    len(m.models),
		"total_scores":    len(m.scores),
		"avg_confidence":  avgConfidence,
	}
}

// ToJSON serializes baseline to JSON.
func (b *Baseline) ToJSON() ([]byte, error) {
	return json.Marshal(b)
}
