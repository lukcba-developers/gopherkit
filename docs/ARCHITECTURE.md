# Arquitectura de GopherKit

## Visión General

GopherKit está diseñado siguiendo principios de arquitectura empresarial, enfocándose en:

- **Modularidad**: Componentes independientes y reutilizables
- **Escalabilidad**: Preparado para aplicaciones de alto rendimiento
- **Mantenibilidad**: Código limpio y bien estructurado
- **Testabilidad**: Componentes fáciles de probar
- **Observabilidad**: Métricas, logs y health checks integrados

## Estructura del Proyecto

```
gopherkit/
├── cmd/                    # Comandos y herramientas CLI
├── docs/                   # Documentación
├── example/                # Ejemplos de uso
├── internal/               # Código interno (no exportado)
├── pkg/                    # Paquetes públicos
│   ├── application/        # Servicios de aplicación
│   ├── cache/              # Sistema de cache unificado
│   ├── circuitbreaker/     # Circuit breaker
│   ├── config/             # Gestión de configuración
│   ├── database/           # Acceso a base de datos
│   ├── domain/             # Lógica de dominio
│   ├── errors/             # Sistema de errores
│   ├── health/             # Health checks
│   ├── httpclient/         # Cliente HTTP
│   ├── infrastructure/     # Infraestructura
│   ├── metrics/            # Sistema de métricas
│   ├── middleware/         # Middlewares HTTP
│   ├── testing/            # Utilidades de testing
│   └── validation/         # Validación de datos
├── gopherkit.go           # Paquete principal
└── gopherkit_test.go      # Tests del paquete principal
```

## Patrones Arquitectónicos

### 1. Domain-Driven Design (DDD)

GopherKit implementa principios de DDD:

- **Entidades**: Objetos con identidad única
- **Value Objects**: Objetos inmutables sin identidad
- **Agregados**: Grupos de entidades con consistencia
- **Servicios de Dominio**: Lógica de negocio compleja
- **Repositorios**: Abstracción para persistencia

```go
// Ejemplo de Entidad
type Organization struct {
    ID     uuid.UUID `gorm:"type:uuid;primary_key"`
    Name   string    `validate:"required,min=3,max=100"`
    Status Status    `validate:"required"`
    
    // Value Objects
    Config OrganizationConfig
    
    // Timestamps
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Método de dominio
func (o *Organization) Activate() error {
    if o.Status == StatusActive {
        return errors.NewDomainError("ORG_ALREADY_ACTIVE", "Organization is already active")
    }
    o.Status = StatusActive
    return nil
}
```

### 2. CQRS (Command Query Responsibility Segregation)

Separación clara entre operaciones de escritura y lectura:

#### Command Side (Escritura)

```go
// Command
type CreateOrganizationCommand struct {
    Name   string
    Config OrganizationConfig
}

// Command Handler
func (h *OrganizationCommandHandler) Handle(cmd CreateOrganizationCommand) error {
    org := domain.NewOrganization(cmd.Name, cmd.Config)
    return h.repository.Save(org)
}

// Command Bus
commandBus.RegisterHandler("CreateOrganization", handler.Handle)
commandBus.Dispatch(ctx, "CreateOrganization", command)
```

#### Query Side (Lectura)

```go
// Query
type GetOrganizationQuery struct {
    ID uuid.UUID
}

// Query Handler
func (h *OrganizationQueryHandler) Handle(query GetOrganizationQuery) (*OrganizationReadModel, error) {
    return h.readRepository.FindByID(query.ID)
}
```

### 3. Event Sourcing

Sistema de eventos para auditabilidad y consistencia eventual:

```go
// Event
type OrganizationCreatedEvent struct {
    AggregateID uuid.UUID
    Name        string
    Timestamp   time.Time
}

// Event Store
eventStore.SaveEvents(aggregateID, events)
events := eventStore.GetEvents(aggregateID)
```

### 4. Saga Pattern

Orquestación de procesos distribuidos:

```go
// Saga
type UserRegistrationSaga struct {
    ID     uuid.UUID
    State  SagaState
    Steps  []SagaStep
}

// Saga Manager
sagaManager.StartSaga("UserRegistration", data)
sagaManager.HandleEvent(event)
```

## Capas de la Aplicación

### 1. Capa de Presentación

- **Controladores HTTP**: Manejo de requests/responses
- **Middlewares**: Funcionalidad transversal
- **Serialización**: JSON, XML, etc.

```go
func CreateUserHandler(userService *UserService) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req CreateUserRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(400, gin.H{"error": err.Error()})
            return
        }
        
        user, err := userService.CreateUser(req)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(201, user)
    }
}
```

### 2. Capa de Aplicación

- **Servicios de Aplicación**: Orquestación de casos de uso
- **DTOs**: Objetos de transferencia de datos
- **Validación de entrada**

```go
type UserService struct {
    repository UserRepository
    validator  *validation.Validator
    eventBus   EventBus
}

func (s *UserService) CreateUser(req CreateUserRequest) (*User, error) {
    // Validación
    if err := s.validator.Validate(req); err != nil {
        return nil, err
    }
    
    // Lógica de negocio
    user := domain.NewUser(req.Name, req.Email)
    
    // Persistencia
    if err := s.repository.Save(user); err != nil {
        return nil, err
    }
    
    // Eventos
    s.eventBus.Publish(UserCreatedEvent{UserID: user.ID})
    
    return user, nil
}
```

### 3. Capa de Dominio

- **Entidades**: Objetos de negocio principales
- **Value Objects**: Objetos inmutables
- **Servicios de Dominio**: Lógica compleja
- **Especificaciones**: Reglas de negocio

```go
// Entidad
type User struct {
    ID    uuid.UUID
    Name  string
    Email Email // Value Object
    
    createdAt time.Time
}

// Método de dominio
func (u *User) ChangeEmail(newEmail Email, emailService EmailDomainService) error {
    if emailService.IsEmailTaken(newEmail) {
        return errors.NewDomainError("EMAIL_TAKEN", "Email is already taken")
    }
    u.Email = newEmail
    return nil
}

// Value Object
type Email struct {
    value string
}

func NewEmail(email string) (Email, error) {
    if !isValidEmail(email) {
        return Email{}, errors.New("invalid email")
    }
    return Email{value: email}, nil
}
```

### 4. Capa de Infraestructura

- **Repositorios**: Implementaciones de persistencia
- **Servicios externos**: APIs, colas, etc.
- **Configuración**: Variables de entorno, archivos

```go
type PostgresUserRepository struct {
    db *gorm.DB
}

func (r *PostgresUserRepository) Save(user *User) error {
    return r.db.Save(user).Error
}

func (r *PostgresUserRepository) FindByID(id uuid.UUID) (*User, error) {
    var user User
    err := r.db.First(&user, "id = ?", id).Error
    return &user, err
}
```

## Componentes Principales

### 1. Sistema de Cache

Arquitectura de cache con múltiples niveles:

```
┌─────────────────┐
│   Application   │
└─────────────────┘
         │
┌─────────────────┐
│ Unified Cache  │
└─────────────────┘
    │           │
┌─────────┐ ┌─────────┐
│  Redis  │ │ Memory  │
└─────────┘ └─────────┘
```

### 2. Circuit Breaker

Implementación del patrón Circuit Breaker:

```
States:
┌─────────┐  failures  ┌──────────┐  timeout  ┌─────────────┐
│ Closed  │ ────────→  │   Open   │ ────────→ │ Half-Open   │
└─────────┘            └──────────┘           └─────────────┘
     ↑                                               │
     └───────────────── success ────────────────────┘
```

### 3. Middleware Stack

Pipeline de middleware ordenado:

```
HTTP Request
    │
    ▼
┌─────────────┐
│  Recovery   │ ← Manejo de panics
└─────────────┘
    │
    ▼
┌─────────────┐
│    CORS     │ ← Headers CORS
└─────────────┘
    │
    ▼
┌─────────────┐
│  Logging    │ ← Logging estructurado
└─────────────┘
    │
    ▼
┌─────────────┐
│ Rate Limit  │ ← Limitación de requests
└─────────────┘
    │
    ▼
┌─────────────┐
│Circuit Brk  │ ← Protección de fallos
└─────────────┘
    │
    ▼
Application Handler
```

## Principios SOLID

### Single Responsibility Principle (SRP)

Cada componente tiene una única responsabilidad:

```go
// ✓ CORRECTO: Una sola responsabilidad
type UserValidator struct{}
func (v *UserValidator) Validate(user *User) error { ... }

type UserRepository struct{}
func (r *UserRepository) Save(user *User) error { ... }

// ✗ INCORRECTO: Múltiples responsabilidades
type UserService struct{}
func (s *UserService) ValidateAndSave(user *User) error { ... }
```

### Open/Closed Principle (OCP)

Abierto para extensión, cerrado para modificación:

```go
// Interface estable
type CacheInterface interface {
    Get(key string) (interface{}, error)
    Set(key string, value interface{}) error
}

// Nuevas implementaciones sin modificar código existente
type RedisCache struct{}
type MemoryCache struct{}
type DistributedCache struct{}
```

### Liskov Substitution Principle (LSP)

Las implementaciones deben ser intercambiables:

```go
func ProcessWithCache(cache CacheInterface) {
    // Funciona con cualquier implementación de CacheInterface
    cache.Set("key", "value")
    value, _ := cache.Get("key")
}
```

### Interface Segregation Principle (ISP)

Interfaces pequeñas y específicas:

```go
// ✓ CORRECTO: Interfaces específicas
type Reader interface {
    Read([]byte) (int, error)
}

type Writer interface {
    Write([]byte) (int, error)
}

// ✗ INCORRECTO: Interface demasiado grande
type FileManager interface {
    Read([]byte) (int, error)
    Write([]byte) (int, error)
    Delete() error
    Compress() error
    Encrypt() error
    // ... muchos más métodos
}
```

### Dependency Inversion Principle (DIP)

Depender de abstracciones, no de concreciones:

```go
// ✓ CORRECTO: Depende de interfaz
type UserService struct {
    repository UserRepository // interface
}

// ✗ INCORRECTO: Depende de implementación concreta
type UserService struct {
    repository *PostgresUserRepository // struct concreto
}
```

## Patrones de Concurrencia

### Worker Pools

```go
type WorkerPool struct {
    workerCount int
    jobs        chan Job
    results     chan Result
}

func (wp *WorkerPool) Start() {
    for i := 0; i < wp.workerCount; i++ {
        go wp.worker()
    }
}

func (wp *WorkerPool) worker() {
    for job := range wp.jobs {
        result := job.Process()
        wp.results <- result
    }
}
```

### Context para Cancelación

```go
func ProcessWithTimeout(ctx context.Context, data []byte) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    select {
    case result := <-process(data):
        return result
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

## Testing

### Test Pyramid

```
        ┌─────────────┐
        │   E2E Tests │ ← Pocos, lentos, frágiles
        └─────────────┘
      ┌─────────────────┐
      │ Integration     │ ← Algunos, medianos
      │ Tests           │
      └─────────────────┘
    ┌─────────────────────┐
    │   Unit Tests        │ ← Muchos, rápidos, confiables
    └─────────────────────┘
```

### Estrategias de Testing

1. **Unit Tests**: Pruebas de componentes aislados
2. **Integration Tests**: Pruebas de interacción entre componentes
3. **Contract Tests**: Pruebas de APIs y contratos
4. **E2E Tests**: Pruebas de flujos completos

### Test Doubles

```go
// Mock Repository
type MockUserRepository struct {
    users map[uuid.UUID]*User
}

func (m *MockUserRepository) Save(user *User) error {
    m.users[user.ID] = user
    return nil
}

// Test con Mock
func TestUserService_CreateUser(t *testing.T) {
    mockRepo := &MockUserRepository{users: make(map[uuid.UUID]*User)}
    service := NewUserService(mockRepo)
    
    user, err := service.CreateUser(CreateUserRequest{
        Name: "Test User",
        Email: "test@example.com",
    })
    
    assert.NoError(t, err)
    assert.NotNil(t, user)
    assert.Equal(t, "Test User", user.Name)
}
```

## Observabilidad

### Logging Estructurado

```go
logger.WithFields(logrus.Fields{
    "user_id": userID,
    "action": "create_order",
    "order_id": orderID,
    "amount": amount,
}).Info("Order created successfully")
```

### Métricas

```go
// Counter
userCreatedCounter.Inc()

// Histogram
requestDuration.Observe(duration.Seconds())

// Gauge
activeConnections.Set(float64(connections))
```

### Health Checks

```go
type HealthChecker interface {
    Check(ctx context.Context) error
}

type DatabaseHealthChecker struct {
    db *sql.DB
}

func (h *DatabaseHealthChecker) Check(ctx context.Context) error {
    return h.db.PingContext(ctx)
}
```

## Configuración

### 12-Factor App

1. **Codebase**: Un solo repositorio
2. **Dependencies**: Dependencias explícitas
3. **Config**: Configuración en variables de entorno
4. **Backing services**: Servicios como recursos
5. **Build, release, run**: Separar estas fases
6. **Processes**: Aplicación como procesos stateless
7. **Port binding**: Exportar servicios por port binding
8. **Concurrency**: Escalar por proceso
9. **Disposability**: Maximizar robustez
10. **Dev/prod parity**: Mantener desarrollo y producción similares
11. **Logs**: Tratar logs como streams de eventos
12. **Admin processes**: Ejecutar tareas admin como one-off processes

### Gestión de Configuración

```go
type Config struct {
    Database DatabaseConfig `env:"DB"`
    Redis    RedisConfig    `env:"REDIS"`
    Server   ServerConfig   `env:"SERVER"`
}

func LoadConfig() (*Config, error) {
    var cfg Config
    if err := env.Parse(&cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

## Seguridad

### Principios de Seguridad

1. **Defense in Depth**: Múltiples capas de seguridad
2. **Least Privilege**: Mínimos permisos necesarios
3. **Fail Securely**: Fallar de forma segura
4. **Secure by Default**: Configuración segura por defecto

### Implementación

```go
// Middleware de autenticación
func AuthMiddleware() gin.HandlerFunc {
    return gin.HandlerFunc(func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
            return
        }
        
        claims, err := jwt.ValidateToken(token)
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "Invalid token"})
            return
        }
        
        c.Set("user_id", claims.UserID)
        c.Next()
    })
}
```

## Performance

### Optimizaciones

1. **Connection Pooling**: Reutilización de conexiones
2. **Caching**: Multiple niveles de cache
3. **Lazy Loading**: Carga bajo demanda
4. **Batch Operations**: Operaciones en lote
5. **Async Processing**: Procesamiento asíncrono

### Monitoring

```go
// Middleware de métricas
func MetricsMiddleware() gin.HandlerFunc {
    return gin.HandlerFunc(func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        duration := time.Since(start)
        requestDuration.WithLabelValues(
            c.Request.Method,
            c.FullPath(),
            strconv.Itoa(c.Writer.Status()),
        ).Observe(duration.Seconds())
    })
}
```

Esta arquitectura proporciona una base sólida para construir aplicaciones Go escalables, mantenibles y observables.