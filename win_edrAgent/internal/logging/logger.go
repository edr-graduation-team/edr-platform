// Package logging provides file-based logging with rotation for the EDR Agent.
package logging

import (
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
		if _, err := w.Write([]byte(entry)); err == nil {
			if w == l.file {
				l.currentSize += int64(len(entry))
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
