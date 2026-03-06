package alert

import (
	"strings"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/google/uuid"
)

// AlertGenerator generates alerts from detection results with enrichment.
type AlertGenerator struct {
	mitreMappings map[string]string // Technique ID -> Tactic mapping
}

// NewAlertGenerator creates a new alert generator.
func NewAlertGenerator() *AlertGenerator {
	return &AlertGenerator{
		mitreMappings: initializeMitreMappings(),
	}
}

// GenerateAlert creates an alert from a detection result with enrichment.
func (ag *AlertGenerator) GenerateAlert(
	detection *domain.DetectionResult,
	event *domain.LogEvent,
) *domain.Alert {
	if detection == nil || !detection.Matched {
		return nil
	}

	// Extract MITRE ATT&CK information
	tactics := ag.extractTactics(detection.Rule.Tags)
	techniques := ag.extractTechniques(detection.Rule.Tags)

	// Calculate alert severity
	alertSeverity := ag.calculateSeverity(detection)

	// Create alert
	alert := &domain.Alert{
		ID:                ag.generateAlertID(),
		RuleID:            detection.Rule.ID,
		RuleTitle:         detection.Rule.Title,
		Severity:          alertSeverity,
		Confidence:        detection.Confidence,
		Timestamp:         time.Now(),
		EventID:           detection.Event.EventID,
		EventCategory:     detection.Event.Category,
		Product:           detection.Event.Product,
		MITRETactics:      tactics,
		MITRETechniques:   techniques,
		MatchedFields:     make(map[string]interface{}),
		MatchedSelections: detection.MatchedSelections,
		EventData:         ag.sanitizeEventData(detection.Event.RawData),
		FalsePositiveRisk: 0.0,
	}

	// Copy matched fields
	for k, v := range detection.MatchedFields {
		alert.MatchedFields[k] = v
	}

	// Add enrichment
	alert.EventData = ag.enrichEventData(event, alert.EventData)

	return alert
}

// extractTactics extracts MITRE ATT&CK tactics from rule tags.
func (ag *AlertGenerator) extractTactics(tags []string) []string {
	tacticsMap := make(map[string]bool)

	for _, tag := range tags {
		if strings.HasPrefix(tag, "attack.") {
			parts := strings.Split(tag, ".")
			if len(parts) >= 2 {
				// Extract tactic name (e.g., "initial_access" from "attack.initial_access")
				tactic := parts[1]
				// Convert to title case
				tactic = strings.Title(strings.ReplaceAll(tactic, "_", " "))
				tacticsMap[tactic] = true
			}
		}
	}

	// Also extract from techniques
	for _, tag := range tags {
		if strings.HasPrefix(tag, "attack.t") {
			techniqueID := tag[7:] // Remove "attack." prefix
			if tactic := ag.techniqueToTactic(techniqueID); tactic != "" {
				tacticsMap[tactic] = true
			}
		}
	}

	tactics := make([]string, 0, len(tacticsMap))
	for tactic := range tacticsMap {
		tactics = append(tactics, tactic)
	}

	return tactics
}

// extractTechniques extracts MITRE ATT&CK technique IDs from rule tags.
func (ag *AlertGenerator) extractTechniques(tags []string) []string {
	var techniques []string

	for _, tag := range tags {
		// Look for patterns like "attack.t1234" (ATT&CK technique IDs)
		if strings.HasPrefix(tag, "attack.t") && len(tag) > 9 {
			techniqueID := strings.ToUpper(tag[7:]) // Extract "T1234" from "attack.t1234"
			techniques = append(techniques, techniqueID)
		}
	}

	return deduplicateStrings(techniques)
}

// calculateSeverity calculates alert severity based on rule level and confidence.
func (ag *AlertGenerator) calculateSeverity(detection *domain.DetectionResult) domain.Severity {
	// Base severity from rule level
	baseSeverity := detection.Rule.Severity()

	// Adjust based on confidence
	if detection.Confidence >= 0.9 {
		// High confidence: escalate severity
		if baseSeverity < domain.SeverityCritical {
			return baseSeverity + 1
		}
	} else if detection.Confidence < 0.5 {
		// Low confidence: reduce severity
		if baseSeverity > domain.SeverityInformational {
			return baseSeverity - 1
		}
	}

	return baseSeverity
}

// enrichEventData enriches event data with additional context.
func (ag *AlertGenerator) enrichEventData(
	event *domain.LogEvent,
	eventData map[string]interface{},
) map[string]interface{} {
	if eventData == nil {
		eventData = make(map[string]interface{})
	}

	// Add parent process info if available
	if parentImage, ok := event.GetField("ParentImage"); ok {
		eventData["parent_process"] = parentImage
	}

	// Add user info if available
	if user, ok := event.GetField("User"); ok {
		eventData["user"] = user
	}

	// Add process ID if available
	if pid, ok := event.GetInt64Field("ProcessId"); ok {
		eventData["process_id"] = pid
	}

	// Add command line if available
	if cmdLine, ok := event.GetField("CommandLine"); ok {
		eventData["command_line"] = cmdLine
	}

	return eventData
}

// sanitizeEventData creates a sanitized copy of event data.
func (ag *AlertGenerator) sanitizeEventData(rawData map[string]interface{}) map[string]interface{} {
	if rawData == nil {
		return nil
	}

	sanitized := make(map[string]interface{})
	sensitiveFields := map[string]bool{
		"password": true,
		"passwd":   true,
		"pwd":      true,
		"secret":   true,
		"token":    true,
		"api_key":  true,
		"apikey":   true,
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

// techniqueToTactic maps MITRE technique ID to tactic.
func (ag *AlertGenerator) techniqueToTactic(techniqueID string) string {
	// Remove sub-technique suffix (e.g., "T1059.001" -> "T1059")
	baseID := techniqueID
	if idx := strings.Index(techniqueID, "."); idx > 0 {
		baseID = techniqueID[:idx]
	}

	if tactic, ok := ag.mitreMappings[baseID]; ok {
		return tactic
	}

	return ""
}

// generateAlertID generates a unique alert ID using UUID v4.
func (ag *AlertGenerator) generateAlertID() string {
	return "alert-" + uuid.New().String()
}

// initializeMitreMappings initializes MITRE ATT&CK technique to tactic mappings.
func initializeMitreMappings() map[string]string {
	return map[string]string{
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
		"T1071": "Command and Control",
		"T1041": "Exfiltration",
		"T1490": "Impact",
		"T1489": "Impact",
		"T1486": "Impact",
		"T1485": "Impact",
		"T1484": "Impact",
		"T1482": "Defense Evasion",
		"T1480": "Defense Evasion",
		"T1478": "Initial Access",
		"T1476": "Initial Access",
		"T1474": "Defense Evasion",
		"T1472": "Defense Evasion",
		"T1470": "Defense Evasion",
		"T1469": "Collection",
		"T1468": "Collection",
		"T1467": "Collection",
		"T1466": "Collection",
		"T1465": "Collection",
		"T1464": "Collection",
		"T1463": "Collection",
		"T1462": "Collection",
		"T1461": "Collection",
		"T1460": "Collection",
		"T1459": "Collection",
		"T1458": "Collection",
		"T1457": "Collection",
		"T1456": "Collection",
		"T1455": "Collection",
		"T1454": "Collection",
		"T1453": "Collection",
		"T1452": "Collection",
		"T1451": "Collection",
		"T1450": "Collection",
		"T1449": "Collection",
		"T1448": "Collection",
		"T1447": "Collection",
		"T1446": "Collection",
		"T1445": "Collection",
		"T1444": "Collection",
		"T1443": "Collection",
		"T1442": "Collection",
		"T1441": "Collection",
		"T1440": "Collection",
		"T1439": "Collection",
		"T1438": "Collection",
		"T1437": "Collection",
		"T1436": "Collection",
		"T1435": "Collection",
		"T1434": "Collection",
		"T1433": "Collection",
		"T1432": "Collection",
		"T1431": "Collection",
		"T1430": "Collection",
		"T1429": "Collection",
		"T1428": "Collection",
		"T1427": "Collection",
		"T1426": "Collection",
		"T1425": "Collection",
		"T1424": "Collection",
		"T1423": "Collection",
		"T1422": "Collection",
		"T1421": "Collection",
		"T1420": "Collection",
		"T1419": "Collection",
		"T1418": "Collection",
		"T1417": "Collection",
		"T1416": "Collection",
		"T1415": "Collection",
		"T1414": "Collection",
		"T1413": "Collection",
		"T1412": "Collection",
		"T1411": "Collection",
		"T1410": "Collection",
		"T1409": "Collection",
		"T1408": "Collection",
		"T1407": "Collection",
		"T1406": "Collection",
		"T1405": "Collection",
		"T1404": "Collection",
		"T1403": "Collection",
		"T1402": "Collection",
		"T1401": "Collection",
		"T1400": "Collection",
	}
}

// deduplicateStrings removes duplicate strings from a slice.
func deduplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))

	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// =============================================================================
// ATOMIC EVENT AGGREGATION - Single alert from multiple rule matches
// =============================================================================

// GenerateAggregatedAlert creates a SINGLE alert from an EventMatchResult.
// This is the key function for reducing alert fatigue.
//
// Algorithm:
//  1. Primary Rule Selection: Choose highest severity rule as the main alert
//  2. Severity Promotion: If matchCount > 3 AND severity is Low/Medium → promote to High
//  3. Combined Confidence: max(confidence) + multi-match bonus
//  4. Context Enrichment: Include all related rules that matched
//
// Returns nil if no matches exist.
func (ag *AlertGenerator) GenerateAggregatedAlert(
	matchResult *domain.EventMatchResult,
) *domain.Alert {
	if matchResult == nil || !matchResult.HasMatches() {
		return nil
	}

	// Step 1: Get the primary (highest severity) match
	primary := matchResult.HighestSeverityMatch()
	if primary == nil || primary.Rule == nil {
		return nil
	}

	// Step 2: Extract MITRE ATT&CK information from ALL matched rules
	allTechniques := matchResult.AllMITRETechniques()
	tactics := ag.extractTacticsFromTechniques(allTechniques)

	// Step 3: Calculate original severity from primary rule
	originalSeverity := primary.Rule.Severity()

	// Step 4: Determine final severity (with potential promotion)
	finalSeverity, wasPromoted := ag.calculateAggregatedSeverity(
		originalSeverity,
		matchResult.MatchCount(),
		matchResult.CombinedConfidence(),
	)

	// Step 5: Build the aggregated alert with EXPLICIT field assignment
	matchCount := matchResult.MatchCount()
	combinedConf := matchResult.CombinedConfidence()
	relatedTitles := matchResult.RelatedRuleTitles()
	relatedIDs := matchResult.RelatedRuleIDs()
	
	alert := &domain.Alert{
		ID:                 ag.generateAlertID(),
		RuleID:             primary.Rule.ID,
		RuleTitle:          primary.Rule.Title,
		Severity:           finalSeverity,
		Confidence:         primary.Confidence,
		Timestamp:          matchResult.Timestamp,
		EventID:            matchResult.Event.EventID,
		EventCategory:      matchResult.Event.Category,
		Product:            matchResult.Event.Product,
		MITRETactics:       tactics,
		MITRETechniques:    allTechniques,
		MatchedFields:      matchResult.AllMatchedFields(),
		MatchedSelections:  primary.MatchedSelections,
		EventData:          ag.sanitizeEventData(matchResult.Event.RawData),
		FalsePositiveRisk:  0.0,
		
		// Aggregation fields - EXPLICIT assignment to ensure they're set
		MatchCount:         matchCount,
		RelatedRules:       relatedTitles,
		RelatedRuleIDs:     relatedIDs,
		CombinedConfidence: combinedConf,
		OriginalSeverity:   originalSeverity,
		SeverityPromoted:   wasPromoted,
	}

	// Step 6: Add enrichment from event
	alert.EventData = ag.enrichEventData(matchResult.Event, alert.EventData)

	return alert
}

// calculateAggregatedSeverity determines the final severity for an aggregated alert.
// Implements severity promotion rules:
//   - If matchCount > 3 AND severity is Low/Medium → promote to High
//   - If matchCount > 5 AND combined confidence > 0.8 → promote to Critical
//
// Returns (finalSeverity, wasPromoted).
func (ag *AlertGenerator) calculateAggregatedSeverity(
	baseSeverity domain.Severity,
	matchCount int,
	combinedConfidence float64,
) (domain.Severity, bool) {
	finalSeverity := baseSeverity
	promoted := false

	// Rule 1: Multiple matches (>3) with Low/Medium severity → promote to High
	if matchCount > 3 && baseSeverity < domain.SeverityHigh {
		finalSeverity = domain.SeverityHigh
		promoted = true
	}

	// Rule 2: Many matches (>5) with high confidence → promote to Critical
	if matchCount > 5 && combinedConfidence > 0.8 && baseSeverity < domain.SeverityCritical {
		finalSeverity = domain.SeverityCritical
		promoted = true
	}

	// Rule 3: High confidence boost (+1 level if confidence > 0.9)
	if combinedConfidence > 0.9 && finalSeverity < domain.SeverityCritical {
		finalSeverity++
		promoted = true
	}

	return finalSeverity, promoted
}

// extractTacticsFromTechniques extracts MITRE tactics from technique IDs.
func (ag *AlertGenerator) extractTacticsFromTechniques(techniques []string) []string {
	tacticsMap := make(map[string]bool)

	for _, technique := range techniques {
		if tactic := ag.techniqueToTactic(technique); tactic != "" {
			tacticsMap[tactic] = true
		}
	}

	tactics := make([]string, 0, len(tacticsMap))
	for tactic := range tacticsMap {
		tactics = append(tactics, tactic)
	}

	return tactics
}

