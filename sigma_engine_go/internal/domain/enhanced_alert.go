package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// EnhancedAlert extends the base Alert with event counting statistics,
// trend analysis, and escalation capabilities.
// This is the enriched alert format for live monitoring systems.
type EnhancedAlert struct {
	// Base alert fields (from Alert)
	AlertID           string                 `json:"alert_id"`
	RuleID            string                 `json:"rule_id"`
	RuleTitle         string                 `json:"rule_title"`
	Severity           Severity              `json:"severity"`
	Confidence         float64               `json:"confidence"`
	Timestamp          time.Time             `json:"timestamp"`
	EventID            *string               `json:"event_id,omitempty"`
	EventCategory      EventCategory         `json:"event_category"`
	Product            string                `json:"product"`
	MITRETactics       []string              `json:"mitre_tactics,omitempty"`
	MITRETechniques    []string              `json:"mitre_techniques,omitempty"`
	MatchedFields      map[string]interface{} `json:"matched_fields,omitempty"`
	MatchedSelections  []string              `json:"matched_selections,omitempty"`
	EventData          map[string]interface{} `json:"event_data,omitempty"`
	FalsePositiveRisk  float64               `json:"false_positive_risk,omitempty"`
	Suppressed          bool                  `json:"suppressed,omitempty"`

	// ✨ Enhanced fields for event counting and statistics
	EventCount         int                   `json:"event_count"`           // Total occurrences in window
	FirstSeen          time.Time             `json:"first_seen"`            // First occurrence timestamp
	LastSeen           time.Time             `json:"last_seen"`             // Last occurrence timestamp
	RatePerMinute      float64               `json:"rate_per_minute"`      // Events per minute
	CountTrend         string                `json:"count_trend"`          // "↑" (uptrend), "↓" (downtrend), "→" (stable)
	WindowSize         time.Duration         `json:"window_size_minutes"`  // Window size in minutes (for JSON)

	// Escalation fields
	ShouldEscalate     bool                  `json:"should_escalate"`      // Whether alert should be escalated
	EscalationReason   string                `json:"escalation_reason,omitempty"` // Reason for escalation

	// Source tracking
	SourceFile         string                `json:"source_file,omitempty"` // Source log file path
}

// NewEnhancedAlert creates an EnhancedAlert from a base Alert.
// Initializes enhanced fields with default values.
func NewEnhancedAlert(alert *Alert) *EnhancedAlert {
	if alert == nil {
		return nil
	}

	now := time.Now()
	return &EnhancedAlert{
		AlertID:           alert.ID,
		RuleID:            alert.RuleID,
		RuleTitle:         alert.RuleTitle,
		Severity:          alert.Severity,
		Confidence:        alert.Confidence,
		Timestamp:         alert.Timestamp,
		EventID:           alert.EventID,
		EventCategory:     alert.EventCategory,
		Product:           alert.Product,
		MITRETactics:      alert.MITRETactics,
		MITRETechniques:   alert.MITRETechniques,
		MatchedFields:     alert.MatchedFields,
		MatchedSelections: alert.MatchedSelections,
		EventData:         alert.EventData,
		FalsePositiveRisk: alert.FalsePositiveRisk,
		Suppressed:        alert.Suppressed,

		// Initialize enhanced fields
		EventCount:        1,
		FirstSeen:         now,
		LastSeen:          now,
		RatePerMinute:     0.0,
		CountTrend:        "→",
		WindowSize:        5 * time.Minute,
		ShouldEscalate:    false,
		EscalationReason:  "",
		SourceFile:        "",
	}
}

// UpdateStatistics updates the enhanced alert with event counting statistics.
func (ea *EnhancedAlert) UpdateStatistics(
	eventCount int,
	firstSeen, lastSeen time.Time,
	ratePerMinute float64,
	countTrend string,
	windowSize time.Duration,
) {
	ea.EventCount = eventCount
	ea.FirstSeen = firstSeen
	ea.LastSeen = lastSeen
	ea.RatePerMinute = ratePerMinute
	ea.CountTrend = countTrend
	ea.WindowSize = windowSize
}

// SetEscalation sets escalation flag and reason.
func (ea *EnhancedAlert) SetEscalation(shouldEscalate bool, reason string) {
	ea.ShouldEscalate = shouldEscalate
	ea.EscalationReason = reason
}

// SetSourceFile sets the source file path.
func (ea *EnhancedAlert) SetSourceFile(filePath string) {
	ea.SourceFile = filePath
}

// AdjustConfidence adjusts confidence based on event statistics.
//
// Calibration reference: NIST SP 800-61r3 §3.2.4 (Corroborating Evidence)
// Higher event counts provide stronger evidence of true positives, but with
// diminishing returns (seeing 50 events is not 5x more confident than 10).
//
// Values calibrated to preserve confidence discrimination:
//   - ×1.3 for 10+ events (moderate cluster → supplementary evidence)
//   - ×1.6 for 50+ events (significant cluster → strong reinforcement)
//   - ×1.15 for uptrend (accelerating frequency → temporal escalation)
//
// Max combined multiplier: 1.6 × 1.15 = 1.84
// This ensures medium-confidence rules (0.65) reach ~1.0 only with 50+
// events AND uptrend, preserving granularity between severity tiers.
//
// Previous values (×2.0/×1.5/×1.3) were over-aggressive:
// a medium rule (0.6) with 50 events would hit 1.0, making it
// indistinguishable from a critical rule — destroying confidence utility.
func (ea *EnhancedAlert) AdjustConfidence() {
	multiplier := 1.0

	// Event count reinforcement (NIST SP 800-61r3 — corroborating evidence)
	if ea.EventCount > 50 {
		multiplier *= 1.6 // Significant cluster: strong pattern evidence
	} else if ea.EventCount > 10 {
		multiplier *= 1.3 // Notable cluster: moderate confidence boost
	}

	// Trend multiplier (MITRE ATT&CK temporal indicator)
	// An increasing event rate suggests active, accelerating attack progression
	if ea.CountTrend == "↑" {
		multiplier *= 1.15 // Accelerating pattern: supplementary evidence
	}

	// Apply multiplier
	ea.Confidence *= multiplier

	// Clamp to [0.0, 1.0]
	if ea.Confidence > 1.0 {
		ea.Confidence = 1.0
	}
	if ea.Confidence < 0.0 {
		ea.Confidence = 0.0
	}
}

// CheckEscalation checks if alert should be escalated based on thresholds.
// Escalation conditions:
//   - eventCount > 100
//   - ratePerMinute > 10.0
//   - trend == "↑" AND eventCount > 50
//   - severity == "critical"
func (ea *EnhancedAlert) CheckEscalation(
	countThreshold int,
	rateThreshold float64,
	enableCriticalEscalation bool,
) {
	var reasons []string

	// Count threshold
	if ea.EventCount > countThreshold {
		reasons = append(reasons, "high event count")
	}

	// Rate threshold
	if ea.RatePerMinute > rateThreshold {
		reasons = append(reasons, "rapid escalation")
	}

	// Trend + count threshold
	if ea.CountTrend == "↑" && ea.EventCount > 50 {
		reasons = append(reasons, "uptrend with high count")
	}

	// Critical severity
	if enableCriticalEscalation && ea.Severity >= SeverityCritical {
		reasons = append(reasons, "critical severity")
	}

	// Set escalation
	if len(reasons) > 0 {
		ea.ShouldEscalate = true
		ea.EscalationReason = formatEscalationReason(reasons)
	} else {
		ea.ShouldEscalate = false
		ea.EscalationReason = ""
	}
}

// formatEscalationReason formats escalation reasons into a readable string.
// It is an internal helper function and should not be exported.
func formatEscalationReason(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0]
	}

	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += "; " + reasons[i]
	}
	return result
}

// MarshalJSON customizes JSON marshaling to convert WindowSize to minutes.
func (ea *EnhancedAlert) MarshalJSON() ([]byte, error) {
	// Create a temporary struct for JSON marshaling
	type Alias EnhancedAlert
	return json.Marshal(&struct {
		WindowSizeMinutes float64 `json:"window_size_minutes"`
		*Alias
	}{
		WindowSizeMinutes: ea.WindowSize.Minutes(),
		Alias:             (*Alias)(ea),
	})
}

// String returns a human-readable string representation.
func (ea *EnhancedAlert) String() string {
	return fmt.Sprintf("EnhancedAlert{id=%s, rule=%q, severity=%s, confidence=%.2f, count=%d, rate=%.2f/min, trend=%s, escalate=%v}",
		ea.AlertID, ea.RuleTitle, ea.Severity.String(), ea.Confidence, ea.EventCount, ea.RatePerMinute, ea.CountTrend, ea.ShouldEscalate)
}

