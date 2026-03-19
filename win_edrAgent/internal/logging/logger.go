// Package logging provides file-based logging with rotation for the EDR Agent.
package logging

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level represents logging severity levels.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string into a log level.
func ParseLevel(s string) Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Config holds logger configuration.
type Config struct {
	Level      string
	FilePath   string
	MaxSizeMB  int
	MaxAgeDays int
}

// LogEncryptor is an optional interface for encrypting log entries before
// they are written to the log file. Stdout output is never encrypted.
type LogEncryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
}

// Logger provides structured logging with file rotation.
type Logger struct {
	mu          sync.Mutex
	level       Level
	file        *os.File
	filePath    string
	maxSize     int64
	maxAge      int
	currentSize int64
	writers     []io.Writer
	encryptor   LogEncryptor // optional, nil = plaintext logs
}

// NewLogger creates a new logger instance.
func NewLogger(cfg Config) *Logger {
	l := &Logger{
		level:    ParseLevel(cfg.Level),
		filePath: cfg.FilePath,
		maxSize:  int64(cfg.MaxSizeMB) * 1024 * 1024,
		maxAge:   cfg.MaxAgeDays,
		writers:  []io.Writer{os.Stdout},
	}

	// Create log directory if needed
	if cfg.FilePath != "" {
		dir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(dir, 0755); err == nil {
			if file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
				l.file = file
				l.writers = append(l.writers, file)

				// Get current file size
				if info, err := file.Stat(); err == nil {
					l.currentSize = info.Size()
				}
			}
		}
	}

	return l
}

// SetLevel changes the logging level at runtime.
func (l *Logger) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = ParseLevel(level)
}

// SetEncryptor enables encryption for log file entries.
// Stdout output remains plaintext; only file writes are encrypted.
func (l *Logger) SetEncryptor(enc LogEncryptor) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.encryptor = enc
}

// Close closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log writes a log entry.
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Format message
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	entry := fmt.Sprintf("[%s] %s: %s\n", timestamp, level.String(), msg)

	// Check for rotation
	if l.file != nil && l.maxSize > 0 && l.currentSize+int64(len(entry)) > l.maxSize {
		l.rotate()
	}

	// Write to all writers
	for _, w := range l.writers {
		var writeData []byte
		if w == l.file && l.encryptor != nil {
			// Encrypt file entries; encode as base64 line for rotation compat.
			enc, err := l.encryptor.Encrypt([]byte(entry))
			if err == nil {
				writeData = []byte(base64.StdEncoding.EncodeToString(enc) + "\n")
			} else {
				writeData = []byte(entry)
			}
		} else {
			writeData = []byte(entry)
		}
		if _, err := w.Write(writeData); err == nil {
			if w == l.file {
				l.currentSize += int64(len(writeData))
			}
		}
	}
}

// rotate performs log file rotation.
func (l *Logger) rotate() {
	if l.file == nil {
		return
	}

	// Close current file
	l.file.Close()

	// Rename to timestamped name
	timestamp := time.Now().Format("20060102_150405")
	rotatedPath := fmt.Sprintf("%s.%s", l.filePath, timestamp)
	os.Rename(l.filePath, rotatedPath)

	// Create new file
	if file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		l.file = file
		l.currentSize = 0
		// Update writers
		l.writers = []io.Writer{os.Stdout, file}
	}

	// Cleanup old files
	l.cleanupOldFiles()
}

// cleanupOldFiles removes log files older than maxAge days.
func (l *Logger) cleanupOldFiles() {
	if l.maxAge <= 0 {
		return
	}

	dir := filepath.Dir(l.filePath)
	base := filepath.Base(l.filePath)
	cutoff := time.Now().AddDate(0, 0, -l.maxAge)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), base+".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string) {
	l.log(LevelDebug, "%s", msg)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs an info message.
func (l *Logger) Info(msg string) {
	l.log(LevelInfo, "%s", msg)
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string) {
	l.log(LevelWarn, "%s", msg)
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string) {
	l.log(LevelError, "%s", msg)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// WithField returns a logger entry with a field (for compatibility).
func (l *Logger) WithField(key string, value interface{}) *Logger {
	// For now, just return self - can be enhanced later
	return l
}

// WithFields returns a logger entry with fields (for compatibility).
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	// For now, just return self - can be enhanced later
	return l
}

// =============================================================================
// LOG MAINTENANCE
// =============================================================================

// StartLogRotation launches a background goroutine that truncates the log file
// every 24 hours to prevent disk space exhaustion. The file handle is preserved;
// only the contents are cleared.
func (l *Logger) StartLogRotation(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.truncateLog()
			}
		}
	}()
}

// truncateLog clears the log file contents while keeping the file open.
func (l *Logger) truncateLog() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	// Truncate to zero bytes.
	if err := l.file.Truncate(0); err != nil {
		fmt.Fprintf(os.Stderr, "[LogRotation] Truncate failed: %v\n", err)
		return
	}
	// Seek to beginning so next write starts at offset 0.
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		fmt.Fprintf(os.Stderr, "[LogRotation] Seek failed: %v\n", err)
		return
	}
	l.currentSize = 0

	// Write a marker line so the file is not completely empty.
	marker := fmt.Sprintf("[%s] INFO: === Log cleared by 24h rotation ===\n",
		time.Now().Format("2006-01-02 15:04:05.000"))
	l.file.WriteString(marker)
	l.currentSize = int64(len(marker))
}

// RetroEncryptExistingLog reads the current log file and retroactively encrypts
// any plaintext lines that were written before the encryptor was initialized.
// This ensures early-startup logs are secured before the agent continues.
//
// It is safe to call this multiple times — already-encrypted lines (valid base64)
// are skipped.
func (l *Logger) RetroEncryptExistingLog() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.encryptor == nil || l.filePath == "" {
		return nil // no encryptor or no file — nothing to do
	}

	// Read the entire file.
	data, err := os.ReadFile(l.filePath)
	if err != nil || len(data) == 0 {
		return nil // empty or unreadable — skip
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var encrypted []string
	plaintextFound := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Check if this line is already encrypted (valid base64 that decodes
		// to at least nonce-size bytes). A plaintext log line starts with "[".
		if !strings.HasPrefix(line, "[") {
			// Already encrypted (base64 line) — keep as-is.
			encrypted = append(encrypted, line)
			continue
		}

		// Plaintext line — encrypt it.
		plaintextFound = true
		enc, encErr := l.encryptor.Encrypt([]byte(line + "\n"))
		if encErr != nil {
			// Cannot encrypt — keep original (best-effort).
			encrypted = append(encrypted, line)
			continue
		}
		encrypted = append(encrypted, base64.StdEncoding.EncodeToString(enc))
	}

	if !plaintextFound {
		return nil // nothing to retroactively encrypt
	}

	// Rewrite the file with encrypted contents.
	l.file.Close()
	newContent := strings.Join(encrypted, "\n") + "\n"
	if err := os.WriteFile(l.filePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("retroactive encryption write failed: %w", err)
	}

	// Re-open for append.
	file, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("retroactive encryption reopen failed: %w", err)
	}
	l.file = file
	l.writers = []io.Writer{os.Stdout, file}
	l.currentSize = int64(len(newContent))

	return nil
}

// FilePath returns the log file path (used by external callers for log management).
func (l *Logger) FilePath() string {
	return l.filePath
}
