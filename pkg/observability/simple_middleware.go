package observability

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// SimpleObservabilityMiddleware provides basic observability without OpenTelemetry
type SimpleObservabilityMiddleware struct {
	logger logger.Logger
	config Config
}

// NewSimpleObservabilityMiddleware creates a basic observability middleware
func NewSimpleObservabilityMiddleware(config Config, logger logger.Logger) *SimpleObservabilityMiddleware {
	return &SimpleObservabilityMiddleware{
		logger: logger,
		config: config,
	}
}

// Metrics middleware records basic HTTP metrics (simplified)
func (som *SimpleObservabilityMiddleware) Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// Simple metric recording without OpenTelemetry
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// Log metrics as structured logs
		som.logger.WithContext(c.Request.Context()).WithFields(map[string]interface{}{
			"event_type":     "http_request",
			"method":         c.Request.Method,
			"path":           c.FullPath(),
			"status_code":    statusCode,
			"duration_ms":    duration.Milliseconds(),
			"response_size":  c.Writer.Size(),
		}).Info("HTTP request processed")
	}
}

// Logging middleware provides structured logging for HTTP requests
func (som *SimpleObservabilityMiddleware) Logging(logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		// Calculate request duration
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// Build log fields
		fields := map[string]interface{}{
			"method":        c.Request.Method,
			"path":          path,
			"status_code":   statusCode,
			"duration_ms":   duration.Milliseconds(),
			"client_ip":     c.ClientIP(),
			"user_agent":    c.Request.UserAgent(),
			"response_size": c.Writer.Size(),
		}

		if raw != "" {
			fields["query"] = raw
		}

		// Add error information if present
		if len(c.Errors) > 0 {
			fields["errors"] = c.Errors.String()
		}

		// Log at appropriate level based on status code
		ctx := c.Request.Context()
		if statusCode >= 500 {
			logger.LogError(ctx, nil, "HTTP server error", fields)
		} else if statusCode >= 400 {
			logger.WithContext(ctx).WithFields(fields).Warn("HTTP client error")
		} else if duration > 1*time.Second {
			logger.LogPerformanceEvent(ctx, "slow_http_request", duration, fields)
		} else {
			logger.WithContext(ctx).WithFields(fields).Info("HTTP request completed")
		}
	}
}

// Tracing middleware provides basic request tracing (simplified without OpenTelemetry)
func (som *SimpleObservabilityMiddleware) Tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Add basic trace information to logs
		som.logger.WithContext(c.Request.Context()).WithFields(map[string]interface{}{
			"trace_event": "request_start",
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
		}).Debug("Request started")

		c.Next()

		som.logger.WithContext(c.Request.Context()).WithFields(map[string]interface{}{
			"trace_event":  "request_end",
			"method":       c.Request.Method,
			"path":         c.Request.URL.Path,
			"status_code":  c.Writer.Status(),
		}).Debug("Request completed")
	}
}

// BusinessMetrics middleware records business-specific metrics
func (som *SimpleObservabilityMiddleware) BusinessMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		endpoint := c.FullPath()

		// Record business events based on endpoint patterns
		labels := map[string]interface{}{
			"endpoint": endpoint,
			"method":   c.Request.Method,
		}

		// Add tenant info if available
		if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
			labels["tenant_id"] = tenantID
		}

		// Add user info if available
		if userID := c.GetHeader("X-User-ID"); userID != "" {
			labels["user_id"] = userID
		}

		// Determine business event type
		eventType := "api_call"
		if strings.Contains(endpoint, "/auth/") {
			eventType = "authentication"
		} else if strings.Contains(endpoint, "/payment/") {
			eventType = "payment"
		} else if strings.Contains(endpoint, "/user/") {
			eventType = "user_management"
		}

		som.logger.LogBusinessEvent(c.Request.Context(), eventType, labels)

		// Record performance events for slow requests
		if duration > 1*time.Second {
			som.logger.LogPerformanceEvent(c.Request.Context(), "slow_api_call", duration, map[string]interface{}{
				"endpoint": endpoint,
				"method":   c.Request.Method,
			})
		}
	}
}