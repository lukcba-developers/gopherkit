package cqrs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ReadModel represents a read-optimized data model
type ReadModel interface {
	GetID() string
	GetType() string
	GetVersion() int
	GetLastUpdated() time.Time
}

// ReadModelStore handles storage and retrieval of read models
type ReadModelStore struct {
	db     *gorm.DB
	logger *logrus.Logger
	config ReadModelConfig
}

// ReadModelConfig holds read model configuration
type ReadModelConfig struct {
	TablePrefix     string        `json:"table_prefix"`
	EnableVersioning bool         `json:"enable_versioning"`
	EnableTTL       bool         `json:"enable_ttl"`
	DefaultTTL      time.Duration `json:"default_ttl"`
	BatchSize       int          `json:"batch_size"`
}

// ReadModelRecord represents a stored read model
type ReadModelRecord struct {
	ID          string    `gorm:"type:uuid;primary_key" json:"id"`
	Type        string    `gorm:"type:varchar(255);not null;index" json:"type"`
	Data        string    `gorm:"type:jsonb;not null" json:"data"`
	Version     int       `gorm:"not null;default:1" json:"version"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	ExpiresAt   *time.Time `gorm:"index" json:"expires_at,omitempty"`
	TenantID    string    `gorm:"type:uuid;index" json:"tenant_id"`
	Metadata    string    `gorm:"type:jsonb" json:"metadata,omitempty"`
}

// BaseReadModel provides a base implementation for read models
type BaseReadModel struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Version     int                    `json:"version"`
	LastUpdated time.Time              `json:"last_updated"`
	TenantID    string                 `json:"tenant_id"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// GetID returns the read model ID
func (brm *BaseReadModel) GetID() string {
	return brm.ID
}

// GetType returns the read model type
func (brm *BaseReadModel) GetType() string {
	return brm.Type
}

// GetVersion returns the read model version
func (brm *BaseReadModel) GetVersion() int {
	return brm.Version
}

// GetLastUpdated returns the last updated timestamp
func (brm *BaseReadModel) GetLastUpdated() time.Time {
	return brm.LastUpdated
}

// NewReadModelStore creates a new read model store
func NewReadModelStore(db *gorm.DB, config ReadModelConfig, logger *logrus.Logger) (*ReadModelStore, error) {
	// Set default config values
	if config.TablePrefix == "" {
		config.TablePrefix = "read_models"
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 24 * time.Hour
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}

	store := &ReadModelStore{
		db:     db,
		logger: logger,
		config: config,
	}

	// Auto-migrate table
	if err := store.autoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate read model tables: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"table_prefix":     config.TablePrefix,
		"enable_versioning": config.EnableVersioning,
		"enable_ttl":       config.EnableTTL,
	}).Info("Read model store initialized successfully")

	return store, nil
}

// Save saves a read model to the store
func (rms *ReadModelStore) Save(ctx context.Context, model ReadModel) error {
	return rms.SaveWithTTL(ctx, model, rms.config.DefaultTTL)
}

// SaveWithTTL saves a read model with a custom TTL
func (rms *ReadModelStore) SaveWithTTL(ctx context.Context, model ReadModel, ttl time.Duration) error {
	// Serialize model data
	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("failed to serialize read model: %w", err)
	}

	now := time.Now()
	var expiresAt *time.Time
	if rms.config.EnableTTL {
		expiry := now.Add(ttl)
		expiresAt = &expiry
	}

	record := &ReadModelRecord{
		ID:        model.GetID(),
		Type:      model.GetType(),
		Data:      string(data),
		Version:   model.GetVersion(),
		UpdatedAt: now,
		ExpiresAt: expiresAt,
	}

	// Extract tenant ID and metadata if available
	if baseModel, ok := model.(*BaseReadModel); ok {
		record.TenantID = baseModel.TenantID
		if len(baseModel.Metadata) > 0 {
			metadataBytes, _ := json.Marshal(baseModel.Metadata)
			record.Metadata = string(metadataBytes)
		}
	}

	tableName := rms.getTableName(model.GetType())

	// Check if record exists
	var existingRecord ReadModelRecord
	err = rms.db.WithContext(ctx).
		Table(tableName).
		Where("id = ?", model.GetID()).
		First(&existingRecord).Error

	if err == gorm.ErrRecordNotFound {
		// Create new record
		record.CreatedAt = now
		err = rms.db.WithContext(ctx).Table(tableName).Create(record).Error
		if err != nil {
			return fmt.Errorf("failed to create read model: %w", err)
		}

		rms.logger.WithFields(logrus.Fields{
			"id":    model.GetID(),
			"type":  model.GetType(),
			"table": tableName,
		}).Debug("Read model created")
	} else if err != nil {
		return fmt.Errorf("failed to check existing read model: %w", err)
	} else {
		// Update existing record
		if rms.config.EnableVersioning && existingRecord.Version >= model.GetVersion() {
			return fmt.Errorf("version conflict: current version %d, provided version %d", 
				existingRecord.Version, model.GetVersion())
		}

		err = rms.db.WithContext(ctx).
			Table(tableName).
			Where("id = ?", model.GetID()).
			Updates(record).Error
		if err != nil {
			return fmt.Errorf("failed to update read model: %w", err)
		}

		rms.logger.WithFields(logrus.Fields{
			"id":           model.GetID(),
			"type":         model.GetType(),
			"old_version":  existingRecord.Version,
			"new_version":  model.GetVersion(),
			"table":        tableName,
		}).Debug("Read model updated")
	}

	return nil
}

// Get retrieves a read model by ID and type
func (rms *ReadModelStore) Get(ctx context.Context, id, modelType string, result interface{}) error {
	tableName := rms.getTableName(modelType)

	var record ReadModelRecord
	err := rms.db.WithContext(ctx).
		Table(tableName).
		Where("id = ?", id).
		First(&record).Error

	if err == gorm.ErrRecordNotFound {
		return fmt.Errorf("read model not found: %s", id)
	}
	if err != nil {
		return fmt.Errorf("failed to get read model: %w", err)
	}

	// Check if expired
	if rms.config.EnableTTL && record.ExpiresAt != nil && time.Now().After(*record.ExpiresAt) {
		// Delete expired record
		rms.db.WithContext(ctx).Table(tableName).Where("id = ?", id).Delete(&record)
		return fmt.Errorf("read model expired: %s", id)
	}

	// Deserialize data
	if err := json.Unmarshal([]byte(record.Data), result); err != nil {
		return fmt.Errorf("failed to deserialize read model: %w", err)
	}

	rms.logger.WithFields(logrus.Fields{
		"id":      id,
		"type":    modelType,
		"version": record.Version,
		"table":   tableName,
	}).Debug("Read model retrieved")

	return nil
}

// List retrieves multiple read models by type with optional filtering
func (rms *ReadModelStore) List(ctx context.Context, modelType string, filter map[string]interface{}, limit, offset int) ([]*ReadModelRecord, int64, error) {
	tableName := rms.getTableName(modelType)

	query := rms.db.WithContext(ctx).Table(tableName).Where("type = ?", modelType)

	// Apply TTL filtering
	if rms.config.EnableTTL {
		query = query.Where("expires_at IS NULL OR expires_at > ?", time.Now())
	}

	// Apply filters
	for key, value := range filter {
		if key == "tenant_id" {
			query = query.Where("tenant_id = ?", value)
		} else {
			// For JSON field searches, use JSONB operators
			query = query.Where("data ->> ? = ?", key, value)
		}
	}

	// Count total records
	var total int64
	countQuery := query
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count read models: %w", err)
	}

	// Apply pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var records []*ReadModelRecord
	if err := query.Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list read models: %w", err)
	}

	rms.logger.WithFields(logrus.Fields{
		"type":   modelType,
		"count":  len(records),
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"table":  tableName,
	}).Debug("Read models listed")

	return records, total, nil
}

// Delete removes a read model
func (rms *ReadModelStore) Delete(ctx context.Context, id, modelType string) error {
	tableName := rms.getTableName(modelType)

	result := rms.db.WithContext(ctx).
		Table(tableName).
		Where("id = ? AND type = ?", id, modelType).
		Delete(&ReadModelRecord{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete read model: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("read model not found: %s", id)
	}

	rms.logger.WithFields(logrus.Fields{
		"id":    id,
		"type":  modelType,
		"table": tableName,
	}).Debug("Read model deleted")

	return nil
}

// DeleteByType removes all read models of a specific type
func (rms *ReadModelStore) DeleteByType(ctx context.Context, modelType string) error {
	tableName := rms.getTableName(modelType)

	result := rms.db.WithContext(ctx).
		Table(tableName).
		Where("type = ?", modelType).
		Delete(&ReadModelRecord{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete read models by type: %w", result.Error)
	}

	rms.logger.WithFields(logrus.Fields{
		"type":          modelType,
		"deleted_count": result.RowsAffected,
		"table":         tableName,
	}).Info("Read models deleted by type")

	return nil
}

// CleanupExpired removes expired read models
func (rms *ReadModelStore) CleanupExpired(ctx context.Context) (int64, error) {
	if !rms.config.EnableTTL {
		return 0, nil
	}

	now := time.Now()
	var deletedCount int64

	// Get all table names (simplified - in production, track model types)
	modelTypes := []string{"user", "booking", "payment", "organization"} // Add more as needed

	for _, modelType := range modelTypes {
		tableName := rms.getTableName(modelType)

		result := rms.db.WithContext(ctx).
			Table(tableName).
			Where("expires_at IS NOT NULL AND expires_at <= ?", now).
			Delete(&ReadModelRecord{})

		if result.Error != nil {
			rms.logger.WithFields(logrus.Fields{
				"type":  modelType,
				"table": tableName,
				"error": result.Error,
			}).Error("Failed to cleanup expired read models")
			continue
		}

		deletedCount += result.RowsAffected
	}

	rms.logger.WithFields(logrus.Fields{
		"deleted_count": deletedCount,
		"cleanup_time":  now,
	}).Info("Expired read models cleaned up")

	return deletedCount, nil
}

// Exists checks if a read model exists
func (rms *ReadModelStore) Exists(ctx context.Context, id, modelType string) (bool, error) {
	tableName := rms.getTableName(modelType)

	var count int64
	err := rms.db.WithContext(ctx).
		Table(tableName).
		Where("id = ? AND type = ?", id, modelType).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check read model existence: %w", err)
	}

	return count > 0, nil
}

// GetStats returns statistics about stored read models
func (rms *ReadModelStore) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count by type
	modelTypes := []string{"user", "booking", "payment", "organization"} // Add more as needed
	totalCount := int64(0)

	for _, modelType := range modelTypes {
		tableName := rms.getTableName(modelType)

		var count int64
		err := rms.db.WithContext(ctx).
			Table(tableName).
			Where("type = ?", modelType).
			Count(&count).Error

		if err == nil {
			stats[fmt.Sprintf("%s_count", modelType)] = count
			totalCount += count
		}
	}

	stats["total_count"] = totalCount

	// Expired count if TTL is enabled
	if rms.config.EnableTTL {
		var expiredCount int64
		for _, modelType := range modelTypes {
			tableName := rms.getTableName(modelType)

			var count int64
			err := rms.db.WithContext(ctx).
				Table(tableName).
				Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
				Count(&count).Error

			if err == nil {
				expiredCount += count
			}
		}
		stats["expired_count"] = expiredCount
	}

	stats["config"] = rms.config

	return stats, nil
}

// Private helper methods

func (rms *ReadModelStore) getTableName(modelType string) string {
	return fmt.Sprintf("%s_%s", rms.config.TablePrefix, modelType)
}

func (rms *ReadModelStore) autoMigrate() error {
	// Create a sample table for auto-migration
	sampleTable := fmt.Sprintf("%s_sample", rms.config.TablePrefix)
	
	if err := rms.db.Table(sampleTable).AutoMigrate(&ReadModelRecord{}); err != nil {
		return fmt.Errorf("failed to migrate read model table: %w", err)
	}

	// Create indexes for better performance
	rms.createIndexes(sampleTable)

	return nil
}

func (rms *ReadModelStore) createIndexes(tableName string) {
	// Create composite indexes for common queries
	rms.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_type_tenant 
		ON %s (type, tenant_id)`, tableName, tableName))
	
	rms.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_type_updated 
		ON %s (type, updated_at DESC)`, tableName, tableName))
	
	if rms.config.EnableTTL {
		rms.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_expires 
			ON %s (expires_at) WHERE expires_at IS NOT NULL`, tableName, tableName))
	}
}

// Close closes the read model store
func (rms *ReadModelStore) Close() error {
	rms.logger.Info("Read model store closed")
	return nil
}