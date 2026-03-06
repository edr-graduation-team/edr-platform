package domain

import (
	"fmt"
	"time"
)

// DetectionResult represents the result of evaluating a rule against an event.
type DetectionResult struct {
	Rule      *SigmaRule           `json:"rule"`
	Event     *LogEvent            `json:"event"`
	Matched   bool                 `json:"matched"`
	Confidence float64             `json:"confidence"`

	MatchedSelections []string               `json:"matched_selections,omitempty"`
	MatchedFields     map[string]interface{} `json:"matched_fields,omitempty"`
	UnmatchedFields   map[string]string      `json:"unmatched_fields,omitempty"`

	EvaluationTimeMS float64   `json:"evaluation_time_ms"`
	Timestamp        time.Time `json:"timestamp"`
}

// NewDetectionResult creates a new DetectionResult for a rule and event.
func NewDetectionResult(rule *SigmaRule, event *LogEvent) *DetectionResult {
	return &DetectionResult{
		Rule:            rule,
		Event:           event,
		Matched:         false,
		Confidence:      0.0,
		MatchedSelections: make([]string, 0),
		MatchedFields:     make(map[string]interface{}),
		UnmatchedFields:   make(map[string]string),
		Timestamp:         time.Now(),
	}
}

// RuleID returns the rule ID.
func (dr *DetectionResult) RuleID() string {
	if dr.Rule != nil {
		return dr.Rule.ID
	}
	return ""
}

// RuleTitle returns the rule title.
func (dr *DetectionResult) RuleTitle() string {
	if dr.Rule != nil {
		return dr.Rule.Title
	}
	return ""
}

// EventID returns the event ID.
func (dr *DetectionResult) EventID() *string {
	if dr.Event != nil {
		return dr.Event.EventID
	}
	return nil
}

// IsHighConfidence checks if the match has high confidence above the threshold.
func (dr *DetectionResult) IsHighConfidence(threshold float64) bool {
	return dr.Matched && dr.Confidence >= threshold
}

// AddMatchedSelection adds a matched selection name.
func (dr *DetectionResult) AddMatchedSelection(selectionName string) {
	for _, name := range dr.MatchedSelections {
		if name == selectionName {
			return
		}
	}
	dr.MatchedSelections = append(dr.MatchedSelections, selectionName)
}

// AddMatchedField adds a matched field and its value.
func (dr *DetectionResult) AddMatchedField(fieldName string, value interface{}) {
	dr.MatchedFields[fieldName] = value
}

// AddUnmatchedField adds an unmatched field with a reason for debugging.
func (dr *DetectionResult) AddUnmatchedField(fieldName, reason string) {
	dr.UnmatchedFields[fieldName] = reason
}

// CalculateConfidence computes the confidence score based on match quality.
func (dr *DetectionResult) CalculateConfidence() float64 {
	if !dr.Matched {
		dr.Confidence = 0.0
		return 0.0
	}

	baseConfidence := 0.7
	fieldBoost := min(0.2, float64(len(dr.MatchedFields))*0.02)
	selectionBoost := min(0.1, float64(len(dr.MatchedSelections))*0.03)

	dr.Confidence = min(1.0, baseConfidence+fieldBoost+selectionBoost)
	return dr.Confidence
}

// Summary returns a human-readable summary of the detection result.
func (dr *DetectionResult) Summary() string {
	if dr.Matched {
		return fmt.Sprintf("MATCH: %s (confidence: %.1f%%, fields: %d)",
			dr.RuleTitle(), dr.Confidence*100, len(dr.MatchedFields))
	}
	return fmt.Sprintf("NO MATCH: %s", dr.RuleTitle())
}

// BatchDetectionResult represents aggregated results from batch detection.
type BatchDetectionResult struct {
	Results        []*DetectionResult `json:"results"`
	TotalEvents    int               `json:"total_events"`
	TotalMatches   int               `json:"total_matches"`
	ElapsedTimeMS  float64           `json:"elapsed_time_ms"`
}

// NewBatchDetectionResult creates a new BatchDetectionResult.
func NewBatchDetectionResult() *BatchDetectionResult {
	return &BatchDetectionResult{
		Results: make([]*DetectionResult, 0),
	}
}

// AddResult adds a detection result to the batch.
func (bdr *BatchDetectionResult) AddResult(result *DetectionResult) {
	bdr.Results = append(bdr.Results, result)
	if result.Matched {
		bdr.TotalMatches++
	}
}

// GetMatches returns only matched results.
func (bdr *BatchDetectionResult) GetMatches() []*DetectionResult {
	matches := make([]*DetectionResult, 0)
	for _, result := range bdr.Results {
		if result.Matched {
			matches = append(matches, result)
		}
	}
	return matches
}

// MatchRate returns the percentage of events that triggered alerts.
func (bdr *BatchDetectionResult) MatchRate() float64 {
	if bdr.TotalEvents == 0 {
		return 0.0
	}
	return float64(bdr.TotalMatches) / float64(bdr.TotalEvents)
}

// EventsPerSecond calculates processing speed.
func (bdr *BatchDetectionResult) EventsPerSecond() float64 {
	if bdr.ElapsedTimeMS == 0 {
		return 0.0
	}
	return float64(bdr.TotalEvents) / (bdr.ElapsedTimeMS / 1000.0)
}

// =============================================================================
// EVENT MATCH RESULT - Aggregated result for a single event
// =============================================================================

// RuleMatch represents a single rule match with its confidence and matched fields.
type RuleMatch struct {
	Rule              *SigmaRule             `json:"rule"`
	Confidence        float64                `json:"confidence"`
	MatchedFields     map[string]interface{} `json:"matched_fields,omitempty"`
	MatchedSelections []string               `json:"matched_selections,omitempty"`
}

// EventMatchResult aggregates ALL rule matches for a single LogEvent.
// This enables atomic event aggregation - one event produces one aggregated alert.
type EventMatchResult struct {
	Event           *LogEvent    `json:"event"`
	Matches         []*RuleMatch `json:"matches"`
	Timestamp       time.Time    `json:"timestamp"`
	EvaluationTimeMS float64     `json:"evaluation_time_ms"`
}

// NewEventMatchResult creates a new EventMatchResult for an event.
func NewEventMatchResult(event *LogEvent) *EventMatchResult {
	return &EventMatchResult{
		Event:     event,
		Matches:   make([]*RuleMatch, 0),
		Timestamp: time.Now(),
	}
}

// AddMatch adds a rule match to the result.
func (emr *EventMatchResult) AddMatch(rule *SigmaRule, confidence float64, matchedFields map[string]interface{}, matchedSelections []string) {
	match := &RuleMatch{
		Rule:              rule,
		Confidence:        confidence,
		MatchedFields:     matchedFields,
		MatchedSelections: matchedSelections,
	}
	emr.Matches = append(emr.Matches, match)
}

// MatchCount returns the number of rules that matched this event.
func (emr *EventMatchResult) MatchCount() int {
	return len(emr.Matches)
}

// HasMatches returns true if at least one rule matched.
func (emr *EventMatchResult) HasMatches() bool {
	return len(emr.Matches) > 0
}

// HighestSeverityMatch returns the match with the highest severity rule.
// Used for primary rule selection in aggregated alerts.
func (emr *EventMatchResult) HighestSeverityMatch() *RuleMatch {
	if len(emr.Matches) == 0 {
		return nil
	}

	highest := emr.Matches[0]
	highestRank := severityRank(highest.Rule.Level)

	for _, match := range emr.Matches[1:] {
		rank := severityRank(match.Rule.Level)
		if rank > highestRank {
			highest = match
			highestRank = rank
		} else if rank == highestRank && match.Confidence > highest.Confidence {
			// Same severity: prefer higher confidence
			highest = match
		}
	}

	return highest
}

// MaxConfidence returns the highest confidence among all matches.
func (emr *EventMatchResult) MaxConfidence() float64 {
	if len(emr.Matches) == 0 {
		return 0.0
	}

	maxConf := 0.0
	for _, match := range emr.Matches {
		if match.Confidence > maxConf {
			maxConf = match.Confidence
		}
	}
	return maxConf
}

// AverageConfidence returns the average confidence across all matches.
func (emr *EventMatchResult) AverageConfidence() float64 {
	if len(emr.Matches) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, match := range emr.Matches {
		sum += match.Confidence
	}
	return sum / float64(len(emr.Matches))
}

// CombinedConfidence calculates aggregated confidence score.
// Formula: max(confidence) + bonus for multiple matches (capped at 1.0)
func (emr *EventMatchResult) CombinedConfidence() float64 {
	if len(emr.Matches) == 0 {
		return 0.0
	}

	maxConf := emr.MaxConfidence()
	
	// Multi-match bonus: +0.05 per additional match, max +0.2
	multiMatchBonus := float64(len(emr.Matches)-1) * 0.05
	if multiMatchBonus > 0.2 {
		multiMatchBonus = 0.2
	}

	combined := maxConf + multiMatchBonus
	if combined > 1.0 {
		combined = 1.0
	}

	return combined
}

// RelatedRuleIDs returns IDs of all matched rules except the primary (highest severity).
func (emr *EventMatchResult) RelatedRuleIDs() []string {
	if len(emr.Matches) <= 1 {
		return nil
	}

	primary := emr.HighestSeverityMatch()
	related := make([]string, 0, len(emr.Matches)-1)

	for _, match := range emr.Matches {
		if match.Rule.ID != primary.Rule.ID {
			related = append(related, match.Rule.ID)
		}
	}

	return related
}

// RelatedRuleTitles returns titles of all matched rules except the primary.
func (emr *EventMatchResult) RelatedRuleTitles() []string {
	if len(emr.Matches) <= 1 {
		return nil
	}

	primary := emr.HighestSeverityMatch()
	related := make([]string, 0, len(emr.Matches)-1)

	for _, match := range emr.Matches {
		if match.Rule.ID != primary.Rule.ID {
			related = append(related, match.Rule.Title)
		}
	}

	return related
}

// AllMITRETechniques returns deduplicated MITRE techniques from all matched rules.
func (emr *EventMatchResult) AllMITRETechniques() []string {
	seen := make(map[string]bool)
	techniques := make([]string, 0)

	for _, match := range emr.Matches {
		for _, tech := range match.Rule.MITRETechniques() {
			if !seen[tech] {
				seen[tech] = true
				techniques = append(techniques, tech)
			}
		}
	}

	return techniques
}

// AllMatchedFields returns merged matched fields from all matches.
func (emr *EventMatchResult) AllMatchedFields() map[string]interface{} {
	merged := make(map[string]interface{})
	for _, match := range emr.Matches {
		for k, v := range match.MatchedFields {
			merged[k] = v
		}
	}
	return merged
}

// severityRank returns numeric rank for severity comparison.
// Higher rank = more severe.
func severityRank(level string) int {
	switch level {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "informational":
		return 1
	default:
		return 0
	}
}

