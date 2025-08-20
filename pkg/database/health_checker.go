// Package database - Health Checking System
// Migrado de ClubPulse a gopherkit con mejoras significativas
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// HealthChecker realiza verificaciones de salud de databases mejorado
type HealthChecker struct {
	config  *Config
	metrics *HealthMetrics
	mu      sync.RWMutex
}

// HealthMetrics métricas del health checker
type HealthMetrics struct {
	TotalChecks       int64                    `json:"total_checks"`
	SuccessfulChecks  int64                    `json:"successful_checks"`
	FailedChecks      int64                    `json:"failed_checks"`
	CheckDurations    []time.Duration          `json:"check_durations"`
	ServiceFailures   map[string]int64         `json:"service_failures"`
	LastCheckTime     time.Time                `json:"last_check_time"`
	AverageCheckTime  time.Duration            `json:"average_check_time"`
	HealthTrends      map[string][]HealthPoint `json:"health_trends"`
	mu                sync.RWMutex
}

// HealthPoint representa un punto en el tiempo para tendencias de salud
type HealthPoint struct {
	Timestamp time.Time     `json:"timestamp"`
	Status    HealthStatus  `json:"status"`
	Duration  time.Duration `json:"duration"`
}

// HealthCheckResult resultado mejorado de una verificación de salud
type HealthCheckResult struct {
	Service      string        `json:"service"`
	Status       HealthStatus  `json:"status"`
	ResponseTime time.Duration `json:"response_time"`
	Error        string        `json:"error,omitempty"`
	Details      interface{}   `json:"details,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
	Checks       []CheckDetail `json:"checks,omitempty"`
	Severity     Severity      `json:"severity"`
	Suggestions  []string      `json:"suggestions,omitempty"`
}

// CheckDetail detalle de una verificación específica
type CheckDetail struct {
	Name        string        `json:"name"`
	Status      HealthStatus  `json:"status"`
	Duration    time.Duration `json:"duration"`
	Description string        `json:"description"`
	Error       string        `json:"error,omitempty"`
}

// HealthStatus estado de salud mejorado
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnknown   HealthStatus = "unknown"
	HealthStatusWarning   HealthStatus = "warning"
)

// Severity nivel de severidad
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// NewHealthChecker crea un nuevo verificador de salud mejorado
func NewHealthChecker(config *Config) *HealthChecker {
	return &HealthChecker{
		config: config,
		metrics: &HealthMetrics{
			ServiceFailures: make(map[string]int64),
			HealthTrends:    make(map[string][]HealthPoint),
		},
	}
}

// CheckDatabase verifica la salud de una database específica con mejoras
func (hc *HealthChecker) CheckDatabase(ctx context.Context, db *sql.DB, serviceName string) *HealthCheckResult {
	start := time.Now()
	result := &HealthCheckResult{
		Service:   serviceName,
		Timestamp: start,
		Checks:    make([]CheckDetail, 0),
		Severity:  SeverityInfo,
	}

	// Incrementar contador de checks
	hc.recordCheckStart()

	// Check básico de conectividad
	connectivityCheck := hc.performConnectivityCheck(ctx, db)
	result.Checks = append(result.Checks, connectivityCheck)
	
	if connectivityCheck.Status != HealthStatusHealthy {
		result.Status = HealthStatusUnhealthy
		result.Error = connectivityCheck.Error
		result.Severity = SeverityCritical
		result.Suggestions = []string{
			"Verificar conectividad de red",
			"Revisar configuración de la base de datos",
			"Comprobar estado del servidor de base de datos",
		}
		hc.recordCheckResult(serviceName, false, time.Since(start))
		return result
	}

	// Verificaciones específicas según el servicio
	serviceChecks := hc.performServiceSpecificChecks(ctx, db, serviceName)
	result.Checks = append(result.Checks, serviceChecks...)

	// Verificaciones de performance
	performanceChecks := hc.performPerformanceChecks(ctx, db, serviceName)
	result.Checks = append(result.Checks, performanceChecks...)

	// Determinar estado general y severidad
	result.Status, result.Severity = hc.calculateOverallStatus(result.Checks)
	result.ResponseTime = time.Since(start)

	// Generar sugerencias basadas en los resultados
	result.Suggestions = hc.generateSuggestions(result)

	// Registrar resultado
	hc.recordCheckResult(serviceName, result.Status == HealthStatusHealthy, result.ResponseTime)
	hc.recordHealthTrend(serviceName, result.Status, result.ResponseTime)

	return result
}

// performConnectivityCheck realiza verificación de conectividad básica
func (hc *HealthChecker) performConnectivityCheck(ctx context.Context, db *sql.DB) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        "connectivity",
		Description: "Basic database connectivity test",
	}

	if err := db.PingContext(ctx); err != nil {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("ping failed: %v", err)
	} else {
		check.Status = HealthStatusHealthy
	}

	check.Duration = time.Since(start)
	return check
}

// performServiceSpecificChecks realiza verificaciones específicas del servicio mejoradas
func (hc *HealthChecker) performServiceSpecificChecks(ctx context.Context, db *sql.DB, serviceName string) []CheckDetail {
	var checks []CheckDetail

	switch serviceName {
	case "auth":
		checks = append(checks, hc.checkAuthService(ctx, db)...)
	case "user":
		checks = append(checks, hc.checkUserService(ctx, db)...)
	case "calendar":
		checks = append(checks, hc.checkCalendarService(ctx, db)...)
	case "unified":
		checks = append(checks, hc.checkUnifiedDatabase(ctx, db)...)
	default:
		checks = append(checks, hc.checkGenericService(ctx, db)...)
	}

	return checks
}

// performPerformanceChecks realiza verificaciones de performance
func (hc *HealthChecker) performPerformanceChecks(ctx context.Context, db *sql.DB, serviceName string) []CheckDetail {
	var checks []CheckDetail

	// Verificación de latencia de consulta
	latencyCheck := hc.checkQueryLatency(ctx, db)
	checks = append(checks, latencyCheck)

	// Verificación de conexiones activas
	connectionsCheck := hc.checkActiveConnections(db)
	checks = append(checks, connectionsCheck)

	// Verificación de locks
	locksCheck := hc.checkDatabaseLocks(ctx, db)
	checks = append(checks, locksCheck)

	return checks
}

// checkQueryLatency verifica la latencia de consultas
func (hc *HealthChecker) checkQueryLatency(ctx context.Context, db *sql.DB) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        "query_latency",
		Description: "Database query response time test",
	}

	// Ejecutar consulta simple de prueba
	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("query test failed: %v", err)
	} else {
		duration := time.Since(start)
		check.Duration = duration

		// Determinar estado basado en latencia
		if duration < 100*time.Millisecond {
			check.Status = HealthStatusHealthy
		} else if duration < 500*time.Millisecond {
			check.Status = HealthStatusWarning
		} else if duration < 1*time.Second {
			check.Status = HealthStatusDegraded
		} else {
			check.Status = HealthStatusUnhealthy
		}
	}

	return check
}

// checkActiveConnections verifica el estado de las conexiones activas
func (hc *HealthChecker) checkActiveConnections(db *sql.DB) CheckDetail {
	check := CheckDetail{
		Name:        "active_connections",
		Description: "Database connection pool status",
	}

	stats := db.Stats()
	
	// Calcular porcentaje de uso del pool
	usage := float64(stats.InUse) / float64(stats.MaxOpenConnections) * 100

	details := map[string]interface{}{
		"open_connections": stats.OpenConnections,
		"in_use":          stats.InUse,
		"idle":            stats.Idle,
		"max_open":        stats.MaxOpenConnections,
		"usage_percent":   usage,
	}

	// Determinar estado basado en el uso del pool
	if usage < 70 {
		check.Status = HealthStatusHealthy
	} else if usage < 85 {
		check.Status = HealthStatusWarning
	} else if usage < 95 {
		check.Status = HealthStatusDegraded
	} else {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("connection pool nearly exhausted: %.1f%% usage", usage)
	}

	// Añadir detalles como string formateado
	check.Description = fmt.Sprintf("Connection pool usage: %.1f%% (%d/%d)", usage, stats.InUse, stats.MaxOpenConnections)

	return check
}

// checkDatabaseLocks verifica bloqueos en la base de datos
func (hc *HealthChecker) checkDatabaseLocks(ctx context.Context, db *sql.DB) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        "database_locks",
		Description: "Database lock status check",
	}

	// Consulta para verificar locks bloqueantes
	query := `
		SELECT COUNT(*) 
		FROM pg_locks 
		WHERE NOT granted AND locktype = 'relation'
	`

	var lockCount int
	if err := db.QueryRowContext(ctx, query).Scan(&lockCount); err != nil {
		check.Status = HealthStatusWarning
		check.Error = fmt.Sprintf("could not check locks: %v", err)
	} else {
		check.Duration = time.Since(start)
		
		if lockCount == 0 {
			check.Status = HealthStatusHealthy
		} else if lockCount < 5 {
			check.Status = HealthStatusWarning
			check.Error = fmt.Sprintf("%d pending locks detected", lockCount)
		} else {
			check.Status = HealthStatusDegraded
			check.Error = fmt.Sprintf("%d pending locks detected - potential deadlock situation", lockCount)
		}
	}

	return check
}

// checkAuthService verificaciones específicas del servicio auth mejoradas
func (hc *HealthChecker) checkAuthService(ctx context.Context, db *sql.DB) []CheckDetail {
	var checks []CheckDetail

	// Verificar tablas críticas
	tables := []string{"users", "refresh_tokens", "email_verification_tokens"}
	for _, table := range tables {
		check := hc.checkTableExists(ctx, db, table)
		checks = append(checks, check)
	}

	// Verificar índices críticos
	indexes := []string{"idx_users_email", "idx_refresh_tokens_token"}
	for _, index := range indexes {
		check := hc.checkIndexExists(ctx, db, index)
		checks = append(checks, check)
	}

	return checks
}

// checkUserService verificaciones específicas del servicio user mejoradas
func (hc *HealthChecker) checkUserService(ctx context.Context, db *sql.DB) []CheckDetail {
	var checks []CheckDetail

	tables := []string{"users", "user_profiles", "user_sports"}
	for _, table := range tables {
		check := hc.checkTableExists(ctx, db, table)
		checks = append(checks, check)
	}

	return checks
}

// checkCalendarService verificaciones específicas del servicio calendar mejoradas
func (hc *HealthChecker) checkCalendarService(ctx context.Context, db *sql.DB) []CheckDetail {
	var checks []CheckDetail

	tables := []string{"reservations", "courts", "facilities"}
	for _, table := range tables {
		check := hc.checkTableExists(ctx, db, table)
		checks = append(checks, check)
	}

	return checks
}

// checkUnifiedDatabase verificaciones para la database unificada mejoradas
func (hc *HealthChecker) checkUnifiedDatabase(ctx context.Context, db *sql.DB) []CheckDetail {
	var checks []CheckDetail

	// Verificar que los schemas existen
	schemas := []string{"auth_schema", "user_schema", "calendar_schema", "championship_schema", "membership_schema"}
	for _, schema := range schemas {
		check := hc.checkSchemaExists(ctx, db, schema)
		checks = append(checks, check)
	}

	// Verificar funciones RLS
	rlsFunctions := []string{"set_tenant_context", "get_current_tenant", "verify_unified_rls_status"}
	for _, fn := range rlsFunctions {
		check := hc.checkFunctionExists(ctx, db, fn)
		checks = append(checks, check)
	}

	return checks
}

// checkGenericService verificaciones genéricas mejoradas
func (hc *HealthChecker) checkGenericService(ctx context.Context, db *sql.DB) []CheckDetail {
	var checks []CheckDetail

	// Verificación básica: contar tablas
	check := CheckDetail{
		Name:        "table_count",
		Description: "Verify database has tables",
	}

	var tableCount int
	query := "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'"
	if err := db.QueryRowContext(ctx, query).Scan(&tableCount); err != nil {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("failed to count tables: %v", err)
	} else if tableCount == 0 {
		check.Status = HealthStatusUnhealthy
		check.Error = "no tables found in database"
	} else {
		check.Status = HealthStatusHealthy
		check.Description = fmt.Sprintf("Found %d tables", tableCount)
	}

	checks = append(checks, check)
	return checks
}

// Helper methods para verificaciones específicas

func (hc *HealthChecker) checkTableExists(ctx context.Context, db *sql.DB, tableName string) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        fmt.Sprintf("table_%s", tableName),
		Description: fmt.Sprintf("Verify table %s exists", tableName),
	}

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1 AND table_schema = 'public')"
	if err := db.QueryRowContext(ctx, query, tableName).Scan(&exists); err != nil {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("failed to check table %s: %v", tableName, err)
	} else if !exists {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("table %s does not exist", tableName)
	} else {
		check.Status = HealthStatusHealthy
	}

	check.Duration = time.Since(start)
	return check
}

func (hc *HealthChecker) checkIndexExists(ctx context.Context, db *sql.DB, indexName string) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        fmt.Sprintf("index_%s", indexName),
		Description: fmt.Sprintf("Verify index %s exists", indexName),
	}

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)"
	if err := db.QueryRowContext(ctx, query, indexName).Scan(&exists); err != nil {
		check.Status = HealthStatusWarning
		check.Error = fmt.Sprintf("failed to check index %s: %v", indexName, err)
	} else if !exists {
		check.Status = HealthStatusWarning
		check.Error = fmt.Sprintf("index %s does not exist", indexName)
	} else {
		check.Status = HealthStatusHealthy
	}

	check.Duration = time.Since(start)
	return check
}

func (hc *HealthChecker) checkSchemaExists(ctx context.Context, db *sql.DB, schemaName string) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        fmt.Sprintf("schema_%s", schemaName),
		Description: fmt.Sprintf("Verify schema %s exists", schemaName),
	}

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)"
	if err := db.QueryRowContext(ctx, query, schemaName).Scan(&exists); err != nil {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("failed to check schema %s: %v", schemaName, err)
	} else if !exists {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("schema %s does not exist", schemaName)
	} else {
		check.Status = HealthStatusHealthy
	}

	check.Duration = time.Since(start)
	return check
}

func (hc *HealthChecker) checkFunctionExists(ctx context.Context, db *sql.DB, functionName string) CheckDetail {
	start := time.Now()
	check := CheckDetail{
		Name:        fmt.Sprintf("function_%s", functionName),
		Description: fmt.Sprintf("Verify function %s exists", functionName),
	}

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_proc WHERE proname = $1)"
	if err := db.QueryRowContext(ctx, query, functionName).Scan(&exists); err != nil {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("failed to check function %s: %v", functionName, err)
	} else if !exists {
		check.Status = HealthStatusUnhealthy
		check.Error = fmt.Sprintf("function %s does not exist", functionName)
	} else {
		check.Status = HealthStatusHealthy
	}

	check.Duration = time.Since(start)
	return check
}

// calculateOverallStatus calcula el estado general basado en todas las verificaciones
func (hc *HealthChecker) calculateOverallStatus(checks []CheckDetail) (HealthStatus, Severity) {
	if len(checks) == 0 {
		return HealthStatusUnknown, SeverityInfo
	}

	healthyCount := 0
	warningCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, check := range checks {
		switch check.Status {
		case HealthStatusHealthy:
			healthyCount++
		case HealthStatusWarning:
			warningCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnhealthy:
			unhealthyCount++
		}
	}

	totalChecks := len(checks)

	// Determinar estado y severidad
	if unhealthyCount > 0 {
		if unhealthyCount >= totalChecks/2 {
			return HealthStatusUnhealthy, SeverityCritical
		}
		return HealthStatusDegraded, SeverityHigh
	}

	if degradedCount > 0 {
		if degradedCount >= totalChecks/3 {
			return HealthStatusDegraded, SeverityMedium
		}
		return HealthStatusWarning, SeverityLow
	}

	if warningCount > 0 {
		return HealthStatusWarning, SeverityLow
	}

	return HealthStatusHealthy, SeverityInfo
}

// generateSuggestions genera sugerencias basadas en los resultados
func (hc *HealthChecker) generateSuggestions(result *HealthCheckResult) []string {
	var suggestions []string

	switch result.Status {
	case HealthStatusUnhealthy:
		suggestions = append(suggestions, "🚨 Sistema en estado crítico - requiere atención inmediata")
		suggestions = append(suggestions, "📞 Contactar al equipo de operaciones")
		suggestions = append(suggestions, "🔄 Considerar activación de fallback automático")
		
		// Sugerencias específicas basadas en checks fallidos
		for _, check := range result.Checks {
			if check.Status == HealthStatusUnhealthy {
				if strings.Contains(check.Name, "connectivity") {
					suggestions = append(suggestions, "🌐 Verificar conectividad de red y estado del servidor")
				} else if strings.Contains(check.Name, "table") {
					suggestions = append(suggestions, "🗃️ Ejecutar migraciones de base de datos")
				} else if strings.Contains(check.Name, "schema") {
					suggestions = append(suggestions, "📋 Verificar esquemas de base de datos")
				}
			}
		}

	case HealthStatusDegraded:
		suggestions = append(suggestions, "⚠️ Sistema degradado - monitorear de cerca")
		suggestions = append(suggestions, "🔍 Investigar servicios con problemas específicos")
		suggestions = append(suggestions, "📊 Revisar métricas de performance")

	case HealthStatusWarning:
		suggestions = append(suggestions, "🟡 Advertencias detectadas - revisar cuando sea posible")
		suggestions = append(suggestions, "📈 Monitorear tendencias de performance")

	case HealthStatusHealthy:
		suggestions = append(suggestions, "✅ Sistema funcionando correctamente")
		suggestions = append(suggestions, "📈 Continuar con monitoreo regular")
	}

	return suggestions
}

// Métodos de métricas

func (hc *HealthChecker) recordCheckStart() {
	hc.metrics.mu.Lock()
	defer hc.metrics.mu.Unlock()
	hc.metrics.TotalChecks++
	hc.metrics.LastCheckTime = time.Now()
}

func (hc *HealthChecker) recordCheckResult(serviceName string, success bool, duration time.Duration) {
	hc.metrics.mu.Lock()
	defer hc.metrics.mu.Unlock()

	if success {
		hc.metrics.SuccessfulChecks++
	} else {
		hc.metrics.FailedChecks++
		hc.metrics.ServiceFailures[serviceName]++
	}

	// Registrar duración (mantener últimas 100)
	hc.metrics.CheckDurations = append(hc.metrics.CheckDurations, duration)
	if len(hc.metrics.CheckDurations) > 100 {
		hc.metrics.CheckDurations = hc.metrics.CheckDurations[1:]
	}

	// Calcular promedio
	if len(hc.metrics.CheckDurations) > 0 {
		var total time.Duration
		for _, d := range hc.metrics.CheckDurations {
			total += d
		}
		hc.metrics.AverageCheckTime = total / time.Duration(len(hc.metrics.CheckDurations))
	}
}

func (hc *HealthChecker) recordHealthTrend(serviceName string, status HealthStatus, duration time.Duration) {
	hc.metrics.mu.Lock()
	defer hc.metrics.mu.Unlock()

	point := HealthPoint{
		Timestamp: time.Now(),
		Status:    status,
		Duration:  duration,
	}

	if hc.metrics.HealthTrends[serviceName] == nil {
		hc.metrics.HealthTrends[serviceName] = make([]HealthPoint, 0)
	}

	hc.metrics.HealthTrends[serviceName] = append(hc.metrics.HealthTrends[serviceName], point)

	// Mantener solo últimos 50 puntos por servicio
	if len(hc.metrics.HealthTrends[serviceName]) > 50 {
		hc.metrics.HealthTrends[serviceName] = hc.metrics.HealthTrends[serviceName][1:]
	}
}

// GetMetrics retorna las métricas del health checker
func (hc *HealthChecker) GetMetrics() *HealthMetrics {
	hc.metrics.mu.RLock()
	defer hc.metrics.mu.RUnlock()
	
	// Retornar copia de las métricas
	metrics := *hc.metrics
	return &metrics
}

// CheckAllServices verifica la salud de todos los servicios con mejoras
func (hc *HealthChecker) CheckAllServices(ctx context.Context) ([]*HealthCheckResult, error) {
	var results []*HealthCheckResult

	if hc.config.DatabaseMode == ModeSingle {
		// Verificar database unificada
		if db, err := sql.Open("postgres", hc.config.DatabaseURL); err == nil {
			defer db.Close()
			result := hc.CheckDatabase(ctx, db, "unified")
			results = append(results, result)
		}
	} else {
		// Verificar databases individuales
		services := []struct {
			name string
			port string
		}{
			{"auth", "5433"},
			{"user", "5434"},
			{"calendar", "5435"},
			{"championship", "5436"},
			{"membership", "5437"},
			{"facilities", "5439"},
			{"notification", "5440"},
			{"booking", "5437"},
			{"payments", "5441"},
			{"super-admin", "5442"},
		}

		for _, service := range services {
			dbURL := fmt.Sprintf("postgresql://postgres:password@localhost:%s/%s_db?sslmode=disable", service.port, service.name)
			if db, err := sql.Open("postgres", dbURL); err == nil {
				result := hc.CheckDatabase(ctx, db, service.name)
				results = append(results, result)
				db.Close()
			} else {
				results = append(results, &HealthCheckResult{
					Service:   service.name,
					Status:    HealthStatusUnhealthy,
					Error:     fmt.Sprintf("failed to connect: %v", err),
					Timestamp: time.Now(),
					Severity:  SeverityCritical,
				})
			}
		}
	}

	return results, nil
}

// GetOverallHealth calcula el estado general de salud mejorado
func (hc *HealthChecker) GetOverallHealth(results []*HealthCheckResult) (HealthStatus, Severity) {
	if len(results) == 0 {
		return HealthStatusUnknown, SeverityInfo
	}

	healthyCount := 0
	warningCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, result := range results {
		switch result.Status {
		case HealthStatusHealthy:
			healthyCount++
		case HealthStatusWarning:
			warningCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnhealthy:
			unhealthyCount++
		}
	}

	totalServices := len(results)
	
	// Determinar estado general con lógica mejorada
	if unhealthyCount > 0 {
		if unhealthyCount >= totalServices/2 {
			return HealthStatusUnhealthy, SeverityCritical
		}
		return HealthStatusDegraded, SeverityHigh
	}

	if degradedCount > 0 {
		if degradedCount >= totalServices/3 {
			return HealthStatusDegraded, SeverityMedium
		}
		return HealthStatusWarning, SeverityLow
	}

	if warningCount > 0 {
		return HealthStatusWarning, SeverityLow
	}

	return HealthStatusHealthy, SeverityInfo
}

// CheckWithRetry verifica con reintentos mejorado
func (hc *HealthChecker) CheckWithRetry(ctx context.Context, db *sql.DB, serviceName string, maxRetries int) *HealthCheckResult {
	var lastResult *HealthCheckResult

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			// Esperar antes del reintento con backoff exponencial
			backoff := time.Duration(i) * time.Second
			select {
			case <-ctx.Done():
				return &HealthCheckResult{
					Service:   serviceName,
					Status:    HealthStatusUnhealthy,
					Error:     "context cancelled during retry",
					Timestamp: time.Now(),
					Severity:  SeverityCritical,
				}
			case <-time.After(backoff):
			}
		}

		result := hc.CheckDatabase(ctx, db, serviceName)
		lastResult = result

		if result.Status == HealthStatusHealthy {
			return result
		}
	}

	// Agregar información sobre los reintentos fallidos
	if lastResult != nil {
		lastResult.Error = fmt.Sprintf("failed after %d retries: %s", maxRetries, lastResult.Error)
	}

	return lastResult
}

// GenerateHealthReport genera un reporte completo de salud mejorado
func (hc *HealthChecker) GenerateHealthReport(ctx context.Context) (HealthReport, error) {
	results, err := hc.CheckAllServices(ctx)
	if err != nil {
		return HealthReport{}, errors.Wrap(err, "failed to check all services")
	}

	overallHealth, overallSeverity := hc.GetOverallHealth(results)

	report := HealthReport{
		Timestamp:       time.Now(),
		DatabaseMode:    string(hc.config.DatabaseMode),
		OverallHealth:   overallHealth,
		OverallSeverity: overallSeverity,
		Services:        results,
		Summary: HealthSummary{
			TotalServices:     len(results),
			HealthyServices:   countByStatus(results, HealthStatusHealthy),
			WarningServices:   countByStatus(results, HealthStatusWarning),
			DegradedServices:  countByStatus(results, HealthStatusDegraded),
			UnhealthyServices: countByStatus(results, HealthStatusUnhealthy),
		},
		Metrics: hc.GetMetrics(),
	}

	// Agregar recomendaciones
	report.Recommendations = hc.generateReportRecommendations(results, overallHealth)

	return report, nil
}

// HealthReport estructura mejorada del reporte de salud
type HealthReport struct {
	Timestamp       time.Time            `json:"timestamp"`
	DatabaseMode    string               `json:"database_mode"`
	OverallHealth   HealthStatus         `json:"overall_health"`
	OverallSeverity Severity             `json:"overall_severity"`
	Services        []*HealthCheckResult `json:"services"`
	Summary         HealthSummary        `json:"summary"`
	Recommendations []string             `json:"recommendations"`
	Metrics         *HealthMetrics       `json:"metrics"`
}

// HealthSummary resumen mejorado del estado de salud
type HealthSummary struct {
	TotalServices     int `json:"total_services"`
	HealthyServices   int `json:"healthy_services"`
	WarningServices   int `json:"warning_services"`
	DegradedServices  int `json:"degraded_services"`
	UnhealthyServices int `json:"unhealthy_services"`
}

// countByStatus cuenta servicios por estado
func countByStatus(results []*HealthCheckResult, status HealthStatus) int {
	count := 0
	for _, result := range results {
		if result.Status == status {
			count++
		}
	}
	return count
}

// generateReportRecommendations genera recomendaciones para el reporte completo
func (hc *HealthChecker) generateReportRecommendations(results []*HealthCheckResult, overallHealth HealthStatus) []string {
	var recommendations []string

	switch overallHealth {
	case HealthStatusUnhealthy:
		recommendations = append(recommendations, "🚨 Sistema en estado crítico - requiere intervención inmediata")
		recommendations = append(recommendations, "📞 Alertar al equipo de operaciones y oncall")
		recommendations = append(recommendations, "🔄 Considerar activación de procedimientos de emergencia")
		recommendations = append(recommendations, "📋 Revisar runbooks de recuperación de desastres")

	case HealthStatusDegraded:
		recommendations = append(recommendations, "⚠️ Sistema degradado - monitorear activamente")
		recommendations = append(recommendations, "🔍 Investigar servicios específicos con problemas")
		recommendations = append(recommendations, "📊 Revisar métricas de performance y tendencias")
		recommendations = append(recommendations, "🛠️ Planificar mantenimiento preventivo")

	case HealthStatusWarning:
		recommendations = append(recommendations, "🟡 Advertencias detectadas - programar revisión")
		recommendations = append(recommendations, "📈 Monitorear tendencias y configurar alertas")
		recommendations = append(recommendations, "🔧 Considerar optimizaciones proactivas")

	case HealthStatusHealthy:
		recommendations = append(recommendations, "✅ Todos los sistemas funcionando óptimamente")
		recommendations = append(recommendations, "📈 Continuar con monitoreo regular y mejora continua")
		recommendations = append(recommendations, "🎯 Revisar objetivos de SLA y KPIs")
	}

	// Recomendaciones específicas por servicios problemáticos
	problemServices := 0
	for _, result := range results {
		if result.Status != HealthStatusHealthy {
			problemServices++
			if problemServices <= 3 { // Limitar a 3 recomendaciones específicas
				recommendations = append(recommendations, 
					fmt.Sprintf("🔧 %s: %s", result.Service, result.Error))
			}
		}
	}

	if problemServices > 3 {
		recommendations = append(recommendations, 
			fmt.Sprintf("📋 Y %d servicios adicionales requieren atención", problemServices-3))
	}

	return recommendations
}