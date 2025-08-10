package telemetry

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/telemetry"
)

// OpenTelemetryMiddleware middleware para integrar OpenTelemetry con Gin
type OpenTelemetryMiddleware struct {
	otelProvider *telemetry.OpenTelemetryProvider
	logger       logger.Logger
	serviceName  string
}

// NewOpenTelemetryMiddleware crea un nuevo middleware de OpenTelemetry
func NewOpenTelemetryMiddleware(otelProvider *telemetry.OpenTelemetryProvider, logger logger.Logger, serviceName string) *OpenTelemetryMiddleware {
	return &OpenTelemetryMiddleware{
		otelProvider: otelProvider,
		logger:       logger,
		serviceName:  serviceName,
	}
}

// TracingMiddleware middleware de tracing para Gin
func (m *OpenTelemetryMiddleware) TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extraer contexto de propagación
		ctx := c.Request.Context()
		
		// Extraer información de tracing desde headers
		carrier := propagation.HeaderCarrier(c.Request.Header)
		ctx = propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		).Extract(ctx, carrier)
		
		// Crear span para esta request
		spanName := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			spanName = c.Request.Method + " " + c.Request.URL.Path
		}
		
		ctx, span := m.otelProvider.StartSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		
		// Actualizar contexto de request
		c.Request = c.Request.WithContext(ctx)
		
		// Añadir atributos estándar al span
		span.SetAttributes(
			semconv.HTTPMethod(c.Request.Method),
			semconv.HTTPURL(c.Request.URL.String()),
			semconv.HTTPRoute(c.FullPath()),
			semconv.HTTPScheme(c.Request.URL.Scheme),
			attribute.String("http.host", c.Request.Host),
			attribute.String("http.user_agent", c.Request.UserAgent()),
			semconv.HTTPRequestContentLength(int(c.Request.ContentLength)),
			attribute.String("service.name", m.serviceName),
		)
		
		// Añadir información de tenant si está disponible
		if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
			span.SetAttributes(attribute.String("tenant.id", tenantID))
		}
		
		// Añadir información de usuario si está disponible
		if userID := c.GetHeader("X-User-ID"); userID != "" {
			span.SetAttributes(attribute.String("user.id", userID))
		}
		
		// Añadir correlation ID si está disponible
		if correlationID := c.GetHeader("X-Correlation-ID"); correlationID != "" {
			span.SetAttributes(attribute.String("correlation.id", correlationID))
		}
		
		// Medir tiempo de inicio
		start := time.Now()
		
		// Procesar request
		c.Next()
		
		// Medir duración
		duration := time.Since(start)
		
		// Añadir atributos de response
		statusCode := c.Writer.Status()
		span.SetAttributes(
			semconv.HTTPStatusCode(statusCode),
			semconv.HTTPResponseContentLength(c.Writer.Size()),
			attribute.Float64("http.duration_ms", float64(duration.Nanoseconds())/1e6),
		)
		
		// Configurar status del span basado en código HTTP
		if statusCode >= 400 {
			span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}
		
		// Registrar métricas
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "unknown"
		}
		
		m.otelProvider.RecordHTTPRequest(
			ctx,
			c.Request.Method,
			c.FullPath(),
			strconv.Itoa(statusCode),
			duration,
			tenantID,
		)
		
		// Log si hay error
		if statusCode >= 400 {
			m.logger.LogError(ctx, nil, "http_request_error", map[string]interface{}{
				"method":      c.Request.Method,
				"path":        c.Request.URL.Path,
				"status_code": statusCode,
				"duration_ms": duration.Milliseconds(),
				"tenant_id":   tenantID,
				"user_agent":  c.Request.UserAgent(),
				"remote_addr": c.ClientIP(),
			})
			
			// Registrar error en métricas
			m.otelProvider.RecordError(ctx, "http_error", m.serviceName)
		}
		
		// Añadir headers de tracing a la response
		if span.SpanContext().IsValid() {
			c.Header("X-Trace-ID", span.SpanContext().TraceID().String())
			c.Header("X-Span-ID", span.SpanContext().SpanID().String())
		}
	}
}

// MetricsMiddleware middleware de métricas para Gin
func (m *OpenTelemetryMiddleware) MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// Procesar request
		c.Next()
		
		// Registrar métricas después del procesamiento
		duration := time.Since(start)
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "unknown"
		}
		
		m.otelProvider.RecordHTTPRequest(
			c.Request.Context(),
			c.Request.Method,
			c.FullPath(),
			strconv.Itoa(c.Writer.Status()),
			duration,
			tenantID,
		)
	}
}

// DatabaseTracingWrapper wrapper para operaciones de base de datos
type DatabaseTracingWrapper struct {
	otelProvider *telemetry.OpenTelemetryProvider
	logger       logger.Logger
}

// NewDatabaseTracingWrapper crea un wrapper para tracing de BD
func NewDatabaseTracingWrapper(otelProvider *telemetry.OpenTelemetryProvider, logger logger.Logger) *DatabaseTracingWrapper {
	return &DatabaseTracingWrapper{
		otelProvider: otelProvider,
		logger:       logger,
	}
}

// TraceQuery wrapper para queries de base de datos
func (w *DatabaseTracingWrapper) TraceQuery(ctx gin.Context, operation, table, query string, fn func() error) error {
	// Crear span para la operación de BD
	spanName := "db." + operation + "." + table
	reqCtx, span := w.otelProvider.StartSpan(ctx.Request.Context(), spanName, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	
	// Añadir atributos al span
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
		semconv.DBSQLTable(table),
		attribute.String("db.query", query),
		attribute.String("db.connection_string", "postgresql://***:***@localhost/db"),
	)
	
	// Medir tiempo
	start := time.Now()
	
	// Ejecutar operación
	err := fn()
	duration := time.Since(start)
	
	// Registrar métricas
	w.otelProvider.RecordDatabaseOperation(reqCtx, operation, table, duration)
	
	// Configurar status del span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		
		w.logger.LogError(reqCtx, err, "database_operation_error", map[string]interface{}{
			"operation":   operation,
			"table":       table,
			"duration_ms": duration.Milliseconds(),
		})
		
		w.otelProvider.RecordError(reqCtx, "db_error", "database")
	} else {
		span.SetStatus(codes.Ok, "")
		
		// Log exitoso solo en debug
		w.logger.LogBusinessEvent(reqCtx, "database_operation_success", map[string]interface{}{
			"operation":   operation,
			"table":       table,
			"duration_ms": duration.Milliseconds(),
		})
	}
	
	// Añadir métricas de duración
	span.SetAttributes(attribute.Float64("db.duration_ms", float64(duration.Nanoseconds())/1e6))
	
	return err
}

// CacheTracingWrapper wrapper para operaciones de cache
type CacheTracingWrapper struct {
	otelProvider *telemetry.OpenTelemetryProvider
	logger       logger.Logger
}

// NewCacheTracingWrapper crea un wrapper para tracing de cache
func NewCacheTracingWrapper(otelProvider *telemetry.OpenTelemetryProvider, logger logger.Logger) *CacheTracingWrapper {
	return &CacheTracingWrapper{
		otelProvider: otelProvider,
		logger:       logger,
	}
}

// TraceOperation wrapper para operaciones de cache
func (w *CacheTracingWrapper) TraceOperation(ctx gin.Context, operation, key string, fn func() (bool, error)) error {
	// Crear span para la operación de cache
	spanName := "cache." + operation
	reqCtx, span := w.otelProvider.StartSpan(ctx.Request.Context(), spanName, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	
	// Añadir atributos al span
	span.SetAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("cache.operation", operation),
		attribute.String("cache.key", key),
	)
	
	// Medir tiempo
	start := time.Now()
	
	// Ejecutar operación
	hit, err := fn()
	duration := time.Since(start)
	
	// Registrar métricas
	w.otelProvider.RecordCacheOperation(reqCtx, operation, duration, hit)
	
	// Configurar status del span
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		
		w.logger.LogError(reqCtx, err, "cache_operation_error", map[string]interface{}{
			"operation":   operation,
			"key":         key,
			"duration_ms": duration.Milliseconds(),
		})
		
		w.otelProvider.RecordError(reqCtx, "cache_error", "redis")
	} else {
		span.SetStatus(codes.Ok, "")
	}
	
	// Añadir atributos adicionales
	span.SetAttributes(
		attribute.Bool("cache.hit", hit),
		attribute.Float64("cache.duration_ms", float64(duration.Nanoseconds())/1e6),
	)
	
	return err
}

// BusinessEventRecorder registrador de eventos de negocio con tracing
type BusinessEventRecorder struct {
	otelProvider *telemetry.OpenTelemetryProvider
	logger       logger.Logger
}

// NewBusinessEventRecorder crea un registrador de eventos de negocio
func NewBusinessEventRecorder(otelProvider *telemetry.OpenTelemetryProvider, logger logger.Logger) *BusinessEventRecorder {
	return &BusinessEventRecorder{
		otelProvider: otelProvider,
		logger:       logger,
	}
}

// RecordEvent registra un evento de negocio con tracing
func (r *BusinessEventRecorder) RecordEvent(ctx gin.Context, eventType, tenantID string, data map[string]interface{}) {
	reqCtx := ctx.Request.Context()
	
	// Crear span para el evento de negocio
	spanName := "business.event." + eventType
	spanCtx, span := r.otelProvider.StartSpan(reqCtx, spanName, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	
	// Añadir atributos al span
	span.SetAttributes(
		attribute.String("business.event.type", eventType),
		attribute.String("business.tenant.id", tenantID),
	)
	
	// Añadir atributos de datos si están disponibles
	if data != nil {
		for key, value := range data {
			switch v := value.(type) {
			case string:
				span.SetAttributes(attribute.String("business.data."+key, v))
			case int:
				span.SetAttributes(attribute.Int("business.data."+key, v))
			case int64:
				span.SetAttributes(attribute.Int64("business.data."+key, v))
			case float64:
				span.SetAttributes(attribute.Float64("business.data."+key, v))
			case bool:
				span.SetAttributes(attribute.Bool("business.data."+key, v))
			}
		}
	}
	
	// Registrar métricas
	r.otelProvider.RecordBusinessEvent(spanCtx, eventType, tenantID)
	
	// Log del evento
	r.logger.LogBusinessEvent(spanCtx, eventType, map[string]interface{}{
		"tenant_id": tenantID,
		"data":      data,
		"trace_id":  span.SpanContext().TraceID().String(),
		"span_id":   span.SpanContext().SpanID().String(),
	})
	
	span.SetStatus(codes.Ok, "")
}