// Package handlers provides the AgentRegistry for real-time agent presence
// tracking and command routing.
package handlers

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// AgentRegistry is a thread-safe in-memory registry of connected agents.
// Each online agent has a buffered command channel through which the REST API
// can push commands for real-time delivery over the agent's gRPC stream.
//
// An agent may have multiple concurrent StreamEvents RPCs (e.g. the Windows
// agent keeps one long-lived stream for commands, and may open short-lived
// StreamEvents to flush event batches when the long stream is not ready yet).
// We refcount streams per agent: the command channel and map entry stay until
// the *last* stream closes. Deregistering after a short stream must not remove
// the row while a long stream is still open — that mismatch caused
// "online" in the API (heartbeat) but AGENT_OFFLINE in C2.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*streamEntry
	logger *logrus.Logger
}

type streamEntry struct {
	ch   chan *edrv1.Command
	refs int
}

// NewAgentRegistry creates a new AgentRegistry.
func NewAgentRegistry(logger *logrus.Logger) *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*streamEntry),
		logger: logger,
	}
}

// Register associates one active StreamEvents session with the agent.
// Multiple concurrent calls share the same command channel; each caller must
// pair with a later Deregister.
func (r *AgentRegistry) Register(agentID string) chan *edrv1.Command {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.agents[agentID]; ok {
		e.refs++
		r.logger.WithFields(logrus.Fields{
			"agent_id":   agentID,
			"open_streams": e.refs,
		}).Debug("Additional StreamEvents — refcount incremented")
		return e.ch
	}

	ch := make(chan *edrv1.Command, 50)
	r.agents[agentID] = &streamEntry{ch: ch, refs: 1}
	r.logger.WithField("agent_id", agentID).Info("Agent registered in stream registry")
	return ch
}

// Deregister ends one StreamEvents session. The agent is removed and the
// command channel closed only when the refcount drops to zero.
func (r *AgentRegistry) Deregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.agents[agentID]
	if !ok {
		return
	}
	e.refs--
	if e.refs > 0 {
		r.logger.WithFields(logrus.Fields{
			"agent_id":   agentID,
			"open_streams": e.refs,
		}).Debug("Stream closed — agent still has other open streams")
		return
	}
	close(e.ch)
	delete(r.agents, agentID)
	r.logger.WithField("agent_id", agentID).Info("Agent deregistered from stream registry (last stream closed)")
}

// Send pushes a command to the agent's command channel (non-blocking).
// Returns an error if the agent is not online or the channel is full.
func (r *AgentRegistry) Send(agentID string, cmd *edrv1.Command) error {
	r.mu.RLock()
	e, exists := r.agents[agentID]
	var ch chan *edrv1.Command
	if exists {
		ch = e.ch
	}
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s is not online", agentID)
	}

	select {
	case ch <- cmd:
		r.logger.WithFields(logrus.Fields{
			"agent_id":   agentID,
			"command_id": cmd.GetCommandId(),
		}).Info("Command pushed to agent stream")
		return nil
	default:
		return fmt.Errorf("agent %s command channel is full", agentID)
	}
}

// IsOnline returns true if the agent has at least one active StreamEvents session.
func (r *AgentRegistry) IsOnline(agentID string) bool {
	r.mu.RLock()
	_, exists := r.agents[agentID]
	r.mu.RUnlock()
	return exists
}

// OnlineCount returns the number of currently connected agents.
func (r *AgentRegistry) OnlineCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}
