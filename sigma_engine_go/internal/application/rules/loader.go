package rules

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// RuleIndex represents a complete indexed set of rules.
type RuleIndex struct {
	Rules    []*domain.SigmaRule
	Indexer  *RuleIndexer
	LoadedAt time.Time
	Errors   []ParsingError
}

// RuleLoader loads and indexes Sigma rules from directories.
type RuleLoader struct {
	parser        *RuleParser
	indexer       *RuleIndexer
	workers       int
	qualityFilter *QualityFilter
}

// NewRuleLoader creates a new rule loader.
func NewRuleLoader(strict bool) *RuleLoader {
	return &RuleLoader{
		parser:  NewRuleParser(strict),
		indexer: NewRuleIndexer(),
	}
}

// SetProductWhitelist sets the product whitelist for rule filtering.
func (rl *RuleLoader) SetProductWhitelist(products []string) {
	rl.parser.SetProductWhitelist(products)
}

// SetParallelWorkers sets the maximum number of parallel parser workers.
// This is useful for IO-bound rule parsing workloads.
func (rl *RuleLoader) SetParallelWorkers(workers int) {
	rl.workers = workers
}

// QualityFilter configures which rules are loaded by level and status (from config).
type QualityFilter struct {
	MinLevel         string   // e.g. "informational", "low", "medium", "high", "critical"
	AllowedStatus    []string // e.g. ["stable", "test", "experimental"]; empty = allow all
	SkipExperimental bool     // if true, drop rules with status experimental
}

// SetQualityFilter sets the level/status filter from config. If nil, all rules pass (permissive).
func (rl *RuleLoader) SetQualityFilter(f *QualityFilter) {
	rl.qualityFilter = f
}

// levelRank returns a numeric rank for severity level (used for min_level filtering).
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
		return 2
	}
}

// LoadRules loads all rules from a directory and builds the index.
// Uses parallel loading with streaming for efficiency.
func (rl *RuleLoader) LoadRules(ctx context.Context, dirPath string) (*RuleIndex, error) {
	start := time.Now()

	logger.Infof("Loading rules from directory: %s", dirPath)

	// Configure parallel loading
	config := DefaultConfig()
	if rl.workers > 0 {
		config.MaxWorkers = rl.workers
	}
	// Reduced logging: Only log every 500 rules instead of 100
	config.ProgressCallback = func(loaded, errors, total int) {
		if loaded%500 == 0 {
			logger.Debugf("Loading progress: %d/%d rules, %d errors", loaded, total, errors)
		}
	}

	// Parse rules in parallel
	ruleChan, errorChan, err := rl.parser.ParseDirectoryParallel(ctx, dirPath, config)
	if err != nil {
		return nil, fmt.Errorf("failed to start loading: %w", err)
	}

	// Collect rules and errors
	var rules []*domain.SigmaRule
	var errors []ParsingError

	var loadedCount int64
	var errorCount int64
	var skippedCount int64
	var skippedLowSeverity int64
	var skippedExperimental int64
	var skippedDeprecated int64

	// filterRule applies config-driven quality filtering (min_level, allowed_status, skip_experimental).
	// If qualityFilter is nil, all rules pass (permissive default).
	filterRule := func(rule *domain.SigmaRule) bool {
		if rule == nil {
			return false
		}

		q := rl.qualityFilter
		if q == nil {
			return true
		}

		// Min level: rule level must be >= config min_level (by rank)
		if strings.TrimSpace(q.MinLevel) != "" {
			if levelRank(rule.Level) < levelRank(q.MinLevel) {
				atomic.AddInt64(&skippedLowSeverity, 1)
				return false
			}
		}

		status := strings.ToLower(strings.TrimSpace(rule.Status))

		// Skip experimental if config says so
		if q.SkipExperimental && status == "experimental" {
			atomic.AddInt64(&skippedExperimental, 1)
			return false
		}

		// Allowed status list: if non-empty, rule status must be in the list (deprecated only if not in list)
		if len(q.AllowedStatus) > 0 {
			allowed := false
			for _, s := range q.AllowedStatus {
				if status == strings.ToLower(strings.TrimSpace(s)) {
					allowed = true
					break
				}
			}
			if !allowed {
				if status == "deprecated" {
					atomic.AddInt64(&skippedDeprecated, 1)
				} else if status == "" || status == "experimental" {
					atomic.AddInt64(&skippedExperimental, 1)
				} else {
					atomic.AddInt64(&skippedLowSeverity, 1)
				}
				return false
			}
		} else if status == "deprecated" {
			// No allowed_status list: still filter deprecated by default
			atomic.AddInt64(&skippedDeprecated, 1)
			return false
		}

		return true
	}

	// Collect results
	done := false
	for !done {
		select {
		case rule, ok := <-ruleChan:
			if !ok {
				ruleChan = nil
			} else {
				// Apply strict filtering
				if filterRule(rule) {
					rules = append(rules, rule)
					atomic.AddInt64(&loadedCount, 1)
				} else {
					atomic.AddInt64(&skippedCount, 1)
				}
			}

		case err, ok := <-errorChan:
			if !ok {
				errorChan = nil
			} else {
				// Check if this is a "SKIP" error (product whitelist filtering)
				// Skip errors should not be counted as real errors
				if strings.Contains(err.Err.Error(), "SKIP:") {
					// Don't log skip messages - too verbose
					continue
				}
				errors = append(errors, err)
				atomic.AddInt64(&errorCount, 1)
				if config.ErrorHandler != nil {
					config.ErrorHandler(err)
				}
			}

		case <-ctx.Done():
			return nil, ctx.Err()
		}

		if ruleChan == nil && errorChan == nil {
			done = true
		}
	}

	// Build index
	rl.indexer.BuildIndex(rules)

	elapsed := time.Since(start)

	// Report results with filtering statistics (use atomic loads for thread safety)
	finalSkippedCount := atomic.LoadInt64(&skippedCount)
	finalSkippedLowSeverity := atomic.LoadInt64(&skippedLowSeverity)
	finalSkippedExperimental := atomic.LoadInt64(&skippedExperimental)
	finalSkippedDeprecated := atomic.LoadInt64(&skippedDeprecated)
	
	// Enhanced summary log with key metrics
	logger.Infof("✓ Loaded %d high-fidelity rules (filtered %d low-quality) | Errors: %d | Time: %v",
		loadedCount, finalSkippedCount, errorCount, elapsed.Round(time.Millisecond))
	
	if finalSkippedCount > 0 {
		logger.Infof("  Filtered: %d low/med/info, %d experimental, %d deprecated",
			finalSkippedLowSeverity, finalSkippedExperimental, finalSkippedDeprecated)
	}
	
	// Only log parsing errors if there are significant issues (more than 10)
	if len(errors) > 10 {
		logger.Warnf("⚠ %d parsing errors detected (showing first 5):", len(errors))
		for i, err := range errors {
			if i >= 5 {
				break
			}
			logger.Warnf("  %s: %v", filepath.Base(err.Path), err.Err)
		}
	}

	return &RuleIndex{
		Rules:    rules,
		Indexer:  rl.indexer,
		LoadedAt: time.Now(),
		Errors:   errors,
	}, nil
}

// GetRules returns rules matching the given logsource.
func (ri *RuleIndex) GetRules(product, category, service string) []*domain.SigmaRule {
	return ri.Indexer.GetRules(product, category, service)
}

// GetRuleByID returns a rule by its ID.
func (ri *RuleIndex) GetRuleByID(ruleID string) *domain.SigmaRule {
	for _, rule := range ri.Rules {
		if rule.ID == ruleID {
			return rule
		}
	}
	return nil
}

// AddRule adds a new rule to the index.
func (ri *RuleIndex) AddRule(rule *domain.SigmaRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("rule validation failed: %w", err)
	}

	// Check for duplicate
	if existing := ri.GetRuleByID(rule.ID); existing != nil {
		return fmt.Errorf("rule already exists: %s", rule.ID)
	}

	// Add to rules
	ri.Rules = append(ri.Rules, rule)

	// Update index
	if err := ri.Indexer.AddRule(rule); err != nil {
		return err
	}

	logger.Infof("Added rule: %s", rule.Title)
	return nil
}

// RemoveRule removes a rule from the index.
func (ri *RuleIndex) RemoveRule(ruleID string) error {
	// Find and remove from rules
	idx := -1
	for i, rule := range ri.Rules {
		if rule.ID == ruleID {
			idx = i
			break
		}
	}

	if idx < 0 {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	// Remove from rules
	ri.Rules = append(ri.Rules[:idx], ri.Rules[idx+1:]...)

	// Update index
	if err := ri.Indexer.RemoveRule(ruleID); err != nil {
		return err
	}

	logger.Infof("Removed rule: %s", ruleID)
	return nil
}

// Stats returns loading and indexing statistics.
func (ri *RuleIndex) Stats() map[string]interface{} {
	indexStats := ri.Indexer.Stats()

	return map[string]interface{}{
		"total_rules":        len(ri.Rules),
		"errors":             len(ri.Errors),
		"loaded_at":          ri.LoadedAt,
		"index_build_time":   indexStats.IndexBuildTime.String(),
		"lookup_count":       indexStats.LookupCount,
		"rules_per_product":  indexStats.RulesPerProduct,
		"rules_per_category": indexStats.RulesPerCategory,
	}
}
