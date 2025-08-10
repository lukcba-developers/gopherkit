package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/metrics"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
	"github.com/lukcba-developers/gopherkit/pkg/server"
)

// ExampleModel modelo de ejemplo para demostrar métricas de BD
type ExampleModel struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Status    string    `json:"status" gorm:"default:active"`
	TenantID  string    `json:"tenant_id" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ExampleModel) TableName() string {
	return "examples"
}

var ExampleModels = []interface{}{
	&ExampleModel{},
}

func main() {
	// Cargar configuración
	cfg, err := config.LoadBaseConfig("metrics-example")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Inicializar logger
	appLogger := logger.New("metrics-example")

	// Inicializar base de datos
	dbClient, err := database.NewPostgresClient(database.PostgresOptions{
		Config: cfg.Database,
		Logger: appLogger,
		Models: ExampleModels,
	})
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbClient.Close()

	// Inicializar cache
	var cacheClient *cache.RedisClient
	if cfg.Cache.Enabled {
		cacheClient, err = cache.NewRedisClient(cache.RedisOptions{
			Config: cfg.Cache,
			Logger: appLogger,
		})
		if err != nil {
			appLogger.LogError(context.Background(), err, "Failed to initialize cache", nil)
		} else {
			defer cacheClient.Close()
		}
	}

	// Inicializar JWT middleware
	jwtMiddleware, err := auth.NewJWTMiddleware(cfg.Security.JWT, appLogger)
	if err != nil {
		log.Fatalf("Failed to initialize JWT middleware: %v", err)
	}

	// Configurar health checks
	healthChecks := []observability.HealthCheck{
		dbClient.HealthCheck(),
	}
	if cacheClient != nil {
		healthChecks = append(healthChecks, cacheClient.HealthCheck())
	}

	// Crear servidor HTTP con métricas integradas
	httpServer, err := server.NewHTTPServer(server.Options{
		Config:       cfg,
		Logger:       appLogger,
		HealthChecks: healthChecks,
		Routes:       setupExampleRoutes(appLogger, jwtMiddleware, dbClient, cacheClient),
	})
	if err != nil {
		log.Fatalf("Failed to create HTTP server: %v", err)
	}

	// Iniciar servidor
	if err := httpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "service_started", map[string]interface{}{
		"service": "metrics-example",
		"port":    cfg.Server.Port,
	})

	// Esperar señal de apagado
	if err := httpServer.WaitForShutdown(); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "service_shutdown", map[string]interface{}{
		"service": "metrics-example",
	})
}

func setupExampleRoutes(
	logger logger.Logger,
	jwtMiddleware *auth.JWTMiddleware,
	dbClient *database.PostgresClient,
	cacheClient *cache.RedisClient,
) func(*gin.Engine) {
	return func(router *gin.Engine) {
		// Crear registradores de métricas de negocio
		businessRecorder := metrics.NewBusinessEventRecorder(nil, logger) // Pasará las métricas del servidor
		dbRecorder := metrics.NewDatabaseMetricsRecorder(nil, logger)
		var cacheRecorder *metrics.CacheMetricsRecorder
		if cacheClient != nil {
			cacheRecorder = metrics.NewCacheMetricsRecorder(nil, logger)
		}

		// Grupo API con versionado
		v1 := router.Group("/api/v1")
		{
			// Rutas públicas
			v1.GET("/ping", func(c *gin.Context) {
				// Registrar evento de negocio
				businessRecorder.RecordEvent("ping_request", "system", map[string]interface{}{
					"user_agent": c.Request.UserAgent(),
					"ip":         c.ClientIP(),
				})

				c.JSON(200, gin.H{
					"message": "pong",
					"service": "metrics-example",
					"time":    time.Now().UTC(),
				})
			})

			// Rutas protegidas
			protected := v1.Group("")
			protected.Use(jwtMiddleware.RequireAuth())
			{
				// CRUD de ejemplos con métricas detalladas
				examples := protected.Group("/examples")
				{
					// Crear ejemplo
					examples.POST("", func(c *gin.Context) {
						start := time.Now()
						tenantID := c.GetString("tenant_id")

						var req struct {
							Name   string `json:"name" binding:"required"`
							Status string `json:"status"`
						}

						if err := c.ShouldBindJSON(&req); err != nil {
							businessRecorder.RecordEvent("validation_error", tenantID, map[string]interface{}{
								"error": err.Error(),
								"endpoint": "/examples",
							})
							c.JSON(400, gin.H{"error": err.Error()})
							return
						}

						// Simular operación de base de datos con métricas
						model := &ExampleModel{
							Name:     req.Name,
							Status:   req.Status,
							TenantID: tenantID,
						}

						dbStart := time.Now()
						if err := dbClient.DB().WithContext(c.Request.Context()).Create(model).Error; err != nil {
							dbDuration := time.Since(dbStart)
							dbRecorder.RecordQuery("INSERT", "examples", dbDuration, err)
							businessRecorder.RecordEvent("create_error", tenantID, map[string]interface{}{
								"error": err.Error(),
							})
							c.JSON(500, gin.H{"error": "failed to create example"})
							return
						}
						dbDuration := time.Since(dbStart)
						dbRecorder.RecordQuery("INSERT", "examples", dbDuration, nil)

						// Invalidar cache si existe
						if cacheClient != nil && cacheRecorder != nil {
							cacheStart := time.Now()
							cacheKey := "examples:" + tenantID
							err := cacheClient.Delete(c.Request.Context(), cacheKey)
							cacheDuration := time.Since(cacheStart)
							cacheRecorder.RecordOperation("DELETE", cacheDuration, false, err)
						}

						// Registrar evento de negocio exitoso
						businessRecorder.RecordEvent("example_created", tenantID, map[string]interface{}{
							"example_id":   model.ID,
							"example_name": model.Name,
							"duration_ms":  time.Since(start).Milliseconds(),
						})

						c.JSON(201, model)
					})

					// Listar ejemplos
					examples.GET("", func(c *gin.Context) {
						start := time.Now()
						tenantID := c.GetString("tenant_id")

						// Parsear parámetros de paginación
						page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
						limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
						offset := (page - 1) * limit

						// Intentar obtener desde cache
						var examples []ExampleModel
						var fromCache bool
						
						if cacheClient != nil && cacheRecorder != nil {
							cacheStart := time.Now()
							cacheKey := "examples:" + tenantID + ":" + strconv.Itoa(page)
							
							// Simular cache miss/hit
							if time.Now().Unix()%3 == 0 { // Simular hit ratio del 66%
								fromCache = true
							}
							
							cacheDuration := time.Since(cacheStart)
							cacheRecorder.RecordOperation("GET", cacheDuration, fromCache, nil)
							
							// Actualizar estadísticas de cache
							cacheRecorder.UpdateStats(0.66, 150)
						}

						if !fromCache {
							// Consultar base de datos
							dbStart := time.Now()
							err := dbClient.DB().WithContext(c.Request.Context()).
								Where("tenant_id = ?", tenantID).
								Limit(limit).Offset(offset).
								Find(&examples).Error

							dbDuration := time.Since(dbStart)
							if err != nil {
								dbRecorder.RecordQuery("SELECT", "examples", dbDuration, err)
								businessRecorder.RecordEvent("list_error", tenantID, map[string]interface{}{
									"error": err.Error(),
								})
								c.JSON(500, gin.H{"error": "failed to list examples"})
								return
							}
							dbRecorder.RecordQuery("SELECT", "examples", dbDuration, nil)

							// Guardar en cache
							if cacheClient != nil && cacheRecorder != nil {
								cacheStart := time.Now()
								// Simular guardado en cache
								cacheDuration := time.Since(cacheStart)
								cacheRecorder.RecordOperation("SET", cacheDuration, true, nil)
							}
						}

						// Registrar evento de negocio
						businessRecorder.RecordEvent("examples_listed", tenantID, map[string]interface{}{
							"count":       len(examples),
							"page":        page,
							"from_cache":  fromCache,
							"duration_ms": time.Since(start).Milliseconds(),
						})

						c.JSON(200, gin.H{
							"data":       examples,
							"page":       page,
							"limit":      limit,
							"from_cache": fromCache,
						})
					})

					// Obtener ejemplo por ID
					examples.GET("/:id", func(c *gin.Context) {
						start := time.Now()
						tenantID := c.GetString("tenant_id")
						id := c.Param("id")

						var example ExampleModel

						dbStart := time.Now()
						err := dbClient.DB().WithContext(c.Request.Context()).
							Where("id = ? AND tenant_id = ?", id, tenantID).
							First(&example).Error

						dbDuration := time.Since(dbStart)
						if err != nil {
							dbRecorder.RecordQuery("SELECT", "examples", dbDuration, err)
							businessRecorder.RecordEvent("get_error", tenantID, map[string]interface{}{
								"example_id": id,
								"error":      err.Error(),
							})
							
							if err.Error() == "record not found" {
								c.JSON(404, gin.H{"error": "example not found"})
								return
							}
							
							c.JSON(500, gin.H{"error": "failed to get example"})
							return
						}
						dbRecorder.RecordQuery("SELECT", "examples", dbDuration, nil)

						businessRecorder.RecordEvent("example_retrieved", tenantID, map[string]interface{}{
							"example_id":   example.ID,
							"example_name": example.Name,
							"duration_ms":  time.Since(start).Milliseconds(),
						})

						c.JSON(200, example)
					})
				}

				// Endpoint para simular diferentes tipos de eventos de negocio
				v1.POST("/simulate-events", func(c *gin.Context) {
					tenantID := c.GetString("tenant_id")

					// Simular diferentes eventos
					events := []string{
						"user_login_success",
						"user_login_failure", 
						"payment_processed",
						"booking_created",
						"notification_sent",
					}

					for i, event := range events {
						businessRecorder.RecordEvent(event, tenantID, map[string]interface{}{
							"simulation": true,
							"batch_id":   i,
						})
					}

					c.JSON(200, gin.H{
						"message": "Events simulated",
						"count":   len(events),
					})
				})
			}
		}

		// Endpoint para métricas personalizadas
		router.GET("/custom-metrics", func(c *gin.Context) {
			// Este endpoint muestra métricas customizadas
			c.JSON(200, gin.H{
				"message": "Custom metrics endpoint",
				"info":    "Check /metrics for Prometheus metrics",
			})
		})
	}
}