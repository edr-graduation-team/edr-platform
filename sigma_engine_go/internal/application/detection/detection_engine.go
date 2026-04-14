package detection

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/application/mapping"
	"github.com/edr-platform/sigma-engine/internal/application/rules"
	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/edr-platform/sigma-engine/pkg/ports"
)

// Note: SigmaDetectionEngine implements the ports.DetectionEngine interface
// with the following exceptions for backward compatibility:
// - LoadRules uses []*domain.SigmaRule instead of []ports.Rule
// This is intentional to maintain compatibility with existing code.

// QualityConfig controls detection quality to reduce false positives in production.
type QualityConfig struct {
	// MinConfidence is the minimum confidence required for a detection result to be returned.
	// Typical production default: 0.6 (60%).
	MinConfidence float64

	// EnableFilters controls whether "filter*" selections suppress detections even if the
	// Sigma condition did not explicitly include "and not filter".
	EnableFilters bool

	// EnableContextValidation enables extra context-based scoring (parent process/user/path).
	// When enabled, confidence may be reduced for missing/weak context.
	EnableContextValidation bool

	// Filtering enables global whitelisting to suppress common legitimate activity.
	Filtering FilteringConfig

	// RuleQuality enables rule-level load filtering.
	RuleQuality RuleQualityConfig
}

// FilteringConfig defines global whitelisting patterns.
type FilteringConfig struct {
	Enabled bool

	WhitelistedProcesses       []string
	WhitelistedUsers           []string
	WhitelistedParentProcesses []string
}

// RuleQualityConfig controls which rules are considered production-quality.
type RuleQualityConfig struct {
	MinLevel         string
	AllowedStatus    []string
	SkipExperimental bool
}

// SigmaDetectionEngine is the core detection engine that matches events against Sigma rules.
// Thread-safe and optimized for high-throughput event processing.
type SigmaDetectionEngine struct {
	rules           []*domain.SigmaRule
	ruleIndex       *rules.RuleIndexer
	selectionEval   *SelectionEvaluator
	conditionParser *rules.ConditionParser
	modifierEngine  *ModifierRegistry
	fieldMapper     *mapping.FieldMapper
	stats           *DetectionStats
	quality         QualityConfig
	mu              sync.RWMutex
}

// NewSigmaDetectionEngine creates a new detection engine.
func NewSigmaDetectionEngine(
	fieldMapper *mapping.FieldMapper,
	modifierEngine *ModifierRegistry,
	fieldCache *cache.FieldResolutionCache,
	quality QualityConfig,
) *SigmaDetectionEngine {
	conditionParser := rules.NewConditionParser()

	// Normalize defaults defensively
	if quality.MinConfidence <= 0 {
		quality.MinConfidence = 0.6
	}

	return &SigmaDetectionEngine{
		selectionEval:   NewSelectionEvaluator(fieldMapper, modifierEngine, fieldCache),
		conditionParser: conditionParser,
		modifierEngine:  modifierEngine,
		fieldMapper:     fieldMapper,
		stats:           NewDetectionStats(),
		ruleIndex:       rules.NewRuleIndexer(),
		quality:         quality,
	}
}

// LoadRules loads rules into the detection engine and builds the index.
// Quality filtering (status, level, experimental) is performed by the
// RuleLoader; this method only applies minimal structural sanity checks.
func (e *SigmaDetectionEngine) LoadRules(rules []*domain.SigmaRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Minimal structural sanity checks — quality filtering was already done by the loader
	filtered := make([]*domain.SigmaRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		// Must have detection selections to be evaluable
		if len(rule.Detection.Selections) == 0 {
			continue
		}
		filtered = append(filtered, rule)
	}

	// Store rules
	e.rules = filtered

	// Build rule index
	e.ruleIndex.BuildIndex(filtered)

	// Pre-parse all conditions for performance
	for _, rule := range filtered {
		selectionNames := rule.GetSelectionNames()
		_, err := e.conditionParser.Parse(rule.Detection.Condition, selectionNames)
		if err != nil {
			logger.Warnf("Failed to parse condition for rule %s: %v", rule.ID, err)
			// Continue loading other rules
		}
	}

	logger.Infof("Loaded %d rules into detection engine", len(filtered))
	return nil
}

// Detect evaluates an event against all loaded rules and returns matching results.
// Thread-safe and optimized for performance (< 1ms target per event).
func (e *SigmaDetectionEngine) Detect(event *domain.LogEvent) []*domain.DetectionResult {
	start := time.Now()
	e.stats.RecordEvent()

	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []*domain.DetectionResult

	// Global whitelist suppression (reduces false positives on common legitimate activity)
	if e.isWhitelistedEvent(event) {
		// Treat as processed with no detections.
		e.stats.RecordProcessingTime(time.Since(start))
		return nil
	}

	// Step 1: Get candidate rules by logsource (O(1) lookup)
	candidates := e.getCandidateRules(event)
	e.stats.RecordCandidateCount(len(candidates))

	// Step 2: Evaluate each candidate rule
	for _, rule := range candidates {
		result := e.evaluateRule(rule, event)
		if result != nil {
			results = append(results, result)
			e.stats.RecordDetection(true)
		}
		e.stats.RecordRuleEvaluation(rule.ID, result != nil)
	}

	// Step 3: Update statistics
	duration := time.Since(start)
	e.stats.RecordProcessingTime(duration)

	if len(results) > 0 {
		logger.Debugf("Event %s matched %d rules in %v", getEventIDString(event), len(results), duration)
	}

	return results
}

// DetectBatch processes multiple events and returns aggregated results.
func (e *SigmaDetectionEngine) DetectBatch(events []*domain.LogEvent) *domain.BatchDetectionResult {
	start := time.Now()

	var allResults []*domain.DetectionResult
	matchedCount := 0

	for _, event := range events {
		results := e.Detect(event)
		if len(results) > 0 {
			allResults = append(allResults, results...)
			matchedCount++
		}
	}

	elapsed := time.Since(start)
	elapsedMs := float64(elapsed.Nanoseconds()) / 1e6

	return &domain.BatchDetectionResult{
		Results:       allResults,
		TotalEvents:   len(events),
		TotalMatches:  matchedCount,
		ElapsedTimeMS: elapsedMs,
	}
}

// =============================================================================
// ATOMIC EVENT AGGREGATION
// =============================================================================

// DetectAggregated evaluates an event against ALL candidate rules and returns
// a single EventMatchResult containing ALL matches.
//
// This is the key method for reducing alert fatigue:
//   - Old behavior (Detect): 1 event + 5 matching rules → 5 separate alerts
//   - New behavior (DetectAggregated): 1 event + 5 matching rules → 1 aggregated alert
//
// Thread-safe and optimized for performance.
func (e *SigmaDetectionEngine) DetectAggregated(event *domain.LogEvent) *domain.EventMatchResult {
	start := time.Now()
	e.stats.RecordEvent()

	e.mu.RLock()
	defer e.mu.RUnlock()

	result := domain.NewEventMatchResult(event)

	// Global whitelist suppression
	if e.isWhitelistedEvent(event) {
		e.stats.RecordProcessingTime(time.Since(start))
		return result // Empty result (no matches)
	}

	// Step 1: Get ALL candidate rules by logsource (O(1) lookup)
	candidates := e.getCandidateRules(event)
	e.stats.RecordCandidateCount(len(candidates))

	// Step 2: Evaluate EVERY candidate rule and collect ALL matches
	matchCount := 0
	for _, rule := range candidates {
		match := e.evaluateRuleForAggregation(rule, event)
		if match != nil {
			result.AddMatch(match.Rule, match.Confidence, match.MatchedFields, match.MatchedSelections)
			e.stats.RecordDetection(true)
			matchCount++
		}
		e.stats.RecordRuleEvaluation(rule.ID, match != nil)
	}

	// Step 3: Record timing
	duration := time.Since(start)
	result.EvaluationTimeMS = float64(duration.Nanoseconds()) / 1e6
	e.stats.RecordProcessingTime(duration)

	// Sampled diagnostic log (every 5000 events) for debugging
	evtCount := e.stats.TotalEvents()
	if evtCount%5000 == 1 {
		cmdLine := event.GetStringField("data.command_line")
		executable := event.GetStringField("data.executable")
		name := event.GetStringField("data.name")
		logger.Infof("🔍 DIAG [evt#%d] cat=%s prod=%s svc=%s | candidates=%d matches=%d | cmdline=%q executable=%q name=%q",
			evtCount, event.Category, event.Product, event.Service,
			len(candidates), matchCount,
			truncate(cmdLine, 80), truncate(executable, 80), truncate(name, 40))
	}

	return result
}

// evaluateRuleForAggregation evaluates a single rule and returns a RuleMatch if matched.
// Similar to evaluateRule but returns RuleMatch instead of DetectionResult.
func (e *SigmaDetectionEngine) evaluateRuleForAggregation(
	rule *domain.SigmaRule,
	event *domain.LogEvent,
) *domain.RuleMatch {
	// Sampled tracing: log first candidate of every 5000th event
	evtCount := e.stats.TotalEvents()
	traceThis := (evtCount%5000 == 1)

	// Step 1: Evaluate all selections
	selectionResults := make(map[string]bool)
	matchedFields := make(map[string]interface{})

	for selectionName, selection := range rule.Detection.Selections {
		trackFields := !isFilterSelection(selectionName)
		matches := e.evaluateSelection(selection, event, matchedFields, trackFields)
		selectionResults[selectionName] = matches
	}

	if traceThis {
		// Log selection results for first candidate per sampled event
		logger.Infof("🔬 TRACE [rule=%s] selections=%v", rule.ID, selectionResults)
		// Log first few fields from event for context
		img, _ := e.getStringField(event, "Image")
		cmd, _ := e.getStringField(event, "CommandLine")
		logger.Infof("🔬 TRACE [rule=%s] Image=%q CommandLine=%q", rule.ID, img, truncate(cmd, 80))
		for selName, sel := range rule.Detection.Selections {
			for _, f := range sel.Fields {
				val, _, _ := e.fieldMapper.ResolveField(event.RawData, f.FieldName)
				logger.Infof("🔬 TRACE [rule=%s][%s] field=%s val=%q expected=%v mods=%v",
					rule.ID, selName, f.FieldName, truncate(fmt.Sprintf("%v", val), 80), f.Values, f.Modifiers)
			}
		}
	}

	// Step 2: Evaluate condition against selection results
	selectionNames := rule.GetSelectionNames()
	conditionAST, err := e.conditionParser.Parse(rule.Detection.Condition, selectionNames)
	if err != nil {
		if traceThis {
			logger.Infof("🔬 TRACE [rule=%s] DROPPED at condition parse: %v", rule.ID, err)
		}
		return nil
	}

	conditionResult := conditionAST.Evaluate(selectionResults)
	if !conditionResult {
		if traceThis {
			logger.Infof("🔬 TRACE [rule=%s] DROPPED at condition eval (false)", rule.ID)
		}
		return nil // Rule did not match
	}

	if traceThis {
		logger.Infof("🔬 TRACE [rule=%s] ✅ CONDITION MATCHED! Checking filters...", rule.ID)
	}

	// Step 3: Evaluate filters (suppression for false positive prevention)
	if e.quality.EnableFilters {
		for selectionName, selection := range rule.Detection.Selections {
			if isFilterSelection(selectionName) {
				filterMatches := e.selectionEval.Evaluate(selection, event)
				if filterMatches {
					if traceThis {
						logger.Infof("🔬 TRACE [rule=%s] DROPPED by filter: %s", rule.ID, selectionName)
					}
					return nil
				}
			}
		}
	}

	// Step 4: Calculate confidence
	confidence := e.calculateConfidence(rule, event, matchedFields)
	if confidence < e.quality.MinConfidence {
		if traceThis {
			logger.Infof("🔬 TRACE [rule=%s] DROPPED by confidence gate: %.3f < %.3f", rule.ID, confidence, e.quality.MinConfidence)
		}
		return nil // Below confidence threshold
	}

	if traceThis {
		logger.Infof("🔬 TRACE [rule=%s] ✅ ALERT EMITTED confidence=%.3f", rule.ID, confidence)
	}

	// Step 5: Return RuleMatch
	matchedSelections := getMatchedSelectionNames(selectionResults)
	return &domain.RuleMatch{
		Rule:              rule,
		Confidence:        confidence,
		MatchedFields:     matchedFields,
		MatchedSelections: matchedSelections,
	}
}

// getCandidateRules returns rules matching the event's logsource.
// Uses O(1) index lookup for performance.
func (e *SigmaDetectionEngine) getCandidateRules(event *domain.LogEvent) []*domain.SigmaRule {
	return e.ruleIndex.GetRulesStrict(
		event.Product,
		string(event.Category),
		event.Service,
	)
}

// evaluateRule evaluates a single rule against an event.
// Returns DetectionResult if rule matches, nil otherwise.
func (e *SigmaDetectionEngine) evaluateRule(
	rule *domain.SigmaRule,
	event *domain.LogEvent,
) *domain.DetectionResult {
	// Step 1: Evaluate all selections
	selectionResults := make(map[string]bool)
	matchedFields := make(map[string]interface{})

	for selectionName, selection := range rule.Detection.Selections {
		// Never let filter selections inflate matched fields (confidence) or matched_fields output.
		trackFields := true
		if isFilterSelection(selectionName) {
			trackFields = false
		}
		matches := e.evaluateSelection(selection, event, matchedFields, trackFields)
		selectionResults[selectionName] = matches
	}

	// Step 2: Evaluate condition against selection results
	selectionNames := rule.GetSelectionNames()
	conditionAST, err := e.conditionParser.Parse(rule.Detection.Condition, selectionNames)
	if err != nil {
		// Condition parse errors are common for invalid rules - don't log
		return nil
	}

	conditionResult := conditionAST.Evaluate(selectionResults)
	if !conditionResult {
		return nil // Rule did not match
	}

	// Step 3: Evaluate filters (negations) - optional suppression for false positive prevention.
	// This is enabled by config to suppress known benign patterns even if the Sigma condition
	// doesn't explicitly include "and not filter".
	if e.quality.EnableFilters {
		for selectionName, selection := range rule.Detection.Selections {
			if isFilterSelection(selectionName) {
				filterMatches := e.selectionEval.Evaluate(selection, event)
				if filterMatches {
					// Filter suppression is expected behavior - don't log
					return nil
				}
			}
		}
	}

	// Step 4: Build result
	confidence := e.calculateConfidence(rule, event, matchedFields)
	if confidence < e.quality.MinConfidence {
		// Confidence gate: keep detection quality high and reduce alert flood.
		return nil
	}
	matchedSelections := getMatchedSelectionNames(selectionResults)

	return &domain.DetectionResult{
		Rule:              rule,
		Event:             event,
		Matched:           true,
		Confidence:        confidence,
		MatchedSelections: matchedSelections,
		MatchedFields:     matchedFields,
		Timestamp:         time.Now(),
	}
}

// evaluateSelection evaluates a selection against an event.
// Returns true if all fields in selection match (AND logic).
func (e *SigmaDetectionEngine) evaluateSelection(
	selection *domain.Selection,
	event *domain.LogEvent,
	matchedFields map[string]interface{},
	trackFields bool,
) bool {
	// Use SelectionEvaluator
	matches := e.selectionEval.Evaluate(selection, event)

	if matches && trackFields {
		// Track matched fields for result
		for _, field := range selection.Fields {
			value, _, err := e.fieldMapper.ResolveField(event.RawData, field.FieldName)
			if err == nil && value != nil {
				matchedFields[field.FieldName] = value
			}
		}
	}

	return matches
}

// calculateConfidence calculates detection confidence based on rule level and matched fields.
func (e *SigmaDetectionEngine) calculateConfidence(
	rule *domain.SigmaRule,
	event *domain.LogEvent,
	matchedFields map[string]interface{},
) float64 {
	// Base confidence from rule level
	baseConf := getLevelConfidence(rule.Level)

	// Field match factor: more fields = higher confidence.
	// FIX ISSUE-06: Count UNIQUE field names across non-filter selections.
	// Previously, totalFields summed across all selections including duplicates,
	// which deflated the ratio when multiple selections referenced the same field
	// (e.g., both 'selection_process' and 'selection_alt' check CommandLine).
	// Now we deduplicate via map key to get the true unique field count.
	fieldCount := len(matchedFields)
	uniqueFields := make(map[string]bool)
	for selName, selection := range rule.Detection.Selections {
		if !isFilterSelection(selName) {
			for _, f := range selection.Fields {
				uniqueFields[f.FieldName] = true
			}
		}
	}
	totalFields := len(uniqueFields)

	fieldFactor := 1.0
	if totalFields > 0 {
		fieldFactor = float64(fieldCount) / float64(totalFields)
	}

	// Context score (optional)
	contextScore := 1.0
	if e.quality.EnableContextValidation {
		contextScore = e.validateContext(rule, event)
	}

	// Calculate final confidence
	confidence := baseConf * fieldFactor * contextScore

	// Clamp to [0.0, 1.0]
	confidence = math.Min(confidence, 1.0)
	if confidence < 0.0 {
		confidence = 0.0
	}

	return confidence
}

// validateContext scores whether this event has sufficient context for this rule.
// This is intentionally conservative: it reduces confidence for missing key context
// but should not outright block detections (confidence gate handles final decision).
func (e *SigmaDetectionEngine) validateContext(rule *domain.SigmaRule, event *domain.LogEvent) float64 {
	score := 1.0

	// If the rule references parent process fields but the event lacks them, reduce confidence.
	needsParent := ruleReferencesField(rule, "ParentImage") || ruleReferencesField(rule, "ParentCommandLine")
	if needsParent {
		if _, ok := e.getStringField(event, "ParentImage"); !ok {
			score *= 0.8
		}
	}

	// If the rule references command line but event lacks it, reduce confidence.
	needsCmd := ruleReferencesField(rule, "CommandLine")
	if needsCmd {
		if _, ok := e.getStringField(event, "CommandLine"); !ok {
			score *= 0.85
		}
	}

	// If user context is missing, reduce a bit (many benign Windows events are SYSTEM).
	needsUser := ruleReferencesField(rule, "User")
	if needsUser {
		if _, ok := e.getStringField(event, "User"); !ok {
			score *= 0.9
		}
	}

	return score
}

func ruleReferencesField(rule *domain.SigmaRule, fieldName string) bool {
	if rule == nil {
		return false
	}
	for _, sel := range rule.Detection.Selections {
		if sel == nil {
			continue
		}
		for _, f := range sel.Fields {
			if strings.EqualFold(f.FieldName, fieldName) {
				return true
			}
		}
	}
	return false
}

func (e *SigmaDetectionEngine) getStringField(event *domain.LogEvent, fieldName string) (string, bool) {
	if event == nil {
		return "", false
	}
	v, _, err := e.fieldMapper.ResolveField(event.RawData, fieldName)
	if err != nil || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if ok && strings.TrimSpace(s) != "" {
		return s, true
	}
	// Fallback: stringify
	str := strings.TrimSpace(toString(v))
	if str == "" {
		return "", false
	}
	return str, true
}

// isWhitelistedEvent returns true if the event matches any configured whitelist rule.
// Whitelisting is evaluated before rule matching to reduce false positives and CPU load.
//
// NOTE (RC-2 fix): User and ParentImage whitelisting have been removed.
// - User whitelist (e.g. "NT AUTHORITY\SYSTEM") was far too broad: it silently
//   dropped the vast majority of Windows process events before Sigma rules could
//   evaluate them, creating massive detection blind spots.
// - ParentImage whitelist (e.g. explorer.exe, services.exe) suppressed events
//   for legitimate attacker parent processes (many tools are spawned by explorer
//   or services). Sigma rules themselves contain fine-grained filter selections
//   that handle false positive suppression per-rule.
func (e *SigmaDetectionEngine) isWhitelistedEvent(event *domain.LogEvent) bool {
	if !e.quality.Filtering.Enabled || event == nil {
		return false
	}

	// Process image whitelist only (exact binary paths like svchost.exe, lsass.exe)
	if image, ok := e.getStringField(event, "Image"); ok {
		if matchAnyPathPattern(image, e.quality.Filtering.WhitelistedProcesses) {
			return true
		}
	}

	return false
}

// matchAnyPathPattern returns true if `path` matches any of the whitelist
// patterns. This is a pure-string implementation that works identically on
// Linux (Docker) and Windows, unlike filepath.Match/filepath.Clean which
// interpret backslashes differently across platforms.
//
// Supported pattern syntax:
//   - Leading `*\` or `*\\` — suffix match ("ends with").
//   - Trailing `*`           — prefix match ("starts with").
//   - No wildcards            — exact match.
func matchAnyPathPattern(path string, patterns []string) bool {
	if path == "" || len(patterns) == 0 {
		return false
	}
	// Normalize: lowercase + unify separators to backslash for Windows paths
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", "\\"))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pLower := strings.ToLower(strings.ReplaceAll(p, "/", "\\"))

		// Wildcard at both ends: *text* — contains
		if strings.HasPrefix(pLower, "*") && strings.HasSuffix(pLower, "*") {
			core := strings.Trim(pLower, "*")
			if core != "" && strings.Contains(normalized, core) {
				return true
			}
			continue
		}

		// Leading wildcard: *\thing.exe — suffix / ends-with
		if strings.HasPrefix(pLower, "*") {
			suffix := pLower[1:] // strip leading *
			if strings.HasSuffix(normalized, suffix) {
				return true
			}
			continue
		}

		// Trailing wildcard: C:\Program Files\Microsoft* — prefix / starts-with
		if strings.HasSuffix(pLower, "*") {
			prefix := pLower[:len(pLower)-1] // strip trailing *
			if strings.HasPrefix(normalized, prefix) {
				return true
			}
			continue
		}

		// Exact match (no wildcards)
		if normalized == pLower {
			return true
		}
	}
	return false
}

func levelRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	case "informational", "info":
		return 0
	default:
		// Unknown levels are treated as medium-ish to avoid dropping potentially relevant rules.
		return 2
	}
}

// getLevelConfidence maps Sigma rule level to base detection confidence.
//
// Calibration methodology: Bayesian prior probability P(attack | rule_match).
// Values derived from the SigmaHQ rule taxonomy and empirical FPR data:
//   - critical: Near-zero FP rate rules → 0.95 (strong prior, but not 1.0
//     per Cromwell's Rule: no prior probability should be 0 or 1 because
//     no update by Bayes' theorem can then revise it)
//   - high:     Confirmed low-FP rules → 0.85
//   - medium:   Moderate FP potential  → 0.65
//   - low:      High FP potential      → 0.45
//   - informational: Observational     → 0.25
//
// Reference: SigmaHQ Rule Specification v2, Bayesian epistemology (Cromwell's Rule)
func getLevelConfidence(level string) float64 {
	switch level {
	case "critical":
		return 0.95
	case "high":
		return 0.85
	case "medium":
		return 0.65
	case "low":
		return 0.45
	case "informational":
		return 0.25
	default:
		return 0.50
	}
}

// getMatchedSelectionNames returns names of selections that matched.
func getMatchedSelectionNames(selectionResults map[string]bool) []string {
	var matched []string
	for name, isMatched := range selectionResults {
		if isMatched {
			matched = append(matched, name)
		}
	}
	return matched
}

// isFilterSelection checks if a selection name indicates a filter (negation).
func isFilterSelection(name string) bool {
	return len(name) >= 6 && name[:6] == "filter"
}

// getEventIDString returns event ID as string.
func getEventIDString(event *domain.LogEvent) string {
	if event.EventID != nil {
		return *event.EventID
	}
	return "unknown"
}

// Stats returns detection statistics snapshot.
func (e *SigmaDetectionEngine) Stats() *DetectionStatsSnapshot {
	return e.stats.Snapshot()
}

// ResetStats resets all statistics.
func (e *SigmaDetectionEngine) ResetStats() {
	e.stats.Reset()
}

// =============================================================================
// PORTS INTERFACE IMPLEMENTATION
// =============================================================================

// Match implements ports.DetectionEngine.Match
// Evaluates a single event against all loaded rules and returns MatchResult.
func (e *SigmaDetectionEngine) Match(ctx context.Context, event ports.Event) (*ports.MatchResult, error) {
	start := time.Now()

	// Convert ports.Event to domain.LogEvent
	logEvent, ok := event.(*domain.LogEvent)
	if !ok {
		return nil, nil // Incompatible event type
	}

	// Use existing detection method
	results := e.Detect(logEvent)

	// Convert to ports.MatchResult
	matches := make([]ports.RuleMatch, 0, len(results))
	for _, r := range results {
		matches = append(matches, ports.RuleMatch{
			RuleID:          r.Rule.ID,
			RuleTitle:       r.Rule.Title,
			Severity:        r.Rule.Level,
			Confidence:      r.Confidence,
			MatchedFields:   r.MatchedFields,
			MITRETechniques: r.Rule.MITRETechniques(),
			Tags:            r.Rule.Tags,
		})
	}

	return &ports.MatchResult{
		EventID:        logEvent.ComputeHash(),
		Matched:        len(matches) > 0,
		MatchCount:     len(matches),
		Matches:        matches,
		EvaluatedRules: e.RuleCount(),
		LatencyMs:      float64(time.Since(start).Nanoseconds()) / 1e6,
		Timestamp:      time.Now(),
	}, nil
}

// MatchBatch implements ports.DetectionEngine.MatchBatch
// Processes multiple events efficiently.
func (e *SigmaDetectionEngine) MatchBatch(ctx context.Context, events []ports.Event) (*ports.BatchMatchResult, error) {
	start := time.Now()

	results := make([]ports.MatchResult, 0, len(events))
	matchedEvents := 0
	totalMatches := 0

	var minLatency, maxLatency, totalLatency float64
	minLatency = math.MaxFloat64

	for _, event := range events {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := e.Match(ctx, event)
		if err != nil {
			continue
		}
		if result == nil {
			continue
		}

		results = append(results, *result)

		if result.Matched {
			matchedEvents++
			totalMatches += result.MatchCount
		}

		totalLatency += result.LatencyMs
		if result.LatencyMs < minLatency {
			minLatency = result.LatencyMs
		}
		if result.LatencyMs > maxLatency {
			maxLatency = result.LatencyMs
		}
	}

	elapsed := time.Since(start)
	elapsedMs := float64(elapsed.Nanoseconds()) / 1e6

	avgLatency := 0.0
	if len(results) > 0 {
		avgLatency = totalLatency / float64(len(results))
	}
	if minLatency == math.MaxFloat64 {
		minLatency = 0
	}

	throughput := 0.0
	if elapsed.Seconds() > 0 {
		throughput = float64(len(events)) / elapsed.Seconds()
	}

	return &ports.BatchMatchResult{
		TotalEvents:   len(events),
		MatchedEvents: matchedEvents,
		TotalMatches:  totalMatches,
		Results:       results,
		Stats: ports.BatchStats{
			TotalTimeMs:   elapsedMs,
			AvgLatencyMs:  avgLatency,
			MaxLatencyMs:  maxLatency,
			MinLatencyMs:  minLatency,
			ThroughputEPS: throughput,
		},
	}, nil
}

// AddRules implements ports.DetectionEngine.AddRules
// Adds rules without replacing existing ones.
func (e *SigmaDetectionEngine) AddRules(ctx context.Context, newRules []ports.Rule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Convert ports.Rule to domain.SigmaRule and add
	for _, r := range newRules {
		domainRule, ok := r.(*domain.SigmaRule)
		if !ok {
			continue // Skip incompatible rule types
		}

		// Check for duplicate
		for _, existing := range e.rules {
			if existing.ID == domainRule.ID {
				return nil // Already exists, skip
			}
		}

		e.rules = append(e.rules, domainRule)
		e.ruleIndex.AddRule(domainRule)
	}

	logger.Infof("Added %d rules, total now: %d", len(newRules), len(e.rules))
	return nil
}

// RemoveRule implements ports.DetectionEngine.RemoveRule
// Removes a single rule by ID.
func (e *SigmaDetectionEngine) RemoveRule(ctx context.Context, ruleID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	found := false
	newRules := make([]*domain.SigmaRule, 0, len(e.rules))
	for _, rule := range e.rules {
		if rule.ID == ruleID {
			found = true
			continue
		}
		newRules = append(newRules, rule)
	}

	if !found {
		return nil // Rule not found, no error
	}

	e.rules = newRules
	e.ruleIndex.RemoveRule(ruleID)

	logger.Infof("Removed rule %s, total now: %d", ruleID, len(e.rules))
	return nil
}

// GetRules implements ports.DetectionEngine.GetRules
// Returns rules matching the filter.
func (e *SigmaDetectionEngine) GetRules(ctx context.Context, filter ports.RuleFilter) ([]ports.Rule, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]ports.Rule, 0)

	for _, rule := range e.rules {
		// Apply filters
		if filter.Product != "" && (rule.LogSource.Product == nil || *rule.LogSource.Product != filter.Product) {
			continue
		}
		if filter.Category != "" && (rule.LogSource.Category == nil || *rule.LogSource.Category != filter.Category) {
			continue
		}
		if filter.Level != "" && rule.Level != filter.Level {
			continue
		}
		if filter.Status != "" && rule.Status != filter.Status {
			continue
		}

		// Check IDs filter
		if len(filter.IDs) > 0 {
			found := false
			for _, id := range filter.IDs {
				if rule.ID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check tags filter (rule must have ALL specified tags)
		if len(filter.Tags) > 0 {
			hasAllTags := true
			for _, tag := range filter.Tags {
				found := false
				for _, ruleTag := range rule.Tags {
					if ruleTag == tag {
						found = true
						break
					}
				}
				if !found {
					hasAllTags = false
					break
				}
			}
			if !hasAllTags {
				continue
			}
		}

		result = append(result, rule)
	}

	return result, nil
}

// RuleCount implements ports.DetectionEngine.RuleCount
func (e *SigmaDetectionEngine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}

// Health implements ports.DetectionEngine.Health
// Returns engine health status.
func (e *SigmaDetectionEngine) Health() *ports.EngineHealth {
	status := ports.HealthStatusHealthy
	// Degrade health if we've had panics (panic count tracked in processor stats)
	// For now, just report healthy since we don't track panics at engine level

	return &ports.EngineHealth{
		Status:    status,
		IsHealthy: status == ports.HealthStatusHealthy,
		CheckedAt: time.Now(),
	}
}

// Shutdown implements ports.DetectionEngine.Shutdown
// Gracefully stops the engine.
func (e *SigmaDetectionEngine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clear rules to prevent new detections
	e.rules = nil
	e.ruleIndex = rules.NewRuleIndexer()

	logger.Info("Detection engine shutdown complete")
	return nil
}

// PortsStats returns stats in the ports.EngineStats format.
func (e *SigmaDetectionEngine) PortsStats() *ports.EngineStats {
	snapshot := e.stats.Snapshot()

	return &ports.EngineStats{
		LoadedRules:     e.RuleCount(),
		EventsProcessed: snapshot.TotalEvents,
		DetectionsFound: snapshot.TotalDetections,
		AvgLatencyMs:    float64(snapshot.AvgProcessingTime.Nanoseconds()) / 1e6,
		LastUpdated:     time.Now(),
	}
}

// truncate shortens a string to maxLen characters for log readability.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
