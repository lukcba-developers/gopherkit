package connection

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresConfig configuración para conexión PostgreSQL
type PostgresConfig struct {
	Host                string
	Port                int
	Database            string
	Username            string
	Password            string
	SSLMode             string
	TimeZone            string
	
	// Connection Pool Settings
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetime     time.Duration
	ConnMaxIdleTime     time.Duration
	
	// Timeouts
	ConnectTimeout      time.Duration
	StatementTimeout    time.Duration
	
	// Logging
	LogLevel            logger.LogLevel
	SlowThreshold       time.Duration
	
	// Advanced Settings
	ApplicationName     string
	SearchPath          string
	PgBouncerMode       bool
}

// DefaultPostgresConfig retorna configuración por defecto
func DefaultPostgresConfig() *PostgresConfig {
	return &PostgresConfig{
		Host:                "localhost",
		Port:                5432,
		Database:            "postgres",
		Username:            "postgres",
		Password:            "",
		SSLMode:             "disable",
		TimeZone:            "UTC",
		MaxOpenConns:        25,
		MaxIdleConns:        10,
		ConnMaxLifetime:     time.Hour,
		ConnMaxIdleTime:     time.Minute * 30,
		ConnectTimeout:      time.Second * 10,
		StatementTimeout:    time.Second * 30,
		LogLevel:            logger.Warn,
		SlowThreshold:       time.Millisecond * 200,
		ApplicationName:     "gopherkit-app",
		SearchPath:          "public",
		PgBouncerMode:       false,
	}
}

// PostgresConnectionManager maneja conexiones PostgreSQL
type PostgresConnectionManager struct {
	config     *PostgresConfig
	logger     *logrus.Logger
	
	// Different connection types
	sqlDB      *sql.DB
	gormDB     *gorm.DB
	sqlxDB     *sqlx.DB
	
	// Connection string
	dsn        string
}

// NewPostgresConnectionManager crea un nuevo manager de conexiones
func NewPostgresConnectionManager(config *PostgresConfig, logger *logrus.Logger) *PostgresConnectionManager {
	if config == nil {
		config = DefaultPostgresConfig()
	}
	
	return &PostgresConnectionManager{
		config: config,
		logger: logger,
		dsn:    buildDSN(config),
	}
}

// Connect establece todas las conexiones de base de datos
func (m *PostgresConnectionManager) Connect(ctx context.Context) error {
	m.logger.WithFields(logrus.Fields{
		"host":     m.config.Host,
		"port":     m.config.Port,
		"database": m.config.Database,
		"username": m.config.Username,
	}).Info("Connecting to PostgreSQL database")

	// Connect with database/sql
	if err := m.connectSQL(ctx); err != nil {
		return fmt.Errorf("failed to connect with database/sql: %w", err)
	}

	// Connect with GORM
	if err := m.connectGORM(); err != nil {
		return fmt.Errorf("failed to connect with GORM: %w", err)
	}

	// Connect with SQLX
	if err := m.connectSQLX(ctx); err != nil {
		return fmt.Errorf("failed to connect with SQLX: %w", err)
	}

	m.logger.Info("Successfully connected to PostgreSQL database with all drivers")
	return nil
}

// connectSQL establece conexión con database/sql
func (m *PostgresConnectionManager) connectSQL(ctx context.Context) error {
	db, err := sql.Open("postgres", m.dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(m.config.MaxOpenConns)
	db.SetMaxIdleConns(m.config.MaxIdleConns)
	db.SetConnMaxLifetime(m.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(m.config.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, m.config.ConnectTimeout)
	defer cancel()
	
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	m.sqlDB = db
	return nil
}

// connectGORM establece conexión con GORM
func (m *PostgresConnectionManager) connectGORM() error {
	// Configure GORM logger
	gormLogger := logger.New(
		m.logger.WithField("component", "gorm"),
		logger.Config{
			SlowThreshold:             m.config.SlowThreshold,
			LogLevel:                  m.config.LogLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	// Configure GORM
	gormConfig := &gorm.Config{
		Logger:                 gormLogger,
		NowFunc:               func() time.Time { return time.Now().UTC() },
		PrepareStmt:           true,
		DisableForeignKeyConstraintWhenMigrating: false,
	}

	db, err := gorm.Open(postgres.Open(m.dsn), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to open GORM connection: %w", err)
	}

	// Get underlying sql.DB and configure pool
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(m.config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(m.config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(m.config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(m.config.ConnMaxIdleTime)

	m.gormDB = db
	return nil
}

// connectSQLX establece conexión con SQLX
func (m *PostgresConnectionManager) connectSQLX(ctx context.Context) error {
	db, err := sqlx.ConnectContext(ctx, "postgres", m.dsn)
	if err != nil {
		return fmt.Errorf("failed to connect with SQLX: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(m.config.MaxOpenConns)
	db.SetMaxIdleConns(m.config.MaxIdleConns)
	db.SetConnMaxLifetime(m.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(m.config.ConnMaxIdleTime)

	m.sqlxDB = db
	return nil
}

// Close cierra todas las conexiones
func (m *PostgresConnectionManager) Close() error {
	var errors []error

	if m.sqlDB != nil {
		if err := m.sqlDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close sql.DB: %w", err))
		}
	}

	if m.gormDB != nil {
		sqlDB, err := m.gormDB.DB()
		if err == nil {
			if err := sqlDB.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close GORM connection: %w", err))
			}
		}
	}

	if m.sqlxDB != nil {
		if err := m.sqlxDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close SQLX connection: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing database connections: %v", errors)
	}

	m.logger.Info("All database connections closed successfully")
	return nil
}

// GetSQL retorna la conexión database/sql
func (m *PostgresConnectionManager) GetSQL() *sql.DB {
	return m.sqlDB
}

// GetGORM retorna la conexión GORM
func (m *PostgresConnectionManager) GetGORM() *gorm.DB {
	return m.gormDB
}

// GetSQLX retorna la conexión SQLX
func (m *PostgresConnectionManager) GetSQLX() *sqlx.DB {
	return m.sqlxDB
}

// GetDSN retorna el DSN de conexión
func (m *PostgresConnectionManager) GetDSN() string {
	return m.dsn
}

// Ping verifica la conectividad de todas las conexiones
func (m *PostgresConnectionManager) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, m.config.ConnectTimeout)
	defer cancel()

	// Ping SQL connection
	if m.sqlDB != nil {
		if err := m.sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("SQL connection failed ping: %w", err)
		}
	}

	// Ping GORM connection
	if m.gormDB != nil {
		sqlDB, err := m.gormDB.DB()
		if err != nil {
			return fmt.Errorf("failed to get GORM underlying DB: %w", err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("GORM connection failed ping: %w", err)
		}
	}

	// Ping SQLX connection
	if m.sqlxDB != nil {
		if err := m.sqlxDB.PingContext(ctx); err != nil {
			return fmt.Errorf("SQLX connection failed ping: %w", err)
		}
	}

	return nil
}

// GetStats retorna estadísticas de las conexiones
func (m *PostgresConnectionManager) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if m.sqlDB != nil {
		dbStats := m.sqlDB.Stats()
		stats["sql"] = map[string]interface{}{
			"max_open_connections":   dbStats.MaxOpenConnections,
			"open_connections":       dbStats.OpenConnections,
			"in_use":                dbStats.InUse,
			"idle":                  dbStats.Idle,
			"wait_count":            dbStats.WaitCount,
			"wait_duration":         dbStats.WaitDuration.String(),
			"max_idle_closed":       dbStats.MaxIdleClosed,
			"max_idle_time_closed":  dbStats.MaxIdleTimeClosed,
			"max_lifetime_closed":   dbStats.MaxLifetimeClosed,
		}
	}

	if m.gormDB != nil {
		if sqlDB, err := m.gormDB.DB(); err == nil {
			dbStats := sqlDB.Stats()
			stats["gorm"] = map[string]interface{}{
				"max_open_connections":   dbStats.MaxOpenConnections,
				"open_connections":       dbStats.OpenConnections,
				"in_use":                dbStats.InUse,
				"idle":                  dbStats.Idle,
				"wait_count":            dbStats.WaitCount,
				"wait_duration":         dbStats.WaitDuration.String(),
			}
		}
	}

	if m.sqlxDB != nil {
		dbStats := m.sqlxDB.Stats()
		stats["sqlx"] = map[string]interface{}{
			"max_open_connections":   dbStats.MaxOpenConnections,
			"open_connections":       dbStats.OpenConnections,
			"in_use":                dbStats.InUse,
			"idle":                  dbStats.Idle,
			"wait_count":            dbStats.WaitCount,
			"wait_duration":         dbStats.WaitDuration.String(),
		}
	}

	return stats
}

// RunMigrations ejecuta migraciones usando GORM
func (m *PostgresConnectionManager) RunMigrations(models ...interface{}) error {
	if m.gormDB == nil {
		return fmt.Errorf("GORM connection not available")
	}

	m.logger.WithField("models_count", len(models)).Info("Running database migrations")

	for i, model := range models {
		m.logger.WithField("model_index", i+1).Debug("Running migration for model")
		
		if err := m.gormDB.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate model %d: %w", i+1, err)
		}
	}

	m.logger.Info("All database migrations completed successfully")
	return nil
}

// ExecuteInTransaction ejecuta una función dentro de una transacción
func (m *PostgresConnectionManager) ExecuteInTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if m.sqlDB == nil {
		return fmt.Errorf("SQL connection not available")
	}

	tx, err := m.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			m.logger.WithError(rollbackErr).Error("Failed to rollback transaction")
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ExecuteInGORMTransaction ejecuta una función dentro de una transacción GORM
func (m *PostgresConnectionManager) ExecuteInGORMTransaction(fn func(tx *gorm.DB) error) error {
	if m.gormDB == nil {
		return fmt.Errorf("GORM connection not available")
	}

	return m.gormDB.Transaction(fn)
}

// buildDSN construye el DSN de conexión PostgreSQL
func buildDSN(config *PostgresConfig) string {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		config.Host, config.Port, config.Username, config.Password, 
		config.Database, config.SSLMode, config.TimeZone)

	if config.ConnectTimeout > 0 {
		dsn += fmt.Sprintf(" connect_timeout=%d", int(config.ConnectTimeout.Seconds()))
	}

	if config.StatementTimeout > 0 {
		dsn += fmt.Sprintf(" statement_timeout=%d", int(config.StatementTimeout.Milliseconds()))
	}

	if config.ApplicationName != "" {
		dsn += fmt.Sprintf(" application_name=%s", config.ApplicationName)
	}

	if config.SearchPath != "" {
		dsn += fmt.Sprintf(" search_path=%s", config.SearchPath)
	}

	return dsn
}

// NewPostgresConfigFromEnv crea configuración desde variables de entorno
func NewPostgresConfigFromEnv() *PostgresConfig {
	// This would typically read from environment variables
	// For now, return default config - can be extended
	return DefaultPostgresConfig()
}

// ValidateConfig valida la configuración
func (c *PostgresConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if c.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.MaxOpenConns <= 0 {
		return fmt.Errorf("max open connections must be positive")
	}
	if c.MaxIdleConns < 0 {
		return fmt.Errorf("max idle connections cannot be negative")
	}
	if c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("max idle connections cannot exceed max open connections")
	}

	return nil
}

// Clone crea una copia de la configuración
func (c *PostgresConfig) Clone() *PostgresConfig {
	clone := *c
	return &clone
}