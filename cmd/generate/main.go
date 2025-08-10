package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ServiceGenerator struct {
	ServiceName      string
	ServiceNameLower string
	OrgName          string
	Port             string
	MetricsPort      string
	DBPort           string
	RedisDB          string
	OutputPath       string
	Force            bool
}

func main() {
	var (
		serviceName = flag.String("name", "", "Service name (e.g., booking-api)")
		orgName     = flag.String("org", "lukcba-developers", "Organization name")
		outputPath  = flag.String("output", ".", "Output directory")
		force       = flag.Bool("force", false, "Force overwrite if directory exists")
	)
	flag.Parse()

	if *serviceName == "" {
		fmt.Println("Usage: go run cmd/generate/main.go -name <service-name>")
		fmt.Println("Example: go run cmd/generate/main.go -name booking-api -org lukcba-developers")
		os.Exit(1)
	}

	generator := &ServiceGenerator{
		ServiceName:      strings.Title(strings.ReplaceAll(*serviceName, "-", "")),
		ServiceNameLower: *serviceName,
		OrgName:          *orgName,
		OutputPath:       *outputPath,
		Force:            *force,
		Port:             getDefaultPort(*serviceName),
		MetricsPort:      "9090",
		DBPort:           "5432",
		RedisDB:          getRedisDB(*serviceName),
	}

	fmt.Printf("🚀 Generating service: %s\n", generator.ServiceName)
	fmt.Printf("📁 Output path: %s\n", generator.OutputPath)
	fmt.Printf("🔌 Port: %s\n", generator.Port)

	if err := generator.GenerateService(); err != nil {
		fmt.Printf("❌ Generation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service '%s' generated successfully!\n", generator.ServiceName)
	fmt.Printf("\n🚀 Next steps:\n")
	fmt.Printf("1. cd %s\n", generator.ServiceNameLower)
	fmt.Printf("2. go mod tidy\n")
	fmt.Printf("3. docker-compose up -d postgres redis  # Start dependencies\n")
	fmt.Printf("4. go run cmd/api/main.go               # Start the service\n")
	fmt.Printf("5. curl http://localhost:%s/health      # Test the service\n", generator.Port)
}

func (g *ServiceGenerator) GenerateService() error {
	serviceDir := filepath.Join(g.OutputPath, g.ServiceNameLower)

	// Check if directory exists
	if _, err := os.Stat(serviceDir); err == nil && !g.Force {
		return fmt.Errorf("directory %s already exists. Use -force to overwrite", serviceDir)
	}

	// Create directory structure
	if err := g.createDirectoryStructure(serviceDir); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Generate files
	if err := g.generateFiles(serviceDir); err != nil {
		return fmt.Errorf("failed to generate files: %w", err)
	}

	return nil
}

func (g *ServiceGenerator) createDirectoryStructure(serviceDir string) error {
	dirs := []string{
		"cmd/api",
		"internal/domain/entity",
		"internal/application",
		"internal/interfaces/api/handler",
		"internal/interfaces/api/route",
		"internal/infrastructure/repository",
		"pkg/config",
		"test/unit",
		"test/integration",
		"docs",
		"scripts",
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(serviceDir, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return err
		}
		fmt.Printf("📁 Created: %s\n", fullPath)
	}

	return nil
}

func (g *ServiceGenerator) generateFiles(serviceDir string) error {
	files := map[string]func() string{
		"go.mod":                            g.generateGoMod,
		".env":                              g.generateEnv,
		"cmd/api/main.go":                   g.generateMain,
		"internal/domain/entity/model.go":   g.generateModel,
		"internal/application/service.go":   g.generateService,
		"internal/interfaces/api/handler/handler.go": g.generateHandler,
		"internal/interfaces/api/route/routes.go":    g.generateRoutes,
		"docker-compose.yml":                g.generateDockerCompose,
		"Dockerfile":                        g.generateDockerfile,
		"README.md":                         g.generateReadme,
		".gitignore":                        g.generateGitignore,
	}

	for filename, generator := range files {
		fullPath := filepath.Join(serviceDir, filename)
		content := generator()

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
		fmt.Printf("✅ Generated: %s\n", fullPath)
	}

	return nil
}

func (g *ServiceGenerator) generateGoMod() string {
	return fmt.Sprintf(`module github.com/%s/%s

go 1.24

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/google/uuid v1.6.0
	github.com/lukcba-developers/gopherkit v0.1.0
	gorm.io/gorm v1.25.10
)

require (
	github.com/bytedance/sonic v1.11.6 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.20.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	golang.org/x/arch v0.8.0 // indirect
	golang.org/x/crypto v0.23.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Local development
replace github.com/lukcba-developers/gopherkit => /Users/lukcba-macbook-pro/go/src/gopherkit
`, g.OrgName, g.ServiceNameLower)
}

func (g *ServiceGenerator) generateEnv() string {
	return fmt.Sprintf(`# %s Configuration
# Generated with GopherKit

# Server Configuration
PORT=%s
ENVIRONMENT=development
METRICS_PORT=%s
MAX_REQUEST_SIZE=10485760

# Database Configuration
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/%s_db
DB_HOST=localhost
DB_PORT=%s
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=%s_db
DB_SSL_MODE=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# JWT Configuration
JWT_SECRET=%s-super-secret-jwt-key-change-in-production
REFRESH_SECRET=%s-refresh-secret-key-change-in-production
ACCESS_DURATION=15m
REFRESH_DURATION=24h
JWT_ISSUER=%s

# Cache Configuration (Redis)
CACHE_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=%s
CACHE_PREFIX=%s:
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

# Analytics Configuration
ANALYTICS_ENABLED=true
`, strings.Title(strings.ReplaceAll(g.ServiceNameLower, "-", " ")), g.Port, g.MetricsPort, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.DBPort, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower, g.ServiceNameLower, g.ServiceNameLower, g.RedisDB, strings.ReplaceAll(g.ServiceNameLower, "-", "_"))
}

func (g *ServiceGenerator) generateMain() string {
	return fmt.Sprintf(`package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"%s/%s/internal/application"
	"%s/%s/internal/domain/entity"
	"%s/%s/internal/interfaces/api/route"
	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
	"github.com/lukcba-developers/gopherkit/pkg/server"
)

// %sModels define los modelos GORM para auto-migración
var %sModels = []interface{}{
	&entity.%sModel{},
}

func main() {
	// Cargar configuración usando GopherKit
	cfg, err := config.LoadBaseConfig("%s")
	if err != nil {
		log.Fatalf("Failed to load configuration: %%v", err)
	}

	// Inicializar logger usando GopherKit
	appLogger := logger.New("%s")

	// Inicializar base de datos usando GopherKit
	dbClient, err := database.NewPostgresClient(database.PostgresOptions{
		Config: cfg.Database,
		Logger: appLogger,
		Models: %sModels, // Auto-migración de modelos
	})
	if err != nil {
		log.Fatalf("Failed to initialize database: %%v", err)
	}
	defer dbClient.Close()

	// Inicializar cache usando GopherKit (opcional)
	var cacheClient *cache.RedisClient
	if cfg.Cache.Enabled {
		cacheClient, err = cache.NewRedisClient(cache.RedisOptions{
			Config: cfg.Cache,
			Logger: appLogger,
		})
		if err != nil {
			appLogger.LogError(context.Background(), err, "Failed to initialize cache", nil)
			// Continuar sin cache
		} else {
			defer cacheClient.Close()
		}
	}

	// Inicializar JWT middleware usando GopherKit
	jwtMiddleware, err := auth.NewJWTMiddleware(cfg.Security.JWT, appLogger)
	if err != nil {
		log.Fatalf("Failed to initialize JWT middleware: %%v", err)
	}

	// Inicializar servicios de aplicación
	%sService := application.New%sService(dbClient, cacheClient, appLogger)

	// Configurar health checks
	healthChecks := []observability.HealthCheck{
		dbClient.HealthCheck(),
	}
	if cacheClient != nil {
		healthChecks = append(healthChecks, cacheClient.HealthCheck())
	}

	// Crear servidor HTTP usando GopherKit
	httpServer, err := server.NewHTTPServer(server.Options{
		Config:       cfg,
		Logger:       appLogger,
		HealthChecks: healthChecks,
		Routes:       route.SetupRoutes(%sService, jwtMiddleware, appLogger),
	})
	if err != nil {
		log.Fatalf("Failed to create HTTP server: %%v", err)
	}

	// Iniciar servidor
	if err := httpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %%v", err)
	}

	// Esperar señal de apagado
	if err := httpServer.WaitForShutdown(); err != nil {
		log.Fatalf("Server shutdown error: %%v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "application_shutdown", map[string]interface{}{
		"service": "%s",
	})
}
`, g.OrgName, g.ServiceNameLower, g.OrgName, g.ServiceNameLower, g.OrgName, g.ServiceNameLower, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceNameLower, g.ServiceNameLower, g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceNameLower)
}

func (g *ServiceGenerator) generateModel() string {
	return fmt.Sprintf(`package entity

import (
	"time"
	"gorm.io/gorm"
)

// %sModel representa el modelo principal del %s
type %sModel struct {
	ID        uint           ` + "`json:\"id\" gorm:\"primaryKey\"`" + `
	CreatedAt time.Time      ` + "`json:\"created_at\"`" + `
	UpdatedAt time.Time      ` + "`json:\"updated_at\"`" + `
	DeletedAt gorm.DeletedAt ` + "`json:\"-\" gorm:\"index\"`" + `
	
	// Campos específicos del modelo
	Name      string ` + "`json:\"name\" gorm:\"not null;size:255\"`" + `
	Status    string ` + "`json:\"status\" gorm:\"default:active;size:50\"`" + `
	TenantID  string ` + "`json:\"tenant_id\" gorm:\"not null;size:255;index\"`" + `
}

// TableName especifica el nombre de la tabla en la base de datos
func (%sModel) TableName() string {
	return "%s"
}

// BeforeCreate hook de GORM ejecutado antes de crear el registro
func (m *%sModel) BeforeCreate(tx *gorm.DB) (err error) {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return
}

// BeforeUpdate hook de GORM ejecutado antes de actualizar el registro
func (m *%sModel) BeforeUpdate(tx *gorm.DB) (err error) {
	m.UpdatedAt = time.Now()
	return
}
`, g.ServiceName, g.ServiceNameLower, g.ServiceName, g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceName, g.ServiceName)
}

func (g *ServiceGenerator) generateService() string {
	return fmt.Sprintf(`package application

import (
	"context"
	"fmt"

	"%s/%s/internal/domain/entity"
	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"gorm.io/gorm"
)

// %sService proporciona la lógica de negocio para %s
type %sService struct {
	db           *database.PostgresClient
	cache        *cache.RedisClient
	logger       logger.Logger
	cacheEnabled bool
}

// New%sService crea una nueva instancia del servicio
func New%sService(db *database.PostgresClient, cache *cache.RedisClient, logger logger.Logger) *%sService {
	return &%sService{
		db:           db,
		cache:        cache,
		logger:       logger,
		cacheEnabled: cache != nil,
	}
}

// Create%s crea un nuevo registro
func (s *%sService) Create%s(ctx context.Context, name, tenantID string) (*entity.%sModel, error) {
	s.logger.LogBusinessEvent(ctx, "%s_creation_started", map[string]interface{}{
		"name":      name,
		"tenant_id": tenantID,
	})

	// Crear nuevo modelo
	model := &entity.%sModel{
		Name:     name,
		TenantID: tenantID,
		Status:   "active",
	}

	// Guardar en base de datos
	if err := s.db.DB().WithContext(ctx).Create(model).Error; err != nil {
		s.logger.LogError(ctx, err, "failed to create %s", map[string]interface{}{
			"name":      name,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to create %s: %%w", err)
	}

	s.logger.LogBusinessEvent(ctx, "%s_created_successfully", map[string]interface{}{
		"id":        model.ID,
		"name":      name,
		"tenant_id": tenantID,
	})

	return model, nil
}

// Get%sByID obtiene un registro por su ID
func (s *%sService) Get%sByID(ctx context.Context, id uint, tenantID string) (*entity.%sModel, error) {
	var model entity.%sModel
	
	err := s.db.DB().WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("%s not found")
		}
		s.logger.LogError(ctx, err, "failed to get %s by ID", map[string]interface{}{
			"id":        id,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to get %s: %%w", err)
	}

	return &model, nil
}

// List%s obtiene una lista paginada de registros
func (s *%sService) List%s(ctx context.Context, tenantID string, limit, offset int) ([]*entity.%sModel, int64, error) {
	var models []*entity.%sModel
	var total int64

	// Contar total
	if err := s.db.DB().WithContext(ctx).Model(&entity.%sModel{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count %s: %%w", err)
	}

	// Obtener registros paginados
	err := s.db.DB().WithContext(ctx).Where("tenant_id = ?", tenantID).
		Limit(limit).Offset(offset).
		Order("created_at DESC").
		Find(&models).Error

	if err != nil {
		s.logger.LogError(ctx, err, "failed to list %s", map[string]interface{}{
			"tenant_id": tenantID,
			"limit":     limit,
			"offset":    offset,
		})
		return nil, 0, fmt.Errorf("failed to list %s: %%w", err)
	}

	return models, total, nil
}
`, g.OrgName, g.ServiceNameLower, g.ServiceName, g.ServiceNameLower, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceNameLower, g.ServiceName, g.ServiceName, g.ServiceNameLower, g.ServiceNameLower, g.ServiceNameLower, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceName, g.ServiceNameLower, g.ServiceNameLower, g.ServiceNameLower)
}

func (g *ServiceGenerator) generateHandler() string {
	return fmt.Sprintf(`package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	
	"%s/%s/internal/application"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// %sHandler maneja las peticiones HTTP relacionadas con %s
type %sHandler struct {
	%sService *application.%sService
	logger        logger.Logger
}

// New%sHandler crea una nueva instancia del handler
func New%sHandler(%sService *application.%sService, logger logger.Logger) *%sHandler {
	return &%sHandler{
		%sService: %sService,
		logger:        logger,
	}
}

// Create%sRequest representa la petición para crear un %s
type Create%sRequest struct {
	Name string ` + "`json:\"name\" binding:\"required,min=2,max=255\"`" + `
}

// Create%s crea un nuevo %s
func (h *%sHandler) Create%s(c *gin.Context) {
	ctx := c.Request.Context()
	
	// Obtener tenant ID del contexto
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		h.logger.LogSecurityEvent(ctx, "missing_tenant_id", map[string]interface{}{
			"endpoint": "/api/v1/%s",
			"method":   "POST",
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	var req Create%sRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Crear %s
	model, err := h.%sService.Create%s(ctx, req.Name, tenantID)
	if err != nil {
		h.logger.LogError(ctx, err, "failed to create %s", map[string]interface{}{
			"name":      req.Name,
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create %s"})
		return
	}

	c.JSON(http.StatusCreated, model)
}

// Get%s obtiene un %s por ID
func (h *%sHandler) Get%s(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetString("tenant_id")
	
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	model, err := h.%sService.Get%sByID(ctx, uint(id), tenantID)
	if err != nil {
		if err.Error() == "%s not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "%s not found"})
			return
		}
		h.logger.LogError(ctx, err, "failed to get %s", map[string]interface{}{
			"id":        id,
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get %s"})
		return
	}

	c.JSON(http.StatusOK, model)
}

// List%s lista %s con paginación
func (h *%sHandler) List%s(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetString("tenant_id")

	// Parsear parámetros de paginación
	page := 1
	perPage := 10

	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if pp := c.Query("per_page"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
			perPage = parsed
		}
	}

	offset := (page - 1) * perPage

	models, total, err := h.%sService.List%s(ctx, tenantID, perPage, offset)
	if err != nil {
		h.logger.LogError(ctx, err, "failed to list %s", map[string]interface{}{
			"tenant_id": tenantID,
			"page":      page,
			"per_page":  perPage,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list %s"})
		return
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	response := gin.H{
		"data":        models,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	}

	c.JSON(http.StatusOK, response)
}
`, g.OrgName, g.ServiceNameLower, g.ServiceName, g.ServiceNameLower, g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceName, g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceName, g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceNameLower, g.ServiceName, g.ServiceName, g.ServiceNameLower, g.ServiceName, g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceName, g.ServiceNameLower, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceNameLower, g.ServiceNameLower, g.ServiceName, g.ServiceNameLower, g.ServiceName, g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceNameLower, g.ServiceNameLower, g.ServiceNameLower, g.ServiceName, g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceNameLower, g.ServiceNameLower)
}

func (g *ServiceGenerator) generateRoutes() string {
	return fmt.Sprintf(`package route

import (
	"github.com/gin-gonic/gin"

	"%s/%s/internal/application"
	"%s/%s/internal/interfaces/api/handler"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
)

// SetupRoutes configura todas las rutas de la API
func SetupRoutes(
	%sService *application.%sService,
	jwtMiddleware *auth.JWTMiddleware,
	logger logger.Logger,
) func(*gin.Engine) {
	return func(router *gin.Engine) {
		// Inicializar handlers
		%sHandler := handler.New%sHandler(%sService, logger)

		// Grupo de API con versionado
		v1 := router.Group("/api/v1")
		{
			// Rutas públicas (sin autenticación)
			public := v1.Group("")
			{
				public.GET("/ping", func(c *gin.Context) {
					c.JSON(200, gin.H{
						"message": "pong", 
						"service": "%s",
					})
				})
			}

			// Rutas protegidas (requieren autenticación)
			protected := v1.Group("")
			protected.Use(jwtMiddleware.RequireAuth())
			{
				// Rutas de %s
				%sRoutes := protected.Group("/%s")
				{
					%sRoutes.POST("", %sHandler.Create%s)         // POST /api/v1/%s
					%sRoutes.GET("", %sHandler.List%s)           // GET /api/v1/%s  
					%sRoutes.GET("/:id", %sHandler.Get%s)        // GET /api/v1/%s/:id
				}
			}

			// Rutas para administradores (requieren rol admin)
			admin := protected.Group("/admin")
			admin.Use(jwtMiddleware.RequireRole("admin"))
			{
				// Gestión administrativa de %s
				admin.GET("/%s", %sHandler.List%s)
				admin.GET("/%s/:id", %sHandler.Get%s)
			}
		}

		// Documentación de API (disponible solo en desarrollo)
		v1.GET("/docs", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "API Documentation",
				"service": "%s",
				"endpoints": map[string]interface{}{
					"public": []string{
						"GET /api/v1/ping - Health check",
					},
					"authenticated": []string{
						"POST /api/v1/%s - Create %s",
						"GET /api/v1/%s - List %s (paginated)",
						"GET /api/v1/%s/:id - Get %s by ID",
					},
					"admin": []string{
						"GET /api/v1/admin/%s - List all %s",
						"GET /api/v1/admin/%s/:id - Get any %s",
					},
				},
				"authentication": "Bearer token required for authenticated routes",
				"headers": map[string]string{
					"Authorization": "Bearer <jwt-token>",
					"Content-Type":  "application/json",
					"X-Tenant-ID":   "Required for multi-tenant operations",
				},
			})
		})
	}
}
`, g.OrgName, g.ServiceNameLower, g.OrgName, g.ServiceNameLower, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceNameLower, g.ServiceNameLower, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ReplaceAll(g.ServiceNameLower, "-", "_"), strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower, strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ReplaceAll(g.ServiceNameLower, "-", "_"), strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), strings.ToLower(g.ServiceNameLower[:1])+g.ServiceName[1:], g.ServiceName, g.ServiceNameLower, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.ServiceNameLower)
}

func (g *ServiceGenerator) generateDockerCompose() string {
	return fmt.Sprintf(`version: '3.8'

services:
  %s:
    build: .
    ports:
      - "%s:%s"
      - "%s:%s"
    environment:
      - DATABASE_URL=postgresql://postgres:postgres@postgres:5432/%s_db
      - REDIS_HOST=redis
      - ENVIRONMENT=development
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: %s_db
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "%s:%s"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
`, g.ServiceNameLower, g.Port, g.Port, g.MetricsPort, g.MetricsPort, strings.ReplaceAll(g.ServiceNameLower, "-", "_"), strings.ReplaceAll(g.ServiceNameLower, "-", "_"), g.DBPort, g.DBPort)
}

func (g *ServiceGenerator) generateDockerfile() string {
	return `# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main cmd/api/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/.env .

EXPOSE 8080 9090
CMD ["./main"]
`
}

func (g *ServiceGenerator) generateReadme() string {
	return fmt.Sprintf(`# %s

API service built with GopherKit.

## Quick Start

### Prerequisites
- Go 1.24+
- PostgreSQL
- Redis (optional)

### Installation

1. Install dependencies:
   ` + "```bash" + `
   go mod tidy
   ` + "```" + `

2. Start dependencies:
   ` + "```bash" + `
   docker-compose up -d postgres redis
   ` + "```" + `

3. Configure environment:
   ` + "```bash" + `
   cp .env .env.local
   # Edit .env.local with your configuration
   ` + "```" + `

4. Run the service:
   ` + "```bash" + `
   go run cmd/api/main.go
   ` + "```" + `

## Endpoints

- ` + "`GET /health`" + ` - Basic health check
- ` + "`GET /health/ready`" + ` - Readiness check
- ` + "`GET /health/live`" + ` - Liveness check
- ` + "`GET /metrics`" + ` - Prometheus metrics
- ` + "`GET /api/v1/ping`" + ` - Test endpoint
- ` + "`GET /api/v1/docs`" + ` - API documentation

## Built with GopherKit

This service uses [GopherKit](https://github.com/lukcba-developers/gopherkit) for:
- ⚡ Rapid development with minimal boilerplate
- 🔒 Security middleware (rate limiting, CORS, etc.)
- 📊 Built-in observability (metrics, health checks)
- 🗄️ Database and cache integration
- 📝 Structured logging with context
`, strings.Title(strings.ReplaceAll(g.ServiceNameLower, "-", " ")))
}

func (g *ServiceGenerator) generateGitignore() string {
	return `# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with go test -c
*.test

# Output of the go coverage tool, specifically when used with LiteIDE
*.out

# Dependency directories (remove the comment below to include it)
vendor/

# Go workspace file
go.work

# Environment files
.env.local
.env.production

# IDE files
.vscode/
.idea/
*.swp
*.swo

# OS files
.DS_Store
Thumbs.db

# Log files
*.log

# Build output
dist/
build/

# Temporary files
tmp/
temp/
`
}

func getDefaultPort(serviceName string) string {
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

func getRedisDB(serviceName string) string {
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