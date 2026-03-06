package domain

import (
	"fmt"
	"strings"
	"time"
)

// Alert represents a generated security alert derived from a DetectionResult.
// It includes MITRE ATT&CK mapping, severity escalation, and output-ready formatting.
// Supports atomic event aggregation: multiple rule matches → single aggregated alert.
type Alert struct {
	ID              string                 `json:"id"`
	RuleID          string                 `json:"rule_id"`
	RuleTitle       string                 `json:"rule_title"`
	Severity        Severity               `json:"severity"`
	Confidence      float64                `json:"confidence"`
	Timestamp       time.Time              `json:"timestamp"`
	EventID         *string                `json:"event_id,omitempty"`
	EventCategory   EventCategory          `json:"event_category"`
	Product         string                 `json:"product"`
	
	// MITRE ATT&CK
	MITRETactics    []string               `json:"mitre_tactics,omitempty"`
	MITRETechniques []string               `json:"mitre_techniques,omitempty"`
	
	// Detection details
	MatchedFields   map[string]interface{} `json:"matched_fields,omitempty"`
	MatchedSelections []string             `json:"matched_selections,omitempty"`
	
	// Event data (sanitized)
	EventData       map[string]interface{} `json:"event_data,omitempty"`
	
	// False positive indicators
	FalsePositiveRisk float64             `json:"false_positive_risk,omitempty"`
	Suppressed        bool                `json:"suppressed,omitempty"`
	
	// Aggregation (legacy)
	AggregationKey  *string               `json:"aggregation_key,omitempty"`
	RelatedAlerts   []string              `json:"related_alerts,omitempty"`

	// ==========================================================================
	// Atomic Event Aggregation Fields
	// When a single event triggers multiple rules, we aggregate into ONE alert.
	// ==========================================================================
	
	// MatchCount is the total number of rules that matched this event.
	// A value > 1 indicates multiple rule correlation.
	MatchCount      int                   `json:"match_count"`
	
	// RelatedRules contains titles of other rules that matched (excluding primary).
	// Empty if only one rule matched.
	RelatedRules    []string              `json:"related_rules,omitempty"`
	
	// RelatedRuleIDs contains IDs of other rules that matched (excluding primary).
	RelatedRuleIDs  []string              `json:"related_rule_ids,omitempty"`
	
	// CombinedConfidence is the aggregated confidence score from all matches.
	// Formula: max(confidence) + multi-match bonus (capped at 1.0)
	CombinedConfidence float64            `json:"combined_confidence"`
	
	// OriginalSeverity is the severity before any promotion.
	// Useful for audit trail when severity is promoted due to multi-match.
	OriginalSeverity Severity             `json:"original_severity,omitempty"`
	
	// SeverityPromoted indicates if severity was escalated due to multi-match.
	SeverityPromoted bool                 `json:"severity_promoted,omitempty"`
}

// NewAlert creates a new Alert from a DetectionResult.
func NewAlert(result *DetectionResult) *Alert {
	if result == nil || !result.Matched {
		return nil
	}

	alert := &Alert{
		ID:                generateAlertID(),
		RuleID:            result.RuleID(),
		RuleTitle:         result.RuleTitle(),
		Severity:          result.Rule.Severity(),
		Confidence:        result.Confidence,
		Timestamp:         result.Timestamp,
		EventID:           result.EventID(),
		EventCategory:     result.Event.Category,
		Product:           result.Event.Product,
		MITRETechniques:   result.Rule.MITRETechniques(),
		MatchedFields:     make(map[string]interface{}),
		MatchedSelections: make([]string, 0),
		EventData:         sanitizeEventData(result.Event.RawData),
		FalsePositiveRisk: 0.0,
		Suppressed:        false,
	}

	// Copy matched fields
	for k, v := range result.MatchedFields {
		alert.MatchedFields[k] = v
	}

	// Copy matched selections
	alert.MatchedSelections = append(alert.MatchedSelections, result.MatchedSelections...)

	// Extract MITRE tactics from techniques
	alert.MITRETactics = extractTacticsFromTechniques(alert.MITRETechniques)

	return alert
}

// ConfidencePercentage returns confidence as a percentage (0-100).
func (a *Alert) ConfidencePercentage() float64 {
	return a.Confidence * 100.0
}

// IsHighSeverity returns true if alert severity is HIGH or CRITICAL.
func (a *Alert) IsHighSeverity() bool {
	return a.Severity >= SeverityHigh
}

// IsHighConfidence returns true if confidence is above threshold (default 0.8).
func (a *Alert) IsHighConfidence(threshold float64) bool {
	if threshold == 0 {
		threshold = 0.8
	}
	return a.Confidence >= threshold
}

// ShouldSuppress returns true if the alert should be suppressed (false positive).
func (a *Alert) ShouldSuppress() bool {
	return a.Suppressed || a.FalsePositiveRisk > 0.7
}

// String returns a human-readable string representation of the alert.
func (a *Alert) String() string {
	eventIDStr := "N/A"
	if a.EventID != nil {
		eventIDStr = *a.EventID
	}
	return fmt.Sprintf("Alert{id=%s, rule=%q, severity=%s, confidence=%.1f%%, event_id=%s}",
		a.ID, a.RuleTitle, a.Severity.String(), a.ConfidencePercentage(), eventIDStr)
}

// generateAlertID generates a unique alert ID.
func generateAlertID() string {
	return fmt.Sprintf("alert-%d", time.Now().UnixNano())
}

// sanitizeEventData creates a sanitized copy of event data for alert output.
// Removes sensitive fields and limits size.
func sanitizeEventData(rawData map[string]interface{}) map[string]interface{} {
	if rawData == nil {
		return nil
	}

	sanitized := make(map[string]interface{})
	sensitiveFields := map[string]bool{
		"password": true,
		"passwd": true,
		"pwd": true,
		"secret": true,
		"token": true,
		"api_key": true,
		"apikey": true,
		"private_key": true,
		"privatekey": true,
	}

	for k, v := range rawData {
		keyLower := strings.ToLower(k)
		if sensitiveFields[keyLower] {
			sanitized[k] = "[REDACTED]"
			continue
		}
		sanitized[k] = v
	}

	return sanitized
}

// extractTacticsFromTechniques extracts MITRE tactics from technique IDs.
// Example: T1059.001 → "Execution"
func extractTacticsFromTechniques(techniques []string) []string {
	tacticsMap := make(map[string]bool)
	
	// MITRE ATT&CK tactic mapping (simplified - full mapping would be more comprehensive)
	tacticMap := map[string]string{
		"T1059": "Execution",
		"T1055": "Defense Evasion",
		"T1003": "Credential Access",
		"T1021": "Lateral Movement",
		"T1047": "Execution",
		"T1078": "Defense Evasion",
		"T1083": "Discovery",
		"T1105": "Command and Control",
		"T1113": "Collection",
		"T1566": "Initial Access",
	}

	for _, technique := range techniques {
		// Extract base technique ID (e.g., "T1059" from "T1059.001")
		baseID := technique
		if idx := strings.Index(technique, "."); idx > 0 {
			baseID = technique[:idx]
		}
		
		if tactic, ok := tacticMap[baseID]; ok {
			tacticsMap[tactic] = true
		}
	}

	tactics := make([]string, 0, len(tacticsMap))
	for tactic := range tacticsMap {
		tactics = append(tactics, tactic)
	}
	return tactics
}

