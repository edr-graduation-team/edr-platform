package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogEvent_ValidEvent(t *testing.T) {
	rawData := map[string]interface{}{
		"@timestamp": "2025-12-26T20:04:05Z",
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "C:\\Windows\\System32\\cmd.exe",
				"CommandLine": "cmd.exe /c dir",
				"ProcessId":   1234,
			},
		},
		"host.name": "TEST-HOST",
	}

	event, err := NewLogEvent(rawData)
	require.NoError(t, err)
	assert.NotNil(t, event)
	assert.Equal(t, EventCategoryProcessCreation, event.Category)
	assert.Equal(t, "windows", event.Product)
}

func TestNewLogEvent_NilData(t *testing.T) {
	event, err := NewLogEvent(nil)
	assert.Error(t, err)
	assert.Nil(t, event)
}

func TestNewLogEvent_EmptyData(t *testing.T) {
	rawData := map[string]interface{}{}
	event, err := NewLogEvent(rawData)
	require.NoError(t, err)
	assert.NotNil(t, event)
}

func TestLogEvent_GetStringField(t *testing.T) {
	rawData := map[string]interface{}{
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "test.exe",
				"CommandLine": "test command",
			},
		},
		"process": map[string]interface{}{
			"executable": "test.exe",
		},
	}

	event, _ := NewLogEvent(rawData)

	// Test existing field using ECS format
	val := event.GetStringField("process.executable")
	assert.Equal(t, "test.exe", val)

	// Test non-existing field
	val = event.GetStringField("nonexistent")
	assert.Empty(t, val)
}

func TestLogEvent_GetInt64Field(t *testing.T) {
	rawData := map[string]interface{}{
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"ProcessId": 1234,
			},
		},
		"process": map[string]interface{}{
			"pid": 1234,
		},
	}

	event, _ := NewLogEvent(rawData)

	val, ok := event.GetInt64Field("process.pid")
	assert.True(t, ok)
	assert.Equal(t, int64(1234), val)
}

func TestLogEvent_GetNestedField(t *testing.T) {
	rawData := map[string]interface{}{
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"ParentImage": "parent.exe",
			},
		},
		"process": map[string]interface{}{
			"parent": map[string]interface{}{
				"executable": "parent.exe",
			},
		},
	}

	event, _ := NewLogEvent(rawData)

	val := event.GetStringField("process.parent.executable")
	assert.Equal(t, "parent.exe", val)
}

func TestLogEvent_CategoryInference(t *testing.T) {
	tests := []struct {
		name     string
		eventID  interface{}
		expected EventCategory
	}{
		{"ProcessCreation", 1, EventCategoryProcessCreation},
		{"NetworkConnection", 3, EventCategoryNetworkConnection},
		{"FileEvent", 11, EventCategoryFileEvent},
		{"RegistryEvent", 13, EventCategoryRegistrySet},
		{"Unknown", 999, EventCategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawData := map[string]interface{}{
				"event.code": tt.eventID,
			}
			event, _ := NewLogEvent(rawData)
			assert.Equal(t, tt.expected, event.Category)
		})
	}
}

func TestLogEvent_FieldCaching(t *testing.T) {
	rawData := map[string]interface{}{
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image": "test.exe",
			},
		},
		"process": map[string]interface{}{
			"executable": "test.exe",
		},
	}

	event, _ := NewLogEvent(rawData)

	// First access
	val1 := event.GetStringField("process.executable")
	assert.NotEmpty(t, val1)

	// Second access (should use cache)
	val2 := event.GetStringField("process.executable")
	assert.Equal(t, val1, val2)
}

func TestLogEvent_HashComputation(t *testing.T) {
	rawData := map[string]interface{}{
		"@timestamp": "2025-12-26T20:04:05Z",
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "test.exe",
				"CommandLine": "test",
			},
		},
	}

	event1, _ := NewLogEvent(rawData)
	event2, _ := NewLogEvent(rawData)

	hash1 := event1.ComputeHash()
	hash2 := event2.ComputeHash()

	// Same event should produce same hash
	assert.Equal(t, hash1, hash2)
	assert.NotEmpty(t, hash1)
}

func TestLogEvent_DifferentHashes(t *testing.T) {
	rawData1 := map[string]interface{}{
		"@timestamp": "2025-12-26T20:04:05Z",
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "test1.exe",
				"CommandLine": "test1 command",
			},
		},
	}

	rawData2 := map[string]interface{}{
		"@timestamp": "2025-12-26T20:04:06Z",
		"event.code": 2,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "test2.exe",
				"CommandLine": "test2 command",
			},
		},
	}

	event1, _ := NewLogEvent(rawData1)
	event2, _ := NewLogEvent(rawData2)

	hash1 := event1.ComputeHash()
	hash2 := event2.ComputeHash()

	// Different events should produce different hashes
	assert.NotEqual(t, hash1, hash2)
}

func BenchmarkLogEvent_GetStringField(b *testing.B) {
	rawData := map[string]interface{}{
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image": "test.exe",
			},
		},
	}

	event, _ := NewLogEvent(rawData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.GetStringField("process.executable")
	}
}

func BenchmarkLogEvent_GetNestedField(b *testing.B) {
	rawData := map[string]interface{}{
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"ParentImage": "parent.exe",
			},
		},
	}

	event, _ := NewLogEvent(rawData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.GetStringField("process.parent.executable")
	}
}

func BenchmarkLogEvent_HashComputation(b *testing.B) {
	rawData := map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339),
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "test.exe",
				"CommandLine": "test",
			},
		},
	}

	event, _ := NewLogEvent(rawData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.ComputeHash()
	}
}

func BenchmarkLogEvent_Creation(b *testing.B) {
	rawData := map[string]interface{}{
		"@timestamp": time.Now().Format(time.RFC3339),
		"event.code": 1,
		"event": map[string]interface{}{
			"EventData": map[string]interface{}{
				"Image":       "test.exe",
				"CommandLine": "test",
				"ProcessId":   1234,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewLogEvent(rawData)
	}
}

