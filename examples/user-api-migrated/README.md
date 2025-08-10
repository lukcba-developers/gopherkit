# User API - Migrated with GopherKit

Este ejemplo demuestra la migración exitosa del `user-api` original al usar GopherKit, mostrando una **reducción del 83% en el código del main.go** y una arquitectura más limpia y mantenible.

## 📊 Comparación: Antes vs Después

| Aspecto | Original | Con GopherKit | Mejora |
|---------|----------|---------------|---------|
| **Líneas en main.go** | 509 líneas | 85 líneas | **83% reducción** |
| **Middlewares personalizados** | 15+ middlewares custom | Incluidos en GopherKit | **Mantenimiento eliminado** |
| **Configuración** | 200+ líneas de config | 1 línea con LoadBaseConfig | **99% reducción** |
| **Logging** | Logger custom + logrus | Logger contextual integrado | **Estandarizado** |
| **Base de datos** | Setup manual complejo | Auto-migración GORM | **Simplificado** |
| **Cache** | Setup Redis manual | Cliente Redis integrado | **Plug & play** |
| **Health checks** | Implementación manual | Health checks automáticos | **Built-in** |
| **Observabilidad** | Setup manual complejo | Métricas y trazas incluidas | **Empresarial** |

## 🚀 Funcionalidades Migradas

### ✅ Mantenidas 100%
- **API completa de usuarios** - Todos los endpoints CRUD
- **Autenticación JWT** - Tokens, refresh, roles
- **Multi-tenancy** - Isolación por tenant
- **Estadísticas de usuario** - Wins, losses, ranking
- **Paginación** - Lista de usuarios paginada
- **Validación** - Todos los validadores de entrada
- **Cache Redis** - Con invalidación inteligente
- **Soft delete** - Eliminación lógica
- **CORS y seguridad** - Headers de seguridad

### 🆕 Mejoras Añadidas
- **Logging contextual automático** - tenant_id, user_id, correlation_id
- **Métricas empresariales** - Prometheus out-of-the-box
- **Health checks robustos** - Base de datos, Redis, servicios externos
- **Circuit breakers** - Para resilencia automática
- **Rate limiting inteligente** - Por usuario/IP/tenant
- **Observabilidad completa** - Traces, métricas, logs estructurados

## 🏗️ Arquitectura

```
user-api-migrated/
├── cmd/api/main.go           # 85 líneas vs 509 originales
├── internal/
│   ├── domain/entity/        # Modelos GORM con hooks
│   ├── application/          # Servicios de negocio
│   └── interfaces/api/       # Handlers y rutas HTTP
├── .env                      # Configuración completa
└── go.mod                    # Dependencias GopherKit
```

## 🔧 Instalación y Uso

### Prerrequisitos
- Go 1.24+
- PostgreSQL
- Redis (opcional)

### 1. Instalar dependencias
```bash
cd examples/user-api-migrated
go mod tidy
```

### 2. Configurar base de datos
```bash
# Crear base de datos
createdb userapi_db

# Las tablas se crean automáticamente con GORM auto-migration
```

### 3. Configurar variables de entorno
```bash
cp .env .env.local
# Editar .env.local con tu configuración
```

### 4. Ejecutar el servicio
```bash
go run cmd/api/main.go
```

El servicio estará disponible en `http://localhost:8081`

## 📋 Endpoints API

### Públicos
- `GET /health` - Health check básico
- `GET /health/ready` - Readiness check
- `GET /health/live` - Liveness check
- `GET /metrics` - Métricas Prometheus
- `GET /api/v1/ping` - Test de conectividad

### Autenticados (requiere JWT)
- `POST /api/v1/users` - Crear usuario
- `GET /api/v1/users` - Listar usuarios (paginado)
- `GET /api/v1/users/me` - Perfil del usuario actual
- `GET /api/v1/users/:id` - Obtener usuario por ID
- `PUT /api/v1/users/:id` - Actualizar usuario
- `DELETE /api/v1/users/:id` - Eliminar usuario

### Administrativos (requiere rol admin)
- `GET /api/v1/admin/users` - Gestión de usuarios
- Todas las operaciones de admin

## 🧪 Ejemplos de Uso

### Crear usuario
```bash
curl -X POST http://localhost:8081/api/v1/users \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: tenant123" \
  -d '{
    "email": "user@example.com",
    "name": "John Doe",
    "skill_level": "INTERMEDIATE",
    "sports_preferences": {
      "tennis": {"level": "advanced", "frequency": "weekly"},
      "football": {"level": "beginner", "frequency": "monthly"}
    }
  }'
```

### Listar usuarios con estadísticas
```bash
curl "http://localhost:8081/api/v1/users?page=1&per_page=10&include_stats=true" \
  -H "Authorization: Bearer <jwt-token>" \
  -H "X-Tenant-ID: tenant123"
```

### Ver perfil actual
```bash
curl http://localhost:8081/api/v1/users/me \
  -H "Authorization: Bearer <jwt-token>"
```

## 💡 Características Destacadas

### Logging Contextual Automático
```go
// GopherKit automáticamente extrae y registra:
// - tenant_id del header X-Tenant-ID  
// - user_id del JWT token
// - correlation_id para trazabilidad
// - request_id único por petición

logger.LogBusinessEvent(ctx, "user_created", map[string]interface{}{
    "user_id": user.ID,
    "email": user.Email,
})
// Output: {"level":"info","tenant_id":"tenant123","user_id":"user456","correlation_id":"abc123","event":"user_created","email":"user@example.com"}
```

### Cache Inteligente
```go
// Cache con invalidación automática
user, err := userService.GetUserByID(ctx, userID, tenantID)
// - Busca en cache primero
// - Si no existe, consulta BD y cachea
// - Invalida automáticamente en updates/deletes
```

### Health Checks Robustos
```bash
# Health check básico
curl http://localhost:8081/health

# Readiness con dependencias
curl http://localhost:8081/health/ready
# Verifica: PostgreSQL ✅, Redis ✅, Servicios externos ✅
```

## 🔒 Seguridad

- **JWT Authentication** - Con refresh tokens
- **Rate limiting** - Por IP, usuario y tenant
- **Input validation** - Sanitización automática
- **CORS configurado** - Headers de seguridad
- **Secrets management** - Variables sensibles protegidas
- **SQL injection prevention** - GORM ORM seguro

## 📈 Observabilidad

### Métricas Disponibles
```bash
curl http://localhost:8081/metrics
```

- `http_requests_total` - Total requests por endpoint
- `http_request_duration_seconds` - Latencia por endpoint  
- `database_connections_active` - Conexiones DB activas
- `cache_operations_total` - Operaciones de cache
- `business_events_total` - Eventos de negocio
- `user_operations_total` - Operaciones de usuario

### Logs Estructurados
Todos los logs incluyen contexto automático:
```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info", 
  "service": "user-api-migrated",
  "tenant_id": "tenant123",
  "user_id": "user456", 
  "correlation_id": "abc123",
  "event": "user_created",
  "user_id": "new-user-789",
  "email": "user@example.com"
}
```

## 🚀 Despliegue

### Docker
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o user-api cmd/api/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/user-api .
COPY --from=builder /app/.env .
EXPOSE 8081 9091
CMD ["./user-api"]
```

### Docker Compose
```yaml
version: '3.8'
services:
  user-api:
    build: .
    ports:
      - "8081:8081"
      - "9091:9091" 
    environment:
      - DATABASE_URL=postgresql://postgres:postgres@postgres:5432/userapi_db
      - REDIS_HOST=redis
    depends_on:
      - postgres
      - redis
      
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: userapi_db
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "5432:5432"
      
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

## 📝 Migración desde Original

Para migrar tu servicio existente:

1. **Instalar GopherKit**
   ```bash
   go get github.com/lukcba-developers/gopherkit
   ```

2. **Usar script de migración**
   ```bash
   ./scripts/migrate-service.sh ../club-management-system-api/user-api
   ```

3. **Adaptar modelos**
   - Convertir structs a modelos GORM
   - Añadir hooks BeforeCreate/BeforeUpdate
   - Configurar relaciones

4. **Migrar servicios**
   - Usar inyección de dependencias
   - Implementar interfaces de repositorio
   - Añadir logging contextual

5. **Actualizar handlers**
   - Usar middleware JWT de GopherKit
   - Extraer tenant_id del contexto
   - Implementar validación estándar

## 🎯 Resultado Final

**De 509 líneas complejas a 85 líneas elegantes** - La migración con GopherKit demuestra cómo una librería bien diseñada puede:

- ✅ **Reducir drasticamente el código boilerplate**
- ✅ **Mantener toda la funcionalidad original** 
- ✅ **Añadir capacidades empresariales**
- ✅ **Estandarizar patrones entre servicios**
- ✅ **Simplificar el mantenimiento**
- ✅ **Acelerar el desarrollo de nuevas funciones**

Este ejemplo sirve como **plantilla de referencia** para migrar otros microservicios del sistema al patrón GopherKit.