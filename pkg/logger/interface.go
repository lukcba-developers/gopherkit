package logger

import (
	"context"
	"time"
)

// Logger defines the interface for contextual logging
type Logger interface {
	// Context-aware logging
	WithContext(ctx context.Context) LoggerEntry
	WithTenant(tenantID string) LoggerEntry
	WithUser(userID string) LoggerEntry
	WithCorrelation(correlationID string) LoggerEntry
	WithOperation(operation string) LoggerEntry
	WithFields(fields map[string]interface{}) LoggerEntry
	WithError(err error) LoggerEntry

	// Business event logging
	LogBusinessEvent(ctx context.Context, event string, data map[string]interface{})
	LogSecurityEvent(ctx context.Context, event string, data map[string]interface{})
	LogPerformanceEvent(ctx context.Context, operation string, duration time.Duration, data map[string]interface{})
	LogError(ctx context.Context, err error, message string, data map[string]interface{})
	LogDatabaseOperation(ctx context.Context, query string, duration time.Duration, rowsAffected int64)
	LogExternalAPICall(ctx context.Context, service string, method string, url string, statusCode int, duration time.Duration)
}

// LoggerEntry defines the interface for a log entry
type LoggerEntry interface {
	// Standard logging levels
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})

	// Formatted logging
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})

	// Context and field manipulation
	WithContext(ctx context.Context) LoggerEntry
	WithField(key string, value interface{}) LoggerEntry
	WithFields(fields map[string]interface{}) LoggerEntry
	WithError(err error) LoggerEntry
}

// Factory function
func NewLogger(serviceName string) Logger {
	return New(serviceName)
}