// Package handlers provides gRPC handler implementations.
package handlers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/connection-manager/internal/cache"
	"github.com/edr-platform/connection-manager/internal/service"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// HeartbeatHandler handles agent heartbeat RPCs.
// It persists agent health data to TWO stores:
//   - Redis: Real-time status for dashboards (5-min TTL, ephemeral)
//   - PostgreSQL: Source of truth for agent state and metrics history
//
// The Redis write happens first (faster, for live dashboards), then the
// DB write follows. DB errors are warn-logged but never block the
// heartbeat response — we always send the agent its health check back.
type HeartbeatHandler struct {
	logger       *logrus.Logger
	redis        *cache.RedisClient
	agentService service.AgentService // Persists status/metrics to PostgreSQL
}

// NewHeartbeatHandler creates a new heartbeat handler.
// agentService is optional — if nil, metrics are only stored in Redis.
func NewHeartbeatHandler(logger *logrus.Logger, redis *cache.RedisClient, agentSvc service.AgentService) *HeartbeatHandler {
	return &HeartbeatHandler{
		logger:       logger,
		redis:        redis,
		agentService: agentSvc,
	}
}

// Heartbeat processes a heartbeat request from an agent.
func (h *HeartbeatHandler) Heartbeat(ctx context.Context, req *edrv1.HeartbeatRequest) (*edrv1.HeartbeatResponse, error) {
	agentID := req.AgentId
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	logger := h.logger.WithField("agent_id", agentID)
	logger.Debug("Heartbeat received")

	// 1. Update agent status in Redis (skip when Redis unavailable)
	if h.redis != nil {
		if err := h.redis.SetAgentStatus(ctx, agentID, mapAgentStatus(req.Status), 10*time.Minute); err != nil {
			logger.WithError(err).Warn("Failed to update agent status")
		}
	}

	// 2. Calculate health score
	healthScore := calculateHealthScore(req)

	// 3. Update health in Redis (skip when Redis unavailable)
	if h.redis != nil {
		healthKey := "agent:health:" + agentID
		h.redis.Client().HSet(ctx, healthKey, map[string]interface{}{
			"score":     healthScore,
			"status":    getHealthStatus(healthScore),
			"timestamp": time.Now().Unix(),
		})
		h.redis.Client().Expire(ctx, healthKey, 10*time.Minute)
	}

	// 4. Check if certificate renewal is needed
	certRenewalNeeded := false
	if req.CertExpiresAt > 0 {
		daysUntilExpiry := (req.CertExpiresAt - time.Now().Unix()) / (24 * 3600)
		certRenewalNeeded = daysUntilExpiry <= 7 // Renew if expires within 7 days
	}

	// 5. Log metrics (debug level to avoid spam)
	logger.WithFields(logrus.Fields{
		"cpu_usage":        req.CpuUsage,
		"memory_mb":        req.MemoryUsedMb,
		"queue_depth":      req.QueueDepth,
		"events_generated": req.EventsGenerated,
		"events_sent":      req.EventsSent,
		"events_dropped":   req.EventsDropped,
		"ip_addresses":     req.IpAddresses,
		"health_score":     healthScore,
	}).Debug("Agent metrics received")

	// 6. Persist to PostgreSQL (source of truth)
	// Redis is ephemeral (5-min TTL) — without DB persistence, all agent
	// state and metrics history is lost on Redis restart or TTL expiry.
	// The AgentService handles both UpdateStatus() and UpdateMetrics()
	// which write to the agents table in PostgreSQL.
	if h.agentService != nil {
		agentUUID, parseErr := uuid.Parse(agentID)
		if parseErr != nil {
			logger.WithError(parseErr).Warn("Invalid agent_id format (cannot persist to DB)")
		} else {
			dbStatus := mapAgentStatus(req.Status)
			dbMetrics := &service.AgentMetrics{
				CPUUsage:        float64(req.CpuUsage),
				MemoryUsedMB:    req.MemoryUsedMb,
				MemoryTotalMB:   req.MemoryTotalMb,
				QueueDepth:      int(req.QueueDepth),
				EventsGenerated: req.EventsGenerated,
				EventsSent:      req.EventsSent,
				EventsDropped:   req.EventsDropped,
				AgentVersion:    req.AgentVersion,
				IPAddresses:     req.IpAddresses,
				CpuCount:        int(req.DiskTotalMb), // CPU count sent via DiskTotalMb field
			}

			// UpdateStatus writes status + last_seen + optional metrics in one call.
			// We pass metrics here so it's a single DB round-trip.
			if err := h.agentService.UpdateStatus(ctx, agentUUID, dbStatus, dbMetrics); err != nil {
				// Warn but don't fail the heartbeat — the agent must always
				// get its response. DB issues are transient; heartbeats are not.
				logger.WithError(err).Warn("Failed to persist heartbeat to database")
			} else {
				logger.Debug("Heartbeat persisted to database")
			}
		}
	}

	// 7. Prepare response
	return &edrv1.HeartbeatResponse{
		AgentId:             agentID,
		ServerTimestamp:     timestamppb.Now(),
		ServerStatus:        edrv1.ServerStatus_SERVER_STATUS_OK,
		HasPendingCommands:  false, // Phase 2 will implement command queue
		CertRenewalRequired: certRenewalNeeded,
	}, nil
}

// mapAgentStatus maps proto AgentStatus to string.
func mapAgentStatus(status edrv1.AgentStatus) string {
	switch status {
	case edrv1.AgentStatus_AGENT_STATUS_HEALTHY:
		return "online"
	case edrv1.AgentStatus_AGENT_STATUS_DEGRADED:
		return "degraded"
	case edrv1.AgentStatus_AGENT_STATUS_CRITICAL:
		return "critical"
	case edrv1.AgentStatus_AGENT_STATUS_UPDATING:
		return "updating"
	case edrv1.AgentStatus_AGENT_STATUS_ISOLATED:
		return "isolated"
	default:
		return "unknown"
	}
}

// calculateHealthScore calculates health score based on agent metrics.
func calculateHealthScore(req *edrv1.HeartbeatRequest) float64 {
	// Delivery ratio (70% weight)
	var deliveryRatio float64 = 100.0
	if req.EventsGenerated > 0 {
		deliveryRatio = float64(req.EventsSent) / float64(req.EventsGenerated) * 100
	}

	// Status score (30% weight)
	statusScore := 100.0
	switch req.Status {
	case edrv1.AgentStatus_AGENT_STATUS_HEALTHY:
		statusScore = 100.0
	case edrv1.AgentStatus_AGENT_STATUS_DEGRADED:
		statusScore = 80.0
	case edrv1.AgentStatus_AGENT_STATUS_CRITICAL:
		statusScore = 50.0
	default:
		statusScore = 70.0
	}

	// Combined score
	return (deliveryRatio * 0.7) + (statusScore * 0.3)
}

// getHealthStatus returns a status string based on health score.
func getHealthStatus(score float64) string {
	switch {
	case score >= 95:
		return "excellent"
	case score >= 80:
		return "good"
	case score >= 60:
		return "acceptable"
	case score >= 40:
		return "degraded"
	default:
		return "critical"
	}
}
