// Package handlers provides gRPC handler implementations.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/pkg/kafka"
)

// =========================================================================
// ASYNC FALLBACK STORE
// =========================================================================

// fallbackItem is one unit of work for the async fallback writer.
type fallbackItem struct {
	batchID  string
	agentID  string
	payload  []byte
	metadata map[string]string
}

// EventFallbackStore provides PostgreSQL-based fallback storage for event
// batches when the primary Kafka pipeline is unavailable.
//
// DESIGN: All writes are asynchronous via a bounded channel. A pool of
// worker goroutines drains the channel and performs INSERT operations.
// This prevents a Kafka outage from blocking the gRPC Recv() loop —
// the main pipeline stays responsive even under high fallback load.
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
	pool    *pgxpool.Pool
	logger  *logrus.Logger
	writeCh chan fallbackItem
	wg      sync.WaitGroup
}

const (
	// fallbackChanSize is the bounded buffer for async fallback writes.
	// When full, the oldest batches are dropped (data loss is already
	// the failure mode — at least the server stays responsive).
	fallbackChanSize = 4096

	// fallbackWorkers is the number of concurrent DB writer goroutines.
	fallbackWorkers = 4
)

// NewEventFallbackStore creates a new async fallback store.
// Returns nil if pool is nil (DB not configured), allowing callers to
// simply nil-check before use. Starts background workers immediately.
func NewEventFallbackStore(pool *pgxpool.Pool, logger *logrus.Logger) *EventFallbackStore {
	if pool == nil {
		return nil
	}
	s := &EventFallbackStore{
		pool:    pool,
		logger:  logger,
		writeCh: make(chan fallbackItem, fallbackChanSize),
	}
	// Start writer workers
	for i := 0; i < fallbackWorkers; i++ {
		s.wg.Add(1)
		go s.writerWorker(i)
	}
	logger.Infof("Fallback store started with %d async writer workers (buffer=%d)", fallbackWorkers, fallbackChanSize)
	return s
}

// Store enqueues an event batch for asynchronous PostgreSQL persistence.
// This method is NON-BLOCKING: it pushes to a bounded channel and returns
// immediately. If the channel is full, the batch is dropped with a log warning.
func (s *EventFallbackStore) Store(_ context.Context, batchID, agentID string, payload []byte, metadata map[string]string) error {
	item := fallbackItem{
		batchID:  batchID,
		agentID:  agentID,
		payload:  payload,
		metadata: metadata,
	}
	select {
	case s.writeCh <- item:
		return nil
	default:
		s.logger.WithFields(logrus.Fields{
			"batch_id": batchID,
			"agent_id": agentID,
			"size":     len(payload),
		}).Error("Fallback write channel full — event batch DROPPED (server under extreme load)")
		return fmt.Errorf("fallback channel full")
	}
}

// writerWorker drains the write channel and persists batches to PostgreSQL.
func (s *EventFallbackStore) writerWorker(id int) {
	defer s.wg.Done()
	for item := range s.writeCh {
		s.persistItem(item)
	}
	s.logger.Debugf("Fallback writer worker %d stopped", id)
}

// persistItem performs the actual INSERT for one fallback batch.
func (s *EventFallbackStore) persistItem(item fallbackItem) {
	metadataJSON, err := json.Marshal(item.metadata)
	if err != nil {
		s.logger.WithError(err).Error("Fallback: failed to marshal metadata")
		return
	}

	query := `
		INSERT INTO event_batches_fallback (batch_id, agent_id, payload, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (batch_id) DO NOTHING
	`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.pool.Exec(ctx, query, item.batchID, item.agentID, item.payload, metadataJSON, time.Now().UTC())
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"batch_id": item.batchID,
			"agent_id": item.agentID,
		}).Error("Fallback DB write failed — event data may be lost")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"batch_id": item.batchID,
		"agent_id": item.agentID,
		"size":     len(item.payload),
	}).Warn("Event batch saved to DB fallback (Kafka unavailable)")
}

// Close stops all writer workers and waits for them to drain.
func (s *EventFallbackStore) Close() {
	close(s.writeCh)
	s.wg.Wait()
	s.logger.Info("Fallback store closed (all workers drained)")
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

// =========================================================================
// FALLBACK REPLAY WORKER
// =========================================================================

// FallbackReplayWorker periodically reads unreplayed event batches from the
// PostgreSQL fallback table and re-publishes them to Kafka. Once successfully
// published, the row is marked replayed=true.
//
// This closes the data-loss gap: events that landed in the fallback during
// a Kafka outage are automatically re-injected when Kafka recovers.
type FallbackReplayWorker struct {
	pool     *pgxpool.Pool
	producer *kafka.EventProducer
	logger   *logrus.Logger
	interval time.Duration
	batchSz  int
}

// NewFallbackReplayWorker creates a new replay worker.
// Returns nil if either pool or producer is nil (both required for replay).
func NewFallbackReplayWorker(pool *pgxpool.Pool, producer *kafka.EventProducer, logger *logrus.Logger) *FallbackReplayWorker {
	if pool == nil || producer == nil {
		return nil
	}
	return &FallbackReplayWorker{
		pool:     pool,
		producer: producer,
		logger:   logger,
		interval: 30 * time.Second,
		batchSz:  100,
	}
}

// Start begins the replay loop. Blocks until ctx is cancelled.
func (w *FallbackReplayWorker) Start(ctx context.Context) {
	w.logger.Info("Fallback replay worker started (interval=30s, batch=100)")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Fallback replay worker stopped")
			return
		case <-ticker.C:
			w.replayBatch(ctx)
		}
	}
}

// replayBatch reads up to batchSz unreplayed rows, publishes each to Kafka,
// and marks successfully published rows as replayed.
func (w *FallbackReplayWorker) replayBatch(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := w.pool.Query(queryCtx, `
		SELECT id, batch_id, agent_id, payload, metadata
		FROM event_batches_fallback
		WHERE replayed = FALSE
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, w.batchSz)
	if err != nil {
		w.logger.WithError(err).Warn("Fallback replay: failed to query unreplayed batches")
		return
	}
	defer rows.Close()

	replayed := 0
	for rows.Next() {
		var (
			id       int64
			batchID  string
			agentID  string
			payload  []byte
			metadata json.RawMessage
		)
		if err := rows.Scan(&id, &batchID, &agentID, &payload, &metadata); err != nil {
			w.logger.WithError(err).Warn("Fallback replay: row scan error")
			continue
		}

		// Parse the stored payload as individual events (same as processBatch)
		var events []json.RawMessage
		if err := json.Unmarshal(payload, &events); err != nil {
			// Payload isn't a JSON array — publish as-is
			headers := map[string]string{
				"batch_id":        batchID,
				"agent_id":        agentID,
				"replay":          "true",
				"replay_raw":      "true",
			}
			if pubErr := w.producer.SendEventBatch(ctx, agentID, payload, headers); pubErr != nil {
				w.logger.WithError(pubErr).WithField("batch_id", batchID).Warn("Fallback replay: Kafka publish failed (raw)")
				continue
			}
		} else {
			// Publish each event individually (matches primary path)
			allOK := true
			for i, evtRaw := range events {
				headers := map[string]string{
					"batch_id":    batchID,
					"agent_id":    agentID,
					"event_index": fmt.Sprintf("%d", i),
					"event_count": fmt.Sprintf("%d", len(events)),
					"replay":      "true",
				}
				if pubErr := w.producer.SendEventBatch(ctx, agentID, evtRaw, headers); pubErr != nil {
					w.logger.WithError(pubErr).WithFields(logrus.Fields{
						"batch_id":    batchID,
						"event_index": i,
					}).Warn("Fallback replay: Kafka publish failed")
					allOK = false
					break
				}
			}
			if !allOK {
				continue // Don't mark as replayed — retry next cycle
			}
		}

		// Mark replayed
		markCtx, markCancel := context.WithTimeout(ctx, 3*time.Second)
		_, err := w.pool.Exec(markCtx, `
			UPDATE event_batches_fallback
			SET replayed = TRUE, replayed_at = $1
			WHERE id = $2
		`, time.Now().UTC(), id)
		markCancel()
		if err != nil {
			w.logger.WithError(err).WithField("batch_id", batchID).Warn("Fallback replay: failed to mark as replayed")
			continue
		}
		replayed++
	}

	if replayed > 0 {
		w.logger.Infof("Fallback replay: successfully re-published %d batch(es) to Kafka", replayed)
	}
}
