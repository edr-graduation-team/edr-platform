// Package kafka provides Kafka consumer implementations.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/pkg/metrics"
)

// ConsumerConfig holds Kafka consumer configuration.
type ConsumerConfig struct {
	Brokers           []string
	Topic             string
	GroupID           string
	StartOffset       string // "newest" or "oldest"
	MinBytes          int
	MaxBytes          int
	MaxWait           time.Duration
	CommitInterval    time.Duration
	HeartbeatInterval time.Duration
	SessionTimeout    time.Duration
}

// DefaultConsumerConfig returns sensible defaults.
func DefaultConsumerConfig(brokers []string, topic, groupID string) *ConsumerConfig {
	return &ConsumerConfig{
		Brokers:           brokers,
		Topic:             topic,
		GroupID:           groupID,
		StartOffset:       "newest",
		MinBytes:          1024,     // 1KB
		MaxBytes:          10485760, // 10MB
		MaxWait:           3 * time.Second,
		CommitInterval:    time.Second,
		HeartbeatInterval: 3 * time.Second,
		SessionTimeout:    30 * time.Second,
	}
}

// EventConsumer consumes events from Kafka.
type EventConsumer struct {
	reader  *kafka.Reader
	config  *ConsumerConfig
	metrics *metrics.Metrics
	logger  *logrus.Logger
	handler MessageHandler
}

// MessageHandler processes consumed messages.
type MessageHandler func(ctx context.Context, msg *ConsumedMessage) error

// ConsumedMessage represents a consumed Kafka message.
type ConsumedMessage struct {
	Key       string
	Value     []byte
	Headers   map[string]string
	Partition int
	Offset    int64
	Topic     string
	Timestamp time.Time
}

// NewEventConsumer creates a new Kafka consumer.
func NewEventConsumer(cfg *ConsumerConfig, m *metrics.Metrics, logger *logrus.Logger, handler MessageHandler) (*EventConsumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker is required")
	}

	startOffset := kafka.LastOffset
	if cfg.StartOffset == "oldest" {
		startOffset = kafka.FirstOffset
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:           cfg.Brokers,
		Topic:             cfg.Topic,
		GroupID:           cfg.GroupID,
		MinBytes:          cfg.MinBytes,
		MaxBytes:          cfg.MaxBytes,
		MaxWait:           cfg.MaxWait,
		CommitInterval:    cfg.CommitInterval,
		HeartbeatInterval: cfg.HeartbeatInterval,
		SessionTimeout:    cfg.SessionTimeout,
		StartOffset:       startOffset,
	})

	logger.WithFields(logrus.Fields{
		"brokers":  cfg.Brokers,
		"topic":    cfg.Topic,
		"group_id": cfg.GroupID,
	}).Info("Kafka consumer initialized")

	return &EventConsumer{
		reader:  reader,
		config:  cfg,
		metrics: m,
		logger:  logger,
		handler: handler,
	}, nil
}

// Start begins consuming messages.
func (c *EventConsumer) Start(ctx context.Context) error {
	c.logger.WithField("topic", c.config.Topic).Info("Starting Kafka consumer")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Kafka consumer stopped by context")
			return ctx.Err()
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				c.logger.WithError(err).Error("Error fetching message")
				if c.metrics != nil {
					c.metrics.ErrorsTotal.WithLabelValues("kafka_fetch").Inc()
				}
				continue
			}

			// Convert to our message type
			consumed := &ConsumedMessage{
				Key:       string(msg.Key),
				Value:     msg.Value,
				Headers:   make(map[string]string),
				Partition: msg.Partition,
				Offset:    msg.Offset,
				Topic:     msg.Topic,
				Timestamp: msg.Time,
			}
			for _, h := range msg.Headers {
				consumed.Headers[h.Key] = string(h.Value)
			}

			// Process message
			if err := c.handler(ctx, consumed); err != nil {
				c.logger.WithFields(logrus.Fields{
					"key":       consumed.Key,
					"partition": consumed.Partition,
					"offset":    consumed.Offset,
					"error":     err.Error(),
				}).Error("Error processing message")

				if c.metrics != nil {
					c.metrics.ErrorsTotal.WithLabelValues("kafka_process").Inc()
				}
			}

			// Commit offset
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				c.logger.WithError(err).Error("Error committing message")
			}
		}
	}
}

// Stats returns consumer statistics.
func (c *EventConsumer) Stats() kafka.ReaderStats {
	return c.reader.Stats()
}

// Close closes the consumer.
func (c *EventConsumer) Close() error {
	c.logger.Info("Closing Kafka consumer...")
	return c.reader.Close()
}

// DLQMonitor monitors the Dead Letter Queue.
type DLQMonitor struct {
	consumer *EventConsumer
	logger   *logrus.Logger
}

// DLQEntry represents a DLQ message.
type DLQEntry struct {
	OriginalKey   string    `json:"original_key"`
	Error         string    `json:"error"`
	FailedAt      time.Time `json:"failed_at"`
	Payload       []byte    `json:"payload"`
	OriginalTopic string    `json:"original_topic"`
}

// NewDLQMonitor creates a DLQ monitor.
func NewDLQMonitor(brokers []string, m *metrics.Metrics, logger *logrus.Logger) (*DLQMonitor, error) {
	cfg := DefaultConsumerConfig(brokers, "events-dlq", "dlq-monitor")
	cfg.StartOffset = "oldest"

	handler := func(ctx context.Context, msg *ConsumedMessage) error {
		entry := &DLQEntry{
			OriginalKey:   msg.Headers["original_key"],
			Error:         msg.Headers["error"],
			Payload:       msg.Value,
			OriginalTopic: msg.Headers["original_topic"],
		}
		if failedAt, ok := msg.Headers["failed_at"]; ok {
			entry.FailedAt, _ = time.Parse(time.RFC3339, failedAt)
		}

		// Log DLQ entry for alerting/investigation
		logger.WithFields(logrus.Fields{
			"original_key": entry.OriginalKey,
			"error":        entry.Error,
			"failed_at":    entry.FailedAt,
			"payload_size": len(entry.Payload),
		}).Warn("DLQ entry detected")

		// Record metric
		if m != nil {
			m.ErrorsTotal.WithLabelValues("dlq_entry").Inc()
		}

		return nil
	}

	consumer, err := NewEventConsumer(cfg, m, logger, handler)
	if err != nil {
		return nil, err
	}

	return &DLQMonitor{
		consumer: consumer,
		logger:   logger,
	}, nil
}

// Start begins monitoring the DLQ.
func (d *DLQMonitor) Start(ctx context.Context) error {
	d.logger.Info("Starting DLQ monitor")
	return d.consumer.Start(ctx)
}

// Close closes the DLQ monitor.
func (d *DLQMonitor) Close() error {
	return d.consumer.Close()
}

// GetDLQEntries retrieves recent DLQ entries (for admin API).
func (d *DLQMonitor) GetDLQEntries(ctx context.Context, limit int) ([]DLQEntry, error) {
	// This would typically query from storage or return cached entries
	// For now, return empty as DLQ monitoring is passive
	return nil, nil
}

// ReplayDLQEntry replays a DLQ entry to the main topic.
func ReplayDLQEntry(ctx context.Context, producer *EventProducer, entry *DLQEntry) error {
	return producer.SendEventBatch(ctx, entry.OriginalKey, entry.Payload, map[string]string{
		"replayed_from_dlq": "true",
		"original_error":    entry.Error,
	})
}

// Utility to serialize for logging
func (e *DLQEntry) ToJSON() string {
	data, _ := json.Marshal(e)
	return string(data)
}
