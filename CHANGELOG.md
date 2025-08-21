# Changelog

Todos los cambios notables de GopherKit serán documentados en este archivo.

El formato está basado en [Keep a Changelog](https://keepachangelog.com/es/1.0.0/),
y este proyecto sigue [Versionado Semántico](https://semver.org/lang/es/).

## [1.0.3] - 2025-08-21

### 🗂️ **Reorganización de Ejemplos**
- **BREAKING**: Consolidación completa de estructura de ejemplos
  - Eliminado directorio duplicado `example/` 
  - Reorganización jerárquica de `examples/` por categorías
  - Nueva estructura: `basic/`, `api-services/`, `observability/`, `infrastructure/`, `guides/`

### 📚 **Documentación**
- **Nuevos READMEs**: Documentación completa para cada categoría de ejemplos
- **Índice principal**: `examples/README.md` with navegación clara
- **Referencias actualizadas**: Links corregidos en `docs/EXAMPLES.md` y `README.md`
- **Guías organizadas**: Migración de guías a `examples/guides/`

### 🏗️ **Estructura Mejorada**
- **Categorización lógica**: Ejemplos organizados por complejidad y propósito
- **Navegación intuitiva**: Progresión natural desde básico hasta avanzado  
- **Escalabilidad**: Estructura preparada para futuros ejemplos
- **Limpieza**: Eliminados archivos binarios y temporales redundantes

### 🔧 **Fixes**
- **Tests de configuración**: Resueltos problemas de compilación en `hot_reload_test.go`
  - Corregido `mockLogger` undefined
  - Arregladas incompatibilidades de tipos en `UpdateConfig`
  - Fixed `RegisterCallback` signature y return values
  - Solucionado lock copy issue en `GetMetrics`

### ✅ **Testing**
- **100% tests passing**: Todos los tests de `pkg/config/` funcionando
- **Ejemplo básico verificado**: `examples/basic/quickstart/` funcional

## [1.0.2] - 2025-08-20

### ✨ Added
- **Encriptación AES-256-GCM**: Implementación completa de encriptación/desencriptación en el sistema de cache
  - Métodos `encrypt()` y `decrypt()` con AES-256-GCM
  - Manejo automático de claves con padding para compatibilidad
  - Soporte para deshabilitación condicional de encriptación
  - Nonces aleatorios para máxima seguridad

### 🔧 Fixed  
- Corregidos warnings de staticcheck y linter
- Optimizada detección de patrones de type assertion (S1034)
- Mejorado manejo de datos vacíos en encriptación
- Removido código en desuso detectado por análisis estático

### 🧪 Testing
- **+100% cobertura** de tests para métodos de encriptación
- Tests comprehensivos para todos los casos edge:
  - Encriptación deshabilitada (passthrough)
  - Claves de diferentes longitudes
  - Datos corruptos/inválidos
  - Datos vacíos y de diferentes tamaños
- Verificación de integridad y seguridad en round-trips

### 🔒 Security
- Implementación segura de AES-256-GCM con nonces únicos
- Validación robusta de datos corruptos
- Manejo seguro de claves de encriptación

## [1.0.1] - 2025-01-20

### 🔧 Fixed
- Resuelto problemas de compatibilidad de interfaces y middleware stack
- Corregido imports problemáticos en el sistema de monitoreo
- Actualizada compatibilidad con versiones recientes de dependencias

### 📦 Added
- **Nuevos Paquetes Enterprise:**
  - `pkg/cache/` - Sistema de cache unificado con soporte Redis y memoria
  - `pkg/config/` - Gestión de configuración con hot-reload
  - `pkg/database/` - Gestión avanzada de conexiones PostgreSQL con fallback
  - `pkg/monitoring/` - Sistema completo de monitoreo en tiempo real con dashboards
  - `pkg/migrations/` - Gestor de migraciones de base de datos

### ⬆️ Updated  
- Dependencias actualizadas a las versiones más recientes
- Testcontainers actualizado a v0.38.0
- OpenTelemetry actualizados para mejor observabilidad
- Docker y dependencias de containerización mejoradas

### 🏗️ Infrastructure
- Mejorada compatibilidad con Go 1.24.5
- Resueltas dependencias conflictivas en go.mod
- Optimizado para mejor rendimiento en entornos containerizados

## [1.0.0] - 2025-01-20

### 🚀 Added
- **Lanzamiento inicial de GopherKit** - Librería Enterprise Go Completa
- **Arquitectura DDD completa:**
  - Domain-Driven Design patterns
  - CQRS y Event Sourcing
  - Saga orchestration
  - Repository patterns

### 🔧 Core Components
- **Cache System:** Redis + memoria con fallback automático
- **Database:** Gestión PostgreSQL con múltiples ORMs (GORM, SQLX, database/sql)
- **Middleware Stack:** Circuit breaker, rate limiting, CORS, logging, metrics
- **Security:** JWT, TOTP, middleware de seguridad, validación

### 📊 Observability
- Métricas detalladas con Prometheus
- Health checks completos
- Logging estructurado con Logrus
- Trazabilidad distribuida con OpenTelemetry

### 🧪 Testing
- Helpers para testing con PostgreSQL
- Fixtures y datos de prueba
- Suites de testing integradas
- HTTP testing utilities

### 📚 Documentation
- README completo con ejemplos de uso
- Documentación de API detallada
- Guías de instalación y configuración
- Ejemplos de implementación completos

### 🏢 Enterprise Features
- **Migración desde Club Management System:**
  - 11 microservicios analizados
  - ~15,000 líneas de código duplicado eliminadas
  - Tiempo de desarrollo reducido de 2-3 días a 2-3 horas

### 🛠️ Development Tools
- Configuraciones optimizadas para desarrollo y producción
- Soporte completo para contenedores Docker
- Integración con herramientas de CI/CD
- Configuraciones de linting y testing

---

## Tipos de cambios
- `Added` para nuevas funcionalidades
- `Changed` para cambios en funcionalidades existentes  
- `Deprecated` para funcionalidades que serán removidas
- `Removed` para funcionalidades removidas
- `Fixed` para corrección de errores
- `Security` para mejoras de seguridad