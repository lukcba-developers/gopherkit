package logger

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

// ContextualLogger provides structured logging with context awareness
type ContextualLogger struct {
	*logrus.Logger
	serviceName string
}

// LogEntry represents a log entry with context
type LogEntry struct {
	*logrus.Entry
}

// Implement LoggerEntry interface methods for LogEntry
func (e *LogEntry) WithContext(ctx context.Context) LoggerEntry {
	return &LogEntry{Entry: e.Entry.WithContext(ctx)}
}

func (e *LogEntry) WithField(key string, value interface{}) LoggerEntry {
	return &LogEntry{Entry: e.Entry.WithField(key, value)}
}

func (e *LogEntry) WithFields(fields map[string]interface{}) LoggerEntry {
	return &LogEntry{Entry: e.Entry.WithFields(logrus.Fields(fields))}
}

func (e *LogEntry) WithError(err error) LoggerEntry {
	return &LogEntry{Entry: e.Entry.WithError(err)}
}

// New creates a new contextual logger
func New(serviceName string) *ContextualLogger {
	logger := logrus.New()

	// Configure based on environment
	if isProduction() {
		// Production - JSON formatting for log aggregation
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "caller",
			},
		})
		logger.SetLevel(logrus.InfoLevel)
	} else {
		// Development - Human readable format
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			ForceColors:     true,
		})
		logger.SetLevel(getLogLevel())
	}

	logger.SetOutput(os.Stdout)
	
	// Set default fields
	logger = logger.WithField("service", serviceName).Logger

	return &ContextualLogger{
		Logger:      logger,
		serviceName: serviceName,
	}
}

// WithContext creates a log entry with context information
func (l *ContextualLogger) WithContext(ctx context.Context) LoggerEntry {
	entry := l.Logger.WithContext(ctx)
	
	// Add tenant ID if available
	if tenantID := getTenantFromContext(ctx); tenantID != "" {
		entry = entry.WithField("tenant_id", tenantID)
	}
	
	// Add correlation ID if available
	if correlationID := getCorrelationIDFromContext(ctx); correlationID != "" {
		entry = entry.WithField("correlation_id", correlationID)
	}
	
	// Add user ID if available
	if userID := getUserIDFromContext(ctx); userID != "" {
		entry = entry.WithField("user_id", userID)
	}
	
	// Add trace information if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		entry = entry.WithFields(logrus.Fields{
			"trace_id": span.SpanContext().TraceID().String(),
			"span_id":  span.SpanContext().SpanID().String(),
		})
	}
	
	// Add request ID if available
	if requestID := getRequestIDFromContext(ctx); requestID != "" {
		entry = entry.WithField("request_id", requestID)
	}
	
	return &LogEntry{Entry: entry}
}

// WithTenant creates a log entry with tenant information
func (l *ContextualLogger) WithTenant(tenantID string) LoggerEntry {
	return &LogEntry{Entry: l.Logger.WithField("tenant_id", tenantID)}
}

// WithUser creates a log entry with user information
func (l *ContextualLogger) WithUser(userID string) LoggerEntry {
	return &LogEntry{Entry: l.Logger.WithField("user_id", userID)}
}

// WithCorrelation creates a log entry with correlation ID
func (l *ContextualLogger) WithCorrelation(correlationID string) LoggerEntry {
	return &LogEntry{Entry: l.Logger.WithField("correlation_id", correlationID)}
}

// WithOperation creates a log entry with operation information
func (l *ContextualLogger) WithOperation(operation string) LoggerEntry {
	return &LogEntry{Entry: l.Logger.WithField("operation", operation)}
}

// WithFields creates a log entry with multiple fields
func (l *ContextualLogger) WithFields(fields map[string]interface{}) LoggerEntry {
	return &LogEntry{Entry: l.Logger.WithFields(logrus.Fields(fields))}
}

// WithError creates a log entry with error information
func (l *ContextualLogger) WithError(err error) LoggerEntry {
	return &LogEntry{Entry: l.Logger.WithError(err)}
}

// Business operation methods
func (l *ContextualLogger) LogBusinessEvent(ctx context.Context, event string, data map[string]interface{}) {
	entry := l.WithContext(ctx).WithFields(data).WithField("event_type", "business")
	entry.Info(event)
}

func (l *ContextualLogger) LogSecurityEvent(ctx context.Context, event string, data map[string]interface{}) {
	entry := l.WithContext(ctx).WithFields(data).WithField("event_type", "security")
	entry.Warn(event)
}

func (l *ContextualLogger) LogPerformanceEvent(ctx context.Context, operation string, duration time.Duration, data map[string]interface{}) {
	fields := make(map[string]interface{})
	if data != nil {
		for k, v := range data {
			fields[k] = v
		}
	}
	fields["event_type"] = "performance"
	fields["operation"] = operation
	fields["duration_ms"] = duration.Milliseconds()
	
	entry := l.WithContext(ctx).WithFields(fields)
	if duration > 1*time.Second {
		entry.Warn("slow operation detected")
	} else {
		entry.Debug("operation completed")
	}
}

func (l *ContextualLogger) LogError(ctx context.Context, err error, message string, data map[string]interface{}) {
	entry := l.WithContext(ctx).WithError(err)
	if data != nil {
		entry = entry.WithFields(data)
	}
	entry.Error(message)
}

func (l *ContextualLogger) LogDatabaseOperation(ctx context.Context, query string, duration time.Duration, rowsAffected int64) {
	entry := l.WithContext(ctx).WithFields(map[string]interface{}{
		"event_type":     "database",
		"query":          maskSensitiveData(query),
		"duration_ms":    duration.Milliseconds(),
		"rows_affected":  rowsAffected,
	})
	
	if duration > 100*time.Millisecond {
		entry.Warn("slow database query")
	} else {
		entry.Debug("database query completed")
	}
}

func (l *ContextualLogger) LogExternalAPICall(ctx context.Context, service string, method string, url string, statusCode int, duration time.Duration) {
	entry := l.WithContext(ctx).WithFields(map[string]interface{}{
		"event_type":   "external_api",
		"service":      service,
		"method":       method,
		"url":          maskSensitiveData(url),
		"status_code":  statusCode,
		"duration_ms":  duration.Milliseconds(),
	})
	
	if statusCode >= 400 {
		entry.Error("external API call failed")
	} else if duration > 5*time.Second {
		entry.Warn("slow external API call")
	} else {
		entry.Info("external API call completed")
	}
}

// Context extraction functions
func getTenantFromContext(ctx context.Context) string {
	if tenantID, ok := ctx.Value("tenant_id").(string); ok {
		return tenantID
	}
	if tenantID, ok := ctx.Value("tenantID").(string); ok {
		return tenantID
	}
	return ""
}

func getCorrelationIDFromContext(ctx context.Context) string {
	if correlationID, ok := ctx.Value("correlation_id").(string); ok {
		return correlationID
	}
	if correlationID, ok := ctx.Value("correlationID").(string); ok {
		return correlationID
	}
	if correlationID, ok := ctx.Value("X-Correlation-ID").(string); ok {
		return correlationID
	}
	return ""
}

func getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	if userID, ok := ctx.Value("userID").(string); ok {
		return userID
	}
	return ""
}

func getRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}
	if requestID, ok := ctx.Value("requestID").(string); ok {
		return requestID
	}
	if requestID, ok := ctx.Value("X-Request-ID").(string); ok {
		return requestID
	}
	return ""
}

// Utility functions
func isProduction() bool {
	env := os.Getenv("ENVIRONMENT")
	return env == "production" || env == "prod"
}

func getLogLevel() logrus.Level {
	levelStr := os.Getenv("LOG_LEVEL")
	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		return logrus.InfoLevel
	}
	return level
}

// maskSensitiveData masks sensitive information in logs
func maskSensitiveData(data string) string {
	// Simple masking - in production, implement more sophisticated masking
	// This is a basic implementation to prevent logging sensitive data
	return data // TODO: Implement proper masking
}