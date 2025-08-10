package security

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// SecurityMiddleware provides security-related middleware
type SecurityMiddleware struct {
	logger      logger.Logger
	rateLimiters map[string]*rate.Limiter
}

// Config holds security middleware configuration
type Config struct {
	RateLimit      config.RateLimitConfig
	CircuitBreaker config.CircuitBreakerConfig
	CORS           config.CORSConfig
}

// NewSecurityMiddleware creates a new security middleware instance
func NewSecurityMiddleware(config Config, logger logger.Logger) (*SecurityMiddleware, error) {
	return &SecurityMiddleware{
		logger:       logger,
		rateLimiters: make(map[string]*rate.Limiter),
	}, nil
}

// CORS middleware handles Cross-Origin Resource Sharing
func (s *SecurityMiddleware) CORS(corsConfig config.CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range corsConfig.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", strings.Join(corsConfig.AllowedMethods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(corsConfig.AllowedHeaders, ", "))
		c.Header("Access-Control-Max-Age", strconv.Itoa(corsConfig.MaxAge))
		c.Header("Access-Control-Allow-Credentials", strconv.FormatBool(corsConfig.AllowCredentials))
		
		if len(corsConfig.ExposedHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(corsConfig.ExposedHeaders, ", "))
		}

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// SecurityHeaders middleware adds security headers
func (s *SecurityMiddleware) SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Enable XSS protection
		c.Header("X-XSS-Protection", "1; mode=block")

		// Strict transport security (HTTPS only)
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy
		c.Header("Content-Security-Policy", "default-src 'self'")

		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Feature policy
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Remove server information
		c.Header("Server", "")

		c.Next()
	}
}

// RateLimit middleware implements rate limiting per client IP
func (s *SecurityMiddleware) RateLimit(config config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !config.Enabled {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		
		// Get or create rate limiter for this IP
		limiter, exists := s.rateLimiters[clientIP]
		if !exists {
			// Create new limiter: requests per minute converted to requests per second
			rps := float64(config.RequestsPerMinute) / 60.0
			limiter = rate.NewLimiter(rate.Limit(rps), config.BurstSize)
			s.rateLimiters[clientIP] = limiter
		}

		// Check if request is allowed
		if !limiter.Allow() {
			s.logger.LogSecurityEvent(c.Request.Context(), "rate_limit_exceeded", map[string]interface{}{
				"client_ip": clientIP,
				"endpoint":  c.FullPath(),
				"method":    c.Request.Method,
			})

			c.Header("X-RateLimit-Limit", strconv.Itoa(config.RequestsPerMinute))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"message": "Too many requests. Please try again later.",
				"code":    "RATE_LIMIT_EXCEEDED",
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(config.RequestsPerMinute))
		remaining := config.BurstSize - 1
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		c.Next()
	}
}

// RequestSizeLimit middleware limits the size of request bodies
func (s *SecurityMiddleware) RequestSizeLimit(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxSize {
			s.logger.LogSecurityEvent(c.Request.Context(), "request_size_limit_exceeded", map[string]interface{}{
				"content_length": c.Request.ContentLength,
				"max_size":       maxSize,
				"client_ip":      c.ClientIP(),
				"endpoint":       c.FullPath(),
			})

			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "Request too large",
				"message": "Request body size exceeds the maximum allowed limit",
				"code":    "REQUEST_TOO_LARGE",
			})
			c.Abort()
			return
		}

		// Set max bytes reader
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}

// InputValidation middleware validates input for common security threats
func (s *SecurityMiddleware) InputValidation() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Check for common injection patterns in query parameters
		for param, values := range c.Request.URL.Query() {
			for _, value := range values {
				if containsSuspiciousPatterns(value) {
					s.logger.LogSecurityEvent(ctx, "suspicious_input_detected", map[string]interface{}{
						"parameter":  param,
						"value":      maskSensitiveValue(value),
						"client_ip":  c.ClientIP(),
						"endpoint":   c.FullPath(),
						"user_agent": c.Request.UserAgent(),
					})

					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid input",
						"message": "Request contains potentially harmful content",
						"code":    "INVALID_INPUT",
					})
					c.Abort()
					return
				}
			}
		}

		// Check User-Agent for suspicious patterns
		userAgent := c.Request.UserAgent()
		if userAgent == "" || containsSuspiciousUserAgent(userAgent) {
			s.logger.LogSecurityEvent(ctx, "suspicious_user_agent", map[string]interface{}{
				"user_agent": userAgent,
				"client_ip":  c.ClientIP(),
				"endpoint":   c.FullPath(),
			})
		}

		c.Next()
	}
}

// IPWhitelist middleware allows only whitelisted IPs
func (s *SecurityMiddleware) IPWhitelist(allowedIPs []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		
		allowed := false
		for _, allowedIP := range allowedIPs {
			if allowedIP == clientIP {
				allowed = true
				break
			}
		}

		if !allowed {
			s.logger.LogSecurityEvent(c.Request.Context(), "ip_not_whitelisted", map[string]interface{}{
				"client_ip": clientIP,
				"endpoint":  c.FullPath(),
			})

			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Access denied",
				"code":    "IP_NOT_ALLOWED",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// CSRFProtection middleware provides CSRF protection
func (s *SecurityMiddleware) CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For API-only services, we typically skip CSRF protection
		// since APIs are usually stateless and use token-based auth
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		// Check for CSRF token in header
		token := c.GetHeader("X-CSRF-Token")
		if token == "" {
			token = c.GetHeader("X-XSRF-Token")
		}

		// For API services, we can be more lenient with CSRF
		// This is a placeholder for more sophisticated CSRF protection
		c.Next()
	}
}

// Helper functions
func containsSuspiciousPatterns(input string) bool {
	suspicious := []string{
		// XSS patterns
		"<script", "</script>", "javascript:", "vbscript:",
		"onload=", "onerror=", "onclick=", "onmouseover=",
		
		// SQL injection patterns
		"SELECT * FROM", "DROP TABLE", "INSERT INTO", "DELETE FROM",
		"UNION SELECT", "' OR '1'='1", "' OR 1=1", "'; DROP TABLE",
		"' UNION", "/*", "*/", "--",
		
		// Path traversal
		"../", "..\\", "/etc/passwd", "/etc/shadow",
		
		// Command injection
		"cmd.exe", "powershell", "/bin/bash", "/bin/sh",
		"&amp;&amp;", "||", ";", "|",
		
		// LDAP injection
		"*)(uid=*", "*)(cn=*",
	}

	inputLower := strings.ToLower(input)
	for _, pattern := range suspicious {
		if strings.Contains(inputLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

func containsSuspiciousUserAgent(userAgent string) bool {
	suspicious := []string{
		// Security scanners
		"sqlmap", "nikto", "nmap", "masscan", "nessus",
		"burpsuite", "dirbuster", "gobuster", "ffuf",
		"zaproxy", "w3af", "skipfish",
		
		// Common automated tools
		"curl", "wget", "python-requests", "python-urllib",
		"postman", "insomnia",
		
		// Suspicious patterns
		"bot", "crawler", "spider", "scraper",
	}

	userAgentLower := strings.ToLower(userAgent)
	for _, pattern := range suspicious {
		if strings.Contains(userAgentLower, pattern) {
			return true
		}
	}

	return false
}

func maskSensitiveValue(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}