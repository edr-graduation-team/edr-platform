// Package scoring provides the Context-Aware Risk Scoring engine for the
// EDR platform's Phase 1 detection enhancement.
//
// The scoring pipeline intercepts a matched EventMatchResult (after Sigma rule
// evaluation) and computes a dynamic risk_score (0–100) by aggregating five
// contextual signals:
//
//  1. Base Score      — derived from the Sigma rule's static severity
//  2. Lineage Bonus  — suspicious parent→child process relationships
//  3. Privilege Bonus — elevated/SYSTEM process context
//  4. Temporal Burst  — repeated firing of the same rule category in 5 min
//  5. FP Discount     — trusted/Microsoft signature reduces final score
//  6. UEBA Bonus/Discount — behavioral baseline anomaly/normalcy adjustment
//
// The ContextSnapshot struct captures the complete forensic picture at
// scoring time, which is stored in the PostgreSQL `context_snapshot` JSONB
// column in Sprint 3.
package scoring

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/edr-platform/sigma-engine/internal/application/baselines"
	"github.com/edr-platform/sigma-engine/internal/domain"
	infracache "github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// =============================================================================
// RiskScorer Interface
// =============================================================================

// ScoringInput bundles everything the RiskScorer needs to compute a score.
// Constructed by the caller (EventLoop or a future interceptor) and passed to Score().
type ScoringInput struct {
	// MatchResult is the output of DetectAggregated — mandatory.
	MatchResult *domain.EventMatchResult

	// Event is the raw LogEvent that triggered the match — mandatory.
	Event *domain.LogEvent

	// AgentID is the UUID of the reporting agent.
	// Derived from the event payload; used as the Redis cache partition key.
	AgentID string
}

// ScoringOutput is the result of a Score() call.
type ScoringOutput struct {
	// RiskScore is the final clamped risk score (0–100).
	RiskScore int

	// FalsePositiveRisk is a probability estimate (0.0–1.0) that this alert
	// is a false positive, based on signature status and known-good paths.
	// Stored in domain.Alert.FalsePositiveRisk in Sprint 3.
	FalsePositiveRisk float64

	// Snapshot is the full forensic evidence captured at scoring time.
	// Stored as JSONB in the alerts table in Sprint 3.
	Snapshot *ContextSnapshot
}

// RiskScorer is the interface for context-aware risk scoring.
// The production implementation is *DefaultRiskScorer.
// A stub (StaticRiskScorer) is provided for unit tests that don't need Redis.
type RiskScorer interface {
	// Score evaluates the contextual risk of a matched event and returns a
	// ScoringOutput. Score is safe for concurrent use by multiple goroutines.
	// Returns an error only for fatal infrastructure failures (e.g., burst
	// counter Redis error); a partial score is returned even on soft errors.
	Score(ctx context.Context, input ScoringInput) (*ScoringOutput, error)
}

// =============================================================================
// DefaultRiskScorer — Production Implementation
// =============================================================================

// DefaultRiskScorer is the full production risk scorer.
// It requires a LineageCache (for process ancestry), a BurstTracker
// (for temporal burst detection), and a BaselineProvider (for UEBA).
type DefaultRiskScorer struct {
	lineageCache     infracache.LineageCache
	burstTracker     BurstTracker
	matrix           *SuspicionMatrix
	baselineProvider baselines.BaselineProvider
}

// NewDefaultRiskScorer constructs the production risk scorer.
// baselineProvider may be baselines.NoopBaselineProvider{} for graceful degradation.
func NewDefaultRiskScorer(
	lineageCache infracache.LineageCache,
	burstTracker BurstTracker,
	baselineProvider baselines.BaselineProvider,
) *DefaultRiskScorer {
	return &DefaultRiskScorer{
		lineageCache:     lineageCache,
		burstTracker:     burstTracker,
		matrix:           NewSuspicionMatrix(),
		baselineProvider: baselineProvider,
	}
}

// Score computes the risk score for a matched event.
//
// Formula:
//
//	risk_score = clamp(
//	    baseScore(severity, matchCount)
//	  + lineageBonus(parentChain)
//	  + privilegeBonus(eventData)
//	  + burstBonus(agentID, ruleCategory)
//	  + uebaAnomalyBonus(agentID, processName, hourOfDay)   // +15 if first-seen hour or spike
//	  - fpDiscount(signatureStatus, executablePath)
//	  - uebaNormalcyDiscount(agentID, processName, hourOfDay) // -10 if within-baseline
//	, 0, 100)
func (rs *DefaultRiskScorer) Score(ctx context.Context, input ScoringInput) (*ScoringOutput, error) {
	if input.MatchResult == nil || input.Event == nil {
		return &ScoringOutput{RiskScore: 0, Snapshot: &ContextSnapshot{}}, nil
	}

	// ── Step 1: Base Score ─────────────────────────────────────────────────────
	primary := input.MatchResult.HighestSeverityMatch()
	if primary == nil || primary.Rule == nil {
		return &ScoringOutput{RiskScore: 0, Snapshot: &ContextSnapshot{}}, nil
	}

	matchCount := input.MatchResult.MatchCount()
	baseScore := computeBaseScore(primary.Rule.Severity(), matchCount)

	// ── Step 2: Lineage Bonus ─────────────────────────────────────────────────
	pid := extractInt64(input.Event.RawData, "pid")
	lineageChain, lineageErr := rs.lineageCache.GetLineageChain(ctx, input.AgentID, pid)
	if lineageErr != nil {
		// Non-fatal: score without lineage context
		lineageErr = fmt.Errorf("lineage lookup: %w", lineageErr)
	}

	// ── DIAGNOSTIC LOG ────────────────────────────────────────────────────────
	agentPfx := input.AgentID
	if len(agentPfx) > 8 {
		agentPfx = agentPfx[:8]
	}
	if lineageErr != nil {
		logger.Debugf("[SCORER] pid=%d agent=%s chainLen=0 err=%v", pid, agentPfx, lineageErr)
	} else {
		logger.Debugf("[SCORER] pid=%d agent=%s chainLen=%d", pid, agentPfx, len(lineageChain))
	}
	// ─────────────────────────────────────────────────────────────
	lineageBonus, lineageSuspicion := rs.matrix.ComputeBonus(lineageChain)

	// ── Step 3: Privilege Bonus ───────────────────────────────────────────────
	privilegeBonus := computePrivilegeBonus(input.Event.RawData)

	// ── Step 4: Temporal Burst Bonus ─────────────────────────────────────────
	ruleCategory := categoryKey(primary.Rule)
	burstCount, burstErr := rs.burstTracker.IncrAndGet(ctx, input.AgentID, ruleCategory)
	if burstErr != nil {
		burstErr = fmt.Errorf("burst tracker: %w", burstErr)
	}
	burstBonus := computeBurstBonus(burstCount)

	// ── Step 5: False-Positive Discount ──────────────────────────────────────
	sigStatus := extractString(input.Event.RawData, "signature_status")
	executable := extractString(input.Event.RawData, "executable")
	fpDiscount := computeFPDiscount(sigStatus, executable)
	fpRisk := computeFPRisk(sigStatus, executable)

	// ── Step 5.5: UEBA Behavioral Baseline Adjustment ────────────────────────
	// Query the in-memory baseline cache to determine if this process is:
	//   a) Anomalous: running at an hour it has never been seen (+15)
	//   b) Normal: running within 1 standard deviation of its baseline (-10)
	// The adjustment is confidence-weighted: it only kicks in when the model
	// has ≥ 0.30 confidence (≈3 days of observations) to avoid false signals
	// on brand-new agents.
	processName := extractString(input.Event.RawData, "name")
	hourOfDay := time.Now().UTC().Hour()
	uebaBonus, uebaDiscount, uebaSignal, uebaErr := rs.computeUEBA(ctx, input.AgentID, processName, hourOfDay)
	if uebaErr != nil {
		uebaErr = fmt.Errorf("ueba baseline: %w", uebaErr)
	}

	// ── Step 6: Final Score ───────────────────────────────────────────────────
	raw := baseScore + lineageBonus + privilegeBonus + burstBonus + uebaBonus - fpDiscount - uebaDiscount
	finalScore := clamp(raw, 0, 100)

	breakdown := ScoreBreakdown{
		BaseScore:      baseScore,
		LineageBonus:   lineageBonus,
		PrivilegeBonus: privilegeBonus,
		BurstBonus:     burstBonus,
		FPDiscount:     fpDiscount,
		UEBABonus:      uebaBonus,
		UEBADiscount:   uebaDiscount,
		UEBASignal:     uebaSignal,
		RawScore:       raw,
		FinalScore:     finalScore,
	}

	// ── Step 7: Build Context Snapshot ───────────────────────────────────────
	snapshot := buildContextSnapshot(input, lineageChain, lineageSuspicion, burstCount, breakdown)

	// Merge non-fatal errors into the snapshot (evidence of degraded context)
	if lineageErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, lineageErr.Error())
	}
	if burstErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, burstErr.Error())
	}
	if uebaErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, uebaErr.Error())
	}

	return &ScoringOutput{
		RiskScore:         finalScore,
		FalsePositiveRisk: fpRisk,
		Snapshot:          snapshot,
	}, nil
}

// =============================================================================
// UEBA Scoring Component
// =============================================================================

// uebaSignalType labels what the UEBA component determined.
const (
	UEBASignalNone    = "none"    // baseline not available / confidence too low
	UEBASignalAnomaly = "anomaly" // process running at first-seen hour or >3σ spike
	UEBASignalNormal  = "normal"  // process running within its expected baseline
)

// computeUEBA queries the baseline provider and computes:
//   - uebaBonus (positive, applied for anomalous behavior): +15
//   - uebaDiscount (positive value, subtracted from score): +10
//   - uebaSignal: "anomaly", "normal", or "none"
func (rs *DefaultRiskScorer) computeUEBA(
	ctx context.Context,
	agentID, processName string,
	hourOfDay int,
) (bonus int, discount int, signal string, err error) {
	if rs.baselineProvider == nil || processName == "" {
		return 0, 0, UEBASignalNone, nil
	}

	baseline, err := rs.baselineProvider.Lookup(ctx, agentID, processName, hourOfDay)
	if err != nil {
		return 0, 0, UEBASignalNone, err
	}

	// No baseline yet → process is too new to profile; no signal
	if baseline == nil {
		return 0, 0, UEBASignalNone, nil
	}

	// Confidence gate: require ≥ 0.30 (≈ 3 days of observations)
	// below this threshold the EMA hasn't converged and would produce noise
	if baseline.ConfidenceScore < 0.30 {
		return 0, 0, UEBASignalNone, nil
	}

	avg := baseline.AvgExecutionsPerHour
	stddev := baseline.StddevExecutions

	// ── Anomaly detection ───────────────────────────────────────────────────
	// Case A: sample_count for this hour is 0 — process has NEVER run at this hour
	if baseline.ObservationDays == 0 || avg < 0.05 {
		return 15, 0, UEBASignalAnomaly, nil
	}

	// Case B: Execution rate spike > 3× std deviation above the mean
	// Since we observe one execution at scoring time, current_count=1.
	// We compare 1 against (avg + 3*stddev) for the spike signal.
	// When stddev is 0 (very consistent process), any execution within the
	// hour window is normal — fall through to normalcy check.
	if stddev > 0 {
		spike := avg + 3.0*stddev
		if float64(1) > spike && !math.IsInf(spike, 1) {
			return 15, 0, UEBASignalAnomaly, nil
		}
	}

	// ── Normalcy check ───────────────────────────────────────────────────────
	// Process is within its expected frequency range — grant discount.
	// Threshold: within 1 standard deviation (or avg > 0.5 when stddev == 0)
	if stddev == 0 {
		if avg >= 0.5 {
			return 0, 10, UEBASignalNormal, nil
		}
	} else {
		if math.Abs(1.0-avg) <= stddev {
			return 0, 10, UEBASignalNormal, nil
		}
	}

	return 0, 0, UEBASignalNone, nil
}

// =============================================================================
// Internal Scoring Functions
// =============================================================================

// computeBaseScore maps a Sigma severity level to an initial risk score,
// then applies a multi-rule bonus for correlated matches.
func computeBaseScore(severity domain.Severity, matchCount int) int {
	var base int
	switch severity {
	case domain.SeverityInformational:
		base = 10
	case domain.SeverityLow:
		base = 25
	case domain.SeverityMedium:
		base = 45
	case domain.SeverityHigh:
		base = 65
	case domain.SeverityCritical:
		base = 85
	default:
		base = 35 // unknown severity → default to above low
	}

	// Multi-rule correlation bonus: +5 per additional matched rule, capped at +15
	if matchCount > 1 {
		bonus := (matchCount - 1) * 5
		if bonus > 15 {
			bonus = 15
		}
		base += bonus
	}

	return base
}

// computePrivilegeBonus evaluates event-level privilege signals and returns
// a cumulative bonus to be added to the risk score.
//
// The cumulative design (additive bonuses) means a SYSTEM-level elevated
// unsigned process running under a known admin SID stacks all relevant signals.
func computePrivilegeBonus(eventData map[string]interface{}) int {
	bonus := 0

	userSID := extractString(eventData, "user_sid")
	integrityLevel := strings.ToLower(extractString(eventData, "integrity_level"))
	isElevated := extractBool(eventData, "is_elevated")
	sigStatus := strings.ToLower(extractString(eventData, "signature_status"))
	executable := strings.ToLower(extractString(eventData, "executable"))

	// SYSTEM account (Local System SID) — strongest signal
	// Legitimate processes rarely initiate suspicious activity as SYSTEM.
	if strings.HasPrefix(userSID, "S-1-5-18") { // NT AUTHORITY\SYSTEM
		bonus += 20
	} else if strings.HasSuffix(userSID, "-500") { // Built-in Administrator
		bonus += 15
	}

	// Integrity level signals
	switch integrityLevel {
	case "system":
		bonus += 15 // rare for non-service processes
	case "high":
		if isElevated {
			bonus += 10 // elevated admin doing something suspicious
		}
	}

	// Elevated token (applies even when integrity level is not "system")
	if isElevated && integrityLevel != "system" {
		bonus += 10
	}

	// Unsigned binary — strong signal for LOLBin-style abuse or malware
	if sigStatus == "unsigned" || sigStatus == "" && executable != "" {
		bonus += 15
	}

	// Cap privilege bonus to prevent over-weighting
	if bonus > 40 {
		bonus = 40
	}

	return bonus
}

// computeBurstBonus returns a bonus based on how many times the same rule
// category has fired in the last 5-minute window.
func computeBurstBonus(count int64) int {
	switch {
	case count >= 30:
		return 30
	case count >= 10:
		return 20
	case count >= 3:
		return 10
	default:
		return 0
	}
}

// computeFPDiscount returns points to subtract when the process carries
// strong trusted-binary signals (signed Microsoft binary from System32).
// The discount reduces alert priority for legitimate system activity.
func computeFPDiscount(sigStatus, executablePath string) int {
	sig := strings.ToLower(sigStatus)
	exe := strings.ToLower(executablePath)

	discount := 0

	// Microsoft-signed binary: trusted publisher
	if sig == "microsoft" {
		discount += 15

		// Additional discount for canonical system paths
		// These binaries are expected to run and are low-FP when not spawned suspiciously.
		systemPaths := []string{
			`\windows\system32\`,
			`\windows\syswow64\`,
			`\windows\sysnative\`,
		}
		for _, path := range systemPaths {
			if strings.Contains(exe, path) {
				discount += 10
				break
			}
		}
	} else if sig == "trusted" {
		// Third-party vendor with a valid signing certificate
		discount += 8
	}

	// Cap discount to prevent score from going very negative before clamp
	if discount > 30 {
		discount = 30
	}

	return discount
}

// computeFPRisk returns the false-positive probability (0.0–1.0) for the alert.
// This is stored separately from the discount for forensic transparency.
func computeFPRisk(sigStatus, executablePath string) float64 {
	sig := strings.ToLower(sigStatus)
	exe := strings.ToLower(executablePath)

	isSystemPath := strings.Contains(exe, `\windows\system32\`) ||
		strings.Contains(exe, `\windows\syswow64\`)

	switch sig {
	case "microsoft":
		if isSystemPath {
			return 0.35 // Low-ish risk: trust but verify
		}
		return 0.25
	case "trusted":
		return 0.20
	case "unsigned":
		return 0.05 // High FP risk reversed: low FP odds → high concern
	default:
		return 0.15
	}
}

// categoryKey derives a stable category identifier from a Sigma rule, used as
// the burst tracker's second key component.
// Priority: category from LogSource → product → rule ID prefix.
func categoryKey(rule *domain.SigmaRule) string {
	if rule == nil {
		return "unknown"
	}
	if rule.LogSource.Category != nil && *rule.LogSource.Category != "" {
		return strings.ToLower(*rule.LogSource.Category)
	}
	if rule.LogSource.Product != nil && *rule.LogSource.Product != "" {
		return strings.ToLower(*rule.LogSource.Product)
	}
	// Fall back to rule ID prefix (first 8 chars) for uniqueness
	if len(rule.ID) >= 8 {
		return rule.ID[:8]
	}
	return rule.ID
}

// clamp constrains an integer to [min, max].
func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// =============================================================================
// EventData Field Extractors (safe, no panics)
// =============================================================================

func extractString(data map[string]interface{}, key string) string {
	if v := resolveField(data, key); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func extractInt64(data map[string]interface{}, key string) int64 {
	if v := resolveField(data, key); v != nil {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		case uint32:
			return int64(n)
		case uint64:
			return int64(n)
		case uint:
			return int64(n)
		}
	}
	return 0
}

func extractBool(data map[string]interface{}, key string) bool {
	if v := resolveField(data, key); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
		if s, ok := v.(string); ok {
			return s == "1" || strings.EqualFold(s, "true")
		}
	}
	return false
}

// resolveField retrieves a value from a flat map[string]interface{} by checking
// the top-level key first, then falling back to the nested "data" sub-map.
//
// The Windows Agent serialises all process-specific fields inside a "data": {}
// JSON sub-object:
//
//	{ "event_type": "process", "data": { "pid": 1234, "name": "cmd.exe", ... } }
//
// The Kafka consumer flat-unmarshals the outer JSON, so these fields live at
// rawData["data"]["pid"], not rawData["pid"]. The field_mapper handles this
// transparently via sigmaToAgentData ("pid" → "data.pid"), but the risk scorer
// extractors were bypassing the mapper — always returning zero/empty for all
// process fields and causing GetLineageChain to fail at depth 0.
func resolveField(data map[string]interface{}, key string) interface{} {
	if data == nil {
		return nil
	}
	// 1. Top-level key (flat events, integration-test fixtures)
	if v, ok := data[key]; ok && v != nil {
		return v
	}
	// 2. Nested "data" sub-map (real Windows Agent events)
	if sub, ok := data["data"]; ok && sub != nil {
		if m, ok := sub.(map[string]interface{}); ok {
			if v, ok := m[key]; ok && v != nil {
				return v
			}
		}
	}
	return nil
}


// =============================================================================
// ContextSnapshot Builder
// =============================================================================

func buildContextSnapshot(
	input ScoringInput,
	chain []*infracache.ProcessLineageEntry,
	lineageSuspicion string,
	burstCount int64,
	breakdown ScoreBreakdown,
) *ContextSnapshot {
	snap := &ContextSnapshot{
		ScoredAt:         time.Now().UTC(),
		LineageSuspicion: lineageSuspicion,
		BurstCount:       int(burstCount),
		BurstWindowSec:   300, // 5-minute window
		ScoreBreakdown:   breakdown,
	}

	// Process image from event
	snap.ProcessName = extractString(input.Event.RawData, "name")
	snap.ProcessPath = extractString(input.Event.RawData, "executable")
	snap.ProcessCmdLine = extractString(input.Event.RawData, "command_line")

	// Privilege fields
	snap.UserSID = extractString(input.Event.RawData, "user_sid")
	snap.UserName = extractString(input.Event.RawData, "user_name")
	snap.IntegrityLevel = extractString(input.Event.RawData, "integrity_level")
	snap.IsElevated = extractBool(input.Event.RawData, "is_elevated")
	snap.SignatureStatus = extractString(input.Event.RawData, "signature_status")

	// Parent info from event fields (quick path)
	snap.ParentPID = extractInt64(input.Event.RawData, "ppid")
	snap.ParentName = extractString(input.Event.RawData, "parent_name")
	snap.ParentPath = extractString(input.Event.RawData, "parent_executable")

	// Populate richer lineage from cache chain
	if len(chain) > 0 {
		// chain[0] = target process; chain[1] = parent; chain[2] = grandparent
		if len(chain) >= 2 {
			snap.ParentName = chain[1].Name
			snap.ParentPath = chain[1].Executable
		}
		if len(chain) >= 3 {
			snap.GrandparentName = chain[2].Name
			snap.GrandparentPath = chain[2].Executable
		}

		// Serialise full chain for forensic replay
		snap.AncestorChain = make([]AncestorEntry, 0, len(chain))
		for _, e := range chain {
			snap.AncestorChain = append(snap.AncestorChain, AncestorEntry{
				PID:        e.PID,
				Name:       e.Name,
				Path:       e.Executable,
				UserSID:    e.UserSID,
				Integrity:  e.IntegrityLevel,
				IsElevated: e.IsElevated,
				SigStatus:  e.SignatureStatus,
				SeenAt:     e.SeenAt,
			})
		}
	}

	// Rule metadata
	primary := input.MatchResult.HighestSeverityMatch()
	if primary != nil && primary.Rule != nil {
		snap.RuleID = primary.Rule.ID
		snap.RuleTitle = primary.Rule.Title
		snap.RuleSeverity = primary.Rule.Severity().String()
		snap.RuleCategory = categoryKey(primary.Rule)
	}
	snap.MatchCount = input.MatchResult.MatchCount()
	snap.RelatedRules = input.MatchResult.RelatedRuleTitles()

	return snap
}
