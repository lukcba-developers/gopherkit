# GopherKit OpenTelemetry Example

Ejemplo completo de implementación de OpenTelemetry con GopherKit, incluyendo trazas distribuidas, métricas y logs con integración completa de Jaeger, Prometheus y Grafana.

## 🚀 Características

### OpenTelemetry Implementado

#### 📊 Tracing (Trazas Distribuidas)
- **HTTP Request Tracing** - Spans automáticos para todas las requests HTTP
- **Database Operation Spans** - Trazas detalladas de queries SQL
- **Cache Operation Spans** - Spans para operaciones Redis
- **Business Logic Spans** - Spans customizados para lógica de negocio
- **Trace Context Propagation** - Propagación automática entre servicios
- **Nested Spans** - Spans anidados para operaciones complejas

#### 📈 Metrics (Métricas)
- **HTTP Metrics** - Request rate, latency, status codes
- **Database Metrics** - Query duration, connection pools
- **Cache Metrics** - Hit ratio, operation latency
- **Business Metrics** - Eventos de negocio customizados
- **System Metrics** - CPU, memoria, goroutines

#### 📝 Logs (Registros)
- **Structured Logging** - Logs estructurados con contexto de trace
- **Correlation IDs** - Correlación automática con traces
- **Business Events** - Logs de eventos de negocio con spans

### Stack de Observabilidad

- **OpenTelemetry Collector** - Recolección y procesamiento centralizado
- **Jaeger** - Interfaz para visualización de trazas distribuidas  
- **Prometheus** - Almacenamiento y consulta de métricas
- **Grafana** - Dashboards y visualización avanzada
- **K6** - Pruebas de carga para generar telemetría

## 🏃‍♂️ Inicio Rápido

### 1. Clonar y configurar

```bash
cd examples/otel-example
cp .env.example .env
```

### 2. Iniciar stack completo

```bash
# Iniciar todos los servicios de observabilidad
docker-compose up -d

# Ver logs del servicio principal
docker-compose logs -f otel-example
```

### 3. Verificar servicios

```bash
# API Health Check
curl http://localhost:8080/health

# Verificar tracing automático
curl http://localhost:8080/api/v1/ping

# Verificar operaciones complejas con spans anidados
curl http://localhost:8080/api/v1/complex-operation
```

## 🔍 Interfaces de Observabilidad

### Jaeger - Trazas Distribuidas
- **URL:** http://localhost:16686
- **Función:** Visualización de trazas distribuidas
- **Características:**
  - Timeline de spans
  - Dependencias entre servicios
  - Análisis de latencia
  - Filtros y búsquedas avanzadas

### Grafana - Dashboards
- **URL:** http://localhost:3000 (admin/admin)
- **Función:** Visualización de métricas y correlación con trazas
- **Dashboards incluidos:**
  - OpenTelemetry Overview
  - Distributed Tracing Analytics
  - Service Performance
  - Business Metrics

### Prometheus - Métricas
- **URL:** http://localhost:9090
- **Función:** Consulta directa de métricas
- **Targets:**
  - otel-example:8080 (Servicio principal)
  - otel-collector:8888 (Collector)

### OTEL Collector - Telemetría
- **Endpoints:**
  - gRPC: localhost:4317
  - HTTP: localhost:4318
  - Metrics: localhost:8888
  - Health: localhost:13133

## 🧪 Endpoints de Testing

### Públicos (con tracing automático)
```bash
# Ping básico con trace ID
curl http://localhost:8080/api/v1/ping

# Operación compleja con spans anidados
curl http://localhost:8080/api/v1/complex-operation

# Información de métricas OTEL
curl http://localhost:8080/otel-metrics
```

### Protegidos (requieren JWT + tienen spans de BD/Cache)
```bash
# Headers necesarios
HEADERS="-H 'Authorization: Bearer YOUR_JWT_TOKEN' -H 'X-Tenant-ID: tenant-otel-1'"

# Crear ejemplo con traza completa (DB + Cache spans)
curl -X POST http://localhost:8080/api/v1/traced-examples \
  $HEADERS \
  -H 'Content-Type: application/json' \
  -d '{"name":"Traced Example","status":"active"}'

# Listar ejemplos con cache tracing
curl http://localhost:8080/api/v1/traced-examples $HEADERS

# Prueba de propagación de contexto
curl -X POST http://localhost:8080/api/v1/trace-propagation \
  $HEADERS \
  -H 'Content-Type: application/json' \
  -d '{}'
```

## 📊 Análisis de Trazas

### Visualización en Jaeger

1. **Acceder a Jaeger UI:** http://localhost:16686
2. **Buscar trazas:**
   - Service: `otel-example`
   - Operation: `POST /api/v1/traced-examples`
   - Tags: `tenant.id=tenant-otel-1`

3. **Analizar spans:**
   - **Root Span:** HTTP Request
   - **Child Spans:** Database Query, Cache Operation
   - **Nested Spans:** Business Logic

### Queries de Prometheus para OTel

```promql
# Request rate por endpoint
sum(rate(http_requests_total[5m])) by (method, route)

# P95 latency de operaciones
histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service))

# Spans creados por segundo
sum(rate(traces_spans_total[5m])) by (service)

# Errores en traces
sum(rate(traces_spans_total{status_code="ERROR"}[5m])) by (service)
```

## 🔥 Pruebas de Carga

### Ejecutar pruebas con K6

```bash
# Prueba básica de OpenTelemetry
docker-compose --profile testing up k6-load-test

# Prueba personalizada con más carga
docker run --rm -i --network otel-example_otel-network \
  -v $PWD/k6-scripts:/scripts \
  grafana/k6:latest run /scripts/otel-load-test.js \
  --vus 10 --duration 5m
```

### Métricas de Pruebas
- **traceable_operations** - Operaciones con tracing
- **distributed_trace_rate** - Tasa de trazas distribuidas  
- **span_duration_ms** - Duración de spans

## 🛠️ Implementación Detallada

### Configuración Automática

```go
// Habilitación automática de OpenTelemetry
httpServer, err := server.NewHTTPServer(server.Options{
    Config:              cfg,
    Logger:              appLogger,
    HealthChecks:        healthChecks,
    Routes:              setupTracedRoutes(...),
    EnableOpenTelemetry: true,  // 🔥 Activación automática
    OTelConfig: &telemetry.OpenTelemetryConfig{
        ServiceName:        "otel-example",
        ServiceVersion:     "1.0.0",
        TracingEnabled:     true,
        MetricsEnabled:     true,
        LogsEnabled:        true,
        TracingEndpoint:    "http://localhost:4317",
    },
})
```

### Tracing Manual

```go
// Crear spans customizados
ctx, span := otelProvider.StartSpan(ctx, "business_operation")
defer span.End()

// Añadir atributos
span.SetAttributes(
    attribute.String("user.id", userID),
    attribute.String("tenant.id", tenantID),
    attribute.String("operation", "complex_validation"),
)

// Registrar eventos
span.AddEvent("validation_started")
span.AddEvent("validation_completed")
```

### Instrumentación de Base de Datos

```go
// Wrapper automático para queries
err := dbWrapper.TraceQuery(c, "INSERT", "examples", "INSERT INTO...", func() error {
    return dbClient.DB().WithContext(ctx).Create(model).Error
})
```

### Instrumentación de Cache

```go
// Wrapper automático para cache
err := cacheWrapper.TraceOperation(c, "GET", "key:example", func() (bool, error) {
    return cacheClient.Get(ctx, "key:example")
})
```

## ⚙️ Configuración Avanzada

### OpenTelemetry Collector

El collector está configurado para:
- **Recibir:** OTLP gRPC/HTTP, Prometheus scraping
- **Procesar:** Batching, resource attributes, filtering
- **Exportar:** Jaeger (traces), Prometheus (metrics), Logs

### Variables de Entorno

```env
# OpenTelemetry Configuration
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SERVICE_NAME=otel-example
OTEL_SERVICE_VERSION=1.0.0
OTEL_TRACES_SAMPLER=always_on
OTEL_METRICS_INTERVAL=30s

# Jaeger Configuration
JAEGER_ENDPOINT=http://localhost:14268/api/traces
JAEGER_SAMPLER_TYPE=const
JAEGER_SAMPLER_PARAM=1

# Service Configuration
ENVIRONMENT=development
SERVICE_NAMESPACE=gopherkit
DEPLOYMENT_ENVIRONMENT=docker-compose
```

### Personalización de Spans

```go
// Span personalizado con contexto completo
func TraceBusinessOperation(ctx context.Context, operation string, data map[string]interface{}) {
    _, span := otel.Tracer("business").Start(ctx, operation)
    defer span.End()
    
    // Atributos dinámicos
    for key, value := range data {
        span.SetAttributes(telemetry.CreateSpanAttributes(map[string]interface{}{
            key: value,
        })...)
    }
    
    // Eventos de negocio
    span.AddEvent("operation.started")
    defer span.AddEvent("operation.completed")
}
```

## 📈 Métricas de Negocio

### Eventos Customizados

```go
// Registrar eventos de negocio con tracing
businessRecorder.RecordEvent(c, "user_registration", tenantID, map[string]interface{}{
    "registration_type": "email",
    "user_tier":         "premium",
    "referral_code":     referralCode,
})
```

### Correlación de Métricas y Trazas

- **Trace ID en métricas** - Correlación automática
- **Business events en spans** - Eventos de negocio como spans
- **Error tracking** - Errores automáticos en traces y métricas

## 🚨 Monitoreo y Alertas

### Alertas Basadas en Traces

```yaml
# Ejemplo de alerta Prometheus
- alert: HighTraceLatency
  expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1.0
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "High trace latency detected"

- alert: TraceErrorRate
  expr: rate(traces_spans_total{status_code="ERROR"}[5m]) > 0.05
  for: 1m
  labels:
    severity: critical
```

## 📚 Referencias

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/instrumentation/go/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [OTEL Collector Configuration](https://opentelemetry.io/docs/collector/configuration/)
- [Distributed Tracing Best Practices](https://opentelemetry.io/docs/concepts/observability-primer/)

## 🤝 Contribución

Para contribuir al sistema de OpenTelemetry:

1. Añadir nuevos instrumentos en `pkg/telemetry/otel.go`
2. Crear nuevos wrappers en `pkg/middleware/telemetry/`
3. Añadir configuraciones en `otel-collector-config.yaml`
4. Documentar en este README

Este ejemplo demuestra la implementación completa de observabilidad distribuida con OpenTelemetry en GopherKit 🚀