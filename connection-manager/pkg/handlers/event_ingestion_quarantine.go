package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// persistQuarantineFromEvents records auto-quarantine telemetry rows into PostgreSQL.
func (h *EventHandler) persistQuarantineFromEvents(ctx context.Context, agentID string, events []map[string]interface{}) {
	if h.quarantineRepo == nil || len(events) == 0 {
		return
	}
	aid, err := uuid.Parse(agentID)
	if err != nil {
		return
	}
	for _, ev := range events {
		data, _ := ev["data"].(map[string]interface{})
		if data == nil {
			continue
		}
		action := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["action"])))
		if action != "auto_quarantined" {
			continue
		}
		qp := strings.TrimSpace(fmt.Sprint(data["quarantine_path"]))
		op := strings.TrimSpace(fmt.Sprint(data["path"]))
		if qp == "" || op == "" {
			continue
		}
		row := &models.QuarantineItem{
			AgentID:        aid,
			EventID:        strings.TrimSpace(fmt.Sprint(ev["event_id"])),
			OriginalPath:   op,
			QuarantinePath: qp,
			SHA256:         strings.TrimSpace(fmt.Sprint(data["sha256"])),
			ThreatName:     strings.TrimSpace(fmt.Sprint(data["threat_name"])),
			Source:         "auto_responder",
			State:          models.QuarantineStateQuarantined,
		}
		if err := h.quarantineRepo.Upsert(ctx, row); err != nil {
			h.logger.WithError(err).WithFields(logrus.Fields{
				"agent_id": agentID,
				"path":     qp,
			}).Warn("[Quarantine] Failed to upsert inventory row from telemetry")
		}
	}
}
