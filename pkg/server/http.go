package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/metrics"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/common"
	metricsMiddleware "github.com/lukcba-developers/gopherkit/pkg/middleware/metrics"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/security"
	telemetryMiddleware "github.com/lukcba-developers/gopherkit/pkg/middleware/telemetry"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
	"github.com/lukcba-developers/gopherkit/pkg/telemetry"
)

// HTTPServer represents an HTTP server with standard middleware
type HTTPServer struct {
	config                *config.BaseConfig
	logger                logger.Logger
	router                *gin.Engine
	server                *http.Server
	middleware            MiddlewareStack
	prometheusMetrics     *metrics.PrometheusMetrics
	otelProvider          *telemetry.OpenTelemetryProvider
	systemMetricsCtx      context.Context
	systemMetricsCancel   context.CancelFunc
}

// MiddlewareStack holds the middleware components
type MiddlewareStack struct {
	Security      *security.SecurityMiddleware
	Common        *common.CommonMiddleware
	Observability *observability.SimpleObservabilityMiddleware
	Metrics       *metricsMiddleware.MetricsMiddleware
	Telemetry     *telemetryMiddleware.OpenTelemetryMiddleware
}

// Options for server configuration
type Options struct {
	Config           *config.BaseConfig
	Logger           logger.Logger
	CustomMiddleware []gin.HandlerFunc
	HealthChecks     []observability.HealthCheck
	Routes           func(*gin.Engine) // Function to setup custom routes
	
	// OpenTelemetry configuration (optional)
	EnableOpenTelemetry bool
	OTelConfig          *telemetry.OpenTelemetryConfig
}

// NewHTTPServer creates a new HTTP server with standard configuration
func NewHTTPServer(opts Options) (*HTTPServer, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Set Gin mode based on environment
	if opts.Config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	} else if opts.Config.IsDevelopment() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.TestMode)
	}

	// Create router
	router := gin.New()

	// Initialize Prometheus metrics
	prometheusMetrics := metrics.NewPrometheusMetrics(
		opts.Config.Observability.ServiceName,
		opts.Config.Observability.ServiceVersion,
	)

	// Initialize OpenTelemetry provider (optional)
	var otelProvider *telemetry.OpenTelemetryProvider
	if opts.EnableOpenTelemetry {
		otelConfig := opts.OTelConfig
		if otelConfig == nil {
			otelConfig = telemetry.DefaultOpenTelemetryConfig()
			otelConfig.ServiceName = opts.Config.Observability.ServiceName
			otelConfig.ServiceVersion = opts.Config.Observability.ServiceVersion
			otelConfig.Environment = opts.Config.Server.Environment
		}
		
		otelProvider, err = telemetry.NewOpenTelemetryProvider(otelConfig, opts.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
		}
	}

	// Initialize middleware stack
	middlewareStack, err := initializeMiddlewareStack(opts.Config, opts.Logger, prometheusMetrics, otelProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize middleware: %w", err)
	}

	// Create context for system metrics collection
	sysMetricsCtx, sysMetricsCancel := context.WithCancel(context.Background())

	// Create server instance
	srv := &HTTPServer{
		config:              opts.Config,
		logger:              opts.Logger,
		router:              router,
		middleware:          middlewareStack,
		prometheusMetrics:   prometheusMetrics,
		otelProvider:        otelProvider,
		systemMetricsCtx:    sysMetricsCtx,
		systemMetricsCancel: sysMetricsCancel,
	}

	// Setup middleware
	srv.setupMiddleware(opts.CustomMiddleware)

	// Setup standard routes
	srv.setupStandardRoutes(opts.HealthChecks)

	// Setup custom routes if provided
	if opts.Routes != nil {
		opts.Routes(router)
	}

	// Create HTTP server
	srv.server = &http.Server{
		Addr:              fmt.Sprintf(":%s", opts.Config.Server.Port),
		Handler:           router,
		ReadTimeout:       opts.Config.Server.ReadTimeout,
		WriteTimeout:      opts.Config.Server.WriteTimeout,
		IdleTimeout:       opts.Config.Server.IdleTimeout,
		ReadHeaderTimeout: 20 * time.Second,
	}

	return srv, nil
}

// setupMiddleware configures the middleware stack
func (s *HTTPServer) setupMiddleware(customMiddleware []gin.HandlerFunc) {
	// Core middleware (applied to all routes)
	s.router.Use(s.middleware.Common.Recovery())
	s.router.Use(s.middleware.Common.RequestID())
	s.router.Use(s.middleware.Common.Correlation())
	
	// Telemetry middleware (if enabled - must be early to capture all spans)
	if s.middleware.Telemetry != nil {
		s.router.Use(s.middleware.Telemetry.TracingMiddleware())
		s.router.Use(s.middleware.Telemetry.MetricsMiddleware())
	}
	
	// Metrics middleware (capture all requests)
	s.router.Use(s.middleware.Metrics.GinMiddleware())
	
	// Observability middleware
	s.router.Use(s.middleware.Observability.Metrics())
	s.router.Use(s.middleware.Observability.Logging(s.logger))
	s.router.Use(s.middleware.Observability.Tracing())

	// Security middleware
	s.router.Use(s.middleware.Security.CORS(s.config.Security.CORS))
	s.router.Use(s.middleware.Security.SecurityHeaders())
	s.router.Use(s.middleware.Security.RateLimit(s.config.Security.RateLimit))
	s.router.Use(s.middleware.Security.RequestSizeLimit(s.config.Server.MaxRequestSize))

	// Custom middleware
	for _, mw := range customMiddleware {
		s.router.Use(mw)
	}
}

// setupStandardRoutes sets up standard endpoints
func (s *HTTPServer) setupStandardRoutes(healthChecks []observability.HealthCheck) {
	// Health check endpoints
	healthGroup := s.router.Group("/health")
	{
		healthGroup.GET("/", s.basicHealthCheck)
		healthGroup.GET("/live", s.livenessCheck)
		healthGroup.GET("/ready", s.readinessCheck(healthChecks))
	}

	// Metrics endpoint
	s.router.GET("/metrics", gin.WrapH(s.prometheusMetrics.GetHandler()))

	// Info endpoint
	s.router.GET("/info", s.infoHandler)
}

// initializeMiddlewareStack creates the middleware stack
func initializeMiddlewareStack(config *config.BaseConfig, logger logger.Logger, prometheusMetrics *metrics.PrometheusMetrics, otelProvider *telemetry.OpenTelemetryProvider) (MiddlewareStack, error) {
	// Initialize security middleware
	securityMW, err := security.NewSecurityMiddleware(security.Config{
		RateLimit:      config.Security.RateLimit,
		CircuitBreaker: config.Security.CircuitBreaker,
		CORS:           config.Security.CORS,
	}, logger)
	if err != nil {
		return MiddlewareStack{}, err
	}

	// Initialize common middleware
	commonMW := common.NewCommonMiddleware(logger)

	// Initialize observability middleware (simplified)
	observabilityMW := observability.NewSimpleObservabilityMiddleware(observability.Config{
		ServiceName:    config.Observability.ServiceName,
		ServiceVersion: config.Observability.ServiceVersion,
		TracingEnabled: config.Observability.TracingEnabled,
		MetricsEnabled: config.Observability.MetricsEnabled,
	}, logger)

	// Initialize metrics middleware
	metricsMW := metricsMiddleware.NewMetricsMiddleware(prometheusMetrics, logger)
	
	// Initialize telemetry middleware (optional)
	var telemetryMW *telemetryMiddleware.OpenTelemetryMiddleware
	if otelProvider != nil {
		telemetryMW = telemetryMiddleware.NewOpenTelemetryMiddleware(
			otelProvider, 
			logger, 
			config.Observability.ServiceName,
		)
	}

	return MiddlewareStack{
		Security:      securityMW,
		Common:        commonMW,
		Observability: observabilityMW,
		Metrics:       metricsMW,
		Telemetry:     telemetryMW,
	}, nil
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	// Start system metrics collection
	go s.middleware.Metrics.SystemMetricsCollector(s.systemMetricsCtx)
	
	// Start server in a goroutine
	go func() {
		s.logger.WithFields(map[string]interface{}{
			"port":        s.config.Server.Port,
			"environment": s.config.Server.Environment,
			"service":     s.config.Observability.ServiceName,
		}).LogBusinessEvent(context.Background(), "server_starting", nil)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.LogError(context.Background(), err, "server failed to start", nil)
			panic(err)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.logger.LogBusinessEvent(context.Background(), "server_stopping", nil)

	// Stop system metrics collection
	if s.systemMetricsCancel != nil {
		s.systemMetricsCancel()
	}

	// Shutdown OpenTelemetry provider
	if s.otelProvider != nil {
		if err := s.otelProvider.Shutdown(ctx); err != nil {
			s.logger.LogError(ctx, err, "failed to shutdown OpenTelemetry provider", nil)
		}
	}

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.LogError(context.Background(), err, "server forced to shutdown", nil)
		return err
	}

	s.logger.LogBusinessEvent(context.Background(), "server_stopped", nil)
	return nil
}

// WaitForShutdown waits for interrupt signal and gracefully shuts down
func (s *HTTPServer) WaitForShutdown() error {
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	s.logger.LogBusinessEvent(context.Background(), "shutdown_signal_received", nil)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Server.ShutdownTimeout)
	defer cancel()

	return s.Stop(ctx)
}

// GetRouter returns the Gin router for additional configuration
func (s *HTTPServer) GetRouter() *gin.Engine {
	return s.router
}

// Health check handlers
func (s *HTTPServer) basicHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": s.config.Observability.ServiceName,
		"version": s.config.Observability.ServiceVersion,
		"time":    time.Now().UTC(),
	})
}

func (s *HTTPServer) livenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
		"time":   time.Now().UTC(),
	})
}

func (s *HTTPServer) readinessCheck(healthChecks []observability.HealthCheck) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		results := make(map[string]interface{})
		allHealthy := true

		// Run health checks
		for _, check := range healthChecks {
			result := check.Check(ctx)
			results[check.Name()] = map[string]interface{}{
				"healthy": result.Healthy,
				"message": result.Message,
				"latency": result.Latency,
			}
			if !result.Healthy {
				allHealthy = false
			}
		}

		status := http.StatusOK
		if !allHealthy {
			status = http.StatusServiceUnavailable
		}

		c.JSON(status, gin.H{
			"status":      map[string]string{"ready": fmt.Sprintf("%t", allHealthy)},
			"checks":      results,
			"service":     s.config.Observability.ServiceName,
			"version":     s.config.Observability.ServiceVersion,
			"timestamp":   time.Now().UTC(),
		})
	}
}

func (s *HTTPServer) infoHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": map[string]interface{}{
			"name":        s.config.Observability.ServiceName,
			"version":     s.config.Observability.ServiceVersion,
			"environment": s.config.Server.Environment,
			"port":        s.config.Server.Port,
		},
		"build": map[string]interface{}{
			"time":    time.Now().UTC(),
			"version": s.config.Observability.ServiceVersion,
		},
		"runtime": map[string]interface{}{
			"uptime": time.Since(time.Now()).String(), // This should be calculated from server start time
		},
	})
}