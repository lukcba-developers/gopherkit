package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// CommonMiddleware provides common middleware functionality
type CommonMiddleware struct {
	logger logger.Logger
}

// NewCommonMiddleware creates a new common middleware instance
func NewCommonMiddleware(logger logger.Logger) *CommonMiddleware {
	return &CommonMiddleware{
		logger: logger,
	}
}

// Recovery middleware handles panics and converts them to errors
func (m *CommonMiddleware) Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		ctx := c.Request.Context()
		
		m.logger.LogError(ctx, 
			&PanicError{Message: recovered}, 
			"panic recovered", 
			map[string]interface{}{
				"method":     c.Request.Method,
				"path":       c.Request.URL.Path,
				"client_ip":  c.ClientIP(),
				"user_agent": c.Request.UserAgent(),
			})

		c.JSON(500, gin.H{
			"error":   "Internal Server Error",
			"message": "An unexpected error occurred",
			"code":    "INTERNAL_ERROR",
		})
	})
}

// RequestID middleware adds a unique request ID to each request
func (m *CommonMiddleware) RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get request ID from header first
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			// Generate new UUID if not provided
			requestID = uuid.New().String()
		}

		// Set in context and header
		ctx := context.WithValue(c.Request.Context(), "request_id", requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", requestID)

		c.Next()
	}
}

// Correlation middleware handles correlation ID for distributed tracing
func (m *CommonMiddleware) Correlation() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get correlation ID from various headers
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			correlationID = c.GetHeader("X-Correlation-Id")
		}
		if correlationID == "" {
			correlationID = c.GetHeader("Correlation-ID")
		}
		
		// Generate new one if not provided
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Set in context and response header
		ctx := context.WithValue(c.Request.Context(), "correlation_id", correlationID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Correlation-ID", correlationID)

		c.Next()
	}
}

// TenantID middleware handles multi-tenant context
func (m *CommonMiddleware) TenantID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get tenant ID from header
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.GetHeader("Tenant-ID")
		}

		// Can also extract from subdomain or path
		if tenantID == "" {
			tenantID = m.extractTenantFromHost(c.Request.Host)
		}

		// Set in context if found
		if tenantID != "" {
			ctx := context.WithValue(c.Request.Context(), "tenant_id", tenantID)
			c.Request = c.Request.WithContext(ctx)
			c.Header("X-Tenant-ID", tenantID)
		}

		c.Next()
	}
}

// UserID middleware extracts user ID from JWT or headers
func (m *CommonMiddleware) UserID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get user ID from header (set by auth middleware)
		userID := c.GetHeader("X-User-ID")
		
		// Try to get from context if set by JWT middleware
		if userID == "" {
			if uid, exists := c.Get("user_id"); exists {
				if uidStr, ok := uid.(string); ok {
					userID = uidStr
				}
			}
		}

		// Set in context if found
		if userID != "" {
			ctx := context.WithValue(c.Request.Context(), "user_id", userID)
			c.Request = c.Request.WithContext(ctx)
		}

		c.Next()
	}
}

// Timeout middleware adds timeout to requests
func (m *CommonMiddleware) Timeout(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		// Channel to signal when the request is done
		done := make(chan struct{})
		
		go func() {
			defer close(done)
			c.Next()
		}()

		select {
		case <-done:
			// Request completed normally
			return
		case <-ctx.Done():
			// Request timed out
			m.logger.LogError(ctx, ctx.Err(), "request timeout", map[string]interface{}{
				"method":   c.Request.Method,
				"path":     c.Request.URL.Path,
				"timeout":  duration.String(),
			})
			
			c.JSON(408, gin.H{
				"error":   "Request Timeout",
				"message": "Request took too long to process",
				"code":    "REQUEST_TIMEOUT",
			})
			c.Abort()
			return
		}
	}
}

// NoCache middleware sets headers to prevent caching
func (m *CommonMiddleware) NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

// GZIP middleware enables gzip compression
func (m *CommonMiddleware) GZIP() gin.HandlerFunc {
	// Return a simple no-op middleware for now
	// In production, you would use github.com/gin-contrib/gzip
	return gin.HandlerFunc(func(c *gin.Context) {
		c.Next()
	})
}

// Helper functions
func (m *CommonMiddleware) extractTenantFromHost(host string) string {
	// Extract tenant from subdomain
	// Example: tenant1.api.example.com -> tenant1
	parts := strings.Split(host, ".")
	if len(parts) >= 3 {
		subdomain := parts[0]
		// Validate subdomain is not "www" or "api"
		if subdomain != "www" && subdomain != "api" && subdomain != "localhost" {
			return subdomain
		}
	}
	return ""
}

// PanicError wraps panic recovery
type PanicError struct {
	Message interface{}
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Message)
}