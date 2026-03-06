// Package kafka provides alert producer for publishing alerts to Kafka.
package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/segmentio/kafka-go"
)

// ProducerConfig configures the Kafka producer.
type ProducerConfig struct {
	Brokers         []string      `yaml:"brokers"`
	Topic           string        `yaml:"topic"`
	BatchSize       int           `yaml:"batch_size"`
	BatchTimeout    time.Duration `yaml:"batch_timeout"`
	MaxRetries      int           `yaml:"max_retries"`
	RequiredAcks    int           `yaml:"required_acks"` // -1 = all, 1 = leader only
	Compression     string        `yaml:"compression"`   // "snappy", "gzip", "lz4", "none"
	MaxMessageBytes int           `yaml:"max_message_bytes"`
}

// DefaultProducerConfig returns default producer configuration.
func DefaultProducerConfig() ProducerConfig {
	return ProducerConfig{
		Brokers:         []string{"localhost:9092"},
		Topic:           "alerts",
		BatchSize:       50,
		BatchTimeout:    100 * time.Millisecond,
		MaxRetries:      3,
		RequiredAcks:    -1, // -1 = all, 1 = leader only
		Compression:     "snappy",
		MaxMessageBytes: 10e6, // 10MB
	}
}

// ProducerMetrics tracks producer statistics.
type ProducerMetrics struct {
	AlertsPublished  uint64
	PublishErrors    uint64
	BytesSent        uint64
	BatchesPublished uint64
	LastPublishTime  time.Time
	mu               sync.RWMutex
}

// Snapshot returns a copy of current metrics.
func (m *ProducerMetrics) Snapshot() ProducerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ProducerMetrics{
		AlertsPublished:  atomic.LoadUint64(&m.AlertsPublished),
		PublishErrors:    atomic.LoadUint64(&m.PublishErrors),
		BytesSent:        atomic.LoadUint64(&m.BytesSent),
		BatchesPublished: atomic.LoadUint64(&m.BatchesPublished),
		LastPublishTime:  m.LastPublishTime,
	}
}

// AlertProducer publishes alerts to Kafka.
type AlertProducer struct {
	writer  *kafka.Writer
	config  ProducerConfig
	metrics *ProducerMetrics

	alertChan chan *domain.Alert
	doneChan  chan struct{}

	running atomic.Bool
	wg      sync.WaitGroup
}

// NewAlertProducer creates a new Kafka alert producer.
func NewAlertProducer(config ProducerConfig, alertBuffer int) (*AlertProducer, error) {
	if alertBuffer <= 0 {
		alertBuffer = 500
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Topic:        config.Topic,
		Balancer:     &kafka.Hash{}, // Partition by key (agent_id)
		BatchSize:    config.BatchSize,
		BatchTimeout: config.BatchTimeout,
		MaxAttempts:  config.MaxRetries,
		RequiredAcks: kafka.RequiredAcks(config.RequiredAcks),
		Compression:  kafka.Snappy,
		ErrorLogger:  kafka.LoggerFunc(func(msg string, args ...interface{}) { logger.Errorf(msg, args...) }),
	}

	return &AlertProducer{
		writer:    writer,
		config:    config,
		metrics:   &ProducerMetrics{},
		alertChan: make(chan *domain.Alert, alertBuffer),
		doneChan:  make(chan struct{}),
	}, nil
}

// Start begins the producer background worker.
func (p *AlertProducer) Start(ctx context.Context) error {
	if p.running.Load() {
		return nil
	}
	p.running.Store(true)

	logger.Infof("Starting Kafka alert producer: brokers=%v topic=%s",
		p.config.Brokers, p.config.Topic)

	p.wg.Add(1)
	go p.publishLoop(ctx)

	return nil
}

// publishLoop batches and publishes alerts.
func (p *AlertProducer) publishLoop(ctx context.Context) {
	defer p.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Panic recovered in publishLoop: %v", r)
		}
	}()

	batch := make([]kafka.Message, 0, p.config.BatchSize)
	ticker := time.NewTicker(p.config.BatchTimeout)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := p.writer.WriteMessages(flushCtx, batch...); err != nil {
			atomic.AddUint64(&p.metrics.PublishErrors, uint64(len(batch)))
			logger.Errorf("Failed to publish %d alerts to Kafka: %v", len(batch), err)
		} else {
			atomic.AddUint64(&p.metrics.AlertsPublished, uint64(len(batch)))
			atomic.AddUint64(&p.metrics.BatchesPublished, 1)
			p.metrics.mu.Lock()
			p.metrics.LastPublishTime = time.Now()
			p.metrics.mu.Unlock()
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-p.doneChan:
			flush()
			return
		case alert, ok := <-p.alertChan:
			if !ok {
				flush()
				return
			}

			msg, err := p.alertToMessage(alert)
			if err != nil {
				logger.Warnf("Failed to serialize alert: %v", err)
				atomic.AddUint64(&p.metrics.PublishErrors, 1)
				continue
			}

			batch = append(batch, msg)
			atomic.AddUint64(&p.metrics.BytesSent, uint64(len(msg.Value)))

			if len(batch) >= p.config.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// alertToMessage converts an alert to a Kafka message.
func (p *AlertProducer) alertToMessage(alert *domain.Alert) (kafka.Message, error) {
	data, err := json.Marshal(alert)
	if err != nil {
		return kafka.Message{}, err
	}

	// Use rule ID as partition key for ordering
	var key []byte
	if alert.RuleID != "" {
		key = []byte(alert.RuleID)
	}

	return kafka.Message{
		Key:   key,
		Value: data,
		Time:  alert.Timestamp,
		Headers: []kafka.Header{
			{Key: "severity", Value: []byte(alert.Severity.String())},
			{Key: "rule_id", Value: []byte(alert.RuleID)},
		},
	}, nil
}

// Publish adds an alert to the publish queue.
func (p *AlertProducer) Publish(alert *domain.Alert) error {
	if !p.running.Load() {
		return nil
	}

	select {
	case p.alertChan <- alert:
		return nil
	case <-time.After(1 * time.Second):
		atomic.AddUint64(&p.metrics.PublishErrors, 1)
		return context.DeadlineExceeded
	}
}

// PublishSync publishes an alert synchronously.
func (p *AlertProducer) PublishSync(ctx context.Context, alert *domain.Alert) error {
	msg, err := p.alertToMessage(alert)
	if err != nil {
		return err
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		atomic.AddUint64(&p.metrics.PublishErrors, 1)
		return err
	}

	atomic.AddUint64(&p.metrics.AlertsPublished, 1)
	p.metrics.mu.Lock()
	p.metrics.LastPublishTime = time.Now()
	p.metrics.mu.Unlock()
	return nil
}

// Alerts returns the channel for receiving alerts to publish.
func (p *AlertProducer) Alerts() chan<- *domain.Alert {
	return p.alertChan
}

// Metrics returns producer metrics.
func (p *AlertProducer) Metrics() ProducerMetrics {
	return p.metrics.Snapshot()
}

// Stop gracefully stops the producer.
func (p *AlertProducer) Stop() error {
	if !p.running.Load() {
		return nil
	}
	p.running.Store(false)

	logger.Info("Stopping Kafka alert producer...")
	close(p.doneChan)
	p.wg.Wait()

	if err := p.writer.Close(); err != nil {
		logger.Errorf("Error closing Kafka writer: %v", err)
		return err
	}

	logger.Info("Kafka alert producer stopped")
	return nil
}

// IsRunning returns whether the producer is running.
func (p *AlertProducer) IsRunning() bool {
	return p.running.Load()
}
