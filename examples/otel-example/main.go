package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/telemetry"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
	"github.com/lukcba-developers/gopherkit/pkg/server"
	otelPkg "github.com/lukcba-developers/gopherkit/pkg/telemetry"
)

// TraceableModel modelo de ejemplo para demostrar trazas de BD
type TraceableModel struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Status    string    `json:"status" gorm:"default:active"`
	TenantID  string    `json:"tenant_id" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TraceableModel) TableName() string {
	return "traceable_examples"
}

var TraceableModels = []interface{}{
	&TraceableModel{},
}

func main() {
	// Cargar configuración
	cfg, err := config.LoadBaseConfig("otel-example")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Inicializar logger
	appLogger := logger.New("otel-example")

	// Configurar OpenTelemetry
	otelConfig := &otelPkg.OpenTelemetryConfig{
		ServiceName:           "otel-example",
		ServiceVersion:        "1.0.0",
		Environment:           cfg.Server.Environment,
		TracingEnabled:        true,
		TracingEndpoint:       "http://localhost:4317",
		TracingSampleRatio:    1.0,
		MetricsEnabled:        true,
		MetricsEndpoint:       "http://localhost:4317",
		MetricsInterval:       30 * time.Second,
		LogsEnabled:           true,
		LogsEndpoint:          "http://localhost:4317",
		DeploymentEnvironment: cfg.Server.Environment,
		ServiceNamespace:      "gopherkit-examples",
		ServiceInstanceID:     "instance-otel-1",
		Propagators:           []string{"tracecontext", "baggage"},
	}

	// Inicializar base de datos
	dbClient, err := database.NewPostgresClient(database.PostgresOptions{
		Config: cfg.Database,
		Logger: appLogger,
		Models: TraceableModels,
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

	// Configurar JWT middleware
	jwtConfig := auth.DefaultJWTConfig(cfg.Security.JWT.Secret)
	jwtConfig.SkipPaths = []string{"/health", "/metrics", "/api/v1/auth/test"}

	// Configurar health checks
	healthChecks := []observability.HealthCheck{
		dbClient.HealthCheck(),
	}
	if cacheClient != nil {
		healthChecks = append(healthChecks, cacheClient.HealthCheck())
	}

	// Crear servidor HTTP con OpenTelemetry habilitado
	httpServer, err := server.NewHTTPServer(server.Options{
		Config:              cfg,
		Logger:              appLogger,
		HealthChecks:        healthChecks,
		Routes:              setupTracedRoutes(appLogger, jwtConfig, dbClient, cacheClient),
		EnableOpenTelemetry: true,
		OTelConfig:          otelConfig,
	})
	if err != nil {
		log.Fatalf("Failed to create HTTP server: %v", err)
	}

	// Iniciar servidor
	if err := httpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "otel_service_started", map[string]interface{}{
		"service":     "otel-example",
		"port":        cfg.Server.Port,
		"otel_config": "enabled",
	})

	// Esperar señal de apagado
	if err := httpServer.WaitForShutdown(); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "otel_service_shutdown", map[string]interface{}{
		"service": "otel-example",
	})
}

func setupTracedRoutes(
	logger logger.Logger,
	jwtConfig *auth.JWTConfig,
	dbClient *database.PostgresClient,
	cacheClient *cache.RedisClient,
) func(*gin.Engine) {
	return func(router *gin.Engine) {
		// Crear wrappers de tracing
		dbWrapper := telemetry.NewDatabaseTracingWrapper(nil, logger) // Será inyectado por el servidor
		var cacheWrapper *telemetry.CacheTracingWrapper
		if cacheClient != nil {
			cacheWrapper = telemetry.NewCacheTracingWrapper(nil, logger)
		}
		businessRecorder := telemetry.NewBusinessEventRecorder(nil, logger)

		// Grupo API con versionado
		v1 := router.Group("/api/v1")
		{
			// Rutas públicas con trazas automáticas
			v1.GET("/ping", func(c *gin.Context) {
				// Los spans HTTP se crean automáticamente por el middleware
				businessRecorder.RecordEvent(*c, "ping_request", "public", map[string]interface{}{
					"user_agent": c.Request.UserAgent(),
					"ip":         c.ClientIP(),
				})

				c.JSON(200, gin.H{
					"message":  "pong with tracing",
					"service":  "otel-example",
					"time":     time.Now().UTC(),
					"trace_id": c.GetHeader("X-Trace-ID"),
				})
			})

			// Endpoint para simular operaciones complejas con spans anidados
			v1.GET("/complex-operation", func(c *gin.Context) {
				ctx := c.Request.Context()
				
				// Simular operación compleja con spans manuales
				// El span principal ya fue creado por el middleware HTTP
				
				// Span para validación
				businessRecorder.RecordEvent(*c, "complex_operation_started", "public", map[string]interface{}{
					"operation": "complex-validation-and-processing",
				})

				// Simular operación de base de datos con tracing
				if dbClient != nil {
					err := dbWrapper.TraceQuery(*c, "SELECT", "traceable_examples", "SELECT COUNT(*) FROM traceable_examples", func() error {
						// Simular query que toma tiempo
						time.Sleep(50 * time.Millisecond)
						return nil
					})
					if err != nil {
						c.JSON(500, gin.H{"error": "database operation failed"})
						return
					}
				}

				// Simular operación de cache con tracing
				if cacheWrapper != nil {
					err := cacheWrapper.TraceOperation(*c, "GET", "complex:operation:result", func() (bool, error) {
						// Simular cache miss
						time.Sleep(10 * time.Millisecond)
						return false, nil
					})
					if err != nil {
						logger.LogError(ctx, err, "cache operation failed", nil)
					}
				}

				// Simular procesamiento de negocio
				time.Sleep(100 * time.Millisecond)

				businessRecorder.RecordEvent(*c, "complex_operation_completed", "public", map[string]interface{}{
					"operation":       "complex-validation-and-processing",
					"processing_time": "150ms",
				})

				c.JSON(200, gin.H{
					"message":    "Complex operation completed successfully",
					"service":    "otel-example",
					"operations": []string{"validation", "database_query", "cache_lookup", "business_processing"},
					"trace_id":   c.GetHeader("X-Trace-ID"),
					"span_id":    c.GetHeader("X-Span-ID"),
				})
			})

			// Rutas protegidas con autenticación trazada
			protected := v1.Group("")
			protected.Use(auth.JWTMiddleware(jwtConfig))
			{
				// CRUD con trazas detalladas
				examples := protected.Group("/traced-examples")
				{
					// Crear con traza completa
					examples.POST("", func(c *gin.Context) {
						tenantID := c.GetString("tenant_id")
						userID := c.GetString("user_id")

						var req struct {
							Name   string `json:"name" binding:"required"`
							Status string `json:"status"`
						}

						if err := c.ShouldBindJSON(&req); err != nil {
							businessRecorder.RecordEvent(*c, "validation_error", tenantID, map[string]interface{}{
								"error":    err.Error(),
								"endpoint": "/traced-examples",
								"user_id":  userID,
							})
							c.JSON(400, gin.H{"error": err.Error()})
							return
						}

						// Operación de base de datos trazada
						model := &TraceableModel{
							Name:     req.Name,
							Status:   req.Status,
							TenantID: tenantID,
						}

						err := dbWrapper.TraceQuery(*c, "INSERT", "traceable_examples", "INSERT INTO traceable_examples...", func() error {
							return dbClient.GetDB().WithContext(c.Request.Context()).Create(model).Error
						})

						if err != nil {
							businessRecorder.RecordEvent(*c, "create_error", tenantID, map[string]interface{}{
								"error":   err.Error(),
								"user_id": userID,
							})
							c.JSON(500, gin.H{"error": "failed to create example"})
							return
						}

						// Invalidar cache trazado
						if cacheWrapper != nil {
							cacheWrapper.TraceOperation(*c, "DELETE", "examples:"+tenantID, func() (bool, error) {
								if cacheClient != nil {
									return false, cacheClient.Delete(c.Request.Context(), "examples:"+tenantID)
								}
								return false, nil
							})
						}

						businessRecorder.RecordEvent(*c, "example_created_with_traces", tenantID, map[string]interface{}{
							"example_id":   model.ID,
							"example_name": model.Name,
							"user_id":      userID,
							"traced":       true,
						})

						c.JSON(201, gin.H{
							"data":     model,
							"trace_id": c.GetHeader("X-Trace-ID"),
							"message":  "Created with full distributed tracing",
						})
					})

					// Listar con cache y trazas
					examples.GET("", func(c *gin.Context) {
						tenantID := c.GetString("tenant_id")
						userID := c.GetString("user_id")

						var examples []TraceableModel
						var fromCache bool

						// Intentar obtener desde cache con tracing
						if cacheWrapper != nil {
							err := cacheWrapper.TraceOperation(*c, "GET", "examples:"+tenantID, func() (bool, error) {
								// Simular probabilidad de cache hit del 70%
								if time.Now().Unix()%10 < 7 {
									fromCache = true
								}
								time.Sleep(5 * time.Millisecond)
								return fromCache, nil
							})
							if err != nil {
								logger.LogError(c.Request.Context(), err, "cache lookup failed", nil)
							}
						}

						if !fromCache {
							// Query de base de datos trazada
							err := dbWrapper.TraceQuery(*c, "SELECT", "traceable_examples", "SELECT * FROM traceable_examples WHERE tenant_id = ?", func() error {
								return dbClient.GetDB().WithContext(c.Request.Context()).
									Where("tenant_id = ?", tenantID).
									Find(&examples).Error
							})

							if err != nil {
								businessRecorder.RecordEvent(*c, "list_error", tenantID, map[string]interface{}{
									"error":   err.Error(),
									"user_id": userID,
								})
								c.JSON(500, gin.H{"error": "failed to list examples"})
								return
							}

							// Guardar en cache con tracing
							if cacheWrapper != nil {
								cacheWrapper.TraceOperation(*c, "SET", "examples:"+tenantID, func() (bool, error) {
									time.Sleep(3 * time.Millisecond)
									return true, nil
								})
							}
						}

						businessRecorder.RecordEvent(*c, "examples_listed_with_traces", tenantID, map[string]interface{}{
							"count":      len(examples),
							"from_cache": fromCache,
							"user_id":    userID,
							"traced":     true,
						})

						c.JSON(200, gin.H{
							"data":       examples,
							"from_cache": fromCache,
							"trace_id":   c.GetHeader("X-Trace-ID"),
							"span_id":    c.GetHeader("X-Span-ID"),
							"traced":     "full_distributed_tracing_enabled",
						})
					})
				}

				// Endpoint para demostrar propagación de contexto
				v1.POST("/trace-propagation", func(c *gin.Context) {
					tenantID := c.GetString("tenant_id")

					// Simular llamada a otro servicio (que propagaría el trace context)
					businessRecorder.RecordEvent(*c, "external_service_call_initiated", tenantID, map[string]interface{}{
						"target_service": "external-api",
						"propagation":    "trace_context",
					})

					// Simular latencia de servicio externo
					time.Sleep(200 * time.Millisecond)

					businessRecorder.RecordEvent(*c, "external_service_call_completed", tenantID, map[string]interface{}{
						"target_service": "external-api",
						"response_time":  "200ms",
						"success":        true,
					})

					c.JSON(200, gin.H{
						"message":           "Trace context propagated successfully",
						"service":           "otel-example",
						"external_call":     "simulated",
						"trace_id":          c.GetHeader("X-Trace-ID"),
						"propagated_to":     "external-api",
						"distributed_trace": "enabled",
					})
				})
			}
		}

		// Endpoint de métricas personalizadas con traces
		router.GET("/otel-metrics", func(c *gin.Context) {
			businessRecorder.RecordEvent(*c, "custom_metrics_requested", "system", map[string]interface{}{
				"endpoint": "/otel-metrics",
			})

			c.JSON(200, gin.H{
				"message":     "OpenTelemetry metrics and traces active",
				"service":     "otel-example",
				"trace_id":    c.GetHeader("X-Trace-ID"),
				"metrics_url": "/metrics",
				"traces_info": "Check Jaeger UI for distributed traces",
				"features": []string{
					"automatic_http_tracing",
					"database_operation_spans",
					"cache_operation_spans",
					"business_event_tracking",
					"trace_context_propagation",
				},
			})
		})
	}
}