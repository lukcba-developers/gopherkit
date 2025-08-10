package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/circuitbreaker"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/metrics"
	"github.com/sirupsen/logrus"
)

// TenantContextMiddleware middleware que extrae información del tenant
func TenantContextMiddleware(logger *logrus.Logger, required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract tenant ID from header, query param, or JWT claims
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.Query("tenant_id")
		}
		if tenantID == "" {
			// Try to get from JWT claims if available
			if claims, exists := c.Get("tenant_id"); exists {
				if tid, ok := claims.(string); ok {
					tenantID = tid
				}
			}
		}

		if tenantID == "" && required {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Tenant ID is required",
				"code":    "TENANT_REQUIRED",
				"message": "X-Tenant-ID header or tenant_id query parameter must be provided",
			})
			c.Abort()
			return
		}

		// Set tenant ID in context
		if tenantID != "" {
			c.Set("tenant_id", tenantID)
			logger.WithField("tenant_id", tenantID).Debug("Tenant context set")
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware añade headers de seguridad
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Security headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")

		c.Next()
	}
}

// RequestValidationMiddleware valida requests básicos
func RequestValidationMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Basic request validation
		if c.Request.ContentLength > 10*1024*1024 { // 10MB limit
			logger.WithFields(logrus.Fields{
				"content_length": c.Request.ContentLength,
				"path":           c.Request.URL.Path,
				"method":         c.Request.Method,
			}).Warn("Request too large")

			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "Request entity too large",
				"code":    "REQUEST_TOO_LARGE",
				"message": "Request body must be less than 10MB",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// SecurityMetricsMiddleware registra métricas de seguridad
func SecurityMetricsMiddleware(metricsMiddleware *metrics.MetricsMiddleware) gin.HandlerFunc {
	return func(c *gin.Context) {
		// This would record security-related metrics
		// For now, just pass through
		c.Next()
	}
}

// BasicHealthHandler handler básico de health check
func BasicHealthHandler(serviceName, serviceVersion string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": serviceName,
			"version": serviceVersion,
		})
	}
}

// DetailedHealthHandler handler detallado de health check
func DetailedHealthHandler(stack *MiddlewareStack) gin.HandlerFunc {
	return func(c *gin.Context) {
		health := gin.H{
			"status":  "healthy",
			"service": stack.ServiceName,
			"version": stack.ServiceVersion,
			"environment": stack.Environment,
			"middlewares": map[string]bool{
				"cors":            stack.EnableCORS,
				"logging":         stack.EnableLogging,
				"metrics":         stack.EnableMetrics,
				"rate_limit":      stack.EnableRateLimit,
				"circuit_breaker": stack.EnableCircuitBreaker,
				"recovery":        stack.EnableRecovery,
				"tenant_context":  stack.EnableTenantContext,
			},
		}

		// Add Redis health if available
		if stack.RedisClient != nil {
			if err := stack.RedisClient.Ping(c.Request.Context()).Err(); err != nil {
				health["redis"] = "unhealthy"
				health["redis_error"] = err.Error()
			} else {
				health["redis"] = "healthy"
			}
		}

		c.JSON(http.StatusOK, health)
	}
}

// ReadinessHandler handler de readiness check
func ReadinessHandler(stack *MiddlewareStack) gin.HandlerFunc {
	return func(c *gin.Context) {
		ready := true
		checks := make(map[string]string)

		// Check Redis if enabled
		if stack.RedisClient != nil {
			if err := stack.RedisClient.Ping(c.Request.Context()).Err(); err != nil {
				ready = false
				checks["redis"] = "unhealthy: " + err.Error()
			} else {
				checks["redis"] = "healthy"
			}
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}

		c.JSON(status, gin.H{
			"ready":  ready,
			"checks": checks,
		})
	}
}

// LivenessHandler handler de liveness check
func LivenessHandler(stack *MiddlewareStack) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"alive":   true,
			"service": stack.ServiceName,
			"version": stack.ServiceVersion,
		})
	}
}

// CircuitBreakerHealthHandler handler para estado del circuit breaker
func CircuitBreakerHealthHandler(cbm *circuitbreaker.CircuitBreakerManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// For now, just return basic info
		// The actual CircuitBreakerManager might need methods to get health info
		c.JSON(http.StatusOK, gin.H{
			"circuit_breakers": "active",
			"status":          "monitoring",
		})
	}
}