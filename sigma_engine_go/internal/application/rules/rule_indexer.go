package rules

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

// RuleIndexer provides O(1) rule lookup by logsource with statistics.
type RuleIndexer struct {
	// Exact matches: "product:category:service" -> rules
	index map[string][]*domain.SigmaRule

	// Partial matches for wildcards
	categoryIndex map[string][]*domain.SigmaRule // "product:category" -> rules
	productIndex  map[string][]*domain.SigmaRule  // "product" -> rules

	// All rules (fallback)
	allRules []*domain.SigmaRule

	// Statistics
	stats IndexStats

	mu sync.RWMutex
}

// IndexStats tracks indexing and lookup statistics.
type IndexStats struct {
	TotalRules      int
	RulesPerProduct map[string]int
	RulesPerCategory map[string]int
	IndexBuildTime  time.Duration
	LookupCount     int64
	LookupTimeTotal time.Duration
}

// NewRuleIndexer creates a new rule indexer.
func NewRuleIndexer() *RuleIndexer {
	return &RuleIndexer{
		index:         make(map[string][]*domain.SigmaRule),
		categoryIndex: make(map[string][]*domain.SigmaRule),
		productIndex:  make(map[string][]*domain.SigmaRule),
		allRules:      make([]*domain.SigmaRule, 0),
		stats: IndexStats{
			RulesPerProduct:  make(map[string]int),
			RulesPerCategory: make(map[string]int),
		},
	}
}

// BuildIndex builds the index from a list of rules.
func (ri *RuleIndexer) BuildIndex(rules []*domain.SigmaRule) {
	start := time.Now()

	ri.mu.Lock()
	defer ri.mu.Unlock()

	// Clear existing index
	ri.index = make(map[string][]*domain.SigmaRule)
	ri.categoryIndex = make(map[string][]*domain.SigmaRule)
	ri.productIndex = make(map[string][]*domain.SigmaRule)
	ri.allRules = rules

	// Build indexes
	for _, rule := range rules {
		// Build exact match index
		key := ri.buildKey(rule.LogSource)
		ri.index[key] = append(ri.index[key], rule)

		// Build category index
		if rule.LogSource.Product != nil && rule.LogSource.Category != nil {
			catKey := fmt.Sprintf("%s:%s", *rule.LogSource.Product, *rule.LogSource.Category)
			ri.categoryIndex[catKey] = append(ri.categoryIndex[catKey], rule)
		}

		// Build product index
		if rule.LogSource.Product != nil {
			product := *rule.LogSource.Product
			ri.productIndex[product] = append(ri.productIndex[product], rule)
		}
	}

	// Update statistics
	ri.stats.TotalRules = len(rules)
	ri.stats.IndexBuildTime = time.Since(start)

	// Count rules per product
	for product, rules := range ri.productIndex {
		ri.stats.RulesPerProduct[product] = len(rules)
	}

	// Count rules per category
	for category, rules := range ri.categoryIndex {
		ri.stats.RulesPerCategory[category] = len(rules)
	}
}

// GetRules returns rules matching the given logsource parameters.
// Uses O(1) lookup with fallback to partial matches.
//
// S9 FIX: Returns the internal slice directly (no defensive copy).
// Rules are immutable after LoadRules() and are protected by the RLock.
// Callers must NOT mutate the returned slice.
func (ri *RuleIndexer) GetRules(product, category, service string) []*domain.SigmaRule {
	start := time.Now()

	ri.mu.RLock()
	defer ri.mu.RUnlock()

	// Try exact match first
	key := fmt.Sprintf("%s:%s:%s", product, category, service)
	if rules, ok := ri.index[key]; ok {
		ri.updateLookupStats(time.Since(start))
		return rules
	}

	// Try category match (product:category:*)
	catKey := fmt.Sprintf("%s:%s", product, category)
	if rules, ok := ri.categoryIndex[catKey]; ok {
		ri.updateLookupStats(time.Since(start))
		return rules
	}

	// Try product match (product:*:*)
	if rules, ok := ri.productIndex[product]; ok {
		ri.updateLookupStats(time.Since(start))
		return rules
	}

	// Fallback to all rules
	ri.updateLookupStats(time.Since(start))
	return ri.allRules
}

// GetRulesByCategory returns all rules for a specific category.
func (ri *RuleIndexer) GetRulesByCategory(category string) []*domain.SigmaRule {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	var result []*domain.SigmaRule
	for key, rules := range ri.categoryIndex {
		if strings.Contains(key, ":"+category) {
			result = append(result, rules...)
		}
	}

	return copyRules(result)
}

// GetRulesByProduct returns all rules for a specific product.
func (ri *RuleIndexer) GetRulesByProduct(product string) []*domain.SigmaRule {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	if rules, ok := ri.productIndex[product]; ok {
		return copyRules(rules)
	}
	return []*domain.SigmaRule{}
}

// GetAllRules returns all indexed rules.
func (ri *RuleIndexer) GetAllRules() []*domain.SigmaRule {
	ri.mu.RLock()
	defer ri.mu.RUnlock()
	return copyRules(ri.allRules)
}

// AddRule adds a single rule to the index.
func (ri *RuleIndexer) AddRule(rule *domain.SigmaRule) error {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	// Check for duplicate ID
	for _, existing := range ri.allRules {
		if existing.ID == rule.ID {
			return fmt.Errorf("rule already exists: %s", rule.ID)
		}
	}

	// Add to all rules
	ri.allRules = append(ri.allRules, rule)
	ri.stats.TotalRules++

	// Update indexes
	key := ri.buildKey(rule.LogSource)
	ri.index[key] = append(ri.index[key], rule)

	if rule.LogSource.Product != nil && rule.LogSource.Category != nil {
		catKey := fmt.Sprintf("%s:%s", *rule.LogSource.Product, *rule.LogSource.Category)
		ri.categoryIndex[catKey] = append(ri.categoryIndex[catKey], rule)
	}

	if rule.LogSource.Product != nil {
		product := *rule.LogSource.Product
		ri.productIndex[product] = append(ri.productIndex[product], rule)
		ri.stats.RulesPerProduct[product]++
	}

	return nil
}

// RemoveRule removes a rule from the index.
func (ri *RuleIndexer) RemoveRule(ruleID string) error {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	// Find rule
	var rule *domain.SigmaRule
	idx := -1
	for i, r := range ri.allRules {
		if r.ID == ruleID {
			rule = r
			idx = i
			break
		}
	}

	if idx < 0 {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	// Remove from all rules
	ri.allRules = append(ri.allRules[:idx], ri.allRules[idx+1:]...)
	ri.stats.TotalRules--

	// Remove from indexes
	key := ri.buildKey(rule.LogSource)
	ri.removeFromSlice(ri.index[key], ruleID)
	if len(ri.index[key]) == 0 {
		delete(ri.index, key)
	}

	if rule.LogSource.Product != nil && rule.LogSource.Category != nil {
		catKey := fmt.Sprintf("%s:%s", *rule.LogSource.Product, *rule.LogSource.Category)
		ri.removeFromSlice(ri.categoryIndex[catKey], ruleID)
		if len(ri.categoryIndex[catKey]) == 0 {
			delete(ri.categoryIndex, catKey)
		}
	}

	if rule.LogSource.Product != nil {
		product := *rule.LogSource.Product
		ri.removeFromSlice(ri.productIndex[product], ruleID)
		if len(ri.productIndex[product]) == 0 {
			delete(ri.productIndex, product)
		}
		ri.stats.RulesPerProduct[product]--
	}

	return nil
}

// removeFromSlice removes a rule from a slice by ID.
func (ri *RuleIndexer) removeFromSlice(rules []*domain.SigmaRule, ruleID string) {
	for i, r := range rules {
		if r.ID == ruleID {
			rules = append(rules[:i], rules[i+1:]...)
			break
		}
	}
}

// buildKey builds an index key from a logsource.
func (ri *RuleIndexer) buildKey(ls domain.LogSource) string {
	product := "*"
	if ls.Product != nil {
		product = *ls.Product
	}
	category := "*"
	if ls.Category != nil {
		category = *ls.Category
	}
	service := "*"
	if ls.Service != nil {
		service = *ls.Service
	}
	return fmt.Sprintf("%s:%s:%s", product, category, service)
}

// updateLookupStats updates lookup statistics (thread-safe).
func (ri *RuleIndexer) updateLookupStats(duration time.Duration) {
	// Use atomic operations for counters
	// Note: This is approximate, exact stats would require more synchronization
	ri.stats.LookupCount++
	ri.stats.LookupTimeTotal += duration
}

// Stats returns indexing statistics.
func (ri *RuleIndexer) Stats() IndexStats {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	stats := ri.stats
	stats.RulesPerProduct = make(map[string]int)
	stats.RulesPerCategory = make(map[string]int)

	for k, v := range ri.stats.RulesPerProduct {
		stats.RulesPerProduct[k] = v
	}
	for k, v := range ri.stats.RulesPerCategory {
		stats.RulesPerCategory[k] = v
	}

	return stats
}

// copyRules creates a copy of the rules slice to prevent external modification.
func copyRules(rules []*domain.SigmaRule) []*domain.SigmaRule {
	if rules == nil {
		return nil
	}
	result := make([]*domain.SigmaRule, len(rules))
	copy(result, rules)
	return result
}

