package monitoring

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// PrometheusConfig configuración para métricas Prometheus
type PrometheusConfig struct {
	ServiceName    string
	Namespace      string
	Subsystem      string
	HistogramBuckets []float64
	
	// Custom labels
	DefaultLabels prometheus.Labels
	
	// Exclusions
	ExcludePaths []string
	ExcludeStatusCodes []int
}

// DefaultPrometheusConfig retorna configuración por defecto
func DefaultPrometheusConfig(serviceName string) *PrometheusConfig {
	return &PrometheusConfig{
		ServiceName: serviceName,
		Namespace:   "clubpulse",
		Subsystem:   "http",
		HistogramBuckets: []float64{
			0.0005, 0.001, 0.002, 0.005, 0.01, 0.02, 0.05,
			0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0,
		},
		DefaultLabels: prometheus.Labels{
			"service": serviceName,
		},
		ExcludePaths: []string{
			"/health", "/health/ready", "/health/live", "/metrics",
		},
		ExcludeStatusCodes: []int{},
	}
}

// PrometheusMetrics contiene todas las métricas Prometheus
type PrometheusMetrics struct {
	config *PrometheusConfig
	
	// HTTP Metrics
	requestDuration    *prometheus.HistogramVec
	requestsTotal      *prometheus.CounterVec
	requestsInFlight   prometheus.Gauge
	requestSizeBytes   *prometheus.HistogramVec
	responseSizeBytes  *prometheus.HistogramVec
	
	// Database Metrics
	dbConnectionsOpen     *prometheus.GaugeVec
	dbConnectionsIdle     *prometheus.GaugeVec
	dbConnectionsInUse    *prometheus.GaugeVec
	dbOperationDuration   *prometheus.HistogramVec
	dbOperationsTotal     *prometheus.CounterVec
	
	// Cache Metrics
	cacheOperationsTotal  *prometheus.CounterVec
	cacheHitRatio        *prometheus.GaugeVec
	cacheOperationDuration *prometheus.HistogramVec
	
	// Circuit Breaker Metrics
	circuitBreakerState   *prometheus.GaugeVec
	circuitBreakerRequests *prometheus.CounterVec
	circuitBreakerFailures *prometheus.CounterVec
	
	// Business Metrics
	activeUsers          prometheus.Gauge
	totalRegistrations   prometheus.Counter
	loginAttempts        *prometheus.CounterVec
	
	// Infrastructure Metrics
	goroutines           prometheus.Gauge
	memoryUsage          prometheus.Gauge
	cpuUsage            prometheus.Gauge
}

// NewPrometheusMetrics crea métricas Prometheus
func NewPrometheusMetrics(config *PrometheusConfig) *PrometheusMetrics {
	if config == nil {
		config = DefaultPrometheusConfig("unknown")
	}
	
	return &PrometheusMetrics{
		config: config,
		
		// HTTP Metrics
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "request_duration_seconds",
				Help:      "Time spent processing HTTP requests",
				Buckets:   config.HistogramBuckets,
				ConstLabels: config.DefaultLabels,
			},
			[]string{"method", "path", "status_code", "handler"},
		),
		
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "requests_total",
				Help:      "Total number of HTTP requests",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"method", "path", "status_code", "handler"},
		),
		
		requestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "requests_in_flight",
				Help:      "Current number of HTTP requests being processed",
				ConstLabels: config.DefaultLabels,
			},
		),
		
		requestSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "request_size_bytes",
				Help:      "Size of HTTP requests in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
				ConstLabels: config.DefaultLabels,
			},
			[]string{"method", "path"},
		),
		
		responseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "response_size_bytes",
				Help:      "Size of HTTP responses in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
				ConstLabels: config.DefaultLabels,
			},
			[]string{"method", "path", "status_code"},
		),
		
		// Database Metrics
		dbConnectionsOpen: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "database",
				Name:      "connections_open",
				Help:      "Number of open database connections",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"database", "host"},
		),
		
		dbConnectionsIdle: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "database",
				Name:      "connections_idle",
				Help:      "Number of idle database connections",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"database", "host"},
		),
		
		dbConnectionsInUse: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "database",
				Name:      "connections_in_use",
				Help:      "Number of database connections in use",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"database", "host"},
		),
		
		dbOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: "database",
				Name:      "operation_duration_seconds",
				Help:      "Time spent on database operations",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
				ConstLabels: config.DefaultLabels,
			},
			[]string{"operation", "table", "result"},
		),
		
		dbOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: "database",
				Name:      "operations_total",
				Help:      "Total number of database operations",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"operation", "table", "result"},
		),
		
		// Cache Metrics
		cacheOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: "cache",
				Name:      "operations_total",
				Help:      "Total number of cache operations",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"operation", "result"},
		),
		
		cacheHitRatio: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "cache",
				Name:      "hit_ratio",
				Help:      "Cache hit ratio (0-1)",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"cache_name"},
		),
		
		cacheOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: "cache",
				Name:      "operation_duration_seconds",
				Help:      "Time spent on cache operations",
				Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
				ConstLabels: config.DefaultLabels,
			},
			[]string{"operation", "result"},
		),
		
		// Circuit Breaker Metrics
		circuitBreakerState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "circuit_breaker",
				Name:      "state",
				Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"name"},
		),
		
		circuitBreakerRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: "circuit_breaker",
				Name:      "requests_total",
				Help:      "Total circuit breaker requests",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"name", "state", "result"},
		),
		
		circuitBreakerFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: "circuit_breaker",
				Name:      "failures_total",
				Help:      "Total circuit breaker failures",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"name"},
		),
		
		// Business Metrics
		activeUsers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "business",
				Name:      "active_users",
				Help:      "Number of currently active users",
				ConstLabels: config.DefaultLabels,
			},
		),
		
		totalRegistrations: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: "business",
				Name:      "registrations_total",
				Help:      "Total number of user registrations",
				ConstLabels: config.DefaultLabels,
			},
		),
		
		loginAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: "business",
				Name:      "login_attempts_total",
				Help:      "Total number of login attempts",
				ConstLabels: config.DefaultLabels,
			},
			[]string{"result", "method"},
		),
		
		// Infrastructure Metrics
		goroutines: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "runtime",
				Name:      "goroutines",
				Help:      "Number of goroutines",
				ConstLabels: config.DefaultLabels,
			},
		),
		
		memoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "runtime",
				Name:      "memory_bytes",
				Help:      "Memory usage in bytes",
				ConstLabels: config.DefaultLabels,
			},
		),
		
		cpuUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: "runtime",
				Name:      "cpu_usage_percent",
				Help:      "CPU usage percentage",
				ConstLabels: config.DefaultLabels,
			},
		),
	}
}

// HTTPMiddleware retorna middleware Gin para métricas HTTP
func (m *PrometheusMetrics) HTTPMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Skip excluded paths
		if m.shouldExcludePath(c.Request.URL.Path) {
			c.Next()
			return
		}
		
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		
		// Request size
		requestSize := c.Request.ContentLength
		if requestSize > 0 {
			m.requestSizeBytes.WithLabelValues(
				c.Request.Method,
				path,
			).Observe(float64(requestSize))
		}
		
		// Increment in-flight requests
		m.requestsInFlight.Inc()
		
		// Process request
		c.Next()
		
		// Decrement in-flight requests
		m.requestsInFlight.Dec()
		
		// Calculate duration
		duration := time.Since(start)
		statusCode := strconv.Itoa(c.Writer.Status())
		
		// Skip excluded status codes
		if m.shouldExcludeStatusCode(c.Writer.Status()) {
			return
		}
		
		// Extract handler name from context or route
		handler := c.HandlerName()
		if handler == "" {
			handler = "unknown"
		}
		
		// Record metrics
		labels := []string{c.Request.Method, path, statusCode, handler}
		
		m.requestDuration.WithLabelValues(labels...).Observe(duration.Seconds())
		m.requestsTotal.WithLabelValues(labels...).Inc()
		
		// Response size
		responseSize := c.Writer.Size()
		if responseSize > 0 {
			m.responseSizeBytes.WithLabelValues(
				c.Request.Method,
				path,
				statusCode,
			).Observe(float64(responseSize))
		}
		
		// Add trace information if available
		if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
			span.SetAttributes(
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.path", path),
				attribute.Int("http.status_code", c.Writer.Status()),
				attribute.Float64("http.duration_seconds", duration.Seconds()),
			)
		}
	})
}

// RecordDatabaseMetrics registra métricas de base de datos
func (m *PrometheusMetrics) RecordDatabaseMetrics(operation, table, result string, duration time.Duration) {
	m.dbOperationDuration.WithLabelValues(operation, table, result).Observe(duration.Seconds())
	m.dbOperationsTotal.WithLabelValues(operation, table, result).Inc()
}

// UpdateDatabaseConnections actualiza métricas de conexiones de BD
func (m *PrometheusMetrics) UpdateDatabaseConnections(database, host string, open, idle, inUse int) {
	m.dbConnectionsOpen.WithLabelValues(database, host).Set(float64(open))
	m.dbConnectionsIdle.WithLabelValues(database, host).Set(float64(idle))
	m.dbConnectionsInUse.WithLabelValues(database, host).Set(float64(inUse))
}

// RecordCacheOperation registra operaciones de cache
func (m *PrometheusMetrics) RecordCacheOperation(operation, result string, duration time.Duration) {
	m.cacheOperationsTotal.WithLabelValues(operation, result).Inc()
	m.cacheOperationDuration.WithLabelValues(operation, result).Observe(duration.Seconds())
}

// UpdateCacheHitRatio actualiza ratio de cache hits
func (m *PrometheusMetrics) UpdateCacheHitRatio(cacheName string, ratio float64) {
	m.cacheHitRatio.WithLabelValues(cacheName).Set(ratio)
}

// RecordCircuitBreakerState registra estado de circuit breaker
func (m *PrometheusMetrics) RecordCircuitBreakerState(name, state string, stateValue float64) {
	m.circuitBreakerState.WithLabelValues(name).Set(stateValue)
}

// RecordCircuitBreakerRequest registra request de circuit breaker
func (m *PrometheusMetrics) RecordCircuitBreakerRequest(name, state, result string) {
	m.circuitBreakerRequests.WithLabelValues(name, state, result).Inc()
}

// RecordCircuitBreakerFailure registra fallo de circuit breaker
func (m *PrometheusMetrics) RecordCircuitBreakerFailure(name string) {
	m.circuitBreakerFailures.WithLabelValues(name).Inc()
}

// Business metrics methods
func (m *PrometheusMetrics) SetActiveUsers(count float64) {
	m.activeUsers.Set(count)
}

func (m *PrometheusMetrics) IncrementRegistrations() {
	m.totalRegistrations.Inc()
}

func (m *PrometheusMetrics) RecordLoginAttempt(result, method string) {
	m.loginAttempts.WithLabelValues(result, method).Inc()
}

// Infrastructure metrics methods
func (m *PrometheusMetrics) UpdateRuntimeMetrics(goroutines int, memoryBytes float64, cpuPercent float64) {
	m.goroutines.Set(float64(goroutines))
	m.memoryUsage.Set(memoryBytes)
	m.cpuUsage.Set(cpuPercent)
}

// shouldExcludePath verifica si una ruta debe excluirse de métricas
func (m *PrometheusMetrics) shouldExcludePath(path string) bool {
	for _, excludePath := range m.config.ExcludePaths {
		if path == excludePath {
			return true
		}
	}
	return false
}

// shouldExcludeStatusCode verifica si un status code debe excluirse
func (m *PrometheusMetrics) shouldExcludeStatusCode(statusCode int) bool {
	for _, excludeCode := range m.config.ExcludeStatusCodes {
		if statusCode == excludeCode {
			return true
		}
	}
	return false
}

// Handler retorna el handler HTTP para métricas Prometheus
func (m *PrometheusMetrics) Handler() http.Handler {
	return promhttp.Handler()
}

// StartRuntimeMetricsCollector inicia recolección de métricas de runtime
func (m *PrometheusMetrics) StartRuntimeMetricsCollector(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.collectRuntimeMetrics()
			}
		}
	}()
}

// collectRuntimeMetrics recolecta métricas de runtime
func (m *PrometheusMetrics) collectRuntimeMetrics() {
	// Esta implementación se puede expandir con métricas más detalladas
	// Por ahora, implementación básica
}

// MetricsHandler crea handler Gin para endpoint de métricas
func (m *PrometheusMetrics) MetricsHandler() gin.HandlerFunc {
	handler := promhttp.Handler()
	return gin.WrapH(handler)
}