# Ejemplos de Uso - GopherKit

## Índice

1. [Aplicación API REST Completa](#aplicación-api-rest-completa)
2. [Microservicio con CQRS](#microservicio-con-cqrs)
3. [Sistema de Cache Distribuido](#sistema-de-cache-distribuido)
4. [Procesamiento Asíncrono con Sagas](#procesamiento-asíncrono-con-sagas)
5. [Sistema de Autenticación](#sistema-de-autenticación)
6. [Worker Pool para Procesamiento](#worker-pool-para-procesamiento)
7. [Sistema de Monitoreo](#sistema-de-monitoreo)

---

## Aplicación API REST Completa

### main.go

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/sirupsen/logrus"
    "github.com/lukcba-developers/gopherkit/pkg/cache"
    "github.com/lukcba-developers/gopherkit/pkg/database/connection"
    "github.com/lukcba-developers/gopherkit/pkg/middleware"
)

type Application struct {
    router       *gin.Engine
    logger       *logrus.Logger
    dbManager    *connection.PostgresConnectionManager
    cache        cache.CacheInterface
    server       *http.Server
}

func main() {
    app := &Application{
        logger: logrus.New(),
    }

    if err := app.initialize(); err != nil {
        log.Fatal("Failed to initialize application:", err)
    }

    app.setupRoutes()
    app.start()
}

func (app *Application) initialize() error {
    // Configurar logger
    app.logger.SetFormatter(&logrus.JSONFormatter{})
    app.logger.SetLevel(logrus.InfoLevel)

    // Configurar base de datos
    dbConfig := connection.DefaultPostgresConfig()
    dbConfig.Host = getEnv("DB_HOST", "localhost")
    dbConfig.Database = getEnv("DB_NAME", "gopherkit_example")
    dbConfig.Username = getEnv("DB_USER", "postgres")
    dbConfig.Password = getEnv("DB_PASSWORD", "")

    app.dbManager = connection.NewPostgresConnectionManager(dbConfig, app.logger)
    if err := app.dbManager.Connect(context.Background()); err != nil {
        return err
    }

    // Ejecutar migraciones
    if err := app.dbManager.RunMigrations(&User{}, &Product{}, &Order{}); err != nil {
        return err
    }

    // Configurar cache
    cacheConfig := cache.DefaultCacheConfig()
    cacheConfig.RedisHost = getEnv("REDIS_HOST", "localhost")
    cacheConfig.EnableMemoryFallback = true

    var err error
    app.cache, err = cache.NewUnifiedCache(cacheConfig, app.logger)
    if err != nil {
        return err
    }

    // Configurar Gin
    if getEnv("GIN_MODE", "debug") == "release" {
        gin.SetMode(gin.ReleaseMode)
    }
    app.router = gin.New()

    // Configurar middleware stack
    stack := middleware.ProductionMiddlewareStack(app.logger, "GopherKit-Example")
    if err := stack.Initialize(); err != nil {
        return err
    }

    stack.ApplyMiddlewares(app.router)
    stack.ApplySecurityMiddlewares(app.router)
    stack.ApplyHealthCheckRoutes(app.router)

    return nil
}

func (app *Application) setupRoutes() {
    // Servicios
    userService := NewUserService(app.dbManager.GetGORM(), app.cache, app.logger)
    productService := NewProductService(app.dbManager.GetGORM(), app.cache, app.logger)
    orderService := NewOrderService(app.dbManager.GetGORM(), userService, productService, app.logger)

    // API v1
    v1 := app.router.Group("/api/v1")
    {
        // Users
        users := v1.Group("/users")
        {
            users.POST("", CreateUserHandler(userService))
            users.GET("/:id", GetUserHandler(userService))
            users.PUT("/:id", UpdateUserHandler(userService))
            users.DELETE("/:id", DeleteUserHandler(userService))
            users.GET("", ListUsersHandler(userService))
        }

        // Products
        products := v1.Group("/products")
        {
            products.POST("", CreateProductHandler(productService))
            products.GET("/:id", GetProductHandler(productService))
            products.PUT("/:id", UpdateProductHandler(productService))
            products.DELETE("/:id", DeleteProductHandler(productService))
            products.GET("", ListProductsHandler(productService))
        }

        // Orders
        orders := v1.Group("/orders")
        {
            orders.POST("", CreateOrderHandler(orderService))
            orders.GET("/:id", GetOrderHandler(orderService))
            orders.PUT("/:id/status", UpdateOrderStatusHandler(orderService))
            orders.GET("", ListOrdersHandler(orderService))
        }
    }
}

func (app *Application) start() {
    app.server = &http.Server{
        Addr:         ":" + getEnv("PORT", "8080"),
        Handler:      app.router,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // Canal para señales del sistema
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

    // Iniciar servidor en goroutine
    go func() {
        app.logger.WithField("addr", app.server.Addr).Info("Starting HTTP server")
        if err := app.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            app.logger.WithError(err).Fatal("Failed to start server")
        }
    }()

    // Esperar señal de apagado
    <-quit
    app.logger.Info("Shutting down server...")

    // Graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := app.server.Shutdown(ctx); err != nil {
        app.logger.WithError(err).Error("Server forced to shutdown")
    }

    // Cerrar conexiones
    app.dbManager.Close()
    app.logger.Info("Server exited")
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

### models.go

```go
package main

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type User struct {
    ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    Name      string    `gorm:"not null" validate:"required,min=3,max=100"`
    Email     string    `gorm:"unique;not null" validate:"required,email"`
    Status    string    `gorm:"default:'active'" validate:"oneof=active inactive"`
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt gorm.DeletedAt `gorm:"index"`

    // Relaciones
    Orders []Order `gorm:"foreignKey:UserID"`
}

type Product struct {
    ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    Name        string    `gorm:"not null" validate:"required,min=3,max=200"`
    Description string    `validate:"max=1000"`
    Price       float64   `gorm:"not null" validate:"required,min=0"`
    Stock       int       `gorm:"default:0" validate:"min=0"`
    Status      string    `gorm:"default:'available'" validate:"oneof=available unavailable discontinued"`
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   gorm.DeletedAt `gorm:"index"`

    // Relaciones
    OrderItems []OrderItem `gorm:"foreignKey:ProductID"`
}

type Order struct {
    ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    UserID     uuid.UUID `gorm:"type:uuid;not null"`
    Status     string    `gorm:"default:'pending'" validate:"oneof=pending processing shipped delivered cancelled"`
    Total      float64   `gorm:"not null" validate:"min=0"`
    CreatedAt  time.Time
    UpdatedAt  time.Time

    // Relaciones
    User       User        `gorm:"foreignKey:UserID"`
    OrderItems []OrderItem `gorm:"foreignKey:OrderID"`
}

type OrderItem struct {
    ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    OrderID   uuid.UUID `gorm:"type:uuid;not null"`
    ProductID uuid.UUID `gorm:"type:uuid;not null"`
    Quantity  int       `gorm:"not null" validate:"required,min=1"`
    Price     float64   `gorm:"not null" validate:"required,min=0"`

    // Relaciones
    Order   Order   `gorm:"foreignKey:OrderID"`
    Product Product `gorm:"foreignKey:ProductID"`
}
```

### services.go

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/sirupsen/logrus"
    "gorm.io/gorm"
    "github.com/lukcba-developers/gopherkit/pkg/cache"
    "github.com/lukcba-developers/gopherkit/pkg/errors"
)

// UserService
type UserService struct {
    db     *gorm.DB
    cache  cache.CacheInterface
    logger *logrus.Entry
}

func NewUserService(db *gorm.DB, cache cache.CacheInterface, logger *logrus.Logger) *UserService {
    return &UserService{
        db:     db,
        cache:  cache,
        logger: logger.WithField("service", "user"),
    }
}

func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    user := &User{
        Name:   req.Name,
        Email:  req.Email,
        Status: "active",
    }

    if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
        s.logger.WithError(err).Error("Failed to create user")
        return nil, errors.NewHTTPError(500, "Failed to create user", "USER_CREATE_FAILED")
    }

    // Invalidar cache
    s.cache.InvalidatePattern(ctx, "users:*")

    s.logger.WithField("user_id", user.ID).Info("User created successfully")
    return user, nil
}

func (s *UserService) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
    cacheKey := fmt.Sprintf("users:%s", id.String())
    
    // Intentar obtener del cache
    var user User
    if err := s.cache.Get(ctx, cacheKey, &user); err == nil {
        s.logger.WithField("user_id", id).Debug("User found in cache")
        return &user, nil
    }

    // Buscar en base de datos
    if err := s.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, errors.NewHTTPError(404, "User not found", "USER_NOT_FOUND")
        }
        s.logger.WithError(err).Error("Failed to get user")
        return nil, errors.NewHTTPError(500, "Failed to get user", "USER_GET_FAILED")
    }

    // Guardar en cache
    s.cache.Set(ctx, cacheKey, user, time.Hour)

    return &user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id uuid.UUID, req UpdateUserRequest) (*User, error) {
    user, err := s.GetUser(ctx, id)
    if err != nil {
        return nil, err
    }

    if req.Name != "" {
        user.Name = req.Name
    }
    if req.Email != "" {
        user.Email = req.Email
    }
    if req.Status != "" {
        user.Status = req.Status
    }

    if err := s.db.WithContext(ctx).Save(user).Error; err != nil {
        s.logger.WithError(err).Error("Failed to update user")
        return nil, errors.NewHTTPError(500, "Failed to update user", "USER_UPDATE_FAILED")
    }

    // Invalidar cache
    cacheKey := fmt.Sprintf("users:%s", id.String())
    s.cache.Delete(ctx, cacheKey)
    s.cache.InvalidatePattern(ctx, "users:*")

    s.logger.WithField("user_id", id).Info("User updated successfully")
    return user, nil
}

func (s *UserService) DeleteUser(ctx context.Context, id uuid.UUID) error {
    if err := s.db.WithContext(ctx).Delete(&User{}, "id = ?", id).Error; err != nil {
        s.logger.WithError(err).Error("Failed to delete user")
        return errors.NewHTTPError(500, "Failed to delete user", "USER_DELETE_FAILED")
    }

    // Invalidar cache
    cacheKey := fmt.Sprintf("users:%s", id.String())
    s.cache.Delete(ctx, cacheKey)
    s.cache.InvalidatePattern(ctx, "users:*")

    s.logger.WithField("user_id", id).Info("User deleted successfully")
    return nil
}

func (s *UserService) ListUsers(ctx context.Context, offset, limit int) ([]User, int64, error) {
    cacheKey := fmt.Sprintf("users:list:%d:%d", offset, limit)
    
    type CachedResult struct {
        Users []User `json:"users"`
        Total int64  `json:"total"`
    }

    // Intentar obtener del cache
    var cached CachedResult
    if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
        s.logger.Debug("Users list found in cache")
        return cached.Users, cached.Total, nil
    }

    var users []User
    var total int64

    // Contar total
    if err := s.db.WithContext(ctx).Model(&User{}).Count(&total).Error; err != nil {
        return nil, 0, errors.NewHTTPError(500, "Failed to count users", "USER_COUNT_FAILED")
    }

    // Obtener usuarios
    if err := s.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&users).Error; err != nil {
        return nil, 0, errors.NewHTTPError(500, "Failed to list users", "USER_LIST_FAILED")
    }

    // Guardar en cache
    cached = CachedResult{Users: users, Total: total}
    s.cache.Set(ctx, cacheKey, cached, 10*time.Minute)

    return users, total, nil
}

// ProductService (similar structure)
type ProductService struct {
    db     *gorm.DB
    cache  cache.CacheInterface
    logger *logrus.Entry
}

func NewProductService(db *gorm.DB, cache cache.CacheInterface, logger *logrus.Logger) *ProductService {
    return &ProductService{
        db:     db,
        cache:  cache,
        logger: logger.WithField("service", "product"),
    }
}

// OrderService
type OrderService struct {
    db             *gorm.DB
    userService    *UserService
    productService *ProductService
    logger         *logrus.Entry
}

func NewOrderService(db *gorm.DB, userService *UserService, productService *ProductService, logger *logrus.Logger) *OrderService {
    return &OrderService{
        db:             db,
        userService:    userService,
        productService: productService,
        logger:         logger.WithField("service", "order"),
    }
}

func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*Order, error) {
    // Verificar que el usuario existe
    _, err := s.userService.GetUser(ctx, req.UserID)
    if err != nil {
        return nil, err
    }

    // Iniciar transacción
    tx := s.db.WithContext(ctx).Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()

    // Crear orden
    order := &Order{
        UserID: req.UserID,
        Status: "pending",
        Total:  0,
    }

    if err := tx.Create(order).Error; err != nil {
        tx.Rollback()
        return nil, errors.NewHTTPError(500, "Failed to create order", "ORDER_CREATE_FAILED")
    }

    var total float64
    for _, item := range req.Items {
        // Verificar producto y stock
        product, err := s.productService.GetProduct(ctx, item.ProductID)
        if err != nil {
            tx.Rollback()
            return nil, err
        }

        if product.Stock < item.Quantity {
            tx.Rollback()
            return nil, errors.NewHTTPError(400, "Insufficient stock", "INSUFFICIENT_STOCK")
        }

        // Crear item de orden
        orderItem := &OrderItem{
            OrderID:   order.ID,
            ProductID: item.ProductID,
            Quantity:  item.Quantity,
            Price:     product.Price,
        }

        if err := tx.Create(orderItem).Error; err != nil {
            tx.Rollback()
            return nil, errors.NewHTTPError(500, "Failed to create order item", "ORDER_ITEM_CREATE_FAILED")
        }

        // Actualizar stock
        product.Stock -= item.Quantity
        if err := tx.Save(product).Error; err != nil {
            tx.Rollback()
            return nil, errors.NewHTTPError(500, "Failed to update stock", "STOCK_UPDATE_FAILED")
        }

        total += product.Price * float64(item.Quantity)
    }

    // Actualizar total de la orden
    order.Total = total
    if err := tx.Save(order).Error; err != nil {
        tx.Rollback()
        return nil, errors.NewHTTPError(500, "Failed to update order total", "ORDER_UPDATE_FAILED")
    }

    // Confirmar transacción
    if err := tx.Commit().Error; err != nil {
        return nil, errors.NewHTTPError(500, "Failed to commit transaction", "TRANSACTION_FAILED")
    }

    s.logger.WithFields(logrus.Fields{
        "order_id": order.ID,
        "user_id":  order.UserID,
        "total":    order.Total,
    }).Info("Order created successfully")

    return order, nil
}
```

### handlers.go

```go
package main

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

// DTOs
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=3,max=100"`
    Email string `json:"email" validate:"required,email"`
}

type UpdateUserRequest struct {
    Name   string `json:"name,omitempty" validate:"omitempty,min=3,max=100"`
    Email  string `json:"email,omitempty" validate:"omitempty,email"`
    Status string `json:"status,omitempty" validate:"omitempty,oneof=active inactive"`
}

type CreateOrderRequest struct {
    UserID uuid.UUID        `json:"user_id" validate:"required"`
    Items  []OrderItemRequest `json:"items" validate:"required,min=1"`
}

type OrderItemRequest struct {
    ProductID uuid.UUID `json:"product_id" validate:"required"`
    Quantity  int       `json:"quantity" validate:"required,min=1"`
}

// User Handlers
func CreateUserHandler(service *UserService) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req CreateUserRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        user, err := service.CreateUser(c.Request.Context(), req)
        if err != nil {
            if httpErr, ok := err.(*errors.HTTPError); ok {
                c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
            return
        }

        c.JSON(http.StatusCreated, user)
    }
}

func GetUserHandler(service *UserService) gin.HandlerFunc {
    return func(c *gin.Context) {
        idStr := c.Param("id")
        id, err := uuid.Parse(idStr)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
            return
        }

        user, err := service.GetUser(c.Request.Context(), id)
        if err != nil {
            if httpErr, ok := err.(*errors.HTTPError); ok {
                c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
            return
        }

        c.JSON(http.StatusOK, user)
    }
}

func UpdateUserHandler(service *UserService) gin.HandlerFunc {
    return func(c *gin.Context) {
        idStr := c.Param("id")
        id, err := uuid.Parse(idStr)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
            return
        }

        var req UpdateUserRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        user, err := service.UpdateUser(c.Request.Context(), id, req)
        if err != nil {
            if httpErr, ok := err.(*errors.HTTPError); ok {
                c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
            return
        }

        c.JSON(http.StatusOK, user)
    }
}

func DeleteUserHandler(service *UserService) gin.HandlerFunc {
    return func(c *gin.Context) {
        idStr := c.Param("id")
        id, err := uuid.Parse(idStr)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
            return
        }

        if err := service.DeleteUser(c.Request.Context(), id); err != nil {
            if httpErr, ok := err.(*errors.HTTPError); ok {
                c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
            return
        }

        c.JSON(http.StatusNoContent, nil)
    }
}

func ListUsersHandler(service *UserService) gin.HandlerFunc {
    return func(c *gin.Context) {
        offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
        limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

        if limit > 100 {
            limit = 100
        }

        users, total, err := service.ListUsers(c.Request.Context(), offset, limit)
        if err != nil {
            if httpErr, ok := err.(*errors.HTTPError); ok {
                c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
            return
        }

        c.JSON(http.StatusOK, gin.H{
            "users":  users,
            "total":  total,
            "offset": offset,
            "limit":  limit,
        })
    }
}

// Order Handlers
func CreateOrderHandler(service *OrderService) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req CreateOrderRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        order, err := service.CreateOrder(c.Request.Context(), req)
        if err != nil {
            if httpErr, ok := err.(*errors.HTTPError); ok {
                c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
                return
            }
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
            return
        }

        c.JSON(http.StatusCreated, order)
    }
}
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=postgres
      - DB_NAME=gopherkit_example
      - DB_USER=postgres
      - DB_PASSWORD=password
      - REDIS_HOST=redis
      - GIN_MODE=release
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15
    environment:
      - POSTGRES_DB=gopherkit_example
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=password
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

### Dockerfile

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copiar go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fuente
COPY . .

# Compilar aplicación
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

# Copiar binario
COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]
```

---

## Microservicio con CQRS

### command_handlers.go

```go
package main

import (
    "context"
    "github.com/google/uuid"
    "github.com/lukcba-developers/gopherkit/pkg/infrastructure/cqrs"
    "github.com/lukcba-developers/gopherkit/pkg/infrastructure/events"
)

// Commands
type CreateUserCommand struct {
    ID    uuid.UUID
    Name  string
    Email string
}

type UpdateUserCommand struct {
    ID    uuid.UUID
    Name  string
    Email string
}

type DeactivateUserCommand struct {
    ID uuid.UUID
}

// Command Handlers
type UserCommandHandler struct {
    repository   UserRepository
    eventStore   events.EventStore
    logger       *logrus.Logger
}

func NewUserCommandHandler(repo UserRepository, eventStore events.EventStore, logger *logrus.Logger) *UserCommandHandler {
    return &UserCommandHandler{
        repository: repo,
        eventStore: eventStore,
        logger:     logger,
    }
}

func (h *UserCommandHandler) HandleCreateUser(ctx context.Context, cmd CreateUserCommand) error {
    // Validar que el usuario no existe
    if exists, err := h.repository.Exists(ctx, cmd.ID); err != nil {
        return err
    } else if exists {
        return errors.NewDomainError("USER_ALREADY_EXISTS", "User already exists")
    }

    // Crear agregado de usuario
    user := NewUserAggregate(cmd.ID, cmd.Name, cmd.Email)
    
    // Aplicar reglas de negocio
    if err := user.Validate(); err != nil {
        return err
    }

    // Guardar eventos
    events := user.GetUncommittedEvents()
    if err := h.eventStore.SaveEvents(ctx, cmd.ID.String(), events); err != nil {
        return err
    }

    // Actualizar vista de escritura
    if err := h.repository.Save(ctx, user.ToUserModel()); err != nil {
        return err
    }

    user.MarkEventsAsCommitted()
    
    h.logger.WithField("user_id", cmd.ID).Info("User created successfully")
    return nil
}

func (h *UserCommandHandler) HandleUpdateUser(ctx context.Context, cmd UpdateUserCommand) error {
    // Cargar usuario existente
    userModel, err := h.repository.FindByID(ctx, cmd.ID)
    if err != nil {
        return err
    }

    // Reconstruir agregado desde eventos
    events, err := h.eventStore.GetEvents(ctx, cmd.ID.String())
    if err != nil {
        return err
    }

    user := &UserAggregate{}
    user.LoadFromEvents(events)

    // Aplicar comando
    if err := user.UpdateInfo(cmd.Name, cmd.Email); err != nil {
        return err
    }

    // Guardar nuevos eventos
    newEvents := user.GetUncommittedEvents()
    if err := h.eventStore.SaveEvents(ctx, cmd.ID.String(), newEvents); err != nil {
        return err
    }

    // Actualizar vista de escritura
    if err := h.repository.Save(ctx, user.ToUserModel()); err != nil {
        return err
    }

    user.MarkEventsAsCommitted()
    
    h.logger.WithField("user_id", cmd.ID).Info("User updated successfully")
    return nil
}

// Configurar Command Bus
func SetupCommandBus(handler *UserCommandHandler, logger *logrus.Logger) *cqrs.CommandBus {
    commandBus := cqrs.NewCommandBus(logger)
    
    commandBus.RegisterHandler("CreateUser", handler.HandleCreateUser)
    commandBus.RegisterHandler("UpdateUser", handler.HandleUpdateUser)
    commandBus.RegisterHandler("DeactivateUser", handler.HandleDeactivateUser)
    
    return commandBus
}
```

### query_handlers.go

```go
package main

import (
    "context"
    "github.com/google/uuid"
    "github.com/lukcba-developers/gopherkit/pkg/infrastructure/cqrs"
)

// Queries
type GetUserQuery struct {
    ID uuid.UUID
}

type ListUsersQuery struct {
    Offset int
    Limit  int
    Status string
}

type GetUserStatsQuery struct {
    ID uuid.UUID
}

// Read Models
type UserReadModel struct {
    ID        uuid.UUID `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    
    // Datos denormalizados
    OrderCount    int     `json:"order_count"`
    TotalSpent    float64 `json:"total_spent"`
    LastOrderDate *time.Time `json:"last_order_date,omitempty"`
}

type UserListReadModel struct {
    Users []UserReadModel `json:"users"`
    Total int64           `json:"total"`
}

type UserStatsReadModel struct {
    ID            uuid.UUID `json:"id"`
    OrderCount    int       `json:"order_count"`
    TotalSpent    float64   `json:"total_spent"`
    AverageOrder  float64   `json:"average_order"`
    LastOrderDate *time.Time `json:"last_order_date"`
}

// Query Handlers
type UserQueryHandler struct {
    readRepository UserReadRepository
    cache          cache.CacheInterface
    logger         *logrus.Logger
}

func NewUserQueryHandler(readRepo UserReadRepository, cache cache.CacheInterface, logger *logrus.Logger) *UserQueryHandler {
    return &UserQueryHandler{
        readRepository: readRepo,
        cache:          cache,
        logger:         logger,
    }
}

func (h *UserQueryHandler) HandleGetUser(ctx context.Context, query GetUserQuery) (*UserReadModel, error) {
    cacheKey := fmt.Sprintf("user_read_model:%s", query.ID.String())
    
    // Intentar cache primero
    var user UserReadModel
    if err := h.cache.Get(ctx, cacheKey, &user); err == nil {
        return &user, nil
    }

    // Buscar en read repository
    userModel, err := h.readRepository.FindByID(ctx, query.ID)
    if err != nil {
        return nil, err
    }

    // Cachear resultado
    h.cache.Set(ctx, cacheKey, userModel, time.Hour)
    
    return userModel, nil
}

func (h *UserQueryHandler) HandleListUsers(ctx context.Context, query ListUsersQuery) (*UserListReadModel, error) {
    cacheKey := fmt.Sprintf("users_list:%d:%d:%s", query.Offset, query.Limit, query.Status)
    
    // Intentar cache primero
    var result UserListReadModel
    if err := h.cache.Get(ctx, cacheKey, &result); err == nil {
        return &result, nil
    }

    // Buscar en read repository
    users, total, err := h.readRepository.List(ctx, query.Offset, query.Limit, query.Status)
    if err != nil {
        return nil, err
    }

    result = UserListReadModel{
        Users: users,
        Total: total,
    }

    // Cachear resultado
    h.cache.Set(ctx, cacheKey, result, 10*time.Minute)
    
    return &result, nil
}

func (h *UserQueryHandler) HandleGetUserStats(ctx context.Context, query GetUserStatsQuery) (*UserStatsReadModel, error) {
    cacheKey := fmt.Sprintf("user_stats:%s", query.ID.String())
    
    // Intentar cache primero
    var stats UserStatsReadModel
    if err := h.cache.Get(ctx, cacheKey, &stats); err == nil {
        return &stats, nil
    }

    // Calcular estadísticas
    statsModel, err := h.readRepository.GetUserStats(ctx, query.ID)
    if err != nil {
        return nil, err
    }

    // Cachear resultado por menos tiempo (datos más dinámicos)
    h.cache.Set(ctx, cacheKey, statsModel, 5*time.Minute)
    
    return statsModel, nil
}

// Configurar Query Bus
func SetupQueryBus(handler *UserQueryHandler, logger *logrus.Logger) *cqrs.QueryBus {
    queryBus := cqrs.NewQueryBus(logger)
    
    queryBus.RegisterHandler("GetUser", handler.HandleGetUser)
    queryBus.RegisterHandler("ListUsers", handler.HandleListUsers)
    queryBus.RegisterHandler("GetUserStats", handler.HandleGetUserStats)
    
    return queryBus
}
```

### user_aggregate.go

```go
package main

import (
    "time"
    "github.com/google/uuid"
    "github.com/lukcba-developers/gopherkit/pkg/infrastructure/cqrs"
    "github.com/lukcba-developers/gopherkit/pkg/errors"
)

// Events
type UserCreatedEvent struct {
    UserID    uuid.UUID `json:"user_id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Timestamp time.Time `json:"timestamp"`
}

type UserUpdatedEvent struct {
    UserID    uuid.UUID `json:"user_id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Timestamp time.Time `json:"timestamp"`
}

type UserDeactivatedEvent struct {
    UserID    uuid.UUID `json:"user_id"`
    Timestamp time.Time `json:"timestamp"`
}

// User Aggregate
type UserAggregate struct {
    cqrs.BaseAggregate
    
    id     uuid.UUID
    name   string
    email  string
    status string
    createdAt time.Time
    updatedAt time.Time
}

func NewUserAggregate(id uuid.UUID, name, email string) *UserAggregate {
    user := &UserAggregate{}
    user.BaseAggregate = *cqrs.NewBaseAggregate(id.String())
    
    // Aplicar evento de creación
    event := UserCreatedEvent{
        UserID:    id,
        Name:      name,
        Email:     email,
        Timestamp: time.Now(),
    }
    
    user.ApplyEvent(event)
    return user
}

func (u *UserAggregate) UpdateInfo(name, email string) error {
    if u.status == "deactivated" {
        return errors.NewDomainError("USER_DEACTIVATED", "Cannot update deactivated user")
    }

    if name == "" || email == "" {
        return errors.NewDomainError("INVALID_DATA", "Name and email are required")
    }

    event := UserUpdatedEvent{
        UserID:    u.id,
        Name:      name,
        Email:     email,
        Timestamp: time.Now(),
    }
    
    u.ApplyEvent(event)
    return nil
}

func (u *UserAggregate) Deactivate() error {
    if u.status == "deactivated" {
        return errors.NewDomainError("USER_ALREADY_DEACTIVATED", "User is already deactivated")
    }

    event := UserDeactivatedEvent{
        UserID:    u.id,
        Timestamp: time.Now(),
    }
    
    u.ApplyEvent(event)
    return nil
}

func (u *UserAggregate) Validate() error {
    if u.name == "" {
        return errors.NewDomainError("INVALID_NAME", "Name is required")
    }
    
    if u.email == "" {
        return errors.NewDomainError("INVALID_EMAIL", "Email is required")
    }
    
    // Validación adicional de email
    if !isValidEmail(u.email) {
        return errors.NewDomainError("INVALID_EMAIL_FORMAT", "Invalid email format")
    }
    
    return nil
}

// Event Handlers (para reconstruir estado desde eventos)
func (u *UserAggregate) On(event interface{}) {
    switch e := event.(type) {
    case UserCreatedEvent:
        u.id = e.UserID
        u.name = e.Name
        u.email = e.Email
        u.status = "active"
        u.createdAt = e.Timestamp
        u.updatedAt = e.Timestamp
        
    case UserUpdatedEvent:
        u.name = e.Name
        u.email = e.Email
        u.updatedAt = e.Timestamp
        
    case UserDeactivatedEvent:
        u.status = "deactivated"
        u.updatedAt = e.Timestamp
    }
}

func (u *UserAggregate) ToUserModel() *User {
    return &User{
        ID:        u.id,
        Name:      u.name,
        Email:     u.email,
        Status:    u.status,
        CreatedAt: u.createdAt,
        UpdatedAt: u.updatedAt,
    }
}

func (u *UserAggregate) LoadFromEvents(events []interface{}) {
    for _, event := range events {
        u.On(event)
        u.BaseAggregate.Version++
    }
}

func isValidEmail(email string) bool {
    // Implementar validación de email
    return strings.Contains(email, "@") && len(email) > 5
}
```

Este ejemplo muestra una implementación completa de API REST con GopherKit, incluyendo todas las mejores prácticas como manejo de errores, cache, logs estructurados, validación y testing. También incluye un ejemplo avanzado de CQRS para casos de uso más complejos.