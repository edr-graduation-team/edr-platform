package detection

import (
	"encoding/base64"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
)

// ModifierFunc defines the signature for a field modifier function.
// fieldValue: The value from the event field
// patternValue: The pattern to match against
// caseInsensitive: Whether to ignore case
// Returns: true if match, false otherwise
type ModifierFunc func(fieldValue interface{}, patternValue interface{}, caseInsensitive bool) (bool, error)

// ModifierRegistry manages modifier functions.
type ModifierRegistry struct {
	modifiers  map[string]ModifierFunc
	regexCache cache.RegexCache
}

// NewModifierRegistry creates a new modifier registry with all built-in modifiers.
func NewModifierRegistry(regexCache cache.RegexCache) *ModifierRegistry {
	registry := &ModifierRegistry{
		modifiers:  make(map[string]ModifierFunc),
		regexCache: regexCache,
	}

	// Register all built-in modifiers
	registry.Register("contains", registry.modifierContains)
	registry.Register("startswith", registry.modifierStartsWith)
	registry.Register("endswith", registry.modifierEndsWith)
	registry.Register("regex", registry.modifierRegex)
	registry.Register("re", registry.modifierRegex) // Alias
	registry.Register("base64", registry.modifierBase64)
	registry.Register("base64offset", registry.modifierBase64Offset)
	registry.Register("windash", registry.modifierWinDash)
	registry.Register("cidr", registry.modifierCIDR)
	registry.Register("lt", registry.modifierLessThan)
	registry.Register("lte", registry.modifierLessThanOrEqual)
	registry.Register("gt", registry.modifierGreaterThan)
	registry.Register("gte", registry.modifierGreaterThanOrEqual)
	registry.Register("all", nil) // Special modifier, handled separately

	return registry
}

// Register registers a modifier function.
func (mr *ModifierRegistry) Register(name string, fn ModifierFunc) {
	mr.modifiers[strings.ToLower(name)] = fn
}

// Get retrieves a modifier function by name.
func (mr *ModifierRegistry) Get(name string) (ModifierFunc, bool) {
	fn, ok := mr.modifiers[strings.ToLower(name)]
	return fn, ok
}

// ApplyModifier applies a modifier to a field value.
// Returns true if the modifier matches, false otherwise.
// Handles the special "all" modifier for AND logic.
func (mr *ModifierRegistry) ApplyModifier(
	fieldValue interface{},
	patternValues []interface{},
	modifiers []string,
	caseInsensitive bool,
) (bool, error) {
	if fieldValue == nil {
		return false, nil
	}

	// Check for "all" modifier (AND logic)
	requireAll := false
	for _, mod := range modifiers {
		if strings.EqualFold(mod, "all") {
			requireAll = true
			break
		}
	}

	// Apply modifiers to each pattern value
	for _, patternValue := range patternValues {
		matched := false
		var err error

		// Try each modifier
		for _, modName := range modifiers {
			if strings.EqualFold(modName, "all") {
				continue
			}

			fn, ok := mr.Get(modName)
			if !ok {
				// Unknown modifier, try default behavior (contains)
				matched, err = mr.modifierContains(fieldValue, patternValue, caseInsensitive)
			} else {
				matched, err = fn(fieldValue, patternValue, caseInsensitive)
			}

			if err != nil {
				return false, fmt.Errorf("modifier %q error: %w", modName, err)
			}

			if matched {
				if !requireAll {
					return true, nil // OR logic: any match succeeds
				}
				break // AND logic: continue checking other patterns
			}
		}

		// For AND logic, if any pattern doesn't match, return false
		if requireAll && !matched {
			return false, nil
		}

		// For OR logic, if no modifier matched, try default contains
		if !requireAll && !matched {
			matched, err = mr.modifierContains(fieldValue, patternValue, caseInsensitive)
			if err == nil && matched {
				return true, nil
			}
		}
	}

	// For AND logic, all patterns matched
	if requireAll {
		return true, nil
	}

	return false, nil
}

// modifierContains checks if field value contains the pattern (substring match).
func (mr *ModifierRegistry) modifierContains(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	if caseInsensitive {
		return strings.Contains(strings.ToLower(fieldStr), strings.ToLower(patternStr)), nil
	}
	return strings.Contains(fieldStr, patternStr), nil
}

// modifierStartsWith checks if field value starts with the pattern.
func (mr *ModifierRegistry) modifierStartsWith(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	if caseInsensitive {
		return strings.HasPrefix(strings.ToLower(fieldStr), strings.ToLower(patternStr)), nil
	}
	return strings.HasPrefix(fieldStr, patternStr), nil
}

// modifierEndsWith checks if field value ends with the pattern.
func (mr *ModifierRegistry) modifierEndsWith(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	if caseInsensitive {
		return strings.HasSuffix(strings.ToLower(fieldStr), strings.ToLower(patternStr)), nil
	}
	return strings.HasSuffix(fieldStr, patternStr), nil
}

// modifierRegex checks if field value matches the regex pattern.
// Uses pre-compiled regex if available (from rule loading), otherwise compiles on-demand.
func (mr *ModifierRegistry) modifierRegex(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	// Try to get pre-compiled regex from cache first (if available)
	// This is a fallback for when pre-compiled regex is not available
	var regex *regexp.Regexp
	compiled, err := mr.regexCache.GetOrCompile(patternStr, 0)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}

	var ok bool
	regex, ok = compiled.(*regexp.Regexp)
	if !ok {
		return false, fmt.Errorf("regex cache returned invalid type")
	}

	return regex.MatchString(fieldStr), nil
}

// modifierBase64 checks if field value contains base64-encoded pattern.
func (mr *ModifierRegistry) modifierBase64(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	// Encode pattern as base64
	encoded := base64.StdEncoding.EncodeToString([]byte(patternStr))

	if caseInsensitive {
		return strings.Contains(strings.ToLower(fieldStr), strings.ToLower(encoded)), nil
	}
	return strings.Contains(fieldStr, encoded), nil
}

// modifierBase64Offset checks if field value contains base64-encoded pattern with offset variations.
func (mr *ModifierRegistry) modifierBase64Offset(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	// Try all three offset positions (0, 1, 2 bytes)
	for offset := 0; offset < 3; offset++ {
		padded := strings.Repeat(" ", offset) + patternStr
		encoded := base64.StdEncoding.EncodeToString([]byte(padded))
		encodedTrimmed := strings.TrimRight(encoded, "=")

		if caseInsensitive {
			if strings.Contains(strings.ToLower(fieldStr), strings.ToLower(encodedTrimmed)) {
				return true, nil
			}
		} else {
			if strings.Contains(fieldStr, encodedTrimmed) {
				return true, nil
			}
		}
	}

	return false, nil
}

// modifierWinDash normalizes Windows paths and command-line arguments.
func (mr *ModifierRegistry) modifierWinDash(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := toString(fieldValue)
	patternStr := toString(patternValue)

	// Generate Windows dash variations
	variations := []string{patternStr}
	if strings.Contains(patternStr, "-") {
		variations = append(variations, strings.ReplaceAll(patternStr, "-", "/"))
	}
	if strings.Contains(patternStr, "/") {
		variations = append(variations, strings.ReplaceAll(patternStr, "/", "-"))
	}
	if strings.Contains(patternStr, "--") {
		variations = append(variations, strings.ReplaceAll(patternStr, "--", "//"))
	}

	fieldLower := strings.ToLower(fieldStr)
	for _, variation := range variations {
		variationLower := strings.ToLower(variation)
		if caseInsensitive {
			if strings.Contains(fieldLower, variationLower) {
				return true, nil
			}
		} else {
			if strings.Contains(fieldStr, variation) {
				return true, nil
			}
		}
	}

	return false, nil
}

// modifierCIDR checks if field value (IP address) is within CIDR range.
func (mr *ModifierRegistry) modifierCIDR(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldStr := strings.TrimSpace(toString(fieldValue))
	patternStr := strings.TrimSpace(toString(patternValue))

	ip := net.ParseIP(fieldStr)
	if ip == nil {
		return false, nil
	}

	_, ipNet, err := net.ParseCIDR(patternStr)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR notation: %w", err)
	}

	return ipNet.Contains(ip), nil
}

// modifierLessThan checks if field value is less than pattern value.
func (mr *ModifierRegistry) modifierLessThan(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldNum, err := toFloat64(fieldValue)
	if err != nil {
		return false, nil
	}

	patternNum, err := toFloat64(patternValue)
	if err != nil {
		return false, fmt.Errorf("pattern value is not numeric: %w", err)
	}

	return fieldNum < patternNum, nil
}

// modifierLessThanOrEqual checks if field value is less than or equal to pattern value.
func (mr *ModifierRegistry) modifierLessThanOrEqual(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldNum, err := toFloat64(fieldValue)
	if err != nil {
		return false, nil
	}

	patternNum, err := toFloat64(patternValue)
	if err != nil {
		return false, fmt.Errorf("pattern value is not numeric: %w", err)
	}

	return fieldNum <= patternNum, nil
}

// modifierGreaterThan checks if field value is greater than pattern value.
func (mr *ModifierRegistry) modifierGreaterThan(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldNum, err := toFloat64(fieldValue)
	if err != nil {
		return false, nil
	}

	patternNum, err := toFloat64(patternValue)
	if err != nil {
		return false, fmt.Errorf("pattern value is not numeric: %w", err)
	}

	return fieldNum > patternNum, nil
}

// modifierGreaterThanOrEqual checks if field value is greater than or equal to pattern value.
func (mr *ModifierRegistry) modifierGreaterThanOrEqual(fieldValue, patternValue interface{}, caseInsensitive bool) (bool, error) {
	fieldNum, err := toFloat64(fieldValue)
	if err != nil {
		return false, nil
	}

	patternNum, err := toFloat64(patternValue)
	if err != nil {
		return false, fmt.Errorf("pattern value is not numeric: %w", err)
	}

	return fieldNum >= patternNum, nil
}

// Helper functions for type conversion

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toFloat64(v interface{}) (float64, error) {
	if v == nil {
		return 0, fmt.Errorf("value is nil")
	}

	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}
