// Package database - Tenant Context Management System  
// Migrado de ClubPulse a gopherkit con mejoras significativas
package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// TenantContextManager maneja el contexto de tenant en la base de datos mejorado
// para implementar Row Level Security (RLS) con funcionalidades avanzadas
type TenantContextManager struct {
	db      *sqlx.DB
	logger  Logger
	metrics *TenantMetrics
	config  *TenantConfig
	cache   *TenantCache
	mu      sync.RWMutex
}

// TenantConfig configuración del sistema de multi-tenancy
type TenantConfig struct {
	EnableCaching          bool          `json:"enable_caching"`
	CacheTTL              time.Duration `json:"cache_ttl"`
	EnableValidation      bool          `json:"enable_validation"`
	EnableAuditLog        bool          `json:"enable_audit_log"`
	MaxConcurrentTenants  int           `json:"max_concurrent_tenants"`
	TenantTimeout         time.Duration `json:"tenant_timeout"`
	EnableMetrics         bool          `json:"enable_metrics"`
	EnableSecurityChecks  bool          `json:"enable_security_checks"`
	DefaultTenantID       string        `json:"default_tenant_id"`
}

// DefaultTenantConfig retorna configuración por defecto
func DefaultTenantConfig() *TenantConfig {
	return &TenantConfig{
		EnableCaching:         true,
		CacheTTL:             15 * time.Minute,
		EnableValidation:     true,
		EnableAuditLog:       true,
		MaxConcurrentTenants: 1000,
		TenantTimeout:        30 * time.Second,
		EnableMetrics:        true,
		EnableSecurityChecks: true,
		DefaultTenantID:      "550e8400-e29b-41d4-a716-446655440000",
	}
}

// TenantMetrics métricas del sistema de tenancy
type TenantMetrics struct {
	TenantSets           int64                    `json:"tenant_sets"`
	TenantGets           int64                    `json:"tenant_gets"`
	TenantClears         int64                    `json:"tenant_clears"`
	ValidationFailures   int64                    `json:"validation_failures"`
	SecurityViolations   int64                    `json:"security_violations"`
	CacheHits           int64                    `json:"cache_hits"`
	CacheMisses         int64                    `json:"cache_misses"`
	ActiveTenants       map[string]time.Time     `json:"active_tenants"`
	TenantOperations    map[string]int64         `json:"tenant_operations"`
	AverageSetTime      time.Duration            `json:"average_set_time"`
	AverageGetTime      time.Duration            `json:"average_get_time"`
	LastOperation       time.Time                `json:"last_operation"`
	OperationHistory    []TenantOperation        `json:"operation_history"`
	mu                  sync.RWMutex
}

// TenantOperation representa una operación de tenant
type TenantOperation struct {
	Type      string    `json:"type"`
	TenantID  string    `json:"tenant_id"`
	Success   bool      `json:"success"`
	Duration  time.Duration `json:"duration"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TenantCache cache simple para contextos de tenant
type TenantCache struct {
	data   map[string]TenantCacheEntry
	mu     sync.RWMutex
	ttl    time.Duration
	maxSize int
}

// TenantCacheEntry entrada del cache de tenant
type TenantCacheEntry struct {
	TenantID  string    `json:"tenant_id"`
	SetTime   time.Time `json:"set_time"`
	LastUsed  time.Time `json:"last_used"`
	UseCount  int64     `json:"use_count"`
}

// NewTenantCache crea un nuevo cache de tenants
func NewTenantCache(ttl time.Duration, maxSize int) *TenantCache {
	cache := &TenantCache{
		data:    make(map[string]TenantCacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
	
	// Iniciar limpieza periódica
	go cache.startCleanup()
	
	return cache
}

// startCleanup inicia la limpieza periódica del cache
func (tc *TenantCache) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		tc.cleanup()
	}
}

// cleanup limpia entradas expiradas del cache
func (tc *TenantCache) cleanup() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	
	now := time.Now()
	for key, entry := range tc.data {
		if now.Sub(entry.LastUsed) > tc.ttl {
			delete(tc.data, key)
		}
	}
}

// Get obtiene un tenant del cache
func (tc *TenantCache) Get(key string) (string, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	
	entry, exists := tc.data[key]
	if !exists || time.Since(entry.LastUsed) > tc.ttl {
		return "", false
	}
	
	// Actualizar última uso
	entry.LastUsed = time.Now()
	entry.UseCount++
	tc.data[key] = entry
	
	return entry.TenantID, true
}

// Set almacena un tenant en el cache
func (tc *TenantCache) Set(key, tenantID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	
	// Si el cache está lleno, eliminar la entrada menos usada
	if len(tc.data) >= tc.maxSize {
		tc.evictLeastUsed()
	}
	
	tc.data[key] = TenantCacheEntry{
		TenantID: tenantID,
		SetTime:  time.Now(),
		LastUsed: time.Now(),
		UseCount: 1,
	}
}

// evictLeastUsed elimina la entrada menos usada
func (tc *TenantCache) evictLeastUsed() {
	var oldestKey string
	var oldestTime time.Time = time.Now()
	
	for key, entry := range tc.data {
		if entry.LastUsed.Before(oldestTime) {
			oldestTime = entry.LastUsed
			oldestKey = key
		}
	}
	
	if oldestKey != "" {
		delete(tc.data, oldestKey)
	}
}

// NewTenantContextManager crea una nueva instancia mejorada del manejador de contexto de tenant
func NewTenantContextManager(db *sqlx.DB, logger Logger, config *TenantConfig) *TenantContextManager {
	if logger == nil {
		logger = &DefaultLogger{}
	}
	if config == nil {
		config = DefaultTenantConfig()
	}

	tcm := &TenantContextManager{
		db:     db,
		logger: logger,
		config: config,
		metrics: &TenantMetrics{
			ActiveTenants:    make(map[string]time.Time),
			TenantOperations: make(map[string]int64),
			OperationHistory: make([]TenantOperation, 0, 100),
		},
	}

	if config.EnableCaching {
		tcm.cache = NewTenantCache(config.CacheTTL, config.MaxConcurrentTenants)
	}

	return tcm
}

// SetTenantContext establece el contexto de tenant en la sesión de base de datos con mejoras
func (tcm *TenantContextManager) SetTenantContext(ctx context.Context, tenantID string) error {
	start := time.Now()
	
	// Validar formato UUID del tenant si está habilitado
	if tcm.config.EnableValidation {
		if err := tcm.validateTenantID(tenantID); err != nil {
			tcm.recordOperation("set", tenantID, false, time.Since(start), err)
			return err
		}
	}

	// Verificar seguridad si está habilitado
	if tcm.config.EnableSecurityChecks {
		if err := tcm.performSecurityChecks(ctx, tenantID); err != nil {
			tcm.recordOperation("set", tenantID, false, time.Since(start), err)
			return err
		}
	}

	// Verificar cache primero
	if tcm.config.EnableCaching && tcm.cache != nil {
		if cachedTenant, exists := tcm.cache.Get("current"); exists && cachedTenant == tenantID {
			tcm.recordCacheHit()
			tcm.recordOperation("set", tenantID, true, time.Since(start), nil)
			return nil
		}
		tcm.recordCacheMiss()
	}

	// Establecer el contexto de tenant en PostgreSQL con timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
	defer cancel()

	query := "SELECT set_tenant_context($1)"
	_, err := tcm.db.ExecContext(ctxWithTimeout, query, tenantID)
	if err != nil {
		tcm.logger.WithField("tenant_id", tenantID).Error(fmt.Sprintf("Failed to set tenant context: %v", err))
		tcm.recordOperation("set", tenantID, false, time.Since(start), err)
		return errors.Wrap(err, "failed to set tenant context")
	}

	// Actualizar cache
	if tcm.config.EnableCaching && tcm.cache != nil {
		tcm.cache.Set("current", tenantID)
	}

	// Registrar métricas
	tcm.recordTenantSet(tenantID)
	tcm.recordOperation("set", tenantID, true, time.Since(start), nil)

	tcm.logger.WithField("tenant_id", tenantID).Info("Tenant context set successfully")

	return nil
}

// validateTenantID valida el formato del tenant ID
func (tcm *TenantContextManager) validateTenantID(tenantID string) error {
	if tenantID == "" {
		tcm.recordValidationFailure()
		return errors.New("tenant ID cannot be empty")
	}

	// Validar formato UUID
	if _, err := uuid.Parse(tenantID); err != nil {
		tcm.recordValidationFailure()
		return errors.Wrapf(err, "invalid tenant ID format: %s", tenantID)
	}

	return nil
}

// performSecurityChecks realiza verificaciones de seguridad
func (tcm *TenantContextManager) performSecurityChecks(ctx context.Context, tenantID string) error {
	// Verificar si el tenant está en la lista de tenants permitidos
	// (En un sistema real, esto consultaría una tabla de tenants)
	
	// Verificar límite de tenants concurrentes
	tcm.metrics.mu.RLock()
	activeCount := len(tcm.metrics.ActiveTenants)
	tcm.metrics.mu.RUnlock()

	if activeCount >= tcm.config.MaxConcurrentTenants {
		tcm.recordSecurityViolation()
		return errors.Errorf("maximum concurrent tenants exceeded: %d", tcm.config.MaxConcurrentTenants)
	}

	return nil
}

// GetCurrentTenant obtiene el tenant_id actual de la sesión de base de datos con mejoras
func (tcm *TenantContextManager) GetCurrentTenant(ctx context.Context) (string, error) {
	start := time.Now()

	// Verificar cache primero
	if tcm.config.EnableCaching && tcm.cache != nil {
		if cachedTenant, exists := tcm.cache.Get("current"); exists {
			tcm.recordCacheHit()
			tcm.recordOperation("get", cachedTenant, true, time.Since(start), nil)
			return cachedTenant, nil
		}
		tcm.recordCacheMiss()
	}

	// Consultar base de datos con timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
	defer cancel()

	var tenantID string
	query := "SELECT get_current_tenant()"
	err := tcm.db.GetContext(ctxWithTimeout, &tenantID, query)
	if err != nil {
		tcm.recordOperation("get", "", false, time.Since(start), err)
		return "", errors.Wrap(err, "failed to get current tenant")
	}

	// Actualizar cache
	if tcm.config.EnableCaching && tcm.cache != nil && tenantID != "" {
		tcm.cache.Set("current", tenantID)
	}

	// Registrar métricas
	tcm.recordTenantGet(tenantID)
	tcm.recordOperation("get", tenantID, true, time.Since(start), nil)

	return tenantID, nil
}

// ClearTenantContext limpia el contexto de tenant de la sesión con mejoras
func (tcm *TenantContextManager) ClearTenantContext(ctx context.Context) error {
	start := time.Now()

	// Obtener tenant actual para logging
	currentTenant, _ := tcm.GetCurrentTenant(ctx)

	// Limpiar contexto con timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
	defer cancel()

	query := "SELECT clear_tenant_context()"
	_, err := tcm.db.ExecContext(ctxWithTimeout, query)
	if err != nil {
		tcm.logger.Error(fmt.Sprintf("Failed to clear tenant context: %v", err))
		tcm.recordOperation("clear", currentTenant, false, time.Since(start), err)
		return errors.Wrap(err, "failed to clear tenant context")
	}

	// Limpiar cache
	if tcm.config.EnableCaching && tcm.cache != nil {
		tcm.cache.mu.Lock()
		delete(tcm.cache.data, "current")
		tcm.cache.mu.Unlock()
	}

	// Registrar métricas
	tcm.recordTenantClear(currentTenant)
	tcm.recordOperation("clear", currentTenant, true, time.Since(start), nil)

	tcm.logger.Info("Tenant context cleared successfully")
	return nil
}

// VerifyRLSStatus verifica que Row Level Security está habilitado con mejoras
func (tcm *TenantContextManager) VerifyRLSStatus(ctx context.Context) ([]RLSStatus, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
	defer cancel()

	query := "SELECT * FROM verify_rls_status()"
	var status []RLSStatus
	err := tcm.db.SelectContext(ctxWithTimeout, &status, query)
	if err != nil {
		return nil, errors.Wrap(err, "failed to verify RLS status")
	}
	return status, nil
}

// RLSStatus representa el estado de Row Level Security para una tabla
type RLSStatus struct {
	DatabaseName string `db:"database_name" json:"database_name"`
	TableName    string `db:"table_name" json:"table_name"`
	RLSEnabled   bool   `db:"rls_enabled" json:"rls_enabled"`
	PolicyCount  int    `db:"policy_count" json:"policy_count"`
}

// WithTenantContext ejecuta una función con contexto de tenant establecido mejorado
func (tcm *TenantContextManager) WithTenantContext(ctx context.Context, tenantID string, fn func(context.Context) error) error {
	// Establecer contexto de tenant
	if err := tcm.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}

	// Crear contexto hijo para cancelación
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Ejecutar función con contexto establecido
	err := fn(childCtx)

	// Limpiar contexto (incluso si hay error)
	if clearErr := tcm.ClearTenantContext(ctx); clearErr != nil {
		tcm.logger.WithField("tenant_id", tenantID).Warn(fmt.Sprintf("Failed to clear tenant context: %v", clearErr))
	}

	return err
}

// TenantAwareQuery ejecuta una consulta con contexto de tenant mejorado
func (tcm *TenantContextManager) TenantAwareQuery(ctx context.Context, tenantID, query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error

	execErr := tcm.WithTenantContext(ctx, tenantID, func(ctx context.Context) error {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
		defer cancel()
		
		rows, err = tcm.db.QueryContext(ctxWithTimeout, query, args...)
		return err
	})

	if execErr != nil {
		return nil, execErr
	}

	return rows, nil
}

// TenantAwareExec ejecuta una declaración SQL con contexto de tenant mejorado
func (tcm *TenantContextManager) TenantAwareExec(ctx context.Context, tenantID, query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	var err error

	execErr := tcm.WithTenantContext(ctx, tenantID, func(ctx context.Context) error {
		ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
		defer cancel()
		
		result, err = tcm.db.ExecContext(ctxWithTimeout, query, args...)
		return err
	})

	if execErr != nil {
		return nil, execErr
	}

	return result, nil
}

// ValidateTenantAccess verifica que el tenant tiene acceso a un recurso específico mejorado
func (tcm *TenantContextManager) ValidateTenantAccess(ctx context.Context, tenantID, resourceTable, resourceID string) (bool, error) {
	// Validar parámetros de entrada
	if tcm.config.EnableValidation {
		if err := tcm.validateTenantID(tenantID); err != nil {
			return false, err
		}
		if resourceTable == "" || resourceID == "" {
			return false, errors.New("resource table and ID cannot be empty")
		}
	}

	query := fmt.Sprintf(`
		SELECT COUNT(*) > 0 as has_access
		FROM %s 
		WHERE id = $1 AND tenant_id = $2
	`, resourceTable)

	// Establecer contexto de tenant
	if err := tcm.SetTenantContext(ctx, tenantID); err != nil {
		return false, err
	}
	defer tcm.ClearTenantContext(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
	defer cancel()

	var hasAccess bool
	err := tcm.db.GetContext(ctxWithTimeout, &hasAccess, query, resourceID, tenantID)
	if err != nil {
		return false, errors.Wrap(err, "failed to validate tenant access")
	}

	return hasAccess, nil
}

// TenantStats proporciona estadísticas sobre los datos del tenant mejorado
type TenantStats struct {
	TenantID    string    `json:"tenant_id"`
	Table       string    `json:"table"`
	Count       int       `json:"count"`
	LastUpdated time.Time `json:"last_updated"`
	Size        int64     `json:"size_bytes"`
}

// GetTenantStats obtiene estadísticas de datos para un tenant específico mejorado
func (tcm *TenantContextManager) GetTenantStats(ctx context.Context, tenantID string, tables []string) ([]TenantStats, error) {
	if tcm.config.EnableValidation {
		if err := tcm.validateTenantID(tenantID); err != nil {
			return nil, err
		}
	}

	var stats []TenantStats

	err := tcm.WithTenantContext(ctx, tenantID, func(ctx context.Context) error {
		for _, table := range tables {
			ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
			
			// Consulta mejorada con información adicional
			query := fmt.Sprintf(`
				SELECT 
					COUNT(*) as count,
					COALESCE(pg_total_relation_size('%s'), 0) as size_bytes,
					NOW() as last_updated
				FROM %s
			`, table, table)
			
			var count int
			var sizeBytes int64
			var lastUpdated time.Time
			
			if err := tcm.db.QueryRowContext(ctxWithTimeout, query).Scan(&count, &sizeBytes, &lastUpdated); err != nil {
				cancel()
				// Si la tabla no existe, continuar con la siguiente
				tcm.logger.WithField("tenant_id", tenantID).Warn(fmt.Sprintf("Failed to get stats for table %s: %v", table, err))
				continue
			}
			cancel()

			stats = append(stats, TenantStats{
				TenantID:    tenantID,
				Table:       table,
				Count:       count,
				LastUpdated: lastUpdated,
				Size:        sizeBytes,
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return stats, nil
}

// HealthCheck verifica que el sistema de multi-tenancy esté funcionando correctamente mejorado
func (tcm *TenantContextManager) HealthCheck(ctx context.Context) error {
	// Verificar que las funciones de RLS existen
	var funcExists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_proc p 
			JOIN pg_namespace n ON p.pronamespace = n.oid 
			WHERE n.nspname = 'public' AND p.proname = 'set_tenant_context'
		)
	`
	
	ctxWithTimeout, cancel := context.WithTimeout(ctx, tcm.config.TenantTimeout)
	defer cancel()
	
	err := tcm.db.GetContext(ctxWithTimeout, &funcExists, query)
	if err != nil {
		return errors.Wrap(err, "failed to check RLS functions")
	}

	if !funcExists {
		return errors.New("RLS functions not found - run setup_row_level_security.sql")
	}

	// Test básico de contexto de tenant
	testTenantID := tcm.config.DefaultTenantID
	if err := tcm.SetTenantContext(ctx, testTenantID); err != nil {
		return errors.Wrap(err, "failed to set test tenant context")
	}

	currentTenant, err := tcm.GetCurrentTenant(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current tenant")
	}

	if currentTenant != testTenantID {
		return errors.Errorf("tenant context mismatch: expected %s, got %s", testTenantID, currentTenant)
	}

	if err := tcm.ClearTenantContext(ctx); err != nil {
		return errors.Wrap(err, "failed to clear test tenant context")
	}

	tcm.logger.Info("Multi-tenancy health check passed successfully")
	return nil
}

// Métodos de métricas

func (tcm *TenantContextManager) recordTenantSet(tenantID string) {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.TenantSets++
	tcm.metrics.TenantOperations[tenantID]++
	tcm.metrics.ActiveTenants[tenantID] = time.Now()
	tcm.metrics.LastOperation = time.Now()
}

func (tcm *TenantContextManager) recordTenantGet(tenantID string) {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.TenantGets++
	tcm.metrics.LastOperation = time.Now()
}

func (tcm *TenantContextManager) recordTenantClear(tenantID string) {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.TenantClears++
	delete(tcm.metrics.ActiveTenants, tenantID)
	tcm.metrics.LastOperation = time.Now()
}

func (tcm *TenantContextManager) recordValidationFailure() {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.ValidationFailures++
}

func (tcm *TenantContextManager) recordSecurityViolation() {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.SecurityViolations++
}

func (tcm *TenantContextManager) recordCacheHit() {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.CacheHits++
}

func (tcm *TenantContextManager) recordCacheMiss() {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.CacheMisses++
}

func (tcm *TenantContextManager) recordOperation(opType, tenantID string, success bool, duration time.Duration, err error) {
	if !tcm.config.EnableMetrics {
		return
	}
	
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	operation := TenantOperation{
		Type:      opType,
		TenantID:  tenantID,
		Success:   success,
		Duration:  duration,
		Timestamp: time.Now(),
	}
	
	if err != nil {
		operation.Error = err.Error()
	}
	
	// Mantener historial de últimas 100 operaciones
	tcm.metrics.OperationHistory = append(tcm.metrics.OperationHistory, operation)
	if len(tcm.metrics.OperationHistory) > 100 {
		tcm.metrics.OperationHistory = tcm.metrics.OperationHistory[1:]
	}
	
	// Actualizar promedios
	switch opType {
	case "set":
		if tcm.metrics.TenantSets > 0 {
			tcm.metrics.AverageSetTime = (tcm.metrics.AverageSetTime*time.Duration(tcm.metrics.TenantSets-1) + duration) / time.Duration(tcm.metrics.TenantSets)
		}
	case "get":
		if tcm.metrics.TenantGets > 0 {
			tcm.metrics.AverageGetTime = (tcm.metrics.AverageGetTime*time.Duration(tcm.metrics.TenantGets-1) + duration) / time.Duration(tcm.metrics.TenantGets)
		}
	}
}

// GetMetrics retorna las métricas del tenant context manager
func (tcm *TenantContextManager) GetMetrics() *TenantMetrics {
	tcm.metrics.mu.RLock()
	defer tcm.metrics.mu.RUnlock()
	
	// Retornar copia de las métricas
	metrics := *tcm.metrics
	
	// Copiar mapas
	metrics.ActiveTenants = make(map[string]time.Time)
	for k, v := range tcm.metrics.ActiveTenants {
		metrics.ActiveTenants[k] = v
	}
	
	metrics.TenantOperations = make(map[string]int64)
	for k, v := range tcm.metrics.TenantOperations {
		metrics.TenantOperations[k] = v
	}
	
	// Copiar historial
	metrics.OperationHistory = make([]TenantOperation, len(tcm.metrics.OperationHistory))
	copy(metrics.OperationHistory, tcm.metrics.OperationHistory)
	
	return &metrics
}

// GetConfig retorna la configuración actual
func (tcm *TenantContextManager) GetConfig() *TenantConfig {
	return tcm.config
}

// GetCacheStats retorna estadísticas del cache
func (tcm *TenantContextManager) GetCacheStats() map[string]interface{} {
	if tcm.cache == nil {
		return map[string]interface{}{
			"enabled": false,
		}
	}
	
	tcm.cache.mu.RLock()
	defer tcm.cache.mu.RUnlock()
	
	totalUseCount := int64(0)
	for _, entry := range tcm.cache.data {
		totalUseCount += entry.UseCount
	}
	
	return map[string]interface{}{
		"enabled":        true,
		"entries":        len(tcm.cache.data),
		"max_size":       tcm.cache.maxSize,
		"ttl":           tcm.cache.ttl,
		"total_uses":     totalUseCount,
		"hit_rate":       tcm.calculateCacheHitRate(),
	}
}

// calculateCacheHitRate calcula la tasa de aciertos del cache
func (tcm *TenantContextManager) calculateCacheHitRate() float64 {
	tcm.metrics.mu.RLock()
	defer tcm.metrics.mu.RUnlock()
	
	total := tcm.metrics.CacheHits + tcm.metrics.CacheMisses
	if total == 0 {
		return 0.0
	}
	
	return float64(tcm.metrics.CacheHits) / float64(total)
}

// ResetMetrics reinicia todas las métricas
func (tcm *TenantContextManager) ResetMetrics() {
	tcm.metrics.mu.Lock()
	defer tcm.metrics.mu.Unlock()
	
	tcm.metrics.TenantSets = 0
	tcm.metrics.TenantGets = 0
	tcm.metrics.TenantClears = 0
	tcm.metrics.ValidationFailures = 0
	tcm.metrics.SecurityViolations = 0
	tcm.metrics.CacheHits = 0
	tcm.metrics.CacheMisses = 0
	tcm.metrics.ActiveTenants = make(map[string]time.Time)
	tcm.metrics.TenantOperations = make(map[string]int64)
	tcm.metrics.AverageSetTime = 0
	tcm.metrics.AverageGetTime = 0
	tcm.metrics.LastOperation = time.Time{}
	tcm.metrics.OperationHistory = make([]TenantOperation, 0, 100)
}