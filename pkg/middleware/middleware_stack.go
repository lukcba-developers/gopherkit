package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	"github.com/lukcba-developers/gopherkit/pkg/middleware/circuitbreaker"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/cors"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/logging"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/metrics"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/ratelimit"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/recovery"
)

// MiddlewareStack configuración completa del stack de middlewares
type MiddlewareStack struct {
	// Core components
	Logger          *logrus.Logger
	RedisClient     *redis.Client
	
	// Configurations
	CORSConfig           *cors.Config
	LoggingConfig        *logging.LoggingConfig
	MetricsConfig        *metrics.MetricsConfig
	RateLimitConfig      *ratelimit.RateLimitConfig
	CircuitBreakerConfig *circuitbreaker.CircuitBreakerConfig
	RecoveryConfig       *recovery.RecoveryConfig
	
	// Components
	MetricsCollector     metrics.MetricsCollector
	CircuitBreakerMgr    *circuitbreaker.CircuitBreakerManager
	RecoveryStatsCollector *recovery.RecoveryStatsCollector
	
	// Settings
	EnableCORS           bool
	EnableLogging        bool
	EnableMetrics        bool
	EnableRateLimit      bool
	EnableCircuitBreaker bool
	EnableRecovery       bool
	EnableTenantContext  bool
	
	// Environment
	Environment          string // "development", "staging", "production"
	ServiceName          string
	ServiceVersion       string
}

// DefaultMiddlewareStack retorna configuración por defecto
func DefaultMiddlewareStack(logger *logrus.Logger, serviceName string) *MiddlewareStack {
	return &MiddlewareStack{
		Logger:                 logger,
		CORSConfig:            cors.DefaultConfig(),
		LoggingConfig:         logging.DefaultLoggingConfig(),
		MetricsConfig:         metrics.DefaultMetricsConfig(),
		RateLimitConfig:       ratelimit.DefaultRateLimitConfig(),
		CircuitBreakerConfig:  circuitbreaker.DefaultCircuitBreakerConfig(),
		RecoveryConfig:        recovery.DefaultRecoveryConfig(),
		EnableCORS:            true,
		EnableLogging:         true,
		EnableMetrics:         true,
		EnableRateLimit:       true,
		EnableCircuitBreaker:  true,
		EnableRecovery:        true,
		EnableTenantContext:   true,
		Environment:           "development",
		ServiceName:           serviceName,
		ServiceVersion:        "1.0.0",
	}
}

// DevelopmentMiddlewareStack configuración para desarrollo
func DevelopmentMiddlewareStack(logger *logrus.Logger, serviceName string) *MiddlewareStack {
	stack := DefaultMiddlewareStack(logger, serviceName)
	
	// Development-specific settings
	stack.Environment = "development"
	stack.LoggingConfig.LogBody = true
	stack.LoggingConfig.LogHeaders = true
	stack.RecoveryConfig.EnableDetailedError = true
	stack.RecoveryConfig.EnableStackTrace = true
	stack.CORSConfig.AllowAllOrigins = true
	stack.RateLimitConfig.RequestsPerMinute = 1000 // More permissive for dev
	
	return stack
}

// ProductionMiddlewareStack configuración para producción
func ProductionMiddlewareStack(logger *logrus.Logger, serviceName string) *MiddlewareStack {
	stack := DefaultMiddlewareStack(logger, serviceName)
	
	// Production-specific settings
	stack.Environment = "production"
	stack.LoggingConfig.LogBody = false
	stack.LoggingConfig.LogHeaders = false
	stack.RecoveryConfig.EnableDetailedError = false
	stack.RecoveryConfig.EnableStackTrace = false
	stack.CORSConfig.AllowAllOrigins = false
	stack.RateLimitConfig.RequestsPerMinute = 100 // More restrictive for prod
	
	return stack
}

// Initialize inicializa todos los componentes del stack
func (ms *MiddlewareStack) Initialize() error {
	// Initialize metrics collector
	if ms.EnableMetrics {
		ms.MetricsCollector = metrics.NewInMemoryMetricsCollector(ms.MetricsConfig, ms.Logger)
	}
	
	// Initialize circuit breaker manager
	if ms.EnableCircuitBreaker {
		ms.CircuitBreakerMgr = circuitbreaker.NewCircuitBreakerManager(ms.Logger)
		// Add default circuit breaker
		ms.CircuitBreakerMgr.AddCircuitBreaker("default", ms.CircuitBreakerConfig)
	}
	
	// Initialize recovery stats collector
	if ms.EnableRecovery {
		ms.RecoveryStatsCollector = recovery.NewRecoveryStatsCollector()
	}
	
	// Configure rate limiting with Redis if available
	if ms.EnableRateLimit && ms.RedisClient != nil {
		ms.RateLimitConfig.UseRedis = true
		ms.RateLimitConfig.RedisClient = ms.RedisClient
	}
	
	ms.Logger.WithFields(logrus.Fields{
		"service":     ms.ServiceName,
		"version":     ms.ServiceVersion,
		"environment": ms.Environment,
		"cors":        ms.EnableCORS,
		"logging":     ms.EnableLogging,
		"metrics":     ms.EnableMetrics,
		"rate_limit":  ms.EnableRateLimit,
		"circuit_breaker": ms.EnableCircuitBreaker,
		"recovery":    ms.EnableRecovery,
	}).Info("Middleware stack initialized")
	
	return nil
}

// ApplyMiddlewares aplica todos los middlewares al router
func (ms *MiddlewareStack) ApplyMiddlewares(router *gin.Engine) {
	// 1. Recovery middleware (should be first to catch panics)
	if ms.EnableRecovery {
		if ms.RecoveryStatsCollector != nil {
			router.Use(recovery.RecoveryWithStatsMiddleware(ms.Logger, ms.RecoveryStatsCollector, ms.RecoveryConfig))
		} else {
			router.Use(recovery.RecoveryMiddleware(ms.Logger, ms.RecoveryConfig))
		}
	}
	
	// 2. CORS middleware
	if ms.EnableCORS {
		router.Use(cors.Middleware(ms.CORSConfig))
	}
	
	// 3. Logging middleware (structured logging with correlation IDs)
	if ms.EnableLogging {
		router.Use(logging.LoggingMiddleware(ms.Logger, ms.LoggingConfig))
		router.Use(logging.StructuredLoggerMiddleware(ms.Logger))
	}
	
	// 4. Tenant context middleware (if enabled)
	if ms.EnableTenantContext {
		router.Use(TenantContextMiddleware(ms.Logger, false)) // Optional tenant by default
	}
	
	// 5. Metrics middleware
	if ms.EnableMetrics && ms.MetricsCollector != nil {
		router.Use(metrics.MetricsMiddleware(ms.MetricsCollector, ms.MetricsConfig))
		router.Use(metrics.BusinessMetricsMiddleware(ms.MetricsCollector, ms.MetricsConfig))
	}
	
	// 6. Rate limiting middleware
	if ms.EnableRateLimit {
		rateLimiter := ratelimit.NewRateLimiter(ms.RateLimitConfig, ms.Logger)
		router.Use(ratelimit.RateLimitMiddleware(rateLimiter, ms.RateLimitConfig))
	}
	
	// 7. Circuit breaker middleware
	if ms.EnableCircuitBreaker && ms.CircuitBreakerMgr != nil {
		router.Use(circuitbreaker.MultiCircuitBreakerMiddleware(ms.CircuitBreakerMgr, "default"))
	}
}

// ApplySecurityMiddlewares aplica middlewares de seguridad adicionales
func (ms *MiddlewareStack) ApplySecurityMiddlewares(router *gin.Engine) {
	// Security headers
	router.Use(SecurityHeadersMiddleware())
	
	// Request validation middleware
	router.Use(RequestValidationMiddleware(ms.Logger))
	
	// Security monitoring
	if ms.EnableMetrics && ms.MetricsCollector != nil {
		router.Use(SecurityMetricsMiddleware(ms.MetricsCollector))
	}
}

// ApplyHealthCheckRoutes añade rutas de health check
func (ms *MiddlewareStack) ApplyHealthCheckRoutes(router *gin.Engine) {
	healthGroup := router.Group("/health")
	{
		healthGroup.GET("", BasicHealthHandler(ms.ServiceName, ms.ServiceVersion))
		healthGroup.GET("/detailed", DetailedHealthHandler(ms))
		healthGroup.GET("/ready", ReadinessHandler(ms))
		healthGroup.GET("/live", LivenessHandler(ms))
	}
	
	// Metrics endpoint
	if ms.EnableMetrics && ms.MetricsCollector != nil {
		router.GET("/metrics", metrics.MetricsHandler(ms.MetricsCollector))
		router.GET("/metrics/prometheus", metrics.NewPrometheusExporter(ms.MetricsCollector, ms.MetricsConfig, ms.Logger).Handler())
	}
	
	// Circuit breaker status
	if ms.EnableCircuitBreaker && ms.CircuitBreakerMgr != nil {
		router.GET("/circuit-breakers", circuitbreaker.CircuitBreakerHealthHandler(ms.CircuitBreakerMgr))
	}
}

// WithRedis configura Redis para el stack
func (ms *MiddlewareStack) WithRedis(client *redis.Client) *MiddlewareStack {
	ms.RedisClient = client
	return ms
}

// WithEnvironment configura el entorno
func (ms *MiddlewareStack) WithEnvironment(env string) *MiddlewareStack {
	ms.Environment = env
	return ms
}

// WithServiceInfo configura información del servicio
func (ms *MiddlewareStack) WithServiceInfo(name, version string) *MiddlewareStack {
	ms.ServiceName = name
	ms.ServiceVersion = version
	return ms
}

// EnableAll habilita todos los middlewares
func (ms *MiddlewareStack) EnableAll() *MiddlewareStack {
	ms.EnableCORS = true
	ms.EnableLogging = true
	ms.EnableMetrics = true
	ms.EnableRateLimit = true
	ms.EnableCircuitBreaker = true
	ms.EnableRecovery = true
	ms.EnableTenantContext = true
	return ms
}

// DisableAll deshabilita todos los middlewares
func (ms *MiddlewareStack) DisableAll() *MiddlewareStack {
	ms.EnableCORS = false
	ms.EnableLogging = false
	ms.EnableMetrics = false
	ms.EnableRateLimit = false
	ms.EnableCircuitBreaker = false
	ms.EnableRecovery = false
	ms.EnableTenantContext = false
	return ms
}

// GetStats retorna estadísticas del stack completo
func (ms *MiddlewareStack) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"service":     ms.ServiceName,
		"version":     ms.ServiceVersion,
		"environment": ms.Environment,
		"enabled_middlewares": map[string]bool{
			"cors":            ms.EnableCORS,
			"logging":         ms.EnableLogging,
			"metrics":         ms.EnableMetrics,
			"rate_limit":      ms.EnableRateLimit,
			"circuit_breaker": ms.EnableCircuitBreaker,
			"recovery":        ms.EnableRecovery,
			"tenant_context":  ms.EnableTenantContext,
		},
	}
	
	// Add component stats if available
	if ms.EnableMetrics && ms.MetricsCollector != nil {
		if inMemoryCollector, ok := ms.MetricsCollector.(*metrics.InMemoryMetricsCollector); ok {
			stats["metrics"] = inMemoryCollector.GetStats()
		}
	}
	
	if ms.EnableCircuitBreaker && ms.CircuitBreakerMgr != nil {
		stats["circuit_breakers"] = ms.CircuitBreakerMgr.GetAllStats()
	}
	
	if ms.EnableRecovery && ms.RecoveryStatsCollector != nil {
		stats["recovery"] = ms.RecoveryStatsCollector.GetStats()
	}
	
	return stats
}

// Shutdown limpia recursos del stack
func (ms *MiddlewareStack) Shutdown() error {
	ms.Logger.Info("Shutting down middleware stack")
	
	// Close any resources that need cleanup
	// Redis client should be managed externally
	
	return nil
}