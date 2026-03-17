// Package cache provides caching infrastructure for the Sigma detection engine.
// This file implements the Redis client provider, connection management,
// and configuration loading from environment variables.
package cache

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/edr-platform/sigma-engine/internal/infrastructure/logger"
	"github.com/redis/go-redis/v9"
)

// RedisConfig holds all configuration needed to connect to Redis.
// Fields are populated from environment variables with sensible fallbacks.
type RedisConfig struct {
	// Addr is the Redis server address in "host:port" format.
	// Env: REDIS_ADDR (default: "localhost:6379")
	Addr string

	// Password for Redis AUTH command. Empty string means no authentication.
	// Env: REDIS_PASSWORD (default: "")
	Password string

	// DB is the Redis logical database index (0–15).
	// Env: REDIS_DB (default: 0)
	DB int

	// DialTimeout is the timeout for establishing a new connection.
	// Env: REDIS_DIAL_TIMEOUT_SEC (default: 5s)
	DialTimeout time.Duration

	// ReadTimeout is the timeout for socket reads.
	// Env: REDIS_READ_TIMEOUT_SEC (default: 3s)
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for socket writes.
	// Env: REDIS_WRITE_TIMEOUT_SEC (default: 3s)
	WriteTimeout time.Duration

	// PoolSize is the maximum number of socket connections in the pool.
	// Env: REDIS_POOL_SIZE (default: 10)
	PoolSize int

	// MinIdleConns is the minimum number of idle connections maintained in the pool.
	// Env: REDIS_MIN_IDLE_CONNS (default: 2)
	MinIdleConns int
}

// RedisConfigFromEnv reads Redis configuration from environment variables,
// using the documented defaults when an environment variable is absent or invalid.
func RedisConfigFromEnv() RedisConfig {
	cfg := RedisConfig{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	}

	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 15 {
			cfg.DB = n
		}
	}
	if v := os.Getenv("REDIS_DIAL_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.DialTimeout = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("REDIS_READ_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ReadTimeout = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("REDIS_WRITE_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WriteTimeout = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("REDIS_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PoolSize = n
		}
	}
	if v := os.Getenv("REDIS_MIN_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MinIdleConns = n
		}
	}

	return cfg
}

// RedisClient wraps a go-redis/v9 client and exposes a safe Close method.
// Obtain a new instance via NewRedisClient.
type RedisClient struct {
	client *redis.Client
	cfg    RedisConfig
}

// NewRedisClient creates a new Redis client, pings the server to verify
// connectivity, and returns a ready-to-use *RedisClient.
//
// Returns an error if the ping fails — the caller can decide whether to
// treat this as a fatal error or degrade gracefully.
func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	// Ping with a short context so startup is not blocked on a down Redis.
	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping failed (addr=%s db=%d): %w", cfg.Addr, cfg.DB, err)
	}

	logger.Infof("Redis connected: addr=%s db=%d pool=%d", cfg.Addr, cfg.DB, cfg.PoolSize)
	return &RedisClient{client: rdb, cfg: cfg}, nil
}

// Client returns the underlying go-redis/v9 client for direct use by
// cache implementations within this package.
func (r *RedisClient) Client() *redis.Client {
	return r.client
}

// Ping sends a PING command and returns an error if the server is unreachable.
// Useful for health-check handlers.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close releases all Redis connections in the pool.
// The client must not be used after Close returns.
func (r *RedisClient) Close() error {
	return r.client.Close()
}
