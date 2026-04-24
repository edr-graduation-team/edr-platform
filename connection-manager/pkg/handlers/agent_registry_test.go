package handlers

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

func TestAgentRegistry_RefCountConcurrentStreams(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	r := NewAgentRegistry(log)

	id := "a1b2c3d4-e5f6-4789-a012-34567890abcd"
	ch1 := r.Register(id)
	if !r.IsOnline(id) {
		t.Fatal("expected online after first register")
	}
	if r.OnlineCount() != 1 {
		t.Fatalf("OnlineCount: got %d want 1", r.OnlineCount())
	}

	ch2 := r.Register(id)
	if ch1 != ch2 {
		t.Fatal("second stream should share the same command channel")
	}
	if r.OnlineCount() != 1 {
		t.Fatalf("OnlineCount: got %d want 1 (one agent, two streams)", r.OnlineCount())
	}

	r.Deregister(id)
	if !r.IsOnline(id) {
		t.Fatal("expected still online after first deregister (second stream open)")
	}

	r.Deregister(id)
	if r.IsOnline(id) {
		t.Fatal("expected offline after last deregister")
	}
	if r.OnlineCount() != 0 {
		t.Fatalf("OnlineCount: got %d want 0", r.OnlineCount())
	}
}

func TestAgentRegistry_DeregisterIdempotent(t *testing.T) {
	r := NewAgentRegistry(logrus.New())
	id := "00000000-0000-4000-8000-000000000001"
	_ = r.Register(id)
	r.Deregister(id)
	r.Deregister(id) // must not panic
}

func TestAgentRegistry_SendToSharedChannel(t *testing.T) {
	r := NewAgentRegistry(logrus.New())
	id := "00000000-0000-4000-8000-000000000002"
	_ = r.Register(id)
	_ = r.Register(id)
	cmd := &edrv1.Command{CommandId: "x"}
	if err := r.Send(id, cmd); err != nil {
		t.Fatalf("Send: %v", err)
	}
}
