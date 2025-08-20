# GopherKit

[![Go Version](https://img.shields.io/badge/Go-1.24.5-blue.svg)](https://golang.org)
[![GoDoc](https://godoc.org/github.com/lukcba-developers/gopherkit?status.svg)](https://godoc.org/github.com/lukcba-developers/gopherkit)
[![Go Report Card](https://goreportcard.com/badge/github.com/lukcba-developers/gopherkit)](https://goreportcard.com/report/github.com/lukcba-developers/gopherkit)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## 📋 **Descripción General**

GopherKit es una librería completa para el desarrollo rápido y consistente de microservicios en Go, diseñada específicamente para sistemas empresariales y arquitecturas modernas.

**🎯 Elimina el 80% del código repetitivo** en la inicialización de microservicios y proporciona patrones consistentes para configuración, logging, seguridad, observabilidad y más.

### **✨ Migración del Proyecto Club Management System**

Esta versión de GopherKit incluye todos los componentes comunes extraídos del análisis del proyecto `club-management-system-api`:

- **11 microservicios analizados** → Patrones comunes identificados y extraídos
- **~15,000 líneas de código duplicado** → Eliminadas y centralizadas
- **Tiempo de desarrollo de nuevos servicios**: De 2-3 días → 2-3 horas

## Características Principales

### 🏗️ Arquitectura Empresarial
- **Domain-Driven Design (DDD)**: Estructuras y patrones para implementar DDD
- **CQRS y Event Sourcing**: Implementación completa con buses de comandos y consultas
- **Sagas**: Orquestación de procesos distribuidos
- **Repository Pattern**: Abstracciones para acceso a datos

### 🔧 Componentes Core

#### Cache Unificado
- Soporte para Redis y cache en memoria
- Fallback automático y alta disponibilidad
- Operaciones atómicas (SET NX, INCR)
- **Encriptación AES-256-GCM**: Protección de datos sensibles en cache
- Compresión automática para optimizar almacenamiento
- Circuit Breaker integrado para resiliencia
- Métricas detalladas y monitoreo en tiempo real

#### Base de Datos
- Gestión de conexiones PostgreSQL
- Soporte para múltiples ORMs (GORM, SQLX, database/sql)
- Pool de conexiones optimizado
- Migraciones automáticas

#### Middleware Stack
- Circuit Breaker
- Rate Limiting
- CORS
- Logging estructurado
- Métricas y monitoreo
- Recovery y manejo de errores

#### Seguridad
- Autenticación JWT
- TOTP (Time-based One-Time Password)
- Middleware de seguridad
- Validación y sanitización de datos

### 📊 Observabilidad
- Métricas detalladas
- Health checks
- Logging estructurado con Logrus
- Trazabilidad distribuida

### 🧪 Testing
- Helpers para testing con PostgreSQL
- Fixtures y datos de prueba
- Suites de testing integradas
- Utilities para HTTP testing

## Instalación

```bash
go get github.com/lukcba-developers/gopherkit
```

## Uso de la Librería

### Uso Básico del Paquete Principal

```go
package main

import (
    "fmt"
    "github.com/lukcba-developers/gopherkit"
)

func main() {
    kit := gopherkit.New("MyGopherApp")
    
    fmt.Println(kit.Greet())     // Output: Hello from MyGopherApp!
    fmt.Println(kit.GetInfo())   // Output: GopherKit: MyGopherApp (version v0.1.0)
}
```

### Usando el Sistema de Cache

```go
import (
    "context"
    "time"
    "github.com/lukcba-developers/gopherkit/pkg/cache"
    "github.com/sirupsen/logrus"
)

func main() {
    // Crear configuración del cache
    config := cache.DefaultCacheConfig()
    config.RedisHost = "localhost"
    config.EnableMemoryFallback = true

    logger := logrus.New()
    unifiedCache, err := cache.NewUnifiedCache(config, logger)
    if err != nil {
        panic(err)
    }

    ctx := context.Background()
    
    // Guardar datos en cache
    data := map[string]string{"user": "john", "role": "admin"}
    err = unifiedCache.Set(ctx, "user:123", data, time.Hour)
    
    // Obtener datos del cache
    var result map[string]string
    err = unifiedCache.Get(ctx, "user:123", &result)
    
    fmt.Printf("Cached data: %+v\n", result)
}
```

### Configurando Middleware para APIs

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/lukcba-developers/gopherkit/pkg/middleware"
    "github.com/sirupsen/logrus"
)

func main() {
    logger := logrus.New()
    router := gin.New()

    // Crear y configurar el stack de middleware
    stack := middleware.ProductionMiddlewareStack(logger, "MyAPI")
    stack.Initialize()

    // Aplicar middlewares al router
    stack.ApplyMiddlewares(router)
    stack.ApplySecurityMiddlewares(router)
    stack.ApplyHealthCheckRoutes(router)

    // Definir rutas de tu aplicación
    router.GET("/api/users", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "Users endpoint"})
    })

    router.Run(":8080")
}
```

### Gestión de Conexiones a Base de Datos

```go
import (
    "context"
    "github.com/lukcba-developers/gopherkit/pkg/database/connection"
    "github.com/sirupsen/logrus"
)

func main() {
    logger := logrus.New()
    
    // Configurar conexión PostgreSQL
    config := connection.DefaultPostgresConfig()
    config.Host = "localhost"
    config.Database = "myapp"
    config.Username = "postgres"
    config.Password = "password"

    // Crear el gestor de conexiones
    connManager := connection.NewPostgresConnectionManager(config, logger)

    ctx := context.Background()
    err := connManager.Connect(ctx)
    if err != nil {
        panic(err)
    }
    defer connManager.Close()

    // Usar diferentes drivers según necesites
    sqlDB := connManager.GetSQL()        // database/sql
    gormDB := connManager.GetGORM()      // GORM ORM
    sqlxDB := connManager.GetSQLX()      // SQLX
    
    // Ejemplo con GORM
    type User struct {
        ID   uint   `gorm:"primarykey"`
        Name string `gorm:"not null"`
    }
    
    // Ejecutar migraciones automáticas
    err = connManager.RunMigrations(&User{})
}
```

### Protección con Circuit Breaker

```go
import (
    "time"
    "github.com/lukcba-developers/gopherkit/pkg/circuitbreaker"
)

func main() {
    // Configurar circuit breaker para proteger llamadas externas
    config := circuitbreaker.DefaultCircuitBreakerConfig()
    config.Name = "external-api"
    config.FailureThreshold = 5
    config.Timeout = 30 * time.Second

    cb, err := circuitbreaker.New(config)
    if err != nil {
        panic(err)
    }

    // Proteger llamadas a servicios externos
    err = cb.Execute(func() error {
        // Tu lógica de llamada externa aquí
        return callExternalService()
    })

    if err != nil {
        // Manejar error o circuit breaker abierto
        fmt.Printf("Error or circuit breaker open: %v\n", err)
    }
}

func callExternalService() error {
    // Simular llamada a servicio externo
    return nil
}
```

### Implementando CQRS

```go
import (
    "context"
    "github.com/lukcba-developers/gopherkit/pkg/infrastructure/cqrs"
    "github.com/sirupsen/logrus"
)

// Definir comando
type CreateUserCommand struct {
    Name  string
    Email string
}

func main() {
    logger := logrus.New()
    
    // Crear bus de comandos
    commandBus := cqrs.NewCommandBus(logger)

    // Registrar handler para el comando
    commandBus.RegisterHandler("CreateUser", func(ctx context.Context, cmd interface{}) error {
        createCmd := cmd.(CreateUserCommand)
        // Tu lógica de negocio aquí
        fmt.Printf("Creating user: %s (%s)\n", createCmd.Name, createCmd.Email)
        return nil
    })

    // Despachar comando
    ctx := context.Background()
    err := commandBus.Dispatch(ctx, "CreateUser", CreateUserCommand{
        Name:  "John Doe",
        Email: "john@example.com",
    })
    
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Paquetes de la Librería

GopherKit está organizada en paquetes modulares que puedes importar según tus necesidades:

### Cache (`pkg/cache`)
Sistema de cache híbrido con soporte para Redis y memoria local.
```go
import "github.com/lukcba-developers/gopherkit/pkg/cache"
```

### Circuit Breaker (`pkg/circuitbreaker`)
Patrón Circuit Breaker para protección contra fallos en cascada.
```go
import "github.com/lukcba-developers/gopherkit/pkg/circuitbreaker"
```

### Base de Datos (`pkg/database`)
Gestión de conexiones PostgreSQL con soporte para múltiples ORMs.
```go
import "github.com/lukcba-developers/gopherkit/pkg/database/connection"
```

### Middleware (`pkg/middleware`)
Stack completo de middleware para APIs HTTP con Gin.
```go
import "github.com/lukcba-developers/gopherkit/pkg/middleware"
```

### CQRS (`pkg/infrastructure/cqrs`)
Implementación de Command Query Responsibility Segregation.
```go
import "github.com/lukcba-developers/gopherkit/pkg/infrastructure/cqrs"
```

### Errores (`pkg/errors`)
Sistema unificado de manejo de errores.
```go
import "github.com/lukcba-developers/gopherkit/pkg/errors"
```

### Validación (`pkg/validation`)
Validación de datos con reglas personalizables.
```go
import "github.com/lukcba-developers/gopherkit/pkg/validation"
```

### Health Checks (`pkg/health`)
Sistema de health checks para monitoreo.
```go
import "github.com/lukcba-developers/gopherkit/pkg/health"
```

### Testing (`pkg/testing`)
Utilidades para facilitar el testing.
```go
import "github.com/lukcba-developers/gopherkit/pkg/testing/suite"
```

## Configuración de la Librería

### Configuración de Cache

```go
config := cache.DefaultCacheConfig()
config.RedisHost = "localhost"
config.RedisPort = "6379"
config.EnableMemoryFallback = true
config.DefaultExpiration = time.Hour
```

### Configuración de Base de Datos

```go
config := connection.DefaultPostgresConfig()
config.Host = "localhost"
config.Port = 5432
config.Database = "myapp"
config.Username = "postgres"
config.Password = "password"
config.MaxOpenConns = 25
```

### Configuración de Middleware

```go
// Para desarrollo
stack := middleware.DevelopmentMiddlewareStack(logger, "MyAPI")

// Para producción
stack := middleware.ProductionMiddlewareStack(logger, "MyAPI")

// Personalizado
stack := middleware.DefaultMiddlewareStack(logger, "MyAPI")
stack.EnableMetrics = true
stack.EnableRateLimit = true
```

## Guías de Uso

### Manejo de Errores

```go
import "github.com/lukcba-developers/gopherkit/pkg/errors"

// Error de dominio
err := errors.NewDomainError("USER_NOT_FOUND", "Usuario no encontrado", nil)

// Error HTTP
httpErr := errors.NewHTTPError(404, "Not Found", "USER_NOT_FOUND")
```

### Validación de Estructuras

```go
import "github.com/lukcba-developers/gopherkit/pkg/validation"

type User struct {
    Name  string `validate:"required,min=3,max=100"`
    Email string `validate:"required,email"`
    Age   int    `validate:"required,min=18"`
}

validator := validation.NewValidator()
err := validator.Validate(user)
```

### Testing con GopherKit

```go
import (
    "testing"
    "github.com/lukcba-developers/gopherkit/pkg/testing/suite"
    "github.com/stretchr/testify/assert"
)

type MyTestSuite struct {
    suite.BaseSuite
}

func (s *MyTestSuite) TestSomething() {
    assert.True(s.T(), true)
}

func TestMySuite(t *testing.T) {
    suite.Run(t, new(MyTestSuite))
}
```

## Ejemplo Completo

### API con Todos los Componentes

```go
package main

import (
    "context"
    "github.com/gin-gonic/gin"
    "github.com/sirupsen/logrus"
    
    "github.com/lukcba-developers/gopherkit"
    "github.com/lukcba-developers/gopherkit/pkg/cache"
    "github.com/lukcba-developers/gopherkit/pkg/database/connection"
    "github.com/lukcba-developers/gopherkit/pkg/middleware"
    "github.com/lukcba-developers/gopherkit/pkg/circuitbreaker"
)

func main() {
    // Inicializar GopherKit
    kit := gopherkit.New("MyAPI")
    logger := logrus.New()
    
    // Configurar cache
    cacheConfig := cache.DefaultCacheConfig()
    cache, _ := cache.NewUnifiedCache(cacheConfig, logger)
    
    // Configurar base de datos
    dbConfig := connection.DefaultPostgresConfig()
    dbManager := connection.NewPostgresConnectionManager(dbConfig, logger)
    dbManager.Connect(context.Background())
    defer dbManager.Close()
    
    // Configurar circuit breaker
    cbConfig := circuitbreaker.DefaultCircuitBreakerConfig()
    cb, _ := circuitbreaker.New(cbConfig)
    
    // Configurar router con middleware
    router := gin.New()
    stack := middleware.ProductionMiddlewareStack(logger, kit.Name)
    stack.Initialize()
    stack.ApplyMiddlewares(router)
    
    // Definir rutas
    router.GET("/health", func(c *gin.Context) {
        c.JSON(200, gin.H{"status": "ok", "service": kit.GetInfo()})
    })
    
    router.GET("/api/users", func(c *gin.Context) {
        // Usar circuit breaker para proteger la operación
        err := cb.Execute(func() error {
            // Tu lógica de negocio aquí
            return nil
        })
        
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(200, gin.H{"users": []string{"user1", "user2"}})
    })
    
    logger.Info("Starting server on :8080")
    router.Run(":8080")
}
```

## Documentación Adicional

- **[API Reference](docs/API.md)**: Documentación detallada de todas las interfaces
- **[Architecture](docs/ARCHITECTURE.md)**: Patrones arquitectónicos y diseño  
- **[Examples](docs/EXAMPLES.md)**: Ejemplos completos de implementación

## Ejecutar Tests de la Librería

```bash
# Ejecutar todos los tests
go test ./...

# Ejecutar tests con coverage
go test -cover ./...

# Ejecutar benchmarks
go test -bench=. ./...
```

## Contribuir a GopherKit

¡Las contribuciones son bienvenidas! Por favor:

1. Fork el repositorio
2. Crea una rama para tu feature (`git checkout -b feature/nueva-funcionalidad`)
3. Escribe tests para tu código
4. Asegúrate de seguir las convenciones de Go (`gofmt`, `golint`)
5. Commit tus cambios (`git commit -m 'Agregar nueva funcionalidad'`)
6. Push a la rama (`git push origin feature/nueva-funcionalidad`)
7. Abre un Pull Request

## Licencia

Este proyecto está licenciado bajo la Licencia MIT - ver el archivo [LICENSE](LICENSE) para más detalles.

## Autores

- **LUKCBA Developers** - [lukcba-developers](https://github.com/lukcba-developers)

---

¿Tienes preguntas o necesitas ayuda? Abre un [issue](https://github.com/lukcba-developers/gopherkit/issues) en GitHub.