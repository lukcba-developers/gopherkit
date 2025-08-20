package cache

import (
	"sync"
	"time"
)

// BasicCacheMetrics recolecta métricas de cache
type BasicCacheMetrics struct {
	operations map[string]*OperationMetrics
	mutex      sync.RWMutex
	startTime  time.Time
}

// OperationMetrics métricas para una operación específica
type OperationMetrics struct {
	Count     int64
	TotalTime time.Duration
	LastTime  time.Time
	Errors    int64
}

// NewBasicCacheMetrics crea un nuevo recolector de métricas
func NewBasicCacheMetrics() *BasicCacheMetrics {
	return &BasicCacheMetrics{
		operations: make(map[string]*OperationMetrics),
		startTime:  time.Now(),
	}
}

// RecordOperation registra una operación de cache
func (m *BasicCacheMetrics) RecordOperation(operation, key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.operations[operation]; !exists {
		m.operations[operation] = &OperationMetrics{}
	}

	metrics := m.operations[operation]
	metrics.Count++
	metrics.LastTime = time.Now()
}

// RecordOperationWithDuration registra una operación con duración
func (m *BasicCacheMetrics) RecordOperationWithDuration(operation, key string, duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.operations[operation]; !exists {
		m.operations[operation] = &OperationMetrics{}
	}

	metrics := m.operations[operation]
	metrics.Count++
	metrics.TotalTime += duration
	metrics.LastTime = time.Now()
}

// RecordError registra un error en una operación
func (m *BasicCacheMetrics) RecordError(operation string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.operations[operation]; !exists {
		m.operations[operation] = &OperationMetrics{}
	}

	m.operations[operation].Errors++
}

// GetStats retorna todas las estadísticas
func (m *BasicCacheMetrics) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"uptime":     time.Since(m.startTime).String(),
		"operations": make(map[string]interface{}),
	}

	for operation, metrics := range m.operations {
		var avgDuration time.Duration
		if metrics.Count > 0 && metrics.TotalTime > 0 {
			avgDuration = metrics.TotalTime / time.Duration(metrics.Count)
		}

		stats["operations"].(map[string]interface{})[operation] = map[string]interface{}{
			"count":        metrics.Count,
			"errors":       metrics.Errors,
			"total_time":   metrics.TotalTime.String(),
			"avg_duration": avgDuration.String(),
			"last_time":    metrics.LastTime.Format(time.RFC3339),
			"error_rate":   calculateErrorRate(metrics),
		}
	}

	return stats
}

// calculateErrorRate calcula la tasa de error
func calculateErrorRate(metrics *OperationMetrics) float64 {
	total := metrics.Count + metrics.Errors
	if total == 0 {
		return 0.0
	}
	return float64(metrics.Errors) / float64(total)
}

// Reset resetea todas las métricas
func (m *BasicCacheMetrics) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.operations = make(map[string]*OperationMetrics)
	m.startTime = time.Now()
}