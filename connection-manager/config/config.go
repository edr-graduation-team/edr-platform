// Package config provides configuration management for the connection-manager.
// It supports loading from YAML files and environment variable overrides.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Config holds all configuration for the connection-manager server.
type Config struct {
	Server     ServerConfig     `mapstructure:"server" validate:"required"`
	Database   DatabaseConfig   `mapstructure:"database" validate:"required"`
	Redis      RedisConfig      `mapstructure:"redis" validate:"required"`
	JWT        JWTConfig        `mapstructure:"jwt" validate:"required"`
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	API        APIConfig        `mapstructure:"api"`
	RateLimit  RateLimitConfig  `mapstructure:"rate_limit"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	SMTP       SMTPConfig       `mapstructure:"smtp"`
}

// SMTPConfig holds outbound email (transactional) settings, currently used
// for MFA verification codes. Credentials are intended to come from env vars
// (SMTP_HOST / SMTP_PORT / SMTP_USERNAME / SMTP_PASSWORD / SMTP_FROM) so the
// YAML never ships with a real password.
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
	UseTLS   bool   `mapstructure:"use_tls"`   // implicit TLS on the socket (port 465)
	StartTLS bool   `mapstructure:"start_tls"` // explicit STARTTLS upgrade (port 587)
	Enabled  bool   `mapstructure:"enabled"`
}

// ServerConfig holds gRPC and HTTP server settings.
type ServerConfig struct {
	GRPCPort    int    `mapstructure:"grpc_port" validate:"required,min=1,max=65535"`
	HTTPPort    int    `mapstructure:"http_port" validate:"required,min=1,max=65535"`
	TLSCertPath string `mapstructure:"tls_cert_path" validate:"required"`
	TLSKeyPath  string `mapstructure:"tls_key_path" validate:"required"`
	CACertPath  string `mapstructure:"ca_cert_path" validate:"required"`

	// gRPC options
	MaxConcurrentStreams uint32        `mapstructure:"max_concurrent_streams"`
	KeepaliveTime        time.Duration `mapstructure:"keepalive_time"`
	KeepaliveTimeout     time.Duration `mapstructure:"keepalive_timeout"`
	MaxConnectionIdle    time.Duration `mapstructure:"max_connection_idle"`
	ReadTimeout          time.Duration `mapstructure:"read_timeout"`
	WriteTimeout         time.Duration `mapstructure:"write_timeout"`

	// Graceful shutdown
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host            string        `mapstructure:"host" validate:"required"`
	Port            int           `mapstructure:"port" validate:"required,min=1,max=65535"`
	User            string        `mapstructure:"user" validate:"required"`
	Password        string        `mapstructure:"password" validate:"required"`
	Name            string        `mapstructure:"name" validate:"required"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

// ConnectionString returns the PostgreSQL connection string.
func (d *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr         string        `mapstructure:"addr" validate:"required"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	PoolTimeout  time.Duration `mapstructure:"pool_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	UseTLS       bool          `mapstructure:"use_tls"`
}

// JWTConfig holds JWT token settings.
type JWTConfig struct {
	PrivateKeyPath string        `mapstructure:"private_key_path" validate:"required"`
	PublicKeyPath  string        `mapstructure:"public_key_path" validate:"required"`
	Issuer         string        `mapstructure:"issuer"`
	Audience       string        `mapstructure:"audience"`
	AccessTTL      time.Duration `mapstructure:"access_ttl"`
	RefreshTTL     time.Duration `mapstructure:"refresh_ttl"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	EventsPerSecond int     `mapstructure:"events_per_second"`
	BurstMultiplier float64 `mapstructure:"burst_multiplier"`
	Enabled         bool    `mapstructure:"enabled"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"` // "json" or "text"
	Output     string `mapstructure:"output"` // "stdout", "stderr", or file path
	TimeFormat string `mapstructure:"time_format"`
}

// MonitoringConfig holds monitoring settings.
type MonitoringConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	MetricsPath    string `mapstructure:"metrics_path"`
	HealthPath     string `mapstructure:"health_path"`
	TracingEnabled bool   `mapstructure:"tracing_enabled"`
	TracingBackend string `mapstructure:"tracing_backend"` // "jaeger", "zipkin"
}

// KafkaConfig holds Kafka settings (Phase 2).
type KafkaConfig struct {
	Brokers     []string      `mapstructure:"brokers"`
	Topic       string        `mapstructure:"topic"`
	DLQTopic    string        `mapstructure:"dlq_topic"`
	Compression string        `mapstructure:"compression"` // "snappy", "gzip", "lz4", "zstd"
	Acks        string        `mapstructure:"acks"`        // "none", "one", "all"
	MaxRetries  int           `mapstructure:"max_retries"`
	BatchSize   int           `mapstructure:"batch_size"`
	Timeout     time.Duration `mapstructure:"timeout"`
	Enabled     bool          `mapstructure:"enabled"`
}

// APIConfig holds REST API settings (Phase 2).
type APIConfig struct {
	Port               int           `mapstructure:"port"`
	CORSAllowOrigins   []string      `mapstructure:"cors_allow_origins"`
	CORSAllowMethods   []string      `mapstructure:"cors_allow_methods"`
	RateLimitRequests  int           `mapstructure:"rate_limit_requests"`
	RateLimitWindow    time.Duration `mapstructure:"rate_limit_window"`
	RequestTimeout     time.Duration `mapstructure:"request_timeout"`
	MaxRequestBodySize int           `mapstructure:"max_request_body_size"`
	Enabled            bool          `mapstructure:"enabled"`
}

// Load loads configuration from file and environment variables.
// Environment variables take precedence over file values.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read from config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Enable environment variable overrides
	v.SetEnvPrefix("EDR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific env vars
	bindEnvVars(v)

	// Unmarshal config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// DATABASE_URL overrides database.* when set (e.g. for Docker)
	if u := os.Getenv("DATABASE_URL"); u != "" {
		if err := applyDatabaseURL(&cfg, u); err != nil {
			return nil, fmt.Errorf("invalid DATABASE_URL: %w", err)
		}
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// applyDatabaseURL parses a postgres/postgresql URL and sets cfg.Database.
// Format: postgres://user:password@host:port/dbname?sslmode=...
func applyDatabaseURL(cfg *Config, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if u.Hostname() != "" {
		cfg.Database.Host = u.Hostname()
	}
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		cfg.Database.Port = p
	}
	if u.User != nil {
		cfg.Database.User = u.User.Username()
		if p, ok := u.User.Password(); ok {
			cfg.Database.Password = p
		}
	}
	if u.Path != "" {
		cfg.Database.Name = strings.TrimPrefix(u.Path, "/")
	}
	if q := u.Query().Get("sslmode"); q != "" {
		cfg.Database.SSLMode = q
	}
	return nil
}

// Validate validates the configuration using struct tags.
func (c *Config) Validate() error {
	validate := validator.New()
	return validate.Struct(c)
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.grpc_port", 50051)
	v.SetDefault("server.http_port", 8090)
	v.SetDefault("server.max_concurrent_streams", 1000)
	v.SetDefault("server.keepalive_time", 30*time.Second)
	v.SetDefault("server.keepalive_timeout", 10*time.Second)
	v.SetDefault("server.max_connection_idle", 5*time.Minute)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 30*time.Second)
	v.SetDefault("server.shutdown_timeout", 30*time.Second)

	// Database defaults
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.ssl_mode", "require")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)
	v.SetDefault("database.conn_max_idle_time", 5*time.Minute)

	// Redis defaults
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 10)
	v.SetDefault("redis.pool_timeout", 30*time.Second)
	v.SetDefault("redis.read_timeout", 3*time.Second)
	v.SetDefault("redis.write_timeout", 3*time.Second)

	// JWT defaults
	v.SetDefault("jwt.issuer", "antigravity-server")
	v.SetDefault("jwt.audience", "agent")
	v.SetDefault("jwt.access_ttl", 24*time.Hour)
	v.SetDefault("jwt.refresh_ttl", 90*24*time.Hour) // 90 days

	// Rate limit defaults
	v.SetDefault("rate_limit.events_per_second", 10000)
	v.SetDefault("rate_limit.burst_multiplier", 1.2)
	v.SetDefault("rate_limit.enabled", true)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.time_format", "2006-01-02T15:04:05.000Z07:00")

	// Monitoring defaults
	v.SetDefault("monitoring.enabled", true)
	v.SetDefault("monitoring.metrics_path", "/metrics")
	v.SetDefault("monitoring.health_path", "/healthz")
	v.SetDefault("monitoring.tracing_enabled", false)

	// Kafka defaults (Phase 2)
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.topic", "events-raw")
	v.SetDefault("kafka.dlq_topic", "events-dlq")
	v.SetDefault("kafka.compression", "snappy")
	v.SetDefault("kafka.acks", "all")
	v.SetDefault("kafka.max_retries", 3)
	v.SetDefault("kafka.batch_size", 16384)
	v.SetDefault("kafka.timeout", 30*time.Second)
	v.SetDefault("kafka.enabled", true)

	// SMTP defaults — credentials MUST come from env vars in production.
	v.SetDefault("smtp.host", "smtp.hostinger.com")
	v.SetDefault("smtp.port", 465)
	v.SetDefault("smtp.use_tls", true)
	v.SetDefault("smtp.start_tls", false)
	v.SetDefault("smtp.from", "")
	v.SetDefault("smtp.enabled", false)

	// API defaults (Phase 2)
	v.SetDefault("api.port", 8080)
	v.SetDefault("api.cors_allow_origins", []string{"*"})
	v.SetDefault("api.cors_allow_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE"})
	v.SetDefault("api.rate_limit_requests", 100)
	v.SetDefault("api.rate_limit_window", time.Minute)
	v.SetDefault("api.request_timeout", 30*time.Second)
	v.SetDefault("api.max_request_body_size", 10*1024*1024) // 10MB
	v.SetDefault("api.enabled", true)
}

// bindEnvVars binds specific environment variables.
func bindEnvVars(v *viper.Viper) {
	// Database (DATABASE_URL is applied in Load() and overrides these when set)
	v.BindEnv("database.host", "DATABASE_HOST")
	v.BindEnv("database.port", "DATABASE_PORT")
	v.BindEnv("database.user", "DATABASE_USER")
	v.BindEnv("database.password", "DATABASE_PASSWORD")
	v.BindEnv("database.name", "DATABASE_NAME")

	// Redis
	v.BindEnv("redis.addr", "REDIS_ADDR")
	v.BindEnv("redis.password", "REDIS_PASSWORD")

	// Server
	v.BindEnv("server.grpc_port", "GRPC_PORT")
	v.BindEnv("server.http_port", "HTTP_PORT")

	// Logging
	v.BindEnv("logging.level", "LOG_LEVEL")

	// Kafka (Phase 2)
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.enabled", "KAFKA_ENABLED")

	// API (Phase 2)
	v.BindEnv("api.port", "API_PORT")
	v.BindEnv("api.enabled", "API_ENABLED")

	// JWT
	v.BindEnv("jwt.private_key_path", "JWT_PRIVATE_KEY_PATH")
	v.BindEnv("jwt.public_key_path", "JWT_PUBLIC_KEY_PATH")

	// SMTP (transactional email — used by MFA email OTP)
	v.BindEnv("smtp.host", "SMTP_HOST")
	v.BindEnv("smtp.port", "SMTP_PORT")
	v.BindEnv("smtp.username", "SMTP_USERNAME")
	v.BindEnv("smtp.password", "SMTP_PASSWORD")
	v.BindEnv("smtp.from", "SMTP_FROM")
	v.BindEnv("smtp.use_tls", "SMTP_USE_TLS")
	v.BindEnv("smtp.start_tls", "SMTP_START_TLS")
	v.BindEnv("smtp.enabled", "SMTP_ENABLED")
}
