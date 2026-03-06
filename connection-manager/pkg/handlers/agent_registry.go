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
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]chan *edrv1.Command
	logger *logrus.Logger
}

// NewAgentRegistry creates a new AgentRegistry.
func NewAgentRegistry(logger *logrus.Logger) *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]chan *edrv1.Command),
		logger: logger,
	}
}

// Register creates a command channel for the given agent and marks it as online.
// If the agent was already registered (e.g. stale stream), the old channel is
// closed first to unblock any goroutine draining it.
func (r *AgentRegistry) Register(agentID string) chan *edrv1.Command {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close stale channel if present (e.g. previous stream not cleaned up)
	if old, exists := r.agents[agentID]; exists {
		close(old)
		r.logger.WithField("agent_id", agentID).Warn("Replacing stale agent stream registration")
	}

	ch := make(chan *edrv1.Command, 50)
	r.agents[agentID] = ch
	r.logger.WithField("agent_id", agentID).Info("Agent registered in stream registry")
	return ch
}

// Deregister removes the agent from the registry and closes its command channel.
func (r *AgentRegistry) Deregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ch, exists := r.agents[agentID]; exists {
		close(ch)
		delete(r.agents, agentID)
		r.logger.WithField("agent_id", agentID).Info("Agent deregistered from stream registry")
	}
}

// Send pushes a command to the agent's command channel (non-blocking).
// Returns an error if the agent is not online or the channel is full.
func (r *AgentRegistry) Send(agentID string, cmd *edrv1.Command) error {
	r.mu.RLock()
	ch, exists := r.agents[agentID]
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

// IsOnline returns true if the agent has an active stream.
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
