package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// RateLimitConfig configuración del rate limiter
type RateLimitConfig struct {
	// Rate limiting settings
	RequestsPerSecond int
	RequestsPerMinute int
	RequestsPerHour   int
	BurstSize         int
	
	// Window settings
	WindowSize time.Duration
	
	// Key generation
	KeyGenerator KeyGeneratorFunc
	
	// Skip conditions
	SkipPaths     []string
	SkipIPs       []string
	WhitelistIPs  []string
	
	// Response settings
	IncludeHeaders bool
	CustomMessage  string
	
	// Storage
	UseRedis      bool
	RedisClient   *redis.Client
	RedisPrefix   string
	
	// Callbacks
	OnLimitExceeded func(c *gin.Context, limit *RateLimit)
	OnReset         func(c *gin.Context, limit *RateLimit)
}

// KeyGeneratorFunc función para generar la clave de rate limiting
type KeyGeneratorFunc func(c *gin.Context) string

// DefaultRateLimitConfig retorna configuración por defecto
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerSecond: 10,
		RequestsPerMinute: 100,
		RequestsPerHour:   1000,
		BurstSize:         20,
		WindowSize:        time.Minute,
		KeyGenerator:      DefaultKeyGenerator,
		SkipPaths:         []string{"/health", "/metrics", "/ping"},
		SkipIPs:           []string{},
		WhitelistIPs:      []string{"127.0.0.1", "::1"},
		IncludeHeaders:    true,
		CustomMessage:     "Rate limit exceeded",
		UseRedis:          false,
		RedisPrefix:       "ratelimit",
	}
}

// DefaultKeyGenerator generador de clave por defecto (por IP)
func DefaultKeyGenerator(c *gin.Context) string {
	return c.ClientIP()
}

// TenantKeyGenerator generador de clave por tenant
func TenantKeyGenerator(c *gin.Context) string {
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID == "" {
		return c.ClientIP()
	}
	return fmt.Sprintf("tenant:%s", tenantID)
}

// UserKeyGenerator generador de clave por usuario
func UserKeyGenerator(c *gin.Context) string {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		return c.ClientIP()
	}
	return fmt.Sprintf("user:%s", userID)
}

// PathKeyGenerator generador de clave por path + IP
func PathKeyGenerator(c *gin.Context) string {
	return fmt.Sprintf("%s:%s", c.ClientIP(), c.Request.URL.Path)
}

// RateLimit representa el estado del rate limiting
type RateLimit struct {
	Key              string
	Limit            int
	Remaining        int
	Reset            time.Time
	ResetDuration    time.Duration
	WindowStart      time.Time
	RequestCount     int
	LastRequestTime  time.Time
}

// RateLimiter interfaz para implementaciones de rate limiting
type RateLimiter interface {
	Allow(ctx context.Context, key string) (*RateLimit, error)
	Reset(ctx context.Context, key string) error
	GetStats(ctx context.Context, key string) (*RateLimit, error)
}

// MemoryRateLimiter implementación en memoria
type MemoryRateLimiter struct {
	config    *RateLimitConfig
	windows   map[string]*RateLimit
	mutex     sync.RWMutex
	logger    *logrus.Logger
	stopChan  chan struct{}
}

// NewMemoryRateLimiter crea un rate limiter en memoria
func NewMemoryRateLimiter(config *RateLimitConfig, logger *logrus.Logger) *MemoryRateLimiter {
	rl := &MemoryRateLimiter{
		config:   config,
		windows:  make(map[string]*RateLimit),
		logger:   logger.WithField("component", "memory_rate_limiter"),
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()
	
	return rl
}

// Allow verifica si se permite la request
func (mrl *MemoryRateLimiter) Allow(ctx context.Context, key string) (*RateLimit, error) {
	mrl.mutex.Lock()
	defer mrl.mutex.Unlock()

	now := time.Now()
	window, exists := mrl.windows[key]

	if !exists || now.Sub(window.WindowStart) >= mrl.config.WindowSize {
		// Create new window
		window = &RateLimit{
			Key:             key,
			Limit:           mrl.config.RequestsPerMinute,
			Remaining:       mrl.config.RequestsPerMinute - 1,
			Reset:           now.Add(mrl.config.WindowSize),
			ResetDuration:   mrl.config.WindowSize,
			WindowStart:     now,
			RequestCount:    1,
			LastRequestTime: now,
		}
		mrl.windows[key] = window
		return window, nil
	}

	// Check if limit exceeded
	if window.RequestCount >= window.Limit {
		return window, nil
	}

	// Allow request
	window.RequestCount++
	window.Remaining = window.Limit - window.RequestCount
	window.LastRequestTime = now

	return window, nil
}

// Reset resetea el rate limit para una clave
func (mrl *MemoryRateLimiter) Reset(ctx context.Context, key string) error {
	mrl.mutex.Lock()
	defer mrl.mutex.Unlock()
	
	delete(mrl.windows, key)
	return nil
}

// GetStats obtiene estadísticas para una clave
func (mrl *MemoryRateLimiter) GetStats(ctx context.Context, key string) (*RateLimit, error) {
	mrl.mutex.RLock()
	defer mrl.mutex.RUnlock()
	
	if window, exists := mrl.windows[key]; exists {
		// Return a copy
		stats := *window
		return &stats, nil
	}
	
	return nil, fmt.Errorf("key not found")
}

// cleanup limpia ventanas expiradas
func (mrl *MemoryRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mrl.mutex.Lock()
			now := time.Now()
			for key, window := range mrl.windows {
				if now.After(window.Reset) {
					delete(mrl.windows, key)
				}
			}
			mrl.mutex.Unlock()
		case <-mrl.stopChan:
			return
		}
	}
}

// Close cierra el rate limiter
func (mrl *MemoryRateLimiter) Close() {
	close(mrl.stopChan)
}

// RedisRateLimiter implementación con Redis
type RedisRateLimiter struct {
	config *RateLimitConfig
	client *redis.Client
	logger *logrus.Logger
}

// NewRedisRateLimiter crea un rate limiter con Redis
func NewRedisRateLimiter(config *RateLimitConfig, logger *logrus.Logger) *RedisRateLimiter {
	return &RedisRateLimiter{
		config: config,
		client: config.RedisClient,
		logger: logger.WithField("component", "redis_rate_limiter"),
	}
}

// Allow verifica si se permite la request usando Redis
func (rrl *RedisRateLimiter) Allow(ctx context.Context, key string) (*RateLimit, error) {
	redisKey := fmt.Sprintf("%s:%s", rrl.config.RedisPrefix, key)
	now := time.Now()
	windowStart := now.Truncate(rrl.config.WindowSize)
	
	// Use Redis pipeline for atomic operations
	pipe := rrl.client.Pipeline()
	
	// Increment counter
	incrCmd := pipe.Incr(ctx, redisKey)
	
	// Set expiration if key is new
	pipe.Expire(ctx, redisKey, rrl.config.WindowSize)
	
	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("Redis pipeline failed: %w", err)
	}
	
	count := incrCmd.Val()
	remaining := rrl.config.RequestsPerMinute - int(count)
	if remaining < 0 {
		remaining = 0
	}
	
	return &RateLimit{
		Key:             key,
		Limit:           rrl.config.RequestsPerMinute,
		Remaining:       remaining,
		Reset:           windowStart.Add(rrl.config.WindowSize),
		ResetDuration:   rrl.config.WindowSize,
		WindowStart:     windowStart,
		RequestCount:    int(count),
		LastRequestTime: now,
	}, nil
}

// Reset resetea el rate limit en Redis
func (rrl *RedisRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("%s:%s", rrl.config.RedisPrefix, key)
	return rrl.client.Del(ctx, redisKey).Err()
}

// GetStats obtiene estadísticas desde Redis
func (rrl *RedisRateLimiter) GetStats(ctx context.Context, key string) (*RateLimit, error) {
	redisKey := fmt.Sprintf("%s:%s", rrl.config.RedisPrefix, key)
	
	// Get current count and TTL
	pipe := rrl.client.Pipeline()
	getCmd := pipe.Get(ctx, redisKey)
	ttlCmd := pipe.TTL(ctx, redisKey)
	
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("Redis pipeline failed: %w", err)
	}
	
	var count int64
	if err != redis.Nil {
		count, _ = strconv.ParseInt(getCmd.Val(), 10, 64)
	}
	
	ttl := ttlCmd.Val()
	now := time.Now()
	
	remaining := rrl.config.RequestsPerMinute - int(count)
	if remaining < 0 {
		remaining = 0
	}
	
	return &RateLimit{
		Key:             key,
		Limit:           rrl.config.RequestsPerMinute,
		Remaining:       remaining,
		Reset:           now.Add(ttl),
		ResetDuration:   ttl,
		WindowStart:     now.Add(-rrl.config.WindowSize + ttl),
		RequestCount:    int(count),
		LastRequestTime: now,
	}, nil
}

// RateLimitMiddleware middleware de rate limiting
func RateLimitMiddleware(rateLimiter RateLimiter, config *RateLimitConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	return func(c *gin.Context) {
		// Skip rate limiting for certain paths
		if shouldSkipPath(c.Request.URL.Path, config.SkipPaths) {
			c.Next()
			return
		}

		// Skip rate limiting for whitelisted IPs
		if isWhitelistedIP(c.ClientIP(), config.WhitelistIPs) {
			c.Next()
			return
		}

		// Generate key
		key := config.KeyGenerator(c)
		
		// Check rate limit
		limit, err := rateLimiter.Allow(c.Request.Context(), key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "rate_limit_error",
				"message": "Rate limiting service error",
				"code":    "RATE_LIMIT_ERROR",
			})
			c.Abort()
			return
		}

		// Add rate limit headers if enabled
		if config.IncludeHeaders {
			c.Header("X-RateLimit-Limit", strconv.Itoa(limit.Limit))
			c.Header("X-RateLimit-Remaining", strconv.Itoa(limit.Remaining))
			c.Header("X-RateLimit-Reset", strconv.FormatInt(limit.Reset.Unix(), 10))
		}

		// Check if limit exceeded
		if limit.Remaining <= 0 {
			// Call callback if configured
			if config.OnLimitExceeded != nil {
				config.OnLimitExceeded(c, limit)
			}

			// Add retry-after header
			retryAfter := int(time.Until(limit.Reset).Seconds())
			c.Header("Retry-After", strconv.Itoa(retryAfter))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded", 
				"message":     config.CustomMessage,
				"code":        "RATE_LIMIT_EXCEEDED",
				"retry_after": retryAfter,
				"limit":       limit.Limit,
				"remaining":   limit.Remaining,
				"reset":       limit.Reset.Unix(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// NewRateLimiter crea un rate limiter basado en la configuración
func NewRateLimiter(config *RateLimitConfig, logger *logrus.Logger) RateLimiter {
	if config.UseRedis && config.RedisClient != nil {
		return NewRedisRateLimiter(config, logger)
	}
	return NewMemoryRateLimiter(config, logger)
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

func isWhitelistedIP(ip string, whitelist []string) bool {
	for _, whitelistedIP := range whitelist {
		if ip == whitelistedIP {
			return true
		}
	}
	return false
}

// PerPathRateLimitMiddleware rate limiting por path específico
func PerPathRateLimitMiddleware(pathLimits map[string]*RateLimitConfig, logger *logrus.Logger) gin.HandlerFunc {
	rateLimiters := make(map[string]RateLimiter)
	
	// Create rate limiters for each path
	for path, config := range pathLimits {
		rateLimiters[path] = NewRateLimiter(config, logger)
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		
		// Check if we have a specific rate limiter for this path
		if config, exists := pathLimits[path]; exists {
			rateLimiter := rateLimiters[path]
			RateLimitMiddleware(rateLimiter, config)(c)
		} else {
			c.Next()
		}
	}
}

// AdaptiveRateLimitMiddleware rate limiting adaptativo basado en load
func AdaptiveRateLimitMiddleware(baseConfig *RateLimitConfig, logger *logrus.Logger) gin.HandlerFunc {
	rateLimiter := NewRateLimiter(baseConfig, logger)
	
	return func(c *gin.Context) {
		// This could be enhanced to adjust limits based on system load,
		// error rates, response times, etc.
		
		RateLimitMiddleware(rateLimiter, baseConfig)(c)
	}
}

// RateLimitStatsHandler handler para obtener estadísticas de rate limiting
func RateLimitStatsHandler(rateLimiter RateLimiter, config *RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := config.KeyGenerator(c)
		
		stats, err := rateLimiter.GetStats(c.Request.Context(), key)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "stats_not_found",
				"message": "Rate limit stats not found for key",
				"key":     key,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"key":              stats.Key,
			"limit":            stats.Limit,
			"remaining":        stats.Remaining,
			"reset":            stats.Reset.Unix(),
			"window_start":     stats.WindowStart.Unix(),
			"request_count":    stats.RequestCount,
			"last_request":     stats.LastRequestTime.Unix(),
		})
	}
}