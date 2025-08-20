// Package database - Fallback Management System
// Migrado de ClubPulse a gopherkit con mejoras significativas
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// FallbackManager gestiona el fallback automático entre modos de database mejorado
type FallbackManager struct {
	config              *Config
	primaryDB           *sql.DB
	fallbackConnections map[string]*sql.DB
	healthChecker       *HealthChecker
	isInFallbackMode    bool
	fallbackMutex       sync.RWMutex
	monitorCancel       context.CancelFunc
	alertCallback       func(string, error)
	metrics             *FallbackMetrics
	logger              Logger
	wg                  sync.WaitGroup
	ctx                 context.Context
}

// FallbackConfig configuración mejorada para el sistema de fallback
type FallbackConfig struct {
	EnableAutoFallback     bool          `json:"enable_auto_fallback"`
	HealthCheckInterval    time.Duration `json:"health_check_interval"`
	FailureThreshold       int           `json:"failure_threshold"`
	RecoveryCheckInterval  time.Duration `json:"recovery_check_interval"`
	FallbackTimeout        time.Duration `json:"fallback_timeout"`
	AlertWebhookURL        string        `json:"alert_webhook_url"`
	MaxRetryAttempts       int           `json:"max_retry_attempts"`
	RetryBackoffMultiplier float64       `json:"retry_backoff_multiplier"`
	EnableNotifications    bool          `json:"enable_notifications"`
	NotificationChannels   []string      `json:"notification_channels"`
	AutoRecoveryEnabled    bool          `json:"auto_recovery_enabled"`
	RecoveryConfidence     float64       `json:"recovery_confidence"`
}

// DefaultFallbackConfig retorna configuración por defecto mejorada
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		EnableAutoFallback:     true,
		HealthCheckInterval:    30 * time.Second,
		FailureThreshold:       3,
		RecoveryCheckInterval:  60 * time.Second,
		FallbackTimeout:        5 * time.Minute,
		MaxRetryAttempts:       5,
		RetryBackoffMultiplier: 2.0,
		EnableNotifications:    true,
		NotificationChannels:   []string{"log", "webhook"},
		AutoRecoveryEnabled:    true,
		RecoveryConfidence:     0.8,
	}
}

// FallbackMetrics métricas del sistema de fallback
type FallbackMetrics struct {
	FallbackActivations   int64                    `json:"fallback_activations"`
	RecoveryAttempts      int64                    `json:"recovery_attempts"`
	SuccessfulRecoveries  int64                    `json:"successful_recoveries"`
	FailedRecoveries      int64                    `json:"failed_recoveries"`
	TimeInFallbackMode    time.Duration            `json:"time_in_fallback_mode"`
	LastFallbackTime      time.Time                `json:"last_fallback_time"`
	LastRecoveryTime      time.Time                `json:"last_recovery_time"`
	HealthCheckFailures   map[string]int64         `json:"health_check_failures"`
	FallbackDuration      map[string]time.Duration `json:"fallback_duration"`
	AlertsSent            int64                    `json:"alerts_sent"`
	mu                    sync.RWMutex
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

func (dl *DefaultLogger) Info(msg string)                                    { fmt.Printf("INFO: %s\n", msg) }
func (dl *DefaultLogger) Warn(msg string)                                    { fmt.Printf("WARN: %s\n", msg) }
func (dl *DefaultLogger) Error(msg string)                                   { fmt.Printf("ERROR: %s\n", msg) }
func (dl *DefaultLogger) Debugf(format string, args ...interface{})         { fmt.Printf("DEBUG: "+format+"\n", args...) }
func (dl *DefaultLogger) WithField(key string, value interface{}) Logger    { return dl }

// NewFallbackManager crea un nuevo gestor de fallback mejorado
func NewFallbackManager(config *Config, logger Logger) (*FallbackManager, error) {
	if logger == nil {
		logger = &DefaultLogger{}
	}

	ctx, cancel := context.WithCancel(context.Background())

	fm := &FallbackManager{
		config:              config,
		fallbackConnections: make(map[string]*sql.DB),
		healthChecker:       NewHealthChecker(config),
		isInFallbackMode:    false,
		monitorCancel:       cancel,
		logger:              logger,
		ctx:                 ctx,
		metrics: &FallbackMetrics{
			HealthCheckFailures: make(map[string]int64),
			FallbackDuration:    make(map[string]time.Duration),
		},
	}

	// Configurar callback de alertas por defecto
	fm.alertCallback = fm.defaultAlertHandler

	// Inicializar conexiones de fallback
	if err := fm.initializeFallbackConnections(); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to initialize fallback connections")
	}

	// Iniciar monitoreo si está habilitado
	fallbackConfig := fm.getFallbackConfig()
	if fallbackConfig.EnableAutoFallback {
		fm.startHealthMonitoring()
	}

	return fm, nil
}

// initializeFallbackConnections prepara las conexiones de fallback con mejoras
func (fm *FallbackManager) initializeFallbackConnections() error {
	fm.logger.Info("🔄 Inicializando conexiones de fallback mejoradas...")

	// Si estamos en modo single, preparar conexiones multi como fallback
	if fm.config.DatabaseMode == ModeSingle {
		services := map[string]string{
			"auth":         "5433",
			"user":         "5434", 
			"calendar":     "5435",
			"championship": "5436",
			"membership":   "5437",
			"facilities":   "5439",
			"notification": "5440",
			"booking":      "5437",
			"payments":     "5441",
			"super-admin":  "5442",
		}

		for service, port := range services {
			fallbackURL := fmt.Sprintf("postgresql://postgres:password@localhost:%s/%s_db?sslmode=disable", port, service)

			// Intentar conexión de fallback con timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if db, err := sql.Open("postgres", fallbackURL); err == nil {
				if err := db.PingContext(ctx); err == nil {
					fm.fallbackConnections[service] = db
					fm.logger.WithField("service", service).Info("✅ Conexión de fallback preparada")
				} else {
					db.Close()
					fm.logger.WithField("service", service).Warn("⚠️ Conexión de fallback no disponible")
					fm.recordHealthCheckFailure(service)
				}
			}
			cancel()
		}
	} else {
		// Si estamos en modo multi, preparar conexión single como fallback
		fallbackURL := os.Getenv("UNIFIED_DATABASE_URL")
		if fallbackURL == "" {
			fallbackURL = "postgresql://postgres:password@localhost:5432/gopherkit_unified?sslmode=disable"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if db, err := sql.Open("postgres", fallbackURL); err == nil {
			if err := db.PingContext(ctx); err == nil {
				fm.fallbackConnections["unified"] = db
				fm.logger.Info("✅ Conexión de fallback unificada preparada")
			} else {
				db.Close()
				fm.logger.Warn("⚠️ Conexión de fallback unificada no disponible")
				fm.recordHealthCheckFailure("unified")
			}
		}
	}

	return nil
}

// startHealthMonitoring inicia el monitoreo continuo de salud mejorado
func (fm *FallbackManager) startHealthMonitoring() {
	fm.logger.Info("🏥 Iniciando monitoreo de salud automático mejorado...")

	fm.wg.Add(1)
	go func() {
		defer fm.wg.Done()
		
		config := fm.getFallbackConfig()
		ticker := time.NewTicker(config.HealthCheckInterval)
		defer ticker.Stop()

		consecutiveFailures := 0
		recoveryAttempts := 0
		lastFallbackTime := time.Time{}

		for {
			select {
			case <-fm.ctx.Done():
				fm.logger.Info("🛑 Monitoreo de salud detenido")
				return
			case <-ticker.C:
				if err := fm.performHealthCheck(); err != nil {
					consecutiveFailures++
					fm.logger.Debugf("❌ Health check falló (%d/%d): %v", consecutiveFailures, config.FailureThreshold, err)

					// Si alcanzamos el threshold, activar fallback
					if consecutiveFailures >= config.FailureThreshold && !fm.isInFallbackMode {
						if err := fm.activateFallback(); err != nil {
							fm.sendAlert("Fallback activation failed", err)
						} else {
							lastFallbackTime = time.Now()
							recoveryAttempts = 0
						}
					}
				} else {
					// Si estamos en fallback y la conexión primaria se recuperó
					if fm.isInFallbackMode && consecutiveFailures > 0 {
						recoveryAttempts++
						fm.logger.Info("✅ Conexión primaria recuperada, evaluando retorno...")
						
						// Verificar confianza en la recuperación
						if fm.shouldAttemptRecovery(recoveryAttempts, lastFallbackTime) {
							if err := fm.attemptRecovery(); err != nil {
								fm.logger.Warn("⚠️ Intento de recuperación falló")
								recoveryAttempts = 0
							} else {
								// Registrar tiempo en fallback
								if !lastFallbackTime.IsZero() {
									fallbackDuration := time.Since(lastFallbackTime)
									fm.recordFallbackDuration("primary", fallbackDuration)
								}
								recoveryAttempts = 0
							}
						}
					}
					consecutiveFailures = 0
				}
			}
		}
	}()
}

// shouldAttemptRecovery determina si debe intentar recuperación basado en métricas
func (fm *FallbackManager) shouldAttemptRecovery(attempts int, lastFallbackTime time.Time) bool {
	config := fm.getFallbackConfig()
	
	// Si la auto-recuperación está deshabilitada
	if !config.AutoRecoveryEnabled {
		return false
	}
	
	// Esperar un mínimo de tiempo antes de intentar recuperación
	if time.Since(lastFallbackTime) < config.RecoveryCheckInterval {
		return false
	}
	
	// Límite de intentos de recuperación
	if attempts > config.MaxRetryAttempts {
		return false
	}
	
	// Calcular confianza basada en historial
	confidence := fm.calculateRecoveryConfidence()
	return confidence >= config.RecoveryConfidence
}

// calculateRecoveryConfidence calcula la confianza en la recuperación
func (fm *FallbackManager) calculateRecoveryConfidence() float64 {
	fm.metrics.mu.RLock()
	defer fm.metrics.mu.RUnlock()
	
	totalAttempts := fm.metrics.RecoveryAttempts
	if totalAttempts == 0 {
		return 1.0 // Primera vez, alta confianza
	}
	
	successRate := float64(fm.metrics.SuccessfulRecoveries) / float64(totalAttempts)
	return successRate
}

// performHealthCheck ejecuta verificación de salud mejorada en la conexión primaria
func (fm *FallbackManager) performHealthCheck() error {
	if fm.primaryDB == nil {
		return errors.New("primary database connection is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Health check básico
	if err := fm.primaryDB.PingContext(ctx); err != nil {
		fm.recordHealthCheckFailure("primary")
		return errors.Wrap(err, "primary database ping failed")
	}

	// Health check avanzado según el modo
	if fm.config.DatabaseMode == ModeSingle {
		return fm.healthCheckUnified(ctx)
	}
	return fm.healthCheckMulti(ctx)
}

// healthCheckUnified verifica salud de la database unificada con mejoras
func (fm *FallbackManager) healthCheckUnified(ctx context.Context) error {
	// Verificar que los schemas existen
	schemas := []string{"auth_schema", "user_schema", "calendar_schema", "championship_schema"}

	for _, schema := range schemas {
		query := "SELECT 1 FROM information_schema.schemata WHERE schema_name = $1"
		var exists int
		if err := fm.primaryDB.QueryRowContext(ctx, query, schema).Scan(&exists); err != nil {
			fm.recordHealthCheckFailure(schema)
			return errors.Wrapf(err, "schema %s health check failed", schema)
		}
	}

	// Verificar tenant context functionality
	testTenantID := "550e8400-e29b-41d4-a716-446655440000"
	if _, err := fm.primaryDB.ExecContext(ctx, "SELECT set_tenant_context($1)", testTenantID); err != nil {
		fm.recordHealthCheckFailure("tenant_context")
		return errors.Wrap(err, "tenant context health check failed")
	}

	return nil
}

// healthCheckMulti verifica salud de las databases múltiples con mejoras
func (fm *FallbackManager) healthCheckMulti(ctx context.Context) error {
	// En modo multi, verificar que la database actual responde a queries básicas
	var count int
	if err := fm.primaryDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables").Scan(&count); err != nil {
		fm.recordHealthCheckFailure("multi_query")
		return errors.Wrap(err, "multi database health check failed")
	}

	// Verificar que hay un número mínimo de tablas
	if count < 5 {
		fm.recordHealthCheckFailure("table_count")
		return errors.Errorf("insufficient tables found: %d", count)
	}

	return nil
}

// activateFallback activa el modo de fallback con mejoras
func (fm *FallbackManager) activateFallback() error {
	fm.fallbackMutex.Lock()
	defer fm.fallbackMutex.Unlock()

	if fm.isInFallbackMode {
		return nil // Ya estamos en modo fallback
	}

	fm.logger.Error("🚨 ACTIVANDO FALLBACK AUTOMÁTICO...")

	// Determinar estrategia de fallback
	var fallbackStrategy string
	if fm.config.DatabaseMode == ModeSingle {
		fallbackStrategy = "single_to_multi"
	} else {
		fallbackStrategy = "multi_to_single"
	}

	fm.logger.Debugf("📋 Estrategia de fallback: %s", fallbackStrategy)

	// Registrar activación de fallback
	fm.metrics.mu.Lock()
	fm.metrics.FallbackActivations++
	fm.metrics.LastFallbackTime = time.Now()
	fm.metrics.mu.Unlock()

	// Ejecutar fallback según estrategia
	var err error
	switch fallbackStrategy {
	case "single_to_multi":
		err = fm.fallbackSingleToMulti()
	case "multi_to_single":
		err = fm.fallbackMultiToSingle()
	default:
		err = errors.New("unknown fallback strategy")
	}

	if err != nil {
		fm.sendAlert("Fallback activation failed", err)
		return err
	}

	fm.isInFallbackMode = true
	fm.sendAlert(fmt.Sprintf("Automatic fallback activated: %s", fallbackStrategy), nil)
	
	return nil
}

// fallbackSingleToMulti cambia de single database a múltiples databases con mejoras
func (fm *FallbackManager) fallbackSingleToMulti() error {
	fm.logger.Info("🔄 Ejecutando fallback Single → Multi Database...")

	// Verificar que tenemos conexiones de fallback disponibles
	if len(fm.fallbackConnections) == 0 {
		return errors.New("no fallback connections available for single to multi fallback")
	}

	// Probar las conexiones de fallback
	workingConnections := 0
	for service, db := range fm.fallbackConnections {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := db.PingContext(ctx); err == nil {
			workingConnections++
			fm.logger.Debugf("✅ Fallback connection verified: %s", service)
		} else {
			fm.logger.Debugf("❌ Fallback connection failed: %s", service)
		}
		cancel()
	}

	if workingConnections == 0 {
		return errors.New("no working fallback connections available")
	}

	// Cambiar configuración de entorno
	if err := fm.updateEnvironmentMode(ModeMulti); err != nil {
		return errors.Wrap(err, "failed to update environment mode")
	}

	// Actualizar configuración de conexión
	fm.config.DatabaseMode = ModeMulti

	fm.logger.Info("✅ Fallback Single → Multi completado")
	return nil
}

// fallbackMultiToSingle cambia de múltiples databases a single database con mejoras
func (fm *FallbackManager) fallbackMultiToSingle() error {
	fm.logger.Info("🔄 Ejecutando fallback Multi → Single Database...")

	// Verificar que tenemos conexión unificada disponible
	unifiedConn, exists := fm.fallbackConnections["unified"]
	if !exists || unifiedConn == nil {
		return errors.New("no unified database connection available for multi to single fallback")
	}

	// Probar la conexión unificada
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	if err := unifiedConn.PingContext(ctx); err != nil {
		return errors.Wrap(err, "unified database connection test failed")
	}

	// Cambiar configuración de entorno
	if err := fm.updateEnvironmentMode(ModeSingle); err != nil {
		return errors.Wrap(err, "failed to update environment mode")
	}

	// Actualizar configuración de conexión
	fm.config.DatabaseMode = ModeSingle
	fm.primaryDB = unifiedConn

	fm.logger.Info("✅ Fallback Multi → Single completado")
	return nil
}

// attemptRecovery intenta recuperar la conexión primaria con mejoras
func (fm *FallbackManager) attemptRecovery() error {
	fm.logger.Info("🔄 Intentando recuperación de conexión primaria...")

	fm.metrics.mu.Lock()
	fm.metrics.RecoveryAttempts++
	fm.metrics.mu.Unlock()

	// Verificar que la conexión primaria original está disponible
	originalURL := fm.getOriginalDatabaseURL()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if db, err := sql.Open("postgres", originalURL); err == nil {
		if err := db.PingContext(ctx); err == nil {
			// Realizar verificación completa antes de confirmar recuperación
			if err := fm.performRecoveryHealthCheck(ctx, db); err != nil {
				db.Close()
				fm.metrics.mu.Lock()
				fm.metrics.FailedRecoveries++
				fm.metrics.mu.Unlock()
				return errors.Wrap(err, "recovery health check failed")
			}

			// Conexión primaria recuperada
			fm.fallbackMutex.Lock()
			defer fm.fallbackMutex.Unlock()

			// Restaurar conexión primaria
			if fm.primaryDB != nil && fm.primaryDB != db {
				fm.primaryDB.Close()
			}
			fm.primaryDB = db

			// Restaurar configuración original
			originalMode := fm.getOriginalDatabaseMode()
			fm.config.DatabaseMode = originalMode
			fm.isInFallbackMode = false

			// Actualizar entorno
			fm.updateEnvironmentMode(originalMode)

			// Registrar recuperación exitosa
			fm.metrics.mu.Lock()
			fm.metrics.SuccessfulRecoveries++
			fm.metrics.LastRecoveryTime = time.Now()
			fm.metrics.mu.Unlock()

			// Enviar alerta de recuperación
			fm.sendAlert(fmt.Sprintf("Database recovery successful: back to %s mode", originalMode), nil)

			fm.logger.Info("✅ Recuperación completada exitosamente")
			return nil
		} else {
			db.Close()
		}
	}

	fm.metrics.mu.Lock()
	fm.metrics.FailedRecoveries++
	fm.metrics.mu.Unlock()

	return errors.New("primary database still not available")
}

// performRecoveryHealthCheck realiza una verificación completa antes de confirmar recuperación
func (fm *FallbackManager) performRecoveryHealthCheck(ctx context.Context, db *sql.DB) error {
	// Verificación básica
	if err := db.PingContext(ctx); err != nil {
		return errors.Wrap(err, "ping test failed")
	}

	// Verificar que podemos realizar queries básicas
	var tableCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables").Scan(&tableCount); err != nil {
		return errors.Wrap(err, "table count query failed")
	}

	// Verificar funcionalidad específica según el modo
	if fm.getOriginalDatabaseMode() == ModeSingle {
		// Test de tenant context para modo single
		testTenantID := "550e8400-e29b-41d4-a716-446655440000"
		if _, err := db.ExecContext(ctx, "SELECT set_tenant_context($1)", testTenantID); err != nil {
			return errors.Wrap(err, "tenant context test failed")
		}
	}

	return nil
}

// recordHealthCheckFailure registra fallas en health checks
func (fm *FallbackManager) recordHealthCheckFailure(component string) {
	fm.metrics.mu.Lock()
	defer fm.metrics.mu.Unlock()
	fm.metrics.HealthCheckFailures[component]++
}

// recordFallbackDuration registra duración en modo fallback
func (fm *FallbackManager) recordFallbackDuration(component string, duration time.Duration) {
	fm.metrics.mu.Lock()
	defer fm.metrics.mu.Unlock()
	fm.metrics.FallbackDuration[component] = duration
	fm.metrics.TimeInFallbackMode += duration
}

// updateEnvironmentMode actualiza las variables de entorno
func (fm *FallbackManager) updateEnvironmentMode(mode DatabaseMode) error {
	// Actualizar variable de entorno en memoria
	os.Setenv("DATABASE_MODE", string(mode))
	
	fm.logger.Debugf("🔧 Modo de database actualizado a: %s", mode)
	return nil
}

// getFallbackConfig obtiene configuración de fallback
func (fm *FallbackManager) getFallbackConfig() FallbackConfig {
	return FallbackConfig{
		EnableAutoFallback:     getEnvBool("ENABLE_AUTO_FALLBACK", true),
		HealthCheckInterval:    getEnvDuration("HEALTH_CHECK_INTERVAL", 30*time.Second),
		FailureThreshold:       getEnvInt("FAILURE_THRESHOLD", 3),
		RecoveryCheckInterval:  getEnvDuration("RECOVERY_CHECK_INTERVAL", 60*time.Second),
		FallbackTimeout:        getEnvDuration("FALLBACK_TIMEOUT", 300*time.Second),
		AlertWebhookURL:        os.Getenv("ALERT_WEBHOOK_URL"),
		MaxRetryAttempts:       getEnvInt("MAX_RETRY_ATTEMPTS", 5),
		RetryBackoffMultiplier: getEnvFloat64("RETRY_BACKOFF_MULTIPLIER", 2.0),
		EnableNotifications:    getEnvBool("ENABLE_NOTIFICATIONS", true),
		AutoRecoveryEnabled:    getEnvBool("AUTO_RECOVERY_ENABLED", true),
		RecoveryConfidence:     getEnvFloat64("RECOVERY_CONFIDENCE", 0.8),
	}
}

// getOriginalDatabaseURL obtiene la URL original de la database
func (fm *FallbackManager) getOriginalDatabaseURL() string {
	if fm.config.DatabaseMode == ModeSingle {
		url := os.Getenv("UNIFIED_DATABASE_URL")
		if url == "" {
			url = "postgresql://postgres:password@localhost:5432/gopherkit_unified?sslmode=disable"
		}
		return url
	}
	return fm.config.DatabaseURL
}

// getOriginalDatabaseMode obtiene el modo original de la database
func (fm *FallbackManager) getOriginalDatabaseMode() DatabaseMode {
	originalMode := os.Getenv("ORIGINAL_DATABASE_MODE")
	if originalMode == "" {
		return fm.config.DatabaseMode
	}
	return DatabaseMode(originalMode)
}

// sendAlert envía una alerta sobre eventos de fallback
func (fm *FallbackManager) sendAlert(message string, err error) {
	fm.metrics.mu.Lock()
	fm.metrics.AlertsSent++
	fm.metrics.mu.Unlock()

	if fm.alertCallback != nil {
		fm.alertCallback(message, err)
	}
}

// defaultAlertHandler maneja alertas por defecto mejorado
func (fm *FallbackManager) defaultAlertHandler(message string, err error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	if err != nil {
		fm.logger.Error(fmt.Sprintf("🚨 ALERT [%s]: %s - Error: %v", timestamp, message, err))
	} else {
		fm.logger.Info(fmt.Sprintf("📢 ALERT [%s]: %s", timestamp, message))
	}

	// Enviar webhook si está configurado
	config := fm.getFallbackConfig()
	if config.AlertWebhookURL != "" && config.EnableNotifications {
		fm.logger.Debugf("📤 Sending webhook to: %s", config.AlertWebhookURL)
		// Aquí se implementaría el envío real del webhook
	}
}

// SetAlertCallback permite configurar un callback personalizado para alertas
func (fm *FallbackManager) SetAlertCallback(callback func(string, error)) {
	fm.alertCallback = callback
}

// SetPrimaryDB establece la conexión primaria
func (fm *FallbackManager) SetPrimaryDB(db *sql.DB) {
	fm.fallbackMutex.Lock()
	defer fm.fallbackMutex.Unlock()
	fm.primaryDB = db
}

// IsInFallbackMode verifica si está en modo fallback
func (fm *FallbackManager) IsInFallbackMode() bool {
	fm.fallbackMutex.RLock()
	defer fm.fallbackMutex.RUnlock()
	return fm.isInFallbackMode
}

// GetCurrentMode obtiene el modo actual de la database
func (fm *FallbackManager) GetCurrentMode() DatabaseMode {
	return fm.config.DatabaseMode
}

// GetMetrics retorna las métricas del fallback manager
func (fm *FallbackManager) GetMetrics() *FallbackMetrics {
	fm.metrics.mu.RLock()
	defer fm.metrics.mu.RUnlock()
	
	// Retornar copia de las métricas
	metrics := *fm.metrics
	return &metrics
}

// ForceRecovery fuerza un intento de recuperación manual
func (fm *FallbackManager) ForceRecovery() error {
	fm.logger.Info("🔧 Forzando intento de recuperación manual...")
	return fm.attemptRecovery()
}

// Stop detiene el monitoreo de fallback con limpieza mejorada
func (fm *FallbackManager) Stop() {
	// Cancelar contexto
	if fm.monitorCancel != nil {
		fm.monitorCancel()
	}

	// Esperar que las goroutines terminen
	fm.wg.Wait()

	// Cerrar conexiones de fallback
	for service, db := range fm.fallbackConnections {
		if db != nil {
			db.Close()
			fm.logger.Debugf("🔒 Conexión de fallback cerrada: %s", service)
		}
	}

	fm.logger.Info("🛑 FallbackManager detenido completamente")
}

// Helper functions mejoradas para variables de entorno
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true"
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvFloat64(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var floatVal float64
		if _, err := fmt.Sscanf(value, "%f", &floatVal); err == nil {
			return floatVal
		}
	}
	return defaultValue
}