package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lukcba-developers/gopherkit/examples/user-api-migrated/internal/application"
	"github.com/lukcba-developers/gopherkit/examples/user-api-migrated/internal/domain/entity"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/middleware/auth"
)

// UserHandler maneja las peticiones HTTP relacionadas con usuarios
type UserHandler struct {
	userService *application.UserService
	logger      logger.Logger
}

// NewUserHandler crea una nueva instancia del handler de usuarios
func NewUserHandler(userService *application.UserService, logger logger.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

// CreateUserRequest representa la petición para crear un usuario
type CreateUserRequest struct {
	Email             string                 `json:"email" binding:"required,email"`
	Name              string                 `json:"name" binding:"required,min=2,max=100"`
	Phone             *string                `json:"phone,omitempty"`
	SportsPreferences map[string]interface{} `json:"sports_preferences,omitempty"`
	SkillLevel        *entity.SkillLevel     `json:"skill_level,omitempty"`
}

// UpdateUserRequest representa la petición para actualizar un usuario
type UpdateUserRequest struct {
	Name              *string                `json:"name,omitempty" binding:"omitempty,min=2,max=100"`
	Phone             *string                `json:"phone,omitempty"`
	SportsPreferences map[string]interface{} `json:"sports_preferences,omitempty"`
	SkillLevel        *entity.SkillLevel     `json:"skill_level,omitempty"`
	IsActive          *bool                  `json:"is_active,omitempty"`
}

// UserResponse representa la respuesta de usuario
type UserResponse struct {
	*entity.User
	Stats *entity.UserStats `json:"stats,omitempty"`
}

// ListUsersResponse representa la respuesta paginada de usuarios
type ListUsersResponse struct {
	Users      []*entity.User `json:"users"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	PerPage    int            `json:"per_page"`
	TotalPages int            `json:"total_pages"`
}

// CreateUser crea un nuevo usuario
// @Summary Crear usuario
// @Description Crear un nuevo usuario en el sistema
// @Tags usuarios
// @Accept json
// @Produce json
// @Param request body CreateUserRequest true "Datos del usuario"
// @Success 201 {object} UserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [post]
// @Security BearerAuth
func (h *UserHandler) CreateUser(c *gin.Context) {
	ctx := c.Request.Context()
	
	// Obtener tenant ID del contexto
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		h.logger.LogSecurityEvent(ctx, "missing_tenant_id", map[string]interface{}{
			"endpoint": "/users",
			"method":   "POST",
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.LogBusinessEvent(ctx, "invalid_create_user_request", map[string]interface{}{
			"error":     err.Error(),
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validar skill level si se proporciona
	if req.SkillLevel != nil {
		tempUser := &entity.User{SkillLevel: req.SkillLevel}
		if !tempUser.IsValidSkillLevel() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid skill level"})
			return
		}
	}

	// Crear usuario
	user, err := h.userService.CreateUser(ctx, req.Email, req.Name, tenantID)
	if err != nil {
		if err.Error() == "user with email "+req.Email+" already exists in tenant "+tenantID {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		h.logger.LogError(ctx, err, "failed to create user", map[string]interface{}{
			"email":     req.Email,
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	// Actualizar campos adicionales si se proporcionaron
	if req.Phone != nil || req.SportsPreferences != nil || req.SkillLevel != nil {
		updates := make(map[string]interface{})
		if req.Phone != nil {
			updates["phone"] = *req.Phone
		}
		if req.SkillLevel != nil {
			updates["skill_level"] = *req.SkillLevel
		}
		if req.SportsPreferences != nil {
			if err := user.SetSportsPreferences(req.SportsPreferences); err != nil {
				h.logger.LogError(ctx, err, "failed to set sports preferences", nil)
			} else {
				updates["sports_preferences"] = user.SportsPreferences
			}
		}

		if len(updates) > 0 {
			user, err = h.userService.UpdateUser(ctx, user.ID, tenantID, updates)
			if err != nil {
				h.logger.LogError(ctx, err, "failed to update user additional fields", nil)
				// No fallar la creación por esto
			}
		}
	}

	c.JSON(http.StatusCreated, UserResponse{User: user})
}

// GetUser obtiene un usuario por ID
// @Summary Obtener usuario
// @Description Obtiene un usuario por su ID
// @Tags usuarios
// @Produce json
// @Param id path string true "ID del usuario"
// @Param include_stats query bool false "Incluir estadísticas del usuario"
// @Success 200 {object} UserResponse
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [get]
// @Security BearerAuth
func (h *UserHandler) GetUser(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Param("id")
	tenantID := c.GetString("tenant_id")
	includeStats := c.Query("include_stats") == "true"

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	user, err := h.userService.GetUserByID(ctx, userID, tenantID)
	if err != nil {
		if err.Error() == "user not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		h.logger.LogError(ctx, err, "failed to get user", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}

	response := UserResponse{User: user}

	// Incluir estadísticas si se solicita
	if includeStats {
		stats, err := h.userService.GetUserStats(ctx, userID, tenantID)
		if err != nil && err.Error() != "user stats not found" {
			h.logger.LogError(ctx, err, "failed to get user stats", nil)
		} else if stats != nil {
			response.Stats = stats
		}
	}

	c.JSON(http.StatusOK, response)
}

// UpdateUser actualiza un usuario existente
// @Summary Actualizar usuario
// @Description Actualiza los datos de un usuario existente
// @Tags usuarios
// @Accept json
// @Produce json
// @Param id path string true "ID del usuario"
// @Param request body UpdateUserRequest true "Datos a actualizar"
// @Success 200 {object} UserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [put]
// @Security BearerAuth
func (h *UserHandler) UpdateUser(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Param("id")
	tenantID := c.GetString("tenant_id")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validar skill level si se proporciona
	if req.SkillLevel != nil {
		tempUser := &entity.User{SkillLevel: req.SkillLevel}
		if !tempUser.IsValidSkillLevel() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid skill level"})
			return
		}
	}

	// Construir mapa de actualizaciones
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.SkillLevel != nil {
		updates["skill_level"] = *req.SkillLevel
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	// Manejar sports preferences por separado
	if req.SportsPreferences != nil {
		// Obtener usuario actual para serializar preferencias
		user, err := h.userService.GetUserByID(ctx, userID, tenantID)
		if err != nil {
			if err.Error() == "user not found" {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
			return
		}

		if err := user.SetSportsPreferences(req.SportsPreferences); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sports preferences"})
			return
		}
		updates["sports_preferences"] = user.SportsPreferences
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	user, err := h.userService.UpdateUser(ctx, userID, tenantID, updates)
	if err != nil {
		if err.Error() == "user not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		h.logger.LogError(ctx, err, "failed to update user", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, UserResponse{User: user})
}

// DeleteUser elimina un usuario (soft delete)
// @Summary Eliminar usuario
// @Description Elimina un usuario del sistema (soft delete)
// @Tags usuarios
// @Param id path string true "ID del usuario"
// @Success 204
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [delete]
// @Security BearerAuth
func (h *UserHandler) DeleteUser(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Param("id")
	tenantID := c.GetString("tenant_id")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	err := h.userService.DeleteUser(ctx, userID, tenantID)
	if err != nil {
		if err.Error() == "user not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		h.logger.LogError(ctx, err, "failed to delete user", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListUsers lista usuarios con paginación
// @Summary Listar usuarios
// @Description Lista usuarios con paginación
// @Tags usuarios
// @Produce json
// @Param page query int false "Número de página" default(1)
// @Param per_page query int false "Usuarios por página" default(10)
// @Success 200 {object} ListUsersResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [get]
// @Security BearerAuth
func (h *UserHandler) ListUsers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetString("tenant_id")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	// Parsear parámetros de paginación
	page := 1
	perPage := 10

	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if pp := c.Query("per_page"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
			perPage = parsed
		}
	}

	offset := (page - 1) * perPage

	users, total, err := h.userService.ListUsers(ctx, tenantID, perPage, offset)
	if err != nil {
		h.logger.LogError(ctx, err, "failed to list users", map[string]interface{}{
			"tenant_id": tenantID,
			"page":      page,
			"per_page":  perPage,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	response := ListUsersResponse{
		Users:      users,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}

	c.JSON(http.StatusOK, response)
}

// GetProfile obtiene el perfil del usuario autenticado
// @Summary Obtener perfil
// @Description Obtiene el perfil del usuario autenticado
// @Tags usuarios
// @Produce json
// @Param include_stats query bool false "Incluir estadísticas del usuario"
// @Success 200 {object} UserResponse
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/me [get]
// @Security BearerAuth
func (h *UserHandler) GetProfile(c *gin.Context) {
	ctx := c.Request.Context()
	
	// Obtener usuario autenticado del contexto JWT
	claims := auth.GetUserFromContext(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	includeStats := c.Query("include_stats") == "true"

	user, err := h.userService.GetUserByID(ctx, claims.UserID, claims.TenantID)
	if err != nil {
		if err.Error() == "user not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		h.logger.LogError(ctx, err, "failed to get user profile", map[string]interface{}{
			"user_id":   claims.UserID,
			"tenant_id": claims.TenantID,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user profile"})
		return
	}

	response := UserResponse{User: user}

	// Incluir estadísticas si se solicita
	if includeStats {
		stats, err := h.userService.GetUserStats(ctx, claims.UserID, claims.TenantID)
		if err != nil && err.Error() != "user stats not found" {
			h.logger.LogError(ctx, err, "failed to get user stats", nil)
		} else if stats != nil {
			response.Stats = stats
		}
	}

	c.JSON(http.StatusOK, response)
}