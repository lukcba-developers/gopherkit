// Package database - Connection Management System
// Migrated from ClubPulse to gopherkit with significant improvements
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

var (
	instance *ConnectionManager
	once     sync.Once
)

// DatabaseMode represents the database operation mode
type DatabaseMode string

const (
	ModeSingle DatabaseMode = "single"
	ModeMulti  DatabaseMode = "multi"
)

// Config holds database configuration with enhanced options
type Config struct {
	// Common fields
	DatabaseMode DatabaseMode `json:"database_mode"`
	DatabaseURL  string       `json:"database_url"`

	// Single mode specific
	DatabaseSchema string `json:"database_schema"`

	// Connection pool settings
	MaxOpenConns        int           `json:"max_open_conns"`
	MaxIdleConns        int           `json:"max_idle_conns"`
	ConnMaxLifetime     time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime     time.Duration `json:"conn_max_idle_time"`
	EnableFallback      bool          `json:"enable_fallback"`
	FallbackDatabaseURL string        `json:"fallback_database_url"`

	// Monitoring and observability
	EnableMetrics         bool          `json:"enable_metrics"`
	ServiceName           string        `json:"service_name"`
	EnableHealthChecks    bool          `json:"enable_health_checks"`
	HealthCheckInterval   time.Duration `json:"health_check_interval"`
	EnableRetryLogic      bool          `json:"enable_retry_logic"`
	MaxRetryAttempts      int           `json:"max_retry_attempts"`
	RetryBackoffDuration  time.Duration `json:"retry_backoff_duration"`
	ConnectionTimeout     time.Duration `json:"connection_timeout"`
	EnableSlowQueryLog    bool          `json:"enable_slow_query_log"`
	SlowQueryThreshold    time.Duration `json:"slow_query_threshold"`
}

// DefaultConfig retorna configuración por defecto mejorada
func DefaultConfig() *Config {
	return &Config{
		DatabaseMode:          ModeMulti,
		DatabaseURL:           "",
		DatabaseSchema:        "",
		MaxOpenConns:          25,
		MaxIdleConns:          5,
		ConnMaxLifetime:       time.Hour,
		ConnMaxIdleTime:       10 * time.Minute,
		EnableFallback:        false,
		FallbackDatabaseURL:   "",
		EnableMetrics:         true,
		ServiceName:           "gopherkit",
		EnableHealthChecks:    true,
		HealthCheckInterval:   30 * time.Second,
		EnableRetryLogic:      true,
		MaxRetryAttempts:      3,
		RetryBackoffDuration:  time.Second,
		ConnectionTimeout:     10 * time.Second,
		EnableSlowQueryLog:    true,
		SlowQueryThreshold:    time.Second,
	}
}

// ConnectionManager manages database connections with enhanced features
type ConnectionManager struct {
	config          *Config
	db              *sql.DB
	fallbackDB      *sql.DB
	mu              sync.RWMutex
	mode            DatabaseMode
	schema          string
	isHealthy       bool
	lastHealthCheck time.Time
	metrics         *ConnectionMetrics
	healthTicker    *time.Ticker
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// ConnectionMetrics tracks enhanced connection statistics
type ConnectionMetrics struct {
	TotalConnections    int64                    `json:"total_connections"`
	ActiveConnections   int64                    `json:"active_connections"`
	FailedConnections   int64                    `json:"failed_connections"`
	FallbackUsed        int64                    `json:"fallback_used"`
	QueryTime           time.Duration            `json:"query_time"`
	SchemaQueries       map[string]int64         `json:"schema_queries"`
	SlowQueries         int64                    `json:"slow_queries"`
	ConnectionErrors    map[string]int64         `json:"connection_errors"`
	QueryHistory        []QueryMetric            `json:"query_history"`
	HealthCheckFailures int64                    `json:"health_check_failures"`
	RecoveryAttempts    int64                    `json:"recovery_attempts"`
	LastError           string                   `json:"last_error"`
	LastErrorTime       time.Time                `json:"last_error_time"`
	mu                  sync.RWMutex
}

// QueryMetric representa métricas de una query individual
type QueryMetric struct {
	Query     string        `json:"query"`
	Duration  time.Duration `json:"duration"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// NewConnectionManager creates a new enhanced connection manager
func NewConnectionManager(config *Config) (*ConnectionManager, error) {
	if config == nil {
		config = loadDefaultConfig()
	}

	// Validar configuración
	if err := validateConfig(config); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &ConnectionManager{
		config:    config,
		mode:      config.DatabaseMode,
		schema:    config.DatabaseSchema,
		isHealthy: true,
		ctx:       ctx,
		cancel:    cancel,
		metrics: &ConnectionMetrics{
			SchemaQueries:    make(map[string]int64),
			ConnectionErrors: make(map[string]int64),
			QueryHistory:     make([]QueryMetric, 0, 100), // Buffer de 100 queries
		},
	}

	// Connect based on mode
	if err := manager.connect(); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to connect")
	}

	// Setup fallback if enabled
	if config.EnableFallback && config.FallbackDatabaseURL != "" {
		if err := manager.setupFallback(); err != nil {
			// Log warning but don't fail
			fmt.Printf("Warning: Failed to setup fallback database: %v\n", err)
		}
	}

	// Start health checker if enabled
	if config.EnableHealthChecks {
		manager.startHealthChecker()
	}

	return manager, nil
}

// validateConfig valida la configuración del connection manager
func validateConfig(config *Config) error {
	if config.DatabaseURL == "" {
		return errors.New("database_url is required")
	}
	if config.MaxOpenConns <= 0 {
		return errors.New("max_open_conns must be positive")
	}
	if config.MaxIdleConns < 0 {
		return errors.New("max_idle_conns cannot be negative")
	}
	if config.ConnMaxLifetime <= 0 {
		return errors.New("conn_max_lifetime must be positive")
	}
	if config.MaxRetryAttempts < 0 {
		return errors.New("max_retry_attempts cannot be negative")
	}
	return nil
}

// GetInstance returns singleton instance of ConnectionManager
func GetInstance(config *Config) (*ConnectionManager, error) {
	var err error
	once.Do(func() {
		instance, err = NewConnectionManager(config)
	})
	if err != nil {
		return nil, err
	}
	return instance, nil
}

// connect establishes database connection based on mode with enhanced error handling
func (cm *ConnectionManager) connect() error {
	var dsn string

	switch cm.mode {
	case ModeSingle:
		dsn = cm.buildSingleModeDSN()
	case ModeMulti:
		dsn = cm.config.DatabaseURL
	default:
		return errors.Errorf("unknown database mode: %s", cm.mode)
	}

	// Create connection with context and timeout
	ctx, cancel := context.WithTimeout(context.Background(), cm.config.ConnectionTimeout)
	defer cancel()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		cm.recordError("connection_open", err)
		return errors.Wrap(err, "failed to open database")
	}

	// Configure connection pool
	cm.configureConnectionPool(db)

	// Test connection with context
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		cm.recordError("connection_ping", err)
		return errors.Wrap(err, "failed to ping database")
	}

	// Set search_path for single mode
	if cm.mode == ModeSingle && cm.schema != "" {
		if err := cm.setSearchPath(db); err != nil {
			db.Close()
			return errors.Wrap(err, "failed to set search path")
		}
	}

	cm.db = db
	cm.recordConnectionSuccess()
	fmt.Printf("Connected to database in %s mode (schema: %s)\n", cm.mode, cm.schema)

	return nil
}

// buildSingleModeDSN builds DSN for single database mode with schema
func (cm *ConnectionManager) buildSingleModeDSN() string {
	baseURL := cm.config.DatabaseURL

	// Parse the URL
	u, err := url.Parse(baseURL)
	if err != nil {
		fmt.Printf("Warning: Failed to parse database URL, using as-is: %v\n", err)
		return baseURL
	}

	// Add or update search_path parameter
	q := u.Query()
	if cm.schema != "" {
		searchPath := fmt.Sprintf("%s,public", cm.schema)
		q.Set("search_path", searchPath)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// setSearchPath sets the PostgreSQL search_path for the connection
func (cm *ConnectionManager) setSearchPath(db *sql.DB) error {
	if cm.schema == "" {
		return nil
	}

	query := fmt.Sprintf("SET search_path TO %s, public", cm.schema)
	_, err := db.Exec(query)
	if err != nil {
		return errors.Wrap(err, "failed to set search_path")
	}

	fmt.Printf("Set search_path to: %s\n", cm.schema)
	return nil
}

// configureConnectionPool sets enhanced connection pool parameters
func (cm *ConnectionManager) configureConnectionPool(db *sql.DB) {
	// Set connection limits based on mode and configuration
	if cm.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cm.config.MaxOpenConns)
	} else {
		// Defaults optimized for different modes
		if cm.mode == ModeSingle {
			db.SetMaxOpenConns(5) // Lower for PgBouncer compatibility
		} else {
			db.SetMaxOpenConns(25)
		}
	}

	if cm.config.MaxIdleConns >= 0 {
		db.SetMaxIdleConns(cm.config.MaxIdleConns)
	} else {
		if cm.mode == ModeSingle {
			db.SetMaxIdleConns(2)
		} else {
			db.SetMaxIdleConns(5)
		}
	}

	db.SetConnMaxLifetime(cm.config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cm.config.ConnMaxIdleTime)
}

// setupFallback configures fallback database connection with enhanced error handling
func (cm *ConnectionManager) setupFallback() error {
	if cm.config.FallbackDatabaseURL == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), cm.config.ConnectionTimeout)
	defer cancel()

	db, err := sql.Open("postgres", cm.config.FallbackDatabaseURL)
	if err != nil {
		return errors.Wrap(err, "failed to open fallback database")
	}

	// Configure fallback connection pool
	cm.configureConnectionPool(db)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return errors.Wrap(err, "failed to ping fallback database")
	}

	cm.fallbackDB = db
	fmt.Println("Fallback database configured successfully")

	return nil
}

// GetDB returns the active database connection with intelligent fallback
func (cm *ConnectionManager) GetDB() *sql.DB {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// If primary is healthy, use it
	if cm.isHealthy && cm.db != nil {
		return cm.db
	}

	// Try fallback if available
	if cm.fallbackDB != nil {
		cm.metrics.mu.Lock()
		cm.metrics.FallbackUsed++
		cm.metrics.mu.Unlock()

		fmt.Println("Warning: Using fallback database connection")
		return cm.fallbackDB
	}

	// Return primary even if unhealthy (might recover)
	return cm.db
}

// ExecuteWithMetrics executes a query with performance metrics
func (cm *ConnectionManager) ExecuteWithMetrics(query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	db := cm.GetDB()
	
	rows, err := db.Query(query, args...)
	duration := time.Since(start)
	
	// Record metrics
	cm.recordQueryMetric(query, duration, err == nil, err)
	
	return rows, err
}

// ExecuteInSchema executes a query in a specific schema (single mode only) with improvements
func (cm *ConnectionManager) ExecuteInSchema(schema string, query string, args ...interface{}) (*sql.Rows, error) {
	if cm.mode != ModeSingle {
		return nil, errors.New("ExecuteInSchema only available in single database mode")
	}

	start := time.Now()
	db := cm.GetDB()

	// Temporarily switch schema
	originalSchema := cm.schema
	if schema != originalSchema {
		setQuery := fmt.Sprintf("SET search_path TO %s, public", schema)
		if _, err := db.Exec(setQuery); err != nil {
			return nil, errors.Wrap(err, "failed to switch schema")
		}
		defer func() {
			// Restore original schema
			restoreQuery := fmt.Sprintf("SET search_path TO %s, public", originalSchema)
			db.Exec(restoreQuery)
		}()
	}

	// Track metrics
	cm.metrics.mu.Lock()
	cm.metrics.SchemaQueries[schema]++
	cm.metrics.mu.Unlock()

	// Execute query
	rows, err := db.Query(query, args...)
	duration := time.Since(start)
	
	// Record query metrics
	cm.recordQueryMetric(fmt.Sprintf("[%s] %s", schema, query), duration, err == nil, err)

	return rows, err
}

// Transaction executes a function within a database transaction with enhanced error handling
func (cm *ConnectionManager) Transaction(fn func(*sql.Tx) error) error {
	db := cm.GetDB()

	tx, err := db.Begin()
	if err != nil {
		cm.recordError("transaction_begin", err)
		return errors.Wrap(err, "failed to begin transaction")
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			cm.recordError("transaction_panic", fmt.Errorf("panic: %v", p))
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			cm.recordError("transaction_rollback", rbErr)
			return errors.Wrapf(err, "transaction failed: %v, rollback failed: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		cm.recordError("transaction_commit", err)
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// startHealthChecker starts the enhanced health checking loop
func (cm *ConnectionManager) startHealthChecker() {
	cm.healthTicker = time.NewTicker(cm.config.HealthCheckInterval)
	
	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		defer cm.healthTicker.Stop()
		
		for {
			select {
			case <-cm.ctx.Done():
				return
			case <-cm.healthTicker.C:
				cm.checkHealth()
			}
		}
	}()
}

// checkHealth verifies database connection health with enhanced checks
func (cm *ConnectionManager) checkHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	var healthCheckPassed bool

	// Check primary database
	if cm.db != nil {
		err := cm.db.PingContext(ctx)
		healthCheckPassed = err == nil
		
		cm.mu.Lock()
		cm.isHealthy = healthCheckPassed
		cm.lastHealthCheck = time.Now()
		cm.mu.Unlock()

		if err != nil {
			cm.recordError("health_check", err)
			cm.metrics.mu.Lock()
			cm.metrics.FailedConnections++
			cm.metrics.HealthCheckFailures++
			cm.metrics.mu.Unlock()

			fmt.Printf("Database health check failed: %v\n", err)
			
			// Try to reconnect
			if cm.config.EnableRetryLogic {
				go cm.tryReconnect()
			}
		}
	}

	// Check fallback database
	if cm.fallbackDB != nil {
		if err := cm.fallbackDB.PingContext(ctx); err != nil {
			fmt.Printf("Fallback database health check failed: %v\n", err)
		}
	}

	// Record health check duration
	cm.recordQueryMetric("health_check", time.Since(start), healthCheckPassed, nil)
}

// tryReconnect attempts to reconnect to the database with exponential backoff
func (cm *ConnectionManager) tryReconnect() {
	maxRetries := cm.config.MaxRetryAttempts
	backoff := cm.config.RetryBackoffDuration

	cm.metrics.mu.Lock()
	cm.metrics.RecoveryAttempts++
	cm.metrics.mu.Unlock()

	for i := 0; i < maxRetries; i++ {
		time.Sleep(backoff)
		backoff *= 2 // Exponential backoff

		if err := cm.connect(); err != nil {
			fmt.Printf("Reconnection attempt %d failed: %v\n", i+1, err)
			continue
		}

		cm.mu.Lock()
		cm.isHealthy = true
		cm.mu.Unlock()

		fmt.Println("Successfully reconnected to database")
		return
	}

	fmt.Printf("Failed to reconnect after %d attempts\n", maxRetries)
}

// recordError records an error in metrics
func (cm *ConnectionManager) recordError(errorType string, err error) {
	if err == nil {
		return
	}
	
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()
	
	cm.metrics.ConnectionErrors[errorType]++
	cm.metrics.LastError = err.Error()
	cm.metrics.LastErrorTime = time.Now()
}

// recordConnectionSuccess records a successful connection
func (cm *ConnectionManager) recordConnectionSuccess() {
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()
	
	cm.metrics.TotalConnections++
}

// recordQueryMetric records metrics for a query
func (cm *ConnectionManager) recordQueryMetric(query string, duration time.Duration, success bool, err error) {
	if !cm.config.EnableMetrics {
		return
	}

	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()

	// Check for slow query
	if duration > cm.config.SlowQueryThreshold {
		cm.metrics.SlowQueries++
		if cm.config.EnableSlowQueryLog {
			fmt.Printf("Slow query detected (%v): %s\n", duration, query)
		}
	}

	// Add to query history (with circular buffer)
	metric := QueryMetric{
		Query:     query,
		Duration:  duration,
		Success:   success,
		Timestamp: time.Now(),
	}
	if err != nil {
		metric.Error = err.Error()
	}

	if len(cm.metrics.QueryHistory) >= 100 {
		// Remove oldest entry
		cm.metrics.QueryHistory = cm.metrics.QueryHistory[1:]
	}
	cm.metrics.QueryHistory = append(cm.metrics.QueryHistory, metric)
}

// GetHealthStatus returns current enhanced health status
func (cm *ConnectionManager) GetHealthStatus() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var stats sql.DBStats
	if cm.db != nil {
		stats = cm.db.Stats()
	}

	status := map[string]interface{}{
		"mode":              cm.mode,
		"schema":            cm.schema,
		"is_healthy":        cm.isHealthy,
		"last_health_check": cm.lastHealthCheck,
		"open_connections":  stats.OpenConnections,
		"in_use":            stats.InUse,
		"idle":              stats.Idle,
		"wait_count":        stats.WaitCount,
		"wait_duration":     stats.WaitDuration.String(),
		"max_open_conns":    stats.MaxOpenConnections,
		"max_idle_conns":    stats.MaxIdleConnections,
		"config": map[string]interface{}{
			"enable_fallback":      cm.config.EnableFallback,
			"enable_metrics":       cm.config.EnableMetrics,
			"enable_health_checks": cm.config.EnableHealthChecks,
			"service_name":         cm.config.ServiceName,
		},
	}

	// Add enhanced metrics
	cm.metrics.mu.RLock()
	status["metrics"] = map[string]interface{}{
		"total_connections":      cm.metrics.TotalConnections,
		"active_connections":     cm.metrics.ActiveConnections,
		"failed_connections":     cm.metrics.FailedConnections,
		"fallback_used":          cm.metrics.FallbackUsed,
		"slow_queries":           cm.metrics.SlowQueries,
		"schema_queries":         cm.metrics.SchemaQueries,
		"connection_errors":      cm.metrics.ConnectionErrors,
		"health_check_failures":  cm.metrics.HealthCheckFailures,
		"recovery_attempts":      cm.metrics.RecoveryAttempts,
		"last_error":             cm.metrics.LastError,
		"last_error_time":        cm.metrics.LastErrorTime,
		"query_history_count":    len(cm.metrics.QueryHistory),
	}
	cm.metrics.mu.RUnlock()

	// Check if fallback is configured
	if cm.fallbackDB != nil {
		fallbackStats := cm.fallbackDB.Stats()
		status["fallback"] = map[string]interface{}{
			"configured":       true,
			"open_connections": fallbackStats.OpenConnections,
			"in_use":           fallbackStats.InUse,
			"idle":             fallbackStats.Idle,
		}
	}

	return status
}

// GetQueryHistory returns recent query history
func (cm *ConnectionManager) GetQueryHistory() []QueryMetric {
	cm.metrics.mu.RLock()
	defer cm.metrics.mu.RUnlock()
	
	// Return copy of query history
	history := make([]QueryMetric, len(cm.metrics.QueryHistory))
	copy(history, cm.metrics.QueryHistory)
	return history
}

// Close closes all database connections with graceful shutdown
func (cm *ConnectionManager) Close() error {
	// Cancel health checker
	if cm.cancel != nil {
		cm.cancel()
	}

	// Wait for health checker to stop
	cm.wg.Wait()

	var errors []error

	if cm.db != nil {
		if err := cm.db.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close primary database: %w", err))
		}
	}

	if cm.fallbackDB != nil {
		if err := cm.fallbackDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close fallback database: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing connections: %v", errors)
	}

	fmt.Println("Database connections closed successfully")
	return nil
}

// loadDefaultConfig loads configuration from environment variables with enhanced options
func loadDefaultConfig() *Config {
	config := DefaultConfig()
	
	// Load from environment
	if env := os.Getenv("DATABASE_MODE"); env != "" {
		config.DatabaseMode = DatabaseMode(env)
	}
	if env := os.Getenv("DATABASE_URL"); env != "" {
		config.DatabaseURL = env
	}
	if env := os.Getenv("DATABASE_SCHEMA"); env != "" {
		config.DatabaseSchema = env
	}
	if env := os.Getenv("ENABLE_FALLBACK"); env == "true" {
		config.EnableFallback = true
	}
	if env := os.Getenv("ENABLE_METRICS"); env == "false" {
		config.EnableMetrics = false
	}
	if env := os.Getenv("SERVICE_NAME"); env != "" {
		config.ServiceName = env
	}
	
	// Parse connection pool settings
	if env := os.Getenv("MAX_OPEN_CONNS"); env != "" {
		fmt.Sscanf(env, "%d", &config.MaxOpenConns)
	}
	if env := os.Getenv("MAX_IDLE_CONNS"); env != "" {
		fmt.Sscanf(env, "%d", &config.MaxIdleConns)
	}

	// Set fallback URL for single mode
	if config.DatabaseMode == ModeSingle && config.EnableFallback {
		serviceName := strings.TrimSuffix(config.ServiceName, "-api")
		config.FallbackDatabaseURL = fmt.Sprintf(
			"postgres://postgres:postgres@%s-postgres:5432/%sapi_db?sslmode=disable",
			serviceName, serviceName,
		)
	}

	return config
}

// ResetMetrics resets all connection metrics
func (cm *ConnectionManager) ResetMetrics() {
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()
	
	cm.metrics.TotalConnections = 0
	cm.metrics.ActiveConnections = 0
	cm.metrics.FailedConnections = 0
	cm.metrics.FallbackUsed = 0
	cm.metrics.SlowQueries = 0
	cm.metrics.HealthCheckFailures = 0
	cm.metrics.RecoveryAttempts = 0
	cm.metrics.SchemaQueries = make(map[string]int64)
	cm.metrics.ConnectionErrors = make(map[string]int64)
	cm.metrics.QueryHistory = make([]QueryMetric, 0, 100)
	cm.metrics.LastError = ""
	cm.metrics.LastErrorTime = time.Time{}
}