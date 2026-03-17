package detection

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/edr-platform/sigma-engine/internal/application/mapping"
	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
)

// SelectionEvaluator evaluates whether an event matches a selection's conditions.
// Thread-safe and optimized for high-performance field matching.
type SelectionEvaluator struct {
	fieldMapper    *mapping.FieldMapper
	modifierEngine *ModifierRegistry
	cache          *cache.FieldResolutionCache
}

// NewSelectionEvaluator creates a new selection evaluator.
func NewSelectionEvaluator(
	fieldMapper *mapping.FieldMapper,
	modifierEngine *ModifierRegistry,
	fieldCache *cache.FieldResolutionCache,
) *SelectionEvaluator {
	return &SelectionEvaluator{
		fieldMapper:    fieldMapper,
		modifierEngine: modifierEngine,
		cache:          fieldCache,
	}
}

// Evaluate evaluates whether an event matches all conditions in a selection.
// Returns true if ALL fields match (AND logic).
// Uses early exit optimization: stops on first mismatch.
func (se *SelectionEvaluator) Evaluate(
	selection *domain.Selection,
	event *domain.LogEvent,
) bool {
	// Handle keyword-based selections
	if selection.IsKeywordSelection {
		return se.evaluateKeywords(selection.Keywords, event)
	}

	// Handle field-based selections (AND logic: all must match)
	for _, field := range selection.Fields {
		if !se.EvaluateField(field, event) {
			return false // Early exit on first mismatch
		}
	}

	return true
}

// EvaluateField evaluates whether a single field condition matches the event.
// Handles field resolution, type conversion, and modifier application.
// Uses pre-compiled regex patterns when available for better performance.
func (se *SelectionEvaluator) EvaluateField(
	field domain.SelectionField,
	event *domain.LogEvent,
) bool {
	// Resolve field value from event
	fieldValue, found := se.resolveFieldValue(field.FieldName, event)
	if !found {
		return false // Field not found = no match
	}

	// Check if we have pre-compiled regex patterns (performance optimization)
	hasRegexModifier := false
	for _, mod := range field.Modifiers {
		if mod == "regex" || mod == "re" {
			hasRegexModifier = true
			break
		}
	}

	// Use pre-compiled regex if available
	if hasRegexModifier && len(field.CompiledRegex) > 0 {
		fieldStr := fmt.Sprintf("%v", fieldValue)
		for _, re := range field.CompiledRegex {
			if re.MatchString(fieldStr) {
				if !field.IsNegated {
					return true // Match found
				}
				// For negated fields, if any value matches, the negation fails
				return false
			}
		}
		// If we get here and field is negated, all patterns didn't match = negation succeeds
		if field.IsNegated {
			return true
		}
		return false
	}

	// Check if this field should use ALL logic
	isAllModifier := false
	for _, mod := range field.Modifiers {
		if strings.EqualFold(mod, "all") {
			isAllModifier = true
			break
		}
	}

	if isAllModifier {
		// Pass the entire Values array to the modifier engine
		match, err := se.modifierEngine.ApplyModifier(
			fieldValue,
			field.Values,
			field.Modifiers,
			true, // Sigma spec dictates case-insensitive matching by default
		)
		if err != nil {
			logger.Debugf("Modifier error: %v", err)
			return false
		}
		if field.IsNegated {
			return !match
		}
		return match
	}

	// Handle multiple expected values (OR logic within field)
	// If any value matches, field matches
	for _, expectedValue := range field.Values {
		if se.compareValue(fieldValue, expectedValue, field.Modifiers, field.IsNegated) {
			if !field.IsNegated {
				return true // Match found
			}
			// For negated fields, if any value matches, the negation fails
			return false
		}
	}

	// If we get here and field is negated, all values didn't match = negation succeeds
	if field.IsNegated {
		return true
	}

	// No values matched
	return false
}

// resolveFieldValue resolves a field value from the event using the field mapper.
func (se *SelectionEvaluator) resolveFieldValue(
	fieldName string,
	event *domain.LogEvent,
) (interface{}, bool) {
	// Check cache first
	if se.cache != nil {
		cacheKey := fmt.Sprintf("%s:%s", event.ComputeHash(), fieldName)
		if cached, ok := se.cache.Get(cacheKey); ok {
			return cached, true
		}
	}

	// Resolve field using mapper
	value, _, err := se.fieldMapper.ResolveField(event.RawData, fieldName)
	if err != nil || value == nil {
		return nil, false
	}

	// Cache result
	if se.cache != nil {
		cacheKey := fmt.Sprintf("%s:%s", event.ComputeHash(), fieldName)
		se.cache.Put(cacheKey, value)
	}

	return value, true
}

// compareValue compares a field value with an expected value, applying modifiers.
func (se *SelectionEvaluator) compareValue(
	fieldValue interface{},
	expectedValue interface{},
	modifiers []string,
	isNegated bool,
) bool {
	if fieldValue == nil {
		return false
	}

	// Apply modifiers if present
	if len(modifiers) > 0 {
		// Convert expectedValue to slice for modifier engine
		expectedValues := []interface{}{expectedValue}
		match, err := se.modifierEngine.ApplyModifier(
			fieldValue,
			expectedValues,
			modifiers,
			true, // Sigma requires case-insensitive matching by default
		)
		if err != nil {
			logger.Debugf("Modifier error: %v", err)
			return false
		}
		return match
	}

	// Direct comparison without modifiers
	return se.directCompare(fieldValue, expectedValue)
}

// directCompare performs direct value comparison without modifiers.
func (se *SelectionEvaluator) directCompare(
	fieldValue interface{},
	expectedValue interface{},
) bool {
	// Type-based comparison
	switch fv := fieldValue.(type) {
	case string:
		ev, ok := expectedValue.(string)
		if !ok {
			ev = toString(expectedValue)
		}
		return strings.EqualFold(fv, ev)

	case int, int8, int16, int32, int64:
		fvNum := convertToFloat64(fieldValue)
		evNum := convertToFloat64(expectedValue)
		return fvNum == evNum

	case uint, uint8, uint16, uint32, uint64:
		fvNum := convertToFloat64(fieldValue)
		evNum := convertToFloat64(expectedValue)
		return fvNum == evNum

	case float32, float64:
		fvNum := convertToFloat64(fieldValue)
		evNum := convertToFloat64(expectedValue)
		return fvNum == evNum

	case bool:
		ev, ok := expectedValue.(bool)
		if !ok {
			ev = convertToBool(expectedValue)
		}
		return fv == ev

	case []interface{}:
		// Array: check if any element matches
		return se.compareArray(fv, expectedValue)

	case []string:
		// String array: check if any element matches
		evStr := fmt.Sprintf("%v", expectedValue)
		for _, item := range fv {
			if strings.EqualFold(item, evStr) {
				return true
			}
		}
		return false

	default:
		// Fallback: convert both to strings and compare
		fvStr := fmt.Sprintf("%v", fieldValue)
		evStr := fmt.Sprintf("%v", expectedValue)
		return strings.EqualFold(fvStr, evStr)
	}
}

// compareArray compares an array value with an expected value.
func (se *SelectionEvaluator) compareArray(
	arrayValue []interface{},
	expectedValue interface{},
) bool {
	// Check if any element in array matches expected value
	for _, item := range arrayValue {
		if se.directCompare(item, expectedValue) {
			return true
		}
	}
	return false
}

// evaluateKeywords evaluates keyword-based selections (full-text search).
// Searches the full event payload (RawData serialized to JSON) for keywords.
func (se *SelectionEvaluator) evaluateKeywords(
	keywords []string,
	event *domain.LogEvent,
) bool {
	// Serialize the full event data so keyword search covers all field values
	eventBytes, err := json.Marshal(event.RawData)
	if err != nil {
		logger.Warnf("Failed to marshal event for keyword search: %v", err)
		return false
	}
	eventStr := strings.ToLower(string(eventBytes))

	for _, keyword := range keywords {
		if strings.Contains(eventStr, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// Helper functions for type conversion
func convertToFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	default:
		// Try reflection for other numeric types
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return float64(rv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return float64(rv.Uint())
		case reflect.Float32, reflect.Float64:
			return rv.Float()
		}
		return 0
	}
}

func convertToBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true") || b == "1"
	case int, int8, int16, int32, int64:
		return reflect.ValueOf(b).Int() != 0
	case uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(b).Uint() != 0
	default:
		return false
	}
}
