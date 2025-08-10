package components

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lukcba-developers/gopherkit/pkg/health"
)

// DatabaseChecker implementa health check para bases de datos SQL
type DatabaseChecker struct {
	name     string
	db       *sqlx.DB
	timeout  time.Duration
	required bool
	testQuery string
}

// NewDatabaseChecker crea un nuevo checker de base de datos
func NewDatabaseChecker(name string, db *sqlx.DB, opts ...DatabaseOption) *DatabaseChecker {
	checker := &DatabaseChecker{
		name:      name,
		db:        db,
		timeout:   10 * time.Second,
		required:  true,
		testQuery: "SELECT 1",
	}
	
	for _, opt := range opts {
		opt(checker)
	}
	
	return checker
}

// DatabaseOption define opciones para DatabaseChecker
type DatabaseOption func(*DatabaseChecker)

// WithDatabaseTimeout configura el timeout
func WithDatabaseTimeout(timeout time.Duration) DatabaseOption {
	return func(c *DatabaseChecker) {
		c.timeout = timeout
	}
}

// WithDatabaseRequired configura si es requerido
func WithDatabaseRequired(required bool) DatabaseOption {
	return func(c *DatabaseChecker) {
		c.required = required
	}
}

// WithDatabaseTestQuery configura la query de test
func WithDatabaseTestQuery(query string) DatabaseOption {
	return func(c *DatabaseChecker) {
		c.testQuery = query
	}
}

// Name implementa HealthChecker
func (d *DatabaseChecker) Name() string {
	return d.name
}

// Check implementa HealthChecker
func (d *DatabaseChecker) Check(ctx context.Context) health.ComponentHealth {
	start := time.Now()
	
	// Verificar si la conexión está viva
	if err := d.db.PingContext(ctx); err != nil {
		return health.NewComponentHealth(health.StatusUnhealthy, "Database ping failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("driver", d.db.DriverName()).
			WithMetadata("test_query", d.testQuery)
	}
	
	// Ejecutar query de test
	var result interface{}
	if err := d.db.GetContext(ctx, &result, d.testQuery); err != nil {
		return health.NewComponentHealth(health.StatusUnhealthy, "Database test query failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("driver", d.db.DriverName()).
			WithMetadata("test_query", d.testQuery)
	}
	
	// Obtener estadísticas de la base de datos
	stats := d.db.Stats()
	
	return health.NewComponentHealth(health.StatusHealthy, "Database is healthy").
		WithDuration(time.Since(start)).
		WithMetadata("driver", d.db.DriverName()).
		WithMetadata("max_open_connections", stats.MaxOpenConnections).
		WithMetadata("open_connections", stats.OpenConnections).
		WithMetadata("in_use", stats.InUse).
		WithMetadata("idle", stats.Idle).
		WithMetadata("wait_count", stats.WaitCount).
		WithMetadata("wait_duration", stats.WaitDuration.String()).
		WithMetadata("max_idle_closed", stats.MaxIdleClosed).
		WithMetadata("max_idle_time_closed", stats.MaxIdleTimeClosed).
		WithMetadata("max_lifetime_closed", stats.MaxLifetimeClosed)
}

// IsRequired implementa HealthChecker
func (d *DatabaseChecker) IsRequired() bool {
	return d.required
}

// Timeout implementa HealthChecker
func (d *DatabaseChecker) Timeout() time.Duration {
	return d.timeout
}

// PostgreSQLChecker es un checker específico para PostgreSQL
type PostgreSQLChecker struct {
	*DatabaseChecker
}

// NewPostgreSQLChecker crea un checker específico para PostgreSQL
func NewPostgreSQLChecker(name string, db *sqlx.DB, opts ...DatabaseOption) *PostgreSQLChecker {
	checker := NewDatabaseChecker(name, db, opts...)
	checker.testQuery = "SELECT version()"
	
	return &PostgreSQLChecker{
		DatabaseChecker: checker,
	}
}

// Check implementa un health check específico para PostgreSQL
func (p *PostgreSQLChecker) Check(ctx context.Context) health.ComponentHealth {
	start := time.Now()
	
	// Ping básico
	if err := p.db.PingContext(ctx); err != nil {
		return health.NewComponentHealth(health.StatusUnhealthy, "PostgreSQL ping failed").
			WithDuration(time.Since(start)).
			WithError(err)
	}
	
	// Obtener versión de PostgreSQL
	var version string
	if err := p.db.GetContext(ctx, &version, "SELECT version()"); err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "PostgreSQL version check failed").
			WithDuration(time.Since(start)).
			WithError(err)
	}
	
	// Verificar permisos básicos creando y eliminando una tabla temporal
	tableName := fmt.Sprintf("health_check_%d", time.Now().Unix())
	createSQL := fmt.Sprintf("CREATE TEMP TABLE %s (id INTEGER)", tableName)
	dropSQL := fmt.Sprintf("DROP TABLE %s", tableName)
	
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "PostgreSQL transaction failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("version", version)
	}
	defer tx.Rollback()
	
	if _, err := tx.ExecContext(ctx, createSQL); err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "PostgreSQL table creation failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("version", version)
	}
	
	if _, err := tx.ExecContext(ctx, dropSQL); err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "PostgreSQL table drop failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("version", version)
	}
	
	if err := tx.Commit(); err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "PostgreSQL transaction commit failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("version", version)
	}
	
	// Obtener estadísticas
	stats := p.db.Stats()
	
	return health.NewComponentHealth(health.StatusHealthy, "PostgreSQL is healthy").
		WithDuration(time.Since(start)).
		WithMetadata("version", version).
		WithMetadata("driver", "postgres").
		WithMetadata("max_open_connections", stats.MaxOpenConnections).
		WithMetadata("open_connections", stats.OpenConnections).
		WithMetadata("in_use", stats.InUse).
		WithMetadata("idle", stats.Idle)
}