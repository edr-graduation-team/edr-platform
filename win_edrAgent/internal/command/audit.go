// Package command provides command audit logging.
package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/edr-platform/win-agent/internal/logging"
)

// AuditLogger logs all command executions for compliance.
type AuditLogger struct {
	logger  *logging.Logger
	logPath string
	mu      sync.Mutex
	file    *os.File
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp   time.Time         `json:"timestamp"`
	CommandID   string            `json:"command_id"`
	CommandType string            `json:"command_type"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Status      string            `json:"status"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
	AgentID     string            `json:"agent_id"`
	Hostname    string            `json:"hostname"`
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(logDir string, logger *logging.Logger) (*AuditLogger, error) {
	if logDir == "" {
		logDir = "C:\\ProgramData\\EDR\\audit"
	}

	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}

	// Create daily audit file
	date := time.Now().Format("2006-01-02")
	logPath := filepath.Join(logDir, "commands_"+date+".jsonl")

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	return &AuditLogger{
		logger:  logger,
		logPath: logPath,
		file:    file,
	}, nil
}

// Log writes an audit entry.
func (a *AuditLogger) Log(entry *AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Sanitize sensitive data
	entry = a.sanitize(entry)

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = a.file.Write(append(data, '\n'))
	if err != nil {
		a.logger.Errorf("Failed to write audit log: %v", err)
	}

	return err
}

// sanitize removes sensitive data from audit entry.
func (a *AuditLogger) sanitize(entry *AuditEntry) *AuditEntry {
	// Create copy
	sanitized := *entry
	sanitized.Parameters = make(map[string]string)

	for k, v := range entry.Parameters {
		// Redact sensitive parameters
		switch k {
		case "password", "token", "secret", "key", "credential":
			sanitized.Parameters[k] = "[REDACTED]"
		default:
			sanitized.Parameters[k] = v
		}
	}

	return &sanitized
}

// LogCommandExecution is a convenience method for logging command execution.
func (a *AuditLogger) LogCommandExecution(
	cmdID, cmdType string,
	params map[string]string,
	result *Result,
	agentID, hostname string,
) {
	entry := &AuditEntry{
		Timestamp:   time.Now().UTC(),
		CommandID:   cmdID,
		CommandType: cmdType,
		Parameters:  params,
		Status:      result.Status,
		Output:      truncate(result.Output, 1000),
		Error:       result.Error,
		DurationMs:  result.Duration.Milliseconds(),
		AgentID:     agentID,
		Hostname:    hostname,
	}

	if err := a.Log(entry); err != nil {
		a.logger.Errorf("Audit log failed: %v", err)
	}
}

// Close closes the audit log file.
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// Rotate creates a new audit file (for daily rotation).
func (a *AuditLogger) Rotate() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close old file
	if a.file != nil {
		a.file.Close()
	}

	// Create new file
	dir := filepath.Dir(a.logPath)
	date := time.Now().Format("2006-01-02")
	newPath := filepath.Join(dir, "commands_"+date+".jsonl")

	file, err := os.OpenFile(newPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	a.file = file
	a.logPath = newPath
	return nil
}

// truncate limits string length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
