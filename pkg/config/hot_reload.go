// Package config - Hot Reload Configuration System
// Migrado de ClubPulse a gopherkit con mejoras significativas
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// HotReloader gestiona la recarga en caliente de configuraciones mejorado
type HotReloader struct {
	configFile    string
	watchedFiles  map[string]bool
	watchers      map[string]*fsnotify.Watcher
	callbacks     map[string][]ReloadCallback
	config        *DynamicConfig
	baseConfig    *BaseConfig
	mutex         sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	reloadChan    chan ReloadEvent
	debounceTimer *time.Timer
	debounceDelay time.Duration
	isRunning     bool
	lastReload    time.Time
	metrics       *HotReloadMetrics
	logger        Logger
	wg            sync.WaitGroup
}

// Logger interface para logging
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Debugf(format string, args ...interface{})
	WithField(key string, value interface{}) Logger
}

// DefaultLogger implementación simple de logger
type DefaultLogger struct{}

func (dl *DefaultLogger) Info(msg string)                                 { fmt.Printf("INFO: %s\n", msg) }
func (dl *DefaultLogger) Warn(msg string)                                 { fmt.Printf("WARN: %s\n", msg) }
func (dl *DefaultLogger) Error(msg string)                                { fmt.Printf("ERROR: %s\n", msg) }
func (dl *DefaultLogger) Debugf(format string, args ...interface{})      { fmt.Printf("DEBUG: "+format+"\n", args...) }
func (dl *DefaultLogger) WithField(key string, value interface{}) Logger { return dl }

// HotReloadMetrics métricas del sistema de hot reload
type HotReloadMetrics struct {
	TotalReloads       int64                    `json:"total_reloads"`
	SuccessfulReloads  int64                    `json:"successful_reloads"`
	FailedReloads      int64                    `json:"failed_reloads"`
	CallbackExecutions int64                    `json:"callback_executions"`
	WatchedFilesCount  int                      `json:"watched_files_count"`
	AverageReloadTime  time.Duration            `json:"average_reload_time"`
	LastReloadTime     time.Time                `json:"last_reload_time"`
	ReloadHistory      []ReloadEvent            `json:"reload_history"`
	FileChangeEvents   map[string]int64         `json:"file_change_events"`
	CallbackErrors     map[string]int64         `json:"callback_errors"`
	mu                 sync.RWMutex
}

// ReloadCallback función que se ejecuta cuando cambia la configuración
type ReloadCallback func(oldConfig, newConfig *DynamicConfig) error

// ReloadEvent evento de recarga mejorado
type ReloadEvent struct {
	File        string    `json:"file"`
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	OldVersion  int       `json:"old_version"`
	NewVersion  int       `json:"new_version"`
	ChangedKeys []string  `json:"changed_keys,omitempty"`
}

// DynamicConfig configuración que puede recargarse en caliente mejorada
type DynamicConfig struct {
	// Configuración de base de datos
	DatabaseMode     string            `json:"database_mode"`
	DatabaseURL      string            `json:"database_url"`
	DatabaseSchema   string            `json:"database_schema"`
	
	// Configuraciones específicas
	CacheConfig      CacheConfigDynamic       `json:"cache"`
	FallbackConfig   FallbackConfigDynamic    `json:"fallback"`
	SecurityConfig   SecurityConfigDynamic    `json:"security"`
	MonitoringConfig MonitoringConfigDynamic  `json:"monitoring"`
	
	// Configuraciones adicionales
	FeatureFlags     map[string]bool   `json:"feature_flags"`
	CustomSettings   map[string]string `json:"custom_settings"`
	
	// Metadatos
	LastUpdated      time.Time         `json:"last_updated"`
	Version          int               `json:"version"`
	LoadedFrom       string            `json:"loaded_from"`
	Environment      string            `json:"environment"`
}

// CacheConfigDynamic configuración de cache que puede cambiar dinámicamente
type CacheConfigDynamic struct {
	Enabled            bool          `json:"enabled"`
	TTL                time.Duration `json:"ttl"`
	Namespace          string        `json:"namespace"`
	MaxSize            int64         `json:"max_size"`
	EvictionPolicy     string        `json:"eviction_policy"`
	CompressionEnabled bool          `json:"compression_enabled"`
	PrefetchEnabled    bool          `json:"prefetch_enabled"`
	CacheWarmupEnabled bool          `json:"cache_warmup_enabled"`
}

// FallbackConfigDynamic configuración de fallback que puede cambiar dinámicamente
type FallbackConfigDynamic struct {
	Enabled               bool          `json:"enabled"`
	HealthCheckInterval   time.Duration `json:"health_check_interval"`
	FailureThreshold      int           `json:"failure_threshold"`
	RecoveryCheckInterval time.Duration `json:"recovery_check_interval"`
	AlertWebhookURL       string        `json:"alert_webhook_url"`
	MaxRetryAttempts      int           `json:"max_retry_attempts"`
	AutoRecoveryEnabled   bool          `json:"auto_recovery_enabled"`
	NotificationsEnabled  bool          `json:"notifications_enabled"`
}

// SecurityConfigDynamic configuración de seguridad que puede cambiar dinámicamente
type SecurityConfigDynamic struct {
	RLSEnabled       bool          `json:"rls_enabled"`
	JWTSecret        string        `json:"jwt_secret"`
	TokenTTL         time.Duration `json:"token_ttl"`
	RateLimitEnabled bool          `json:"rate_limit_enabled"`
	RateLimitRPS     int           `json:"rate_limit_rps"`
	CORSOrigins      []string      `json:"cors_origins"`
	SecurityHeaders  bool          `json:"security_headers"`
	EncryptionEnabled bool         `json:"encryption_enabled"`
	AuditLogEnabled  bool          `json:"audit_log_enabled"`
}

// MonitoringConfigDynamic configuración de monitoreo que puede cambiar dinámicamente
type MonitoringConfigDynamic struct {
	Enabled          bool    `json:"enabled"`
	MetricsEnabled   bool    `json:"metrics_enabled"`
	MetricsPort      int     `json:"metrics_port"`
	HealthCheckPort  int     `json:"health_check_port"`
	LogLevel         string  `json:"log_level"`
	SamplingRate     float64 `json:"sampling_rate"`
	AlertsEnabled    bool    `json:"alerts_enabled"`
	DashboardEnabled bool    `json:"dashboard_enabled"`
	TracingEnabled   bool    `json:"tracing_enabled"`
	ProfilerEnabled  bool    `json:"profiler_enabled"`
}

// NewHotReloader crea un nuevo sistema de hot-reload mejorado
func NewHotReloader(configFile string, baseConfig *BaseConfig, logger Logger) (*HotReloader, error) {
	if logger == nil {
		logger = &DefaultLogger{}
	}

	ctx, cancel := context.WithCancel(context.Background())

	hr := &HotReloader{
		configFile:    configFile,
		baseConfig:    baseConfig,
		watchedFiles:  make(map[string]bool),
		watchers:      make(map[string]*fsnotify.Watcher),
		callbacks:     make(map[string][]ReloadCallback),
		ctx:           ctx,
		cancel:        cancel,
		reloadChan:    make(chan ReloadEvent, 100),
		debounceDelay: 500 * time.Millisecond,
		isRunning:     false,
		logger:        logger,
		metrics: &HotReloadMetrics{
			ReloadHistory:    make([]ReloadEvent, 0, 50),
			FileChangeEvents: make(map[string]int64),
			CallbackErrors:   make(map[string]int64),
		},
	}

	// Cargar configuración inicial
	if err := hr.loadConfig(); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to load initial config")
	}

	// Configurar archivos a observar
	if err := hr.setupWatchers(); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to setup watchers")
	}

	return hr, nil
}

// Start inicia el sistema de hot-reload mejorado
func (hr *HotReloader) Start() error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if hr.isRunning {
		return errors.New("hot reloader is already running")
	}

	hr.logger.Info("🔥 Iniciando sistema de hot-reload de configuración mejorado...")

	// Iniciar goroutines de monitoreo
	hr.wg.Add(2)
	go hr.watchFiles()
	go hr.processReloadEvents()

	hr.isRunning = true
	hr.logger.Info("✅ Sistema de hot-reload iniciado exitosamente")

	return nil
}

// Stop detiene el sistema de hot-reload con limpieza mejorada
func (hr *HotReloader) Stop() error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if !hr.isRunning {
		return nil
	}

	hr.logger.Info("🛑 Deteniendo sistema de hot-reload...")

	// Cancelar contexto
	hr.cancel()

	// Cerrar watchers
	for file, watcher := range hr.watchers {
		if watcher != nil {
			watcher.Close()
			hr.logger.Debugf("📂 Watcher cerrado para: %s", file)
		}
	}

	// Esperar que las goroutines terminen
	hr.wg.Wait()

	hr.isRunning = false
	hr.logger.Info("✅ Sistema de hot-reload detenido completamente")

	return nil
}

// setupWatchers configura los observadores de archivos mejorado
func (hr *HotReloader) setupWatchers() error {
	filesToWatch := []string{
		hr.configFile,
		".env",
		".env.local",
		".env.development",
		".env.production",
		".env.staging",
		"config/database.json",
		"config/cache.json",
		"config/security.json",
		"config/monitoring.json",
		"config/features.json",
	}

	watchedCount := 0
	for _, file := range filesToWatch {
		if _, err := os.Stat(file); err == nil {
			if err := hr.addWatcher(file); err != nil {
				hr.logger.Warn(fmt.Sprintf("⚠️ No se pudo observar %s: %v", file, err))
			} else {
				hr.logger.Debugf("👁️ Observando archivo: %s", file)
				watchedCount++
			}
		}
	}

	hr.metrics.mu.Lock()
	hr.metrics.WatchedFilesCount = watchedCount
	hr.metrics.mu.Unlock()

	hr.logger.Info(fmt.Sprintf("📁 Observando %d archivos de configuración", watchedCount))
	return nil
}

// addWatcher agrega un observador para un archivo específico mejorado
func (hr *HotReloader) addWatcher(file string) error {
	absFile, err := filepath.Abs(file)
	if err != nil {
		return errors.Wrapf(err, "failed to get absolute path for %s", file)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrapf(err, "failed to create watcher for %s", file)
	}

	if err := watcher.Add(absFile); err != nil {
		watcher.Close()
		return errors.Wrapf(err, "failed to add file %s to watcher", file)
	}

	hr.watchers[file] = watcher
	hr.watchedFiles[file] = true

	return nil
}

// watchFiles observa cambios en los archivos mejorado
func (hr *HotReloader) watchFiles() {
	defer hr.wg.Done()
	
	for {
		select {
		case <-hr.ctx.Done():
			hr.logger.Info("🔚 Deteniendo observación de archivos")
			return

		default:
			for file, watcher := range hr.watchers {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						continue
					}
					hr.handleFileEvent(file, event)

				case err, ok := <-watcher.Errors:
					if !ok {
						continue
					}
					hr.logger.Error(fmt.Sprintf("❌ Error watching file %s: %v", file, err))

				case <-time.After(100 * time.Millisecond):
					// Continue to next watcher
				}
			}
		}
	}
}

// handleFileEvent maneja eventos de cambio en archivos mejorado
func (hr *HotReloader) handleFileEvent(file string, event fsnotify.Event) {
	if event.Op&fsnotify.Write != fsnotify.Write && event.Op&fsnotify.Create != fsnotify.Create {
		return
	}

	hr.logger.Debugf("📝 Detectado cambio en archivo: %s (operación: %s)", file, event.Op.String())

	// Registrar evento en métricas
	hr.metrics.mu.Lock()
	hr.metrics.FileChangeEvents[file]++
	hr.metrics.mu.Unlock()

	// Debounce para evitar múltiples reloads rápidos
	if hr.debounceTimer != nil {
		hr.debounceTimer.Stop()
	}

	hr.debounceTimer = time.AfterFunc(hr.debounceDelay, func() {
		hr.triggerReload(file)
	})
}

// triggerReload dispara una recarga de configuración mejorado
func (hr *HotReloader) triggerReload(file string) {
	start := time.Now()
	hr.logger.Info(fmt.Sprintf("🔄 Iniciando recarga por cambio en: %s", file))

	// Obtener configuración actual
	oldConfig := hr.GetConfig()
	oldVersion := 0
	if oldConfig != nil {
		oldVersion = oldConfig.Version
	}

	event := ReloadEvent{
		File:       file,
		Type:       "config_change",
		Timestamp:  start,
		Success:    false,
		OldVersion: oldVersion,
	}

	// Intentar recargar configuración
	if err := hr.reloadConfig(); err != nil {
		event.Error = err.Error()
		event.Duration = time.Since(start)
		hr.logger.Error(fmt.Sprintf("❌ Error al recargar configuración: %v", err))
		hr.recordFailedReload()
	} else {
		newConfig := hr.GetConfig()
		event.Success = true
		event.Duration = time.Since(start)
		event.NewVersion = newConfig.Version
		event.ChangedKeys = hr.detectChangedKeys(oldConfig, newConfig)
		
		hr.logger.Info(fmt.Sprintf("✅ Configuración recargada exitosamente desde: %s (v%d->v%d)", 
			file, oldVersion, newConfig.Version))
		hr.recordSuccessfulReload(event.Duration)
	}

	// Enviar evento al canal
	select {
	case hr.reloadChan <- event:
	default:
		hr.logger.Warn("⚠️ Canal de eventos de recarga lleno")
	}
}

// detectChangedKeys detecta qué claves cambiaron entre configuraciones
func (hr *HotReloader) detectChangedKeys(oldConfig, newConfig *DynamicConfig) []string {
	if oldConfig == nil || newConfig == nil {
		return []string{"all"}
	}

	var changedKeys []string

	if oldConfig.DatabaseMode != newConfig.DatabaseMode {
		changedKeys = append(changedKeys, "database_mode")
	}
	if oldConfig.DatabaseURL != newConfig.DatabaseURL {
		changedKeys = append(changedKeys, "database_url")
	}
	if oldConfig.CacheConfig.Enabled != newConfig.CacheConfig.Enabled {
		changedKeys = append(changedKeys, "cache.enabled")
	}
	if oldConfig.FallbackConfig.Enabled != newConfig.FallbackConfig.Enabled {
		changedKeys = append(changedKeys, "fallback.enabled")
	}
	if oldConfig.MonitoringConfig.LogLevel != newConfig.MonitoringConfig.LogLevel {
		changedKeys = append(changedKeys, "monitoring.log_level")
	}

	// Detectar cambios en feature flags
	for key, newValue := range newConfig.FeatureFlags {
		if oldValue, exists := oldConfig.FeatureFlags[key]; !exists || oldValue != newValue {
			changedKeys = append(changedKeys, fmt.Sprintf("feature_flags.%s", key))
		}
	}

	return changedKeys
}

// processReloadEvents procesa eventos de recarga mejorado
func (hr *HotReloader) processReloadEvents() {
	defer hr.wg.Done()
	
	for {
		select {
		case <-hr.ctx.Done():
			hr.logger.Info("🔚 Deteniendo procesamiento de eventos de recarga")
			return

		case event := <-hr.reloadChan:
			hr.handleReloadEvent(event)
		}
	}
}

// handleReloadEvent maneja un evento de recarga mejorado
func (hr *HotReloader) handleReloadEvent(event ReloadEvent) {
	// Ejecutar callbacks registrados
	callbackCount := 0
	for callbackName, callbacks := range hr.callbacks {
		for _, callback := range callbacks {
			callbackCount++
			if err := callback(nil, hr.config); err != nil {
				hr.logger.Error(fmt.Sprintf("❌ Error en callback %s: %v", callbackName, err))
				hr.recordCallbackError(callbackName)
			} else {
				hr.logger.Debugf("✅ Callback %s ejecutado exitosamente", callbackName)
			}
		}
	}

	// Registrar métricas
	hr.metrics.mu.Lock()
	hr.metrics.CallbackExecutions += int64(callbackCount)
	hr.metrics.mu.Unlock()

	// Persistir evento para auditoría
	hr.logReloadEvent(event)
	hr.addToHistory(event)
}

// loadConfig carga la configuración desde archivo mejorado
func (hr *HotReloader) loadConfig() error {
	environment := "development"
	if hr.baseConfig != nil {
		environment = hr.baseConfig.Server.Environment
	}

	config := &DynamicConfig{
		DatabaseMode:   getEnvOrDefault("DATABASE_MODE", "single"),
		DatabaseURL:    getEnvOrDefault("DATABASE_URL", ""),
		DatabaseSchema: getEnvOrDefault("DATABASE_SCHEMA", "public"),
		FeatureFlags:   make(map[string]bool),
		CustomSettings: make(map[string]string),
		LastUpdated:    time.Now(),
		Version:        1,
		LoadedFrom:     hr.configFile,
		Environment:    environment,
	}

	// Cargar configuración de cache desde env y baseConfig
	config.CacheConfig = hr.loadCacheConfig()
	config.FallbackConfig = hr.loadFallbackConfig()
	config.SecurityConfig = hr.loadSecurityConfig()
	config.MonitoringConfig = hr.loadMonitoringConfig()

	// Cargar feature flags por defecto
	config.FeatureFlags = map[string]bool{
		"hot_reload_enabled":     true,
		"metrics_collection":     true,
		"advanced_caching":       getEnvBoolOrDefault("FEATURE_ADVANCED_CACHING", false),
		"auto_scaling":          getEnvBoolOrDefault("FEATURE_AUTO_SCALING", false),
		"database_fallback":     getEnvBoolOrDefault("FEATURE_DATABASE_FALLBACK", true),
		"real_time_monitoring":  getEnvBoolOrDefault("FEATURE_REAL_TIME_MONITORING", true),
		"security_audit":        getEnvBoolOrDefault("FEATURE_SECURITY_AUDIT", false),
	}

	// Cargar desde archivo JSON si existe
	if _, err := os.Stat(hr.configFile); err == nil {
		if err := hr.loadFromJSONFile(config); err != nil {
			hr.logger.Warn(fmt.Sprintf("⚠️ Error loading JSON config: %v", err))
		}
	}

	hr.mutex.Lock()
	if hr.config != nil {
		config.Version = hr.config.Version + 1
	}
	hr.config = config
	hr.lastReload = time.Now()
	hr.mutex.Unlock()

	return nil
}

// loadCacheConfig carga configuración de cache
func (hr *HotReloader) loadCacheConfig() CacheConfigDynamic {
	config := CacheConfigDynamic{
		Enabled:            getEnvBoolOrDefault("CACHE_ENABLED", true),
		TTL:                getEnvDurationOrDefault("CACHE_TTL", 5*time.Minute),
		Namespace:          getEnvOrDefault("CACHE_NAMESPACE", "gopherkit"),
		MaxSize:            getEnvInt64OrDefault("CACHE_MAX_SIZE", 100*1024*1024), // 100MB
		EvictionPolicy:     getEnvOrDefault("CACHE_EVICTION_POLICY", "lru"),
		CompressionEnabled: getEnvBoolOrDefault("CACHE_COMPRESSION", false),
		PrefetchEnabled:    getEnvBoolOrDefault("CACHE_PREFETCH_ENABLED", false),
		CacheWarmupEnabled: getEnvBoolOrDefault("CACHE_WARMUP_ENABLED", false),
	}

	// Integrar con baseConfig si está disponible
	if hr.baseConfig != nil {
		config.Enabled = hr.baseConfig.Cache.Enabled
		config.TTL = hr.baseConfig.Cache.TTL
	}

	return config
}

// loadFallbackConfig carga configuración de fallback
func (hr *HotReloader) loadFallbackConfig() FallbackConfigDynamic {
	return FallbackConfigDynamic{
		Enabled:               getEnvBoolOrDefault("ENABLE_AUTO_FALLBACK", true),
		HealthCheckInterval:   getEnvDurationOrDefault("HEALTH_CHECK_INTERVAL", 30*time.Second),
		FailureThreshold:      getEnvIntOrDefault("FAILURE_THRESHOLD", 3),
		RecoveryCheckInterval: getEnvDurationOrDefault("RECOVERY_CHECK_INTERVAL", 60*time.Second),
		AlertWebhookURL:       getEnvOrDefault("ALERT_WEBHOOK_URL", ""),
		MaxRetryAttempts:      getEnvIntOrDefault("MAX_RETRY_ATTEMPTS", 3),
		AutoRecoveryEnabled:   getEnvBoolOrDefault("AUTO_RECOVERY_ENABLED", true),
		NotificationsEnabled:  getEnvBoolOrDefault("NOTIFICATIONS_ENABLED", true),
	}
}

// loadSecurityConfig carga configuración de seguridad
func (hr *HotReloader) loadSecurityConfig() SecurityConfigDynamic {
	config := SecurityConfigDynamic{
		RLSEnabled:        getEnvBoolOrDefault("RLS_ENABLED", true),
		JWTSecret:         getEnvOrDefault("JWT_SECRET", ""),
		TokenTTL:          getEnvDurationOrDefault("TOKEN_TTL", 1*time.Hour),
		RateLimitEnabled:  getEnvBoolOrDefault("RATE_LIMIT_ENABLED", true),
		RateLimitRPS:      getEnvIntOrDefault("RATE_LIMIT_RPS", 100),
		CORSOrigins:       getEnvSliceOrDefault("CORS_ORIGINS", []string{"http://localhost:3000"}),
		SecurityHeaders:   getEnvBoolOrDefault("SECURITY_HEADERS_ENABLED", true),
		EncryptionEnabled: getEnvBoolOrDefault("ENCRYPTION_ENABLED", false),
		AuditLogEnabled:   getEnvBoolOrDefault("AUDIT_LOG_ENABLED", false),
	}

	// Integrar con baseConfig si está disponible
	if hr.baseConfig != nil {
		config.JWTSecret = hr.baseConfig.Security.JWT.Secret
		config.TokenTTL = hr.baseConfig.Security.JWT.AccessDuration
		config.RateLimitEnabled = hr.baseConfig.Security.RateLimit.Enabled
		config.RateLimitRPS = hr.baseConfig.Security.RateLimit.RequestsPerMinute
		config.CORSOrigins = hr.baseConfig.Security.CORS.AllowedOrigins
	}

	return config
}

// loadMonitoringConfig carga configuración de monitoreo
func (hr *HotReloader) loadMonitoringConfig() MonitoringConfigDynamic {
	config := MonitoringConfigDynamic{
		Enabled:          getEnvBoolOrDefault("MONITORING_ENABLED", true),
		MetricsEnabled:   getEnvBoolOrDefault("METRICS_ENABLED", true),
		MetricsPort:      getEnvIntOrDefault("METRICS_PORT", 9090),
		HealthCheckPort:  getEnvIntOrDefault("HEALTH_CHECK_PORT", 8080),
		LogLevel:         getEnvOrDefault("LOG_LEVEL", "info"),
		SamplingRate:     getEnvFloatOrDefault("SAMPLING_RATE", 0.1),
		AlertsEnabled:    getEnvBoolOrDefault("ALERTS_ENABLED", true),
		DashboardEnabled: getEnvBoolOrDefault("DASHBOARD_ENABLED", true),
		TracingEnabled:   getEnvBoolOrDefault("TRACING_ENABLED", true),
		ProfilerEnabled:  getEnvBoolOrDefault("PROFILER_ENABLED", false),
	}

	// Integrar con baseConfig si está disponible
	if hr.baseConfig != nil {
		config.Enabled = hr.baseConfig.Observability.MetricsEnabled
		config.MetricsEnabled = hr.baseConfig.Observability.MetricsEnabled
		config.LogLevel = hr.baseConfig.Observability.LogLevel
		config.TracingEnabled = hr.baseConfig.Observability.TracingEnabled
	}

	return config
}

// loadFromJSONFile carga configuración desde archivo JSON mejorado
func (hr *HotReloader) loadFromJSONFile(config *DynamicConfig) error {
	data, err := os.ReadFile(hr.configFile)
	if err != nil {
		return errors.Wrap(err, "failed to read config file")
	}

	var jsonConfig map[string]interface{}
	if err := json.Unmarshal(data, &jsonConfig); err != nil {
		return errors.Wrap(err, "failed to unmarshal JSON config")
	}

	// Actualizar configuración con valores del JSON
	if dbMode, ok := jsonConfig["database_mode"].(string); ok {
		config.DatabaseMode = dbMode
	}
	if dbURL, ok := jsonConfig["database_url"].(string); ok {
		config.DatabaseURL = dbURL
	}

	// Cargar feature flags
	if flags, ok := jsonConfig["feature_flags"].(map[string]interface{}); ok {
		for key, value := range flags {
			if boolValue, ok := value.(bool); ok {
				config.FeatureFlags[key] = boolValue
			}
		}
	}

	// Cargar configuraciones anidadas
	if cacheConfig, ok := jsonConfig["cache"].(map[string]interface{}); ok {
		hr.loadCacheConfigFromJSON(cacheConfig, &config.CacheConfig)
	}

	config.LoadedFrom = hr.configFile
	hr.logger.Debugf("📄 Configuración cargada desde JSON: %s", hr.configFile)

	return nil
}

// loadCacheConfigFromJSON carga configuración de cache desde JSON mejorado
func (hr *HotReloader) loadCacheConfigFromJSON(jsonConfig map[string]interface{}, cacheConfig *CacheConfigDynamic) {
	if enabled, ok := jsonConfig["enabled"].(bool); ok {
		cacheConfig.Enabled = enabled
	}
	if ttl, ok := jsonConfig["ttl"].(string); ok {
		if duration, err := time.ParseDuration(ttl); err == nil {
			cacheConfig.TTL = duration
		}
	}
	if namespace, ok := jsonConfig["namespace"].(string); ok {
		cacheConfig.Namespace = namespace
	}
	if maxSize, ok := jsonConfig["max_size"].(float64); ok {
		cacheConfig.MaxSize = int64(maxSize)
	}
	if compression, ok := jsonConfig["compression_enabled"].(bool); ok {
		cacheConfig.CompressionEnabled = compression
	}
}

// reloadConfig recarga la configuración
func (hr *HotReloader) reloadConfig() error {
	oldConfig := hr.GetConfig()

	if err := hr.loadConfig(); err != nil {
		return errors.Wrap(err, "failed to reload config")
	}

	newConfig := hr.GetConfig()
	hr.logger.Info(fmt.Sprintf("📊 Configuración recargada - Versión %d -> %d", 
		oldConfig.Version, newConfig.Version))

	return nil
}

// Métodos de métricas

func (hr *HotReloader) recordSuccessfulReload(duration time.Duration) {
	hr.metrics.mu.Lock()
	defer hr.metrics.mu.Unlock()
	
	hr.metrics.TotalReloads++
	hr.metrics.SuccessfulReloads++
	hr.metrics.LastReloadTime = time.Now()
	
	// Calcular promedio
	if hr.metrics.SuccessfulReloads > 0 {
		hr.metrics.AverageReloadTime = (hr.metrics.AverageReloadTime*time.Duration(hr.metrics.SuccessfulReloads-1) + duration) / time.Duration(hr.metrics.SuccessfulReloads)
	}
}

func (hr *HotReloader) recordFailedReload() {
	hr.metrics.mu.Lock()
	defer hr.metrics.mu.Unlock()
	
	hr.metrics.TotalReloads++
	hr.metrics.FailedReloads++
}

func (hr *HotReloader) recordCallbackError(callbackName string) {
	hr.metrics.mu.Lock()
	defer hr.metrics.mu.Unlock()
	
	hr.metrics.CallbackErrors[callbackName]++
}

func (hr *HotReloader) addToHistory(event ReloadEvent) {
	hr.metrics.mu.Lock()
	defer hr.metrics.mu.Unlock()
	
	hr.metrics.ReloadHistory = append(hr.metrics.ReloadHistory, event)
	
	// Mantener solo últimos 50 eventos
	if len(hr.metrics.ReloadHistory) > 50 {
		hr.metrics.ReloadHistory = hr.metrics.ReloadHistory[1:]
	}
}

// GetConfig obtiene la configuración actual de forma thread-safe
func (hr *HotReloader) GetConfig() *DynamicConfig {
	hr.mutex.RLock()
	defer hr.mutex.RUnlock()

	if hr.config == nil {
		return &DynamicConfig{}
	}

	// Devolver copia para evitar race conditions
	configCopy := *hr.config
	
	// Copiar mapas
	configCopy.FeatureFlags = make(map[string]bool)
	for k, v := range hr.config.FeatureFlags {
		configCopy.FeatureFlags[k] = v
	}
	
	configCopy.CustomSettings = make(map[string]string)
	for k, v := range hr.config.CustomSettings {
		configCopy.CustomSettings[k] = v
	}
	
	return &configCopy
}

// GetMetrics retorna las métricas del hot reloader
func (hr *HotReloader) GetMetrics() *HotReloadMetrics {
	hr.metrics.mu.RLock()
	defer hr.metrics.mu.RUnlock()
	
	// Retornar copia de las métricas
	metrics := *hr.metrics
	
	// Copiar mapas
	metrics.FileChangeEvents = make(map[string]int64)
	for k, v := range hr.metrics.FileChangeEvents {
		metrics.FileChangeEvents[k] = v
	}
	
	metrics.CallbackErrors = make(map[string]int64)
	for k, v := range hr.metrics.CallbackErrors {
		metrics.CallbackErrors[k] = v
	}
	
	// Copiar historial
	metrics.ReloadHistory = make([]ReloadEvent, len(hr.metrics.ReloadHistory))
	copy(metrics.ReloadHistory, hr.metrics.ReloadHistory)
	
	return &metrics
}

// RegisterCallback registra un callback para cambios de configuración
func (hr *HotReloader) RegisterCallback(name string, callback ReloadCallback) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if hr.callbacks[name] == nil {
		hr.callbacks[name] = make([]ReloadCallback, 0)
	}

	hr.callbacks[name] = append(hr.callbacks[name], callback)
	hr.logger.Info(fmt.Sprintf("📝 Callback '%s' registrado para hot-reload", name))
}

// UpdateConfig actualiza la configuración programáticamente mejorado
func (hr *HotReloader) UpdateConfig(updates map[string]interface{}) error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if hr.config == nil {
		return errors.New("config not initialized")
	}

	oldConfig := *hr.config

	// Aplicar actualizaciones
	for key, value := range updates {
		switch key {
		case "database_mode":
			if mode, ok := value.(string); ok {
				hr.config.DatabaseMode = mode
			}
		case "cache_enabled":
			if enabled, ok := value.(bool); ok {
				hr.config.CacheConfig.Enabled = enabled
			}
		case "fallback_enabled":
			if enabled, ok := value.(bool); ok {
				hr.config.FallbackConfig.Enabled = enabled
			}
		case "log_level":
			if level, ok := value.(string); ok {
				hr.config.MonitoringConfig.LogLevel = level
			}
		default:
			// Manejar feature flags
			if strings.HasPrefix(key, "feature_flags.") {
				flagName := strings.TrimPrefix(key, "feature_flags.")
				if enabled, ok := value.(bool); ok {
					if hr.config.FeatureFlags == nil {
						hr.config.FeatureFlags = make(map[string]bool)
					}
					hr.config.FeatureFlags[flagName] = enabled
				}
			} else {
				// Configuraciones personalizadas
				if hr.config.CustomSettings == nil {
					hr.config.CustomSettings = make(map[string]string)
				}
				hr.config.CustomSettings[key] = fmt.Sprintf("%v", value)
			}
		}
	}

	hr.config.LastUpdated = time.Now()
	hr.config.Version++

	// Disparar callbacks
	go func() {
		for name, callbacks := range hr.callbacks {
			for _, callback := range callbacks {
				if err := callback(&oldConfig, hr.config); err != nil {
					hr.logger.Error(fmt.Sprintf("❌ Error en callback %s: %v", name, err))
					hr.recordCallbackError(name)
				}
			}
		}
	}()

	hr.logger.Info(fmt.Sprintf("✅ Configuración actualizada programáticamente (versión %d)", hr.config.Version))
	return nil
}

// SaveConfig guarda la configuración actual al archivo
func (hr *HotReloader) SaveConfig() error {
	hr.mutex.RLock()
	config := hr.config
	hr.mutex.RUnlock()

	if config == nil {
		return errors.New("no config to save")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal config")
	}

	if err := os.WriteFile(hr.configFile, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}

	hr.logger.Info(fmt.Sprintf("💾 Configuración guardada en: %s", hr.configFile))
	return nil
}

// GetLastReloadTime obtiene el tiempo de la última recarga
func (hr *HotReloader) GetLastReloadTime() time.Time {
	hr.mutex.RLock()
	defer hr.mutex.RUnlock()
	return hr.lastReload
}

// IsRunning verifica si el sistema está corriendo
func (hr *HotReloader) IsRunning() bool {
	hr.mutex.RLock()
	defer hr.mutex.RUnlock()
	return hr.isRunning
}

// GetWatchedFiles obtiene la lista de archivos observados
func (hr *HotReloader) GetWatchedFiles() []string {
	hr.mutex.RLock()
	defer hr.mutex.RUnlock()

	files := make([]string, 0, len(hr.watchedFiles))
	for file := range hr.watchedFiles {
		files = append(files, file)
	}

	return files
}

// IsFeatureEnabled verifica si una feature flag está habilitada
func (hr *HotReloader) IsFeatureEnabled(featureName string) bool {
	config := hr.GetConfig()
	if config.FeatureFlags == nil {
		return false
	}
	
	enabled, exists := config.FeatureFlags[featureName]
	return exists && enabled
}

// logReloadEvent registra un evento de recarga para auditoría
func (hr *HotReloader) logReloadEvent(event ReloadEvent) {
	logFile := "/tmp/gopherkit_config_reload.log"

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		hr.logger.Warn(fmt.Sprintf("⚠️ No se pudo abrir log de eventos: %v", err))
		return
	}
	defer file.Close()

	eventJSON, _ := json.Marshal(event)
	fmt.Fprintf(file, "[%s] %s\n", event.Timestamp.Format("2006-01-02 15:04:05"), eventJSON)
}