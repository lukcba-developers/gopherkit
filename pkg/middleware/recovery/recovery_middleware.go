package recovery

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RecoveryConfig configuración del middleware de recovery
type RecoveryConfig struct {
	// Stack trace settings
	EnableStackTrace   bool
	StackSize          int
	SkipFrames         int
	
	// Response settings
	EnableDetailedError bool
	CustomErrorMessage  string
	
	// Logging settings
	LogLevel            logrus.Level
	LogStackTrace       bool
	
	// Callbacks
	OnPanic            func(c *gin.Context, err interface{})
	OnRecovery         func(c *gin.Context, err interface{}, stack []byte)
	
	// Security settings
	SanitizeResponse   bool
	IncludeRequestInfo bool
}

// DefaultRecoveryConfig retorna configuración por defecto
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		EnableStackTrace:    true,
		StackSize:          4096,
		SkipFrames:         2,
		EnableDetailedError: false, // Por seguridad, no exponer detalles en producción
		CustomErrorMessage:  "Internal server error occurred",
		LogLevel:           logrus.ErrorLevel,
		LogStackTrace:      true,
		SanitizeResponse:   true,
		IncludeRequestInfo: true,
	}
}

// PanicInfo información sobre el panic
type PanicInfo struct {
	Error         interface{}       `json:"error"`
	Stack         string           `json:"stack,omitempty"`
	RequestInfo   *RequestInfo     `json:"request_info,omitempty"`
	Timestamp     time.Time        `json:"timestamp"`
	CorrelationID string           `json:"correlation_id,omitempty"`
	RequestID     string           `json:"request_id,omitempty"`
}

// RequestInfo información de la request que causó el panic
type RequestInfo struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      string            `json:"query,omitempty"`
	UserAgent  string            `json:"user_agent,omitempty"`
	ClientIP   string            `json:"client_ip"`
	Headers    map[string]string `json:"headers,omitempty"`
	TenantID   string            `json:"tenant_id,omitempty"`
	UserID     string            `json:"user_id,omitempty"`
}

// RecoveryMiddleware middleware para recuperación de panics
func RecoveryMiddleware(logger *logrus.Logger, config *RecoveryConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get stack trace
				var stack []byte
				if config.EnableStackTrace {
					stack = make([]byte, config.StackSize)
					stack = stack[:runtime.Stack(stack, false)]
				}

				// Create panic info
				panicInfo := &PanicInfo{
					Error:         err,
					Timestamp:     time.Now(),
					CorrelationID: getCorrelationID(c),
					RequestID:     getRequestID(c),
				}

				// Add stack trace if enabled
				if config.EnableStackTrace && len(stack) > 0 {
					panicInfo.Stack = cleanStackTrace(string(stack), config.SkipFrames)
				}

				// Add request info if enabled
				if config.IncludeRequestInfo {
					panicInfo.RequestInfo = extractRequestInfo(c)
				}

				// Log the panic
				logFields := logrus.Fields{
					"panic_error":    err,
					"correlation_id": panicInfo.CorrelationID,
					"request_id":     panicInfo.RequestID,
					"method":         c.Request.Method,
					"path":           c.Request.URL.Path,
					"client_ip":      c.ClientIP(),
				}

				if config.LogStackTrace && len(stack) > 0 {
					logFields["stack_trace"] = panicInfo.Stack
				}

				logEntry := logger.WithFields(logFields)
				
				switch config.LogLevel {
				case logrus.FatalLevel:
					logEntry.Fatal("Panic recovered")
				case logrus.ErrorLevel:
					logEntry.Error("Panic recovered")
				case logrus.WarnLevel:
					logEntry.Warn("Panic recovered")
				default:
					logEntry.Error("Panic recovered")
				}

				// Call panic callback if configured
				if config.OnPanic != nil {
					config.OnPanic(c, err)
				}

				// Call recovery callback if configured
				if config.OnRecovery != nil {
					config.OnRecovery(c, err, stack)
				}

				// Send appropriate response
				sendErrorResponse(c, panicInfo, config)

				// Abort the request
				c.Abort()
			}
		}()

		c.Next()
	}
}

// CustomRecoveryMiddleware middleware de recovery con handler personalizado
func CustomRecoveryMiddleware(logger *logrus.Logger, handler func(c *gin.Context, err interface{})) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic
				stack := make([]byte, 4096)
				stack = stack[:runtime.Stack(stack, false)]

				logger.WithFields(logrus.Fields{
					"panic_error":    err,
					"stack_trace":    string(stack),
					"correlation_id": getCorrelationID(c),
					"request_id":     getRequestID(c),
					"method":         c.Request.Method,
					"path":           c.Request.URL.Path,
					"client_ip":      c.ClientIP(),
				}).Error("Panic recovered with custom handler")

				// Call custom handler
				handler(c, err)
				c.Abort()
			}
		}()

		c.Next()
	}
}

// SecureRecoveryMiddleware middleware de recovery que no expone información sensible
func SecureRecoveryMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	config := &RecoveryConfig{
		EnableStackTrace:    false,
		EnableDetailedError: false,
		CustomErrorMessage:  "An internal error occurred",
		LogLevel:           logrus.ErrorLevel,
		LogStackTrace:      true,
		SanitizeResponse:   true,
		IncludeRequestInfo: false,
	}

	return RecoveryMiddleware(logger, config)
}

// sendErrorResponse envía la respuesta de error apropiada
func sendErrorResponse(c *gin.Context, panicInfo *PanicInfo, config *RecoveryConfig) {
	// Determine HTTP status code based on error type
	statusCode := http.StatusInternalServerError
	
	// Check if it's a specific error type that should return different status
	if httpError, ok := panicInfo.Error.(HTTPError); ok {
		statusCode = httpError.StatusCode
	}

	// Prepare response
	response := gin.H{
		"error":   "internal_server_error",
		"message": config.CustomErrorMessage,
		"code":    "INTERNAL_SERVER_ERROR",
	}

	// Add correlation and request IDs for debugging
	if panicInfo.CorrelationID != "" {
		response["correlation_id"] = panicInfo.CorrelationID
	}
	if panicInfo.RequestID != "" {
		response["request_id"] = panicInfo.RequestID
	}

	// Add detailed error information only if enabled (typically for development)
	if config.EnableDetailedError {
		response["details"] = gin.H{
			"error":     fmt.Sprintf("%v", panicInfo.Error),
			"timestamp": panicInfo.Timestamp.Format(time.RFC3339),
		}

		if config.EnableStackTrace && panicInfo.Stack != "" {
			response["stack_trace"] = panicInfo.Stack
		}

		if config.IncludeRequestInfo && panicInfo.RequestInfo != nil {
			response["request_info"] = panicInfo.RequestInfo
		}
	}

	c.JSON(statusCode, response)
}

// extractRequestInfo extrae información de la request
func extractRequestInfo(c *gin.Context) *RequestInfo {
	requestInfo := &RequestInfo{
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		Query:     c.Request.URL.RawQuery,
		UserAgent: c.Request.UserAgent(),
		ClientIP:  c.ClientIP(),
		Headers:   make(map[string]string),
	}

	// Add tenant and user IDs if available
	if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
		requestInfo.TenantID = tenantID
	}
	if userID := c.GetHeader("X-User-ID"); userID != "" {
		requestInfo.UserID = userID
	}

	// Add selected headers (avoid sensitive ones)
	safeHeaders := []string{
		"Content-Type",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Request-ID",
		"X-Correlation-ID",
		"X-Tenant-ID",
		"X-User-ID",
	}

	for _, header := range safeHeaders {
		if value := c.GetHeader(header); value != "" {
			requestInfo.Headers[header] = value
		}
	}

	return requestInfo
}

// cleanStackTrace limpia y formatea el stack trace
func cleanStackTrace(stack string, skipFrames int) string {
	lines := strings.Split(stack, "\n")
	if len(lines) <= skipFrames*2 {
		return stack
	}

	// Skip the first few frames (recovery middleware frames)
	cleanedLines := lines[skipFrames*2:]
	
	// Limit the number of lines to avoid extremely long stack traces
	maxLines := 50
	if len(cleanedLines) > maxLines {
		cleanedLines = cleanedLines[:maxLines]
		cleanedLines = append(cleanedLines, "... (truncated)")
	}

	return strings.Join(cleanedLines, "\n")
}

// getCorrelationID obtiene el correlation ID del contexto
func getCorrelationID(c *gin.Context) string {
	if correlationID, exists := c.Get("correlation_id"); exists {
		if id, ok := correlationID.(string); ok {
			return id
		}
	}
	return c.GetHeader("X-Correlation-ID")
}

// getRequestID obtiene el request ID del contexto
func getRequestID(c *gin.Context) string {
	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return c.GetHeader("X-Request-ID")
}

// HTTPError representa un error HTTP con código de estado específico
type HTTPError struct {
	StatusCode int
	Message    string
	Details    interface{}
}

func (e HTTPError) Error() string {
	return e.Message
}

// NewHTTPError crea un nuevo HTTPError
func NewHTTPError(statusCode int, message string, details interface{}) HTTPError {
	return HTTPError{
		StatusCode: statusCode,
		Message:    message,
		Details:    details,
	}
}

// PanicHandler función helper para hacer panic con HTTPError
func PanicWithHTTPError(statusCode int, message string, details interface{}) {
	panic(NewHTTPError(statusCode, message, details))
}

// Common panic helpers
func PanicBadRequest(message string, details interface{}) {
	PanicWithHTTPError(http.StatusBadRequest, message, details)
}

func PanicUnauthorized(message string, details interface{}) {
	PanicWithHTTPError(http.StatusUnauthorized, message, details)
}

func PanicForbidden(message string, details interface{}) {
	PanicWithHTTPError(http.StatusForbidden, message, details)
}

func PanicNotFound(message string, details interface{}) {
	PanicWithHTTPError(http.StatusNotFound, message, details)
}

func PanicConflict(message string, details interface{}) {
	PanicWithHTTPError(http.StatusConflict, message, details)
}

func PanicInternalError(message string, details interface{}) {
	PanicWithHTTPError(http.StatusInternalServerError, message, details)
}

// RecoveryStats estadísticas de recovery
type RecoveryStats struct {
	TotalPanics    int64                    `json:"total_panics"`
	PanicsByType   map[string]int64         `json:"panics_by_type"`
	PanicsByPath   map[string]int64         `json:"panics_by_path"`
	LastPanic      time.Time                `json:"last_panic"`
	RecentPanics   []PanicInfo              `json:"recent_panics"`
}

// RecoveryStatsCollector recolecta estadísticas de recovery
type RecoveryStatsCollector struct {
	stats RecoveryStats
	mutex sync.RWMutex
}

// NewRecoveryStatsCollector crea un nuevo recolector de estadísticas
func NewRecoveryStatsCollector() *RecoveryStatsCollector {
	return &RecoveryStatsCollector{
		stats: RecoveryStats{
			PanicsByType: make(map[string]int64),
			PanicsByPath: make(map[string]int64),
			RecentPanics: make([]PanicInfo, 0),
		},
	}
}

// RecordPanic registra un panic
func (rsc *RecoveryStatsCollector) RecordPanic(panicInfo *PanicInfo) {
	rsc.mutex.Lock()
	defer rsc.mutex.Unlock()

	rsc.stats.TotalPanics++
	rsc.stats.LastPanic = panicInfo.Timestamp

	// Record by type
	errorType := fmt.Sprintf("%T", panicInfo.Error)
	rsc.stats.PanicsByType[errorType]++

	// Record by path
	if panicInfo.RequestInfo != nil {
		rsc.stats.PanicsByPath[panicInfo.RequestInfo.Path]++
	}

	// Keep recent panics (last 10)
	rsc.stats.RecentPanics = append(rsc.stats.RecentPanics, *panicInfo)
	if len(rsc.stats.RecentPanics) > 10 {
		rsc.stats.RecentPanics = rsc.stats.RecentPanics[1:]
	}
}

// GetStats retorna las estadísticas actuales
func (rsc *RecoveryStatsCollector) GetStats() RecoveryStats {
	rsc.mutex.RLock()
	defer rsc.mutex.RUnlock()

	// Return a copy
	statsCopy := RecoveryStats{
		TotalPanics:  rsc.stats.TotalPanics,
		PanicsByType: make(map[string]int64),
		PanicsByPath: make(map[string]int64),
		LastPanic:    rsc.stats.LastPanic,
		RecentPanics: make([]PanicInfo, len(rsc.stats.RecentPanics)),
	}

	for k, v := range rsc.stats.PanicsByType {
		statsCopy.PanicsByType[k] = v
	}
	for k, v := range rsc.stats.PanicsByPath {
		statsCopy.PanicsByPath[k] = v
	}
	copy(statsCopy.RecentPanics, rsc.stats.RecentPanics)

	return statsCopy
}

// RecoveryWithStatsMiddleware middleware de recovery que recolecta estadísticas
func RecoveryWithStatsMiddleware(logger *logrus.Logger, collector *RecoveryStatsCollector, config *RecoveryConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	originalOnRecovery := config.OnRecovery
	config.OnRecovery = func(c *gin.Context, err interface{}, stack []byte) {
		// Create panic info
		panicInfo := &PanicInfo{
			Error:         err,
			Timestamp:     time.Now(),
			CorrelationID: getCorrelationID(c),
			RequestID:     getRequestID(c),
		}

		if config.EnableStackTrace && len(stack) > 0 {
			panicInfo.Stack = cleanStackTrace(string(stack), config.SkipFrames)
		}

		if config.IncludeRequestInfo {
			panicInfo.RequestInfo = extractRequestInfo(c)
		}

		// Record in stats
		collector.RecordPanic(panicInfo)

		// Call original callback if it exists
		if originalOnRecovery != nil {
			originalOnRecovery(c, err, stack)
		}
	}

	return RecoveryMiddleware(logger, config)
}