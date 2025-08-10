package observability

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// ObservabilityMiddleware provides observability middleware
type ObservabilityMiddleware struct {
	logger           logger.Logger
	metricsCollector *MetricsCollector
	tracer          trace.Tracer
	config          Config
}

// Config holds observability middleware configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	TracingEnabled bool
	MetricsEnabled bool
}

// NewObservabilityMiddleware creates a new observability middleware
func NewObservabilityMiddleware(config Config, logger logger.Logger) (*ObservabilityMiddleware, error) {
	var metricsCollector *MetricsCollector
	var err error

	if config.MetricsEnabled {
		metricsConfig := MetricsConfig{
			ServiceName:    config.ServiceName,
			ServiceVersion: config.ServiceVersion,
			Enabled:        true,
		}
		metricsCollector, err = NewMetricsCollector(metricsConfig, logger)
		if err != nil {
			return nil, err
		}
	}

	var tracer trace.Tracer
	if config.TracingEnabled {
		tracer = otel.Tracer(config.ServiceName)
	}

	return &ObservabilityMiddleware{
		logger:           logger,
		metricsCollector: metricsCollector,
		tracer:          tracer,
		config:          config,
	}, nil
}

// Metrics middleware records HTTP metrics
func (om *ObservabilityMiddleware) Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !om.config.MetricsEnabled || om.metricsCollector == nil {
			c.Next()
			return
		}

		start := time.Now()
		
		// Increment in-flight requests
		om.metricsCollector.IncHTTPRequestsInFlight(c.Request.Context())
		defer om.metricsCollector.DecHTTPRequestsInFlight(c.Request.Context())

		c.Next()

		// Record metrics after request completion
		duration := time.Since(start)
		statusCode := c.Writer.Status()
		
		om.metricsCollector.RecordHTTPRequest(
			c.Request.Context(),
			c.Request.Method,
			c.FullPath(),
			statusCode,
			duration,
		)

		// Record errors if status code indicates an error
		if statusCode >= 400 {
			errorType := "client_error"
			if statusCode >= 500 {
				errorType = "server_error"
			}
			om.metricsCollector.RecordError(c.Request.Context(), errorType, "http")
		}
	}
}

// Logging middleware provides structured logging for HTTP requests
func (om *ObservabilityMiddleware) Logging(logger logger.Logger) gin.HandlerFunc {
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

// Tracing middleware provides distributed tracing
func (om *ObservabilityMiddleware) Tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !om.config.TracingEnabled || om.tracer == nil {
			c.Next()
			return
		}

		// Extract tracing context from incoming request
		ctx := c.Request.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(c.Request.Header))

		// Start a new span
		spanName := c.Request.Method + " " + c.FullPath()
		ctx, span := om.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		// Set span attributes
		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.url", c.Request.URL.String()),
			attribute.String("http.scheme", c.Request.URL.Scheme),
			attribute.String("http.host", c.Request.Host),
			attribute.String("http.user_agent", c.Request.UserAgent()),
			attribute.String("http.client_ip", c.ClientIP()),
		)

		// Add span to context
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		// Set response attributes
		statusCode := c.Writer.Status()
		span.SetAttributes(
			attribute.Int("http.status_code", statusCode),
			attribute.Int("http.response_size", c.Writer.Size()),
		)

		// Set span status based on HTTP status code
		if statusCode >= 400 {
			span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Record any errors
		if len(c.Errors) > 0 {
			span.RecordError(c.Errors.Last())
		}
	}
}

// BusinessMetrics middleware records business-specific metrics
func (om *ObservabilityMiddleware) BusinessMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		if !om.config.MetricsEnabled || om.metricsCollector == nil {
			return
		}

		// Extract business context
		duration := time.Since(start)
		endpoint := c.FullPath()

		// Record business events based on endpoint patterns
		labels := map[string]string{
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

		om.metricsCollector.RecordBusinessEvent(c.Request.Context(), eventType, labels)

		// Record performance events for slow requests
		if duration > 1*time.Second {
			om.logger.LogPerformanceEvent(c.Request.Context(), "slow_api_call", duration, map[string]interface{}{
				"endpoint": endpoint,
				"method":   c.Request.Method,
			})
		}
	}
}

// GetMetricsCollector returns the metrics collector for additional custom metrics
func (om *ObservabilityMiddleware) GetMetricsCollector() *MetricsCollector {
	return om.metricsCollector
}