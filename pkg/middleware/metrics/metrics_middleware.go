package metrics

import (
	"context"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/metrics"
)

// MetricsMiddleware proporciona middleware de métricas para Gin
type MetricsMiddleware struct {
	metrics *metrics.PrometheusMetrics
	logger  logger.Logger
}

// NewMetricsMiddleware crea un nuevo middleware de métricas
func NewMetricsMiddleware(m *metrics.PrometheusMetrics, logger logger.Logger) *MetricsMiddleware {
	return &MetricsMiddleware{
		metrics: m,
		logger:  logger,
	}
}

// GinMiddleware devuelve el middleware de Gin para métricas
func (m *MetricsMiddleware) GinMiddleware() gin.HandlerFunc {
	return m.metrics.GinMiddleware()
}

// SystemMetricsCollector inicia la recolección automática de métricas del sistema
func (m *MetricsMiddleware) SystemMetricsCollector(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collectSystemMetrics()
		}
	}
}

func (m *MetricsMiddleware) collectSystemMetrics() {
	// Recolectar métricas de runtime
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Actualizar métricas
	m.metrics.SetGoroutines(runtime.NumGoroutine())
	m.metrics.SetMemoryUsage(memStats.Alloc)

	m.logger.LogBusinessEvent(context.Background(), "system_metrics_collected", map[string]interface{}{
		"goroutines":      runtime.NumGoroutine(),
		"memory_alloc":    memStats.Alloc,
		"memory_sys":      memStats.Sys,
		"gc_cycles":       memStats.NumGC,
		"heap_objects":    memStats.HeapObjects,
	})
}

// BusinessEventRecorder proporciona una interfaz para registrar eventos de negocio
type BusinessEventRecorder struct {
	metrics *metrics.PrometheusMetrics
	logger  logger.Logger
}

// NewBusinessEventRecorder crea un nuevo registrador de eventos de negocio
func NewBusinessEventRecorder(m *metrics.PrometheusMetrics, logger logger.Logger) *BusinessEventRecorder {
	return &BusinessEventRecorder{
		metrics: m,
		logger:  logger,
	}
}

// RecordEvent registra un evento de negocio
func (r *BusinessEventRecorder) RecordEvent(eventType, tenantID string, data map[string]interface{}) {
	r.metrics.RecordBusinessEvent(eventType, tenantID)
	r.logger.LogBusinessEvent(context.Background(), eventType, map[string]interface{}{
		"tenant_id": tenantID,
		"data":      data,
	})
}

// RecordUserLogin registra un login de usuario
func (r *BusinessEventRecorder) RecordUserLogin(userID, tenantID string, successful bool) {
	status := "success"
	if !successful {
		status = "failure"
	}
	
	r.metrics.RecordBusinessEvent("user_login_"+status, tenantID)
	r.logger.LogBusinessEvent(context.Background(), "user_login", map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
		"success":   successful,
	})
}

// RecordAPICall registra una llamada API específica
func (r *BusinessEventRecorder) RecordAPICall(endpoint, method, tenantID string, duration time.Duration, statusCode int) {
	r.metrics.RecordHTTPRequest(method, endpoint, string(rune(statusCode)), tenantID, duration.Seconds())
	r.logger.LogBusinessEvent(context.Background(), "api_call", map[string]interface{}{
		"endpoint":    endpoint,
		"method":      method,
		"tenant_id":   tenantID,
		"duration_ms": duration.Milliseconds(),
		"status_code": statusCode,
	})
}

// DatabaseMetricsRecorder registra métricas específicas de base de datos
type DatabaseMetricsRecorder struct {
	metrics *metrics.PrometheusMetrics
	logger  logger.Logger
}

// NewDatabaseMetricsRecorder crea un nuevo registrador de métricas de BD
func NewDatabaseMetricsRecorder(m *metrics.PrometheusMetrics, logger logger.Logger) *DatabaseMetricsRecorder {
	return &DatabaseMetricsRecorder{
		metrics: m,
		logger:  logger,
	}
}

// RecordQuery registra una query de base de datos
func (r *DatabaseMetricsRecorder) RecordQuery(operation, table string, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	
	r.metrics.RecordDBQuery(operation, table, status, duration.Seconds())
	
	if err != nil {
		r.logger.LogError(context.Background(), err, "database_query_error", map[string]interface{}{
			"operation": operation,
			"table":     table,
			"duration":  duration.Milliseconds(),
		})
	} else {
		r.logger.LogBusinessEvent(context.Background(), "database_query", map[string]interface{}{
			"operation": operation,
			"table":     table,
			"duration":  duration.Milliseconds(),
		})
	}
}

// UpdateConnections actualiza las métricas de conexiones de BD
func (r *DatabaseMetricsRecorder) UpdateConnections(active, idle, max int) {
	r.metrics.SetDBConnections(active, idle, max)
	r.logger.LogBusinessEvent(context.Background(), "database_connections_updated", map[string]interface{}{
		"active": active,
		"idle":   idle,
		"max":    max,
	})
}

// CacheMetricsRecorder registra métricas específicas de cache
type CacheMetricsRecorder struct {
	metrics *metrics.PrometheusMetrics
	logger  logger.Logger
}

// NewCacheMetricsRecorder crea un nuevo registrador de métricas de cache
func NewCacheMetricsRecorder(m *metrics.PrometheusMetrics, logger logger.Logger) *CacheMetricsRecorder {
	return &CacheMetricsRecorder{
		metrics: m,
		logger:  logger,
	}
}

// RecordOperation registra una operación de cache
func (r *CacheMetricsRecorder) RecordOperation(operation string, duration time.Duration, hit bool, err error) {
	status := "success"
	if err != nil {
		status = "error"
	} else if operation == "GET" && !hit {
		status = "miss"
	}
	
	r.metrics.RecordCacheOperation(operation, status, duration.Seconds())
	
	r.logger.LogBusinessEvent(context.Background(), "cache_operation", map[string]interface{}{
		"operation": operation,
		"status":    status,
		"duration":  duration.Microseconds(),
		"hit":       hit,
		"error":     err != nil,
	})
}

// UpdateStats actualiza estadísticas generales del cache
func (r *CacheMetricsRecorder) UpdateStats(hitRatio float64, totalKeys int) {
	r.metrics.SetCacheHitRatio(hitRatio)
	r.metrics.SetCacheKeysTotal(totalKeys)
	
	r.logger.LogBusinessEvent(context.Background(), "cache_stats_updated", map[string]interface{}{
		"hit_ratio":   hitRatio,
		"total_keys":  totalKeys,
	})
}