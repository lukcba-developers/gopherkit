package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// HealthStatus representa el estado de salud
type HealthStatus struct {
	Status      string                 `json:"status"`
	Version     string                 `json:"version"`
	Service     string                 `json:"service"`
	Timestamp   string                 `json:"timestamp"`
	Uptime      string                 `json:"uptime,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

var startTime = time.Now()

// BasicHealthHandler handler básico de health check
func BasicHealthHandler(serviceName, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		health := HealthStatus{
			Status:    "healthy",
			Version:   version,
			Service:   serviceName,
			Timestamp: time.Now().Format(time.RFC3339),
		}
		
		c.JSON(http.StatusOK, health)
	}
}

// DetailedHealthHandler handler detallado de health check
func DetailedHealthHandler(stack *MiddlewareStack) gin.HandlerFunc {
	return func(c *gin.Context) {
		health := HealthStatus{
			Status:      "healthy",
			Version:     stack.ServiceVersion,
			Service:     stack.ServiceName,
			Timestamp:   time.Now().Format(time.RFC3339),
			Uptime:      time.Since(startTime).String(),
			Environment: stack.Environment,
			Details:     make(map[string]interface{}),
		}
		
		// Check individual components
		componentStatus := make(map[string]string)
		overallHealthy := true
		
		// Check Redis if available
		if stack.RedisClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			
			if err := stack.RedisClient.Ping(ctx).Err(); err != nil {
				componentStatus["redis"] = "unhealthy"
				overallHealthy = false
				stack.Logger.WithError(err).Warn("Redis health check failed")
			} else {
				componentStatus["redis"] = "healthy"
			}
		}
		
		// Check circuit breakers
		if stack.EnableCircuitBreaker && stack.CircuitBreakerMgr != nil {
			cbHealth := stack.CircuitBreakerMgr.HealthCheck()
			allCBHealthy := true
			for _, state := range cbHealth {
				if state == "OPEN" {
					allCBHealthy = false
					break
				}
			}
			if allCBHealthy {
				componentStatus["circuit_breakers"] = "healthy"
			} else {
				componentStatus["circuit_breakers"] = "degraded"
				// Circuit breakers open doesn't mean unhealthy, just degraded
			}
			health.Details["circuit_breakers"] = cbHealth
		}
		
		// Add middleware status
		middlewareStatus := map[string]bool{
			"cors":            stack.EnableCORS,
			"logging":         stack.EnableLogging,
			"metrics":         stack.EnableMetrics,
			"rate_limit":      stack.EnableRateLimit,
			"circuit_breaker": stack.EnableCircuitBreaker,
			"recovery":        stack.EnableRecovery,
			"tenant_context":  stack.EnableTenantContext,
		}
		health.Details["middleware"] = middlewareStatus
		health.Details["components"] = componentStatus
		
		// Add stats if available
		if stack.EnableMetrics && stack.MetricsCollector != nil {
			health.Details["metrics_summary"] = "metrics_available"
		}
		
		// Set overall status
		if !overallHealthy {
			health.Status = "unhealthy"
			c.JSON(http.StatusServiceUnavailable, health)
		} else {
			c.JSON(http.StatusOK, health)
		}
	}
}

// ReadinessHandler verifica si el servicio está listo para recibir tráfico
func ReadinessHandler(stack *MiddlewareStack) gin.HandlerFunc {
	return func(c *gin.Context) {
		ready := true
		details := make(map[string]interface{})
		
		// Check Redis connectivity if enabled for critical features
		if stack.EnableRateLimit && stack.RateLimitConfig.UseRedis && stack.RedisClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			
			if err := stack.RedisClient.Ping(ctx).Err(); err != nil {
				ready = false
				details["redis_error"] = err.Error()
			}
		}
		
		// Check if critical circuit breakers are open
		if stack.EnableCircuitBreaker && stack.CircuitBreakerMgr != nil {
			cbHealth := stack.CircuitBreakerMgr.HealthCheck()
			openCircuits := 0
			for name, state := range cbHealth {
				if state == "OPEN" {
					openCircuits++
					details["circuit_breaker_"+name] = "open"
				}
			}
			// If too many circuit breakers are open, consider not ready
			if openCircuits > len(cbHealth)/2 {
				ready = false
			}
		}
		
		response := gin.H{
			"ready":     ready,
			"service":   stack.ServiceName,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		
		if len(details) > 0 {
			response["details"] = details
		}
		
		if ready {
			c.JSON(http.StatusOK, response)
		} else {
			c.JSON(http.StatusServiceUnavailable, response)
		}
	}
}

// LivenessHandler verifica si el servicio está vivo (no colgado)
func LivenessHandler(stack *MiddlewareStack) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Simple liveness check - if we can respond, we're alive
		response := gin.H{
			"alive":     true,
			"service":   stack.ServiceName,
			"timestamp": time.Now().Format(time.RFC3339),
			"uptime":    time.Since(startTime).String(),
		}
		
		// Add goroutine count as a basic health indicator
		// In production, you might add more sophisticated checks
		response["goroutines"] = 0 // Would get runtime.NumGoroutine() in a real implementation
		
		c.JSON(http.StatusOK, response)
	}
}

// TenantContextMiddleware extrae y valida el contexto de tenant
func TenantContextMiddleware(logger *logrus.Logger, requireTenant bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip tenant validation for infrastructure endpoints
		if isInfrastructureEndpoint(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Extract tenant ID from headers
		tenantID := extractTenantID(c)
		
		// Validate tenant ID if required
		if requireTenant && tenantID == "" {
			logger.WithFields(logrus.Fields{
				"client_ip":  c.ClientIP(),
				"user_agent": c.GetHeader("User-Agent"),
				"endpoint":   c.Request.URL.Path,
				"method":     c.Request.Method,
			}).Warn("Missing required tenant ID")
			
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "missing_tenant_id",
				"message": "X-Tenant-ID header is required",
				"code":    "TENANT_REQUIRED",
			})
			c.Abort()
			return
		}
		
		// Add tenant ID to context if provided
		if tenantID != "" {
			// Validate tenant ID format (basic UUID validation)
			if !isValidTenantIDFormat(tenantID) {
				logger.WithFields(logrus.Fields{
					"tenant_id": tenantID,
					"client_ip": c.ClientIP(),
					"endpoint":  c.Request.URL.Path,
					"method":    c.Request.Method,
				}).Warn("Invalid tenant ID format")
				
				c.JSON(http.StatusBadRequest, gin.H{
					"error":     "invalid_tenant_id_format",
					"message":   "Tenant ID must be a valid UUID format",
					"code":      "INVALID_TENANT_FORMAT",
					"tenant_id": tenantID,
				})
				c.Abort()
				return
			}

			// Add to Gin context
			c.Set("tenant_id", tenantID)
			
			// Add to request context for downstream propagation
			ctx := context.WithValue(c.Request.Context(), "tenant_id", tenantID)
			c.Request = c.Request.WithContext(ctx)
			
			// Log tenant context establishment
			logger.WithFields(logrus.Fields{
				"tenant_id": tenantID,
				"endpoint":  c.Request.URL.Path,
				"method":    c.Request.Method,
				"client_ip": c.ClientIP(),
			}).Debug("Tenant context established")
		}
		
		c.Next()
	}
}

// Helper functions

func extractTenantID(c *gin.Context) string {
	// Check common tenant ID headers in priority order
	tenantHeaders := []string{
		"X-Tenant-ID",
		"X-Tenant-Id", 
		"Tenant-ID",
		"Tenant-Id",
		"X-Organization-ID",
		"Organization-ID",
	}
	
	for _, header := range tenantHeaders {
		if tenantID := c.GetHeader(header); tenantID != "" {
			return tenantID
		}
	}
	
	// Check query parameter as fallback for development/testing
	if tenantID := c.Query("tenant_id"); tenantID != "" {
		return tenantID
	}
	
	return ""
}

func isValidTenantIDFormat(tenantID string) bool {
	// Basic UUID format validation
	if len(tenantID) != 36 {
		return false
	}
	
	// Simple check for UUID pattern (8-4-4-4-12)
	parts := tenantID
	if len(parts) == 36 && parts[8] == '-' && parts[13] == '-' && parts[18] == '-' && parts[23] == '-' {
		return true
	}
	
	return false
}