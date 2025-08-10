package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetrics contiene todas las métricas de Prometheus
type PrometheusMetrics struct {
	// Métricas HTTP
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestSize      *prometheus.HistogramVec
	httpResponseSize     *prometheus.HistogramVec
	httpActiveRequests   *prometheus.GaugeVec

	// Métricas de base de datos
	dbConnectionsActive  prometheus.Gauge
	dbConnectionsIdle    prometheus.Gauge
	dbConnectionsMax     prometheus.Gauge
	dbQueryDuration      *prometheus.HistogramVec
	dbQueryTotal         *prometheus.CounterVec

	// Métricas de cache (Redis)
	cacheOperationsTotal *prometheus.CounterVec
	cacheOperationsDuration *prometheus.HistogramVec
	cacheHitRatio        prometheus.Gauge
	cacheKeysTotal       prometheus.Gauge

	// Métricas de JWT
	jwtTokensIssued      *prometheus.CounterVec
	jwtTokensValidated   *prometheus.CounterVec
	jwtValidationDuration *prometheus.HistogramVec

	// Métricas de negocio
	businessEventsTotal  *prometheus.CounterVec
	userSessionsActive   prometheus.Gauge
	tenantRequestsTotal  *prometheus.CounterVec

	// Métricas del sistema
	goroutinesActive     prometheus.Gauge
	memoryUsage          prometheus.Gauge
	cpuUsage             prometheus.Gauge

	registry *prometheus.Registry
}

// NewPrometheusMetrics crea una nueva instancia de métricas de Prometheus
func NewPrometheusMetrics(serviceName, version string) *PrometheusMetrics {
	registry := prometheus.NewRegistry()

	// Labels comunes
	commonLabels := prometheus.Labels{
		"service": serviceName,
		"version": version,
	}

	metrics := &PrometheusMetrics{
		registry: registry,

		// Métricas HTTP
		httpRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "http_requests_total",
				Help:        "Total number of HTTP requests",
				ConstLabels: commonLabels,
			},
			[]string{"method", "endpoint", "status", "tenant_id"},
		),

		httpRequestDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "http_request_duration_seconds",
				Help:        "HTTP request duration in seconds",
				ConstLabels: commonLabels,
				Buckets:     prometheus.DefBuckets,
			},
			[]string{"method", "endpoint", "status"},
		),

		httpRequestSize: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "http_request_size_bytes",
				Help:        "HTTP request size in bytes",
				ConstLabels: commonLabels,
				Buckets:     []float64{1, 10, 100, 1000, 10000, 100000, 1000000},
			},
			[]string{"method", "endpoint"},
		),

		httpResponseSize: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "http_response_size_bytes",
				Help:        "HTTP response size in bytes",
				ConstLabels: commonLabels,
				Buckets:     []float64{1, 10, 100, 1000, 10000, 100000, 1000000},
			},
			[]string{"method", "endpoint"},
		),

		httpActiveRequests: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "http_active_requests",
				Help:        "Number of active HTTP requests",
				ConstLabels: commonLabels,
			},
			[]string{"method", "endpoint"},
		),

		// Métricas de base de datos
		dbConnectionsActive: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "db_connections_active",
				Help:        "Number of active database connections",
				ConstLabels: commonLabels,
			},
		),

		dbConnectionsIdle: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "db_connections_idle",
				Help:        "Number of idle database connections",
				ConstLabels: commonLabels,
			},
		),

		dbConnectionsMax: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "db_connections_max",
				Help:        "Maximum number of database connections",
				ConstLabels: commonLabels,
			},
		),

		dbQueryDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "db_query_duration_seconds",
				Help:        "Database query duration in seconds",
				ConstLabels: commonLabels,
				Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"operation", "table", "status"},
		),

		dbQueryTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "db_queries_total",
				Help:        "Total number of database queries",
				ConstLabels: commonLabels,
			},
			[]string{"operation", "table", "status"},
		),

		// Métricas de cache
		cacheOperationsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "cache_operations_total",
				Help:        "Total number of cache operations",
				ConstLabels: commonLabels,
			},
			[]string{"operation", "status"},
		),

		cacheOperationsDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "cache_operations_duration_seconds",
				Help:        "Cache operation duration in seconds",
				ConstLabels: commonLabels,
				Buckets:     []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"operation"},
		),

		cacheHitRatio: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "cache_hit_ratio",
				Help:        "Cache hit ratio (0-1)",
				ConstLabels: commonLabels,
			},
		),

		cacheKeysTotal: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "cache_keys_total",
				Help:        "Total number of keys in cache",
				ConstLabels: commonLabels,
			},
		),

		// Métricas de JWT
		jwtTokensIssued: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "jwt_tokens_issued_total",
				Help:        "Total number of JWT tokens issued",
				ConstLabels: commonLabels,
			},
			[]string{"token_type", "tenant_id"},
		),

		jwtTokensValidated: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "jwt_tokens_validated_total",
				Help:        "Total number of JWT tokens validated",
				ConstLabels: commonLabels,
			},
			[]string{"status", "reason"},
		),

		jwtValidationDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "jwt_validation_duration_seconds",
				Help:        "JWT validation duration in seconds",
				ConstLabels: commonLabels,
				Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"status"},
		),

		// Métricas de negocio
		businessEventsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "business_events_total",
				Help:        "Total number of business events",
				ConstLabels: commonLabels,
			},
			[]string{"event_type", "tenant_id"},
		),

		userSessionsActive: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "user_sessions_active",
				Help:        "Number of active user sessions",
				ConstLabels: commonLabels,
			},
		),

		tenantRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name:        "tenant_requests_total",
				Help:        "Total number of requests per tenant",
				ConstLabels: commonLabels,
			},
			[]string{"tenant_id", "endpoint"},
		),

		// Métricas del sistema
		goroutinesActive: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "goroutines_active",
				Help:        "Number of active goroutines",
				ConstLabels: commonLabels,
			},
		),

		memoryUsage: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "memory_usage_bytes",
				Help:        "Memory usage in bytes",
				ConstLabels: commonLabels,
			},
		),

		cpuUsage: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name:        "cpu_usage_percent",
				Help:        "CPU usage percentage",
				ConstLabels: commonLabels,
			},
		),
	}

	// Registrar métricas del sistema Go predeterminadas
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	return metrics
}

// GetRegistry devuelve el registry de Prometheus
func (m *PrometheusMetrics) GetRegistry() *prometheus.Registry {
	return m.registry
}

// GetHandler devuelve el handler HTTP para métricas de Prometheus
func (m *PrometheusMetrics) GetHandler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Middleware de Gin para métricas HTTP
func (m *PrometheusMetrics) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// Obtener información de la request
		method := c.Request.Method
		endpoint := c.FullPath()
		if endpoint == "" {
			endpoint = "unknown"
		}
		
		// Incrementar requests activos
		m.httpActiveRequests.WithLabelValues(method, endpoint).Inc()
		
		// Medir tamaño de request
		if c.Request.ContentLength > 0 {
			m.httpRequestSize.WithLabelValues(method, endpoint).Observe(float64(c.Request.ContentLength))
		}

		// Procesar request
		c.Next()

		// Decrementar requests activos
		m.httpActiveRequests.WithLabelValues(method, endpoint).Dec()

		// Obtener información de la response
		status := strconv.Itoa(c.Writer.Status())
		tenantID := c.GetString("tenant_id")
		if tenantID == "" {
			tenantID = "unknown"
		}

		// Registrar métricas
		duration := time.Since(start).Seconds()
		m.httpRequestDuration.WithLabelValues(method, endpoint, status).Observe(duration)
		m.httpRequestsTotal.WithLabelValues(method, endpoint, status, tenantID).Inc()
		m.tenantRequestsTotal.WithLabelValues(tenantID, endpoint).Inc()

		// Medir tamaño de response
		responseSize := c.Writer.Size()
		if responseSize > 0 {
			m.httpResponseSize.WithLabelValues(method, endpoint).Observe(float64(responseSize))
		}
	}
}

// Métricas HTTP
func (m *PrometheusMetrics) RecordHTTPRequest(method, endpoint, status, tenantID string, duration float64) {
	m.httpRequestsTotal.WithLabelValues(method, endpoint, status, tenantID).Inc()
	m.httpRequestDuration.WithLabelValues(method, endpoint, status).Observe(duration)
}

// Métricas de base de datos
func (m *PrometheusMetrics) RecordDBQuery(operation, table, status string, duration float64) {
	m.dbQueryTotal.WithLabelValues(operation, table, status).Inc()
	m.dbQueryDuration.WithLabelValues(operation, table, status).Observe(duration)
}

func (m *PrometheusMetrics) SetDBConnections(active, idle, max int) {
	m.dbConnectionsActive.Set(float64(active))
	m.dbConnectionsIdle.Set(float64(idle))
	m.dbConnectionsMax.Set(float64(max))
}

// Métricas de cache
func (m *PrometheusMetrics) RecordCacheOperation(operation, status string, duration float64) {
	m.cacheOperationsTotal.WithLabelValues(operation, status).Inc()
	m.cacheOperationsDuration.WithLabelValues(operation).Observe(duration)
}

func (m *PrometheusMetrics) SetCacheHitRatio(ratio float64) {
	m.cacheHitRatio.Set(ratio)
}

func (m *PrometheusMetrics) SetCacheKeysTotal(total int) {
	m.cacheKeysTotal.Set(float64(total))
}

// Métricas de JWT
func (m *PrometheusMetrics) RecordJWTTokenIssued(tokenType, tenantID string) {
	m.jwtTokensIssued.WithLabelValues(tokenType, tenantID).Inc()
}

func (m *PrometheusMetrics) RecordJWTValidation(status, reason string, duration float64) {
	m.jwtTokensValidated.WithLabelValues(status, reason).Inc()
	m.jwtValidationDuration.WithLabelValues(status).Observe(duration)
}

// Métricas de negocio
func (m *PrometheusMetrics) RecordBusinessEvent(eventType, tenantID string) {
	m.businessEventsTotal.WithLabelValues(eventType, tenantID).Inc()
}

func (m *PrometheusMetrics) SetActiveUserSessions(count int) {
	m.userSessionsActive.Set(float64(count))
}

// Métricas del sistema
func (m *PrometheusMetrics) SetGoroutines(count int) {
	m.goroutinesActive.Set(float64(count))
}

func (m *PrometheusMetrics) SetMemoryUsage(bytes uint64) {
	m.memoryUsage.Set(float64(bytes))
}

func (m *PrometheusMetrics) SetCPUUsage(percent float64) {
	m.cpuUsage.Set(percent)
}

// StartSystemMetricsCollection inicia la recolección automática de métricas del sistema
func (m *PrometheusMetrics) StartSystemMetricsCollection(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.collectSystemMetrics()
			}
		}
	}()
}

func (m *PrometheusMetrics) collectSystemMetrics() {
	// Las métricas del sistema Go se recolectan automáticamente
	// por los collectors registrados en NewPrometheusMetrics
}