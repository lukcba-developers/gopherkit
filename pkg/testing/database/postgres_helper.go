package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgresTestConfig configura el container de PostgreSQL para tests
type PostgresTestConfig struct {
	Image           string
	Database        string
	Username        string
	Password        string
	Port            string
	InitScripts     []string
	MaxConnections  int
	ConnMaxLifetime time.Duration
	Logger          *logrus.Logger
}

// DefaultPostgresConfig retorna configuración por defecto para PostgreSQL de tests
func DefaultPostgresConfig() *PostgresTestConfig {
	return &PostgresTestConfig{
		Image:           "postgres:15-alpine",
		Database:        "testdb",
		Username:        "testuser",
		Password:        "testpass",
		Port:            "5432/tcp",
		MaxConnections:  10,
		ConnMaxLifetime: time.Hour,
		Logger:          logrus.New(),
	}
}

// PostgresTestHelper maneja la configuración de PostgreSQL para tests
type PostgresTestHelper struct {
	config    *PostgresTestConfig
	container testcontainers.Container
	db        *sql.DB
	gormDB    *gorm.DB
	sqlxDB    *sqlx.DB
	logger    *logrus.Logger
}

// NewPostgresTestHelper crea un nuevo helper de PostgreSQL
func NewPostgresTestHelper(config *PostgresTestConfig) *PostgresTestHelper {
	if config == nil {
		config = DefaultPostgresConfig()
	}
	
	return &PostgresTestHelper{
		config: config,
		logger: config.Logger,
	}
}

// Start inicia el container de PostgreSQL
func (h *PostgresTestHelper) Start(ctx context.Context) error {
	req := testcontainers.ContainerRequest{
		Image:        h.config.Image,
		ExposedPorts: []string{h.config.Port},
		Env: map[string]string{
			"POSTGRES_DB":       h.config.Database,
			"POSTGRES_USER":     h.config.Username,
			"POSTGRES_PASSWORD": h.config.Password,
			"POSTGRES_SSLMODE":  "disable",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections"),
			wait.ForListeningPort(nat.Port(h.config.Port)),
		).WithDeadline(2 * time.Minute),
	}
	
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}
	
	h.container = container
	
	// Wait for container to be ready
	if err := h.waitForReady(ctx); err != nil {
		return fmt.Errorf("PostgreSQL container not ready: %w", err)
	}
	
	// Setup database connections
	if err := h.setupConnections(ctx); err != nil {
		return fmt.Errorf("failed to setup database connections: %w", err)
	}
	
	h.logger.Info("PostgreSQL test container started successfully")
	return nil
}

// Stop detiene el container de PostgreSQL
func (h *PostgresTestHelper) Stop(ctx context.Context) error {
	if h.db != nil {
		h.db.Close()
	}
	
	if h.container != nil {
		if err := h.container.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate PostgreSQL container: %w", err)
		}
	}
	
	h.logger.Info("PostgreSQL test container stopped")
	return nil
}

// GetDB retorna la conexión database/sql
func (h *PostgresTestHelper) GetDB() *sql.DB {
	return h.db
}

// GetGORM retorna la conexión GORM
func (h *PostgresTestHelper) GetGORM() *gorm.DB {
	return h.gormDB
}

// GetSQLX retorna la conexión SQLX
func (h *PostgresTestHelper) GetSQLX() *sqlx.DB {
	return h.sqlxDB
}

// GetConnectionString retorna la cadena de conexión
func (h *PostgresTestHelper) GetConnectionString(ctx context.Context) (string, error) {
	if h.container == nil {
		return "", fmt.Errorf("container not started")
	}
	
	host, err := h.container.Host(ctx)
	if err != nil {
		return "", err
	}
	
	port, err := h.container.MappedPort(ctx, nat.Port(h.config.Port))
	if err != nil {
		return "", err
	}
	
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port.Port(), h.config.Username, h.config.Password, h.config.Database), nil
}

// RunMigrations ejecuta scripts de migración
func (h *PostgresTestHelper) RunMigrations(ctx context.Context, migrations []string) error {
	if h.db == nil {
		return fmt.Errorf("database connection not available")
	}
	
	for i, migration := range migrations {
		h.logger.WithField("migration", i+1).Debug("Running migration")
		
		if _, err := h.db.ExecContext(ctx, migration); err != nil {
			return fmt.Errorf("failed to run migration %d: %w", i+1, err)
		}
	}
	
	h.logger.WithField("count", len(migrations)).Info("All migrations completed successfully")
	return nil
}

// TruncateAllTables limpia todas las tablas
func (h *PostgresTestHelper) TruncateAllTables(ctx context.Context) error {
	if h.db == nil {
		return fmt.Errorf("database connection not available")
	}
	
	// Get all table names
	rows, err := h.db.QueryContext(ctx, `
		SELECT tablename FROM pg_tables 
		WHERE schemaname = 'public' 
		AND tablename != 'schema_migrations'
	`)
	if err != nil {
		return fmt.Errorf("failed to get table names: %w", err)
	}
	defer rows.Close()
	
	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}
	
	// Truncate all tables
	if len(tables) > 0 {
		for _, table := range tables {
			query := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", pq.QuoteIdentifier(table))
			if _, err := h.db.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("failed to truncate table %s: %w", table, err)
			}
		}
		h.logger.WithField("tables", len(tables)).Debug("All tables truncated")
	}
	
	return nil
}

// CreateTestDatabase crea una base de datos de prueba
func (h *PostgresTestHelper) CreateTestDatabase(ctx context.Context, dbName string) error {
	query := fmt.Sprintf("CREATE DATABASE %s", pq.QuoteIdentifier(dbName))
	_, err := h.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create test database %s: %w", dbName, err)
	}
	
	h.logger.WithField("database", dbName).Info("Test database created")
	return nil
}

// DropTestDatabase elimina una base de datos de prueba
func (h *PostgresTestHelper) DropTestDatabase(ctx context.Context, dbName string) error {
	// Terminate active connections first
	terminateQuery := fmt.Sprintf(`
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = %s AND pid <> pg_backend_pid()
	`, pq.QuoteLiteral(dbName))
	
	_, _ = h.db.ExecContext(ctx, terminateQuery)
	
	// Drop database
	query := fmt.Sprintf("DROP DATABASE IF EXISTS %s", pq.QuoteIdentifier(dbName))
	_, err := h.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop test database %s: %w", dbName, err)
	}
	
	h.logger.WithField("database", dbName).Info("Test database dropped")
	return nil
}

// waitForReady espera a que el container esté listo
func (h *PostgresTestHelper) waitForReady(ctx context.Context) error {
	connectionString, err := h.GetConnectionString(ctx)
	if err != nil {
		return err
	}
	
	// Try to connect multiple times
	for i := 0; i < 30; i++ {
		db, err := sql.Open("postgres", connectionString)
		if err == nil {
			if err := db.Ping(); err == nil {
				db.Close()
				return nil
			}
			db.Close()
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			continue
		}
	}
	
	return fmt.Errorf("PostgreSQL container not ready after 30 seconds")
}

// setupConnections configura las conexiones de base de datos
func (h *PostgresTestHelper) setupConnections(ctx context.Context) error {
	connectionString, err := h.GetConnectionString(ctx)
	if err != nil {
		return err
	}
	
	// Setup database/sql connection
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return fmt.Errorf("failed to create database connection: %w", err)
	}
	
	db.SetMaxOpenConns(h.config.MaxConnections)
	db.SetMaxIdleConns(h.config.MaxConnections / 2)
	db.SetConnMaxLifetime(h.config.ConnMaxLifetime)
	
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	
	h.db = db
	
	// Setup GORM connection
	gormDB, err := gorm.Open(postgres.Open(connectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Reduce noise in tests
	})
	if err != nil {
		return fmt.Errorf("failed to create GORM connection: %w", err)
	}
	
	h.gormDB = gormDB
	
	// Setup SQLX connection
	sqlxDB, err := sqlx.ConnectContext(ctx, "postgres", connectionString)
	if err != nil {
		return fmt.Errorf("failed to create SQLX connection: %w", err)
	}
	
	h.sqlxDB = sqlxDB
	
	return nil
}