package saga

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SagaRepository handles persistence of saga instances
type SagaRepository struct {
	db     *gorm.DB
	logger *logrus.Logger
	config SagaRepositoryConfig
}

// SagaRepositoryConfig holds repository configuration
type SagaRepositoryConfig struct {
	TableName       string        `json:"table_name"`
	RetentionPeriod time.Duration `json:"retention_period"`
	EnableMetrics   bool          `json:"enable_metrics"`
}

// SagaFilter defines filtering options for saga queries
type SagaFilter struct {
	SagaType      string     `json:"saga_type,omitempty"`
	Status        SagaStatus `json:"status,omitempty"`
	TenantID      string     `json:"tenant_id,omitempty"`
	CorrelationID string     `json:"correlation_id,omitempty"`
	CreatedBy     string     `json:"created_by,omitempty"`
	StartedAfter  *time.Time `json:"started_after,omitempty"`
	StartedBefore *time.Time `json:"started_before,omitempty"`
}

// NewSagaRepository creates a new saga repository
func NewSagaRepository(db *gorm.DB, config SagaRepositoryConfig, logger *logrus.Logger) (*SagaRepository, error) {
	// Set default config values
	if config.TableName == "" {
		config.TableName = "saga_instances"
	}
	if config.RetentionPeriod == 0 {
		config.RetentionPeriod = 30 * 24 * time.Hour // 30 days
	}

	repo := &SagaRepository{
		db:     db,
		logger: logger,
		config: config,
	}

	// Auto-migrate table
	if err := repo.autoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate saga table: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"table_name":       config.TableName,
		"retention_period": config.RetentionPeriod,
	}).Info("Saga repository initialized successfully")

	return repo, nil
}

// Save saves a saga instance
func (sr *SagaRepository) Save(ctx context.Context, instance *SagaInstance) error {
	if err := sr.db.WithContext(ctx).Table(sr.config.TableName).Create(instance).Error; err != nil {
		sr.logger.WithFields(logrus.Fields{
			"saga_id":   instance.ID,
			"saga_type": instance.SagaType,
			"error":     err,
		}).Error("Failed to save saga instance")
		return fmt.Errorf("failed to save saga instance: %w", err)
	}

	sr.logger.WithFields(logrus.Fields{
		"saga_id":        instance.ID,
		"saga_type":      instance.SagaType,
		"status":         instance.Status,
		"correlation_id": instance.CorrelationID,
	}).Debug("Saga instance saved successfully")

	return nil
}

// Update updates a saga instance
func (sr *SagaRepository) Update(ctx context.Context, instance *SagaInstance) error {
	result := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("id = ?", instance.ID).
		Updates(instance)

	if result.Error != nil {
		sr.logger.WithFields(logrus.Fields{
			"saga_id": instance.ID,
			"error":   result.Error,
		}).Error("Failed to update saga instance")
		return fmt.Errorf("failed to update saga instance: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("saga instance not found: %s", instance.ID)
	}

	sr.logger.WithFields(logrus.Fields{
		"saga_id":   instance.ID,
		"status":    instance.Status,
		"current_step": instance.CurrentStep,
	}).Debug("Saga instance updated successfully")

	return nil
}

// GetByID retrieves a saga instance by ID
func (sr *SagaRepository) GetByID(ctx context.Context, sagaID string) (*SagaInstance, error) {
	var instance SagaInstance
	err := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("id = ?", sagaID).
		First(&instance).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("saga instance not found: %s", sagaID)
	}
	if err != nil {
		sr.logger.WithFields(logrus.Fields{
			"saga_id": sagaID,
			"error":   err,
		}).Error("Failed to get saga instance")
		return nil, fmt.Errorf("failed to get saga instance: %w", err)
	}

	sr.logger.WithFields(logrus.Fields{
		"saga_id":   sagaID,
		"saga_type": instance.SagaType,
		"status":    instance.Status,
	}).Debug("Saga instance retrieved successfully")

	return &instance, nil
}

// GetByCorrelationID retrieves saga instances by correlation ID
func (sr *SagaRepository) GetByCorrelationID(ctx context.Context, correlationID string) ([]*SagaInstance, error) {
	var instances []*SagaInstance
	err := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("correlation_id = ?", correlationID).
		Find(&instances).Error

	if err != nil {
		sr.logger.WithFields(logrus.Fields{
			"correlation_id": correlationID,
			"error":          err,
		}).Error("Failed to get saga instances by correlation ID")
		return nil, fmt.Errorf("failed to get saga instances by correlation ID: %w", err)
	}

	sr.logger.WithFields(logrus.Fields{
		"correlation_id": correlationID,
		"count":          len(instances),
	}).Debug("Saga instances retrieved by correlation ID")

	return instances, nil
}

// List retrieves saga instances with filtering and pagination
func (sr *SagaRepository) List(ctx context.Context, filter SagaFilter, limit, offset int) ([]*SagaInstance, int64, error) {
	query := sr.db.WithContext(ctx).Table(sr.config.TableName)

	// Apply filters
	query = sr.applyFilters(query, filter)

	// Count total records
	var total int64
	countQuery := query
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count saga instances: %w", err)
	}

	// Apply pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var instances []*SagaInstance
	if err := query.Order("started_at DESC").Find(&instances).Error; err != nil {
		sr.logger.WithError(err).Error("Failed to list saga instances")
		return nil, 0, fmt.Errorf("failed to list saga instances: %w", err)
	}

	sr.logger.WithFields(logrus.Fields{
		"count":  len(instances),
		"total":  total,
		"filter": filter,
		"limit":  limit,
		"offset": offset,
	}).Debug("Saga instances listed successfully")

	return instances, total, nil
}

// GetRunning retrieves all running saga instances
func (sr *SagaRepository) GetRunning(ctx context.Context) ([]*SagaInstance, error) {
	var instances []*SagaInstance
	err := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("status IN (?)", []SagaStatus{SagaStatusRunning, SagaStatusPending}).
		Find(&instances).Error

	if err != nil {
		sr.logger.WithError(err).Error("Failed to get running saga instances")
		return nil, fmt.Errorf("failed to get running saga instances: %w", err)
	}

	sr.logger.WithField("count", len(instances)).Debug("Running saga instances retrieved")

	return instances, nil
}

// GetStuck retrieves saga instances that appear to be stuck
func (sr *SagaRepository) GetStuck(ctx context.Context, timeout time.Duration) ([]*SagaInstance, error) {
	cutoff := time.Now().Add(-timeout)
	
	var instances []*SagaInstance
	err := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("status IN (?) AND updated_at < ?", 
			[]SagaStatus{SagaStatusRunning, SagaStatusCompensating}, cutoff).
		Find(&instances).Error

	if err != nil {
		sr.logger.WithError(err).Error("Failed to get stuck saga instances")
		return nil, fmt.Errorf("failed to get stuck saga instances: %w", err)
	}

	sr.logger.WithFields(logrus.Fields{
		"count":   len(instances),
		"timeout": timeout,
		"cutoff":  cutoff,
	}).Debug("Stuck saga instances retrieved")

	return instances, nil
}

// Delete deletes a saga instance
func (sr *SagaRepository) Delete(ctx context.Context, sagaID string) error {
	result := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("id = ?", sagaID).
		Delete(&SagaInstance{})

	if result.Error != nil {
		sr.logger.WithFields(logrus.Fields{
			"saga_id": sagaID,
			"error":   result.Error,
		}).Error("Failed to delete saga instance")
		return fmt.Errorf("failed to delete saga instance: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("saga instance not found: %s", sagaID)
	}

	sr.logger.WithField("saga_id", sagaID).Debug("Saga instance deleted successfully")

	return nil
}

// CleanupOld removes old saga instances based on retention period
func (sr *SagaRepository) CleanupOld(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-sr.config.RetentionPeriod)

	result := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Where("completed_at IS NOT NULL AND completed_at < ?", cutoff).
		Delete(&SagaInstance{})

	if result.Error != nil {
		sr.logger.WithError(result.Error).Error("Failed to cleanup old saga instances")
		return 0, fmt.Errorf("failed to cleanup old saga instances: %w", result.Error)
	}

	sr.logger.WithFields(logrus.Fields{
		"deleted_count":    result.RowsAffected,
		"retention_period": sr.config.RetentionPeriod,
		"cutoff":           cutoff,
	}).Info("Old saga instances cleaned up")

	return result.RowsAffected, nil
}

// GetStatistics returns saga statistics
func (sr *SagaRepository) GetStatistics(ctx context.Context, from, to time.Time) (*SagaStatistics, error) {
	stats := &SagaStatistics{
		FromDate: from,
		ToDate:   to,
	}

	// Count by status
	var statusCounts []struct {
		Status SagaStatus `gorm:"column:status"`
		Count  int64      `gorm:"column:count"`
	}

	err := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Select("status, COUNT(*) as count").
		Where("started_at BETWEEN ? AND ?", from, to).
		Group("status").
		Scan(&statusCounts).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get status counts: %w", err)
	}

	stats.StatusCounts = make(map[SagaStatus]int64)
	for _, sc := range statusCounts {
		stats.StatusCounts[sc.Status] = sc.Count
		stats.TotalCount += sc.Count
	}

	// Count by saga type
	var typeCounts []struct {
		SagaType string `gorm:"column:saga_type"`
		Count    int64  `gorm:"column:count"`
	}

	err = sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Select("saga_type, COUNT(*) as count").
		Where("started_at BETWEEN ? AND ?", from, to).
		Group("saga_type").
		Scan(&typeCounts).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get type counts: %w", err)
	}

	stats.TypeCounts = make(map[string]int64)
	for _, tc := range typeCounts {
		stats.TypeCounts[tc.SagaType] = tc.Count
	}

	// Average duration for completed sagas
	var avgDuration struct {
		AvgDuration float64 `gorm:"column:avg_duration"`
	}

	err = sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Select("AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) as avg_duration").
		Where("started_at BETWEEN ? AND ? AND completed_at IS NOT NULL", from, to).
		Scan(&avgDuration).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get average duration: %w", err)
	}

	stats.AverageDurationSeconds = avgDuration.AvgDuration

	sr.logger.WithFields(logrus.Fields{
		"from_date":        from,
		"to_date":          to,
		"total_count":      stats.TotalCount,
		"avg_duration":     stats.AverageDurationSeconds,
		"status_counts":    len(stats.StatusCounts),
		"type_counts":      len(stats.TypeCounts),
	}).Debug("Saga statistics retrieved")

	return stats, nil
}

// SagaStatistics represents saga statistics
type SagaStatistics struct {
	FromDate               time.Time            `json:"from_date"`
	ToDate                 time.Time            `json:"to_date"`
	TotalCount             int64                `json:"total_count"`
	StatusCounts           map[SagaStatus]int64 `json:"status_counts"`
	TypeCounts             map[string]int64     `json:"type_counts"`
	AverageDurationSeconds float64              `json:"average_duration_seconds"`
}

// applyFilters applies filtering to the query
func (sr *SagaRepository) applyFilters(query *gorm.DB, filter SagaFilter) *gorm.DB {
	if filter.SagaType != "" {
		query = query.Where("saga_type = ?", filter.SagaType)
	}

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}

	if filter.CorrelationID != "" {
		query = query.Where("correlation_id = ?", filter.CorrelationID)
	}

	if filter.CreatedBy != "" {
		query = query.Where("created_by = ?", filter.CreatedBy)
	}

	if filter.StartedAfter != nil {
		query = query.Where("started_at >= ?", *filter.StartedAfter)
	}

	if filter.StartedBefore != nil {
		query = query.Where("started_at <= ?", *filter.StartedBefore)
	}

	return query
}

// autoMigrate auto-migrates the saga table
func (sr *SagaRepository) autoMigrate() error {
	if err := sr.db.Table(sr.config.TableName).AutoMigrate(&SagaInstance{}); err != nil {
		return fmt.Errorf("failed to migrate saga table: %w", err)
	}

	// Create indexes for better performance
	sr.createIndexes()

	return nil
}

// createIndexes creates database indexes
func (sr *SagaRepository) createIndexes() {
	tableName := sr.config.TableName

	// Create composite indexes for common queries
	sr.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_status_type 
		ON %s (status, saga_type)`, tableName, tableName))
	
	sr.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_tenant_status 
		ON %s (tenant_id, status)`, tableName, tableName))
	
	sr.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_correlation 
		ON %s (correlation_id)`, tableName, tableName))
	
	sr.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_started_at 
		ON %s (started_at DESC)`, tableName, tableName))
	
	sr.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_updated_status 
		ON %s (updated_at, status) WHERE status IN ('running', 'compensating')`, tableName, tableName))

	sr.logger.Debug("Saga repository indexes created")
}

// HealthCheck performs a health check on the repository
func (sr *SagaRepository) HealthCheck(ctx context.Context) error {
	// Test database connectivity
	var count int64
	err := sr.db.WithContext(ctx).
		Table(sr.config.TableName).
		Count(&count).Error

	if err != nil {
		return fmt.Errorf("saga repository health check failed: %w", err)
	}

	sr.logger.WithField("saga_count", count).Debug("Saga repository health check passed")
	return nil
}

// Close closes the repository
func (sr *SagaRepository) Close() error {
	sr.logger.Info("Saga repository closed")
	return nil
}