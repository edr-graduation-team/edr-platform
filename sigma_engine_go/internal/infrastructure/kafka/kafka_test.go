// Package kafka provides unit tests for Kafka consumer.
package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConsumerConfig(t *testing.T) {
	cfg := DefaultConsumerConfig()

	assert.Equal(t, []string{"localhost:9092"}, cfg.Brokers)
	assert.Equal(t, "events-raw", cfg.Topic)
	assert.Equal(t, "sigma-engine-group", cfg.GroupID)
	assert.Equal(t, 5*time.Second, cfg.MaxWait)
	assert.Equal(t, int64(-1), cfg.StartOffset) // LastOffset
}

func TestDefaultProducerConfig(t *testing.T) {
	cfg := DefaultProducerConfig()

	assert.Equal(t, []string{"localhost:9092"}, cfg.Brokers)
	assert.Equal(t, "alerts", cfg.Topic)
	assert.Equal(t, 50, cfg.BatchSize)
	assert.Equal(t, -1, cfg.RequiredAcks)
	assert.Equal(t, "snappy", cfg.Compression)
}

func TestDefaultEventLoopConfig(t *testing.T) {
	cfg := DefaultEventLoopConfig()

	assert.Equal(t, 4, cfg.Workers)
	assert.Equal(t, 1000, cfg.EventBuffer)
	assert.Equal(t, 500, cfg.AlertBuffer)
	assert.Equal(t, 10*time.Second, cfg.StatsInterval)
}

func TestConsumerMetricsSnapshot(t *testing.T) {
	m := &ConsumerMetrics{}

	m.MessagesConsumed = 100
	m.MessagesProcessed = 95
	m.DeserializeErrors = 5

	snapshot := m.Snapshot()

	assert.Equal(t, uint64(100), snapshot.MessagesConsumed)
	assert.Equal(t, uint64(95), snapshot.MessagesProcessed)
	assert.Equal(t, uint64(5), snapshot.DeserializeErrors)
}

func TestProducerMetricsSnapshot(t *testing.T) {
	m := &ProducerMetrics{}

	m.AlertsPublished = 50
	m.PublishErrors = 2
	m.BytesSent = 10000

	snapshot := m.Snapshot()

	assert.Equal(t, uint64(50), snapshot.AlertsPublished)
	assert.Equal(t, uint64(2), snapshot.PublishErrors)
	assert.Equal(t, uint64(10000), snapshot.BytesSent)
}

func TestEventLoopMetricsSnapshot(t *testing.T) {
	m := &EventLoopMetrics{}

	m.EventsReceived = 1000
	m.EventsProcessed = 990
	m.AlertsGenerated = 50
	m.AverageLatencyMs = 1.5

	snapshot := m.Snapshot()

	assert.Equal(t, uint64(1000), snapshot.EventsReceived)
	assert.Equal(t, uint64(990), snapshot.EventsProcessed)
	assert.Equal(t, uint64(50), snapshot.AlertsGenerated)
	assert.InDelta(t, 1.5, snapshot.AverageLatencyMs, 0.01)
}

func TestKafkaConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.Enabled) // Disabled by default
	assert.Equal(t, "events-raw", cfg.Consumer.Topic)
	assert.Equal(t, "alerts", cfg.Producer.Topic)
}

func TestKafkaConfigValidate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true

	err := cfg.Validate()
	require.NoError(t, err)

	// Check defaults were applied
	assert.NotEmpty(t, cfg.Consumer.Brokers)
	assert.NotEmpty(t, cfg.Consumer.Topic)
	assert.NotEmpty(t, cfg.Consumer.GroupID)
}

func TestNewEventConsumer(t *testing.T) {
	cfg := DefaultConsumerConfig()

	consumer, err := NewEventConsumer(cfg, 100)
	require.NoError(t, err)
	require.NotNil(t, consumer)

	assert.NotNil(t, consumer.Events())
	assert.NotNil(t, consumer.Errors())
	assert.False(t, consumer.IsRunning())
}

func TestNewAlertProducer(t *testing.T) {
	cfg := DefaultProducerConfig()

	producer, err := NewAlertProducer(cfg, 100)
	require.NoError(t, err)
	require.NotNil(t, producer)

	assert.NotNil(t, producer.Alerts())
	assert.False(t, producer.IsRunning())
}

func TestEventConsumerDefaultBuffer(t *testing.T) {
	cfg := DefaultConsumerConfig()

	// Test with zero buffer (should default to 1000)
	consumer, err := NewEventConsumer(cfg, 0)
	require.NoError(t, err)
	require.NotNil(t, consumer)
}

func TestAlertProducerDefaultBuffer(t *testing.T) {
	cfg := DefaultProducerConfig()

	// Test with negative buffer (should default to 500)
	producer, err := NewAlertProducer(cfg, -1)
	require.NoError(t, err)
	require.NotNil(t, producer)
}

func TestEventConsumerStopWithoutStart(t *testing.T) {
	cfg := DefaultConsumerConfig()

	consumer, err := NewEventConsumer(cfg, 100)
	require.NoError(t, err)

	// Should not error when stopping without starting
	err = consumer.Stop()
	assert.NoError(t, err)
}

func TestAlertProducerStopWithoutStart(t *testing.T) {
	cfg := DefaultProducerConfig()

	producer, err := NewAlertProducer(cfg, 100)
	require.NoError(t, err)

	// Should not error when stopping without starting
	err = producer.Stop()
	assert.NoError(t, err)
}

func TestEventConsumerStartStop(t *testing.T) {
	cfg := DefaultConsumerConfig()

	consumer, err := NewEventConsumer(cfg, 100)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should work (but connection will fail - that's ok for unit test)
	err = consumer.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, consumer.IsRunning())

	// Cancel context to stop
	cancel()
	time.Sleep(100 * time.Millisecond)

	err = consumer.Stop()
	assert.NoError(t, err)
}

func TestAlertProducerStartStop(t *testing.T) {
	cfg := DefaultProducerConfig()

	producer, err := NewAlertProducer(cfg, 100)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = producer.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, producer.IsRunning())

	cancel()
	time.Sleep(100 * time.Millisecond)

	err = producer.Stop()
	assert.NoError(t, err)
}
