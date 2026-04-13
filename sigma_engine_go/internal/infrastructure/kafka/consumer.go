// Package kafka provides Kafka consumer and producer for Sigma Engine.
// This enables real-time event processing from Kafka topics instead of file-based input.
package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	metricsPkg "github.com/edr-platform/sigma-engine/internal/metrics"
	"github.com/segmentio/kafka-go"
)

// ConsumerConfig configures the Kafka consumer.
type ConsumerConfig struct {
	Brokers        []string      `yaml:"brokers"`
	Topic          string        `yaml:"topic"`
	GroupID        string        `yaml:"group_id"`
	MinBytes       int           `yaml:"min_bytes"`
	MaxBytes       int           `yaml:"max_bytes"`
	MaxWait        time.Duration `yaml:"max_wait"`
	CommitInterval time.Duration `yaml:"commit_interval"`
	StartOffset    int64         `yaml:"start_offset"` // -1 = latest, -2 = earliest
	// S1 FIX: Number of parallel reader goroutines that call ReadMessage().
	// More readers = better partition-level parallelism for multi-partition topics.
	ConsumerReaders int `yaml:"consumer_readers"`
}

// DefaultConsumerConfig returns default consumer configuration.
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		Brokers:         []string{"localhost:9092"},
		Topic:           "events-raw",
		GroupID:         "sigma-engine-group",
		MinBytes:        1,
		MaxBytes:        10e6, // 10MB
		MaxWait:         5 * time.Second,
		CommitInterval:  1 * time.Second,
		StartOffset:     kafka.LastOffset, // -1 = latest
		ConsumerReaders: 2,                // S1 FIX: default 2 parallel readers
	}
}

// ConsumerMetrics tracks consumer statistics.
type ConsumerMetrics struct {
	MessagesConsumed  uint64
	MessagesProcessed uint64
	DeserializeErrors uint64
	ProcessingErrors  uint64
	BatchesProcessed  uint64
	LastMessageTime   time.Time
	ConsumerLag       int64
	mu                sync.RWMutex
}

// Snapshot returns a copy of current metrics.
func (m *ConsumerMetrics) Snapshot() ConsumerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ConsumerMetrics{
		MessagesConsumed:  atomic.LoadUint64(&m.MessagesConsumed),
		MessagesProcessed: atomic.LoadUint64(&m.MessagesProcessed),
		DeserializeErrors: atomic.LoadUint64(&m.DeserializeErrors),
		ProcessingErrors:  atomic.LoadUint64(&m.ProcessingErrors),
		BatchesProcessed:  atomic.LoadUint64(&m.BatchesProcessed),
		LastMessageTime:   m.LastMessageTime,
		ConsumerLag:       atomic.LoadInt64(&m.ConsumerLag),
	}
}

// EventConsumer consumes events from Kafka and converts them to LogEvent.
type EventConsumer struct {
	reader  *kafka.Reader
	config  ConsumerConfig
	metrics *ConsumerMetrics

	eventChan chan *domain.LogEvent
	errorChan chan error
	doneChan  chan struct{}

	running   atomic.Bool
	wg        sync.WaitGroup
	closeOnce sync.Once // S1 FIX: protect channel close from multiple goroutines
}

// NewEventConsumer creates a new Kafka event consumer.
func NewEventConsumer(config ConsumerConfig, eventBuffer int) (*EventConsumer, error) {
	if eventBuffer <= 0 {
		eventBuffer = 1000
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.GroupID,
		MinBytes:       config.MinBytes,
		MaxBytes:       config.MaxBytes,
		MaxWait:        config.MaxWait,
		CommitInterval: config.CommitInterval,
		StartOffset:    config.StartOffset,
		ErrorLogger:    kafka.LoggerFunc(func(msg string, args ...interface{}) { logger.Errorf(msg, args...) }),
	})

	return &EventConsumer{
		reader:    reader,
		config:    config,
		metrics:   &ConsumerMetrics{},
		eventChan: make(chan *domain.LogEvent, eventBuffer),
		errorChan: make(chan error, 100),
		doneChan:  make(chan struct{}),
	}, nil
}

// Start begins consuming messages from Kafka.
func (c *EventConsumer) Start(ctx context.Context) error {
	if c.running.Load() {
		return nil
	}
	c.running.Store(true)

	readers := c.config.ConsumerReaders
	if readers <= 0 {
		readers = 2
	}

	logger.Infof("Starting Kafka consumer: brokers=%v topic=%s group=%s readers=%d",
		c.config.Brokers, c.config.Topic, c.config.GroupID, readers)

	// S1 FIX: Spawn multiple consumeLoop goroutines for partition-parallel reads.
	// segmentio/kafka-go Reader.ReadMessage() is concurrency-safe in consumer-group mode.
	for i := 0; i < readers; i++ {
		c.wg.Add(1)
		go c.consumeLoop(ctx, i)
	}

	return nil
}

// consumeLoop is the main consumer loop. Multiple instances may run in parallel (S1).
func (c *EventConsumer) consumeLoop(ctx context.Context, readerID int) {
	defer c.wg.Done()
	// Only close channels once across all goroutines (first to exit wins).
	defer c.closeOnce.Do(func() {
		close(c.eventChan)
		close(c.errorChan)
	})
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered in consumeLoop[%d]: %v", readerID, r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Consumer context cancelled, stopping...")
			return
		case <-c.doneChan:
			logger.Info("Consumer stop requested, shutting down...")
			return
		default:
			// Read message with timeout
			readCtx, cancel := context.WithTimeout(ctx, c.config.MaxWait)
			msg, err := c.reader.ReadMessage(readCtx)
			cancel()

			if err != nil {
				if ctx.Err() != nil {
					return // Context cancelled
				}
				if err == context.DeadlineExceeded {
					continue // No messages, retry
				}
				logger.Warnf("Error reading Kafka message: %v", err)
				select {
				case c.errorChan <- err:
				default:
				}
				continue
			}

			atomic.AddUint64(&c.metrics.MessagesConsumed, 1)
			c.metrics.mu.Lock()
			c.metrics.LastMessageTime = time.Now()
			c.metrics.mu.Unlock()

			// Convert to LogEvent
			event, err := c.parseMessage(msg)
			if err != nil {
				atomic.AddUint64(&c.metrics.DeserializeErrors, 1)
				logger.Debugf("Failed to parse Kafka message: %v", err)
				continue
			}

			// Send to channel (with short timeout to prevent blocking)
			// S8 FIX: Reduced from 5s to 500ms. Under backlog, 5s stalls per
			// dropped event cascaded into unrecoverable consumer lag.
			select {
			case c.eventChan <- event:
				atomic.AddUint64(&c.metrics.MessagesProcessed, 1)
			case <-time.After(500 * time.Millisecond):
				logger.Warn("Event channel full, dropping message (500ms timeout)")
				atomic.AddUint64(&c.metrics.ProcessingErrors, 1)
				metricsPkg.DefaultMetrics.RecordError("consumer_event_channel_full_drop")
			case <-ctx.Done():
				return
			}
		}
	}
}

// parseMessage converts a Kafka message to LogEvent.
func (c *EventConsumer) parseMessage(msg kafka.Message) (*domain.LogEvent, error) {
	// Parse JSON payload
	var rawData map[string]interface{}
	if err := json.Unmarshal(msg.Value, &rawData); err != nil {
		return nil, err
	}

	// Add Kafka metadata
	rawData["_kafka_partition"] = msg.Partition
	rawData["_kafka_offset"] = msg.Offset
	rawData["_kafka_topic"] = msg.Topic
	rawData["_kafka_key"] = string(msg.Key)
	rawData["_kafka_time"] = msg.Time.Format(time.RFC3339)

	// Create LogEvent
	return domain.NewLogEvent(rawData)
}

// Events returns the channel for receiving parsed events.
func (c *EventConsumer) Events() <-chan *domain.LogEvent {
	return c.eventChan
}

// Errors returns the channel for receiving errors.
func (c *EventConsumer) Errors() <-chan error {
	return c.errorChan
}

// Metrics returns consumer metrics.
func (c *EventConsumer) Metrics() ConsumerMetrics {
	return c.metrics.Snapshot()
}

// Stop gracefully stops the consumer.
func (c *EventConsumer) Stop() error {
	if !c.running.Load() {
		return nil
	}
	c.running.Store(false)

	logger.Info("Stopping Kafka consumer...")
	close(c.doneChan)
	c.wg.Wait()

	if err := c.reader.Close(); err != nil {
		logger.Errorf("Error closing Kafka reader: %v", err)
		return err
	}

	logger.Info("Kafka consumer stopped")
	return nil
}

// IsRunning returns whether the consumer is running.
func (c *EventConsumer) IsRunning() bool {
	return c.running.Load()
}

// Lag returns the current consumer lag.
func (c *EventConsumer) Lag() int64 {
	return atomic.LoadInt64(&c.metrics.ConsumerLag)
}
