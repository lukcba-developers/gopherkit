package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/lukcba-developers/gopherkit/pkg/errors"
)

// Entity interface que deben implementar todas las entidades
type Entity interface {
	GetID() string
	SetID(id string)
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	SetCreatedAt(time.Time)
	SetUpdatedAt(time.Time)
	TableName() string
}

// BaseEntity proporciona campos comunes para todas las entidades
type BaseEntity struct {
	ID        string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (e *BaseEntity) GetID() string          { return e.ID }
func (e *BaseEntity) SetID(id string)       { e.ID = id }
func (e *BaseEntity) GetCreatedAt() time.Time { return e.CreatedAt }
func (e *BaseEntity) GetUpdatedAt() time.Time { return e.UpdatedAt }
func (e *BaseEntity) SetCreatedAt(t time.Time) { e.CreatedAt = t }
func (e *BaseEntity) SetUpdatedAt(t time.Time) { e.UpdatedAt = t }

// BaseRepository proporciona operaciones CRUD comunes
type BaseRepository[T Entity] struct {
	db          *gorm.DB
	tracer      trace.Tracer
	entityType  reflect.Type
	serviceName string
}

// NewBaseRepository crea una nueva instancia del repositorio base
func NewBaseRepository[T Entity](db *gorm.DB, serviceName string) *BaseRepository[T] {
	var entity T
	entityType := reflect.TypeOf(entity).Elem()
	
	return &BaseRepository[T]{
		db:          db,
		tracer:      otel.Tracer(fmt.Sprintf("%s-repository", serviceName)),
		entityType:  entityType,
		serviceName: serviceName,
	}
}

// Create crea una nueva entidad
func (r *BaseRepository[T]) Create(ctx context.Context, entity T) error {
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.create", entity.TableName()))
	defer span.End()

	// Generar ID si no existe
	if entity.GetID() == "" {
		entity.SetID(uuid.New().String())
	}

	// Establecer timestamps
	now := time.Now()
	entity.SetCreatedAt(now)
	entity.SetUpdatedAt(now)

	span.SetAttributes(
		attribute.String("entity.id", entity.GetID()),
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "create"),
	)

	if err := r.db.WithContext(ctx).Create(entity).Error; err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.handleError("create", err)
	}

	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// FindByID busca una entidad por ID
func (r *BaseRepository[T]) FindByID(ctx context.Context, id string) (T, error) {
	var entity T
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.find_by_id", entity.TableName()))
	defer span.End()

	span.SetAttributes(
		attribute.String("entity.id", id),
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "find_by_id"),
	)

	if id == "" {
		span.SetAttributes(attribute.Bool("error", true))
		return entity, errors.NewDomainError(
			"INVALID_ID",
			"Entity ID cannot be empty",
			errors.CategoryValidation,
			errors.SeverityMedium,
		).WithMetadata("entity_type", entity.TableName())
	}

	err := r.db.WithContext(ctx).Where("id = ?", id).First(&entity).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			span.SetAttributes(attribute.Bool("not_found", true))
			return entity, errors.NewDomainError(
				"ENTITY_NOT_FOUND",
				fmt.Sprintf("%s not found", entity.TableName()),
				errors.CategoryBusiness,
				errors.SeverityMedium,
			).WithMetadata("entity_id", id).WithMetadata("entity_type", entity.TableName())
		}
		span.SetAttributes(attribute.Bool("error", true))
		return entity, r.handleError("find_by_id", err)
	}

	span.SetAttributes(attribute.Bool("success", true))
	return entity, nil
}

// Update actualiza una entidad existente
func (r *BaseRepository[T]) Update(ctx context.Context, entity T) error {
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.update", entity.TableName()))
	defer span.End()

	span.SetAttributes(
		attribute.String("entity.id", entity.GetID()),
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "update"),
	)

	// Actualizar timestamp
	entity.SetUpdatedAt(time.Now())

	result := r.db.WithContext(ctx).Save(entity)
	if result.Error != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.handleError("update", result.Error)
	}

	if result.RowsAffected == 0 {
		span.SetAttributes(attribute.Bool("not_found", true))
		return errors.NewDomainError(
			"ENTITY_NOT_FOUND",
			fmt.Sprintf("%s not found for update", entity.TableName()),
			errors.CategoryBusiness,
			errors.SeverityMedium,
		).WithMetadata("entity_id", entity.GetID()).WithMetadata("entity_type", entity.TableName())
	}

	span.SetAttributes(
		attribute.Bool("success", true),
		attribute.Int64("rows_affected", result.RowsAffected),
	)
	return nil
}

// Delete elimina una entidad por ID
func (r *BaseRepository[T]) Delete(ctx context.Context, id string) error {
	var entity T
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.delete", entity.TableName()))
	defer span.End()

	span.SetAttributes(
		attribute.String("entity.id", id),
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "delete"),
	)

	if id == "" {
		span.SetAttributes(attribute.Bool("error", true))
		return errors.NewDomainError(
			"INVALID_ID",
			"Entity ID cannot be empty",
			errors.CategoryValidation,
			errors.SeverityMedium,
		).WithMetadata("entity_type", entity.TableName())
	}

	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&entity)
	if result.Error != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.handleError("delete", result.Error)
	}

	if result.RowsAffected == 0 {
		span.SetAttributes(attribute.Bool("not_found", true))
		return errors.NewDomainError(
			"ENTITY_NOT_FOUND",
			fmt.Sprintf("%s not found for deletion", entity.TableName()),
			errors.CategoryBusiness,
			errors.SeverityMedium,
		).WithMetadata("entity_id", id).WithMetadata("entity_type", entity.TableName())
	}

	span.SetAttributes(
		attribute.Bool("success", true),
		attribute.Int64("rows_affected", result.RowsAffected),
	)
	return nil
}

// FindAll obtiene todas las entidades con paginación
func (r *BaseRepository[T]) FindAll(ctx context.Context, offset, limit int) ([]T, int64, error) {
	var entity T
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.find_all", entity.TableName()))
	defer span.End()

	span.SetAttributes(
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "find_all"),
		attribute.Int("offset", offset),
		attribute.Int("limit", limit),
	)

	var entities []T
	var count int64

	// Obtener count total
	if err := r.db.WithContext(ctx).Model(&entity).Count(&count).Error; err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return nil, 0, r.handleError("find_all_count", err)
	}

	// Obtener entidades con paginación
	query := r.db.WithContext(ctx).Offset(offset)
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&entities).Error; err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return nil, 0, r.handleError("find_all", err)
	}

	span.SetAttributes(
		attribute.Bool("success", true),
		attribute.Int64("total_count", count),
		attribute.Int("returned_count", len(entities)),
	)

	return entities, count, nil
}

// FindByCondition busca entidades que cumplan una condición
func (r *BaseRepository[T]) FindByCondition(ctx context.Context, condition string, args ...interface{}) ([]T, error) {
	var entity T
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.find_by_condition", entity.TableName()))
	defer span.End()

	span.SetAttributes(
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "find_by_condition"),
		attribute.String("condition", condition),
	)

	var entities []T
	if err := r.db.WithContext(ctx).Where(condition, args...).Find(&entities).Error; err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return nil, r.handleError("find_by_condition", err)
	}

	span.SetAttributes(
		attribute.Bool("success", true),
		attribute.Int("found_count", len(entities)),
	)

	return entities, nil
}

// Exists verifica si una entidad existe por ID
func (r *BaseRepository[T]) Exists(ctx context.Context, id string) (bool, error) {
	var entity T
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.exists", entity.TableName()))
	defer span.End()

	span.SetAttributes(
		attribute.String("entity.id", id),
		attribute.String("entity.table", entity.TableName()),
		attribute.String("operation", "exists"),
	)

	var count int64
	err := r.db.WithContext(ctx).Model(&entity).Where("id = ?", id).Count(&count).Error
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return false, r.handleError("exists", err)
	}

	exists := count > 0
	span.SetAttributes(
		attribute.Bool("success", true),
		attribute.Bool("exists", exists),
	)

	return exists, nil
}

// Transaction ejecuta operaciones dentro de una transacción
func (r *BaseRepository[T]) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("repository.%s.transaction", r.serviceName))
	defer span.End()

	span.SetAttributes(attribute.String("operation", "transaction"))

	err := r.db.WithContext(ctx).Transaction(fn)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.handleError("transaction", err)
	}

	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// GetDB retorna la instancia de GORM para queries personalizadas
func (r *BaseRepository[T]) GetDB() *gorm.DB {
	return r.db
}

// handleError convierte errores de base de datos en domain errors
func (r *BaseRepository[T]) handleError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// Mapping de errores específicos de PostgreSQL/GORM
	switch {
	case err == gorm.ErrRecordNotFound:
		return errors.NewDomainError(
			"ENTITY_NOT_FOUND",
			"Entity not found",
			errors.CategoryBusiness,
			errors.SeverityMedium,
		).WithCause(err).WithMetadata("operation", operation)

	case err == sql.ErrNoRows:
		return errors.NewDomainError(
			"ENTITY_NOT_FOUND",
			"Entity not found",
			errors.CategoryBusiness,
			errors.SeverityMedium,
		).WithCause(err).WithMetadata("operation", operation)

	default:
		// Error genérico de infraestructura
		return errors.NewDomainError(
			"DATABASE_ERROR",
			"Database operation failed",
			errors.CategoryInfrastructure,
			errors.SeverityHigh,
		).WithCause(err).WithMetadata("operation", operation).WithStackTrace(1)
	}
}