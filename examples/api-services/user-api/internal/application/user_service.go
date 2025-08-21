package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/lukcba-developers/gopherkit/examples/user-api-migrated/internal/domain/entity"
	"github.com/lukcba-developers/gopherkit/pkg/cache"
	"github.com/lukcba-developers/gopherkit/pkg/database"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// UserService proporciona la lógica de negocio para usuarios
type UserService struct {
	db           *database.PostgresClient
	cache        *cache.RedisClient
	logger       logger.Logger
	cacheEnabled bool
}

// NewUserService crea una nueva instancia del servicio de usuarios
func NewUserService(db *database.PostgresClient, cache *cache.RedisClient, logger logger.Logger) *UserService {
	return &UserService{
		db:           db,
		cache:        cache,
		logger:       logger,
		cacheEnabled: cache != nil,
	}
}

// CreateUser crea un nuevo usuario
func (s *UserService) CreateUser(ctx context.Context, email, name, tenantID string) (*entity.User, error) {
	s.logger.LogBusinessEvent(ctx, "user_creation_started", map[string]interface{}{
		"email":     email,
		"name":      name,
		"tenant_id": tenantID,
	})

	// Validar datos de entrada
	if email == "" || name == "" || tenantID == "" {
		return nil, fmt.Errorf("email, name and tenant_id are required")
	}

	// Crear nuevo usuario
	user := entity.NewUser(email, name, tenantID)
	user.ID = uuid.New().String()

	// Verificar si el usuario ya existe
	existingUser, err := s.GetUserByEmail(ctx, email, tenantID)
	if err == nil && existingUser != nil {
		return nil, fmt.Errorf("user with email %s already exists in tenant %s", email, tenantID)
	}

	// Guardar en base de datos
	if err := s.db.DB().WithContext(ctx).Create(user).Error; err != nil {
		s.logger.LogError(ctx, err, "failed to create user", map[string]interface{}{
			"email":     email,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Crear estadísticas iniciales del usuario
	userStats := &entity.UserStats{
		UserID:   user.ID,
		TenantID: tenantID,
		Points:   0,
		Level:    1,
	}

	if err := s.db.DB().WithContext(ctx).Create(userStats).Error; err != nil {
		s.logger.LogError(ctx, err, "failed to create user stats", map[string]interface{}{
			"user_id":   user.ID,
			"tenant_id": tenantID,
		})
		// No fallar la creación del usuario por esto
	}

	// Limpiar caché si está habilitado
	if s.cacheEnabled {
		s.invalidateUserCache(ctx, user.ID, email, tenantID)
	}

	s.logger.LogBusinessEvent(ctx, "user_created_successfully", map[string]interface{}{
		"user_id":   user.ID,
		"email":     email,
		"tenant_id": tenantID,
	})

	return user, nil
}

// GetUserByID obtiene un usuario por su ID
func (s *UserService) GetUserByID(ctx context.Context, userID, tenantID string) (*entity.User, error) {
	cacheKey := fmt.Sprintf("user:id:%s:%s", userID, tenantID)

	// Intentar obtener del caché primero
	if s.cacheEnabled {
		var user entity.User
		if err := s.cache.Get(ctx, cacheKey, &user); err == nil {
			s.logger.LogPerformanceEvent(ctx, "cache_hit", 0, map[string]interface{}{
				"cache_key": cacheKey,
				"user_id":   userID,
			})
			return &user, nil
		}
	}

	// Obtener de la base de datos
	var user entity.User
	start := time.Now()
	err := s.db.DB().WithContext(ctx).Where("id = ? AND tenant_id = ?", userID, tenantID).First(&user).Error
	duration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "SELECT users WHERE id = ? AND tenant_id = ?", duration, 1)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		s.logger.LogError(ctx, err, "failed to get user by ID", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Guardar en caché
	if s.cacheEnabled {
		if err := s.cache.Set(ctx, cacheKey, user, 5*time.Minute); err != nil {
			s.logger.LogError(ctx, err, "failed to cache user", map[string]interface{}{
				"cache_key": cacheKey,
				"user_id":   userID,
			})
		}
	}

	return &user, nil
}

// GetUserByEmail obtiene un usuario por su email
func (s *UserService) GetUserByEmail(ctx context.Context, email, tenantID string) (*entity.User, error) {
	cacheKey := fmt.Sprintf("user:email:%s:%s", email, tenantID)

	// Intentar obtener del caché primero
	if s.cacheEnabled {
		var user entity.User
		if err := s.cache.Get(ctx, cacheKey, &user); err == nil {
			s.logger.LogPerformanceEvent(ctx, "cache_hit", 0, map[string]interface{}{
				"cache_key": cacheKey,
				"email":     email,
			})
			return &user, nil
		}
	}

	// Obtener de la base de datos
	var user entity.User
	start := time.Now()
	err := s.db.DB().WithContext(ctx).Where("email = ? AND tenant_id = ?", email, tenantID).First(&user).Error
	duration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "SELECT users WHERE email = ? AND tenant_id = ?", duration, 1)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		s.logger.LogError(ctx, err, "failed to get user by email", map[string]interface{}{
			"email":     email,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Guardar en caché
	if s.cacheEnabled {
		if err := s.cache.Set(ctx, cacheKey, user, 5*time.Minute); err != nil {
			s.logger.LogError(ctx, err, "failed to cache user", map[string]interface{}{
				"cache_key": cacheKey,
				"email":     email,
			})
		}
	}

	return &user, nil
}

// UpdateUser actualiza un usuario existente
func (s *UserService) UpdateUser(ctx context.Context, userID, tenantID string, updates map[string]interface{}) (*entity.User, error) {
	s.logger.LogBusinessEvent(ctx, "user_update_started", map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
		"fields":    len(updates),
	})

	// Obtener el usuario actual
	user, err := s.GetUserByID(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}

	// Aplicar actualizaciones
	start := time.Now()
	err = s.db.DB().WithContext(ctx).Model(user).Where("id = ? AND tenant_id = ?", userID, tenantID).Updates(updates).Error
	duration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "UPDATE users SET ... WHERE id = ? AND tenant_id = ?", duration, 1)

	if err != nil {
		s.logger.LogError(ctx, err, "failed to update user", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Limpiar caché
	if s.cacheEnabled {
		s.invalidateUserCache(ctx, userID, user.Email, tenantID)
	}

	// Obtener el usuario actualizado
	updatedUser, err := s.GetUserByID(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}

	s.logger.LogBusinessEvent(ctx, "user_updated_successfully", map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
	})

	return updatedUser, nil
}

// DeleteUser marca un usuario como eliminado (soft delete)
func (s *UserService) DeleteUser(ctx context.Context, userID, tenantID string) error {
	s.logger.LogBusinessEvent(ctx, "user_deletion_started", map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
	})

	// Obtener el usuario para logging
	user, err := s.GetUserByID(ctx, userID, tenantID)
	if err != nil {
		return err
	}

	// Soft delete
	start := time.Now()
	err = s.db.DB().WithContext(ctx).Where("id = ? AND tenant_id = ?", userID, tenantID).Delete(&entity.User{}).Error
	duration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = ? AND tenant_id = ?", duration, 1)

	if err != nil {
		s.logger.LogError(ctx, err, "failed to delete user", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Limpiar caché
	if s.cacheEnabled {
		s.invalidateUserCache(ctx, userID, user.Email, tenantID)
	}

	s.logger.LogBusinessEvent(ctx, "user_deleted_successfully", map[string]interface{}{
		"user_id":   userID,
		"tenant_id": tenantID,
		"email":     user.Email,
	})

	return nil
}

// ListUsers obtiene una lista paginada de usuarios
func (s *UserService) ListUsers(ctx context.Context, tenantID string, limit, offset int) ([]*entity.User, int64, error) {
	var users []*entity.User
	var total int64

	// Contar total de usuarios
	start := time.Now()
	err := s.db.DB().WithContext(ctx).Model(&entity.User{}).Where("tenant_id = ?", tenantID).Count(&total).Error
	countDuration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "SELECT COUNT(*) FROM users WHERE tenant_id = ?", countDuration, 1)

	if err != nil {
		s.logger.LogError(ctx, err, "failed to count users", map[string]interface{}{
			"tenant_id": tenantID,
		})
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Obtener usuarios paginados
	start = time.Now()
	err = s.db.DB().WithContext(ctx).Where("tenant_id = ?", tenantID).
		Limit(limit).Offset(offset).
		Order("created_at DESC").
		Find(&users).Error
	listDuration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "SELECT * FROM users WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?", listDuration, len(users))

	if err != nil {
		s.logger.LogError(ctx, err, "failed to list users", map[string]interface{}{
			"tenant_id": tenantID,
			"limit":     limit,
			"offset":    offset,
		})
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	s.logger.LogBusinessEvent(ctx, "users_listed", map[string]interface{}{
		"tenant_id":   tenantID,
		"count":       len(users),
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	})

	return users, total, nil
}

// GetUserStats obtiene las estadísticas de un usuario
func (s *UserService) GetUserStats(ctx context.Context, userID, tenantID string) (*entity.UserStats, error) {
	var stats entity.UserStats
	start := time.Now()
	err := s.db.DB().WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&stats).Error
	duration := time.Since(start)

	s.logger.LogDatabaseOperation(ctx, "SELECT * FROM user_stats WHERE user_id = ? AND tenant_id = ?", duration, 1)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user stats not found")
		}
		s.logger.LogError(ctx, err, "failed to get user stats", map[string]interface{}{
			"user_id":   userID,
			"tenant_id": tenantID,
		})
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	return &stats, nil
}

// invalidateUserCache limpia las entradas de caché relacionadas con el usuario
func (s *UserService) invalidateUserCache(ctx context.Context, userID, email, tenantID string) {
	if !s.cacheEnabled {
		return
	}

	keys := []string{
		fmt.Sprintf("user:id:%s:%s", userID, tenantID),
		fmt.Sprintf("user:email:%s:%s", email, tenantID),
	}

	for _, key := range keys {
		if err := s.cache.Delete(ctx, key); err != nil {
			s.logger.LogError(ctx, err, "failed to invalidate cache", map[string]interface{}{
				"cache_key": key,
			})
		}
	}
}