package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"github.com/sirupsen/logrus"

	"github.com/lukcba-developers/gopherkit/pkg/domain/entity"
	"github.com/lukcba-developers/gopherkit/pkg/domain/service"
)

// PostgresOrganizationConfigRepository implements OrganizationConfigRepository using PostgreSQL
type PostgresOrganizationConfigRepository struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewPostgresOrganizationConfigRepository creates a new PostgreSQL repository
func NewPostgresOrganizationConfigRepository(db *gorm.DB, logger *logrus.Logger) service.OrganizationConfigRepository {
	return &PostgresOrganizationConfigRepository{
		db:     db,
		logger: logger,
	}
}

// GetByOrganizationID retrieves an organization config by organization ID
func (r *PostgresOrganizationConfigRepository) GetByOrganizationID(ctx context.Context, organizationID string) (*entity.OrganizationConfig, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID cannot be empty")
	}

	var config entity.OrganizationConfig
	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).First(&config).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			r.logger.WithFields(logrus.Fields{
				"organization_id": organizationID,
			}).Debug("Organization config not found")
			return nil, nil // Return nil, nil for "not found" to distinguish from errors
		}
		
		r.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"error":          err,
		}).Error("Failed to get organization config")
		return nil, fmt.Errorf("failed to get organization config: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
		"config_id":       config.ID,
		"template_type":   config.TemplateType,
	}).Debug("Organization config retrieved successfully")

	return &config, nil
}

// Create creates a new organization config
func (r *PostgresOrganizationConfigRepository) Create(ctx context.Context, config *entity.OrganizationConfig) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid organization config: %w", err)
	}

	err := r.db.WithContext(ctx).Create(config).Error
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"organization_id": config.OrganizationID,
			"template_type":   config.TemplateType,
			"error":          err,
		}).Error("Failed to create organization config")
		return fmt.Errorf("failed to create organization config: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"organization_id": config.OrganizationID,
		"config_id":       config.ID,
		"template_type":   config.TemplateType,
	}).Info("Organization config created successfully")

	return nil
}

// Update updates an existing organization config
func (r *PostgresOrganizationConfigRepository) Update(ctx context.Context, config *entity.OrganizationConfig) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid organization config: %w", err)
	}

	// Check if the config exists
	var existingConfig entity.OrganizationConfig
	err := r.db.WithContext(ctx).Where("organization_id = ?", config.OrganizationID).First(&existingConfig).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("organization config not found")
		}
		return fmt.Errorf("failed to check existing config: %w", err)
	}

	// Update the config
	err = r.db.WithContext(ctx).Model(&entity.OrganizationConfig{}).
		Where("organization_id = ?", config.OrganizationID).
		Updates(config).Error
	
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"organization_id": config.OrganizationID,
			"config_id":       config.ID,
			"error":          err,
		}).Error("Failed to update organization config")
		return fmt.Errorf("failed to update organization config: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"organization_id": config.OrganizationID,
		"config_id":       config.ID,
	}).Info("Organization config updated successfully")

	return nil
}

// Delete deletes an organization config
func (r *PostgresOrganizationConfigRepository) Delete(ctx context.Context, organizationID string) error {
	if organizationID == "" {
		return errors.New("organization ID cannot be empty")
	}

	// Check if the config exists
	var config entity.OrganizationConfig
	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("organization config not found")
		}
		return fmt.Errorf("failed to check existing config: %w", err)
	}

	// Delete the config
	err = r.db.WithContext(ctx).Where("organization_id = ?", organizationID).Delete(&entity.OrganizationConfig{}).Error
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"organization_id": organizationID,
			"error":          err,
		}).Error("Failed to delete organization config")
		return fmt.Errorf("failed to delete organization config: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"organization_id": organizationID,
		"config_id":       config.ID,
	}).Info("Organization config deleted successfully")

	return nil
}

// List returns a paginated list of organization configs
func (r *PostgresOrganizationConfigRepository) List(ctx context.Context, limit, offset int) ([]*entity.OrganizationConfig, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 1000 {
		limit = 1000 // Maximum limit
	}
	if offset < 0 {
		offset = 0
	}

	var configs []*entity.OrganizationConfig
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"limit":  limit,
			"offset": offset,
			"error":  err,
		}).Error("Failed to list organization configs")
		return nil, fmt.Errorf("failed to list organization configs: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"count":  len(configs),
		"limit":  limit,
		"offset": offset,
	}).Debug("Organization configs listed successfully")

	return configs, nil
}

// GetByTemplateType returns all configs with a specific template type
func (r *PostgresOrganizationConfigRepository) GetByTemplateType(ctx context.Context, templateType entity.TemplateType) ([]*entity.OrganizationConfig, error) {
	var configs []*entity.OrganizationConfig
	err := r.db.WithContext(ctx).
		Where("template_type = ?", templateType).
		Order("created_at DESC").
		Find(&configs).Error
	
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"template_type": templateType,
			"error":        err,
		}).Error("Failed to get configs by template type")
		return nil, fmt.Errorf("failed to get configs by template type: %w", err)
	}

	return configs, nil
}

// GetActiveConfigs returns all active organization configs
func (r *PostgresOrganizationConfigRepository) GetActiveConfigs(ctx context.Context) ([]*entity.OrganizationConfig, error) {
	var configs []*entity.OrganizationConfig
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Find(&configs).Error
	
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to get active configs")
		return nil, fmt.Errorf("failed to get active configs: %w", err)
	}

	return configs, nil
}

// Count returns the total number of organization configs
func (r *PostgresOrganizationConfigRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&entity.OrganizationConfig{}).Count(&count).Error
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to count organization configs")
		return 0, fmt.Errorf("failed to count organization configs: %w", err)
	}

	return count, nil
}

// CountByTemplateType returns the count of configs by template type
func (r *PostgresOrganizationConfigRepository) CountByTemplateType(ctx context.Context) (map[entity.TemplateType]int64, error) {
	type TemplateCount struct {
		TemplateType entity.TemplateType `json:"template_type"`
		Count        int64               `json:"count"`
	}

	var results []TemplateCount
	err := r.db.WithContext(ctx).
		Model(&entity.OrganizationConfig{}).
		Select("template_type, COUNT(*) as count").
		Group("template_type").
		Find(&results).Error
	
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to count configs by template type")
		return nil, fmt.Errorf("failed to count configs by template type: %w", err)
	}

	counts := make(map[entity.TemplateType]int64)
	for _, result := range results {
		counts[result.TemplateType] = result.Count
	}

	return counts, nil
}

// GetConfigWithServiceUsage returns config with additional service usage statistics
func (r *PostgresOrganizationConfigRepository) GetConfigWithServiceUsage(ctx context.Context, organizationID string) (*OrganizationConfigWithUsage, error) {
	config, err := r.GetByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}

	// Get service usage statistics (this would be implemented based on actual usage tracking)
	// For now, we'll return the basic config with empty usage stats
	usage := &OrganizationConfigWithUsage{
		OrganizationConfig: *config,
		ServiceUsage:       make(map[string]ServiceUsageStats),
	}

	// TODO: Implement actual service usage tracking
	// This would involve querying usage metrics from various services

	return usage, nil
}

// OrganizationConfigWithUsage extends OrganizationConfig with usage statistics
type OrganizationConfigWithUsage struct {
	entity.OrganizationConfig
	ServiceUsage map[string]ServiceUsageStats `json:"service_usage"`
}

// ServiceUsageStats contains usage statistics for a service
type ServiceUsageStats struct {
	TotalRequests    int64   `json:"total_requests"`
	MonthlyRequests  int64   `json:"monthly_requests"`
	AverageLatency   float64 `json:"average_latency"`
	ErrorRate        float64 `json:"error_rate"`
	LastUsed         string  `json:"last_used"`
}

// AutoMigrate creates the database table if it doesn't exist
func (r *PostgresOrganizationConfigRepository) AutoMigrate() error {
	err := r.db.AutoMigrate(&entity.OrganizationConfig{})
	if err != nil {
		r.logger.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to auto migrate organization config table")
		return fmt.Errorf("failed to auto migrate organization config table: %w", err)
	}

	r.logger.Info("Organization config table auto migration completed")
	return nil
}