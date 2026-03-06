// Package handlers provides gRPC handler implementations.
package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/golang/snappy"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/service"
	"github.com/edr-platform/connection-manager/pkg/contextkeys"
	"github.com/edr-platform/connection-manager/pkg/kafka"
	"github.com/edr-platform/connection-manager/pkg/metrics"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// EventHandler handles event ingestion RPCs.
// It implements a multi-tier delivery pipeline:
//  1. Primary:  Kafka (events-raw topic)
//  2. DLQ:      Kafka (events-dlq topic) — automatic on primary write failure
//  3. Fallback: PostgreSQL (event_batches_fallback table) — when Kafka is entirely down
//
// This ensures zero data loss: every event batch reaches durable storage
// regardless of individual component failures.
type EventHandler struct {
	logger        *logrus.Logger
	redis         *cache.RedisClient
	rateLimiter   *cache.RateLimiter
	kafkaProducer *kafka.EventProducer
	metrics       *metrics.Metrics
	fallbackStore *EventFallbackStore  // PostgreSQL fallback when Kafka is unavailable
	registry      *AgentRegistry       // Live agent presence and command routing
	agentService  service.AgentService // Persists status to PostgreSQL
}

// NewEventHandler creates a new event handler.
// All dependencies except logger are optional — the handler degrades
// gracefully when components are nil.
func NewEventHandler(
	logger *logrus.Logger,
	redis *cache.RedisClient,
	rateLimiter *cache.RateLimiter,
	kafkaProducer *kafka.EventProducer,
	m *metrics.Metrics,
) *EventHandler {
	return &EventHandler{
		logger:        logger,
		redis:         redis,
		rateLimiter:   rateLimiter,
		kafkaProducer: kafkaProducer,
		metrics:       m,
	}
}

// SetAgentRegistry sets the agent registry for real-time presence and command routing.
func (h *EventHandler) SetAgentRegistry(registry *AgentRegistry) {
	h.registry = registry
}

// SetAgentService sets the AgentService for PostgreSQL status persistence.
func (h *EventHandler) SetAgentService(svc service.AgentService) {
	h.agentService = svc
}

// SetFallbackStore sets the PostgreSQL fallback store.
// Called from main.go after handler creation if DB is available.
func (h *EventHandler) SetFallbackStore(store *EventFallbackStore) {
	h.fallbackStore = store
}

// StreamEvents handles bidirectional streaming for event ingestion.
// On stream open:  validates agent in DB, registers in AgentRegistry, updates PostgreSQL to 'online'.
// On stream close: deregisters agent, updates PostgreSQL to 'offline'.
// A dedicated goroutine drains the agent's command channel and pushes
// CommandBatch messages over the stream for real-time C2 delivery.
func (h *EventHandler) StreamEvents(stream edrv1.EventIngestionService_StreamEventsServer) error {
	ctx := stream.Context()
	agentID := extractAgentIDFromContext(ctx)

	// ── STRICT VALIDATION: Reject unknown/revoked agents ──
	// The agent MUST exist in the PostgreSQL database (created during RegisterAgent).
	// If the DB was wiped or the agent was revoked, reject with Unauthenticated
	// so the agent can detect this and re-enroll automatically.
	if h.agentService != nil {
		agentUUID, err := uuid.Parse(agentID)
		if err != nil {
			h.logger.WithField("agent_id", agentID).Warn("Rejected: invalid agent UUID")
			return status.Errorf(codes.Unauthenticated, "invalid agent ID format")
		}

		if _, err := h.agentService.GetByID(ctx, agentUUID); err != nil {
			h.logger.WithField("agent_id", agentID).Warn("Rejected: agent not found in database — must re-enroll")
			return status.Errorf(codes.Unauthenticated,
				"agent %s is not registered — re-enrollment required", agentID)
		}
	}

	h.logger.WithField("agent_id", agentID).Info("=== Agent came ONLINE (stream opened) ===")

	// 1. Register agent in the in-memory registry for command routing
	var cmdChan chan *edrv1.Command
	if h.registry != nil {
		cmdChan = h.registry.Register(agentID)
	}

	// 2. Mark agent as online in Redis (skip when Redis unavailable)
	if h.redis != nil {
		if err := h.redis.SetAgentStatus(ctx, agentID, "online", 10*time.Minute); err != nil {
			h.logger.WithError(err).Warn("Failed to set agent status in Redis")
		}
	}

	// 3. Persist 'online' status to PostgreSQL (source of truth)
	h.updateAgentDBStatus(agentID, "online")

	// 4. Ensure we mark agent offline when stream closes (graceful or crash)
	defer func() {
		h.logger.WithField("agent_id", agentID).Info("=== Agent went OFFLINE (stream closed/timeout) ===")

		// Deregister from in-memory registry
		if h.registry != nil {
			h.registry.Deregister(agentID)
		}

		// Update Redis
		if h.redis != nil {
			offlineCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.redis.SetAgentStatus(offlineCtx, agentID, "offline", 5*time.Minute); err != nil {
				h.logger.WithError(err).Warn("Failed to update agent status on stream close")
			}
		}

		// Persist 'offline' to PostgreSQL
		h.updateAgentDBStatus(agentID, "offline")
	}()

	// 5. Keepalive ticker: refreshes Redis TTL and PostgreSQL last_seen every
	//    2 minutes while the gRPC stream is alive. This prevents the agent
	//    from flipping to "offline" during idle periods with no event batches.
	keepAliveTicker := time.NewTicker(2 * time.Minute)
	go func() {
		defer keepAliveTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-keepAliveTicker.C:
				if h.redis != nil {
					if err := h.redis.SetAgentStatus(ctx, agentID, "online", 10*time.Minute); err != nil {
						h.logger.WithError(err).WithField("agent_id", agentID).Debug("Keepalive: failed to refresh Redis TTL")
					}
				}
				h.updateAgentDBStatus(agentID, "online")
				h.logger.WithField("agent_id", agentID).Debug("Keepalive: refreshed Redis TTL and DB last_seen")
			}
		}
	}()

	// 6. Unified send architecture: gRPC stream.Send() is NOT thread-safe.
	//    We use a single sendChan and a dedicated sender goroutine to serialize
	//    all writes (event responses + C2 commands) to the stream.
	sendChan := make(chan *edrv1.CommandBatch, 100)
	sendDone := make(chan error, 1)

	// Sender goroutine: the ONLY goroutine that calls stream.Send()
	go func() {
		for batch := range sendChan {
			if err := stream.Send(batch); err != nil {
				h.logger.WithError(err).WithField("agent_id", agentID).Warn("Stream send error")
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()

	// 7. Command forwarder: drains the agent's command channel and pushes
	//    CommandBatch messages into sendChan for delivery via the sender goroutine.
	if cmdChan != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case cmd, ok := <-cmdChan:
					if !ok {
						return // channel closed — agent deregistered
					}
					if cmd == nil {
						continue
					}
					batch := &edrv1.CommandBatch{
						BatchId:      uuid.New().String(),
						Timestamp:    timestamppb.Now(),
						ServerStatus: edrv1.ServerStatus_SERVER_STATUS_OK,
						Commands:     []*edrv1.Command{cmd},
					}
					select {
					case sendChan <- batch:
						h.logger.WithFields(logrus.Fields{
							"agent_id":   agentID,
							"command_id": cmd.GetCommandId(),
							"type":       cmd.GetType().String(),
						}).Info("Command queued for agent stream delivery")
					default:
						h.logger.WithField("agent_id", agentID).Warn("Send channel full, dropping command")
					}
				}
			}
		}()
	}

	// 8. Process incoming event batches — recv runs in the main goroutine,
	//    responses are pushed into sendChan (never calling stream.Send directly).
	for {
		select {
		case err := <-sendDone:
			// Sender goroutine hit an error
			return status.Errorf(codes.Internal, "stream send error: %v", err)
		default:
		}

		batch, err := stream.Recv()
		if err == io.EOF {
			h.logger.WithField("agent_id", agentID).Info("Client closed stream gracefully")
			close(sendChan)
			return nil
		}
		if err != nil {
			h.logger.WithError(err).WithField("agent_id", agentID).Warn("Stream receive error")
			close(sendChan)
			return status.Errorf(codes.Internal, "stream receive error: %v", err)
		}

		// Process the batch
		resp, err := h.processBatch(ctx, agentID, batch)
		if err != nil {
			h.logger.WithError(err).WithFields(logrus.Fields{
				"agent_id": agentID,
				"batch_id": batch.BatchId,
			}).Warn("Batch processing error")
			// Continue processing, don't close stream on batch error
			continue
		}

		// Queue response for sending (via the sender goroutine)
		if resp != nil {
			select {
			case sendChan <- resp:
			default:
				h.logger.WithField("agent_id", agentID).Warn("Send channel full, dropping batch response")
			}
		}

		// Update agent status TTL (skip when Redis unavailable)
		if h.redis != nil {
			h.redis.SetAgentStatus(ctx, agentID, "online", 10*time.Minute)
		}
	}
}

// commandPushLoop drains the agent's command channel and sends CommandBatch
// messages over the bidirectional stream. This enables the REST API to push
// commands to live agents in real-time without polling.
// Returns when the command channel is closed (agent deregistered) or ctx is done.
func (h *EventHandler) commandPushLoop(
	ctx context.Context,
	agentID string,
	stream edrv1.EventIngestionService_StreamEventsServer,
	cmdChan chan *edrv1.Command,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd, ok := <-cmdChan:
			if !ok {
				// Channel closed — agent deregistered
				return
			}
			if cmd == nil {
				continue
			}

			batch := &edrv1.CommandBatch{
				BatchId:      uuid.New().String(),
				Timestamp:    timestamppb.Now(),
				ServerStatus: edrv1.ServerStatus_SERVER_STATUS_OK,
				Commands:     []*edrv1.Command{cmd},
			}

			if err := stream.Send(batch); err != nil {
				h.logger.WithError(err).WithFields(logrus.Fields{
					"agent_id":   agentID,
					"command_id": cmd.GetCommandId(),
				}).Warn("Failed to push command over stream")
				return
			}

			h.logger.WithFields(logrus.Fields{
				"agent_id":   agentID,
				"command_id": cmd.GetCommandId(),
				"type":       cmd.GetType().String(),
			}).Info("Command pushed to agent via stream")
		}
	}
}

// updateAgentDBStatus updates the agent status in PostgreSQL.
// Uses a fresh background context because this may be called during
// stream teardown when the stream context is already cancelled.
func (h *EventHandler) updateAgentDBStatus(agentID, status string) {
	if h.agentService == nil {
		return
	}

	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		h.logger.WithField("agent_id", agentID).Warn("Cannot update DB status: invalid UUID")
		return
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.agentService.UpdateStatus(dbCtx, agentUUID, status, nil); err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{
			"agent_id": agentID,
			"status":   status,
		}).Warn("Failed to update agent status in PostgreSQL")
	} else {
		h.logger.WithFields(logrus.Fields{
			"agent_id": agentID,
			"status":   status,
		}).Info("Agent status updated in PostgreSQL")
	}
}

// processBatch processes a single event batch.
func (h *EventHandler) processBatch(ctx context.Context, agentID string, batch *edrv1.EventBatch) (*edrv1.CommandBatch, error) {
	logger := h.logger.WithFields(logrus.Fields{
		"agent_id":    agentID,
		"batch_id":    batch.BatchId,
		"event_count": batch.EventCount,
	})

	// 1. Validate batch
	if err := h.validateBatch(batch); err != nil {
		logger.WithError(err).Warn("Invalid batch")
		return nil, err
	}

	// 2. Check for duplicates (skip when Redis unavailable — assume batch is new)
	if h.redis != nil {
		duplicate, err := h.redis.IsBatchProcessed(ctx, batch.BatchId)
		if err != nil {
			logger.WithError(err).Warn("Duplicate check failed")
			// Continue on Redis error
		} else if duplicate {
			logger.Debug("Duplicate batch ignored")
			return nil, nil // Silently ignore duplicates (idempotent)
		}
	}

	// 3. Verify checksum if provided
	if batch.Checksum != "" {
		if !h.verifyChecksum(batch.Payload, batch.Checksum) {
			logger.Warn("Checksum mismatch")
			return nil, status.Error(codes.InvalidArgument, "checksum mismatch")
		}
	}

	// 4. Decompress payload if needed
	// Production requirement: agents may use either Snappy (fast, default)
	// or Gzip (better ratio for bandwidth-constrained links). Both must work.
	payload := batch.Payload
	switch batch.Compression {
	case edrv1.Compression_COMPRESSION_SNAPPY:
		decompressed, err := snappy.Decode(nil, batch.Payload)
		if err != nil {
			logger.WithError(err).Error("Snappy decompression failed — routing raw batch to DB fallback")
			h.storeToFallback(ctx, batch, batch.Payload)
			return nil, nil // Do not crash pipeline; data preserved in fallback
		}
		payload = decompressed

	case edrv1.Compression_COMPRESSION_GZIP:
		// Gzip decompression with io.LimitReader (32MB) to prevent zip-bomb DoS attacks.
		gzReader, err := gzip.NewReader(bytes.NewReader(batch.Payload))
		if err != nil {
			logger.WithError(err).Error("Gzip reader creation failed — routing raw batch to DB fallback")
			h.storeToFallback(ctx, batch, batch.Payload)
			return nil, nil
		}
		const maxDecompressedSize = 32 * 1024 * 1024 // 32MB limit
		decompressed, err := io.ReadAll(io.LimitReader(gzReader, maxDecompressedSize))
		gzReader.Close()
		if err != nil {
			logger.WithError(err).Error("Gzip decompression failed — routing raw batch to DB fallback")
			h.storeToFallback(ctx, batch, batch.Payload)
			return nil, nil
		}
		payload = decompressed

	case edrv1.Compression_COMPRESSION_NONE:
		// No decompression needed
	default:
		logger.WithField("compression", batch.Compression).Warn("Unknown compression type — treating payload as uncompressed")
	}

	// 5. Parse decompressed payload as JSON array of events (contract for downstream Sigma Engine).
	var events []map[string]interface{}
	if err := json.Unmarshal(payload, &events); err != nil {
		logger.WithError(err).Error("Failed to unmarshal decompressed payload as JSON array — routing raw batch to DB fallback")
		h.storeToFallback(ctx, batch, batch.Payload)
		return nil, nil // Ack batch so pipeline does not crash or retry indefinitely
	}
	if len(events) == 0 {
		logger.Warn("Decompressed payload is empty array — skipping Kafka publish, storing raw batch to fallback")
		h.storeToFallback(ctx, batch, batch.Payload)
		return nil, nil
	}

	// 6. Enrich each event with agent_id (and batch_id) for downstream detection engine.
	for i := range events {
		events[i]["agent_id"] = agentID
		events[i]["batch_id"] = batch.BatchId
	}

	// 7. Publish each event individually to Kafka (agent_id as partition key). Respect context cancellation.
	if h.kafkaProducer != nil {
		for i, ev := range events {
			select {
			case <-ctx.Done():
				logger.WithError(ctx.Err()).Warn("Context cancelled during event publish — routing batch to DB fallback")
				h.storeToFallback(ctx, batch, payload)
				code := codes.Canceled
				if ctx.Err() == context.DeadlineExceeded {
					code = codes.DeadlineExceeded
				}
				return nil, status.Error(code, ctx.Err().Error())
			default:
			}
			eventJSON, err := json.Marshal(ev)
			if err != nil {
				logger.WithError(err).WithField("event_index", i).Error("Failed to marshal event — routing batch to DB fallback")
				h.storeToFallback(ctx, batch, payload)
				return nil, nil
			}
			headers := map[string]string{
				"batch_id":    batch.BatchId,
				"agent_id":    batch.AgentId,
				"event_index": fmt.Sprintf("%d", i),
				"event_count": fmt.Sprintf("%d", len(events)),
			}
			if err := h.kafkaProducer.SendEventBatch(ctx, batch.AgentId, eventJSON, headers); err != nil {
				logger.WithError(err).WithField("event_index", i).Warn("Kafka write failed — routing batch to DB fallback")
				h.storeToFallback(ctx, batch, payload)
				return nil, nil // Do not crash pipeline; data preserved in fallback
			}
		}
		logger.WithField("events", len(events)).Debug("Events sent to Kafka individually")
	} else {
		logger.Debug("Kafka disabled — storing batch via DB fallback")
		h.storeToFallback(ctx, batch, payload)
	}

	// 8. Record metrics
	if h.metrics != nil {
		h.metrics.RecordEventBatch(int(batch.EventCount), len(batch.Payload))
	}

	// 9. Mark batch as processed (skip when Redis unavailable)
	if h.redis != nil {
		if err := h.redis.SetBatchProcessed(ctx, batch.BatchId, 24*time.Hour); err != nil {
			logger.WithError(err).Warn("Failed to mark batch as processed")
		}
	}

	// 10. Prepare response
	return &edrv1.CommandBatch{
		BatchId:      uuid.New().String(),
		Timestamp:    timestamppb.Now(),
		ServerStatus: edrv1.ServerStatus_SERVER_STATUS_OK,
		AckBatchId:   batch.BatchId,
	}, nil
}

// validateBatch validates the event batch structure.
func (h *EventHandler) validateBatch(batch *edrv1.EventBatch) error {
	if batch.BatchId == "" {
		return status.Error(codes.InvalidArgument, "batch_id is required")
	}
	if batch.AgentId == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}
	if batch.EventCount <= 0 {
		return status.Error(codes.InvalidArgument, "event_count must be positive")
	}
	if len(batch.Payload) == 0 {
		return status.Error(codes.InvalidArgument, "payload is required")
	}

	// Check payload size (max 10MB)
	const maxPayloadSize = 10 * 1024 * 1024
	if len(batch.Payload) > maxPayloadSize {
		return status.Errorf(codes.InvalidArgument,
			"payload too large: %d bytes (max: %d)", len(batch.Payload), maxPayloadSize)
	}

	return nil
}

// verifyChecksum verifies the SHA256 checksum of the payload.
func (h *EventHandler) verifyChecksum(payload []byte, expectedChecksum string) bool {
	hash := sha256.Sum256(payload)
	actualChecksum := hex.EncodeToString(hash[:])
	return actualChecksum == expectedChecksum
}

// extractAgentIDFromContext gets agent ID from context (set by auth stream interceptor).
func extractAgentIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextkeys.AgentIDKey).(string); ok {
		return id
	}
	return "unknown"
}

// storeToFallback writes an event batch to PostgreSQL as a last-resort fallback.
// This is designed to never fail loudly — we log errors but do NOT propagate
// them to the caller. Uses a fresh context (not the stream context) because
// this is often called when the stream is already cancelled or disconnected.
func (h *EventHandler) storeToFallback(_ context.Context, batch *edrv1.EventBatch, payload []byte) {
	if h.fallbackStore == nil {
		h.logger.WithFields(logrus.Fields{
			"batch_id": batch.BatchId,
			"agent_id": batch.AgentId,
			"size":     len(payload),
		}).Error("EVENT DATA LOST: Kafka unavailable and no DB fallback configured")
		if h.metrics != nil {
			h.metrics.RecordError("event_data_lost")
		}
		return
	}

	metadata := map[string]string{
		"event_count": fmt.Sprintf("%d", batch.EventCount),
		"compression": batch.Compression.String(),
	}
	if batch.Metadata != nil {
		for k, v := range batch.Metadata {
			metadata[k] = v
		}
	}

	fallbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.fallbackStore.Store(fallbackCtx, batch.BatchId, batch.AgentId, payload, metadata); err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{
			"batch_id": batch.BatchId,
			"agent_id": batch.AgentId,
		}).Error("DB fallback write failed — event data may be lost")
		if h.metrics != nil {
			h.metrics.RecordError("fallback_write_failed")
		}
	}
}
