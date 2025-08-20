package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

// NewChecker crea un nuevo health checker agregado
func NewChecker(service, version string, opts ...CheckerOption) *Checker {
	checker := &Checker{
		checkers:  make([]HealthChecker, 0),
		service:   service,
		version:   version,
		startTime: time.Now(),
		systemInfo: SystemInfo{
			Version:   version,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			StartTime: time.Now(),
		},
	}
	
	for _, opt := range opts {
		opt(checker)
	}
	
	return checker
}

// WithEnvironment configura el environment
func WithEnvironment(env string) CheckerOption {
	return func(c *Checker) {
		c.environment = env
		c.systemInfo.Environment = env
	}
}

// WithBuildInfo configura información de build
func WithBuildInfo(buildTime, gitCommit string) CheckerOption {
	return func(c *Checker) {
		c.systemInfo.BuildTime = buildTime
		c.systemInfo.GitCommit = gitCommit
	}
}

// AddChecker añade un health checker
func (c *Checker) AddChecker(checker HealthChecker) {
	c.checkers = append(c.checkers, checker)
}

// Check ejecuta todos los health checks
func (c *Checker) Check(ctx context.Context) HealthResponse {
	startTime := time.Now()
	
	// Actualizar información del sistema
	c.systemInfo.Uptime = time.Since(c.startTime).String()
	
	response := HealthResponse{
		Service:     c.service,
		Version:     c.version,
		Timestamp:   startTime,
		Uptime:      time.Since(c.startTime),
		Components:  make(map[string]ComponentHealth),
		SystemInfo:  c.systemInfo,
		Environment: c.environment,
		RequestID:   uuid.New().String(),
	}
	
	if len(c.checkers) == 0 {
		response.Status = StatusHealthy
		return response
	}
	
	// Ejecutar checks en paralelo
	results := c.runChecksParallel(ctx)
	
	// Procesar resultados
	for _, result := range results {
		response.Components[result.Name] = result.Health
		
		if result.Error != nil {
			logrus.WithFields(logrus.Fields{
				"component": result.Name,
				"error":     result.Error,
				"duration":  result.Health.Duration,
			}).Warn("Health check failed")
		}
	}
	
	// Calcular estado general
	response.Status = OverallStatus(response.Components)
	
	logrus.WithFields(logrus.Fields{
		"service":         c.service,
		"overall_status":  response.Status,
		"components":      len(response.Components),
		"check_duration":  time.Since(startTime),
	}).Debug("Health check completed")
	
	return response
}

// runChecksParallel ejecuta todos los health checks en paralelo
func (c *Checker) runChecksParallel(ctx context.Context) []CheckResult {
	results := make([]CheckResult, len(c.checkers))
	var wg sync.WaitGroup
	
	for i, checker := range c.checkers {
		wg.Add(1)
		go func(index int, hc HealthChecker) {
			defer wg.Done()
			
			// Crear contexto con timeout específico para este checker
			checkCtx, cancel := context.WithTimeout(ctx, hc.Timeout())
			defer cancel()
			
			startTime := time.Now()
			
			// Ejecutar el check con recover para evitar panics
			var health ComponentHealth
			var err error
			
			func() {
				defer func() {
					if r := recover(); r != nil {
						health = NewComponentHealth(StatusUnhealthy, "Health check panicked")
						health = health.WithMetadata("panic", r)
						err = nil // No propagamos el panic como error
					}
				}()
				
				health = hc.Check(checkCtx)
			}()
			
			// Asegurar que la duración esté establecida
			if health.Duration == 0 {
				health.Duration = time.Since(startTime)
			}
			
			results[index] = CheckResult{
				Name:   hc.Name(),
				Health: health,
				Error:  err,
			}
			
		}(i, checker)
	}
	
	wg.Wait()
	return results
}

// IsHealthy verifica si el servicio está saludable
func (c *Checker) IsHealthy(ctx context.Context) bool {
	response := c.Check(ctx)
	return response.Status == StatusHealthy
}

// IsReady verifica si el servicio está listo (readiness probe)
func (c *Checker) IsReady(ctx context.Context) bool {
	response := c.Check(ctx)
	// El servicio está listo si no está completamente unhealthy
	return response.Status != StatusUnhealthy
}

// IsLive verifica si el servicio está vivo (liveness probe)
func (c *Checker) IsLive(ctx context.Context) bool {
	// Liveness probe simple - solo verifica que la aplicación responda
	// No ejecuta health checks complejos para evitar reiniciar por problemas temporales
	return true
}

// GetComponentStatus obtiene el estado de un componente específico
func (c *Checker) GetComponentStatus(ctx context.Context, componentName string) (ComponentHealth, bool) {
	for _, checker := range c.checkers {
		if checker.Name() == componentName {
			health := checker.Check(ctx)
			return health, true
		}
	}
	
	return ComponentHealth{}, false
}

// ListComponents retorna la lista de nombres de componentes
func (c *Checker) ListComponents() []string {
	names := make([]string, len(c.checkers))
	for i, checker := range c.checkers {
		names[i] = checker.Name()
	}
	return names
}

// GetServiceInfo retorna información básica del servicio
func (c *Checker) GetServiceInfo() map[string]interface{} {
	return map[string]interface{}{
		"service":     c.service,
		"version":     c.version,
		"uptime":      time.Since(c.startTime).String(),
		"start_time":  c.startTime,
		"environment": c.environment,
		"components":  len(c.checkers),
	}
}

// === NUEVAS FUNCIONALIDADES AVANZADAS PARA GOPHERKIT ===

// Status representa el estado de salud de un componente con estados extendidos
type AdvancedStatus string

const (
	AdvancedStatusHealthy     AdvancedStatus = "healthy"
	AdvancedStatusUnhealthy   AdvancedStatus = "unhealthy"
	AdvancedStatusDegraded    AdvancedStatus = "degraded"
	AdvancedStatusUnknown     AdvancedStatus = "unknown"
	AdvancedStatusMaintenance AdvancedStatus = "maintenance"
	AdvancedStatusWarning     AdvancedStatus = "warning"
	AdvancedStatusCritical    AdvancedStatus = "critical"
	AdvancedStatusRecovering  AdvancedStatus = "recovering"
)

// Severity define la severidad de un problema de salud
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
)

// AdvancedCheckResult representa el resultado de un health check con información extendida
type AdvancedCheckResult struct {
	Name        string                 `json:"name"`
	Status      AdvancedStatus         `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Critical    bool                   `json:"critical"`
	Timeout     time.Duration          `json:"timeout"`
	LastSuccess time.Time              `json:"last_success,omitempty"`
	
	// Campos extendidos para gopherkit
	Severity        Severity               `json:"severity"`
	Component       string                 `json:"component"`
	Category        string                 `json:"category"`
	Recommendations []string               `json:"recommendations,omitempty"`
	Dependencies    []string               `json:"dependencies,omitempty"`
	Metrics         map[string]float64     `json:"metrics,omitempty"`
	Trend           HealthTrend            `json:"trend,omitempty"`
	RetryCount      int                    `json:"retry_count,omitempty"`
	MaxRetries      int                    `json:"max_retries,omitempty"`
}

// HealthTrend representa la tendencia de salud de un componente
type HealthTrend struct {
	Direction     string    `json:"direction"` // "improving", "declining", "stable"
	Confidence    float64   `json:"confidence"`
	DataPoints    int       `json:"data_points"`
	LastUpdated   time.Time `json:"last_updated"`
	TrendMetrics  map[string]float64 `json:"trend_metrics,omitempty"`
}

// AdvancedHealthResult representa el resultado general de health checks con funcionalidades avanzadas
type AdvancedHealthResult struct {
	Status    AdvancedStatus                    `json:"status"`
	Timestamp time.Time                         `json:"timestamp"`
	Duration  time.Duration                     `json:"duration"`
	Checks    map[string]*AdvancedCheckResult   `json:"checks"`
	Metadata  map[string]interface{}            `json:"metadata,omitempty"`
	Version   string                            `json:"version,omitempty"`
	Service   string                            `json:"service"`
	Mode      string                            `json:"mode"`
	
	// Campos extendidos
	Environment      string                 `json:"environment"`
	OverallSeverity  Severity               `json:"overall_severity"`
	Summary          AdvancedHealthSummary  `json:"summary"`
	SystemInfo       AdvancedSystemInfo     `json:"system_info"`
	Alerts           []HealthAlert          `json:"alerts,omitempty"`
	Recommendations  []string               `json:"recommendations,omitempty"`
	TrendAnalysis    map[string]HealthTrend `json:"trend_analysis,omitempty"`
}

// AdvancedHealthSummary proporciona un resumen ejecutivo del estado de salud
type AdvancedHealthSummary struct {
	TotalChecks    int                    `json:"total_checks"`
	HealthyChecks  int                    `json:"healthy_checks"`
	CriticalIssues int                    `json:"critical_issues"`
	Warnings       int                    `json:"warnings"`
	ByCategory     map[string]int         `json:"by_category"`
	BySeverity     map[Severity]int       `json:"by_severity"`
	UpTime         time.Duration          `json:"uptime"`
	LastRestart    time.Time              `json:"last_restart,omitempty"`
}

// AdvancedSystemInfo proporciona información extendida del sistema
type AdvancedSystemInfo struct {
	GoVersion      string            `json:"go_version"`
	Runtime        RuntimeInfo       `json:"runtime"`
	Dependencies   map[string]string `json:"dependencies"`
	Configuration  map[string]string `json:"configuration"`
	Resources      ResourceUsage     `json:"resources"`
	Capabilities   []string          `json:"capabilities"`
}

// RuntimeInfo información del runtime de Go
type RuntimeInfo struct {
	NumGoroutines int               `json:"num_goroutines"`
	NumCPU        int               `json:"num_cpu"`
	MemStats      runtime.MemStats  `json:"mem_stats"`
	GCStats       GCStats           `json:"gc_stats"`
}

// GCStats estadísticas del garbage collector
type GCStats struct {
	NextGC       uint64        `json:"next_gc"`
	LastGC       time.Time     `json:"last_gc"`
	PauseTotalNs uint64        `json:"pause_total_ns"`
	NumGC        uint32        `json:"num_gc"`
	GCPercent    int           `json:"gc_percent"`
}

// ResourceUsage información de uso de recursos
type ResourceUsage struct {
	MemoryUsageMB    float64   `json:"memory_usage_mb"`
	CPUUsagePercent  float64   `json:"cpu_usage_percent"`
	DiskUsageMB      float64   `json:"disk_usage_mb"`
	NetworkBytesIn   uint64    `json:"network_bytes_in"`
	NetworkBytesOut  uint64    `json:"network_bytes_out"`
	LastUpdated      time.Time `json:"last_updated"`
}

// HealthAlert representa una alerta de salud
type HealthAlert struct {
	ID          string                 `json:"id"`
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	Component   string                 `json:"component"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AdvancedHealthChecker realiza health checks con funcionalidades avanzadas
type AdvancedHealthChecker struct {
	config     *AdvancedCheckerConfig
	db         *sql.DB
	redis      *redis.Client
	checks     map[string]AdvancedCheckFunc
	mu         sync.RWMutex
	lastResult *AdvancedHealthResult
	
	// Funcionalidades avanzadas
	trendAnalyzer    *TrendAnalyzer
	alertManager     *monitoring.AlertManager
	metricsCollector *HealthMetricsCollector
	
	// Estado interno
	startTime       time.Time
	activeAlerts    map[string]*HealthAlert
	lastSystemInfo  AdvancedSystemInfo
	
	// Configuración de comportamiento
	enableTrendAnalysis   bool
	enableSystemInfo      bool
	enableRecommendations bool
	maxHistoryEntries     int
	
	// Canales para operaciones asíncronas
	shutdownCh     chan struct{}
	backgroundWG   sync.WaitGroup
	
	// Métricas atómicas
	totalChecks    int64
	failedChecks   int64
	lastCheckTime  int64
	
	// Pool de workers para checks paralelos
	workerPool chan struct{}
}

// AdvancedCheckFunc representa una función de health check mejorada
type AdvancedCheckFunc func(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult

// AdvancedCheckerConfig mantiene la configuración del health checker avanzado
type AdvancedCheckerConfig struct {
	ServiceName    string
	DatabaseMode   string
	DatabaseSchema string
	DatabaseURL    string
	RedisURL       string
	RedisNamespace string
	DefaultTimeout time.Duration
	CriticalChecks []string
	Metadata       map[string]interface{}
	
	// Configuración extendida
	Environment           string
	EnableTrendAnalysis   bool
	EnableSystemInfo      bool
	EnableRecommendations bool
	EnablePrometheusMetrics bool
	MaxHistoryEntries     int
	CheckInterval         time.Duration
	ParallelCheckLimit    int
	
	// Integración con gopherkit
	BaseConfig            *config.BaseConfig
	AlertManager          *monitoring.AlertManager
	// PrometheusIntegration *monitoring.PrometheusIntegration // TODO: implement when available
}

// TrendAnalyzer analiza tendencias de salud a lo largo del tiempo
type TrendAnalyzer struct {
	dataPoints     map[string][]TrendDataPoint
	maxDataPoints  int
	analysisWindow time.Duration
	mu             sync.RWMutex
}

// TrendDataPoint representa un punto de datos para análisis de tendencias
type TrendDataPoint struct {
	Timestamp time.Time
	Status    AdvancedStatus
	Duration  time.Duration
	Metrics   map[string]float64
}

// HealthMetricsCollector recopila métricas de salud para Prometheus
type HealthMetricsCollector struct {
	healthStatus        *prometheus.GaugeVec
	checkDuration       *prometheus.HistogramVec
	checkTotal          *prometheus.CounterVec
	systemInfo          *prometheus.GaugeVec
	alertsActive        prometheus.Gauge
	uptimeSeconds       prometheus.Gauge
	mu                  sync.Mutex
}

// NewAdvancedHealthChecker crea un nuevo health checker con funcionalidades avanzadas
func NewAdvancedHealthChecker(config *AdvancedCheckerConfig, db *sql.DB, redisClient *redis.Client) *AdvancedHealthChecker {
	if config == nil {
		config = LoadAdvancedDefaultConfig()
	}

	applyAdvancedDefaultConfig(config)

	checker := &AdvancedHealthChecker{
		config:         config,
		db:            db,
		redis:         redisClient,
		checks:        make(map[string]AdvancedCheckFunc),
		startTime:     time.Now(),
		activeAlerts:  make(map[string]*HealthAlert),
		shutdownCh:    make(chan struct{}),
		
		enableTrendAnalysis:   config.EnableTrendAnalysis,
		enableSystemInfo:      config.EnableSystemInfo,
		enableRecommendations: config.EnableRecommendations,
		maxHistoryEntries:     config.MaxHistoryEntries,
		workerPool:           make(chan struct{}, config.ParallelCheckLimit),
	}

	// Inicializar componentes avanzados
	if config.EnableTrendAnalysis {
		checker.trendAnalyzer = NewTrendAnalyzer(100, 24*time.Hour)
	}

	if config.EnablePrometheusMetrics {
		checker.metricsCollector = NewHealthMetricsCollector(config.ServiceName)
	}

	if config.AlertManager != nil {
		checker.alertManager = config.AlertManager
	}

	// Registrar checks por defecto
	checker.registerAdvancedDefaultChecks()

	// Iniciar servicios en background
	checker.startBackgroundServices()

	logrus.WithFields(logrus.Fields{
		"service":              config.ServiceName,
		"trend_analysis":       config.EnableTrendAnalysis,
		"system_info":          config.EnableSystemInfo,
		"prometheus_metrics":   config.EnablePrometheusMetrics,
		"parallel_check_limit": config.ParallelCheckLimit,
	}).Info("Advanced Health Checker inicializado exitosamente")

	return checker
}

// LoadAdvancedDefaultConfig carga configuración por defecto avanzada
func LoadAdvancedDefaultConfig() *AdvancedCheckerConfig {
	return &AdvancedCheckerConfig{
		ServiceName:           getEnvOrDefault("SERVICE_NAME", "gopherkit-service"),
		DatabaseMode:          getEnvOrDefault("DATABASE_MODE", "multi"),
		DatabaseSchema:        getEnvOrDefault("DATABASE_SCHEMA", ""),
		DatabaseURL:           getEnvOrDefault("DATABASE_URL", ""),
		RedisURL:              getEnvOrDefault("REDIS_URL", ""),
		RedisNamespace:        getEnvOrDefault("REDIS_NAMESPACE", ""),
		Environment:           getEnvOrDefault("ENVIRONMENT", "development"),
		DefaultTimeout:        5 * time.Second,
		CriticalChecks:        []string{"database", "service"},
		EnableTrendAnalysis:   true,
		EnableSystemInfo:      true,
		EnableRecommendations: true,
		EnablePrometheusMetrics: true,
		MaxHistoryEntries:     100,
		CheckInterval:         30 * time.Second,
		ParallelCheckLimit:    10,
		Metadata: map[string]interface{}{
			"go_version": runtime.Version(),
			"git_commit": getEnvOrDefault("GIT_COMMIT", "unknown"),
			"build_time": getEnvOrDefault("BUILD_TIME", "unknown"),
		},
	}
}

func applyAdvancedDefaultConfig(config *AdvancedCheckerConfig) {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 5 * time.Second
	}
	if config.MaxHistoryEntries == 0 {
		config.MaxHistoryEntries = 100
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 30 * time.Second
	}
	if config.ParallelCheckLimit == 0 {
		config.ParallelCheckLimit = 10
	}
}

// registerAdvancedDefaultChecks registra los health checks avanzados por defecto
func (hc *AdvancedHealthChecker) registerAdvancedDefaultChecks() {
	// Check básico del servicio
	hc.RegisterAdvancedCheck("service", hc.checkAdvancedService)

	// Check de conectividad de base de datos
	if hc.db != nil {
		hc.RegisterAdvancedCheck("database", hc.checkAdvancedDatabase)
	}

	// Check de Redis si está configurado
	if hc.redis != nil {
		hc.RegisterAdvancedCheck("redis", hc.checkAdvancedRedis)
	}

	// Checks avanzados de sistema si están habilitados
	if hc.enableSystemInfo {
		hc.RegisterAdvancedCheck("system_resources", hc.checkSystemResources)
		hc.RegisterAdvancedCheck("memory_usage", hc.checkMemoryUsage)
		hc.RegisterAdvancedCheck("goroutines", hc.checkGoroutines)
	}

	logrus.Infof("Registrados %d health checks avanzados por defecto", len(hc.checks))
}

// RegisterAdvancedCheck registra un nuevo health check avanzado
func (hc *AdvancedHealthChecker) RegisterAdvancedCheck(name string, checkFunc AdvancedCheckFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	hc.checks[name] = checkFunc
	logrus.WithField("check_name", name).Debug("Advanced health check registrado")
}

// CheckAdvancedHealth realiza todos los health checks avanzados
func (hc *AdvancedHealthChecker) CheckAdvancedHealth(ctx context.Context) *AdvancedHealthResult {
	start := time.Now()
	atomic.StoreInt64(&hc.lastCheckTime, start.Unix())

	result := &AdvancedHealthResult{
		Timestamp:   start,
		Checks:      make(map[string]*AdvancedCheckResult),
		Service:     hc.config.ServiceName,
		Mode:        hc.config.DatabaseMode,
		Environment: hc.config.Environment,
		Metadata:    hc.config.Metadata,
		Version:     getEnvOrDefault("SERVICE_VERSION", "unknown"),
		Summary: AdvancedHealthSummary{
			ByCategory: make(map[string]int),
			BySeverity: make(map[Severity]int),
			UpTime:     time.Since(hc.startTime),
		},
	}

	// Recopilar información del sistema si está habilitado
	if hc.enableSystemInfo {
		result.SystemInfo = hc.collectAdvancedSystemInfo()
		hc.lastSystemInfo = result.SystemInfo
	}

	// Ejecutar checks en paralelo
	checkChan := make(chan *AdvancedCheckResult, len(hc.checks))
	var wg sync.WaitGroup

	hc.mu.RLock()
	checkNames := make([]string, 0, len(hc.checks))
	for name := range hc.checks {
		checkNames = append(checkNames, name)
	}
	hc.mu.RUnlock()

	sort.Strings(checkNames)

	for _, name := range checkNames {
		wg.Add(1)
		go func(checkName string) {
			defer wg.Done()
			
			hc.workerPool <- struct{}{}
			defer func() { <-hc.workerPool }()

			checkCtx, cancel := context.WithTimeout(ctx, hc.config.DefaultTimeout)
			defer cancel()

			checkResult := hc.executeAdvancedCheck(checkCtx, checkName)
			checkChan <- checkResult
		}(name)
	}

	go func() {
		wg.Wait()
		close(checkChan)
	}()

	// Recopilar resultados
	overallStatus := AdvancedStatusHealthy
	overallSeverity := SeverityInfo
	var alerts []HealthAlert
	var recommendations []string

	for checkResult := range checkChan {
		result.Checks[checkResult.Name] = checkResult

		// Actualizar resumen
		result.Summary.TotalChecks++
		result.Summary.ByCategory[checkResult.Category]++
		result.Summary.BySeverity[checkResult.Severity]++

		// Determinar estado general
		switch checkResult.Status {
		case AdvancedStatusUnhealthy, AdvancedStatusCritical:
			if hc.isAdvancedCriticalCheck(checkResult.Name) {
				overallStatus = AdvancedStatusUnhealthy
				overallSeverity = SeverityCritical
			} else if overallStatus != AdvancedStatusUnhealthy {
				overallStatus = AdvancedStatusDegraded
			}
		case AdvancedStatusDegraded, AdvancedStatusWarning:
			if overallStatus == AdvancedStatusHealthy {
				overallStatus = AdvancedStatusDegraded
			}
		case AdvancedStatusHealthy:
			result.Summary.HealthyChecks++
		}

		// Recopilar alertas y recomendaciones
		if checkResult.Status != AdvancedStatusHealthy {
			alert := HealthAlert{
				ID:        fmt.Sprintf("%s_%d", checkResult.Name, time.Now().Unix()),
				Level:     string(checkResult.Status),
				Message:   checkResult.Message,
				Component: checkResult.Component,
				Timestamp: checkResult.Timestamp,
				Resolved:  false,
				Metadata:  checkResult.Metadata,
			}
			alerts = append(alerts, alert)
		}

		recommendations = append(recommendations, checkResult.Recommendations...)
	}

	// Finalizar resultado
	result.Status = overallStatus
	result.OverallSeverity = overallSeverity
	result.Duration = time.Since(start)
	result.Alerts = alerts
	result.Recommendations = hc.deduplicateAdvancedRecommendations(recommendations)
	result.Summary.CriticalIssues = len(alerts)
	result.Summary.Warnings = result.Summary.BySeverity[SeverityWarning]

	// Análisis de tendencias si está habilitado
	if hc.enableTrendAnalysis && hc.trendAnalyzer != nil {
		hc.trendAnalyzer.AddAdvancedDataPoint(result)
		result.TrendAnalysis = hc.trendAnalyzer.AnalyzeAdvancedTrends()
	}

	// Almacenar resultado
	hc.mu.Lock()
	hc.lastResult = result
	hc.mu.Unlock()

	// Enviar métricas a Prometheus
	if hc.metricsCollector != nil {
		hc.metricsCollector.RecordAdvancedHealthCheck(result)
	}

	// Enviar alertas si hay un alert manager configurado
	if hc.alertManager != nil && len(alerts) > 0 {
		hc.sendAdvancedAlertsToManager(alerts)
	}

	logrus.WithFields(logrus.Fields{
		"status":       result.Status,
		"duration_ms":  result.Duration.Milliseconds(),
		"checks":       result.Summary.TotalChecks,
		"healthy":      result.Summary.HealthyChecks,
		"alerts":       len(alerts),
	}).Info("Advanced health check completado")

	return result
}

// executeAdvancedCheck ejecuta un health check individual avanzado
func (hc *AdvancedHealthChecker) executeAdvancedCheck(ctx context.Context, checkName string) *AdvancedCheckResult {
	start := time.Now()

	hc.mu.RLock()
	checkFunc, exists := hc.checks[checkName]
	hc.mu.RUnlock()

	if !exists {
		return &AdvancedCheckResult{
			Name:      checkName,
			Status:    AdvancedStatusUnknown,
			Error:     "Check function not found",
			Duration:  time.Since(start),
			Timestamp: start,
			Severity:  SeverityError,
			Component: "health_checker",
			Category:  "system",
		}
	}

	metadata := map[string]interface{}{
		"service":     hc.config.ServiceName,
		"environment": hc.config.Environment,
		"check_name":  checkName,
		"timestamp":   start,
	}

	var result *AdvancedCheckResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = &AdvancedCheckResult{
					Name:      checkName,
					Status:    AdvancedStatusUnhealthy,
					Error:     fmt.Sprintf("Check panicked: %v", r),
					Duration:  time.Since(start),
					Timestamp: start,
					Severity:  SeverityCritical,
					Component: "health_checker",
					Category:  "system",
				}
			}
		}()

		result = checkFunc(ctx, metadata)
	}()

	// Validar y completar resultado
	if result == nil {
		result = &AdvancedCheckResult{
			Name:      checkName,
			Status:    AdvancedStatusUnknown,
			Error:     "Check returned nil result",
			Duration:  time.Since(start),
			Timestamp: start,
			Severity:  SeverityError,
			Component: "health_checker",
			Category:  "system",
		}
	}

	// Completar campos faltantes
	result.Name = checkName
	if result.Timestamp.IsZero() {
		result.Timestamp = start
	}
	if result.Duration == 0 {
		result.Duration = time.Since(start)
	}
	if result.Component == "" {
		result.Component = "unknown"
	}
	if result.Category == "" {
		result.Category = "general"
	}

	// Agregar recomendaciones si están habilitadas
	if hc.enableRecommendations {
		result.Recommendations = hc.generateAdvancedRecommendations(result)
	}

	return result
}

// Implementaciones de health checks avanzados

func (hc *AdvancedHealthChecker) checkAdvancedService(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult {
	start := time.Now()

	result := &AdvancedCheckResult{
		Critical:  true,
		Timeout:   1 * time.Second,
		Component: "service",
		Category:  "core",
		Severity:  SeverityInfo,
		Metadata: map[string]interface{}{
			"service":     hc.config.ServiceName,
			"mode":        hc.config.DatabaseMode,
			"environment": hc.config.Environment,
			"uptime":      time.Since(hc.startTime).String(),
		},
		Metrics: map[string]float64{
			"uptime_seconds": time.Since(hc.startTime).Seconds(),
		},
	}

	if hc.config.ServiceName == "unknown" || hc.config.ServiceName == "" {
		result.Status = AdvancedStatusDegraded
		result.Message = "Service name not configured"
		result.Severity = SeverityWarning
	} else {
		result.Status = AdvancedStatusHealthy
		result.Message = fmt.Sprintf("Service %s is running normally", hc.config.ServiceName)
	}

	result.Duration = time.Since(start)
	return result
}

func (hc *AdvancedHealthChecker) checkAdvancedDatabase(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult {
	start := time.Now()

	result := &AdvancedCheckResult{
		Critical:  true,
		Timeout:   5 * time.Second,
		Component: "database",
		Category:  "infrastructure",
		Severity:  SeverityInfo,
		Metrics:   make(map[string]float64),
	}

	if hc.db == nil {
		result.Status = AdvancedStatusUnhealthy
		result.Error = "Database connection not initialized"
		result.Severity = SeverityCritical
		result.Duration = time.Since(start)
		return result
	}

	// Test de conectividad
	pingStart := time.Now()
	if err := hc.db.PingContext(ctx); err != nil {
		result.Status = AdvancedStatusUnhealthy
		result.Error = fmt.Sprintf("Database ping failed: %v", err)
		result.Severity = SeverityCritical
		result.Duration = time.Since(start)
		return result
	}
	
	pingDuration := time.Since(pingStart)
	result.Metrics["ping_duration_ms"] = float64(pingDuration.Milliseconds())

	// Obtener estadísticas de la base de datos
	stats := hc.db.Stats()
	result.Metadata = map[string]interface{}{
		"open_connections": stats.OpenConnections,
		"in_use":          stats.InUse,
		"idle":            stats.Idle,
		"wait_count":      stats.WaitCount,
		"wait_duration":   stats.WaitDuration.String(),
	}

	result.Metrics["open_connections"] = float64(stats.OpenConnections)
	result.Metrics["in_use_connections"] = float64(stats.InUse)
	result.Metrics["idle_connections"] = float64(stats.Idle)

	result.Status = AdvancedStatusHealthy
	result.Message = "Database connection is healthy"
	result.Duration = time.Since(start)
	return result
}

func (hc *AdvancedHealthChecker) checkAdvancedRedis(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult {
	start := time.Now()

	result := &AdvancedCheckResult{
		Critical:  false,
		Timeout:   3 * time.Second,
		Component: "redis",
		Category:  "infrastructure",
		Severity:  SeverityInfo,
	}

	if hc.redis == nil {
		result.Status = AdvancedStatusDegraded
		result.Message = "Redis client not initialized"
		result.Duration = time.Since(start)
		return result
	}

	// Test Redis connectivity
	pong, err := hc.redis.Ping(ctx).Result()
	if err != nil {
		result.Status = AdvancedStatusUnhealthy
		result.Error = fmt.Sprintf("Redis ping failed: %v", err)
		result.Severity = SeverityError
		result.Duration = time.Since(start)
		return result
	}

	result.Metadata = map[string]interface{}{
		"ping_response": pong,
	}

	result.Status = AdvancedStatusHealthy
	result.Message = "Redis connection is healthy"
	result.Duration = time.Since(start)
	return result
}

func (hc *AdvancedHealthChecker) checkSystemResources(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult {
	start := time.Now()

	result := &AdvancedCheckResult{
		Critical:  false,
		Timeout:   2 * time.Second,
		Component: "system",
		Category:  "resources",
		Severity:  SeverityInfo,
		Metrics:   make(map[string]float64),
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memoryUsageMB := float64(memStats.Alloc) / 1024 / 1024
	result.Metrics["memory_usage_mb"] = memoryUsageMB
	result.Metrics["num_goroutines"] = float64(runtime.NumGoroutine())

	result.Metadata = map[string]interface{}{
		"memory_usage_mb": memoryUsageMB,
		"num_goroutines":  runtime.NumGoroutine(),
		"num_cpu":         runtime.NumCPU(),
	}

	if memoryUsageMB > 500 {
		result.Status = AdvancedStatusWarning
		result.Message = "High memory usage detected"
		result.Severity = SeverityWarning
	} else {
		result.Status = AdvancedStatusHealthy
		result.Message = "System resources are healthy"
	}

	result.Duration = time.Since(start)
	return result
}

func (hc *AdvancedHealthChecker) checkMemoryUsage(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult {
	start := time.Now()

	result := &AdvancedCheckResult{
		Critical:  false,
		Component: "memory",
		Category:  "resources",
		Severity:  SeverityInfo,
		Metrics:   make(map[string]float64),
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memoryUsageMB := float64(memStats.Alloc) / 1024 / 1024
	memoryHeapMB := float64(memStats.HeapAlloc) / 1024 / 1024

	result.Metrics["memory_usage_mb"] = memoryUsageMB
	result.Metrics["memory_heap_mb"] = memoryHeapMB
	result.Metrics["gc_cycles"] = float64(memStats.NumGC)

	result.Metadata = map[string]interface{}{
		"memory_usage_mb": memoryUsageMB,
		"memory_heap_mb":  memoryHeapMB,
		"gc_cycles":       memStats.NumGC,
	}

	if memoryUsageMB > 1000 {
		result.Status = AdvancedStatusDegraded
		result.Message = "Very high memory usage"
		result.Severity = SeverityError
	} else if memoryUsageMB > 500 {
		result.Status = AdvancedStatusWarning
		result.Message = "High memory usage"
		result.Severity = SeverityWarning
	} else {
		result.Status = AdvancedStatusHealthy
		result.Message = "Memory usage is normal"
	}

	result.Duration = time.Since(start)
	return result
}

func (hc *AdvancedHealthChecker) checkGoroutines(ctx context.Context, metadata map[string]interface{}) *AdvancedCheckResult {
	start := time.Now()

	result := &AdvancedCheckResult{
		Critical:  false,
		Component: "goroutines",
		Category:  "resources",
		Severity:  SeverityInfo,
		Metrics:   make(map[string]float64),
	}

	numGoroutines := runtime.NumGoroutine()
	result.Metrics["num_goroutines"] = float64(numGoroutines)

	result.Metadata = map[string]interface{}{
		"num_goroutines": numGoroutines,
		"num_cpu":        runtime.NumCPU(),
	}

	if numGoroutines > 5000 {
		result.Status = AdvancedStatusDegraded
		result.Message = "Very high number of goroutines"
		result.Severity = SeverityError
	} else if numGoroutines > 1000 {
		result.Status = AdvancedStatusWarning
		result.Message = "High number of goroutines"
		result.Severity = SeverityWarning
	} else {
		result.Status = AdvancedStatusHealthy
		result.Message = "Goroutine count is normal"
	}

	result.Duration = time.Since(start)
	return result
}

// Métodos auxiliares avanzados

func (hc *AdvancedHealthChecker) isAdvancedCriticalCheck(name string) bool {
	for _, critical := range hc.config.CriticalChecks {
		if critical == name {
			return true
		}
	}
	return false
}

func (hc *AdvancedHealthChecker) generateAdvancedRecommendations(result *AdvancedCheckResult) []string {
	var recommendations []string

	switch result.Status {
	case AdvancedStatusUnhealthy, AdvancedStatusCritical:
		switch result.Component {
		case "database":
			recommendations = append(recommendations, "Check database connection configuration")
			recommendations = append(recommendations, "Verify database server status")
		case "redis":
			recommendations = append(recommendations, "Verify Redis server connectivity")
		case "system", "memory", "goroutines":
			recommendations = append(recommendations, "Monitor system resource usage")
			recommendations = append(recommendations, "Consider scaling resources")
		}
	case AdvancedStatusDegraded, AdvancedStatusWarning:
		switch result.Component {
		case "database":
			recommendations = append(recommendations, "Monitor database performance")
		case "system", "memory", "goroutines":
			recommendations = append(recommendations, "Monitor resource trends")
		}
	}

	return recommendations
}

func (hc *AdvancedHealthChecker) deduplicateAdvancedRecommendations(recommendations []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, rec := range recommendations {
		if !seen[rec] && rec != "" {
			seen[rec] = true
			result = append(result, rec)
		}
	}

	return result
}

func (hc *AdvancedHealthChecker) collectAdvancedSystemInfo() AdvancedSystemInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	gcStats := GCStats{
		NextGC:       memStats.NextGC,
		LastGC:       time.Unix(0, int64(memStats.LastGC)),
		PauseTotalNs: memStats.PauseTotalNs,
		NumGC:        memStats.NumGC,
		GCPercent:    100,
	}

	runtimeInfo := RuntimeInfo{
		NumGoroutines: runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		MemStats:      memStats,
		GCStats:       gcStats,
	}

	resourceUsage := ResourceUsage{
		MemoryUsageMB: float64(memStats.Alloc) / 1024 / 1024,
		LastUpdated:   time.Now(),
	}

	dependencies := map[string]string{
		"go_runtime": runtime.Version(),
	}
	if hc.db != nil {
		dependencies["database"] = "connected"
	}
	if hc.redis != nil {
		dependencies["redis"] = "connected"
	}

	configuration := map[string]string{
		"environment":   hc.config.Environment,
		"database_mode": hc.config.DatabaseMode,
		"service_name":  hc.config.ServiceName,
	}

	capabilities := []string{"health_checks", "metrics", "logging"}
	if hc.enableTrendAnalysis {
		capabilities = append(capabilities, "trend_analysis")
	}

	return AdvancedSystemInfo{
		GoVersion:     runtime.Version(),
		Runtime:       runtimeInfo,
		Dependencies:  dependencies,
		Configuration: configuration,
		Resources:     resourceUsage,
		Capabilities:  capabilities,
	}
}

func (hc *AdvancedHealthChecker) startBackgroundServices() {
	hc.backgroundWG.Add(1)
	go hc.backgroundCleanup()
}

func (hc *AdvancedHealthChecker) backgroundCleanup() {
	defer hc.backgroundWG.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.cleanupOldAdvancedAlerts()
		case <-hc.shutdownCh:
			return
		}
	}
}

func (hc *AdvancedHealthChecker) cleanupOldAdvancedAlerts() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	for id, alert := range hc.activeAlerts {
		if alert.Timestamp.Before(cutoff) && alert.Resolved {
			delete(hc.activeAlerts, id)
		}
	}
}

func (hc *AdvancedHealthChecker) sendAdvancedAlertsToManager(alerts []HealthAlert) {
	for _, alert := range alerts {
		go func(a HealthAlert) {
			if hc.alertManager != nil {
				hc.alertManager.FireAlert("health-checker", a.Component, a.Message, "error", map[string]interface{}{
					"component": a.Component,
					"level":     a.Level,
				})
			}
		}(alert)
	}
}

// GetAdvancedLastResult retorna el último resultado de health check avanzado
func (hc *AdvancedHealthChecker) GetAdvancedLastResult() *AdvancedHealthResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastResult
}

// CloseAdvanced cierra el health checker avanzado de forma graciosa
func (hc *AdvancedHealthChecker) CloseAdvanced() error {
	close(hc.shutdownCh)
	hc.backgroundWG.Wait()
	logrus.Info("Advanced Health Checker cerrado exitosamente")
	return nil
}

// HTTPAdvancedHandler retorna un handler HTTP para health checks avanzados
func (hc *AdvancedHealthChecker) HTTPAdvancedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result := hc.CheckAdvancedHealth(ctx)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("X-Health-Check-Version", "gopherkit-2.0")

		switch result.Status {
		case AdvancedStatusHealthy:
			w.WriteHeader(http.StatusOK)
		case AdvancedStatusDegraded, AdvancedStatusWarning:
			w.WriteHeader(http.StatusOK)
		case AdvancedStatusUnhealthy, AdvancedStatusCritical:
			w.WriteHeader(http.StatusServiceUnavailable)
		case AdvancedStatusMaintenance:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}

		if err := json.NewEncoder(w).Encode(result); err != nil {
			logrus.WithError(err).Error("Error encoding advanced health check response")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// Stubs para funcionalidades avanzadas

func NewTrendAnalyzer(maxDataPoints int, analysisWindow time.Duration) *TrendAnalyzer {
	return &TrendAnalyzer{
		dataPoints:     make(map[string][]TrendDataPoint),
		maxDataPoints:  maxDataPoints,
		analysisWindow: analysisWindow,
	}
}

func (ta *TrendAnalyzer) AddAdvancedDataPoint(result *AdvancedHealthResult) {
	// Implementación simplificada
}

func (ta *TrendAnalyzer) AnalyzeAdvancedTrends() map[string]HealthTrend {
	return make(map[string]HealthTrend)
}

func NewHealthMetricsCollector(serviceName string) *HealthMetricsCollector {
	return &HealthMetricsCollector{
		healthStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gopherkit",
			Subsystem: "health",
			Name:      "status",
			Help:      "Health check status",
		}, []string{"service", "check"}),
	}
}

func (hmc *HealthMetricsCollector) RecordAdvancedHealthCheck(result *AdvancedHealthResult) {
	// Implementación simplificada
}

// LoadAdvancedConfigFromBase carga configuración avanzada desde BaseConfig de gopherkit
func LoadAdvancedConfigFromBase(baseConfig *config.BaseConfig, serviceName string) *AdvancedCheckerConfig {
	config := LoadAdvancedDefaultConfig()
	config.ServiceName = serviceName
	config.Environment = baseConfig.Server.Environment
	config.BaseConfig = baseConfig
	
	if baseConfig.Observability.MetricsEnabled {
		config.EnablePrometheusMetrics = true
	}
	
	return config
}

// Helper function para obtener variables de entorno
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}