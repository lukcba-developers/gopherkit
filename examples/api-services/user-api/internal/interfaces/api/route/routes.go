package route

import (
	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/examples/user-api-migrated/internal/application"
	"github.com/lukcba-developers/gopherkit/examples/user-api-migrated/internal/interfaces/api/handler"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
)

// SetupRoutes configura todas las rutas de la API
func SetupRoutes(
	router *gin.Engine,
	userService *application.UserService,
	jwtMiddleware *auth.JWTMiddleware,
	logger logger.Logger,
) {
	// Inicializar handlers
	userHandler := handler.NewUserHandler(userService, logger)

	// Grupo de API con versionado
	v1 := router.Group("/api/v1")
	{
		// Rutas públicas (sin autenticación)
		public := v1.Group("")
		{
			// Health check ya está configurado en main.go
			public.GET("/ping", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "pong", "service": "user-api-migrated"})
			})
		}

		// Rutas protegidas (requieren autenticación)
		protected := v1.Group("")
		protected.Use(jwtMiddleware.RequireAuth())
		{
			// Rutas de usuarios
			users := protected.Group("/users")
			{
				// CRUD básico de usuarios
				users.POST("", userHandler.CreateUser)           // POST /api/v1/users
				users.GET("", userHandler.ListUsers)             // GET /api/v1/users
				users.GET("/me", userHandler.GetProfile)         // GET /api/v1/users/me
				users.GET("/:id", userHandler.GetUser)           // GET /api/v1/users/:id
				users.PUT("/:id", userHandler.UpdateUser)        // PUT /api/v1/users/:id
				users.DELETE("/:id", userHandler.DeleteUser)     // DELETE /api/v1/users/:id
			}

			// Rutas para administradores (requieren rol admin)
			admin := protected.Group("/admin")
			admin.Use(jwtMiddleware.RequireRole("admin"))
			{
				// Gestión administrativa de usuarios
				admin.GET("/users", userHandler.ListUsers)
				admin.GET("/users/:id", userHandler.GetUser)
				admin.PUT("/users/:id", userHandler.UpdateUser)
				admin.DELETE("/users/:id", userHandler.DeleteUser)
			}
		}
	}

	// Documentación de API (disponible solo en desarrollo)
	// En producción real, esto debería estar protegido o deshabilitado
	v1.GET("/docs", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "API Documentation",
			"endpoints": map[string]interface{}{
				"public": []string{
					"GET /api/v1/ping - Health check",
				},
				"authenticated": []string{
					"POST /api/v1/users - Create user",
					"GET /api/v1/users - List users (paginated)",
					"GET /api/v1/users/me - Get current user profile",
					"GET /api/v1/users/:id - Get user by ID",
					"PUT /api/v1/users/:id - Update user",
					"DELETE /api/v1/users/:id - Delete user",
				},
				"admin": []string{
					"GET /api/v1/admin/users - List all users",
					"GET /api/v1/admin/users/:id - Get any user",
					"PUT /api/v1/admin/users/:id - Update any user",
					"DELETE /api/v1/admin/users/:id - Delete any user",
				},
			},
			"authentication": "Bearer token required for authenticated routes",
			"headers": map[string]string{
				"Authorization": "Bearer <jwt-token>",
				"Content-Type":  "application/json",
				"X-Tenant-ID":   "Required for multi-tenant operations",
			},
		})
	})
}