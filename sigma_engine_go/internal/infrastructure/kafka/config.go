// Package kafka provides Kafka configuration for Sigma Engine.
package kafka

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains all Kafka-related configuration.
type Config struct {
	// Enabled controls whether Kafka integration is active
	Enabled bool `yaml:"enabled"`

	// Consumer configuration
	Consumer ConsumerConfig `yaml:"consumer"`

	// Producer configuration
	Producer ProducerConfig `yaml:"producer"`
}

// DefaultConfig returns default Kafka configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:  false, // Disabled by default, use file monitoring
		Consumer: DefaultConsumerConfig(),
		Producer: DefaultProducerConfig(),
	}
}

// LoadFromEnv loads Kafka configuration from environment variables.
func LoadFromEnv() Config {
	cfg := DefaultConfig()

	// KAFKA_ENABLED
	if v := os.Getenv("KAFKA_ENABLED"); v != "" {
		cfg.Enabled = v == "true" || v == "1"
	}

	// KAFKA_BROKERS (comma-separated)
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		brokers := strings.Split(v, ",")
		cfg.Consumer.Brokers = brokers
		cfg.Producer.Brokers = brokers
	}

	// Consumer settings
	if v := os.Getenv("KAFKA_CONSUMER_TOPIC"); v != "" {
		cfg.Consumer.Topic = v
	}
	if v := os.Getenv("KAFKA_CONSUMER_GROUP"); v != "" {
		cfg.Consumer.GroupID = v
	}
	if v := os.Getenv("KAFKA_CONSUMER_OFFSET"); v != "" {
		if offset, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Consumer.StartOffset = offset
		}
	}

	// Producer settings
	if v := os.Getenv("KAFKA_PRODUCER_TOPIC"); v != "" {
		cfg.Producer.Topic = v
	}
	if v := os.Getenv("KAFKA_PRODUCER_BATCH_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Producer.BatchSize = size
		}
	}
	if v := os.Getenv("KAFKA_PRODUCER_COMPRESSION"); v != "" {
		cfg.Producer.Compression = v
	}

	return cfg
}

// Validate validates the Kafka configuration.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if disabled
	}

	if len(c.Consumer.Brokers) == 0 {
		c.Consumer.Brokers = []string{"localhost:9092"}
	}
	if len(c.Producer.Brokers) == 0 {
		c.Producer.Brokers = c.Consumer.Brokers
	}

	if c.Consumer.Topic == "" {
		c.Consumer.Topic = "events-raw"
	}
	if c.Consumer.GroupID == "" {
		c.Consumer.GroupID = "sigma-engine-group"
	}
	if c.Consumer.MaxWait == 0 {
		c.Consumer.MaxWait = 5 * time.Second
	}

	if c.Producer.Topic == "" {
		c.Producer.Topic = "alerts"
	}
	if c.Producer.BatchSize <= 0 {
		c.Producer.BatchSize = 50
	}
	if c.Producer.BatchTimeout == 0 {
		c.Producer.BatchTimeout = 100 * time.Millisecond
	}

	return nil
}
