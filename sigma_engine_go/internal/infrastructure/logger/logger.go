package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var defaultLogger *logrus.Logger

func init() {
	defaultLogger = logrus.New()
	defaultLogger.SetOutput(os.Stdout)
	defaultLogger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})
	defaultLogger.SetLevel(logrus.InfoLevel)
}

// Logger returns the default logger instance.
func Logger() *logrus.Logger {
	return defaultLogger
}

// SetLevel sets the logging level.
func SetLevel(level string) {
	switch level {
	case "debug":
		defaultLogger.SetLevel(logrus.DebugLevel)
	case "info":
		defaultLogger.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		defaultLogger.SetLevel(logrus.WarnLevel)
	case "error":
		defaultLogger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		defaultLogger.SetLevel(logrus.FatalLevel)
	case "panic":
		defaultLogger.SetLevel(logrus.PanicLevel)
	default:
		defaultLogger.SetLevel(logrus.InfoLevel)
	}
}

// Debug logs a debug message.
func Debug(args ...interface{}) {
	defaultLogger.Debug(args...)
}

// Debugf logs a formatted debug message.
func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

// Info logs an info message.
func Info(args ...interface{}) {
	defaultLogger.Info(args...)
}

// Infof logs a formatted info message.
func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

// Warn logs a warning message.
func Warn(args ...interface{}) {
	defaultLogger.Warn(args...)
}

// Warnf logs a formatted warning message.
func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

// Error logs an error message.
func Error(args ...interface{}) {
	defaultLogger.Error(args...)
}

// Errorf logs a formatted error message.
func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
}

// Fatal logs a fatal message and exits.
func Fatal(args ...interface{}) {
	defaultLogger.Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits.
func Fatalf(format string, args ...interface{}) {
	defaultLogger.Fatalf(format, args...)
}

