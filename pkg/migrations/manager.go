package migrations

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/monitoring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

var logger = logrus.WithField("component", "gopherkit.migrations")

// Manager handles database migrations with advanced features for both single and multi database modes
type Manager struct {
	config *Config
	db     *sql.DB
	
	// Funcionalidades avanzadas
	alertManager     *monitoring.AlertManager
	metricsCollector *MigrationsMetricsCollector
	
	// Estado interno
	migrationHistory []MigrationExecution
	mu               sync.RWMutex
	
	// Configuración de comportamiento
	enableMetrics      bool
	enableValidation   bool
	enableRollback     bool
	enableDryRun       bool
	maxRetryAttempts   int
	backupBeforeMigration bool
}

// Config holds migration configuration with enhanced options
type Config struct {
	DatabaseURL    string
	DatabaseMode   string // "single" or "multi"
	DatabaseSchema string // for single mode
	MigrationsPath string
	ServiceName    string
	AutoMigrate    bool
	TargetVersion  uint // 0 means latest
	
	// Configuración avanzada
	Environment           string
	EnableMetrics         bool
	EnableValidation      bool
	EnableRollback        bool
	EnableDryRun          bool
	MaxRetryAttempts      int
	BackupBeforeMigration bool
	ValidationTimeout     time.Duration
	LockTimeout           time.Duration
	
	// Integración con gopherkit
	BaseConfig   *config.BaseConfig
	AlertManager *monitoring.AlertManager
}

// MigrationInfo holds comprehensive information about a migration
type MigrationInfo struct {
	Version        uint                   `json:"version"`
	Dirty          bool                   `json:"dirty"`
	AppliedAt      time.Time              `json:"applied_at"`
	Source         string                 `json:"source"`
	ExecutionTime  time.Duration          `json:"execution_time"`
	Status         MigrationStatus        `json:"status"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Checksum       string                 `json:"checksum,omitempty"`
	RollbackPath   string                 `json:"rollback_path,omitempty"`
}

// MigrationStatus representa el estado de una migración
type MigrationStatus string

const (
	StatusPending   MigrationStatus = "pending"
	StatusRunning   MigrationStatus = "running"
	StatusCompleted MigrationStatus = "completed"
	StatusFailed    MigrationStatus = "failed"
	StatusRolledBack MigrationStatus = "rolled_back"
	StatusSkipped   MigrationStatus = "skipped"
)

// MigrationExecution represents a migration execution record
type MigrationExecution struct {
	ID            string                 `json:"id"`
	Version       uint                   `json:"version"`
	Direction     string                 `json:"direction"` // "up" or "down"
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Duration      time.Duration          `json:"duration"`
	Status        MigrationStatus        `json:"status"`
	Error         string                 `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	ServiceName   string                 `json:"service_name"`
	DatabaseMode  string                 `json:"database_mode"`
	Schema        string                 `json:"schema,omitempty"`
}

// MigrationsMetricsCollector recopila métricas de migraciones para Prometheus
type MigrationsMetricsCollector struct {
	migrationsTotal        *prometheus.CounterVec
	migrationDuration      *prometheus.HistogramVec
	migrationStatus        *prometheus.GaugeVec
	currentVersion         *prometheus.GaugeVec
	rollbacksTotal         prometheus.Counter
	validationFailures     prometheus.Counter
	mu                     sync.Mutex
}

// MigrationValidationResult representa el resultado de validación de una migración
type MigrationValidationResult struct {
	Valid        bool                   `json:"valid"`
	Issues       []ValidationIssue      `json:"issues,omitempty"`
	Warnings     []string               `json:"warnings,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	ValidatedAt  time.Time              `json:"validated_at"`
}

// ValidationIssue representa un problema encontrado durante la validación
type ValidationIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"` // "error", "warning", "info"
	Message     string `json:"message"`
	Suggestion  string `json:"suggestion,omitempty"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
}

// NewManager creates a new migration manager with enhanced capabilities
func NewManager(config *Config, db *sql.DB) *Manager {
	if config == nil {
		config = LoadDefaultConfig()
	}

	applyDefaultConfig(config)

	manager := &Manager{
		config:                config,
		db:                    db,
		migrationHistory:      make([]MigrationExecution, 0),
		enableMetrics:         config.EnableMetrics,
		enableValidation:      config.EnableValidation,
		enableRollback:        config.EnableRollback,
		enableDryRun:          config.EnableDryRun,
		maxRetryAttempts:      config.MaxRetryAttempts,
		backupBeforeMigration: config.BackupBeforeMigration,
	}

	// Inicializar recolector de métricas
	if config.EnableMetrics {
		manager.metricsCollector = NewMigrationsMetricsCollector(config.ServiceName)
	}

	// Configurar alert manager
	if config.AlertManager != nil {
		manager.alertManager = config.AlertManager
	}

	logger.WithFields(logrus.Fields{
		"service":         config.ServiceName,
		"database_mode":   config.DatabaseMode,
		"enable_metrics":  config.EnableMetrics,
		"enable_validation": config.EnableValidation,
		"enable_rollback": config.EnableRollback,
	}).Info("Migration Manager inicializado exitosamente")

	return manager
}

// LoadDefaultConfig loads migration configuration from environment with enhanced options
func LoadDefaultConfig() *Config {
	return &Config{
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		DatabaseMode:          getEnvOrDefault("DATABASE_MODE", "multi"),
		DatabaseSchema:        os.Getenv("DATABASE_SCHEMA"),
		MigrationsPath:        getEnvOrDefault("MIGRATIONS_PATH", "./migrations"),
		ServiceName:           getEnvOrDefault("SERVICE_NAME", "gopherkit-service"),
		Environment:           getEnvOrDefault("ENVIRONMENT", "development"),
		AutoMigrate:           getEnvOrDefault("AUTO_MIGRATE", "true") == "true",
		TargetVersion:         0, // Latest by default
		EnableMetrics:         getEnvOrDefault("MIGRATIONS_ENABLE_METRICS", "true") == "true",
		EnableValidation:      getEnvOrDefault("MIGRATIONS_ENABLE_VALIDATION", "true") == "true",
		EnableRollback:        getEnvOrDefault("MIGRATIONS_ENABLE_ROLLBACK", "true") == "true",
		EnableDryRun:          getEnvOrDefault("MIGRATIONS_ENABLE_DRY_RUN", "false") == "true",
		MaxRetryAttempts:      3,
		BackupBeforeMigration: getEnvOrDefault("MIGRATIONS_BACKUP_BEFORE", "false") == "true",
		ValidationTimeout:     30 * time.Second,
		LockTimeout:           5 * time.Minute,
	}
}

func applyDefaultConfig(config *Config) {
	if config.MaxRetryAttempts == 0 {
		config.MaxRetryAttempts = 3
	}
	if config.ValidationTimeout == 0 {
		config.ValidationTimeout = 30 * time.Second
	}
	if config.LockTimeout == 0 {
		config.LockTimeout = 5 * time.Minute
	}
}

// Run executes migrations based on the database mode with enhanced features
func (m *Manager) Run() error {
	start := time.Now()
	executionID := fmt.Sprintf("%s_%d", m.config.ServiceName, start.Unix())

	logger.WithFields(logrus.Fields{
		"execution_id":   executionID,
		"database_mode":  m.config.DatabaseMode,
		"service":        m.config.ServiceName,
		"auto_migrate":   m.config.AutoMigrate,
	}).Info("Iniciando ejecución de migraciones")

	// Pre-validation if enabled
	if m.enableValidation {
		if err := m.PreValidateMigrations(); err != nil {
			logger.WithError(err).Error("Pre-validación de migraciones falló")
			m.recordMigrationEvent(executionID, 0, "up", StatusFailed, err.Error())
			return fmt.Errorf("pre-validation failed: %w", err)
		}
	}

	// Create backup if enabled
	if m.backupBeforeMigration {
		if err := m.createBackup(); err != nil {
			logger.WithError(err).Warn("Backup antes de migración falló, continuando...")
		}
	}

	var err error
	if m.config.DatabaseMode == "single" {
		err = m.runSingleDatabaseMigrations(executionID)
	} else {
		err = m.runMultiDatabaseMigrations(executionID)
	}

	duration := time.Since(start)
	
	if err != nil {
		logger.WithFields(logrus.Fields{
			"execution_id": executionID,
			"duration_ms":  duration.Milliseconds(),
			"error":        err,
		}).Error("Ejecución de migraciones falló")
		
		// Send alert if configured
		if m.alertManager != nil {
			m.alertManager.FireAlert(m.config.ServiceName, "migrations_failed", 
				fmt.Sprintf("Migration failed for service %s: %v", m.config.ServiceName, err), "error", nil)
		}
		
		return err
	}

	logger.WithFields(logrus.Fields{
		"execution_id": executionID,
		"duration_ms":  duration.Milliseconds(),
	}).Info("Ejecución de migraciones completada exitosamente")

	// Record metrics
	if m.metricsCollector != nil {
		m.metricsCollector.RecordMigrationSuccess(m.config.ServiceName, duration)
	}

	return nil
}

// runSingleDatabaseMigrations handles migrations for single database mode with enhanced features
func (m *Manager) runSingleDatabaseMigrations(executionID string) error {
	logger.Infof("Running migrations for single database mode (schema: %s)", m.config.DatabaseSchema)

	// Create schema-specific migration instance
	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer migrateInstance.Close()

	// Set search path to the service schema
	if m.config.DatabaseSchema != "" {
		if err := m.setSearchPath(m.config.DatabaseSchema); err != nil {
			return fmt.Errorf("failed to set search path: %w", err)
		}
	}

	// Run migrations with enhanced error handling
	if err := m.executeMigrationsWithRetry(migrateInstance, executionID); err != nil {
		return fmt.Errorf("failed to execute migrations: %w", err)
	}

	// Update migration metadata
	if err := m.updateMigrationMetadata(); err != nil {
		logger.Warnf("Failed to update migration metadata: %v", err)
	}

	// Post-validation if enabled
	if m.enableValidation {
		if err := m.PostValidateMigrations(); err != nil {
			logger.WithError(err).Warn("Post-validación de migraciones falló")
		}
	}

	logger.Infof("✅ Migrations completed successfully for schema: %s", m.config.DatabaseSchema)
	return nil
}

// runMultiDatabaseMigrations handles migrations for multi database mode with enhanced features
func (m *Manager) runMultiDatabaseMigrations(executionID string) error {
	logger.Infof("Running migrations for multi database mode (service: %s)", m.config.ServiceName)

	// Create migration instance for dedicated database
	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer migrateInstance.Close()

	// Run migrations with enhanced error handling
	if err := m.executeMigrationsWithRetry(migrateInstance, executionID); err != nil {
		return fmt.Errorf("failed to execute migrations: %w", err)
	}

	logger.Infof("✅ Migrations completed successfully for service: %s", m.config.ServiceName)
	return nil
}

// createMigrateInstance creates a golang-migrate instance with enhanced configuration
func (m *Manager) createMigrateInstance() (*migrate.Migrate, error) {
	// Create postgres driver instance with enhanced config
	driver, err := postgres.WithInstance(m.db, &postgres.Config{
		MigrationsTable:       m.getMigrationsTableName(),
		SchemaName:           m.config.DatabaseSchema,
		DatabaseName:         "", // Use default
		StatementTimeout:     m.config.ValidationTimeout,
		MigrationsTableQuoted: false,
		MultiStatementEnabled: true,
		MultiStatementMaxSize: 10 << 20, // 10MB
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	// Determine migrations source path
	sourcePath := m.getMigrationsSourcePath()

	// Create migrate instance
	migrateInstance, err := migrate.NewWithDatabaseInstance(
		sourcePath,
		"postgres",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Configure migrate instance with timeout
	migrateInstance.LockTimeout = m.config.LockTimeout

	return migrateInstance, nil
}

// getMigrationsTableName returns the appropriate migrations table name with enhanced naming
func (m *Manager) getMigrationsTableName() string {
	if m.config.DatabaseMode == "single" && m.config.DatabaseSchema != "" {
		return fmt.Sprintf("%s_schema_migrations", m.config.DatabaseSchema)
	}
	return "schema_migrations"
}

// getMigrationsSourcePath returns the source path for migrations with enhanced path resolution
func (m *Manager) getMigrationsSourcePath() string {
	basePath := m.config.MigrationsPath

	if m.config.DatabaseMode == "single" {
		// Use schema-specific migrations if they exist
		schemaPath := filepath.Join(basePath, "schemas", m.config.DatabaseSchema)
		if _, err := os.Stat(schemaPath); err == nil {
			return fmt.Sprintf("file://%s", schemaPath)
		}

		// Fall back to unified migrations
		unifiedPath := filepath.Join(basePath, "unified")
		if _, err := os.Stat(unifiedPath); err == nil {
			return fmt.Sprintf("file://%s", unifiedPath)
		}
	}

	// Use service-specific migrations for multi mode
	servicePath := filepath.Join(basePath, m.config.ServiceName)
	if _, err := os.Stat(servicePath); err == nil {
		return fmt.Sprintf("file://%s", servicePath)
	}

	// Default to base migrations path
	return fmt.Sprintf("file://%s", basePath)
}

// setSearchPath sets the PostgreSQL search_path with enhanced error handling
func (m *Manager) setSearchPath(schema string) error {
	query := fmt.Sprintf("SET search_path TO %s, public", schema)
	_, err := m.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to set search_path to %s: %w", schema, err)
	}

	logger.Debugf("Set search_path to: %s", schema)
	return nil
}

// executeMigrationsWithRetry runs migrations with retry logic and enhanced error handling
func (m *Manager) executeMigrationsWithRetry(migrateInstance *migrate.Migrate, executionID string) error {
	var lastErr error
	
	for attempt := 1; attempt <= m.maxRetryAttempts; attempt++ {
		logger.WithFields(logrus.Fields{
			"attempt":      attempt,
			"max_attempts": m.maxRetryAttempts,
		}).Info("Attempting migration execution")

		err := m.executeMigrations(migrateInstance, executionID)
		if err == nil {
			return nil
		}

		lastErr = err
		logger.WithFields(logrus.Fields{
			"attempt": attempt,
			"error":   err,
		}).Warn("Migration attempt failed")

		// Don't retry on certain types of errors
		if isNonRetryableError(err) {
			logger.WithError(err).Error("Non-retryable error encountered")
			break
		}

		// Wait before retry (exponential backoff)
		if attempt < m.maxRetryAttempts {
			waitTime := time.Duration(attempt*attempt) * time.Second
			logger.Infof("Waiting %v before retry...", waitTime)
			time.Sleep(waitTime)
		}
	}

	return fmt.Errorf("migration failed after %d attempts: %w", m.maxRetryAttempts, lastErr)
}

// executeMigrations runs the actual migrations with enhanced monitoring
func (m *Manager) executeMigrations(migrateInstance *migrate.Migrate, executionID string) error {
	start := time.Now()

	// Get current version
	currentVersion, dirty, err := migrateInstance.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"current_version": currentVersion,
		"dirty":          dirty,
		"target_version": m.config.TargetVersion,
	}).Info("Migration status before execution")

	// Handle dirty state
	if dirty {
		logger.Warnf("Database is in dirty state at version %d, attempting to force version", currentVersion)
		if err := migrateInstance.Force(int(currentVersion)); err != nil {
			return fmt.Errorf("failed to force version %d: %w", currentVersion, err)
		}
		logger.Infof("Forced version %d to clean dirty state", currentVersion)
	}

	// Record migration start
	m.recordMigrationEvent(executionID, currentVersion, "up", StatusRunning, "")

	// Determine target version and execute
	targetVersion := m.config.TargetVersion
	if targetVersion == 0 {
		// Migrate to latest
		logger.Info("Migrating to latest version...")
		if err := migrateInstance.Up(); err != nil && err != migrate.ErrNoChange {
			m.recordMigrationEvent(executionID, currentVersion, "up", StatusFailed, err.Error())
			return fmt.Errorf("failed to migrate up: %w", err)
		}
	} else {
		// Migrate to specific version
		logger.Infof("Migrating to version %d...", targetVersion)
		if err := migrateInstance.Migrate(targetVersion); err != nil && err != migrate.ErrNoChange {
			m.recordMigrationEvent(executionID, currentVersion, "up", StatusFailed, err.Error())
			return fmt.Errorf("failed to migrate to version %d: %w", targetVersion, err)
		}
	}

	// Get final version
	finalVersion, dirty, err := migrateInstance.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get final version: %w", err)
	}

	if dirty {
		m.recordMigrationEvent(executionID, finalVersion, "up", StatusFailed, "Migration left database in dirty state")
		return fmt.Errorf("migration left database in dirty state at version %d", finalVersion)
	}

	duration := time.Since(start)

	if err == migrate.ErrNilVersion {
		logger.Info("No migrations to apply")
		m.recordMigrationEvent(executionID, 0, "up", StatusSkipped, "No migrations available")
	} else {
		logger.WithFields(logrus.Fields{
			"final_version": finalVersion,
			"duration_ms":   duration.Milliseconds(),
		}).Info("Database migrated successfully")
		m.recordMigrationEvent(executionID, finalVersion, "up", StatusCompleted, "")
	}

	// Record metrics
	if m.metricsCollector != nil {
		m.metricsCollector.RecordMigration(m.config.ServiceName, "up", finalVersion, duration)
	}

	return nil
}

// updateMigrationMetadata updates metadata about migrations with enhanced information
func (m *Manager) updateMigrationMetadata() error {
	// Create metadata table if it doesn't exist
	metadataTable := m.getMetadataTableName()
	createTableQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			service_name VARCHAR(100) NOT NULL,
			schema_name VARCHAR(100),
			migration_version BIGINT,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			database_mode VARCHAR(20) NOT NULL,
			environment VARCHAR(50),
			execution_id VARCHAR(100),
			duration_ms BIGINT,
			status VARCHAR(20) DEFAULT 'completed',
			error_message TEXT,
			metadata JSONB DEFAULT '{}',
			checksum VARCHAR(64),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, metadataTable)

	if _, err := m.db.Exec(createTableQuery); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// Insert or update metadata
	upsertQuery := fmt.Sprintf(`
		INSERT INTO %s (service_name, schema_name, migration_version, database_mode, environment, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (service_name, COALESCE(schema_name, ''))
		DO UPDATE SET
			migration_version = $3,
			applied_at = CURRENT_TIMESTAMP,
			database_mode = $4,
			environment = $5,
			metadata = $6,
			updated_at = CURRENT_TIMESTAMP
	`, metadataTable)

	// Get current migration version
	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return err
	}
	defer migrateInstance.Close()

	version, _, err := migrateInstance.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return err
	}

	metadata := map[string]interface{}{
		"auto_migrate":       m.config.AutoMigrate,
		"migrations_path":    m.config.MigrationsPath,
		"applied_by":         "gopherkit-migration-manager",
		"enable_metrics":     m.config.EnableMetrics,
		"enable_validation":  m.config.EnableValidation,
		"enable_rollback":    m.config.EnableRollback,
		"max_retry_attempts": m.config.MaxRetryAttempts,
		"golang_version":     "go1.21+",
		"manager_version":    "2.0.0",
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	_, err = m.db.Exec(upsertQuery,
		m.config.ServiceName,
		m.config.DatabaseSchema,
		version,
		m.config.DatabaseMode,
		m.config.Environment,
		string(metadataJSON),
	)

	return err
}

// getMetadataTableName returns the metadata table name with enhanced naming
func (m *Manager) getMetadataTableName() string {
	if m.config.DatabaseMode == "single" {
		return "shared_schema.migration_metadata"
	}
	return "migration_metadata"
}

// recordMigrationEvent registra un evento de migración en el historial
func (m *Manager) recordMigrationEvent(executionID string, version uint, direction string, status MigrationStatus, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	execution := MigrationExecution{
		ID:           executionID,
		Version:      version,
		Direction:    direction,
		StartedAt:    time.Now(),
		Status:       status,
		Error:        errorMsg,
		ServiceName:  m.config.ServiceName,
		DatabaseMode: m.config.DatabaseMode,
		Schema:       m.config.DatabaseSchema,
		Metadata: map[string]interface{}{
			"environment":      m.config.Environment,
			"auto_migrate":     m.config.AutoMigrate,
			"migrations_path":  m.config.MigrationsPath,
		},
	}

	if status == StatusCompleted || status == StatusFailed {
		now := time.Now()
		execution.CompletedAt = &now
		execution.Duration = now.Sub(execution.StartedAt)
	}

	m.migrationHistory = append(m.migrationHistory, execution)

	// Mantener solo los últimos 100 registros
	if len(m.migrationHistory) > 100 {
		m.migrationHistory = m.migrationHistory[len(m.migrationHistory)-100:]
	}
}

// GetMigrationInfo returns comprehensive information about current migrations
func (m *Manager) GetMigrationInfo() (*MigrationInfo, error) {
	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return nil, err
	}
	defer migrateInstance.Close()

	start := time.Now()
	version, dirty, err := migrateInstance.Version()
	executionTime := time.Since(start)
	
	if err != nil && err != migrate.ErrNilVersion {
		return nil, err
	}

	status := StatusCompleted
	if dirty {
		status = StatusFailed
	}

	info := &MigrationInfo{
		Version:       version,
		Dirty:         dirty,
		Source:        m.getMigrationsSourcePath(),
		ExecutionTime: executionTime,
		Status:        status,
		Metadata: map[string]interface{}{
			"service_name":     m.config.ServiceName,
			"database_mode":    m.config.DatabaseMode,
			"schema":           m.config.DatabaseSchema,
			"environment":      m.config.Environment,
			"auto_migrate":     m.config.AutoMigrate,
			"enable_metrics":   m.config.EnableMetrics,
			"enable_validation": m.config.EnableValidation,
		},
	}

	if err == migrate.ErrNilVersion {
		info.Version = 0
		info.Status = StatusPending
	}

	// Get applied timestamp from metadata
	metadataTable := m.getMetadataTableName()
	query := fmt.Sprintf(`
		SELECT applied_at, status, error_message FROM %s 
		WHERE service_name = $1 AND COALESCE(schema_name, '') = COALESCE($2, '')
		ORDER BY applied_at DESC LIMIT 1
	`, metadataTable)

	var appliedAt time.Time
	var dbStatus string
	var errorMessage sql.NullString
	err = m.db.QueryRow(query, m.config.ServiceName, m.config.DatabaseSchema).Scan(&appliedAt, &dbStatus, &errorMessage)
	if err == nil {
		info.AppliedAt = appliedAt
		if errorMessage.Valid {
			info.ErrorMessage = errorMessage.String
		}
	}

	return info, nil
}

// GetMigrationHistory retorna el historial de ejecuciones de migraciones
func (m *Manager) GetMigrationHistory() []MigrationExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Crear copia para evitar race conditions
	history := make([]MigrationExecution, len(m.migrationHistory))
	copy(history, m.migrationHistory)
	
	return history
}

// PreValidateMigrations valida las migraciones antes de ejecutarlas
func (m *Manager) PreValidateMigrations() error {
	logger.Info("Ejecutando pre-validación de migraciones...")

	sourcePath := m.getMigrationsSourcePath()
	if !strings.HasPrefix(sourcePath, "file://") {
		return fmt.Errorf("unsupported source path format: %s", sourcePath)
	}

	migrationsDir := strings.TrimPrefix(sourcePath, "file://")
	
	// Verificar que el directorio exista
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		return fmt.Errorf("migrations directory does not exist: %s", migrationsDir)
	}

	// Leer archivos de migración
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Validar cada archivo de migración
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		filePath := filepath.Join(migrationsDir, file.Name())
		if err := m.validateMigrationFile(filePath); err != nil {
			logger.WithFields(logrus.Fields{
				"file":  file.Name(),
				"error": err,
			}).Warn("Migration file validation warning")
		}
	}

	logger.Info("✅ Pre-validación de migraciones completada")
	return nil
}

// validateMigrationFile valida un archivo de migración individual
func (m *Manager) validateMigrationFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	contentStr := string(content)
	
	// Validaciones básicas
	if len(contentStr) == 0 {
		return fmt.Errorf("migration file is empty")
	}

	// Verificar comandos peligrosos
	dangerousCommands := []string{
		"DROP DATABASE",
		"DROP SCHEMA",
		"TRUNCATE",
		"DELETE FROM",
	}

	for _, cmd := range dangerousCommands {
		if strings.Contains(strings.ToUpper(contentStr), cmd) {
			logger.WithFields(logrus.Fields{
				"file":    filepath.Base(filePath),
				"command": cmd,
			}).Warn("Potentially dangerous command detected in migration")
		}
	}

	return nil
}

// PostValidateMigrations valida el estado después de ejecutar migraciones
func (m *Manager) PostValidateMigrations() error {
	logger.Info("Ejecutando post-validación de migraciones...")

	// Verificar que no hay estado dirty
	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return err
	}
	defer migrateInstance.Close()

	_, dirty, err := migrateInstance.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		return fmt.Errorf("database is in dirty state after migration")
	}

	logger.Info("✅ Post-validación de migraciones completada")
	return nil
}

// createBackup crea un backup antes de ejecutar migraciones
func (m *Manager) createBackup() error {
	logger.Info("Creando backup antes de migraciones...")

	// Esta sería una implementación simplificada
	// En un entorno real, se integraría con herramientas de backup específicas
	backupDir := filepath.Join(m.config.MigrationsPath, "..", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("pre_migration_%s_%s.sql", m.config.ServiceName, timestamp))

	// Placeholder para comando de backup real
	logger.WithField("backup_file", backupFile).Info("Backup placeholder created")
	
	return nil
}

// Rollback rolls back to a specific version with enhanced features
func (m *Manager) Rollback(targetVersion uint) error {
	if !m.enableRollback {
		return fmt.Errorf("rollback is disabled in configuration")
	}

	executionID := fmt.Sprintf("rollback_%s_%d", m.config.ServiceName, time.Now().Unix())
	
	logger.WithFields(logrus.Fields{
		"execution_id":   executionID,
		"target_version": targetVersion,
	}).Info("Iniciando rollback de migraciones")

	m.recordMigrationEvent(executionID, targetVersion, "down", StatusRunning, "")

	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		m.recordMigrationEvent(executionID, targetVersion, "down", StatusFailed, err.Error())
		return err
	}
	defer migrateInstance.Close()

	start := time.Now()
	if err := migrateInstance.Migrate(targetVersion); err != nil {
		m.recordMigrationEvent(executionID, targetVersion, "down", StatusFailed, err.Error())
		return fmt.Errorf("failed to rollback to version %d: %w", targetVersion, err)
	}
	duration := time.Since(start)

	m.recordMigrationEvent(executionID, targetVersion, "down", StatusCompleted, "")

	// Record metrics
	if m.metricsCollector != nil {
		m.metricsCollector.RecordMigration(m.config.ServiceName, "down", targetVersion, duration)
		m.metricsCollector.RecordRollback()
	}

	// Send alert
	if m.alertManager != nil {
		m.alertManager.FireAlert(m.config.ServiceName, "migration_rollback", 
			fmt.Sprintf("Migration rolled back for service %s to version %d", m.config.ServiceName, targetVersion), "warning", nil)
	}

	logger.WithFields(logrus.Fields{
		"target_version": targetVersion,
		"duration_ms":    duration.Milliseconds(),
	}).Info("✅ Rollback completado exitosamente")

	return nil
}

// Reset drops all tables and re-runs migrations with enhanced safety (dangerous!)
func (m *Manager) Reset() error {
	if m.config.Environment == "production" {
		return fmt.Errorf("reset operation is not allowed in production environment")
	}

	logger.Warn("🚨 Resetting database - this will drop all tables!")

	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return err
	}
	defer migrateInstance.Close()

	if err := migrateInstance.Drop(); err != nil {
		return fmt.Errorf("failed to drop database: %w", err)
	}

	if err := migrateInstance.Up(); err != nil {
		return fmt.Errorf("failed to re-run migrations: %w", err)
	}

	logger.Info("✅ Database reset completed")
	return nil
}

// ValidateMigrations checks if migrations are consistent with enhanced validation
func (m *Manager) ValidateMigrations() error {
	logger.Info("Validating migrations...")

	migrateInstance, err := m.createMigrateInstance()
	if err != nil {
		return err
	}
	defer migrateInstance.Close()

	// Check if database is dirty
	_, dirty, err := migrateInstance.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get version: %w", err)
	}

	if dirty {
		if m.metricsCollector != nil {
			m.metricsCollector.RecordValidationFailure()
		}
		return fmt.Errorf("database is in dirty state - manual intervention required")
	}

	// Additional validations for single database mode
	if m.config.DatabaseMode == "single" {
		if err := m.validateSchemaIntegrity(); err != nil {
			if m.metricsCollector != nil {
				m.metricsCollector.RecordValidationFailure()
			}
			return fmt.Errorf("schema integrity validation failed: %w", err)
		}
	}

	logger.Info("✅ Migration validation passed")
	return nil
}

// validateSchemaIntegrity checks schema integrity for single database mode with enhanced checks
func (m *Manager) validateSchemaIntegrity() error {
	// Check if schema exists
	if m.config.DatabaseSchema != "" {
		var exists bool
		query := "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)"
		err := m.db.QueryRow(query, m.config.DatabaseSchema).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check schema existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("schema %s does not exist", m.config.DatabaseSchema)
		}
	}

	// Check if migration table exists
	migrationTable := m.getMigrationsTableName()
	var tableExists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = COALESCE($1, 'public') 
			AND table_name = $2
		)
	`
	err := m.db.QueryRow(query, m.config.DatabaseSchema, migrationTable).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("failed to check migration table existence: %w", err)
	}

	if !tableExists {
		logger.Warnf("Migration table %s does not exist yet", migrationTable)
	}

	return nil
}

// CreateMigration creates a new migration file with enhanced templates
func (m *Manager) CreateMigration(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("migration name cannot be empty")
	}

	// Clean the name
	cleanName := strings.ReplaceAll(strings.ToLower(name), " ", "_")
	timestamp := time.Now().Unix()

	// Determine target directory
	targetDir := m.getMigrationsTargetDir()
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// Create up and down migration files
	upFile := filepath.Join(targetDir, fmt.Sprintf("%d_%s.up.sql", timestamp, cleanName))
	downFile := filepath.Join(targetDir, fmt.Sprintf("%d_%s.down.sql", timestamp, cleanName))

	// Enhanced up migration template
	upContent := fmt.Sprintf(`-- Migration: %s
-- Created: %s
-- Database Mode: %s
-- Schema: %s
-- Service: %s
-- Environment: %s
-- Generated by: gopherkit-migration-manager v2.0

-- ===========================================
-- UP MIGRATION
-- ===========================================

-- TODO: Add your up migration here
-- Example:
-- CREATE TABLE example_table (
--     id SERIAL PRIMARY KEY,
--     name VARCHAR(255) NOT NULL,
--     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
-- );

`, name, time.Now().Format(time.RFC3339), m.config.DatabaseMode, m.config.DatabaseSchema, m.config.ServiceName, m.config.Environment)

	if err := os.WriteFile(upFile, []byte(upContent), 0644); err != nil {
		return fmt.Errorf("failed to create up migration file: %w", err)
	}

	// Enhanced down migration template
	downContent := fmt.Sprintf(`-- Migration: %s (DOWN)
-- Created: %s
-- Database Mode: %s
-- Schema: %s
-- Service: %s
-- Environment: %s
-- Generated by: gopherkit-migration-manager v2.0

-- ===========================================
-- DOWN MIGRATION (ROLLBACK)
-- ===========================================

-- TODO: Add your down migration here (to reverse the up migration)
-- Example:
-- DROP TABLE IF EXISTS example_table;

`, name, time.Now().Format(time.RFC3339), m.config.DatabaseMode, m.config.DatabaseSchema, m.config.ServiceName, m.config.Environment)

	if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
		return fmt.Errorf("failed to create down migration file: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"migration_name": name,
		"timestamp":      timestamp,
		"target_dir":     targetDir,
	}).Info("✅ Created migration files:")
	logger.Infof("  Up:   %s", upFile)
	logger.Infof("  Down: %s", downFile)

	return nil
}

// getMigrationsTargetDir returns the target directory for new migrations with enhanced path logic
func (m *Manager) getMigrationsTargetDir() string {
	basePath := m.config.MigrationsPath

	if m.config.DatabaseMode == "single" && m.config.DatabaseSchema != "" {
		return filepath.Join(basePath, "schemas", m.config.DatabaseSchema)
	}

	return filepath.Join(basePath, m.config.ServiceName)
}

// GetMigrationMetrics retorna métricas del migration manager
func (m *Manager) GetMigrationMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, _ := m.GetMigrationInfo()
	
	metrics := map[string]interface{}{
		"service_name":         m.config.ServiceName,
		"database_mode":        m.config.DatabaseMode,
		"schema":               m.config.DatabaseSchema,
		"environment":          m.config.Environment,
		"current_version":      0,
		"execution_history":    len(m.migrationHistory),
		"enable_metrics":       m.config.EnableMetrics,
		"enable_validation":    m.config.EnableValidation,
		"enable_rollback":      m.config.EnableRollback,
		"max_retry_attempts":   m.config.MaxRetryAttempts,
		"backup_before_migration": m.config.BackupBeforeMigration,
	}

	if info != nil {
		metrics["current_version"] = info.Version
		metrics["dirty"] = info.Dirty
		metrics["status"] = info.Status
		metrics["last_applied"] = info.AppliedAt
	}

	return metrics
}

// Utility functions

func isNonRetryableError(err error) bool {
	errorStr := strings.ToLower(err.Error())
	nonRetryablePatterns := []string{
		"syntax error",
		"relation does not exist",
		"column does not exist",
		"permission denied",
		"authentication failed",
		"duplicate",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// NewMigrationsMetricsCollector crea un recolector de métricas para migraciones
func NewMigrationsMetricsCollector(serviceName string) *MigrationsMetricsCollector {
	return &MigrationsMetricsCollector{
		migrationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gopherkit",
			Subsystem: "migrations",
			Name:      "total",
			Help:      "Total number of migrations executed",
		}, []string{"service", "direction", "status"}),
		
		migrationDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gopherkit",
			Subsystem: "migrations",
			Name:      "duration_seconds",
			Help:      "Migration execution duration",
			Buckets:   []float64{.1, .5, 1, 5, 10, 30, 60, 120, 300},
		}, []string{"service", "direction"}),
		
		migrationStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gopherkit",
			Subsystem: "migrations",
			Name:      "status",
			Help:      "Current migration status (0=healthy, 1=dirty, 2=failed)",
		}, []string{"service", "schema"}),
		
		currentVersion: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gopherkit",
			Subsystem: "migrations",
			Name:      "current_version",
			Help:      "Current migration version",
		}, []string{"service", "schema"}),
		
		rollbacksTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gopherkit",
			Subsystem: "migrations",
			Name:      "rollbacks_total",
			Help:      "Total number of migration rollbacks",
		}),
		
		validationFailures: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gopherkit",
			Subsystem: "migrations",
			Name:      "validation_failures_total",
			Help:      "Total number of migration validation failures",
		}),
	}
}

// RecordMigration registra una migración en las métricas
func (mmc *MigrationsMetricsCollector) RecordMigration(service, direction string, version uint, duration time.Duration) {
	mmc.mu.Lock()
	defer mmc.mu.Unlock()
	
	mmc.migrationsTotal.WithLabelValues(service, direction, "success").Inc()
	mmc.migrationDuration.WithLabelValues(service, direction).Observe(duration.Seconds())
	mmc.currentVersion.WithLabelValues(service, "").Set(float64(version))
}

// RecordMigrationSuccess registra una migración exitosa
func (mmc *MigrationsMetricsCollector) RecordMigrationSuccess(service string, duration time.Duration) {
	mmc.mu.Lock()
	defer mmc.mu.Unlock()
	
	mmc.migrationsTotal.WithLabelValues(service, "up", "success").Inc()
}

// RecordRollback registra un rollback
func (mmc *MigrationsMetricsCollector) RecordRollback() {
	mmc.rollbacksTotal.Inc()
}

// RecordValidationFailure registra un fallo de validación
func (mmc *MigrationsMetricsCollector) RecordValidationFailure() {
	mmc.validationFailures.Inc()
}

// LoadConfigFromBase carga configuración desde BaseConfig de gopherkit
func LoadConfigFromBase(baseConfig *config.BaseConfig, serviceName string) *Config {
	config := LoadDefaultConfig()
	config.ServiceName = serviceName
	config.Environment = baseConfig.Server.Environment
	config.BaseConfig = baseConfig
	
	// Configurar según el entorno
	if baseConfig.IsProduction() {
		config.EnableDryRun = false
		config.BackupBeforeMigration = true
		config.MaxRetryAttempts = 5
	}
	
	if baseConfig.Observability.MetricsEnabled {
		config.EnableMetrics = true
	}
	
	return config
}

// Helper function to get environment variable with default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}