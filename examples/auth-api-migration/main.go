package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/server"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
)

// User model example
type User struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	Email    string `json:"email" gorm:"uniqueIndex"`
	Password string `json:"-"`
	Name     string `json:"name"`
}

func main() {
	// Load configuration
	cfg, err := config.LoadBaseConfig("auth-api")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	appLogger := logger.NewLogger("auth-api")

	// Initialize database
	dbClient, err := database.NewPostgresClient(database.PostgresOptions{
		Config: cfg.Database,
		Logger: appLogger,
		Models: []interface{}{&User{}}, // Auto-migrate models
	})
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbClient.Close()

	// Initialize cache (optional)
	var cacheClient *cache.RedisClient
	if cfg.Cache.Enabled {
		cacheClient, err = cache.NewRedisClient(cache.RedisOptions{
			Config: cfg.Cache,
			Logger: appLogger,
		})
		if err != nil {
			appLogger.LogError(context.Background(), err, "Failed to initialize cache", nil)
			// Continue without cache
		} else {
			defer cacheClient.Close()
		}
	}

	// Setup health checks
	healthChecks := []observability.HealthCheck{
		dbClient.HealthCheck(),
	}
	if cacheClient != nil {
		healthChecks = append(healthChecks, cacheClient.HealthCheck())
	}

	// Create HTTP server with gopherkit
	httpServer, err := server.NewHTTPServer(server.Options{
		Config:       cfg,
		Logger:       appLogger,
		HealthChecks: healthChecks,
		Routes:       setupRoutes(dbClient, cacheClient, appLogger),
	})
	if err != nil {
		log.Fatalf("Failed to create HTTP server: %v", err)
	}

	// Start server
	if err := httpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for shutdown signal
	if err := httpServer.WaitForShutdown(); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	appLogger.LogBusinessEvent(context.Background(), "application_shutdown", nil)
}

// setupRoutes configures API routes
func setupRoutes(db *database.PostgresClient, cache *cache.RedisClient, logger logger.Logger) func(*gin.Engine) {
	return func(router *gin.Engine) {
		// Auth routes
		authGroup := router.Group("/api/v1/auth")
		{
			authGroup.POST("/register", registerHandler(db, logger))
			authGroup.POST("/login", loginHandler(db, cache, logger))
			authGroup.POST("/logout", logoutHandler(cache, logger))
			authGroup.GET("/profile", authMiddleware(), profileHandler(db, logger))
		}

		// User routes (protected)
		userGroup := router.Group("/api/v1/users")
		userGroup.Use(authMiddleware())
		{
			userGroup.GET("", listUsersHandler(db, logger))
			userGroup.GET("/:id", getUserHandler(db, logger))
			userGroup.PUT("/:id", updateUserHandler(db, logger))
			userGroup.DELETE("/:id", deleteUserHandler(db, logger))
		}
	}
}

// Example handlers with gopherkit integration
func registerHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		var req struct {
			Email    string `json:"email" binding:"required,email"`
			Password string `json:"password" binding:"required,min=8"`
			Name     string `json:"name" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			logger.LogError(ctx, err, "invalid request body", nil)
			c.JSON(400, gin.H{"error": "Invalid request", "details": err.Error()})
			return
		}

		// Create user using gopherkit database utilities
		user := User{
			Email:    req.Email,
			Password: hashPassword(req.Password), // Implement password hashing
			Name:     req.Name,
		}

		if err := db.WithContext(ctx).Create(&user).Error; err != nil {
			logger.LogError(ctx, err, "failed to create user", map[string]interface{}{
				"email": req.Email,
			})
			c.JSON(500, gin.H{"error": "Failed to create user"})
			return
		}

		logger.LogBusinessEvent(ctx, "user_registered", map[string]interface{}{
			"user_id": user.ID,
			"email":   user.Email,
		})

		c.JSON(201, gin.H{
			"message": "User created successfully",
			"user_id": user.ID,
		})
	}
}

func loginHandler(db *database.PostgresClient, cache *cache.RedisClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		var req struct {
			Email    string `json:"email" binding:"required,email"`
			Password string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		// Find user
		var user User
		if err := db.WithContext(ctx).Where("email = ?", req.Email).First(&user).Error; err != nil {
			logger.LogSecurityEvent(ctx, "failed_login_attempt", map[string]interface{}{
				"email":     req.Email,
				"client_ip": c.ClientIP(),
			})
			c.JSON(401, gin.H{"error": "Invalid credentials"})
			return
		}

		// Verify password
		if !verifyPassword(req.Password, user.Password) {
			logger.LogSecurityEvent(ctx, "failed_login_attempt", map[string]interface{}{
				"email":     req.Email,
				"user_id":   user.ID,
				"client_ip": c.ClientIP(),
			})
			c.JSON(401, gin.H{"error": "Invalid credentials"})
			return
		}

		// Generate JWT token (implement JWT logic)
		token := generateJWTToken(user)

		// Store session in cache if available
		if cache != nil {
			sessionData := map[string]interface{}{
				"user_id":    user.ID,
				"email":      user.Email,
				"login_time": "now",
			}
			cache.Set(ctx, fmt.Sprintf("session:%d", user.ID), sessionData, 24*time.Hour)
		}

		logger.LogBusinessEvent(ctx, "user_login", map[string]interface{}{
			"user_id":   user.ID,
			"email":     user.Email,
			"client_ip": c.ClientIP(),
		})

		c.JSON(200, gin.H{
			"token": token,
			"user": gin.H{
				"id":    user.ID,
				"email": user.Email,
				"name":  user.Name,
			},
		})
	}
}

func logoutHandler(cache *cache.RedisClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		// Get user ID from context (set by auth middleware)
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		// Remove session from cache
		if cache != nil {
			cache.Delete(ctx, fmt.Sprintf("session:%v", userID))
		}

		logger.LogBusinessEvent(ctx, "user_logout", map[string]interface{}{
			"user_id": userID,
		})

		c.JSON(200, gin.H{"message": "Logged out successfully"})
	}
}

func profileHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		var user User
		if err := db.WithContext(ctx).First(&user, userID).Error; err != nil {
			logger.LogError(ctx, err, "failed to get user profile", map[string]interface{}{
				"user_id": userID,
			})
			c.JSON(404, gin.H{"error": "User not found"})
			return
		}

		c.JSON(200, gin.H{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		})
	}
}

func listUsersHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		
		var users []User
		if err := db.WithContext(ctx).Find(&users).Error; err != nil {
			logger.LogError(ctx, err, "failed to list users", nil)
			c.JSON(500, gin.H{"error": "Failed to retrieve users"})
			return
		}

		c.JSON(200, gin.H{"users": users})
	}
}

func getUserHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")
		
		var user User
		if err := db.WithContext(ctx).First(&user, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "User not found"})
			return
		}

		c.JSON(200, user)
	}
}

func updateUserHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")
		
		var user User
		if err := db.WithContext(ctx).First(&user, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "User not found"})
			return
		}

		var req struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		user.Name = req.Name
		user.Email = req.Email

		if err := db.WithContext(ctx).Save(&user).Error; err != nil {
			logger.LogError(ctx, err, "failed to update user", map[string]interface{}{
				"user_id": user.ID,
			})
			c.JSON(500, gin.H{"error": "Failed to update user"})
			return
		}

		logger.LogBusinessEvent(ctx, "user_updated", map[string]interface{}{
			"user_id": user.ID,
		})

		c.JSON(200, user)
	}
}

func deleteUserHandler(db *database.PostgresClient, logger logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")
		
		if err := db.WithContext(ctx).Delete(&User{}, id).Error; err != nil {
			logger.LogError(ctx, err, "failed to delete user", map[string]interface{}{
				"user_id": id,
			})
			c.JSON(500, gin.H{"error": "Failed to delete user"})
			return
		}

		logger.LogBusinessEvent(ctx, "user_deleted", map[string]interface{}{
			"user_id": id,
		})

		c.JSON(200, gin.H{"message": "User deleted successfully"})
	}
}

// Placeholder auth middleware
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Implement JWT token validation here
		// For now, just pass through
		c.Set("user_id", uint(1)) // Mock user ID
		c.Next()
	}
}

// Placeholder functions - implement these based on your needs
func hashPassword(password string) string {
	// Implement bcrypt or similar
	return "hashed_" + password
}

func verifyPassword(password, hash string) bool {
	// Implement bcrypt verification
	return "hashed_"+password == hash
}

func generateJWTToken(user User) string {
	// Implement JWT token generation
	return "jwt_token_" + user.Email
}