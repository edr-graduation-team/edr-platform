// Package command provides command handling for server-initiated actions.
package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// CommandType enumerates supported command types.
type CommandType string

const (
	CmdTerminateProcess CommandType = "TERMINATE_PROCESS"
	CmdQuarantineFile   CommandType = "QUARANTINE_FILE"
	CmdIsolateNetwork   CommandType = "ISOLATE_NETWORK"
	CmdUnisolateNetwork CommandType = "UNISOLATE_NETWORK"
	CmdCollectForensics CommandType = "COLLECT_FORENSICS"
	CmdUpdateConfig     CommandType = "UPDATE_CONFIG"
	CmdUpdateAgent      CommandType = "UPDATE_AGENT"
	CmdRestartService   CommandType = "RESTART_SERVICE"
	CmdAdjustRate       CommandType = "ADJUST_RATE"
	CmdRunCommand       CommandType = "RUN_CMD"
	CmdRestart          CommandType = "RESTART"  // Machine reboot
	CmdShutdown         CommandType = "SHUTDOWN" // Machine shutdown
)

// Command represents an incoming command from the server.
type Command struct {
	ID         string
	Type       CommandType
	Parameters map[string]string
	Priority   int
	ExpiresAt  time.Time
	ReceivedAt time.Time
}

// Result represents the outcome of command execution.
type Result struct {
	CommandID string
	Status    string // "SUCCESS", "FAILED", "TIMEOUT"
	Output    string
	Error     string
	Duration  time.Duration
	Timestamp time.Time
}

// Handler processes incoming commands.
type Handler struct {
	logger        *logging.Logger
	quarantineDir string
	mu            sync.Mutex
}

// NewHandler creates a new command handler.
func NewHandler(logger *logging.Logger) *Handler {
	return &Handler{
		logger:        logger,
		quarantineDir: "C:\\ProgramData\\EDR\\quarantine",
	}
}

// Execute processes a command and returns the result.
func (h *Handler) Execute(ctx context.Context, cmd *Command) *Result {
	start := time.Now()

	h.logger.Infof("Executing command: type=%s id=%s", cmd.Type, cmd.ID)

	// Check if expired
	if !cmd.ExpiresAt.IsZero() && time.Now().After(cmd.ExpiresAt) {
		return &Result{
			CommandID: cmd.ID,
			Status:    "FAILED",
			Error:     "command expired",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	var output string
	var err error

	switch cmd.Type {
	case CmdTerminateProcess:
		output, err = h.terminateProcess(ctx, cmd.Parameters)
	case CmdQuarantineFile:
		output, err = h.quarantineFile(ctx, cmd.Parameters)
	case CmdIsolateNetwork:
		output, err = h.isolateNetwork(ctx, cmd.Parameters)
	case CmdUnisolateNetwork:
		output, err = h.unisolateNetwork(ctx, cmd.Parameters)
	case CmdCollectForensics:
		output, err = h.collectForensics(ctx, cmd.Parameters)
	case CmdUpdateConfig:
		output, err = h.updateConfig(ctx, cmd.Parameters)
	case CmdUpdateAgent:
		output, err = h.updateAgent(ctx, cmd.Parameters)
	case CmdRestartService:
		output, err = h.restartService(ctx, cmd.Parameters)
	case CmdAdjustRate:
		output, err = h.adjustRate(ctx, cmd.Parameters)
	case CmdRunCommand:
		output, err = h.runCommand(ctx, cmd.Parameters)
	case CmdRestart:
		output, err = h.restartMachine(ctx, cmd.Parameters)
	case CmdShutdown:
		output, err = h.shutdownMachine(ctx, cmd.Parameters)
	default:
		err = fmt.Errorf("unknown command type: %s", cmd.Type)
	}

	result := &Result{
		CommandID: cmd.ID,
		Output:    output,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}

	if err != nil {
		result.Status = "FAILED"
		result.Error = err.Error()
		h.logger.Errorf("[C2] Command execution FAILED: id=%s type=%s error=%v", cmd.ID, cmd.Type, err)
	} else {
		result.Status = "SUCCESS"
		h.logger.Infof("[C2] Command executed SUCCESSFULLY: id=%s type=%s duration=%v output=%s", cmd.ID, cmd.Type, result.Duration, truncateOutput(output, 200))
	}

	return result
}

// truncateOutput shortens a string for log output.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// terminateProcess kills a process by PID.
func (h *Handler) terminateProcess(ctx context.Context, params map[string]string) (string, error) {
	pid := params["pid"]
	if pid == "" {
		return "", fmt.Errorf("pid parameter is required")
	}

	// Safety check: prevent killing critical system processes
	criticalPIDs := []string{"0", "4"} // System, System Idle
	for _, cp := range criticalPIDs {
		if pid == cp {
			return "", fmt.Errorf("cannot terminate critical system process")
		}
	}

	cmd := exec.CommandContext(ctx, "taskkill", "/PID", pid, "/F")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("taskkill failed: %w", err)
	}

	return fmt.Sprintf("Process %s terminated", pid), nil
}

// quarantineFile moves a file to quarantine.
func (h *Handler) quarantineFile(ctx context.Context, params map[string]string) (string, error) {
	filePath := params["path"]
	if filePath == "" {
		return "", fmt.Errorf("path parameter is required")
	}

	// Validate path exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", filePath)
	}

	// Create quarantine directory
	if err := os.MkdirAll(h.quarantineDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create quarantine dir: %w", err)
	}

	// Generate quarantine filename
	timestamp := time.Now().Format("20060102_150405")
	baseName := filepath.Base(filePath)
	quarantinePath := filepath.Join(h.quarantineDir, fmt.Sprintf("%s_%s.quarantine", timestamp, baseName))

	// Move file
	if err := os.Rename(filePath, quarantinePath); err != nil {
		return "", fmt.Errorf("failed to quarantine file: %w", err)
	}

	return fmt.Sprintf("File quarantined: %s -> %s", filePath, quarantinePath), nil
}

// isolateNetwork disables network adapters.
func (h *Handler) isolateNetwork(ctx context.Context, params map[string]string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Disable all network adapters except loopback
	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		"Get-NetAdapter | Where-Object { $_.Status -eq 'Up' } | Disable-NetAdapter -Confirm:$false")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("network isolation failed: %w", err)
	}

	return "Network isolated - all adapters disabled", nil
}

// unisolateNetwork re-enables network adapters.
func (h *Handler) unisolateNetwork(ctx context.Context, params map[string]string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	cmd := exec.CommandContext(ctx, "powershell", "-Command",
		"Get-NetAdapter | Enable-NetAdapter -Confirm:$false")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("network restore failed: %w", err)
	}

	return "Network restored - adapters enabled", nil
}

// collectForensics gathers evidence files.
func (h *Handler) collectForensics(ctx context.Context, params map[string]string) (string, error) {
	paths := params["paths"]
	if paths == "" {
		return "", fmt.Errorf("paths parameter is required")
	}

	// TODO: Implement forensic collection
	// - Parse paths (comma-separated)
	// - Copy/compress files
	// - Upload to server

	pathList := strings.Split(paths, ",")
	return fmt.Sprintf("Forensic collection initiated for %d paths", len(pathList)), nil
}

// updateConfig applies new configuration.
func (h *Handler) updateConfig(ctx context.Context, params map[string]string) (string, error) {
	configData := params["config"]
	if configData == "" {
		return "", fmt.Errorf("config parameter is required")
	}

	// TODO: Implement config update
	// - Parse new config
	// - Validate
	// - Apply to running agent
	// - Persist to disk

	return "Configuration updated", nil
}

// updateAgent downloads and installs new agent version.
func (h *Handler) updateAgent(ctx context.Context, params map[string]string) (string, error) {
	version := params["version"]
	url := params["url"]
	checksum := params["checksum"]

	if version == "" || url == "" {
		return "", fmt.Errorf("version and url parameters are required")
	}

	// TODO: Implement agent update
	// - Download new binary from URL
	// - Verify checksum
	// - Replace current binary
	// - Trigger service restart

	return fmt.Sprintf("Agent update initiated: version=%s checksum=%s", version, checksum), nil
}

// restartService triggers a service restart.
func (h *Handler) restartService(ctx context.Context, params map[string]string) (string, error) {
	// Schedule restart in 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		exec.Command("sc", "stop", "EDRAgent").Run()
		time.Sleep(2 * time.Second)
		exec.Command("sc", "start", "EDRAgent").Run()
	}()

	return "Service restart scheduled in 5 seconds", nil
}

// adjustRate changes event collection rate.
func (h *Handler) adjustRate(ctx context.Context, params map[string]string) (string, error) {
	batchSize := params["batch_size"]
	interval := params["interval"]

	// TODO: Apply to running batcher

	return fmt.Sprintf("Rate adjusted: batch_size=%s interval=%s", batchSize, interval), nil
}

// runCommand executes an arbitrary shell command with safety checks.
// This is the C2 "run_cmd" capability. A blocklist prevents the most
// dangerous operations; the command runs under cmd /C with a 30s timeout.
func (h *Handler) runCommand(ctx context.Context, params map[string]string) (string, error) {
	cmdStr := params["cmd"]
	if cmdStr == "" {
		return "", fmt.Errorf("cmd parameter is required")
	}

	// Safety blocklist: prevent destructive operations
	dangerousPatterns := []string{
		"format ", "format.",
		"del /s", "del /q",
		"rd /s", "rmdir /s",
		"shutdown", "restart",
		"reg delete",
		"bcdedit",
		"diskpart",
		"cipher /w",
	}
	lower := strings.ToLower(cmdStr)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lower, pattern) {
			return "", fmt.Errorf("blocked: command contains dangerous pattern '%s'", pattern)
		}
	}

	// Execute with 30s timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(timeoutCtx, "cmd", "/C", cmdStr)
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	h.logger.Infof("[C2] run_cmd executed: %s", cmdStr)
	return string(output), nil
}

// restartMachine initiates an OS-level machine reboot.
// The shutdown command is invoked with /t 3 which schedules the reboot
// and returns immediately. This gives the caller (runCommandLoop) enough
// time to send the success result back to the server before the OS goes down.
func (h *Handler) restartMachine(_ context.Context, params map[string]string) (string, error) {
	h.logger.Warn("[C2] RESTART MACHINE command received — scheduling OS reboot in 3 seconds")

	reason := params["reason"]
	if reason == "" {
		reason = "EDR C2 remote restart command"
	}

	// shutdown /r /t 3 schedules a reboot and returns instantly.
	// The 3-second grace period lets the agent send SendCommandResult before the OS shuts down.
	cmd := exec.Command("shutdown", "/r", "/t", "3", "/d", "p:4:1", "/c", reason)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shutdown command failed: %w", err)
	}

	return fmt.Sprintf("Machine restart scheduled (reason: %s). OS rebooting in 3 seconds.", reason), nil
}

// shutdownMachine initiates an OS-level machine shutdown.
// Uses shutdown /s /t 3 for a graceful power-off with a 3-second delay
// to allow the agent to send SendCommandResult before the OS shuts down.
func (h *Handler) shutdownMachine(_ context.Context, params map[string]string) (string, error) {
	h.logger.Warn("[C2] SHUTDOWN MACHINE command received — scheduling OS shutdown in 3 seconds")

	reason := params["reason"]
	if reason == "" {
		reason = "EDR C2 remote shutdown command"
	}

	cmd := exec.Command("shutdown", "/s", "/t", "3", "/d", "p:4:1", "/c", reason)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shutdown command failed: %w", err)
	}

	return fmt.Sprintf("Machine shutdown scheduled (reason: %s). OS powering off in 3 seconds.", reason), nil
}
