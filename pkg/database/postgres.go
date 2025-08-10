package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
)

// PostgresClient provides PostgreSQL database operations
type PostgresClient struct {
	db     *gorm.DB
	sqlDB  *sql.DB
	logger logger.Logger
	config config.DatabaseConfig
}

// PostgresOptions holds options for PostgreSQL client
type PostgresOptions struct {
	Config config.DatabaseConfig
	Logger logger.Logger
	Models []interface{} // Models to auto-migrate
}

// NewPostgresClient creates a new PostgreSQL client
func NewPostgresClient(opts PostgresOptions) (*PostgresClient, error) {
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	client := &PostgresClient{
		config: opts.Config,
		logger: opts.Logger,
	}

	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := client.configure(); err != nil {
		return nil, fmt.Errorf("failed to configure database: %w", err)
	}

	// Auto-migrate models if provided
	if len(opts.Models) > 0 {
		if err := client.autoMigrate(opts.Models...); err != nil {
			return nil, fmt.Errorf("failed to auto-migrate models: %w", err)
		}
	}

	opts.Logger.LogBusinessEvent(context.Background(), "database_connected", map[string]interface{}{
		"host":     opts.Config.Host,
		"database": opts.Config.Database,
	})

	return client, nil
}

// connect establishes database connection
func (pc *PostgresClient) connect() error {
	dsn := pc.config.GetDSN()

	// Create custom logger for GORM
	gormLogger := NewGormLogger(pc.logger)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
		NamingStrategy: gorm.NamingStrategy{
			TablePrefix:   "",
			SingularTable: false,
		},
		DisableForeignKeyConstraintWhenMigrating: false,
		SkipDefaultTransaction:                   true, // Better performance
	})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	pc.db = db

	// Get underlying sql.DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	pc.sqlDB = sqlDB

	return nil
}

// configure sets up connection pool and other database settings
func (pc *PostgresClient) configure() error {
	if pc.config.MaxOpenConns > 0 {
		pc.sqlDB.SetMaxOpenConns(pc.config.MaxOpenConns)
	}

	if pc.config.MaxIdleConns > 0 {
		pc.sqlDB.SetMaxIdleConns(pc.config.MaxIdleConns)
	}

	if pc.config.ConnMaxLifetime > 0 {
		pc.sqlDB.SetConnMaxLifetime(pc.config.ConnMaxLifetime)
	}

	if pc.config.ConnMaxIdleTime > 0 {
		pc.sqlDB.SetConnMaxIdleTime(pc.config.ConnMaxIdleTime)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pc.sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// autoMigrate runs auto-migration for provided models
func (pc *PostgresClient) autoMigrate(models ...interface{}) error {
	return pc.db.AutoMigrate(models...)
}

// GetDB returns the GORM database instance
func (pc *PostgresClient) GetDB() *gorm.DB {
	return pc.db
}

// GetSQLDB returns the underlying sql.DB instance
func (pc *PostgresClient) GetSQLDB() *sql.DB {
	return pc.sqlDB
}

// WithContext returns a new GORM DB instance with context
func (pc *PostgresClient) WithContext(ctx context.Context) *gorm.DB {
	return pc.db.WithContext(ctx)
}

// Transaction executes a function within a database transaction
func (pc *PostgresClient) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return pc.db.WithContext(ctx).Transaction(fn)
}

// Close closes the database connection
func (pc *PostgresClient) Close() error {
	if pc.sqlDB != nil {
		return pc.sqlDB.Close()
	}
	return nil
}

// HealthCheck returns a health check for the database
func (pc *PostgresClient) HealthCheck() observability.HealthCheck {
	return observability.NewDatabaseHealthCheck("postgres", pc.sqlDB)
}

// Stats returns database connection statistics
func (pc *PostgresClient) Stats() sql.DBStats {
	return pc.sqlDB.Stats()
}

// Custom GORM logger that integrates with our logger
type GormLogger struct {
	logger logger.Logger
	level  gormlogger.LogLevel
}

// NewGormLogger creates a new GORM logger
func NewGormLogger(logger logger.Logger) *GormLogger {
	return &GormLogger{
		logger: logger,
		level:  gormlogger.Info,
	}
}

// LogMode sets the log level
func (gl *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &GormLogger{
		logger: gl.logger,
		level:  level,
	}
}

// Info logs info messages
func (gl *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if gl.level >= gormlogger.Info {
		gl.logger.WithContext(ctx).WithFields(map[string]interface{}{
			"component": "gorm",
			"level":     "info",
		}).Info(fmt.Sprintf(msg, data...))
	}
}

// Warn logs warning messages
func (gl *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if gl.level >= gormlogger.Warn {
		gl.logger.WithContext(ctx).WithFields(map[string]interface{}{
			"component": "gorm",
			"level":     "warn",
		}).Warn(fmt.Sprintf(msg, data...))
	}
}

// Error logs error messages
func (gl *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if gl.level >= gormlogger.Error {
		gl.logger.LogError(ctx, nil, fmt.Sprintf(msg, data...), map[string]interface{}{
			"component": "gorm",
		})
	}
}

// Trace logs SQL queries
func (gl *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if gl.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := map[string]interface{}{
		"component":      "gorm",
		"duration_ms":    elapsed.Milliseconds(),
		"rows_affected":  rows,
		"sql":           maskSensitiveSQL(sql),
	}

	if err != nil {
		gl.logger.LogError(ctx, err, "database query failed", fields)
		return
	}

	// Log slow queries as warnings
	if elapsed > 100*time.Millisecond {
		gl.logger.WithContext(ctx).WithFields(fields).Warn("slow database query")
	} else if gl.level >= gormlogger.Info {
		gl.logger.WithContext(ctx).WithFields(fields).Debug("database query executed")
	}
}

// Helper function to mask sensitive data in SQL queries
func maskSensitiveSQL(sql string) string {
	// In production, implement proper SQL masking to prevent logging sensitive data
	// This is a basic implementation
	return sql
}

// Repository provides common repository patterns
type Repository struct {
	db     *gorm.DB
	logger logger.Logger
}

// NewRepository creates a new repository instance
func NewRepository(db *gorm.DB, logger logger.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

// Create creates a new record
func (r *Repository) Create(ctx context.Context, model interface{}) error {
	start := time.Now()
	
	err := r.db.WithContext(ctx).Create(model).Error
	
	r.logger.LogDatabaseOperation(ctx, "CREATE", time.Since(start), 1)
	
	if err != nil {
		r.logger.LogError(ctx, err, "failed to create record", map[string]interface{}{
			"operation": "create",
		})
	}
	
	return err
}

// GetByID retrieves a record by ID
func (r *Repository) GetByID(ctx context.Context, model interface{}, id interface{}) error {
	start := time.Now()
	
	err := r.db.WithContext(ctx).First(model, id).Error
	
	r.logger.LogDatabaseOperation(ctx, "SELECT", time.Since(start), 1)
	
	if err != nil && err != gorm.ErrRecordNotFound {
		r.logger.LogError(ctx, err, "failed to get record by ID", map[string]interface{}{
			"operation": "get_by_id",
			"id":        id,
		})
	}
	
	return err
}

// Update updates a record
func (r *Repository) Update(ctx context.Context, model interface{}) error {
	start := time.Now()
	
	result := r.db.WithContext(ctx).Save(model)
	err := result.Error
	
	r.logger.LogDatabaseOperation(ctx, "UPDATE", time.Since(start), result.RowsAffected)
	
	if err != nil {
		r.logger.LogError(ctx, err, "failed to update record", map[string]interface{}{
			"operation": "update",
		})
	}
	
	return err
}

// Delete deletes a record
func (r *Repository) Delete(ctx context.Context, model interface{}, id interface{}) error {
	start := time.Now()
	
	result := r.db.WithContext(ctx).Delete(model, id)
	err := result.Error
	
	r.logger.LogDatabaseOperation(ctx, "DELETE", time.Since(start), result.RowsAffected)
	
	if err != nil {
		r.logger.LogError(ctx, err, "failed to delete record", map[string]interface{}{
			"operation": "delete",
			"id":        id,
		})
	}
	
	return err
}