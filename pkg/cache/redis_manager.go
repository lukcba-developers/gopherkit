package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

var (
	instance *RedisManager
	once     sync.Once
	logger   = logrus.WithField("component", "gopherkit.cache")
)

// RedisManager gestiona conexiones Redis con soporte avanzado para namespace, métricas y configuración dinámica
type RedisManager struct {
	client    *redis.Client
	clusterClient *redis.ClusterClient
	namespace string
	ttl       time.Duration
	mode      string // "single", "cluster", "sentinel"
	mu        sync.RWMutex
	metrics   *CacheMetrics
	config    *Config
	
	// Capacidades avanzadas
	circuitBreaker *CircuitBreaker
	retryPolicy    *RetryPolicy
	compressionEnabled bool
	encryptionEnabled  bool
	
	// Gestión de salud y rendimiento
	healthStatus   atomic.Bool
	lastHealthCheck time.Time
	performanceStats *PerformanceStats
	
	// Canal para shutdown graceful
	shutdownCh chan struct{}
	wg         sync.WaitGroup
}

// CacheMetrics rastrea estadísticas avanzadas de cache con métricas de negocio
type CacheMetrics struct {
	Hits     int64
	Misses   int64
	Sets     int64
	Deletes  int64
	Errors   int64
	HitRatio float64
	
	// Métricas avanzadas
	LatencySum    int64 // Microsegundos
	OperationCount int64
	EvictionCount  int64
	CompressionRatio float64
	NetworkErrors    int64
	TimeoutErrors    int64
	
	// Métricas de negocio específicas
	UserCacheHits      int64
	SessionCacheHits   int64
	QueryCacheHits     int64
	BusinessMetrics    map[string]int64
	
	mu sync.RWMutex
	
	// Métricas Prometheus
	hitCounter      prometheus.Counter
	missCounter     prometheus.Counter
	errorCounter    prometheus.Counter
	latencyHistogram prometheus.Histogram
	operationsGauge  prometheus.Gauge
}

// Config mantiene configuración avanzada de Redis
type Config struct {
	// Configuración básica
	Host      string
	Port      string
	Password  string
	DB        int
	Namespace string
	TTL       time.Duration
	Mode      string

	// Pool de conexiones optimizado
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	MaxConnAge   time.Duration

	// Timeouts configurables
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolTimeout  time.Duration

	// Configuración de cluster (si aplica)
	ClusterAddrs []string
	
	// Características avanzadas
	EnableCompression  bool
	EnableEncryption   bool
	EnableCircuitBreaker bool
	EnableRetryPolicy    bool
	
	// Configuración de salud
	HealthCheckInterval time.Duration
	MaxFailureThreshold int
	
	// Configuración de métricas
	EnablePrometheusMetrics bool
	MetricsNamespace       string
	
	// Integración con gopherkit
	BaseConfig     *config.BaseConfig
	AlertManager   *monitoring.AlertManager
	PrometheusIntegration *monitoring.PrometheusIntegration
}

// CircuitBreaker implementa patrón circuit breaker para Redis
type CircuitBreaker struct {
	failureCount    int64
	lastFailureTime time.Time
	threshold       int
	timeout         time.Duration
	state          string // "closed", "open", "half-open"
	mu             sync.RWMutex
}

// RetryPolicy define política de reintentos
type RetryPolicy struct {
	MaxAttempts    int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	BackoffFactor  float64
	RetryCondition func(error) bool
}

// PerformanceStats mantiene estadísticas de rendimiento
type PerformanceStats struct {
	TotalOperations     int64
	AverageLatency      float64
	P95Latency          float64
	P99Latency          float64
	ThroughputPerSecond float64
	LastUpdated         time.Time
	mu                  sync.RWMutex
}

// NewRedisManager crea un nuevo gestor Redis con configuración mejorada
func NewRedisManager(config *Config) (*RedisManager, error) {
	if config == nil {
		return nil, fmt.Errorf("configuración de Redis requerida")
	}

	// Aplicar configuración por defecto si es necesario
	applyDefaultConfig(config)

	// Validar configuración
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuración inválida: %w", err)
	}

	manager := &RedisManager{
		namespace:          config.Namespace,
		ttl:               config.TTL,
		mode:              config.Mode,
		config:            config,
		shutdownCh:        make(chan struct{}),
		compressionEnabled: config.EnableCompression,
		encryptionEnabled:  config.EnableEncryption,
		performanceStats:   &PerformanceStats{},
	}

	// Inicializar métricas con Prometheus si está habilitado
	if config.EnablePrometheusMetrics {
		manager.metrics = initializePrometheusMetrics(config.MetricsNamespace)
	} else {
		manager.metrics = &CacheMetrics{
			BusinessMetrics: make(map[string]int64),
		}
	}

	// Configurar circuit breaker
	if config.EnableCircuitBreaker {
		manager.circuitBreaker = &CircuitBreaker{
			threshold: config.MaxFailureThreshold,
			timeout:   30 * time.Second,
			state:     "closed",
		}
	}

	// Configurar política de reintentos
	if config.EnableRetryPolicy {
		manager.retryPolicy = &RetryPolicy{
			MaxAttempts:   3,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      2 * time.Second,
			BackoffFactor: 2.0,
			RetryCondition: func(err error) bool {
				// Reintentar en errores de red, no en errores de validación
				return strings.Contains(err.Error(), "connection") ||
					   strings.Contains(err.Error(), "timeout") ||
					   strings.Contains(err.Error(), "EOF")
			},
		}
	}

	// Crear cliente Redis basado en el modo
	if err := manager.createRedisClient(); err != nil {
		return nil, fmt.Errorf("error creando cliente Redis: %w", err)
	}

	// Verificar conectividad inicial
	if err := manager.testConnection(); err != nil {
		return nil, fmt.Errorf("error conectando a Redis: %w", err)
	}

	// Iniciar servicios en background
	manager.startBackgroundServices()

	// Integrar con sistemas de monitoreo si están disponibles
	if config.AlertManager != nil {
		manager.integratewithMonitoring(config.AlertManager)
	}

	logger.WithFields(logrus.Fields{
		"namespace":          config.Namespace,
		"mode":              config.Mode,
		"ttl":               config.TTL,
		"compression":       config.EnableCompression,
		"circuit_breaker":   config.EnableCircuitBreaker,
		"prometheus_metrics": config.EnablePrometheusMetrics,
	}).Info("Redis Manager inicializado exitosamente")

	return manager, nil
}

// GetInstance retorna instancia singleton de RedisManager con configuración thread-safe
func GetInstance(config *Config) (*RedisManager, error) {
	var err error
	once.Do(func() {
		instance, err = NewRedisManager(config)
		if err != nil {
			logger.WithError(err).Error("Error creando instancia Redis Manager")
		}
	})
	return instance, err
}

// createRedisClient crea el cliente apropiado basado en el modo configurado
func (rm *RedisManager) createRedisClient() error {
	switch rm.mode {
	case "single":
		return rm.createSingleClient()
	case "cluster":
		return rm.createClusterClient()
	case "sentinel":
		return rm.createSentinelClient()
	default:
		return fmt.Errorf("modo Redis no soportado: %s", rm.mode)
	}
}

// createSingleClient crea cliente Redis simple
func (rm *RedisManager) createSingleClient() error {
	options := &redis.Options{
		Addr:         fmt.Sprintf("%s:%s", rm.config.Host, rm.config.Port),
		Password:     rm.config.Password,
		DB:           rm.config.DB,
		PoolSize:     rm.config.PoolSize,
		MinIdleConns: rm.config.MinIdleConns,
		MaxRetries:   rm.config.MaxRetries,
		DialTimeout:  rm.config.DialTimeout,
		ReadTimeout:  rm.config.ReadTimeout,
		WriteTimeout: rm.config.WriteTimeout,
		PoolTimeout:  rm.config.PoolTimeout,
		MaxConnAge:   rm.config.MaxConnAge,
	}

	rm.client = redis.NewClient(options)
	return nil
}

// createClusterClient crea cliente Redis Cluster
func (rm *RedisManager) createClusterClient() error {
	if len(rm.config.ClusterAddrs) == 0 {
		return fmt.Errorf("direcciones de cluster requeridas para modo cluster")
	}

	options := &redis.ClusterOptions{
		Addrs:        rm.config.ClusterAddrs,
		Password:     rm.config.Password,
		PoolSize:     rm.config.PoolSize,
		MinIdleConns: rm.config.MinIdleConns,
		MaxRetries:   rm.config.MaxRetries,
		DialTimeout:  rm.config.DialTimeout,
		ReadTimeout:  rm.config.ReadTimeout,
		WriteTimeout: rm.config.WriteTimeout,
		PoolTimeout:  rm.config.PoolTimeout,
		MaxConnAge:   rm.config.MaxConnAge,
	}

	rm.clusterClient = redis.NewClusterClient(options)
	return nil
}

// createSentinelClient crea cliente Redis Sentinel
func (rm *RedisManager) createSentinelClient() error {
	// Implementación de Redis Sentinel
	options := &redis.FailoverOptions{
		MasterName:    "master",
		SentinelAddrs: rm.config.ClusterAddrs,
		Password:      rm.config.Password,
		DB:            rm.config.DB,
		PoolSize:      rm.config.PoolSize,
		MinIdleConns:  rm.config.MinIdleConns,
		MaxRetries:    rm.config.MaxRetries,
		DialTimeout:   rm.config.DialTimeout,
		ReadTimeout:   rm.config.ReadTimeout,
		WriteTimeout:  rm.config.WriteTimeout,
		PoolTimeout:   rm.config.PoolTimeout,
		MaxConnAge:    rm.config.MaxConnAge,
	}

	rm.client = redis.NewFailoverClient(options)
	return nil
}

// testConnection verifica la conectividad a Redis
func (rm *RedisManager) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	if rm.clusterClient != nil {
		err = rm.clusterClient.Ping(ctx).Err()
	} else {
		err = rm.client.Ping(ctx).Err()
	}

	if err != nil {
		rm.healthStatus.Store(false)
		return err
	}

	rm.healthStatus.Store(true)
	rm.lastHealthCheck = time.Now()
	return nil
}

// buildKey crea una clave con namespace mejorado
func (rm *RedisManager) buildKey(key string) string {
	if rm.namespace == "" {
		return key
	}
	
	// Soportar namespace jerárquico
	if strings.Contains(rm.namespace, ":") {
		return fmt.Sprintf("%s:%s", rm.namespace, key)
	}
	
	return fmt.Sprintf("%s:%s", rm.namespace, key)
}

// Get recupera un valor de cache con circuit breaker y métricas
func (rm *RedisManager) Get(ctx context.Context, key string) (string, error) {
	start := time.Now()
	defer func() {
		rm.recordLatency(time.Since(start))
	}()

	// Verificar circuit breaker
	if rm.circuitBreaker != nil && !rm.circuitBreaker.allowRequest() {
		rm.recordError("circuit_breaker_open")
		return "", fmt.Errorf("circuit breaker abierto")
	}

	fullKey := rm.buildKey(key)

	var val string
	var err error

	// Ejecutar con política de reintentos
	if rm.retryPolicy != nil {
		val, err = rm.executeWithRetry(func() (interface{}, error) {
			return rm.getClient().Get(ctx, fullKey).Result()
		}).(string), err
	} else {
		val, err = rm.getClient().Get(ctx, fullKey).Result()
	}

	if err != nil {
		if err == redis.Nil {
			rm.recordMiss()
			return "", nil
		}
		rm.recordError("get_failed")
		if rm.circuitBreaker != nil {
			rm.circuitBreaker.recordFailure()
		}
		return "", fmt.Errorf("error obteniendo clave %s: %w", fullKey, err)
	}

	rm.recordHit()
	if rm.circuitBreaker != nil {
		rm.circuitBreaker.recordSuccess()
	}

	// Descomprimir si está habilitado
	if rm.compressionEnabled {
		decompressed, err := rm.decompress([]byte(val))
		if err != nil {
			logger.WithError(err).Warn("Error descomprimiendo valor")
			return val, nil // Retornar valor original si falla descompresión
		}
		return string(decompressed), nil
	}

	return val, nil
}

// GetJSON recupera y deserializa un valor JSON del cache
func (rm *RedisManager) GetJSON(ctx context.Context, key string, dest interface{}) error {
	val, err := rm.Get(ctx, key)
	if err != nil {
		return err
	}

	if val == "" {
		return redis.Nil
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		rm.recordError("json_unmarshal_failed")
		return fmt.Errorf("error deserializando JSON: %w", err)
	}

	return nil
}

// Set almacena un valor en cache con TTL por defecto y características avanzadas
func (rm *RedisManager) Set(ctx context.Context, key string, value interface{}) error {
	return rm.SetWithTTL(ctx, key, value, rm.ttl)
}

// SetWithTTL almacena un valor en cache con TTL personalizado
func (rm *RedisManager) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	start := time.Now()
	defer func() {
		rm.recordLatency(time.Since(start))
	}()

	// Verificar circuit breaker
	if rm.circuitBreaker != nil && !rm.circuitBreaker.allowRequest() {
		rm.recordError("circuit_breaker_open")
		return fmt.Errorf("circuit breaker abierto")
	}

	fullKey := rm.buildKey(key)

	// Convertir valor a string
	var val []byte
	switch v := value.(type) {
	case string:
		val = []byte(v)
	case []byte:
		val = v
	default:
		// Marshal a JSON para tipos complejos
		data, err := json.Marshal(value)
		if err != nil {
			rm.recordError("json_marshal_failed")
			return fmt.Errorf("error serializando valor: %w", err)
		}
		val = data
	}

	// Comprimir si está habilitado
	if rm.compressionEnabled {
		compressed, err := rm.compress(val)
		if err != nil {
			logger.WithError(err).Warn("Error comprimiendo valor, usando original")
		} else {
			val = compressed
			rm.updateCompressionRatio(len(val), len(compressed))
		}
	}

	// Encriptar si está habilitado
	if rm.encryptionEnabled {
		encrypted, err := rm.encrypt(val)
		if err != nil {
			logger.WithError(err).Warn("Error encriptando valor, usando original")
		} else {
			val = encrypted
		}
	}

	// Ejecutar con política de reintentos
	var err error
	if rm.retryPolicy != nil {
		_, err = rm.executeWithRetry(func() (interface{}, error) {
			return nil, rm.getClient().Set(ctx, fullKey, val, ttl).Err()
		})
	} else {
		err = rm.getClient().Set(ctx, fullKey, val, ttl).Err()
	}

	if err != nil {
		rm.recordError("set_failed")
		if rm.circuitBreaker != nil {
			rm.circuitBreaker.recordFailure()
		}
		return fmt.Errorf("error almacenando clave %s: %w", fullKey, err)
	}

	rm.recordSet()
	if rm.circuitBreaker != nil {
		rm.circuitBreaker.recordSuccess()
	}

	return nil
}

// SetNX almacena un valor solo si la clave no existe (operación atómica)
func (rm *RedisManager) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	fullKey := rm.buildKey(key)

	// Convertir valor a string
	var val string
	switch v := value.(type) {
	case string:
		val = v
	case []byte:
		val = string(v)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			rm.recordError("json_marshal_failed")
			return false, fmt.Errorf("error serializando valor: %w", err)
		}
		val = string(data)
	}

	result, err := rm.getClient().SetNX(ctx, fullKey, val, ttl).Result()
	if err != nil {
		rm.recordError("setnx_failed")
		return false, fmt.Errorf("error en SetNX para clave %s: %w", fullKey, err)
	}

	if result {
		rm.recordSet()
	}

	return result, nil
}

// Delete elimina una o más claves del cache
func (rm *RedisManager) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		rm.recordLatency(time.Since(start))
	}()

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rm.buildKey(key)
	}

	var err error
	if rm.retryPolicy != nil {
		_, err = rm.executeWithRetry(func() (interface{}, error) {
			return nil, rm.getClient().Del(ctx, fullKeys...).Err()
		})
	} else {
		err = rm.getClient().Del(ctx, fullKeys...).Err()
	}

	if err != nil {
		rm.recordError("delete_failed")
		return fmt.Errorf("error eliminando claves: %w", err)
	}

	rm.recordDelete()
	return nil
}

// DeletePattern elimina todas las claves que coinciden con un patrón
func (rm *RedisManager) DeletePattern(ctx context.Context, pattern string) error {
	fullPattern := rm.buildKey(pattern)

	// Usar SCAN para encontrar claves que coincidan con el patrón
	var cursor uint64
	var keys []string

	for {
		var scanKeys []string
		var err error
		
		scanKeys, cursor, err = rm.getClient().Scan(ctx, cursor, fullPattern, 100).Result()
		if err != nil {
			rm.recordError("scan_failed")
			return fmt.Errorf("error escaneando claves: %w", err)
		}

		keys = append(keys, scanKeys...)

		if cursor == 0 {
			break
		}
	}

	if len(keys) > 0 {
		// Eliminar en lotes para mejor rendimiento
		batchSize := 100
		for i := 0; i < len(keys); i += batchSize {
			end := i + batchSize
			if end > len(keys) {
				end = len(keys)
			}

			if err := rm.getClient().Del(ctx, keys[i:end]...).Err(); err != nil {
				rm.recordError("batch_delete_failed")
				return fmt.Errorf("error eliminando lote de claves: %w", err)
			}
		}
		
		rm.recordDelete()
		logger.WithField("deleted_keys", len(keys)).Info("Eliminadas claves por patrón")
	}

	return nil
}

// GetMetrics retorna métricas completas del cache
func (rm *RedisManager) GetMetrics() map[string]interface{} {
	rm.metrics.mu.RLock()
	defer rm.metrics.mu.RUnlock()

	rm.performanceStats.mu.RLock()
	defer rm.performanceStats.mu.RUnlock()

	hitRatio := float64(0)
	total := rm.metrics.Hits + rm.metrics.Misses
	if total > 0 {
		hitRatio = float64(rm.metrics.Hits) / float64(total)
	}

	metrics := map[string]interface{}{
		"basic": map[string]interface{}{
			"namespace": rm.namespace,
			"mode":      rm.mode,
			"ttl":       rm.ttl.String(),
			"hits":      rm.metrics.Hits,
			"misses":    rm.metrics.Misses,
			"sets":      rm.metrics.Sets,
			"deletes":   rm.metrics.Deletes,
			"errors":    rm.metrics.Errors,
			"hit_ratio": hitRatio,
			"total_ops": total + rm.metrics.Sets + rm.metrics.Deletes,
		},
		"advanced": map[string]interface{}{
			"average_latency_ms":    rm.performanceStats.AverageLatency,
			"p95_latency_ms":       rm.performanceStats.P95Latency,
			"p99_latency_ms":       rm.performanceStats.P99Latency,
			"throughput_per_sec":   rm.performanceStats.ThroughputPerSecond,
			"eviction_count":       rm.metrics.EvictionCount,
			"compression_ratio":    rm.metrics.CompressionRatio,
			"network_errors":       rm.metrics.NetworkErrors,
			"timeout_errors":       rm.metrics.TimeoutErrors,
		},
		"business": rm.metrics.BusinessMetrics,
		"health": map[string]interface{}{
			"status":            rm.healthStatus.Load(),
			"last_health_check": rm.lastHealthCheck,
			"circuit_breaker":   rm.getCircuitBreakerStatus(),
		},
	}

	return metrics
}

// Health verifica la salud de la conexión Redis
func (rm *RedisManager) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := rm.getClient().Ping(ctx).Err()
	if err != nil {
		rm.healthStatus.Store(false)
		rm.recordError("health_check_failed")
		return fmt.Errorf("Redis health check falló: %w", err)
	}

	rm.healthStatus.Store(true)
	rm.lastHealthCheck = time.Now()
	return nil
}

// Close cierra la conexión Redis de forma graciosa
func (rm *RedisManager) Close() error {
	close(rm.shutdownCh)
	rm.wg.Wait()

	if rm.clusterClient != nil {
		return rm.clusterClient.Close()
	}
	return rm.client.Close()
}

// Métodos auxiliares y utilitarios

func (rm *RedisManager) getClient() redis.Cmdable {
	if rm.clusterClient != nil {
		return rm.clusterClient
	}
	return rm.client
}

func (rm *RedisManager) startBackgroundServices() {
	// Servicio de actualización de métricas
	rm.wg.Add(1)
	go rm.metricsUpdater()

	// Servicio de health checks
	if rm.config.HealthCheckInterval > 0 {
		rm.wg.Add(1)
		go rm.healthChecker()
	}

	// Servicio de limpieza de estadísticas
	rm.wg.Add(1)
	go rm.statsCleanup()
}

func (rm *RedisManager) metricsUpdater() {
	defer rm.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.updateCalculatedMetrics()
		case <-rm.shutdownCh:
			return
		}
	}
}

func (rm *RedisManager) healthChecker() {
	defer rm.wg.Done()
	ticker := time.NewTicker(rm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := rm.Health(ctx); err != nil {
				logger.WithError(err).Warn("Health check falló")
			}
			cancel()
		case <-rm.shutdownCh:
			return
		}
	}
}

func (rm *RedisManager) statsCleanup() {
	defer rm.wg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.resetOldStats()
		case <-rm.shutdownCh:
			return
		}
	}
}

func (rm *RedisManager) updateCalculatedMetrics() {
	rm.metrics.mu.Lock()
	total := rm.metrics.Hits + rm.metrics.Misses
	if total > 0 {
		rm.metrics.HitRatio = float64(rm.metrics.Hits) / float64(total)
	}
	rm.metrics.mu.Unlock()

	// Actualizar métricas de rendimiento
	rm.performanceStats.mu.Lock()
	if rm.metrics.OperationCount > 0 {
		rm.performanceStats.AverageLatency = float64(rm.metrics.LatencySum) / float64(rm.metrics.OperationCount) / 1000 // ms
	}
	rm.performanceStats.LastUpdated = time.Now()
	rm.performanceStats.mu.Unlock()
}

func (rm *RedisManager) resetOldStats() {
	// Reset periódico de estadísticas para evitar overflow
	rm.metrics.mu.Lock()
	if rm.metrics.OperationCount > 1000000 {
		rm.metrics.LatencySum = rm.metrics.LatencySum / 2
		rm.metrics.OperationCount = rm.metrics.OperationCount / 2
		logger.Info("Reset estadísticas de cache para prevenir overflow")
	}
	rm.metrics.mu.Unlock()
}

// Métodos de registro de métricas optimizados
func (rm *RedisManager) recordHit() {
	atomic.AddInt64(&rm.metrics.Hits, 1)
	rm.recordBusinessMetric("cache_hits", 1)
	if rm.metrics.hitCounter != nil {
		rm.metrics.hitCounter.Inc()
	}
}

func (rm *RedisManager) recordMiss() {
	atomic.AddInt64(&rm.metrics.Misses, 1)
	rm.recordBusinessMetric("cache_misses", 1)
	if rm.metrics.missCounter != nil {
		rm.metrics.missCounter.Inc()
	}
}

func (rm *RedisManager) recordSet() {
	atomic.AddInt64(&rm.metrics.Sets, 1)
	rm.recordBusinessMetric("cache_sets", 1)
}

func (rm *RedisManager) recordDelete() {
	atomic.AddInt64(&rm.metrics.Deletes, 1)
	rm.recordBusinessMetric("cache_deletes", 1)
}

func (rm *RedisManager) recordError(errorType string) {
	atomic.AddInt64(&rm.metrics.Errors, 1)
	rm.recordBusinessMetric("cache_errors", 1)
	rm.recordBusinessMetric("cache_errors_"+errorType, 1)
	
	if rm.metrics.errorCounter != nil {
		rm.metrics.errorCounter.Inc()
	}
	
	logger.WithField("error_type", errorType).Warn("Error de cache registrado")
}

func (rm *RedisManager) recordLatency(duration time.Duration) {
	microseconds := duration.Microseconds()
	atomic.AddInt64(&rm.metrics.LatencySum, microseconds)
	atomic.AddInt64(&rm.metrics.OperationCount, 1)
	
	if rm.metrics.latencyHistogram != nil {
		rm.metrics.latencyHistogram.Observe(duration.Seconds())
	}
}

func (rm *RedisManager) recordBusinessMetric(name string, value int64) {
	rm.metrics.mu.Lock()
	if rm.metrics.BusinessMetrics == nil {
		rm.metrics.BusinessMetrics = make(map[string]int64)
	}
	rm.metrics.BusinessMetrics[name] += value
	rm.metrics.mu.Unlock()
}

func (rm *RedisManager) updateCompressionRatio(original, compressed int) {
	if original > 0 {
		ratio := float64(compressed) / float64(original)
		rm.metrics.mu.Lock()
		rm.metrics.CompressionRatio = (rm.metrics.CompressionRatio + ratio) / 2 // Promedio móvil simple
		rm.metrics.mu.Unlock()
	}
}

// Placeholder para métodos de compresión/descompresión
func (rm *RedisManager) compress(data []byte) ([]byte, error) {
	// Implementar compresión (gzip, lz4, etc.)
	return data, nil
}

func (rm *RedisManager) decompress(data []byte) ([]byte, error) {
	// Implementar descompresión
	return data, nil
}

// Placeholder para métodos de encriptación/desencriptación
func (rm *RedisManager) encrypt(data []byte) ([]byte, error) {
	// Implementar encriptación (AES, etc.)
	return data, nil
}

func (rm *RedisManager) decrypt(data []byte) ([]byte, error) {
	// Implementar desencriptación
	return data, nil
}

// Métodos del circuit breaker
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == "closed" {
		return true
	}

	if cb.state == "open" {
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = "half-open"
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	}

	// half-open state
	return true
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == "half-open" {
		cb.state = "closed"
		cb.failureCount = 0
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= int64(cb.threshold) {
		cb.state = "open"
	}
}

func (rm *RedisManager) getCircuitBreakerStatus() map[string]interface{} {
	if rm.circuitBreaker == nil {
		return map[string]interface{}{"enabled": false}
	}

	rm.circuitBreaker.mu.RLock()
	defer rm.circuitBreaker.mu.RUnlock()

	return map[string]interface{}{
		"enabled":          true,
		"state":           rm.circuitBreaker.state,
		"failure_count":   rm.circuitBreaker.failureCount,
		"last_failure":    rm.circuitBreaker.lastFailureTime,
		"threshold":       rm.circuitBreaker.threshold,
	}
}

// executeWithRetry ejecuta una función con política de reintentos
func (rm *RedisManager) executeWithRetry(fn func() (interface{}, error)) (interface{}, error) {
	var lastErr error
	
	for attempt := 0; attempt < rm.retryPolicy.MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		
		if !rm.retryPolicy.RetryCondition(err) {
			break
		}

		if attempt < rm.retryPolicy.MaxAttempts-1 {
			delay := time.Duration(float64(rm.retryPolicy.InitialDelay) * 
				float64(attempt) * rm.retryPolicy.BackoffFactor)
			if delay > rm.retryPolicy.MaxDelay {
				delay = rm.retryPolicy.MaxDelay
			}
			time.Sleep(delay)
		}
	}

	return nil, lastErr
}

// integratewithMonitoring integra con el sistema de monitoreo de gopherkit
func (rm *RedisManager) integratewithMonitoring(alertManager *monitoring.AlertManager) {
	// Configurar alertas específicas para Redis
	if alertManager != nil {
		// Registrar métricas customizadas
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					metrics := rm.GetMetrics()
					// Enviar métricas al sistema de monitoreo
					if basicMetrics, ok := metrics["basic"].(map[string]interface{}); ok {
						if hitRatio, ok := basicMetrics["hit_ratio"].(float64); ok && hitRatio < 0.5 {
							alertManager.TriggerAlert("redis_low_hit_ratio", 
								fmt.Sprintf("Ratio de aciertos Redis bajo: %.2f", hitRatio))
						}
					}
				case <-rm.shutdownCh:
					return
				}
			}
		}()
	}
}

// initializePrometheusMetrics inicializa métricas de Prometheus
func initializePrometheusMetrics(namespace string) *CacheMetrics {
	if namespace == "" {
		namespace = "gopherkit"
	}

	metrics := &CacheMetrics{
		BusinessMetrics: make(map[string]int64),
		hitCounter: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "hits_total",
			Help:      "Total number of cache hits",
		}),
		missCounter: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "misses_total",
			Help:      "Total number of cache misses",
		}),
		errorCounter: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "errors_total",
			Help:      "Total number of cache errors",
		}),
		latencyHistogram: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "operation_duration_seconds",
			Help:      "Cache operation duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}),
		operationsGauge: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "operations_in_flight",
			Help:      "Number of cache operations currently in flight",
		}),
	}

	return metrics
}

// Funciones de configuración

func applyDefaultConfig(config *Config) {
	if config.PoolSize == 0 {
		config.PoolSize = 20
	}
	if config.MinIdleConns == 0 {
		config.MinIdleConns = 5
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 3 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 3 * time.Second
	}
	if config.PoolTimeout == 0 {
		config.PoolTimeout = 4 * time.Second
	}
	if config.MaxConnAge == 0 {
		config.MaxConnAge = 0 // Sin límite por defecto
	}
	if config.TTL == 0 {
		config.TTL = 5 * time.Minute
	}
	if config.Mode == "" {
		config.Mode = "single"
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.MaxFailureThreshold == 0 {
		config.MaxFailureThreshold = 5
	}
	if config.MetricsNamespace == "" {
		config.MetricsNamespace = "gopherkit"
	}
}

func validateConfig(config *Config) error {
	if config.Host == "" {
		return fmt.Errorf("host Redis requerido")
	}
	if config.Port == "" {
		return fmt.Errorf("puerto Redis requerido")
	}
	if config.Mode == "cluster" && len(config.ClusterAddrs) == 0 {
		return fmt.Errorf("direcciones de cluster requeridas para modo cluster")
	}
	if config.TTL < 0 {
		return fmt.Errorf("TTL no puede ser negativo")
	}
	if config.PoolSize < 1 {
		return fmt.Errorf("tamaño de pool debe ser mayor a 0")
	}
	return nil
}

// LoadConfigFromBase carga configuración Redis desde BaseConfig de gopherkit
func LoadConfigFromBase(baseConfig *config.BaseConfig, serviceName string) *Config {
	config := &Config{
		Host:      baseConfig.Cache.Host,
		Port:      baseConfig.Cache.Port,
		Password:  baseConfig.Cache.Password,
		DB:        baseConfig.Cache.DB,
		Namespace: baseConfig.Cache.Prefix,
		TTL:       baseConfig.Cache.TTL,
		Mode:      "single", // Por defecto
		
		// Usar configuración optimizada basada en el entorno
		PoolSize:     20,
		MinIdleConns: 5,
		MaxRetries:   3,
		MaxConnAge:   30 * time.Minute,
		
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
		
		// Habilitar características avanzadas en producción
		EnableCompression:       baseConfig.IsProduction(),
		EnableEncryption:        baseConfig.IsProduction(),
		EnableCircuitBreaker:    true,
		EnableRetryPolicy:       true,
		EnablePrometheusMetrics: baseConfig.Observability.MetricsEnabled,
		
		HealthCheckInterval: 30 * time.Second,
		MaxFailureThreshold: 5,
		MetricsNamespace:   "gopherkit_" + serviceName,
		
		BaseConfig: baseConfig,
	}

	// Ajustar configuración según el entorno
	if baseConfig.IsDevelopment() {
		config.EnableCompression = false
		config.EnableEncryption = false
		config.HealthCheckInterval = 60 * time.Second
	}

	return config
}