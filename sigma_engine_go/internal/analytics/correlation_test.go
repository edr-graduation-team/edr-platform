package analytics

import (
	"testing"
	"time"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

func TestBestEdgeSameRule(t *testing.T) {
	t0 := time.Now().UTC()
	a := &domain.Alert{ID: "a", RuleID: "R1", Timestamp: t0, EventData: map[string]interface{}{"agent_id": "ag1"}}
	b := &domain.Alert{ID: "b", RuleID: "R1", Timestamp: t0.Add(30 * time.Second), EventData: map[string]interface{}{"agent_id": "ag1"}}
	o, n := orderedByTimeThenID(a, b)
	ct, score, ok := bestEdge(o, n)
	if !ok || ct != CorrSameRule || score <= minEdgeScore {
		t.Fatalf("expected same_rule edge, got ok=%v type=%s score=%v", ok, ct, score)
	}
}

func TestBestEdgeSameAgentDifferentRule(t *testing.T) {
	t0 := time.Now().UTC()
	a := &domain.Alert{ID: "a", RuleID: "R1", Timestamp: t0, EventData: map[string]interface{}{"agent_id": "ag1"}}
	b := &domain.Alert{ID: "b", RuleID: "R2", Timestamp: t0.Add(1 * time.Minute), EventData: map[string]interface{}{"agent_id": "ag1"}}
	o, n := orderedByTimeThenID(a, b)
	ct, score, ok := bestEdge(o, n)
	if !ok || ct != CorrSameAgent {
		t.Fatalf("expected same_agent edge, got ok=%v type=%s score=%v", ok, ct, score)
	}
}

func TestBestEdgeSameUser(t *testing.T) {
	t0 := time.Now().UTC()
	a := &domain.Alert{ID: "a", RuleID: "R1", Timestamp: t0, EventData: map[string]interface{}{"user_sid": "S-1-5-21-1"}}
	b := &domain.Alert{ID: "b", RuleID: "R2", Timestamp: t0.Add(2 * time.Minute), EventData: map[string]interface{}{"user_sid": "S-1-5-21-1"}}
	o, n := orderedByTimeThenID(a, b)
	ct, score, ok := bestEdge(o, n)
	if !ok || ct != CorrSameUser {
		t.Fatalf("expected same_user edge, got ok=%v type=%s score=%v", ok, ct, score)
	}
}

func TestCanonicalPair(t *testing.T) {
	l, h := canonicalPair("z", "a")
	if l != "a" || h != "z" {
		t.Fatalf("canonicalPair: got %s %s", l, h)
	}
}
