package logging

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Constants for context keys
const (
	LoggerKey = "logger"
)

// LoggingConfig configuración del middleware de logging
type LoggingConfig struct {
	// Request/Response logging
	LogRequests     bool
	LogResponses    bool
	LogHeaders      bool
	LogBody         bool
	LogQueryParams  bool
	
	// Performance logging
	LogSlowRequests bool
	SlowThreshold   time.Duration
	
	// Security logging
	LogFailedAuth   bool
	LogSuspicious   bool
	
	// Filtering
	SkipPaths       []string
	SkipMethods     []string
	MaxBodySize     int
	
	// Privacy
	SensitiveHeaders []string
	MaskSensitive    bool
}

// DefaultLoggingConfig retorna configuración por defecto
func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		LogRequests:      true,
		LogResponses:     true,
		LogHeaders:       false,
		LogBody:          false,
		LogQueryParams:   true,
		LogSlowRequests:  true,
		SlowThreshold:    time.Second * 2,
		LogFailedAuth:    true,
		LogSuspicious:    true,
		SkipPaths:        []string{"/health", "/metrics", "/ping"},
		SkipMethods:      []string{},
		MaxBodySize:      1024 * 10, // 10KB
		SensitiveHeaders: []string{"Authorization", "X-API-Key", "Cookie", "Set-Cookie"},
		MaskSensitive:    true,
	}
}

// responseBodyWriter wrapper para capturar el body de respuesta
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// LoggingMiddleware middleware de logging estructurado
func LoggingMiddleware(logger *logrus.Logger, config *LoggingConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultLoggingConfig()
	}

	return func(c *gin.Context) {
		// Skip logging for certain paths
		if shouldSkipPath(c.Request.URL.Path, config.SkipPaths) {
			c.Next()
			return
		}

		// Skip logging for certain methods
		if shouldSkipMethod(c.Request.Method, config.SkipMethods) {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Generate correlation ID if not present
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
			c.Header("X-Correlation-ID", correlationID)
		}
		c.Set("correlation_id", correlationID)

		// Generate request ID
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		// Prepare base log fields
		baseFields := logrus.Fields{
			"correlation_id": correlationID,
			"request_id":     requestID,
			"method":         c.Request.Method,
			"path":           path,
			"client_ip":      c.ClientIP(),
			"user_agent":     c.Request.UserAgent(),
		}

		// Add tenant ID if available
		if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
			baseFields["tenant_id"] = tenantID
		}

		// Add user ID if available
		if userID := c.GetHeader("X-User-ID"); userID != "" {
			baseFields["user_id"] = userID
		}

		// Add query parameters if enabled
		if config.LogQueryParams && raw != "" {
			baseFields["query"] = raw
		}

		// Capture request body if enabled
		var requestBody []byte
		if config.LogBody {
			requestBody = readAndRestoreBody(c)
			if len(requestBody) > 0 && len(requestBody) <= config.MaxBodySize {
				baseFields["request_body"] = string(requestBody)
			}
		}

		// Log request headers if enabled
		if config.LogHeaders {
			headers := make(map[string]string)
			for name, values := range c.Request.Header {
				if len(values) > 0 {
					if config.MaskSensitive && isSensitiveHeader(name, config.SensitiveHeaders) {
						headers[name] = "[MASKED]"
					} else {
						headers[name] = values[0]
					}
				}
			}
			baseFields["request_headers"] = headers
		}

		// Log incoming request
		if config.LogRequests {
			logger.WithFields(baseFields).Info("Incoming request")
		}

		// Wrap response writer to capture response body
		var responseBody *bytes.Buffer
		if config.LogResponses && config.LogBody {
			responseBody = &bytes.Buffer{}
			c.Writer = &responseBodyWriter{
				ResponseWriter: c.Writer,
				body:          responseBody,
			}
		}

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)
		statusCode := c.Writer.Status()

		// Prepare response log fields
		responseFields := logrus.Fields{
			"correlation_id": correlationID,
			"request_id":     requestID,
			"method":         c.Request.Method,
			"path":           path,
			"client_ip":      c.ClientIP(),
			"status_code":    statusCode,
			"latency":        latency.String(),
			"latency_ms":     latency.Milliseconds(),
		}

		// Add response body if enabled and captured
		if config.LogResponses && config.LogBody && responseBody != nil {
			if responseBody.Len() <= config.MaxBodySize {
				responseFields["response_body"] = responseBody.String()
			}
		}

		// Add response headers if enabled
		if config.LogHeaders {
			responseHeaders := make(map[string]string)
			for name, values := range c.Writer.Header() {
				if len(values) > 0 {
					if config.MaskSensitive && isSensitiveHeader(name, config.SensitiveHeaders) {
						responseHeaders[name] = "[MASKED]"
					} else {
						responseHeaders[name] = values[0]
					}
				}
			}
			responseFields["response_headers"] = responseHeaders
		}

		// Log response with appropriate level
		logLevel := getLogLevel(statusCode, latency, config)
		logEntry := logger.WithFields(responseFields)

		switch logLevel {
		case logrus.ErrorLevel:
			logEntry.Error("Request completed with error")
		case logrus.WarnLevel:
			logEntry.Warn("Request completed with warning")
		case logrus.InfoLevel:
			logEntry.Info("Request completed")
		default:
			logEntry.Debug("Request completed")
		}

		// Log slow requests if enabled
		if config.LogSlowRequests && latency > config.SlowThreshold {
			logger.WithFields(responseFields).WithField("slow_request", true).
				Warn("Slow request detected")
		}

		// Log authentication failures if enabled
		if config.LogFailedAuth && (statusCode == 401 || statusCode == 403) {
			logger.WithFields(responseFields).WithField("auth_failure", true).
				Warn("Authentication/authorization failure")
		}

		// Log suspicious activity if enabled
		if config.LogSuspicious && isSuspiciousRequest(c, statusCode) {
			logger.WithFields(responseFields).WithField("suspicious", true).
				Warn("Suspicious request detected")
		}
	}
}

// StructuredLoggerMiddleware configura el logger estructurado para el contexto
func StructuredLoggerMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.GetString("correlation_id")
		requestID := c.GetString("request_id")
		
		contextLogger := logger.WithFields(logrus.Fields{
			"correlation_id": correlationID,
			"request_id":     requestID,
		})
		
		c.Set("logger", contextLogger)
		c.Next()
	}
}

// GetLoggerFromContext obtiene el logger del contexto
func GetLoggerFromContext(c *gin.Context) *logrus.Entry {
	if logger, exists := c.Get("logger"); exists {
		if contextLogger, ok := logger.(*logrus.Entry); ok {
			return contextLogger
		}
	}
	
	// Fallback to default logger
	return logrus.NewEntry(logrus.StandardLogger())
}

// RequestLogger crea un logger específico para una request
func RequestLogger(c *gin.Context, component string) *logrus.Entry {
	baseLogger := GetLoggerFromContext(c)
	return baseLogger.WithField("component", component)
}

// Helper functions

func shouldSkipPath(path string, skipPaths []string) bool {
	for _, skipPath := range skipPaths {
		if path == skipPath {
			return true
		}
	}
	return false
}

func shouldSkipMethod(method string, skipMethods []string) bool {
	for _, skipMethod := range skipMethods {
		if method == skipMethod {
			return true
		}
	}
	return false
}

func isSensitiveHeader(headerName string, sensitiveHeaders []string) bool {
	for _, sensitive := range sensitiveHeaders {
		if headerName == sensitive {
			return true
		}
	}
	return false
}

func readAndRestoreBody(req *gin.Context) []byte {
	if req.Request.Body == nil {
		return nil
	}

	body, err := io.ReadAll(req.Request.Body)
	if err != nil {
		return nil
	}

	// Restore body for next middleware/handler
	req.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body
}

func getLogLevel(statusCode int, latency time.Duration, config *LoggingConfig) logrus.Level {
	if statusCode >= 500 {
		return logrus.ErrorLevel
	}
	if statusCode >= 400 {
		return logrus.WarnLevel
	}
	if config.LogSlowRequests && latency > config.SlowThreshold {
		return logrus.WarnLevel
	}
	return logrus.InfoLevel
}

func isSuspiciousRequest(c *gin.Context, statusCode int) bool {
	// Simple heuristics for suspicious activity
	userAgent := c.Request.UserAgent()
	
	// Check for common bot patterns
	suspiciousUserAgents := []string{
		"bot", "crawler", "spider", "scraper",
		"curl", "wget", "python", "go-http-client",
	}
	
	for _, suspicious := range suspiciousUserAgents {
		if bytes.Contains([]byte(userAgent), []byte(suspicious)) {
			return true
		}
	}
	
	// Check for multiple 404s (potential scanning)
	if statusCode == 404 {
		return true
	}
	
	// Check for rapid requests (basic rate limiting detection)
	// This would need more sophisticated implementation in production
	
	return false
}

// SecurityLogger logs security events
func SecurityLogger(logger *logrus.Logger) *logrus.Entry {
	return logger.WithField("category", "security")
}

// AuditLogger logs audit events
func AuditLogger(logger *logrus.Logger) *logrus.Entry {
	return logger.WithField("category", "audit")
}

// BusinessLogger logs business events
func BusinessLogger(logger *logrus.Logger) *logrus.Entry {
	return logger.WithField("category", "business")
}