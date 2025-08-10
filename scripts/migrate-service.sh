#!/bin/bash

# Migration script for existing services to GopherKit
# Usage: ./migrate-service.sh <service-path>

set -e

SERVICE_PATH="$1"

if [ -z "$SERVICE_PATH" ]; then
    echo "Usage: $0 <service-path>"
    echo "Example: $0 ../club-management-system-api/user-api"
    exit 1
fi

if [ ! -d "$SERVICE_PATH" ]; then
    echo "Error: Service path '$SERVICE_PATH' does not exist"
    exit 1
fi

SERVICE_NAME=$(basename "$SERVICE_PATH")
echo "🚀 Migrating service: $SERVICE_NAME"
echo "📁 Path: $SERVICE_PATH"

# Create backup directory
BACKUP_DIR="$SERVICE_PATH/backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

echo "💾 Creating backup in: $BACKUP_DIR"

# Backup original files
if [ -f "$SERVICE_PATH/cmd/api/main.go" ]; then
    cp "$SERVICE_PATH/cmd/api/main.go" "$BACKUP_DIR/main.go.backup"
    echo "✅ Backed up main.go"
fi

if [ -f "$SERVICE_PATH/go.mod" ]; then
    cp "$SERVICE_PATH/go.mod" "$BACKUP_DIR/go.mod.backup" 
    echo "✅ Backed up go.mod"
fi

if [ -f "$SERVICE_PATH/.env" ]; then
    cp "$SERVICE_PATH/.env" "$BACKUP_DIR/.env.backup"
    echo "✅ Backed up .env"
fi

# Extract service configuration
echo "🔍 Analyzing existing service..."

# Try to detect current port from main.go or .env
CURRENT_PORT="8080"
if [ -f "$SERVICE_PATH/cmd/api/main.go" ]; then
    PORT_FROM_MAIN=$(grep -o 'PORT.*[0-9]\+' "$SERVICE_PATH/cmd/api/main.go" | head -1 | grep -o '[0-9]\+' || echo "")
    if [ -n "$PORT_FROM_MAIN" ]; then
        CURRENT_PORT="$PORT_FROM_MAIN"
    fi
fi

if [ -f "$SERVICE_PATH/.env" ]; then
    PORT_FROM_ENV=$(grep "^PORT=" "$SERVICE_PATH/.env" | cut -d'=' -f2 || echo "")
    if [ -n "$PORT_FROM_ENV" ]; then
        CURRENT_PORT="$PORT_FROM_ENV"
    fi
fi

echo "📊 Detected configuration:"
echo "  - Service: $SERVICE_NAME"
echo "  - Port: $CURRENT_PORT"

# Add GopherKit dependency
echo "📦 Adding GopherKit dependency..."
cd "$SERVICE_PATH"

# Add gopherkit to go.mod
if ! grep -q "github.com/lukcba-developers/gopherkit" go.mod 2>/dev/null; then
    go get github.com/lukcba-developers/gopherkit
    echo "✅ Added GopherKit dependency"
else
    echo "✅ GopherKit already present in go.mod"
fi

# Generate new main.go
echo "🏗️  Generating new main.go..."

SERVICE_NAME_TITLE=$(echo "$SERVICE_NAME" | sed 's/-/ /g' | sed 's/\b\(.\)/\u\1/g' | sed 's/ //g')
SERVICE_NAME_LOWER=$(echo "$SERVICE_NAME" | tr '[:upper:]' '[:lower:]')

cat > cmd/api/main.go << EOF
package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/server"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
)

// TODO: Add your GORM models here
// Example:
// type User struct {
//     ID        uint      \`json:"id" gorm:"primaryKey"\`
//     CreatedAt time.Time \`json:"created_at"\`
//     UpdatedAt time.Time \`json:"updated_at"\`
//     Name      string    \`json:"name"\`
//     Email     string    \`json:"email" gorm:"uniqueIndex"\`
// }

var ${SERVICE_NAME_TITLE}Models = []interface{}{
	// &User{}, // Add your models here
}

func main() {
	// Load configuration
	cfg, err := config.LoadBaseConfig("$SERVICE_NAME_LOWER")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	appLogger := logger.NewLogger("$SERVICE_NAME_LOWER")

	// Initialize database
	dbClient, err := database.NewPostgresClient(database.PostgresOptions{
		Config: cfg.Database,
		Logger: appLogger,
		Models: ${SERVICE_NAME_TITLE}Models,
	})
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbClient.Close()

	// Initialize cache (optional)
	var cacheClient *cache.RedisClient
	if cfg.Cache.Enabled {
		cacheClient, err = cache.NewRedisClient(cache.RedisOptions{
			Config: cfg.Cache,
			Logger: appLogger,
		})
		if err != nil {
			appLogger.LogError(context.Background(), err, "Failed to initialize cache", nil)
			// Continue without cache
		} else {
			defer cacheClient.Close()
		}
	}

	// Setup health checks
	healthChecks := []observability.HealthCheck{
		dbClient.HealthCheck(),
	}
	if cacheClient != nil {
		healthChecks = append(healthChecks, cacheClient.HealthCheck())
	}

	// Create HTTP server with gopherkit
	httpServer, err := server.NewHTTPServer(server.Options{
		Config:       cfg,
		Logger:       appLogger,
		HealthChecks: healthChecks,
		Routes:       setupRoutes(dbClient, cacheClient, appLogger),
	})
	if err != nil {
		log.Fatalf("Failed to create HTTP server: %v", err)
	}

	// Start server
	if err := httpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for shutdown signal
	if err := httpServer.WaitForShutdown(); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "application_shutdown", nil)
}

// setupRoutes configures API routes
// TODO: Move your existing routes here
func setupRoutes(db *database.PostgresClient, cache *cache.RedisClient, logger logger.Logger) func(*gin.Engine) {
	return func(router *gin.Engine) {
		// API routes
		apiGroup := router.Group("/api/v1")
		{
			// TODO: Add your existing routes here
			// Example:
			apiGroup.GET("/example", exampleHandler(db, logger))
		}
	}
}

// TODO: Migrate your existing handlers here
// Example handler
func exampleHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		logger.LogBusinessEvent(ctx, "example_request", map[string]interface{}{
			"endpoint": c.FullPath(),
		})

		c.JSON(200, gin.H{
			"message": "$SERVICE_NAME_TITLE API is running with GopherKit",
			"service": "$SERVICE_NAME_LOWER",
		})
	}
}
EOF

echo "✅ Generated new main.go"

# Generate .env file with detected configuration
echo "⚙️  Generating .env configuration..."

cat > .env << EOF
# $SERVICE_NAME_TITLE Configuration
# Generated by GopherKit migration script

# Server Configuration
PORT=$CURRENT_PORT
ENVIRONMENT=development
METRICS_PORT=9090
MAX_REQUEST_SIZE=10485760

# Database Configuration
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/${SERVICE_NAME_LOWER}_db
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=${SERVICE_NAME_LOWER}_db
DB_SSL_MODE=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-${SERVICE_NAME_LOWER}-change-in-production
REFRESH_SECRET=your-refresh-secret-key-${SERVICE_NAME_LOWER}-change-in-production
ACCESS_DURATION=15m
REFRESH_DURATION=24h
JWT_ISSUER=${SERVICE_NAME_LOWER}-api

# Cache Configuration (Redis)
CACHE_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
CACHE_PREFIX=${SERVICE_NAME_LOWER}:
CACHE_TTL=5m

# Security Configuration
RATE_LIMIT_ENABLED=true
RATE_LIMIT_RPS=100
RATE_LIMIT_BURST=10

CIRCUIT_BREAKER_ENABLED=true
CB_MAX_REQUESTS=10
CB_INTERVAL=60s
CB_TIMEOUT=30s

# CORS Configuration
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:3001
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS,PATCH
CORS_ALLOWED_HEADERS=Origin,Content-Type,Accept,Authorization,X-Tenant-ID,X-Correlation-ID,X-User-ID
CORS_ALLOW_CREDENTIALS=true
CORS_MAX_AGE=86400

# External Services
AUTH_API_URL=http://localhost:8083
USER_API_URL=http://localhost:8081
NOTIFICATION_API_URL=http://localhost:8090

EXTERNAL_TIMEOUT=30s
EXTERNAL_RETRY_ATTEMPTS=3
EXTERNAL_RETRY_DELAY=1s

# Observability Configuration
TRACING_ENABLED=true
METRICS_ENABLED=true
LOG_LEVEL=info
SERVICE_VERSION=1.0.0

# Analytics Configuration (Optional)
ANALYTICS_ENABLED=true
EOF

echo "✅ Generated .env configuration"

# Update dependencies
echo "📦 Updating dependencies..."
go mod tidy

echo ""
echo "🎉 Migration completed successfully!"
echo ""
echo "📊 Migration Summary:"
echo "  ✅ Backup created: $BACKUP_DIR"
echo "  ✅ New main.go generated ($(wc -l < cmd/api/main.go) lines vs ~400+ before)"
echo "  ✅ GopherKit dependency added"
echo "  ✅ Environment configuration generated"
echo "  ✅ Dependencies updated"
echo ""
echo "📋 Next Steps:"
echo "  1. Review the new main.go and add your models to ${SERVICE_NAME_TITLE}Models"
echo "  2. Migrate your existing handlers to setupRoutes function"
echo "  3. Update the .env file with your specific configuration"
echo "  4. Test the migrated service:"
echo "     cd $SERVICE_PATH"
echo "     go run cmd/api/main.go"
echo "     curl http://localhost:$CURRENT_PORT/health"
echo ""
echo "📚 Migration guide: https://github.com/lukcba-developers/gopherkit/blob/main/examples/migration-guide.md"

echo ""
echo "⚠️  Manual Migration Required:"
echo "  - Move your domain models to ${SERVICE_NAME_TITLE}Models slice"
echo "  - Port your existing handlers to use GopherKit patterns"
echo "  - Update database models for GORM compatibility"
echo "  - Review and test all endpoints"
echo ""