# GopherKit Metrics Example

Ejemplo completo de implementación de métricas de Prometheus con GopherKit, incluyendo visualización con Grafana y pruebas de carga con K6.

## 🚀 Características

### Métricas Implementadas

#### HTTP Métricas
- `http_requests_total` - Contador total de requests HTTP
- `http_request_duration_seconds` - Histograma de duración de requests
- `http_request_size_bytes` - Tamaño de requests HTTP
- `http_response_size_bytes` - Tamaño de responses HTTP
- `http_active_requests` - Gauge de requests activos

#### Database Métricas
- `db_connections_active` - Conexiones activas a la BD
- `db_connections_idle` - Conexiones idle en el pool
- `db_connections_max` - Máximo de conexiones configuradas
- `db_query_duration_seconds` - Duración de queries SQL
- `db_queries_total` - Contador total de queries

#### Cache Métricas (Redis)
- `cache_operations_total` - Operaciones de cache totales
- `cache_operations_duration_seconds` - Duración de operaciones
- `cache_hit_ratio` - Ratio de hits del cache
- `cache_keys_total` - Total de keys en cache

#### JWT Métricas
- `jwt_tokens_issued_total` - Tokens JWT emitidos
- `jwt_tokens_validated_total` - Validaciones de tokens
- `jwt_validation_duration_seconds` - Tiempo de validación

#### Business Métricas
- `business_events_total` - Eventos de negocio por tipo
- `user_sessions_active` - Sesiones de usuario activas
- `tenant_requests_total` - Requests por tenant

#### System Métricas
- `goroutines_active` - Goroutines activos
- `memory_usage_bytes` - Uso de memoria
- `cpu_usage_percent` - Uso de CPU

## 📦 Stack de Monitoreo

- **Prometheus** - Recolección y almacenamiento de métricas
- **Grafana** - Visualización y dashboards
- **AlertManager** - Gestión de alertas
- **Node Exporter** - Métricas del sistema host
- **PostgreSQL Exporter** - Métricas de PostgreSQL
- **Redis Exporter** - Métricas de Redis
- **K6** - Pruebas de carga para generar métricas

## 🏃‍♂️ Inicio Rápido

### 1. Clonar y configurar

```bash
cd examples/metrics-example
cp .env.example .env
```

### 2. Iniciar stack completo

```bash
# Iniciar todos los servicios
docker-compose up -d

# Ver logs del servicio principal
docker-compose logs -f metrics-example
```

### 3. Verificar servicios

```bash
# API Health Check
curl http://localhost:8080/health

# Métricas de Prometheus
curl http://localhost:8080/metrics

# Prometheus UI
open http://localhost:9090

# Grafana Dashboard (admin/admin)
open http://localhost:3000
```

## 📊 Dashboards y Visualización

### Grafana Dashboards

1. **Acceso:** http://localhost:3000 (admin/admin)
2. **Dashboard Principal:** "GopherKit Microservices Dashboard"
3. **Paneles incluidos:**
   - HTTP Request Rate y Response Time
   - Database Performance y Connections
   - Cache Hit Ratio y Operations
   - Business Events Tracking
   - System Resource Usage
   - JWT Operations Monitoring

### Prometheus Queries de Ejemplo

```promql
# Request rate por servicio
sum(rate(http_requests_total[5m])) by (service)

# 95th percentile response time
histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service))

# Database connection usage
(db_connections_active / db_connections_max) * 100

# Cache hit ratio
cache_hit_ratio

# Business events por tenant
sum(rate(business_events_total[5m])) by (tenant_id, event_type)
```

## 🔥 Pruebas de Carga

### Ejecutar pruebas con K6

```bash
# Prueba básica
docker-compose --profile testing up k6-load-test

# Prueba personalizada
docker run --rm -i --network metrics-example_gopherkit \
  -v $PWD/k6-scripts:/scripts \
  grafana/k6:latest run /scripts/load-test.js
```

### Escenarios de Prueba

1. **Constant Load** - Carga constante de 5 usuarios por 5 minutos
2. **Spike Test** - Pico de 20 usuarios por 30 segundos
3. **Stress Test** - Escalado gradual hasta 20 usuarios

## 🚨 Alertas y Monitoreo

### Alertas Configuradas

- **HighHTTPErrorRate** - Tasa de error > 5%
- **HighHTTPResponseTime** - Tiempo respuesta > 1s
- **HighDatabaseConnectionUsage** - Uso conexiones > 80%
- **SlowDatabaseQueries** - Queries > 500ms
- **LowCacheHitRate** - Hit ratio < 70%
- **HighMemoryUsage** - Memoria > 512MB
- **ServiceDown** - Servicio no disponible

### AlertManager

- **UI:** http://localhost:9093
- **Configuración:** `alertmanager.yml`
- **Reglas:** `rules/gopherkit-alerts.yml`

## 🧪 Endpoints de Testing

### Públicos
```bash
# Health check
curl http://localhost:8080/health

# Ping
curl http://localhost:8080/api/v1/ping
```

### Protegidos (requieren JWT)
```bash
# Headers necesarios
HEADERS="-H 'Authorization: Bearer YOUR_JWT_TOKEN' -H 'X-Tenant-ID: tenant-1'"

# Crear ejemplo
curl -X POST http://localhost:8080/api/v1/examples \
  $HEADERS \
  -H 'Content-Type: application/json' \
  -d '{"name":"Test Example","status":"active"}'

# Listar ejemplos
curl http://localhost:8080/api/v1/examples $HEADERS

# Simular eventos de negocio
curl -X POST http://localhost:8080/api/v1/simulate-events \
  $HEADERS \
  -H 'Content-Type: application/json' \
  -d '{}'
```

## 📈 Métricas de Negocio Personalizadas

El ejemplo incluye grabación automática de eventos de negocio:

```go
// Registrar evento de negocio
businessRecorder.RecordEvent("example_created", tenantID, map[string]interface{}{
    "example_id":   model.ID,
    "example_name": model.Name,
    "duration_ms":  time.Since(start).Milliseconds(),
})

// Registrar operación de base de datos
dbRecorder.RecordQuery("INSERT", "examples", duration, nil)

// Registrar operación de cache
cacheRecorder.RecordOperation("GET", duration, hit, nil)
```

## 🔧 Configuración

### Variables de Entorno

```env
# Servidor
PORT=8080
ENVIRONMENT=development
METRICS_PORT=9080

# Base de datos
DATABASE_URL=postgresql://postgres:postgres@postgres:5432/metrics_example_db
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# Cache
CACHE_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_DB=0
CACHE_TTL=5m

# Observabilidad
TRACING_ENABLED=true
METRICS_ENABLED=true
LOG_LEVEL=info
SERVICE_VERSION=1.0.0
```

### Personalización de Métricas

```go
// Crear métricas personalizadas
customMetrics := metrics.NewPrometheusMetrics("mi-servicio", "1.0.0")

// Registrar eventos personalizados
customMetrics.RecordBusinessEvent("pedido_creado", "tenant-123")
customMetrics.RecordHTTPRequest("POST", "/api/pedidos", "201", "tenant-123", 0.150)
```

## 🛠️ Desarrollo

### Estructura del Proyecto

```
metrics-example/
├── main.go                    # Aplicación principal
├── docker-compose.yml        # Stack completo
├── prometheus.yml            # Configuración Prometheus
├── rules/                    # Reglas de alertas
│   └── gopherkit-alerts.yml
├── grafana/                  # Dashboards Grafana
│   └── gopherkit-dashboard.json
├── k6-scripts/              # Scripts de pruebas
│   └── load-test.js
└── README.md
```

### Desarrollo Local

```bash
# Instalar dependencias
go mod tidy

# Ejecutar solo la aplicación
go run main.go

# Ejecutar con live reload (requiere air)
air
```

## 📚 Documentación Adicional

- [GopherKit Metrics Package](../../pkg/metrics/)
- [Prometheus Metrics Guide](https://prometheus.io/docs/practices/naming/)
- [Grafana Dashboard Guide](https://grafana.com/docs/grafana/latest/dashboards/)
- [K6 Load Testing](https://k6.io/docs/)

## 🤝 Contribución

Para contribuir al sistema de métricas:

1. Añadir nuevas métricas en `pkg/metrics/prometheus.go`
2. Crear dashboards en `examples/grafana/`
3. Añadir alertas en `examples/prometheus/rules/`
4. Documentar en este README

## 📄 Licencia

Este ejemplo es parte del proyecto GopherKit bajo licencia MIT.