// Package handlers provides gRPC handler implementations.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
)

// EventFallbackStore provides PostgreSQL-based fallback storage for event
// batches when the primary Kafka pipeline is unavailable.
//
// DESIGN DECISION: This is intentionally simple — a single table, single
// INSERT, no complex repository interfaces. In an EDR system, the fallback
// path must be as reliable as possible, and simplicity = reliability.
//
// The fallback table acts as a durable buffer. A separate background worker
// (not implemented here) should periodically read from this table and
// replay events to Kafka once connectivity is restored.
//
// Schema expected:
//
//	CREATE TABLE IF NOT EXISTS event_batches_fallback (
//	    id          BIGSERIAL PRIMARY KEY,
//	    batch_id    TEXT NOT NULL UNIQUE,
//	    agent_id    TEXT NOT NULL,
//	    payload     BYTEA NOT NULL,
//	    metadata    JSONB,
//	    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    replayed    BOOLEAN NOT NULL DEFAULT FALSE,
//	    replayed_at TIMESTAMPTZ
//	);
//	CREATE INDEX idx_fallback_unreplayed ON event_batches_fallback (replayed) WHERE NOT replayed;
type EventFallbackStore struct {
	pool   *pgxpool.Pool
	logger *logrus.Logger
}

// NewEventFallbackStore creates a new fallback store.
// Returns nil if pool is nil (DB not configured), allowing callers to
// simply nil-check before use.
func NewEventFallbackStore(pool *pgxpool.Pool, logger *logrus.Logger) *EventFallbackStore {
	if pool == nil {
		return nil
	}
	return &EventFallbackStore{
		pool:   pool,
		logger: logger,
	}
}

// Store persists an event batch to PostgreSQL as a fallback.
// This is the last line of defense against data loss — it only runs when
// both Kafka primary and DLQ writes have failed.
//
// Returns nil on success, error on failure. Callers should log but NOT
// propagate the error to the agent — we want the agent to keep streaming
// rather than disconnecting on a transient DB error.
func (s *EventFallbackStore) Store(ctx context.Context, batchID, agentID string, payload []byte, metadata map[string]string) error {
	// Serialize metadata to JSON; nil/empty map yields valid JSON "null"
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := `
		INSERT INTO event_batches_fallback (batch_id, agent_id, payload, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (batch_id) DO NOTHING
	`

	// Use a 5s timeout to prevent a slow DB from blocking the stream
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = s.pool.Exec(ctx, query, batchID, agentID, payload, metadataJSON, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert fallback batch: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"batch_id": batchID,
		"agent_id": agentID,
		"size":     len(payload),
	}).Warn("Event batch saved to DB fallback (Kafka unavailable)")

	return nil
}

// EnsureTable creates the fallback table if it doesn't exist.
// Call this during server startup so the fallback path is always available.
func (s *EventFallbackStore) EnsureTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS event_batches_fallback (
			id          BIGSERIAL PRIMARY KEY,
			batch_id    TEXT NOT NULL UNIQUE,
			agent_id    TEXT NOT NULL,
			payload     BYTEA NOT NULL,
			metadata    JSONB,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			replayed    BOOLEAN NOT NULL DEFAULT FALSE,
			replayed_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_fallback_unreplayed
			ON event_batches_fallback (replayed) WHERE NOT replayed;
	`

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("create fallback table: %w", err)
	}

	s.logger.Info("Event fallback table ensured")
	return nil
}
