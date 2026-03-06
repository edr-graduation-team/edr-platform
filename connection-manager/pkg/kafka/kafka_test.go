// Package kafka provides unit tests for Kafka producer.
package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultProducerConfig(t *testing.T) {
	brokers := []string{"localhost:9092", "localhost:9093"}
	cfg := DefaultProducerConfig(brokers)

	assert.Equal(t, brokers, cfg.Brokers)
	assert.Equal(t, "events-raw", cfg.Topic)
	assert.Equal(t, "events-dlq", cfg.DLQTopic)
	assert.Equal(t, "snappy", cfg.Compression)
	assert.Equal(t, "all", cfg.Acks)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 16384, cfg.BatchSize)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestProducerConfig_Validation(t *testing.T) {
	tests := []struct {
		name        string
		brokers     []string
		expectError bool
	}{
		{
			name:        "empty brokers fails",
			brokers:     []string{},
			expectError: true,
		},
		{
			name:        "nil brokers fails",
			brokers:     nil,
			expectError: true,
		},
		{
			name:        "valid brokers succeeds",
			brokers:     []string{"localhost:9092"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultProducerConfig(tt.brokers)
			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel)

			_, err := NewEventProducer(cfg, nil, logger)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Note: This will fail without actual Kafka, but validates config
				// In integration tests, we'd have a real Kafka
				assert.NoError(t, err)
			}
		})
	}
}

func TestProducerConfig_Compression(t *testing.T) {
	tests := []struct {
		name        string
		compression string
	}{
		{"snappy", "snappy"},
		{"gzip", "gzip"},
		{"lz4", "lz4"},
		{"zstd", "zstd"},
		{"default", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultProducerConfig([]string{"localhost:9092"})
			cfg.Compression = tt.compression

			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel)

			producer, err := NewEventProducer(cfg, nil, logger)
			require.NoError(t, err)
			assert.NotNil(t, producer)

			// Cleanup
			producer.Close()
		})
	}
}

func TestProducerConfig_Acks(t *testing.T) {
	tests := []struct {
		name string
		acks string
	}{
		{"none", "none"},
		{"one", "one"},
		{"all", "all"},
		{"default", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultProducerConfig([]string{"localhost:9092"})
			cfg.Acks = tt.acks

			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel)

			producer, err := NewEventProducer(cfg, nil, logger)
			require.NoError(t, err)
			assert.NotNil(t, producer)

			producer.Close()
		})
	}
}

func TestEventProducer_Close(t *testing.T) {
	cfg := DefaultProducerConfig([]string{"localhost:9092"})
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	producer, err := NewEventProducer(cfg, nil, logger)
	require.NoError(t, err)

	// Close should not panic
	err = producer.Close()
	assert.NoError(t, err)
}

func TestEventProducer_Stats(t *testing.T) {
	cfg := DefaultProducerConfig([]string{"localhost:9092"})
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	producer, err := NewEventProducer(cfg, nil, logger)
	require.NoError(t, err)
	defer producer.Close()

	stats := producer.Stats()
	// Initial stats should be empty
	assert.Equal(t, int64(0), stats.Messages)
	assert.Equal(t, int64(0), stats.Errors)
}

// Integration test (requires Kafka)
func TestEventProducer_HealthCheck_NoKafka(t *testing.T) {
	cfg := DefaultProducerConfig([]string{"localhost:19999"}) // Non-existent
	cfg.Timeout = 2 * time.Second

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	producer, err := NewEventProducer(cfg, nil, logger)
	require.NoError(t, err)
	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = producer.HealthCheck(ctx)
	assert.Error(t, err)
}

func TestDefaultConsumerConfig(t *testing.T) {
	brokers := []string{"localhost:9092"}
	cfg := DefaultConsumerConfig(brokers, "test-topic", "test-group")

	assert.Equal(t, brokers, cfg.Brokers)
	assert.Equal(t, "test-topic", cfg.Topic)
	assert.Equal(t, "test-group", cfg.GroupID)
	assert.Equal(t, "newest", cfg.StartOffset)
	assert.Equal(t, 1024, cfg.MinBytes)
	assert.Equal(t, 10485760, cfg.MaxBytes)
}

func TestConsumedMessage(t *testing.T) {
	msg := &ConsumedMessage{
		Key:       "agent-123",
		Value:     []byte("test payload"),
		Headers:   map[string]string{"batch_id": "batch-456"},
		Partition: 2,
		Offset:    100,
		Topic:     "events-raw",
		Timestamp: time.Now(),
	}

	assert.Equal(t, "agent-123", msg.Key)
	assert.Equal(t, 2, msg.Partition)
	assert.Equal(t, int64(100), msg.Offset)
	assert.Equal(t, "batch-456", msg.Headers["batch_id"])
}

func TestDLQEntry_ToJSON(t *testing.T) {
	entry := &DLQEntry{
		OriginalKey:   "agent-123",
		Error:         "connection timeout",
		FailedAt:      time.Now(),
		Payload:       []byte("test"),
		OriginalTopic: "events-raw",
	}

	json := entry.ToJSON()
	assert.Contains(t, json, "agent-123")
	assert.Contains(t, json, "connection timeout")
	assert.Contains(t, json, "events-raw")
}
