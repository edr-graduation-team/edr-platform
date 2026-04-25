package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/edr-platform/connection-manager/pkg/models"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// parseQuarantineAckPaths parses agent output from QUARANTINE_FILE:
// "File quarantined: <orig> -> <qpath>" (spacing/casing tolerant; supports Unicode arrow).
func parseQuarantineAckPaths(out string) (original, quarantine string, ok bool) {
	out = strings.TrimSpace(strings.ReplaceAll(out, "\u2192", "->"))
	if out == "" {
		return "", "", false
	}
	lower := strings.ToLower(out)
	const prefix = "file quarantined:"
	idx := strings.Index(lower, prefix)
	if idx < 0 {
		return "", "", false
	}
	rest := strings.TrimSpace(out[idx+len(prefix):])
	sepIdx := strings.Index(rest, " -> ")
	sepLen := 4
	if sepIdx < 0 {
		sepIdx = strings.Index(rest, "->")
		sepLen = 2
	}
	if sepIdx < 0 || sepIdx+sepLen > len(rest) {
		return "", "", false
	}
	o := strings.TrimSpace(rest[:sepIdx])
	q := strings.TrimSpace(rest[sepIdx+sepLen:])
	if o == "" || q == "" {
		return "", "", false
	}
	return o, q, true
}

// quarantineAckOutput prefers the gRPC payload, then the persisted command result JSON.
func quarantineAckOutput(res *edrv1.CommandResult, cmd *models.Command) string {
	if res != nil {
		if s := strings.TrimSpace(res.GetOutput()); s != "" {
			return s
		}
	}
	if cmd != nil && cmd.Result != nil {
		if v, ok := cmd.Result["output"]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
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
		out := quarantineAckOutput(res, cmd)
		orig, qpath, ok := parseQuarantineAckPaths(out)
		if !ok {
			if out != "" {
				s.logger.WithField("output_snippet", truncate(out, 200)).
					Warn("[Quarantine] Could not parse quarantine_file ACK; inventory not updated")
			} else {
				s.logger.Warn("[Quarantine] quarantine_file ACK had empty output; inventory not updated")
			}
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
		} else {
			s.logger.Infof("[Quarantine] Recorded manual quarantine ACK agent=%s orig=%q qpath=%q",
				agentID.String(), orig, qpath)
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
