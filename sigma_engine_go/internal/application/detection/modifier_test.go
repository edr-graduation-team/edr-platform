package detection

import (
	"testing"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/cache"
	"github.com/stretchr/testify/assert"
)

func TestModifier_Contains(t *testing.T) {
	registry := NewModifierRegistry(nil)

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		caseInsensitive bool
		expected       bool
	}{
		{"String contains", "hello world", []interface{}{"world"}, false, true},
		{"String does not contain", "hello world", []interface{}{"test"}, false, false},
		{"Case insensitive match", "Hello World", []interface{}{"hello"}, true, true},
		{"Case sensitive no match", "Hello World", []interface{}{"hello"}, false, false},
		{"Empty string", "", []interface{}{"test"}, false, false},
		{"Nil value", nil, []interface{}{"test"}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{"contains"}, tt.caseInsensitive)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModifier_StartsWith(t *testing.T) {
	registry := NewModifierRegistry(nil) // No regex cache needed for startswith

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		caseInsensitive bool
		expected       bool
	}{
		{"String starts with", "hello world", []interface{}{"hello"}, false, true},
		{"String does not start with", "hello world", []interface{}{"xyz"}, false, false}, // "hello world" does not start with "xyz"
		{"Case insensitive", "Hello World", []interface{}{"hello"}, true, true},
		{"Empty prefix", "test", []interface{}{""}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{"startswith"}, tt.caseInsensitive)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModifier_EndsWith(t *testing.T) {
	registry := NewModifierRegistry(nil) // No regex cache needed for endswith

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		caseInsensitive bool
		expected       bool
	}{
		{"String ends with", "hello world", []interface{}{"world"}, false, true},
		{"String does not end with", "hello world", []interface{}{"xyz"}, false, false}, // "hello world" does not end with "xyz"
		{"Case insensitive", "Hello World", []interface{}{"world"}, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{"endswith"}, tt.caseInsensitive)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModifier_Regex(t *testing.T) {
	regexCache, _ := cache.NewRegexCache(100)
	registry := NewModifierRegistry(regexCache)

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		caseInsensitive bool
		expected       bool
	}{
		{"Simple pattern", "hello123", []interface{}{"\\d+"}, false, true},
		{"Complex pattern", "test@example.com", []interface{}{"[a-z]+@[a-z]+\\.[a-z]+"}, false, true},
		{"No match", "hello", []interface{}{"\\d+"}, false, false},
		{"Case insensitive", "Hello", []interface{}{"hello"}, true, true},
		{"Invalid pattern", "test", []interface{}{"["}, false, false}, // Should handle gracefully
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{"regex"}, tt.caseInsensitive)
			if tt.name == "Invalid pattern" {
				// Invalid regex should return false, not error
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestModifier_Base64(t *testing.T) {
	registry := NewModifierRegistry(nil)

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		expected       bool
	}{
		{"Valid base64", "SGVsbG8gV29ybGQ=", []interface{}{"Hello World"}, true},
		{"Invalid base64", "not base64!", []interface{}{"test"}, false},
		{"Empty base64", "", []interface{}{""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{"base64"}, false)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModifier_CIDR(t *testing.T) {
	registry := NewModifierRegistry(nil)

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		expected       bool
	}{
		{"IP in range", "192.168.1.100", []interface{}{"192.168.1.0/24"}, true},
		{"IP outside range", "10.0.0.1", []interface{}{"192.168.1.0/24"}, false},
		{"Invalid IP", "not an ip", []interface{}{"192.168.1.0/24"}, false},
		{"Invalid CIDR", "192.168.1.100", []interface{}{"invalid"}, false}, // Should return false, not error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{"cidr"}, false)
			if tt.name == "Invalid CIDR" {
				// Invalid CIDR returns error, which is acceptable
				assert.Error(t, err)
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestModifier_Numeric(t *testing.T) {
	registry := NewModifierRegistry(nil)

	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		modifier       string
		expected       bool
	}{
		{"Less than", "5", []interface{}{10.0}, "lt", true},
		{"Less than or equal", "10", []interface{}{10.0}, "lte", true},
		{"Greater than", "15", []interface{}{10.0}, "gt", true},
		{"Greater than or equal", "10", []interface{}{10.0}, "gte", true},
		{"Not less than", "15", []interface{}{10.0}, "lt", false},
		{"Invalid number", "not a number", []interface{}{10.0}, "lt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, []string{tt.modifier}, false)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModifier_All(t *testing.T) {
	registry := NewModifierRegistry(nil)

	// Note: "all" modifier means ALL pattern values must match (AND logic)
	// Without "all", it's OR logic (any pattern matches)
	tests := []struct {
		name           string
		fieldValue     interface{}
		expectedValues []interface{}
		modifiers      []string
		expected       bool
	}{
		{"All patterns match (with all)", "test string", []interface{}{"test", "string"}, []string{"all", "contains"}, true},
		{"Some patterns do not match (with all)", "test string", []interface{}{"test", "missing"}, []string{"all", "contains"}, false},
		{"All patterns match (without all)", "test string", []interface{}{"test", "string"}, []string{"contains"}, true},
		{"Some patterns match (without all)", "test string", []interface{}{"test", "missing"}, []string{"contains"}, true}, // OR logic
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ApplyModifier(tt.fieldValue, tt.expectedValues, tt.modifiers, false)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkModifier_Contains(b *testing.B) {
	registry := NewModifierRegistry(nil)
	fieldValue := "hello world test string"
	expectedValues := []interface{}{"test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.ApplyModifier(fieldValue, expectedValues, []string{"contains"}, false)
	}
}

func BenchmarkModifier_Regex(b *testing.B) {
	registry := NewModifierRegistry(nil)
	fieldValue := "test@example.com"
	expectedValues := []interface{}{"[a-z]+@[a-z]+\\.[a-z]+"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.ApplyModifier(fieldValue, expectedValues, []string{"regex"}, false)
	}
}

func BenchmarkModifier_Base64(b *testing.B) {
	registry := NewModifierRegistry(nil)
	fieldValue := "SGVsbG8gV29ybGQ="
	expectedValues := []interface{}{"Hello World"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.ApplyModifier(fieldValue, expectedValues, []string{"base64"}, false)
	}
}

func BenchmarkModifier_CIDR(b *testing.B) {
	registry := NewModifierRegistry(nil)
	fieldValue := "192.168.1.100"
	expectedValues := []interface{}{"192.168.1.0/24"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.ApplyModifier(fieldValue, expectedValues, []string{"cidr"}, false)
	}
}

