package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/edr-platform/connection-manager/pkg/models"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// parseQuarantineAckPaths parses agent output from QUARANTINE_FILE: "File quarantined: <orig> -> <qpath>".
func parseQuarantineAckPaths(out string) (original, quarantine string, ok bool) {
	const prefix = "File quarantined:"
	idx := strings.Index(out, prefix)
	if idx < 0 {
		return "", "", false
	}
	rest := strings.TrimSpace(out[idx+len(prefix):])
	parts := strings.SplitN(rest, " -> ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	o, q := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if o == "" || q == "" {
		return "", "", false
	}
	return o, q, true
}

func metaUUID(meta map[string]any, key string) (uuid.UUID, bool) {
	if meta == nil {
		return uuid.Nil, false
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return uuid.Nil, false
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) applyQuarantineInventoryOnSuccess(ctx context.Context, res *edrv1.CommandResult, cmd *models.Command, agentID uuid.UUID) {
	if s.quarantineRepo == nil || agentID == uuid.Nil {
		return
	}
	ct := strings.ToLower(string(cmd.CommandType))
	switch ct {
	case "quarantine_file":
		orig, qpath, ok := parseQuarantineAckPaths(res.GetOutput())
		if !ok {
			return
		}
		row := &models.QuarantineItem{
			AgentID:        agentID,
			OriginalPath:   orig,
			QuarantinePath: qpath,
			Source:         "manual_c2",
			State:          models.QuarantineStateQuarantined,
		}
		if err := s.quarantineRepo.Upsert(ctx, row); err != nil {
			s.logger.WithError(err).Warn("[Quarantine] Failed to record manual quarantine ACK")
		}
	case "restore_quarantine_file":
		if qid, ok := metaUUID(cmd.Metadata, "quarantine_item_id"); ok {
			if err := s.quarantineRepo.SetState(ctx, qid, models.QuarantineStateRestored); err != nil {
				s.logger.WithError(err).Warn("[Quarantine] Failed to mark entry restored")
			}
		}
	case "delete_quarantine_file":
		if qid, ok := metaUUID(cmd.Metadata, "quarantine_item_id"); ok {
			if err := s.quarantineRepo.SetState(ctx, qid, models.QuarantineStateDeleted); err != nil {
				s.logger.WithError(err).Warn("[Quarantine] Failed to mark entry deleted")
			}
		}
	}
}
