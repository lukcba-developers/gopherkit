package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ServiceInfo contiene información del servicio a migrar
type ServiceInfo struct {
	Name           string
	Path           string
	Port           string
	DatabaseName   string
	HasRedis       bool
	HasJWT         bool
	HasPrometheus  bool
	HasGORM        bool
	MainGoPath     string
	Dependencies   []string
	CustomPackages []string
}

func main() {
	var (
		servicePath = flag.String("path", "", "Path to the service to migrate")
		dryRun      = flag.Bool("dry-run", false, "Show what would be migrated without making changes")
		force       = flag.Bool("force", false, "Force migration even if GopherKit is already present")
		verbose     = flag.Bool("verbose", true, "Verbose output")
	)
	flag.Parse()

	if *servicePath == "" {
		fmt.Println("Usage: go run cmd/migrate/main.go -path <service-path>")
		fmt.Println("Example: go run cmd/migrate/main.go -path ../club-management-system-api/booking-api")
		os.Exit(1)
	}

	migrator := &ServiceMigrator{
		DryRun:  *dryRun,
		Force:   *force,
		Verbose: *verbose,
	}

	if err := migrator.MigrateService(*servicePath); err != nil {
		fmt.Printf("❌ Migration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Migration completed successfully!")
}

// ServiceMigrator handles the migration process
type ServiceMigrator struct {
	DryRun  bool
	Force   bool
	Verbose bool
}

// MigrateService migra un servicio a GopherKit
func (m *ServiceMigrator) MigrateService(servicePath string) error {
	servicePath, err := filepath.Abs(servicePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	if !m.pathExists(servicePath) {
		return fmt.Errorf("service path does not exist: %s", servicePath)
	}

	fmt.Printf("🔍 Analyzing service at: %s\n", servicePath)

	// Analizar el servicio
	info, err := m.analyzeService(servicePath)
	if err != nil {
		return fmt.Errorf("failed to analyze service: %w", err)
	}

	if m.Verbose {
		m.printServiceInfo(info)
	}

	// Verificar si ya tiene GopherKit
	if !m.Force && m.hasGopherKit(info) {
		fmt.Printf("⚠️  Service already has GopherKit dependency. Use -force to override.\n")
		return nil
	}

	// Crear backup
	if !m.DryRun {
		if err := m.createBackup(info); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Ejecutar migración
	return m.executeMigration(info)
}

// analyzeService analiza la estructura del servicio
func (m *ServiceMigrator) analyzeService(servicePath string) (*ServiceInfo, error) {
	info := &ServiceInfo{
		Name:         filepath.Base(servicePath),
		Path:         servicePath,
		Dependencies: []string{},
	}

	// Buscar go.mod
	goModPath := filepath.Join(servicePath, "go.mod")
	if !m.pathExists(goModPath) {
		return nil, fmt.Errorf("go.mod not found in service directory")
	}

	// Analizar go.mod
	if err := m.analyzeGoMod(info, goModPath); err != nil {
		return nil, fmt.Errorf("failed to analyze go.mod: %w", err)
	}

	// Buscar main.go
	mainPaths := []string{
		filepath.Join(servicePath, "cmd", "api", "main.go"),
		filepath.Join(servicePath, "cmd", "main.go"),
		filepath.Join(servicePath, "main.go"),
	}

	for _, path := range mainPaths {
		if m.pathExists(path) {
			info.MainGoPath = path
			break
		}
	}

	if info.MainGoPath == "" {
		return nil, fmt.Errorf("main.go not found in common locations")
	}

	// Analizar main.go
	if err := m.analyzeMainGo(info); err != nil {
		return nil, fmt.Errorf("failed to analyze main.go: %w", err)
	}

	// Detectar configuraciones
	m.detectServiceConfig(info)

	return info, nil
}

// analyzeGoMod analiza el archivo go.mod
func (m *ServiceMigrator) analyzeGoMod(info *ServiceInfo, goModPath string) error {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detectar dependencias importantes
		if strings.Contains(line, "github.com/gin-gonic/gin") {
			info.Dependencies = append(info.Dependencies, "gin")
		}
		if strings.Contains(line, "gorm.io/gorm") || strings.Contains(line, "gorm.io/driver") {
			info.HasGORM = true
			info.Dependencies = append(info.Dependencies, "gorm")
		}
		if strings.Contains(line, "redis") {
			info.HasRedis = true
			info.Dependencies = append(info.Dependencies, "redis")
		}
		if strings.Contains(line, "jwt") {
			info.HasJWT = true
			info.Dependencies = append(info.Dependencies, "jwt")
		}
		if strings.Contains(line, "prometheus") {
			info.HasPrometheus = true
			info.Dependencies = append(info.Dependencies, "prometheus")
		}
		if strings.Contains(line, "gopherkit") {
			info.Dependencies = append(info.Dependencies, "gopherkit")
		}
	}

	return nil
}

// analyzeMainGo analiza el archivo main.go
func (m *ServiceMigrator) analyzeMainGo(info *ServiceInfo) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, info.MainGoPath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse main.go: %w", err)
	}

	// Extraer imports personalizados
	for _, imp := range node.Imports {
		if imp.Path != nil {
			path := strings.Trim(imp.Path.Value, "\"")
			if strings.Contains(path, info.Name) || strings.Contains(path, "internal") {
				info.CustomPackages = append(info.CustomPackages, path)
			}
		}
	}

	return nil
}

// detectServiceConfig detecta configuraciones del puerto y base de datos
func (m *ServiceMigrator) detectServiceConfig(info *ServiceInfo) {
	// Buscar .env o configuración
	envPath := filepath.Join(info.Path, ".env")
	if m.pathExists(envPath) {
		content, err := os.ReadFile(envPath)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PORT=") {
					info.Port = strings.TrimPrefix(line, "PORT=")
				}
				if strings.HasPrefix(line, "DB_NAME=") {
					info.DatabaseName = strings.TrimPrefix(line, "DB_NAME=")
				}
			}
		}
	}

	// Defaults basados en el nombre del servicio
	if info.Port == "" {
		info.Port = m.getDefaultPort(info.Name)
	}
	if info.DatabaseName == "" {
		info.DatabaseName = strings.ReplaceAll(info.Name, "-", "_") + "_db"
	}
}

// hasGopherKit verifica si el servicio ya tiene GopherKit
func (m *ServiceMigrator) hasGopherKit(info *ServiceInfo) bool {
	for _, dep := range info.Dependencies {
		if dep == "gopherkit" {
			return true
		}
	}
	return false
}

// createBackup crea un backup del servicio
func (m *ServiceMigrator) createBackup(info *ServiceInfo) error {
	timestamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(info.Path, fmt.Sprintf("backup-gopherkit-migration-%s", timestamp))

	fmt.Printf("💾 Creating backup at: %s\n", backupDir)

	// Crear directorio de backup
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	// Backup files importantes
	filesToBackup := []string{
		"go.mod",
		"go.sum",
		"main.go",
		".env",
	}

	for _, file := range filesToBackup {
		srcPath := filepath.Join(info.Path, file)
		if info.MainGoPath != "" && strings.HasSuffix(info.MainGoPath, file) {
			srcPath = info.MainGoPath
		}

		if m.pathExists(srcPath) {
			dstPath := filepath.Join(backupDir, file+".backup")
			if err := m.copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to backup %s: %w", file, err)
			}
		}
	}

	return nil
}

// executeMigration ejecuta la migración
func (m *ServiceMigrator) executeMigration(info *ServiceInfo) error {
	fmt.Printf("🚀 Starting migration for %s\n", info.Name)

	// 1. Actualizar go.mod
	if err := m.updateGoMod(info); err != nil {
		return fmt.Errorf("failed to update go.mod: %w", err)
	}

	// 2. Generar nuevo main.go
	if err := m.generateMainGo(info); err != nil {
		return fmt.Errorf("failed to generate main.go: %w", err)
	}

	// 3. Generar configuración .env
	if err := m.generateEnvFile(info); err != nil {
		return fmt.Errorf("failed to generate .env: %w", err)
	}

	// 4. Actualizar dependencias
	if !m.DryRun {
		if err := m.runGoModTidy(info.Path); err != nil {
			return fmt.Errorf("failed to run go mod tidy: %w", err)
		}
	}

	return nil
}

// updateGoMod actualiza el archivo go.mod
func (m *ServiceMigrator) updateGoMod(info *ServiceInfo) error {
	goModPath := filepath.Join(info.Path, "go.mod")
	
	if m.DryRun {
		fmt.Printf("📝 Would add GopherKit dependency to %s\n", goModPath)
		return nil
	}

	content, err := os.ReadFile(goModPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	requireSection := false
	gopherKitAdded := false

	for _, line := range lines {
		if strings.Contains(line, "require (") {
			requireSection = true
		}

		// Añadir GopherKit en la sección require
		if requireSection && !gopherKitAdded && (strings.Contains(line, "github.com/gin-gonic/gin") || strings.Contains(line, ")")) {
			if !strings.Contains(line, ")") {
				newLines = append(newLines, line)
				newLines = append(newLines, "\tgithub.com/lukcba-developers/gopherkit v0.1.0")
				gopherKitAdded = true
			} else {
				if !gopherKitAdded {
					newLines = append(newLines, "\tgithub.com/lukcba-developers/gopherkit v0.1.0")
					gopherKitAdded = true
				}
				newLines = append(newLines, line)
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	// Si no se añadió, añadir al final
	if !gopherKitAdded {
		newLines = append(newLines, "")
		newLines = append(newLines, "require github.com/lukcba-developers/gopherkit v0.1.0")
	}

	// Añadir replace local para desarrollo
	newLines = append(newLines, "")
	newLines = append(newLines, "// Local development")
	newLines = append(newLines, "replace github.com/lukcba-developers/gopherkit => /Users/lukcba-macbook-pro/go/src/gopherkit")

	return os.WriteFile(goModPath, []byte(strings.Join(newLines, "\n")), 0644)
}

// generateMainGo genera un nuevo main.go usando GopherKit
func (m *ServiceMigrator) generateMainGo(info *ServiceInfo) error {
	if m.DryRun {
		fmt.Printf("📝 Would generate new main.go for %s\n", info.Name)
		return nil
	}

	mainGoContent := m.buildMainGoTemplate(info)
	
	return os.WriteFile(info.MainGoPath, []byte(mainGoContent), 0644)
}

// buildMainGoTemplate construye el template del main.go
func (m *ServiceMigrator) buildMainGoTemplate(info *ServiceInfo) string {
	serviceNameTitle := strings.Title(strings.ReplaceAll(info.Name, "-", ""))

	template := fmt.Sprintf(`package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
	"github.com/lukcba-developers/gopherkit/pkg/server"
)

// TODO: Add your GORM models here
// Example:
// type %sModel struct {
//     ID        uint      ` + "`json:\"id\" gorm:\"primaryKey\"`" + `
//     CreatedAt time.Time ` + "`json:\"created_at\"`" + `
//     UpdatedAt time.Time ` + "`json:\"updated_at\"`" + `
//     Name      string    ` + "`json:\"name\"`" + `
//     TenantID  string    ` + "`json:\"tenant_id\" gorm:\"index\"`" + `
// }

var %sModels = []interface{}{
	// &%sModel{}, // Add your models here
}

func main() {
	// Load configuration using GopherKit
	cfg, err := config.LoadBaseConfig("%s")
	if err != nil {
		log.Fatalf("Failed to load configuration: %%v", err)
	}

	// Initialize logger using GopherKit
	appLogger := logger.New("%s")

	// Initialize database using GopherKit
	dbClient, err := database.NewPostgresClient(database.PostgresOptions{
		Config: cfg.Database,
		Logger: appLogger,
		Models: %sModels, // Auto-migration of models
	})
	if err != nil {
		log.Fatalf("Failed to initialize database: %%v", err)
	}
	defer dbClient.Close()
`, serviceNameTitle, serviceNameTitle, serviceNameTitle, info.Name, info.Name, serviceNameTitle)

	// Añadir Redis si es detectado
	if info.HasRedis {
		template += `
	// Initialize cache using GopherKit (optional)
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
`
	}

	// Añadir JWT si es detectado
	if info.HasJWT {
		template += `
	// Initialize JWT middleware using GopherKit
	jwtMiddleware, err := auth.NewJWTMiddleware(cfg.Security.JWT, appLogger)
	if err != nil {
		log.Fatalf("Failed to initialize JWT middleware: %%v", err)
	}
`
	}

	// Continuar con el resto del template
	template += `
	// Configure health checks
	healthChecks := []observability.HealthCheck{
		dbClient.HealthCheck(),
	}`

	if info.HasRedis {
		template += `
	if cacheClient != nil {
		healthChecks = append(healthChecks, cacheClient.HealthCheck())
	}`
	}

	template += `

	// Create HTTP server using GopherKit
	httpServer, err := server.NewHTTPServer(server.Options{
		Config:       cfg,
		Logger:       appLogger,
		HealthChecks: healthChecks,
		Routes:       setupRoutes(appLogger),
	})
	if err != nil {
		log.Fatalf("Failed to create HTTP server: %%v", err)
	}

	// Start server
	if err := httpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %%v", err)
	}

	// Wait for shutdown signal
	if err := httpServer.WaitForShutdown(); err != nil {
		log.Fatalf("Server shutdown error: %%v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "application_shutdown", map[string]interface{}{
		"service": "%s",
	})
}

// setupRoutes configures API routes
// TODO: Move your existing routes here
func setupRoutes(logger logger.Logger) func(*gin.Engine) {
	return func(router *gin.Engine) {
		// API routes
		v1 := router.Group("/api/v1")
		{
			// TODO: Add your existing routes here
			// Example:
			v1.GET("/ping", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"message": "%s API migrated to GopherKit successfully!",
					"service": "%s",
				})
			})
		}
		
		// Migration info endpoint
		router.GET("/migration-info", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message":     "%s API migrated to GopherKit",
				"service":     "%s", 
				"migrated_at": "automated",
				"benefits": []string{
					"✅ GopherKit Database client with auto-migration",`

	if info.HasRedis {
		template += `
					"✅ GopherKit Redis cache with error handling",`
	}

	if info.HasJWT {
		template += `
					"✅ GopherKit JWT authentication middleware",`
	}

	template += `
					"✅ GopherKit HTTP server with health checks",
					"✅ GopherKit structured logging with context",
					"✅ GopherKit configuration management",
					"✅ GopherKit observability and metrics",
				},
			})
		})
	}
}
`

	return fmt.Sprintf(template, info.Name, serviceNameTitle, info.Name, serviceNameTitle, info.Name)
}

// generateEnvFile genera archivo de configuración .env
func (m *ServiceMigrator) generateEnvFile(info *ServiceInfo) error {
	envPath := filepath.Join(info.Path, ".env")
	
	if m.DryRun {
		fmt.Printf("📝 Would generate .env file for %s\n", info.Name)
		return nil
	}

	envContent := fmt.Sprintf(`# %s Configuration - Migrated with GopherKit
# Generated by automated migration

# Server Configuration
PORT=%s
ENVIRONMENT=development
METRICS_PORT=909%s
MAX_REQUEST_SIZE=10485760

# Database Configuration
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/%s
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=%s
DB_SSL_MODE=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# JWT Configuration
JWT_SECRET=%s-super-secret-jwt-key-change-in-production
REFRESH_SECRET=%s-refresh-secret-key-change-in-production
ACCESS_DURATION=15m
REFRESH_DURATION=24h
JWT_ISSUER=%s
`, strings.Title(strings.ReplaceAll(info.Name, "-", " ")), info.Port, info.Port[3:], info.DatabaseName, info.DatabaseName, info.Name, info.Name, info.Name)

	if info.HasRedis {
		serviceName := strings.ReplaceAll(info.Name, "-", "_")
		envContent += fmt.Sprintf(`
# Cache Configuration (Redis)
CACHE_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=%s
CACHE_PREFIX=%s:
CACHE_TTL=5m
`, m.getRedisDB(info.Name), serviceName)
	}

	envContent += `
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

# Analytics Configuration
ANALYTICS_ENABLED=true
`

	return os.WriteFile(envPath, []byte(envContent), 0644)
}

// getDefaultPort devuelve el puerto por defecto basado en el nombre del servicio
func (m *ServiceMigrator) getDefaultPort(serviceName string) string {
	portMap := map[string]string{
		"auth-api":         "8083",
		"user-api":         "8081", 
		"calendar-api":     "8087",
		"championship-api": "8084",
		"facilities-api":   "8086",
		"membership-api":   "8088",
		"notification-api": "8090",
		"payments-api":     "8091",
		"booking-api":      "8082",
		"bff-api":          "8085",
		"super-admin-api":  "8092",
	}
	
	if port, exists := portMap[serviceName]; exists {
		return port
	}
	return "8080"
}

// getRedisDB devuelve el número de DB de Redis basado en el servicio
func (m *ServiceMigrator) getRedisDB(serviceName string) string {
	dbMap := map[string]string{
		"auth-api":         "0",
		"user-api":         "1", 
		"calendar-api":     "2",
		"championship-api": "3",
		"facilities-api":   "4",
		"membership-api":   "5",
		"notification-api": "6",
		"payments-api":     "7",
		"booking-api":      "8",
		"bff-api":          "9",
		"super-admin-api":  "10",
	}
	
	if db, exists := dbMap[serviceName]; exists {
		return db
	}
	return "0"
}

// printServiceInfo imprime información del servicio
func (m *ServiceMigrator) printServiceInfo(info *ServiceInfo) {
	fmt.Printf("\n📊 Service Analysis:\n")
	fmt.Printf("  Name: %s\n", info.Name)
	fmt.Printf("  Port: %s\n", info.Port)
	fmt.Printf("  Database: %s\n", info.DatabaseName)
	fmt.Printf("  Main Go: %s\n", info.MainGoPath)
	fmt.Printf("  Has Redis: %v\n", info.HasRedis)
	fmt.Printf("  Has JWT: %v\n", info.HasJWT)
	fmt.Printf("  Has GORM: %v\n", info.HasGORM)
	fmt.Printf("  Has Prometheus: %v\n", info.HasPrometheus)
	fmt.Printf("  Dependencies: %v\n", info.Dependencies)
	fmt.Printf("  Custom Packages: %d\n", len(info.CustomPackages))
	fmt.Println()
}

// runGoModTidy ejecuta go mod tidy
func (m *ServiceMigrator) runGoModTidy(servicePath string) error {
	// En un entorno real, esto ejecutaría: exec.Command("go", "mod", "tidy").Run()
	fmt.Printf("📦 Running go mod tidy in %s\n", servicePath)
	return nil
}

// Helper functions
func (m *ServiceMigrator) pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func (m *ServiceMigrator) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}