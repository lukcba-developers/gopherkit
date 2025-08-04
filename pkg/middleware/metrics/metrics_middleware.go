package metrics

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// MetricsConfig configuración del middleware de métricas
type MetricsConfig struct {
	// Metric collection settings
	EnableRequestMetrics   bool
	EnableResponseMetrics  bool
	EnableDurationMetrics  bool
	EnablePathMetrics      bool
	EnableMethodMetrics    bool
	EnableStatusMetrics    bool
	EnableTenantMetrics    bool
	
	// Path normalization
	NormalizePaths         bool
	PathPatterns          map[string]string // Regex patterns to normalize paths
	
	// Filtering
	SkipPaths             []string
	SkipMethods           []string
	
	// Buckets for histogram metrics
	DurationBuckets       []float64
	
	// Business metrics
	EnableBusinessMetrics bool
	BusinessEventTypes    []string
}

// DefaultMetricsConfig retorna configuración por defecto
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		EnableRequestMetrics:   true,
		EnableResponseMetrics:  true,
		EnableDurationMetrics:  true,
		EnablePathMetrics:      true,
		EnableMethodMetrics:    true,
		EnableStatusMetrics:    true,
		EnableTenantMetrics:    true,
		NormalizePaths:         true,
		PathPatterns: map[string]string{
			`/api/v1/users/[^/]+`:        "/api/v1/users/{id}",
			`/api/v1/bookings/[^/]+`:     "/api/v1/bookings/{id}",
			`/api/v1/facilities/[^/]+`:   "/api/v1/facilities/{id}",
			`/api/v1/payments/[^/]+`:     "/api/v1/payments/{id}",
		},
		SkipPaths:   []string{"/health", "/metrics", "/ping"},
		SkipMethods: []string{},
		DurationBuckets: []float64{
			0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
		},
		EnableBusinessMetrics: true,
		BusinessEventTypes: []string{
			"booking_created", "booking_cancelled", "payment_processed",
			"user_registered", "notification_sent",
		},
	}
}

// MetricsCollector interfaz para recolección de métricas
type MetricsCollector interface {
	// HTTP metrics
	RecordHTTPRequest(method, path, status string, duration time.Duration, tenantID string)
	RecordHTTPDuration(method, path string, duration time.Duration)
	RecordHTTPStatus(method, path, status string)
	
	// Business metrics
	RecordBusinessEvent(eventType, tenantID string, metadata map[string]interface{})
	RecordCounter(name string, labels map[string]string, value float64)
	RecordGauge(name string, labels map[string]string, value float64)
	RecordHistogram(name string, labels map[string]string, value float64)
	
	// Error metrics
	RecordError(errorType, tenantID, service string)
	RecordSecurityThreatDetected(threatType, severity, tenantID, userID string)
	RecordSecurityValidationFailed(validationType, tenantID, reason string)
}

// InMemoryMetricsCollector implementación en memoria para desarrollo/testing
type InMemoryMetricsCollector struct {
	config   *MetricsConfig
	logger   *logrus.Entry
	mutex    sync.RWMutex
	
	// Counters
	counters   map[string]float64
	
	// Gauges
	gauges     map[string]float64
	
	// Histograms (simple implementation)
	histograms map[string][]float64
	
	// Business events
	businessEvents []BusinessEvent
}

// BusinessEvent representa un evento de negocio
type BusinessEvent struct {
	Type      string                 `json:"type"`
	TenantID  string                 `json:"tenant_id"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// NewInMemoryMetricsCollector crea un nuevo recolector en memoria
func NewInMemoryMetricsCollector(config *MetricsConfig, logger *logrus.Logger) *InMemoryMetricsCollector {
	return &InMemoryMetricsCollector{
		config:         config,
		logger:         logger.WithField("component", "metrics_collector"),
		counters:       make(map[string]float64),
		gauges:         make(map[string]float64),
		histograms:     make(map[string][]float64),
		businessEvents: make([]BusinessEvent, 0),
	}
}

// RecordHTTPRequest registra una request HTTP
func (imc *InMemoryMetricsCollector) RecordHTTPRequest(method, path, status string, duration time.Duration, tenantID string) {
	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	// Record request counter
	if imc.config.EnableRequestMetrics {
		key := imc.buildMetricKey("http_requests_total", map[string]string{
			"method":    method,
			"path":      path,
			"status":    status,
			"tenant_id": tenantID,
		})
		imc.counters[key]++
	}

	// Record duration
	if imc.config.EnableDurationMetrics {
		key := imc.buildMetricKey("http_request_duration_seconds", map[string]string{
			"method":    method,
			"path":      path,
			"tenant_id": tenantID,
		})
		if _, exists := imc.histograms[key]; !exists {
			imc.histograms[key] = make([]float64, 0)
		}
		imc.histograms[key] = append(imc.histograms[key], duration.Seconds())
	}
}

// RecordHTTPDuration registra la duración de una request HTTP
func (imc *InMemoryMetricsCollector) RecordHTTPDuration(method, path string, duration time.Duration) {
	if !imc.config.EnableDurationMetrics {
		return
	}

	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	key := imc.buildMetricKey("http_duration_seconds", map[string]string{
		"method": method,
		"path":   path,
	})
	
	if _, exists := imc.histograms[key]; !exists {
		imc.histograms[key] = make([]float64, 0)
	}
	imc.histograms[key] = append(imc.histograms[key], duration.Seconds())
}

// RecordHTTPStatus registra el status code de una response HTTP
func (imc *InMemoryMetricsCollector) RecordHTTPStatus(method, path, status string) {
	if !imc.config.EnableStatusMetrics {
		return
	}

	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	key := imc.buildMetricKey("http_responses_total", map[string]string{
		"method": method,
		"path":   path,
		"status": status,
	})
	imc.counters[key]++
}

// RecordBusinessEvent registra un evento de negocio
func (imc *InMemoryMetricsCollector) RecordBusinessEvent(eventType, tenantID string, metadata map[string]interface{}) {
	if !imc.config.EnableBusinessMetrics {
		return
	}

	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	// Record counter
	key := imc.buildMetricKey("business_events_total", map[string]string{
		"event_type": eventType,
		"tenant_id":  tenantID,
	})
	imc.counters[key]++

	// Store event details
	event := BusinessEvent{
		Type:      eventType,
		TenantID:  tenantID,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}
	imc.businessEvents = append(imc.businessEvents, event)

	// Keep only last 1000 events
	if len(imc.businessEvents) > 1000 {
		imc.businessEvents = imc.businessEvents[len(imc.businessEvents)-1000:]
	}
}

// RecordCounter registra un contador
func (imc *InMemoryMetricsCollector) RecordCounter(name string, labels map[string]string, value float64) {
	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	key := imc.buildMetricKey(name, labels)
	imc.counters[key] += value
}

// RecordGauge registra un gauge
func (imc *InMemoryMetricsCollector) RecordGauge(name string, labels map[string]string, value float64) {
	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	key := imc.buildMetricKey(name, labels)
	imc.gauges[key] = value
}

// RecordHistogram registra un histograma
func (imc *InMemoryMetricsCollector) RecordHistogram(name string, labels map[string]string, value float64) {
	imc.mutex.Lock()
	defer imc.mutex.Unlock()

	key := imc.buildMetricKey(name, labels)
	if _, exists := imc.histograms[key]; !exists {
		imc.histograms[key] = make([]float64, 0)
	}
	imc.histograms[key] = append(imc.histograms[key], value)
}

// RecordError registra un error
func (imc *InMemoryMetricsCollector) RecordError(errorType, tenantID, service string) {
	imc.RecordCounter("errors_total", map[string]string{
		"error_type": errorType,
		"tenant_id":  tenantID,
		"service":    service,
	}, 1)
}

// RecordSecurityThreatDetected registra una amenaza de seguridad detectada
func (imc *InMemoryMetricsCollector) RecordSecurityThreatDetected(threatType, severity, tenantID, userID string) {
	imc.RecordCounter("security_threats_total", map[string]string{
		"threat_type": threatType,
		"severity":    severity,
		"tenant_id":   tenantID,
		"user_id":     userID,
	}, 1)
}

// RecordSecurityValidationFailed registra una validación de seguridad fallida
func (imc *InMemoryMetricsCollector) RecordSecurityValidationFailed(validationType, tenantID, reason string) {
	imc.RecordCounter("security_validation_failures_total", map[string]string{
		"validation_type": validationType,
		"tenant_id":       tenantID,
		"reason":          reason,
	}, 1)
}

// buildMetricKey construye una clave única para la métrica
func (imc *InMemoryMetricsCollector) buildMetricKey(name string, labels map[string]string) string {
	key := name
	for k, v := range labels {
		key += "_" + k + "_" + v
	}
	return key
}

// GetStats retorna todas las estadísticas
func (imc *InMemoryMetricsCollector) GetStats() map[string]interface{} {
	imc.mutex.RLock()
	defer imc.mutex.RUnlock()

	stats := map[string]interface{}{
		"counters":         make(map[string]float64),
		"gauges":           make(map[string]float64),
		"histograms":       make(map[string]interface{}),
		"business_events":  len(imc.businessEvents),
	}

	// Copy counters
	for k, v := range imc.counters {
		stats["counters"].(map[string]float64)[k] = v
	}

	// Copy gauges
	for k, v := range imc.gauges {
		stats["gauges"].(map[string]float64)[k] = v
	}

	// Process histograms
	histogramStats := make(map[string]interface{})
	for k, values := range imc.histograms {
		if len(values) > 0 {
			sum := 0.0
			min := values[0]
			max := values[0]
			
			for _, v := range values {
				sum += v
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			
			histogramStats[k] = map[string]interface{}{
				"count": len(values),
				"sum":   sum,
				"avg":   sum / float64(len(values)),
				"min":   min,
				"max":   max,
			}
		}
	}
	stats["histograms"] = histogramStats

	return stats
}

// MetricsMiddleware middleware para recolección de métricas
func MetricsMiddleware(collector MetricsCollector, config *MetricsConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	return func(c *gin.Context) {
		// Skip metrics for certain paths
		if shouldSkipPath(c.Request.URL.Path, config.SkipPaths) {
			c.Next()
			return
		}

		// Skip metrics for certain methods
		if shouldSkipMethod(c.Request.Method, config.SkipMethods) {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Normalize path if enabled
		if config.NormalizePaths {
			path = normalizePath(path, config.PathPatterns)
		}

		// Get tenant ID
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "unknown"
		}

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)
		status := strconv.Itoa(c.Writer.Status())

		// Record metrics
		if config.EnableRequestMetrics {
			collector.RecordHTTPRequest(method, path, status, duration, tenantID)
		}

		if config.EnableDurationMetrics {
			collector.RecordHTTPDuration(method, path, duration)
		}

		if config.EnableStatusMetrics {
			collector.RecordHTTPStatus(method, path, status)
		}
	}
}

// BusinessMetricsMiddleware middleware para métricas de negocio
func BusinessMetricsMiddleware(collector MetricsCollector, config *MetricsConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	return func(c *gin.Context) {
		c.Next()

		// Record business-specific metrics based on endpoints
		if config.EnableBusinessMetrics {
			recordBusinessMetrics(c, collector, config)
		}
	}
}

// recordBusinessMetrics registra métricas específicas de negocio
func recordBusinessMetrics(c *gin.Context, collector MetricsCollector, config *MetricsConfig) {
	path := c.Request.URL.Path
	method := c.Request.Method
	status := c.Writer.Status()
	tenantID := c.GetHeader("X-Tenant-ID")

	if tenantID == "" {
		tenantID = "unknown"
	}

	// Record metrics based on successful operations
	if status >= 200 && status < 300 {
		switch {
		case method == "POST" && strings.Contains(path, "/bookings"):
			collector.RecordBusinessEvent("booking_created", tenantID, map[string]interface{}{
				"path":   path,
				"method": method,
			})
		case method == "DELETE" && strings.Contains(path, "/bookings"):
			collector.RecordBusinessEvent("booking_cancelled", tenantID, map[string]interface{}{
				"path":   path,
				"method": method,
			})
		case method == "POST" && strings.Contains(path, "/payments"):
			collector.RecordBusinessEvent("payment_processed", tenantID, map[string]interface{}{
				"path":   path,
				"method": method,
			})
		case method == "POST" && strings.Contains(path, "/users"):
			collector.RecordBusinessEvent("user_registered", tenantID, map[string]interface{}{
				"path":   path,
				"method": method,
			})
		case method == "POST" && strings.Contains(path, "/notifications"):
			collector.RecordBusinessEvent("notification_sent", tenantID, map[string]interface{}{
				"path":   path,
				"method": method,
			})
		}
	}
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

func shouldSkipMethod(method string, skipMethods []string) bool {
	for _, skipMethod := range skipMethods {
		if method == skipMethod {
			return true
		}
	}
	return false
}

func normalizePath(path string, patterns map[string]string) string {
	// Simple path normalization - in production, use proper regex
	for pattern, _ := range patterns {
		// This is a simplified implementation
		// In production, you would use regexp.MustCompile(pattern).ReplaceAllString(path, replacement)
		if strings.Contains(path, strings.Split(pattern, "/")[len(strings.Split(pattern, "/"))-2]) {
			parts := strings.Split(path, "/")
			if len(parts) >= 4 {
				// Replace UUID-like patterns with {id}
				for i, part := range parts {
					if len(part) > 20 && (strings.Contains(part, "-") || len(part) == 36) {
						parts[i] = "{id}"
					}
				}
				return strings.Join(parts, "/")
			}
		}
	}
	return path
}

// MetricsHandler handler para exponer métricas
func MetricsHandler(collector MetricsCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if collector supports stats
		if statsCollector, ok := collector.(*InMemoryMetricsCollector); ok {
			stats := statsCollector.GetStats()
			c.JSON(200, gin.H{
				"metrics":   stats,
				"timestamp": time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(200, gin.H{
				"message":   "Metrics are being collected",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
	}
}

// PrometheusExporter implementación para exportar a Prometheus
// Esta sería una implementación más completa en producción
type PrometheusExporter struct {
	collector MetricsCollector
	config    *MetricsConfig
	logger    *logrus.Entry
}

// NewPrometheusExporter crea un nuevo exportador de Prometheus
func NewPrometheusExporter(collector MetricsCollector, config *MetricsConfig, logger *logrus.Logger) *PrometheusExporter {
	return &PrometheusExporter{
		collector: collector,
		config:    config,
		logger:    logger.WithField("component", "prometheus_exporter"),
	}
}

// Handler retorna un handler para métricas de Prometheus
func (pe *PrometheusExporter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// En una implementación real, esto generaría formato Prometheus
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.String(200, "# Prometheus metrics would be here\n# This is a placeholder implementation\n")
	}
}