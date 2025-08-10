package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/errors"
)

// RateLimitStrategy define diferentes estrategias de rate limiting
type RateLimitStrategy string

const (
	// StrategyTokenBucket token bucket algorithm
	StrategyTokenBucket RateLimitStrategy = "token_bucket"
	// StrategyFixedWindow fixed window algorithm
	StrategyFixedWindow RateLimitStrategy = "fixed_window"
	// StrategySlidingWindow sliding window algorithm
	StrategySlidingWindow RateLimitStrategy = "sliding_window"
	// StrategyAdaptive adaptive rate limiting based on system load
	StrategyAdaptive RateLimitStrategy = "adaptive"
)

// TierConfig configuración por tier de usuario
type TierConfig struct {
	Name           string
	RequestsPerMin int
	BurstSize      int
	WindowSize     time.Duration
}

// IntelligentRateLimitConfig configuración del rate limiter inteligente
type IntelligentRateLimitConfig struct {
	// Strategy to use
	Strategy RateLimitStrategy
	
	// Default limits
	DefaultRequestsPerMin int
	DefaultBurstSize      int
	DefaultWindowSize     time.Duration
	
	// User tiers
	UserTiers map[string]*TierConfig
	
	// Key generators
	KeyGenerator        func(*gin.Context) string
	UserTierExtractor   func(*gin.Context) string
	
	// Adaptive settings
	AdaptiveConfig *AdaptiveConfig
	
	// Storage
	CacheClient cache.Client
	CachePrefix string
	
	// Behavior
	SkipSuccessfulRequests bool
	SkipPaths             []string
	Headers               *HeaderConfig
	
	// Callbacks
	OnLimitExceeded func(*gin.Context, *LimitInfo)
	OnLimitWarning  func(*gin.Context, *LimitInfo)
	
	// Monitoring
	MetricsEnabled bool
}

// AdaptiveConfig configuración para rate limiting adaptivo
type AdaptiveConfig struct {
	// CPU thresholds
	CPULowThreshold    float64 // < 50%
	CPUMediumThreshold float64 // < 80%
	CPUHighThreshold   float64 // >= 80%
	
	// Memory thresholds
	MemoryLowThreshold    float64 // < 60%
	MemoryMediumThreshold float64 // < 85%
	MemoryHighThreshold   float64 // >= 85%
	
	// Response time thresholds
	ResponseTimeLow    time.Duration // < 100ms
	ResponseTimeMedium time.Duration // < 500ms
	ResponseTimeHigh   time.Duration // >= 500ms
	
	// Adjustment factors
	LowLoadMultiplier    float64 // 1.5x normal limit
	MediumLoadMultiplier float64 // 1.0x normal limit
	HighLoadMultiplier   float64 // 0.5x normal limit
	
	// Evaluation interval
	EvaluationInterval time.Duration
}

// HeaderConfig configuración de headers de rate limit
type HeaderConfig struct {
	RateLimitHeader     string // X-RateLimit-Limit
	RemainingHeader     string // X-RateLimit-Remaining
	ResetHeader         string // X-RateLimit-Reset
	RetryAfterHeader    string // Retry-After
}

// LimitInfo información sobre el límite actual
type LimitInfo struct {
	Key           string
	Limit         int
	Remaining     int
	ResetTime     time.Time
	RetryAfter    time.Duration
	WindowSize    time.Duration
	Strategy      RateLimitStrategy
	UserTier      string
	IsExceeded    bool
	IsWarning     bool
}

// IntelligentRateLimiter implementa rate limiting inteligente
type IntelligentRateLimiter struct {
	config    *IntelligentRateLimitConfig
	tracer    trace.Tracer
	mutex     sync.RWMutex
	
	// Runtime metrics
	systemMetrics *SystemMetrics
	
	// Adaptive state
	currentMultiplier float64
	lastAdjustment    time.Time
}

// SystemMetrics métricas del sistema para adaptive rate limiting
type SystemMetrics struct {
	CPUUsage        float64
	MemoryUsage     float64
	AvgResponseTime time.Duration
	RequestRate     float64
	ErrorRate       float64
	
	LastUpdated     time.Time
	mutex           sync.RWMutex
}

// DefaultConfig retorna configuración por defecto
func DefaultConfig() *IntelligentRateLimitConfig {
	return &IntelligentRateLimitConfig{
		Strategy:          StrategyTokenBucket,
		DefaultRequestsPerMin: 100,
		DefaultBurstSize:     20,
		DefaultWindowSize:    time.Minute,
		CachePrefix:         "ratelimit:",
		
		UserTiers: map[string]*TierConfig{
			"free": {
				Name:           "free",
				RequestsPerMin: 60,
				BurstSize:      10,
				WindowSize:     time.Minute,
			},
			"premium": {
				Name:           "premium",
				RequestsPerMin: 300,
				BurstSize:      50,
				WindowSize:     time.Minute,
			},
			"enterprise": {
				Name:           "enterprise",
				RequestsPerMin: 1000,
				BurstSize:      100,
				WindowSize:     time.Minute,
			},
		},
		
		AdaptiveConfig: &AdaptiveConfig{
			CPULowThreshold:       50.0,
			CPUMediumThreshold:    80.0,
			CPUHighThreshold:      90.0,
			MemoryLowThreshold:    60.0,
			MemoryMediumThreshold: 85.0,
			MemoryHighThreshold:   95.0,
			ResponseTimeLow:       100 * time.Millisecond,
			ResponseTimeMedium:    500 * time.Millisecond,
			ResponseTimeHigh:      time.Second,
			LowLoadMultiplier:     1.5,
			MediumLoadMultiplier:  1.0,
			HighLoadMultiplier:    0.5,
			EvaluationInterval:    30 * time.Second,
		},
		
		Headers: &HeaderConfig{
			RateLimitHeader:  "X-RateLimit-Limit",
			RemainingHeader:  "X-RateLimit-Remaining",
			ResetHeader:      "X-RateLimit-Reset",
			RetryAfterHeader: "Retry-After",
		},
		
		SkipPaths: []string{"/health", "/metrics"},
		MetricsEnabled: true,
	}
}

// NewIntelligentRateLimiter crea un nuevo rate limiter inteligente
func NewIntelligentRateLimiter(config *IntelligentRateLimitConfig) *IntelligentRateLimiter {
	if config == nil {
		config = DefaultConfig()
	}
	
	// Set default key generator if not provided
	if config.KeyGenerator == nil {
		config.KeyGenerator = func(c *gin.Context) string {
			// Try to get user ID from context, fallback to IP
			if userID, exists := c.Get("user_id"); exists {
				if uid, ok := userID.(string); ok && uid != "" {
					return fmt.Sprintf("user:%s", uid)
				}
			}
			return fmt.Sprintf("ip:%s", c.ClientIP())
		}
	}
	
	// Set default user tier extractor
	if config.UserTierExtractor == nil {
		config.UserTierExtractor = func(c *gin.Context) string {
			if role, exists := c.Get("role"); exists {
				if r, ok := role.(string); ok {
					switch r {
					case "admin", "enterprise":
						return "enterprise"
					case "premium":
						return "premium"
					default:
						return "free"
					}
				}
			}
			return "free"
		}
	}
	
	limiter := &IntelligentRateLimiter{
		config:            config,
		tracer:           otel.Tracer("intelligent-rate-limiter"),
		systemMetrics:    &SystemMetrics{},
		currentMultiplier: 1.0,
		lastAdjustment:   time.Now(),
	}
	
	// Start adaptive adjustment routine if enabled
	if config.Strategy == StrategyAdaptive {
		go limiter.startAdaptiveAdjustment()
	}
	
	return limiter
}

// Middleware retorna middleware Gin
func (irl *IntelligentRateLimiter) Middleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Skip if path is excluded
		if irl.shouldSkipPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		
		ctx, span := irl.tracer.Start(c.Request.Context(), "rate_limiter.check")
		defer span.End()
		
		// Generate key and get user tier
		key := irl.config.KeyGenerator(c)
		userTier := irl.config.UserTierExtractor(c)
		
		span.SetAttributes(
			attribute.String("rate_limit.key", key),
			attribute.String("rate_limit.tier", userTier),
			attribute.String("rate_limit.strategy", string(irl.config.Strategy)),
		)
		
		// Check rate limit
		limitInfo, err := irl.checkRateLimit(ctx, key, userTier)
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limit check failed",
				"code":  "RATE_LIMIT_ERROR",
			})
			c.Abort()
			return
		}
		
		// Add rate limit headers
		irl.addHeaders(c, limitInfo)
		
		// Check if limit exceeded
		if limitInfo.IsExceeded {
			span.SetAttributes(
				attribute.Bool("rate_limit.exceeded", true),
				attribute.Int("rate_limit.remaining", limitInfo.Remaining),
			)
			
			if irl.config.OnLimitExceeded != nil {
				irl.config.OnLimitExceeded(c, limitInfo)
			}
			
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":      "Rate limit exceeded",
				"code":       "RATE_LIMIT_EXCEEDED",
				"limit":      limitInfo.Limit,
				"remaining":  limitInfo.Remaining,
				"reset_time": limitInfo.ResetTime,
				"retry_after": int(limitInfo.RetryAfter.Seconds()),
			})
			c.Abort()
			return
		}
		
		// Check for warning threshold (80% of limit)
		if limitInfo.IsWarning && irl.config.OnLimitWarning != nil {
			irl.config.OnLimitWarning(c, limitInfo)
		}
		
		span.SetAttributes(
			attribute.Bool("rate_limit.allowed", true),
			attribute.Int("rate_limit.remaining", limitInfo.Remaining),
		)
		
		c.Next()
		
		// Update system metrics for adaptive limiting
		if irl.config.Strategy == StrategyAdaptive {
			irl.updateSystemMetrics(c, time.Since(time.Now()))
		}
	})
}

// checkRateLimit verifica el rate limit según la estrategia configurada
func (irl *IntelligentRateLimiter) checkRateLimit(ctx context.Context, key, userTier string) (*LimitInfo, error) {
	// Get tier config
	tierConfig := irl.getTierConfig(userTier)
	
	// Apply adaptive multiplier if needed
	effectiveLimit := tierConfig.RequestsPerMin
	if irl.config.Strategy == StrategyAdaptive {
		effectiveLimit = int(float64(effectiveLimit) * irl.getCurrentMultiplier())
	}
	
	switch irl.config.Strategy {
	case StrategyTokenBucket, StrategyAdaptive:
		return irl.checkTokenBucket(ctx, key, effectiveLimit, tierConfig.BurstSize, tierConfig.WindowSize)
	case StrategyFixedWindow:
		return irl.checkFixedWindow(ctx, key, effectiveLimit, tierConfig.WindowSize)
	case StrategySlidingWindow:
		return irl.checkSlidingWindow(ctx, key, effectiveLimit, tierConfig.WindowSize)
	default:
		return irl.checkTokenBucket(ctx, key, effectiveLimit, tierConfig.BurstSize, tierConfig.WindowSize)
	}
}

// checkTokenBucket implementa token bucket algorithm
func (irl *IntelligentRateLimiter) checkTokenBucket(ctx context.Context, key string, limit, burstSize int, window time.Duration) (*LimitInfo, error) {
	cacheKey := fmt.Sprintf("%stb:%s", irl.config.CachePrefix, key)
	now := time.Now()
	
	// Get current bucket state
	type BucketState struct {
		Tokens      int       `json:"tokens"`
		LastRefill  time.Time `json:"last_refill"`
		WindowStart time.Time `json:"window_start"`
	}
	
	var bucket BucketState
	err := irl.config.CacheClient.GetJSON(ctx, cacheKey, &bucket)
	if err != nil {
		// Initialize new bucket
		bucket = BucketState{
			Tokens:      burstSize,
			LastRefill:  now,
			WindowStart: now,
		}
	}
	
	// Calculate tokens to add based on time passed
	timePassed := now.Sub(bucket.LastRefill)
	tokensToAdd := int(timePassed.Seconds() * float64(limit) / 60.0) // per minute rate
	
	bucket.Tokens = min(burstSize, bucket.Tokens+tokensToAdd)
	bucket.LastRefill = now
	
	// Check if we have tokens available
	hasTokens := bucket.Tokens > 0
	if hasTokens {
		bucket.Tokens--
	}
	
	// Update bucket state
	expiration := window + time.Minute // Extra buffer
	irl.config.CacheClient.SetJSON(ctx, cacheKey, bucket, expiration)
	
	// Calculate reset time
	var resetTime time.Time
	if bucket.Tokens == 0 {
		secondsToNext := 60.0 / float64(limit)
		resetTime = now.Add(time.Duration(secondsToNext) * time.Second)
	} else {
		resetTime = bucket.WindowStart.Add(window)
	}
	
	return &LimitInfo{
		Key:        key,
		Limit:      limit,
		Remaining:  bucket.Tokens,
		ResetTime:  resetTime,
		RetryAfter: time.Until(resetTime),
		WindowSize: window,
		Strategy:   irl.config.Strategy,
		IsExceeded: !hasTokens,
		IsWarning:  bucket.Tokens <= burstSize/5, // Warning at 20% remaining
	}, nil
}

// checkFixedWindow implementa fixed window algorithm
func (irl *IntelligentRateLimiter) checkFixedWindow(ctx context.Context, key string, limit int, window time.Duration) (*LimitInfo, error) {
	now := time.Now()
	windowStart := now.Truncate(window)
	cacheKey := fmt.Sprintf("%sfw:%s:%d", irl.config.CachePrefix, key, windowStart.Unix())
	
	// Get current count
	current, err := irl.config.CacheClient.Get(ctx, cacheKey)
	count := 0
	if err == nil {
		count, _ = strconv.Atoi(current)
	}
	
	// Check if limit exceeded
	exceeded := count >= limit
	if !exceeded {
		count++
		// Set with expiration to end of window
		expiration := window - time.Since(windowStart) + time.Second
		irl.config.CacheClient.Set(ctx, cacheKey, strconv.Itoa(count), expiration)
	}
	
	remaining := max(0, limit-count)
	resetTime := windowStart.Add(window)
	
	return &LimitInfo{
		Key:        key,
		Limit:      limit,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: time.Until(resetTime),
		WindowSize: window,
		Strategy:   StrategyFixedWindow,
		IsExceeded: exceeded,
		IsWarning:  remaining <= limit/5, // Warning at 20% remaining
	}, nil
}

// checkSlidingWindow implementa sliding window algorithm
func (irl *IntelligentRateLimiter) checkSlidingWindow(ctx context.Context, key string, limit int, window time.Duration) (*LimitInfo, error) {
	now := time.Now()
	windowStart := now.Add(-window)
	cacheKey := fmt.Sprintf("%ssw:%s", irl.config.CachePrefix, key)
	
	// For simplicity, using fixed window with smaller intervals
	// Real sliding window would require more complex logic
	interval := window / 10 // 10 sub-intervals
	var totalCount int
	
	for i := 0; i < 10; i++ {
		subWindowStart := windowStart.Add(time.Duration(i) * interval)
		subKey := fmt.Sprintf("%s:%d", cacheKey, subWindowStart.Unix()/int64(interval.Seconds()))
		
		current, err := irl.config.CacheClient.Get(ctx, subKey)
		if err == nil {
			count, _ := strconv.Atoi(current)
			totalCount += count
		}
	}
	
	// Current sub-window
	currentSubWindow := now.Truncate(interval)
	currentSubKey := fmt.Sprintf("%s:%d", cacheKey, currentSubWindow.Unix()/int64(interval.Seconds()))
	
	exceeded := totalCount >= limit
	if !exceeded {
		// Increment current sub-window
		current, _ := irl.config.CacheClient.Get(ctx, currentSubKey)
		count := 1
		if current != "" {
			existing, _ := strconv.Atoi(current)
			count = existing + 1
		}
		
		expiration := interval + time.Second
		irl.config.CacheClient.Set(ctx, currentSubKey, strconv.Itoa(count), expiration)
	}
	
	remaining := max(0, limit-totalCount-1)
	resetTime := now.Add(time.Duration(remaining) * window / time.Duration(limit))
	
	return &LimitInfo{
		Key:        key,
		Limit:      limit,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: time.Duration(window.Nanoseconds() / int64(limit)),
		WindowSize: window,
		Strategy:   StrategySlidingWindow,
		IsExceeded: exceeded,
		IsWarning:  remaining <= limit/5,
	}, nil
}

// getTierConfig obtiene configuración del tier
func (irl *IntelligentRateLimiter) getTierConfig(userTier string) *TierConfig {
	if config, exists := irl.config.UserTiers[userTier]; exists {
		return config
	}
	
	// Default config
	return &TierConfig{
		Name:           "default",
		RequestsPerMin: irl.config.DefaultRequestsPerMin,
		BurstSize:      irl.config.DefaultBurstSize,
		WindowSize:     irl.config.DefaultWindowSize,
	}
}

// getCurrentMultiplier obtiene el multiplicador adaptivo actual
func (irl *IntelligentRateLimiter) getCurrentMultiplier() float64 {
	irl.mutex.RLock()
	defer irl.mutex.RUnlock()
	return irl.currentMultiplier
}

// startAdaptiveAdjustment inicia el ajuste adaptivo
func (irl *IntelligentRateLimiter) startAdaptiveAdjustment() {
	ticker := time.NewTicker(irl.config.AdaptiveConfig.EvaluationInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		irl.adjustLimitsBasedOnLoad()
	}
}

// adjustLimitsBasedOnLoad ajusta los límites basado en la carga del sistema
func (irl *IntelligentRateLimiter) adjustLimitsBasedOnLoad() {
	irl.systemMetrics.mutex.RLock()
	cpu := irl.systemMetrics.CPUUsage
	memory := irl.systemMetrics.MemoryUsage
	responseTime := irl.systemMetrics.AvgResponseTime
	irl.systemMetrics.mutex.RUnlock()
	
	config := irl.config.AdaptiveConfig
	var newMultiplier float64
	
	// Determine load level based on multiple factors
	highLoad := cpu >= config.CPUHighThreshold ||
		memory >= config.MemoryHighThreshold ||
		responseTime >= config.ResponseTimeHigh
	
	mediumLoad := (cpu >= config.CPUMediumThreshold && cpu < config.CPUHighThreshold) ||
		(memory >= config.MemoryMediumThreshold && memory < config.MemoryHighThreshold) ||
		(responseTime >= config.ResponseTimeMedium && responseTime < config.ResponseTimeHigh)
	
	if highLoad {
		newMultiplier = config.HighLoadMultiplier
	} else if mediumLoad {
		newMultiplier = config.MediumLoadMultiplier
	} else {
		newMultiplier = config.LowLoadMultiplier
	}
	
	irl.mutex.Lock()
	irl.currentMultiplier = newMultiplier
	irl.lastAdjustment = time.Now()
	irl.mutex.Unlock()
}

// updateSystemMetrics actualiza métricas del sistema
func (irl *IntelligentRateLimiter) updateSystemMetrics(c *gin.Context, responseTime time.Duration) {
	irl.systemMetrics.mutex.Lock()
	defer irl.systemMetrics.mutex.Unlock()
	
	// Simple moving average for response time
	if irl.systemMetrics.AvgResponseTime == 0 {
		irl.systemMetrics.AvgResponseTime = responseTime
	} else {
		// Exponential moving average
		alpha := 0.1
		irl.systemMetrics.AvgResponseTime = time.Duration(
			float64(irl.systemMetrics.AvgResponseTime)*(1-alpha) +
				float64(responseTime)*alpha,
		)
	}
	
	irl.systemMetrics.LastUpdated = time.Now()
}

// addHeaders añade headers de rate limit
func (irl *IntelligentRateLimiter) addHeaders(c *gin.Context, info *LimitInfo) {
	headers := irl.config.Headers
	if headers == nil {
		return
	}
	
	if headers.RateLimitHeader != "" {
		c.Header(headers.RateLimitHeader, strconv.Itoa(info.Limit))
	}
	
	if headers.RemainingHeader != "" {
		c.Header(headers.RemainingHeader, strconv.Itoa(info.Remaining))
	}
	
	if headers.ResetHeader != "" {
		c.Header(headers.ResetHeader, strconv.FormatInt(info.ResetTime.Unix(), 10))
	}
	
	if info.IsExceeded && headers.RetryAfterHeader != "" {
		c.Header(headers.RetryAfterHeader, strconv.Itoa(int(info.RetryAfter.Seconds())))
	}
}

// shouldSkipPath verifica si una ruta debe omitir rate limiting
func (irl *IntelligentRateLimiter) shouldSkipPath(path string) bool {
	for _, skipPath := range irl.config.SkipPaths {
		if path == skipPath {
			return true
		}
	}
	return false
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}