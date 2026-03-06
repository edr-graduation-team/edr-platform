package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSigmaRule_Validation(t *testing.T) {
	tests := []struct {
		name    string
		rule    *SigmaRule
		wantErr bool
	}{
		{
			name: "Valid rule",
			rule: &SigmaRule{
				ID:    "test-id",
				Title: "Test Rule",
				LogSource: LogSource{
					Product:  stringPtr("windows"),
					Category: stringPtr("process_creation"),
				},
				Detection: Detection{
					Selections: map[string]*Selection{
						"selection1": {
							Name: "selection1",
							Fields: []SelectionField{
								{FieldName: "Image", Values: []interface{}{"test.exe"}},
							},
						},
					},
					Condition: "selection1",
				},
				Level:  "medium",
				Status: "stable",
			},
			wantErr: false,
		},
		{
			name: "Missing title",
			rule: &SigmaRule{
				ID: "test-id",
				LogSource: LogSource{
					Product: stringPtr("windows"),
				},
				Detection: Detection{
					Selections: map[string]*Selection{
						"selection1": {Name: "selection1"},
					},
					Condition: "selection1",
				},
			},
			wantErr: true,
		},
		{
			name: "Missing detection",
			rule: &SigmaRule{
				ID:    "test-id",
				Title: "Test Rule",
				LogSource: LogSource{
					Product: stringPtr("windows"),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLogSource_Matches(t *testing.T) {
	tests := []struct {
		name         string
		logSource    LogSource
		product      string
		category     string
		service      string
		shouldMatch  bool
	}{
		{
			name: "Exact match",
			logSource: LogSource{
				Product:  stringPtr("windows"),
				Category: stringPtr("process_creation"),
				Service:  stringPtr("sysmon"),
			},
			product:     "windows",
			category:    "process_creation",
			service:     "sysmon",
			shouldMatch: true,
		},
		{
			name: "Partial match - product and category",
			logSource: LogSource{
				Product:  stringPtr("windows"),
				Category: stringPtr("process_creation"),
			},
			product:     "windows",
			category:    "process_creation",
			service:     "sysmon",
			shouldMatch: true,
		},
		{
			name: "Product only match",
			logSource: LogSource{
				Product: stringPtr("windows"),
			},
			product:     "windows",
			category:    "process_creation",
			service:     "sysmon",
			shouldMatch: true,
		},
		{
			name: "No match - wrong product",
			logSource: LogSource{
				Product: stringPtr("linux"),
			},
			product:     "windows",
			category:    "process_creation",
			service:     "sysmon",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.logSource.Matches(tt.product, tt.category, tt.service)
			assert.Equal(t, tt.shouldMatch, result)
		})
	}
}

func TestLogSource_IndexKey(t *testing.T) {
	tests := []struct {
		name      string
		logSource LogSource
		expected  string
	}{
		{
			name: "Full logsource",
			logSource: LogSource{
				Product:  stringPtr("windows"),
				Category: stringPtr("process_creation"),
				Service:  stringPtr("sysmon"),
			},
			expected: "windows:process_creation:sysmon",
		},
		{
			name: "Partial logsource",
			logSource: LogSource{
				Product:  stringPtr("windows"),
				Category: stringPtr("process_creation"),
			},
			expected: "windows:process_creation:*",
		},
		{
			name: "Product only",
			logSource: LogSource{
				Product: stringPtr("windows"),
			},
			expected: "windows:*:*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.logSource.IndexKey()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSigmaRule_Severity(t *testing.T) {
	tests := []struct {
		level    string
		expected Severity
	}{
		{"critical", SeverityCritical},
		{"high", SeverityHigh},
		{"medium", SeverityMedium},
		{"low", SeverityLow},
		{"informational", SeverityInformational},
		{"unknown", SeverityMedium}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			rule := &SigmaRule{Level: tt.level}
			result := rule.Severity()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSigmaRule_MITRETechniques(t *testing.T) {
	rule := &SigmaRule{
		Tags: []string{
			"attack.execution",
			"attack.t1059",
			"attack.t1059.001",
			"attack.command_and_control",
		},
	}

	techniques := rule.MITRETechniques()
	assert.Contains(t, techniques, "T1059")
	assert.Contains(t, techniques, "T1059.001")
	assert.NotContains(t, techniques, "execution")
}

func BenchmarkSigmaRule_Validation(b *testing.B) {
	rule := &SigmaRule{
		ID:    "test-id",
		Title: "Test Rule",
		LogSource: LogSource{
			Product:  stringPtr("windows"),
			Category: stringPtr("process_creation"),
		},
		Detection: Detection{
			Selections: map[string]*Selection{
				"selection1": {
					Name: "selection1",
					Fields: []SelectionField{
						{FieldName: "Image", Values: []interface{}{"test.exe"}},
					},
				},
			},
			Condition: "selection1",
		},
		Level:  "medium",
		Status: "stable",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rule.Validate()
	}
}

func BenchmarkLogSource_Matching(b *testing.B) {
	logSource := LogSource{
		Product:  stringPtr("windows"),
		Category: stringPtr("process_creation"),
		Service:  stringPtr("sysmon"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logSource.Matches("windows", "process_creation", "sysmon")
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}

