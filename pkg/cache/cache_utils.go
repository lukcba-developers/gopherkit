package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// CacheKey construye claves de cache consistentes y optimizadas
type CacheKey struct {
	Prefix    string
	Service   string
	Entity    string
	ID        string
	Version   string
	Tenant    string
	Suffix    string
}

// String convierte CacheKey a string con formato optimizado
func (ck CacheKey) String() string {
	parts := []string{}
	
	if ck.Prefix != "" {
		parts = append(parts, ck.Prefix)
	}
	if ck.Service != "" {
		parts = append(parts, ck.Service)
	}
	if ck.Entity != "" {
		parts = append(parts, ck.Entity)
	}
	if ck.Tenant != "" {
		parts = append(parts, "tenant", ck.Tenant)
	}
	if ck.ID != "" {
		parts = append(parts, ck.ID)
	}
	if ck.Version != "" {
		parts = append(parts, "v", ck.Version)
	}
	if ck.Suffix != "" {
		parts = append(parts, ck.Suffix)
	}
	
	return strings.Join(parts, ":")
}

// CachePattern genera patrones para búsqueda/eliminación masiva
func (ck CacheKey) Pattern() string {
	key := ck.String()
	if !strings.HasSuffix(key, "*") {
		key += "*"
	}
	return key
}

// TTLCalculator calcula TTLs dinámicos basados en contexto
type TTLCalculator struct {
	baseTTL     time.Duration
	strategies  map[string]TTLStrategy
	mu          sync.RWMutex
}

// TTLStrategy define estrategia de TTL para diferentes tipos de datos
type TTLStrategy struct {
	MinTTL      time.Duration
	MaxTTL      time.Duration
	Multiplier  float64
	Randomness  float64 // Factor de aleatoridad para evitar cache stampede
	AccessBased bool    // TTL basado en frecuencia de acceso
}

// NewTTLCalculator crea un calculador de TTL con estrategias predefinidas
func NewTTLCalculator(baseTTL time.Duration) *TTLCalculator {
	calc := &TTLCalculator{
		baseTTL:    baseTTL,
		strategies: make(map[string]TTLStrategy),
	}
	
	// Estrategias predefinidas para diferentes tipos de datos
	calc.strategies["user_session"] = TTLStrategy{
		MinTTL:      15 * time.Minute,
		MaxTTL:      24 * time.Hour,
		Multiplier:  1.0,
		Randomness:  0.1,
		AccessBased: true,
	}
	
	calc.strategies["user_profile"] = TTLStrategy{
		MinTTL:      30 * time.Minute,
		MaxTTL:      4 * time.Hour,
		Multiplier:  2.0,
		Randomness:  0.15,
		AccessBased: false,
	}
	
	calc.strategies["configuration"] = TTLStrategy{
		MinTTL:      5 * time.Minute,
		MaxTTL:      1 * time.Hour,
		Multiplier:  0.5,
		Randomness:  0.05,
		AccessBased: false,
	}
	
	calc.strategies["query_result"] = TTLStrategy{
		MinTTL:      1 * time.Minute,
		MaxTTL:      15 * time.Minute,
		Multiplier:  0.8,
		Randomness:  0.2,
		AccessBased: true,
	}
	
	calc.strategies["static_content"] = TTLStrategy{
		MinTTL:      1 * time.Hour,
		MaxTTL:      24 * time.Hour,
		Multiplier:  5.0,
		Randomness:  0.1,
		AccessBased: false,
	}
	
	return calc
}

// CalculateTTL calcula TTL dinámico para un tipo de dato específico
func (tc *TTLCalculator) CalculateTTL(dataType string, accessCount int64) time.Duration {
	tc.mu.RLock()
	strategy, exists := tc.strategies[dataType]
	tc.mu.RUnlock()
	
	if !exists {
		return tc.baseTTL
	}
	
	// Calcular TTL base
	ttl := time.Duration(float64(tc.baseTTL) * strategy.Multiplier)
	
	// Ajustar basado en frecuencia de acceso si está habilitado
	if strategy.AccessBased && accessCount > 0 {
		// Más accesos = TTL más largo (hasta el máximo)
		accessMultiplier := 1.0 + (float64(accessCount)/100.0)*0.5
		if accessMultiplier > 3.0 {
			accessMultiplier = 3.0
		}
		ttl = time.Duration(float64(ttl) * accessMultiplier)
	}
	
	// Aplicar aleatorización para evitar cache stampede
	if strategy.Randomness > 0 {
		randomFactor := 1.0 + (strategy.Randomness * (2.0*float64(time.Now().UnixNano()%1000)/1000.0 - 1.0))
		ttl = time.Duration(float64(ttl) * randomFactor)
	}
	
	// Aplicar límites mínimos y máximos
	if ttl < strategy.MinTTL {
		ttl = strategy.MinTTL
	}
	if ttl > strategy.MaxTTL {
		ttl = strategy.MaxTTL
	}
	
	return ttl
}

// CacheWrapper proporciona funcionalidades de alto nivel para operaciones de cache comunes
type CacheWrapper struct {
	manager      *RedisManager
	ttlCalc      *TTLCalculator
	namespace    string
	logger       *logrus.Entry
	
	// Configuración de comportamiento
	fallbackEnabled    bool
	compressionEnabled bool
	serializationFormat string // "json", "msgpack", "protobuf"
	
	// Métricas específicas del wrapper
	wrapperMetrics *WrapperMetrics
}

// WrapperMetrics mantiene métricas específicas del wrapper
type WrapperMetrics struct {
	FallbackHits     int64
	SerializationErrors int64
	DeserializationErrors int64
	CompressionSavings int64
	mu sync.RWMutex
}

// NewCacheWrapper crea un wrapper de cache con funcionalidades avanzadas
func NewCacheWrapper(manager *RedisManager, namespace string) *CacheWrapper {
	return &CacheWrapper{
		manager:             manager,
		ttlCalc:            NewTTLCalculator(5 * time.Minute),
		namespace:          namespace,
		logger:             logrus.WithFields(logrus.Fields{
			"component": "gopherkit.cache.wrapper",
			"namespace": namespace,
		}),
		fallbackEnabled:     true,
		compressionEnabled:  false, // Habilitado a nivel de manager
		serializationFormat: "json",
		wrapperMetrics:     &WrapperMetrics{},
	}
}

// GetOrSet implementa patrón cache-aside con función de carga
func (cw *CacheWrapper) GetOrSet(ctx context.Context, key CacheKey, dataType string, 
	loadFunc func() (interface{}, error)) (interface{}, error) {
	
	keyStr := key.String()
	
	// Intentar obtener del cache primero
	var cachedData interface{}
	err := cw.manager.GetJSON(ctx, keyStr, &cachedData)
	if err == nil && cachedData != nil {
		cw.logger.WithField("key", keyStr).Debug("Cache hit en GetOrSet")
		return cachedData, nil
	}
	
	// Cache miss - cargar datos usando la función proporcionada
	data, err := loadFunc()
	if err != nil {
		cw.logger.WithFields(logrus.Fields{
			"key": keyStr,
			"error": err,
		}).Error("Error cargando datos para cache")
		return nil, fmt.Errorf("error cargando datos: %w", err)
	}
	
	// Calcular TTL dinámico
	ttl := cw.ttlCalc.CalculateTTL(dataType, 0) // TODO: Implementar contador de acceso
	
	// Almacenar en cache de forma asíncrona para no bloquear la respuesta
	go func() {
		setCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := cw.manager.SetWithTTL(setCtx, keyStr, data, ttl); err != nil {
			cw.logger.WithFields(logrus.Fields{
				"key": keyStr,
				"ttl": ttl,
				"error": err,
			}).Error("Error almacenando en cache")
		} else {
			cw.logger.WithFields(logrus.Fields{
				"key": keyStr,
				"ttl": ttl,
			}).Debug("Datos almacenados en cache exitosamente")
		}
	}()
	
	return data, nil
}

// GetOrSetJSON implementa GetOrSet específicamente para objetos JSON
func (cw *CacheWrapper) GetOrSetJSON(ctx context.Context, key CacheKey, dataType string, 
	target interface{}, loadFunc func() error) error {
	
	keyStr := key.String()
	
	// Intentar obtener del cache
	err := cw.manager.GetJSON(ctx, keyStr, target)
	if err == nil {
		cw.logger.WithField("key", keyStr).Debug("Cache hit en GetOrSetJSON")
		return nil
	}
	
	// Cache miss - cargar datos
	if err := loadFunc(); err != nil {
		return fmt.Errorf("error cargando datos: %w", err)
	}
	
	// Almacenar en cache
	ttl := cw.ttlCalc.CalculateTTL(dataType, 0)
	go func() {
		setCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := cw.manager.SetWithTTL(setCtx, keyStr, target, ttl); err != nil {
			cw.logger.WithError(err).Error("Error almacenando JSON en cache")
		}
	}()
	
	return nil
}

// InvalidatePattern invalida múltiples claves basadas en patrón
func (cw *CacheWrapper) InvalidatePattern(ctx context.Context, pattern CacheKey) error {
	patternStr := pattern.Pattern()
	
	cw.logger.WithField("pattern", patternStr).Info("Invalidando cache por patrón")
	
	err := cw.manager.DeletePattern(ctx, patternStr)
	if err != nil {
		cw.logger.WithFields(logrus.Fields{
			"pattern": patternStr,
			"error": err,
		}).Error("Error invalidando cache por patrón")
		return fmt.Errorf("error invalidando patrón %s: %w", patternStr, err)
	}
	
	return nil
}

// WarmCache precarga datos críticos en el cache
func (cw *CacheWrapper) WarmCache(ctx context.Context, warmupData map[CacheKey]WarmupEntry) error {
	cw.logger.WithField("entries", len(warmupData)).Info("Iniciando precarga de cache")
	
	for key, entry := range warmupData {
		keyStr := key.String()
		ttl := cw.ttlCalc.CalculateTTL(entry.DataType, 0)
		
		// Ejecutar función de carga de datos
		data, err := entry.LoadFunc()
		if err != nil {
			cw.logger.WithFields(logrus.Fields{
				"key": keyStr,
				"error": err,
			}).Warn("Error cargando datos para precarga")
			continue
		}
		
		// Almacenar en cache
		if err := cw.manager.SetWithTTL(ctx, keyStr, data, ttl); err != nil {
			cw.logger.WithFields(logrus.Fields{
				"key": keyStr,
				"error": err,
			}).Error("Error almacenando datos de precarga")
			continue
		}
		
		cw.logger.WithFields(logrus.Fields{
			"key": keyStr,
			"ttl": ttl,
		}).Debug("Datos precargados exitosamente")
	}
	
	cw.logger.Info("Precarga de cache completada")
	return nil
}

// WarmupEntry define una entrada para precarga de cache
type WarmupEntry struct {
	DataType string
	LoadFunc func() (interface{}, error)
}

// RefreshCache actualiza datos en cache de forma proactiva
func (cw *CacheWrapper) RefreshCache(ctx context.Context, key CacheKey, dataType string, 
	loadFunc func() (interface{}, error)) error {
	
	keyStr := key.String()
	
	// Cargar datos frescos
	data, err := loadFunc()
	if err != nil {
		return fmt.Errorf("error cargando datos frescos: %w", err)
	}
	
	// Actualizar cache con TTL extendido (datos frescos)
	ttl := cw.ttlCalc.CalculateTTL(dataType, 10) // Simular alta frecuencia de acceso
	
	if err := cw.manager.SetWithTTL(ctx, keyStr, data, ttl); err != nil {
		return fmt.Errorf("error actualizando cache: %w", err)
	}
	
	cw.logger.WithFields(logrus.Fields{
		"key": keyStr,
		"ttl": ttl,
	}).Debug("Cache actualizado proactivamente")
	
	return nil
}

// GetMultiple obtiene múltiples claves de cache de forma eficiente
func (cw *CacheWrapper) GetMultiple(ctx context.Context, keys []CacheKey) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Obtener claves en paralelo con límite de concurrencia
	semaphore := make(chan struct{}, 10) // Máximo 10 operaciones concurrentes
	
	for _, key := range keys {
		wg.Add(1)
		go func(k CacheKey) {
			defer wg.Done()
			semaphore <- struct{}{} // Adquirir semáforo
			defer func() { <-semaphore }() // Liberar semáforo
			
			keyStr := k.String()
			var data interface{}
			
			if err := cw.manager.GetJSON(ctx, keyStr, &data); err == nil && data != nil {
				mu.Lock()
				results[keyStr] = data
				mu.Unlock()
			}
		}(key)
	}
	
	wg.Wait()
	
	cw.logger.WithFields(logrus.Fields{
		"requested": len(keys),
		"found": len(results),
	}).Debug("Operación GetMultiple completada")
	
	return results, nil
}

// SetMultiple almacena múltiples claves de cache de forma eficiente
func (cw *CacheWrapper) SetMultiple(ctx context.Context, entries map[CacheKey]MultipleEntry) error {
	var wg sync.WaitGroup
	var errorCount int64
	
	semaphore := make(chan struct{}, 10) // Límite de concurrencia
	
	for key, entry := range entries {
		wg.Add(1)
		go func(k CacheKey, e MultipleEntry) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			keyStr := k.String()
			ttl := cw.ttlCalc.CalculateTTL(e.DataType, 0)
			
			if err := cw.manager.SetWithTTL(ctx, keyStr, e.Data, ttl); err != nil {
				cw.logger.WithFields(logrus.Fields{
					"key": keyStr,
					"error": err,
				}).Error("Error en SetMultiple")
				
				// Incrementar contador de errores de forma atómica
				atomic.AddInt64(&errorCount, 1)
			}
		}(key, entry)
	}
	
	wg.Wait()
	
	if errorCount > 0 {
		return fmt.Errorf("errores en %d de %d operaciones SetMultiple", errorCount, len(entries))
	}
	
	cw.logger.WithField("entries", len(entries)).Debug("SetMultiple completado exitosamente")
	return nil
}

// MultipleEntry define una entrada para operaciones múltiples
type MultipleEntry struct {
	Data     interface{}
	DataType string
}

// Lock implementa locks distribuidos usando Redis
func (cw *CacheWrapper) Lock(ctx context.Context, lockKey string, ttl time.Duration) (*DistributedLock, error) {
	return NewDistributedLock(cw.manager, lockKey, ttl)
}

// DistributedLock implementa un lock distribuido usando Redis
type DistributedLock struct {
	manager   *RedisManager
	key       string
	value     string
	ttl       time.Duration
	acquired  bool
	mu        sync.Mutex
}

// NewDistributedLock crea un nuevo lock distribuido
func NewDistributedLock(manager *RedisManager, key string, ttl time.Duration) (*DistributedLock, error) {
	lockValue := fmt.Sprintf("%d_%s", time.Now().UnixNano(), "gopherkit_lock")
	
	lock := &DistributedLock{
		manager: manager,
		key:     fmt.Sprintf("lock:%s", key),
		value:   lockValue,
		ttl:     ttl,
	}
	
	return lock, nil
}

// Acquire intenta adquirir el lock
func (dl *DistributedLock) Acquire(ctx context.Context) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	
	if dl.acquired {
		return fmt.Errorf("lock ya adquirido")
	}
	
	// Usar SetNX para adquisición atómica del lock
	acquired, err := dl.manager.SetNX(ctx, dl.key, dl.value, dl.ttl)
	if err != nil {
		return fmt.Errorf("error adquiriendo lock: %w", err)
	}
	
	if !acquired {
		return fmt.Errorf("lock no disponible")
	}
	
	dl.acquired = true
	
	// Iniciar renovación automática del lock
	go dl.autoRenew(ctx)
	
	return nil
}

// Release libera el lock
func (dl *DistributedLock) Release(ctx context.Context) error {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	
	if !dl.acquired {
		return fmt.Errorf("lock no adquirido")
	}
	
	// Script Lua para liberación atómica (verificar valor antes de eliminar)
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	
	// Ejecutar script usando el cliente Redis directamente
	result := dl.manager.getClient().Eval(ctx, script, []string{dl.key}, dl.value)
	if result.Err() != nil {
		return fmt.Errorf("error liberando lock: %w", result.Err())
	}
	
	dl.acquired = false
	return nil
}

// autoRenew renueva automáticamente el lock para evitar expiración
func (dl *DistributedLock) autoRenew(ctx context.Context) {
	ticker := time.NewTicker(dl.ttl / 3) // Renovar cada 1/3 del TTL
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			dl.mu.Lock()
			if !dl.acquired {
				dl.mu.Unlock()
				return
			}
			
			// Renovar TTL del lock
			err := dl.manager.getClient().Expire(ctx, dl.key, dl.ttl).Err()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"lock_key": dl.key,
					"error": err,
				}).Warn("Error renovando lock distribuido")
			}
			dl.mu.Unlock()
			
		case <-ctx.Done():
			return
		}
	}
}

// GetMetrics retorna métricas del wrapper
func (cw *CacheWrapper) GetMetrics() map[string]interface{} {
	cw.wrapperMetrics.mu.RLock()
	defer cw.wrapperMetrics.mu.RUnlock()
	
	baseMetrics := cw.manager.GetMetrics()
	
	wrapperMetrics := map[string]interface{}{
		"fallback_hits":           cw.wrapperMetrics.FallbackHits,
		"serialization_errors":    cw.wrapperMetrics.SerializationErrors,
		"deserialization_errors":  cw.wrapperMetrics.DeserializationErrors,
		"compression_savings":     cw.wrapperMetrics.CompressionSavings,
		"namespace":              cw.namespace,
		"fallback_enabled":       cw.fallbackEnabled,
		"compression_enabled":    cw.compressionEnabled,
		"serialization_format":   cw.serializationFormat,
	}
	
	// Combinar métricas base con métricas del wrapper
	if baseMetrics != nil {
		baseMetrics["wrapper"] = wrapperMetrics
		return baseMetrics
	}
	
	return map[string]interface{}{
		"wrapper": wrapperMetrics,
	}
}

// Helper functions para construcción de claves

// UserCacheKey construye clave de cache para datos de usuario
func UserCacheKey(userID, dataType string, tenant ...string) CacheKey {
	key := CacheKey{
		Prefix:  "gopherkit",
		Service: "user",
		Entity:  dataType,
		ID:      userID,
	}
	if len(tenant) > 0 && tenant[0] != "" {
		key.Tenant = tenant[0]
	}
	return key
}

// SessionCacheKey construye clave de cache para datos de sesión
func SessionCacheKey(sessionID string, tenant ...string) CacheKey {
	key := CacheKey{
		Prefix:  "gopherkit",
		Service: "auth",
		Entity:  "session",
		ID:      sessionID,
	}
	if len(tenant) > 0 && tenant[0] != "" {
		key.Tenant = tenant[0]
	}
	return key
}

// QueryCacheKey construye clave de cache para resultados de consulta
func QueryCacheKey(service, queryType, hash string, tenant ...string) CacheKey {
	key := CacheKey{
		Prefix:  "gopherkit",
		Service: service,
		Entity:  "query",
		Suffix:  queryType,
		ID:      hash,
	}
	if len(tenant) > 0 && tenant[0] != "" {
		key.Tenant = tenant[0]
	}
	return key
}

// ConfigCacheKey construye clave de cache para configuración
func ConfigCacheKey(service, configType string, tenant ...string) CacheKey {
	key := CacheKey{
		Prefix:  "gopherkit",
		Service: service,
		Entity:  "config",
		Suffix:  configType,
	}
	if len(tenant) > 0 && tenant[0] != "" {
		key.Tenant = tenant[0]
	}
	return key
}