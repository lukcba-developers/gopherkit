# API Reference - GopherKit

## Índice

1. [Paquete Principal](#paquete-principal)
2. [Cache](#cache)
3. [Circuit Breaker](#circuit-breaker)
4. [Base de Datos](#base-de-datos)
5. [Middleware](#middleware)
6. [Errores](#errores)
7. [CQRS](#cqrs)
8. [Validación](#validación)

---

## Paquete Principal

### `gopherkit`

#### Tipos

##### `Kit`

```go
type Kit struct {
    Name    string
    Version string
}
```

Estructura principal de GopherKit que encapsula la información básica del kit.

**Campos:**
- `Name`: Nombre de la aplicación
- `Version`: Versión actual del kit

#### Funciones

##### `New`

```go
func New(name string) *Kit
```

Crea una nueva instancia de Kit.

**Parámetros:**
- `name` (string): Nombre de la aplicación

**Retorna:**
- `*Kit`: Nueva instancia de Kit con versión v0.1.0

**Ejemplo:**
```go
kit := gopherkit.New("MyApp")
```

##### `Greet`

```go
func (k *Kit) Greet() string
```

Retorna un saludo personalizado.

**Retorna:**
- `string`: Mensaje de saludo

**Ejemplo:**
```go
fmt.Println(kit.Greet()) // "Hello from MyApp!"
```

##### `GetInfo`

```go
func (k *Kit) GetInfo() string
```

Obtiene información del kit.

**Retorna:**
- `string`: Información del kit con nombre y versión

---

## Cache

### `cache.CacheInterface`

```go
type CacheInterface interface {
    // Operaciones básicas
    Get(ctx context.Context, key string, dest interface{}) error
    Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) bool
    
    // Operaciones avanzadas
    GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error)
    SetMultiple(ctx context.Context, items map[string]interface{}, expiration time.Duration) error
    InvalidatePattern(ctx context.Context, pattern string) error
    
    // Operaciones atómicas
    SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
    Increment(ctx context.Context, key string, delta int64) (int64, error)
    
    // Operaciones de información
    GetTTL(ctx context.Context, key string) time.Duration
    Ping(ctx context.Context) error
    GetStats(ctx context.Context) (map[string]interface{}, error)
}
```

Interfaz unificada para operaciones de cache.

### `cache.CacheConfig`

```go
type CacheConfig struct {
    // Configuración Redis
    RedisHost           string
    RedisPort           string
    RedisPassword       string
    RedisDB             int
    
    // Pool de conexiones
    PoolSize            int
    MinIdleConns        int
    MaxConnAge          time.Duration
    PoolTimeout         time.Duration
    IdleTimeout         time.Duration
    
    // Timeouts
    ReadTimeout         time.Duration
    WriteTimeout        time.Duration
    DialTimeout         time.Duration
    
    // Comportamiento del Cache
    DefaultExpiration   time.Duration
    KeyPrefix           string
    EnableCompression   bool
    EnableMetrics       bool
    
    // Configuración de Fallback
    EnableMemoryFallback bool
    MemoryCacheSize      int
}
```

### Funciones

#### `DefaultCacheConfig`

```go
func DefaultCacheConfig() *CacheConfig
```

Retorna configuración por defecto del cache.

**Valores por defecto:**
- RedisHost: "localhost"
- RedisPort: "6379"
- PoolSize: 10
- DefaultExpiration: 1 hora
- EnableMemoryFallback: true

#### `NewUnifiedCache`

```go
func NewUnifiedCache(config *CacheConfig, logger *logrus.Logger) (*UnifiedCache, error)
```

Crea una nueva instancia de cache unificado.

**Parámetros:**
- `config`: Configuración del cache
- `logger`: Logger para registrar eventos

**Retorna:**
- `*UnifiedCache`: Nueva instancia de cache
- `error`: Error si falla la inicialización

---

## Circuit Breaker

### `circuitbreaker.CircuitBreaker`

```go
type CircuitBreaker struct {
    // campos privados
}
```

Implementación thread-safe de circuit breaker.

### `circuitbreaker.Config`

```go
type Config struct {
    Name                    string
    MaxRequests             uint32
    Interval                time.Duration
    Timeout                 time.Duration
    FailureThreshold        uint32
    FailureRatio            float64
    MinimumRequestThreshold uint32
    ReadyToTrip             func(counts Counts) bool
    OnStateChange           func(name string, from State, to State)
    IsSuccessful            func(err error) bool
}
```

### Estados

```go
type State int

const (
    StateClosed State = iota
    StateOpen
    StateHalfOpen
)
```

### Métodos Principales

#### `Execute`

```go
func (cb *CircuitBreaker) Execute(fn func() error) error
```

Ejecuta una función con protección del circuit breaker.

**Parámetros:**
- `fn`: Función a ejecutar

**Retorna:**
- `error`: Error de la función o error del circuit breaker

#### `ExecuteWithContext`

```go
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error
```

Ejecuta una función con contexto.

#### `State`

```go
func (cb *CircuitBreaker) State() State
```

Retorna el estado actual del circuit breaker.

#### `Reset`

```go
func (cb *CircuitBreaker) Reset()
```

Reinicia el circuit breaker al estado cerrado.

---

## Base de Datos

### `connection.PostgresConfig`

```go
type PostgresConfig struct {
    Host                string
    Port                int
    Database            string
    Username            string
    Password            string
    SSLMode             string
    TimeZone            string
    
    // Pool de conexiones
    MaxOpenConns        int
    MaxIdleConns        int
    ConnMaxLifetime     time.Duration
    ConnMaxIdleTime     time.Duration
    
    // Timeouts
    ConnectTimeout      time.Duration
    StatementTimeout    time.Duration
    
    // Logging
    LogLevel            logger.LogLevel
    SlowThreshold       time.Duration
    
    // Configuración avanzada
    ApplicationName     string
    SearchPath          string
    PgBouncerMode       bool
}
```

### `connection.PostgresConnectionManager`

Manager para gestionar conexiones PostgreSQL con múltiples drivers.

#### Métodos

##### `Connect`

```go
func (m *PostgresConnectionManager) Connect(ctx context.Context) error
```

Establece todas las conexiones de base de datos.

##### `GetSQL`

```go
func (m *PostgresConnectionManager) GetSQL() *sql.DB
```

Retorna la conexión database/sql.

##### `GetGORM`

```go
func (m *PostgresConnectionManager) GetGORM() *gorm.DB
```

Retorna la conexión GORM.

##### `GetSQLX`

```go
func (m *PostgresConnectionManager) GetSQLX() *sqlx.DB
```

Retorna la conexión SQLX.

##### `RunMigrations`

```go
func (m *PostgresConnectionManager) RunMigrations(models ...interface{}) error
```

Ejecuta migraciones usando GORM.

##### `ExecuteInTransaction`

```go
func (m *PostgresConnectionManager) ExecuteInTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
```

Ejecuta una función dentro de una transacción.

---

## Middleware

### `middleware.MiddlewareStack`

```go
type MiddlewareStack struct {
    // Componentes core
    Logger          *logrus.Logger
    RedisClient     *redis.Client
    
    // Configuraciones
    CORSConfig           *cors.Config
    LoggingConfig        *logging.LoggingConfig
    MetricsConfig        *metrics.MetricsConfig
    RateLimitConfig      *ratelimit.RateLimitConfig
    CircuitBreakerConfig *circuitbreaker.CircuitBreakerConfig
    RecoveryConfig       *recovery.RecoveryConfig
    
    // Componentes
    MetricsCollector     metrics.MetricsCollector
    CircuitBreakerMgr    *circuitbreaker.CircuitBreakerManager
    RecoveryStatsCollector *recovery.RecoveryStatsCollector
    
    // Configuración
    EnableCORS           bool
    EnableLogging        bool
    EnableMetrics        bool
    EnableRateLimit      bool
    EnableCircuitBreaker bool
    EnableRecovery       bool
    EnableTenantContext  bool
    
    // Entorno
    Environment          string
    ServiceName          string
    ServiceVersion       string
}
```

#### Métodos Principales

##### `Initialize`

```go
func (ms *MiddlewareStack) Initialize() error
```

Inicializa todos los componentes del stack.

##### `ApplyMiddlewares`

```go
func (ms *MiddlewareStack) ApplyMiddlewares(router *gin.Engine)
```

Aplica todos los middlewares al router en el orden correcto.

##### `ApplySecurityMiddlewares`

```go
func (ms *MiddlewareStack) ApplySecurityMiddlewares(router *gin.Engine)
```

Aplica middlewares de seguridad adicionales.

##### `ApplyHealthCheckRoutes`

```go
func (ms *MiddlewareStack) ApplyHealthCheckRoutes(router *gin.Engine)
```

Añade rutas de health check al router.

### Funciones de Configuración

#### `DefaultMiddlewareStack`

```go
func DefaultMiddlewareStack(logger *logrus.Logger, serviceName string) *MiddlewareStack
```

Retorna configuración por defecto.

#### `DevelopmentMiddlewareStack`

```go
func DevelopmentMiddlewareStack(logger *logrus.Logger, serviceName string) *MiddlewareStack
```

Configuración optimizada para desarrollo.

#### `ProductionMiddlewareStack`

```go
func ProductionMiddlewareStack(logger *logrus.Logger, serviceName string) *MiddlewareStack
```

Configuración optimizada para producción.

---

## Errores

### `errors.DomainError`

```go
type DomainError struct {
    Code    string
    Message string
    Details map[string]interface{}
}
```

Error de dominio con información estructurada.

### `errors.HTTPError`

```go
type HTTPError struct {
    StatusCode int
    Message    string
    Code       string
}
```

Error HTTP con código de estado.

### Constructores

#### `NewDomainError`

```go
func NewDomainError(code, message string, details map[string]interface{}) *DomainError
```

Crea un nuevo error de dominio.

#### `NewHTTPError`

```go
func NewHTTPError(statusCode int, message, code string) *HTTPError
```

Crea un nuevo error HTTP.

---

## CQRS

### `cqrs.CommandBus`

```go
type CommandBus struct {
    // campos privados
}
```

Bus de comandos para implementación CQRS.

#### Métodos

##### `RegisterHandler`

```go
func (cb *CommandBus) RegisterHandler(commandType string, handler CommandHandler)
```

Registra un handler para un tipo de comando.

##### `Dispatch`

```go
func (cb *CommandBus) Dispatch(ctx context.Context, commandType string, command interface{}) error
```

Despacha un comando al handler correspondiente.

### `cqrs.QueryBus`

```go
type QueryBus struct {
    // campos privados
}
```

Bus de consultas para implementación CQRS.

#### Métodos similares a CommandBus

---

## Validación

### `validation.Validator`

```go
type Validator struct {
    // campos privados
}
```

Validador mejorado con reglas personalizadas.

#### Métodos

##### `Validate`

```go
func (v *Validator) Validate(data interface{}) error
```

Valida una estructura según sus tags.

##### `RegisterCustomValidation`

```go
func (v *Validator) RegisterCustomValidation(tag string, fn validator.Func) error
```

Registra una validación personalizada.

### Tags de Validación Soportados

- `required`: Campo requerido
- `email`: Formato de email válido
- `min`: Valor o longitud mínima
- `max`: Valor o longitud máxima
- `len`: Longitud exacta
- `url`: URL válida
- `uuid`: UUID válido

### Ejemplo de Uso

```go
type User struct {
    Name     string `validate:"required,min=3,max=100"`
    Email    string `validate:"required,email"`
    Age      int    `validate:"required,min=18,max=120"`
    Website  string `validate:"omitempty,url"`
}

validator := validation.NewValidator()
err := validator.Validate(user)
```