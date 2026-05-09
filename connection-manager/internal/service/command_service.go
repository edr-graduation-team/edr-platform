package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
	"github.com/edr-platform/connection-manager/pkg/models"
	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// CommandRegistry is the minimal interface the CommandService needs from AgentRegistry.
type CommandRegistry interface {
	Send(agentID string, cmd *edrv1.Command) error
	IsOnline(agentID string) bool
}

// CommandService handles command execution for playbooks
type CommandService struct {
	logger        *logrus.Logger
	commandRepo   repository.CommandRepository
	executionRepo repository.PlaybookExecutionRepository
	metricsRepo   repository.AutomationMetricsRepository
	registry      CommandRegistry
}

// NewCommandService creates a new command service instance
func NewCommandService(
	logger *logrus.Logger,
	commandRepo repository.CommandRepository,
	executionRepo repository.PlaybookExecutionRepository,
	metricsRepo repository.AutomationMetricsRepository,
) *CommandService {
	return &CommandService{
		logger:        logger,
		commandRepo:   commandRepo,
		executionRepo: executionRepo,
		metricsRepo:   metricsRepo,
	}
}

// SetRegistry injects the AgentRegistry so commands are pushed
// directly to online agents over their live gRPC stream.
func (s *CommandService) SetRegistry(r CommandRegistry) {
	s.registry = r
}

// ExecutePlaybookCommand executes a command from a playbook
func (s *CommandService) ExecutePlaybookCommand(ctx context.Context, executionID uuid.UUID, cmd models.PlaybookCommand, agentID uuid.UUID) *CommandResult {
	// Inject the playbook-context marker into run_cmd parameters so the agent
	// grants this command access to the extended playbookAllowedCommands
	// whitelist (which includes powershell -Command and other safe ops).
	// This is safe because playbooks are server-authored and RBAC-protected.
	params := cmd.Parameters
	if params == nil {
		params = make(map[string]interface{})
	}
	// Inject the playbook-context marker for ALL command types so the agent
	// knows this was server-authored and RBAC-protected.
	// For run_cmd this specifically unlocks the extended playbookAllowedCommands
	// whitelist (powershell -Command, mountvol, etc.).
	if _, exists := params["from_playbook"]; !exists {
		params["from_playbook"] = "true"
	}

	// Set a reasonable timeout default (30s) if not specified
	timeout := cmd.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	// Convert playbook command to system command
	command := &models.Command{
		AgentID:        agentID,
		CommandType:    models.CommandType(cmd.Type),
		Parameters:     params,
		Priority:       10, // High priority for playbook commands
		TimeoutSeconds: timeout,
		Status:         models.CommandStatusPending,
		IssuedAt:       time.Now(),
		ExpiresAt:      time.Now().Add(time.Duration(timeout) * time.Second),
		Metadata: map[string]interface{}{
			"playbook_execution_id": executionID,
			"description":           cmd.Description,
			"trigger":               "auto_response",   // shown in Dashboard Response tab
			"source":                "automation_rule", // identifies auto vs manual
		},
	}

	// Insert command into queue — agent picks it up via gRPC stream.
	// Fire-and-forget: we do NOT block waiting for the result here.
	// Results come back asynchronously via SendCommandResult RPC.
	if err := s.commandRepo.Create(ctx, command); err != nil {
		s.logger.WithError(err).Errorf("[playbook] Failed to queue command %s for agent %s", cmd.Type, agentID)
		return &CommandResult{
			Status:      "failed",
			Error:       err.Error(),
			CompletedAt: time.Now(),
		}
	}

	s.logger.Infof("[playbook] Queued command %s for agent %s (cmd_id: %s)", cmd.Type, agentID, command.ID)

	// Push the command to the agent's live gRPC stream immediately
	if s.registry != nil {
		protoCmd := &edrv1.Command{
			CommandId:      command.ID.String(),
			Type:           string(command.CommandType),
			ParametersJson: mustMarshalJSON(command.Parameters),
			TimeoutSeconds: int32(command.TimeoutSeconds),
			Priority:       int32(command.Priority),
		}
		if err := s.registry.Send(agentID.String(), protoCmd); err != nil {
			s.logger.WithError(err).Warnf("[playbook] Agent %s not online — command %s will be delivered on reconnect", agentID, command.ID)
		} else {
			s.logger.Infof("[playbook] Command %s pushed to agent %s live stream ✅", command.ID, agentID)
		}
	}

	// Return success immediately — command is queued and (if online) pushed
	return &CommandResult{
		Status:      "queued",
		CompletedAt: time.Now(),
	}
}

// mustMarshalJSON marshals v to JSON, returning "{}" on error
func mustMarshalJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// WaitForCommandResult waits for command execution result
func (s *CommandService) WaitForCommandResult(ctx context.Context, commandID uuid.UUID, timeout time.Duration) *CommandResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return &CommandResult{
				Status:    "timeout",
				Error:     "Command execution timeout",
				CompletedAt: time.Now(),
			}
		case <-ticker.C:
			cmd, err := s.commandRepo.GetByID(ctx, commandID)
			if err != nil {
				return &CommandResult{
					Status:    "error",
					Error:     err.Error(),
					CompletedAt: time.Now(),
				}
			}
			
			if cmd.Status == models.CommandStatusCompleted || cmd.Status == models.CommandStatusFailed {
				completedAt := time.Now()
				if cmd.CompletedAt != nil {
					completedAt = *cmd.CompletedAt
				}
				return &CommandResult{
					Status:    string(cmd.Status),
					Result:    cmd.Result,
					Error:     cmd.ErrorMessage,
					CompletedAt: completedAt,
				}
			}
		}
	}
}

// Create creates a new command (for backward compatibility)
func (s *CommandService) Create(ctx context.Context, command *models.Command) error {
	return s.commandRepo.Create(ctx, command)
}

// GetByID retrieves a command by ID (for backward compatibility)
func (s *CommandService) GetByID(ctx context.Context, id uuid.UUID) (*models.Command, error) {
	return s.commandRepo.GetByID(ctx, id)
}

// CommandResult represents the result of command execution
type CommandResult struct {
	Status      string                 `json:"status"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	CompletedAt time.Time              `json:"completed_at"`
}
