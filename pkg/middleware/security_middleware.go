package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/metrics"
)

// SecurityHeadersMiddleware añade headers de seguridad estándar
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		
		// Enable XSS protection
		c.Header("X-XSS-Protection", "1; mode=block")
		
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		
		// Strict transport security (HTTPS only in production)
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		
		// Content security policy (basic)
		c.Header("Content-Security-Policy", "default-src 'self'")
		
		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// Permissions policy
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		
		c.Next()
	}
}

// RequestValidationMiddleware valida requests básicas de seguridad
func RequestValidationMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for suspicious request patterns
		userAgent := c.Request.UserAgent()
		path := c.Request.URL.Path
		
		// Block requests without User-Agent (simple bot detection)
		if userAgent == "" && !isInfrastructureEndpoint(path) {
			logger.WithFields(logrus.Fields{
				"client_ip": c.ClientIP(),
				"path":      path,
				"method":    c.Request.Method,
			}).Warn("Request blocked: missing User-Agent")
			
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "bad_request",
				"message": "Invalid request format",
				"code":    "INVALID_REQUEST",
			})
			c.Abort()
			return
		}
		
		// Check for excessively long headers
		for name, values := range c.Request.Header {
			for _, value := range values {
				if len(value) > 8192 { // 8KB limit
					logger.WithFields(logrus.Fields{
						"client_ip": c.ClientIP(),
						"path":      path,
						"header":    name,
						"length":    len(value),
					}).Warn("Request blocked: header too long")
					
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "bad_request",
						"message": "Request header too long",
						"code":    "HEADER_TOO_LONG",
					})
					c.Abort()
					return
				}
			}
		}
		
		// Check for suspicious path patterns
		if containsSuspiciousPathPatterns(path) {
			logger.WithFields(logrus.Fields{
				"client_ip": c.ClientIP(),
				"path":      path,
				"method":    c.Request.Method,
			}).Warn("Request blocked: suspicious path pattern")
			
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "Access denied",
				"code":    "SUSPICIOUS_PATH",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// SecurityMetricsMiddleware registra métricas de seguridad
func SecurityMetricsMiddleware(collector metrics.MetricsCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		
		// Record security-related metrics
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "unknown"
		}
		
		status := c.Writer.Status()
		path := c.Request.URL.Path
		
		// Record failed authentication attempts
		if status == 401 {
			collector.RecordCounter("security_auth_failures_total", map[string]string{
				"tenant_id": tenantID,
				"path":      path,
			}, 1)
		}
		
		// Record authorization failures
		if status == 403 {
			collector.RecordCounter("security_authz_failures_total", map[string]string{
				"tenant_id": tenantID,
				"path":      path,
			}, 1)
		}
		
		// Record suspicious requests (4xx except common ones)
		if status >= 400 && status < 500 && status != 404 && status != 401 && status != 403 {
			collector.RecordCounter("security_suspicious_requests_total", map[string]string{
				"tenant_id":   tenantID,
				"path":        path,
				"status_code": string(rune(status)),
			}, 1)
		}
	}
}

// Helper functions

func isInfrastructureEndpoint(path string) bool {
	infrastructurePaths := []string{
		"/health", "/metrics", "/ready", "/live", "/ping", "/version",
		"/debug", "/swagger", "/docs", "/favicon.ico",
	}
	
	for _, infraPath := range infrastructurePaths {
		if strings.HasPrefix(path, infraPath) {
			return true
		}
	}
	return false
}

func containsSuspiciousPathPatterns(path string) bool {
	suspiciousPatterns := []string{
		"../", "..", "etc/passwd", "proc/", "var/log",
		".env", ".git", ".ssh", "admin", "phpmyadmin",
		"wp-admin", "wp-login", ".php", ".asp", ".jsp",
		"eval(", "javascript:", "<script", "union select",
		"' or '1'='1", "base64", "cmd=", "exec=",
	}
	
	pathLower := strings.ToLower(path)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(pathLower, pattern) {
			return true
		}
	}
	
	return false
}