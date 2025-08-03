package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// EventStore provides event sourcing capabilities
type EventStore struct {
	db     *gorm.DB
	logger *logrus.Logger
	config EventStoreConfig
}

// EventStoreConfig holds event store configuration
type EventStoreConfig struct {
	TableName       string        `json:"table_name"`
	SnapshotTable   string        `json:"snapshot_table"`
	SnapshotInterval int          `json:"snapshot_interval"`
	RetentionPeriod time.Duration `json:"retention_period"`
	EnableSnapshot  bool          `json:"enable_snapshot"`
}

// StoredEvent represents an event stored in the event store
type StoredEvent struct {
	ID              string                 `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AggregateID     string                 `gorm:"type:uuid;not null;index" json:"aggregate_id"`
	AggregateType   string                 `gorm:"type:varchar(255);not null;index" json:"aggregate_type"`
	EventType       string                 `gorm:"type:varchar(255);not null" json:"event_type"`
	EventData       string                 `gorm:"type:jsonb;not null" json:"event_data"`
	EventMetadata   string                 `gorm:"type:jsonb" json:"event_metadata"`
	Version         int                    `gorm:"not null;index" json:"version"`
	Timestamp       time.Time              `gorm:"not null;index" json:"timestamp"`
	CorrelationID   string                 `gorm:"type:uuid;index" json:"correlation_id"`
	CausationID     string                 `gorm:"type:uuid;index" json:"causation_id"`
	UserID          string                 `gorm:"type:uuid;index" json:"user_id"`
	TenantID        string                 `gorm:"type:uuid;index" json:"tenant_id"`
	CreatedAt       time.Time              `gorm:"autoCreateTime" json:"created_at"`
}

// Snapshot represents an aggregate snapshot
type Snapshot struct {
	ID            string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AggregateID   string    `gorm:"type:uuid;not null;index" json:"aggregate_id"`
	AggregateType string    `gorm:"type:varchar(255);not null" json:"aggregate_type"`
	Version       int       `gorm:"not null" json:"version"`
	Data          string    `gorm:"type:jsonb;not null" json:"data"`
	Timestamp     time.Time `gorm:"not null;index" json:"timestamp"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// Event represents a domain event
type Event interface {
	GetEventType() string
	GetAggregateID() string
	GetEventData() map[string]interface{}
	GetMetadata() map[string]interface{}
	GetVersion() int
	GetTimestamp() time.Time
}

// BaseEvent provides a base implementation for events
type BaseEvent struct {
	AggregateID   string                 `json:"aggregate_id"`
	EventType     string                 `json:"event_type"`
	Data          map[string]interface{} `json:"data"`
	Metadata      map[string]interface{} `json:"metadata"`
	Version       int                    `json:"version"`
	Timestamp     time.Time              `json:"timestamp"`
	CorrelationID string                 `json:"correlation_id"`
	CausationID   string                 `json:"causation_id"`
	UserID        string                 `json:"user_id"`
	TenantID      string                 `json:"tenant_id"`
}

// GetEventType returns the event type
func (e *BaseEvent) GetEventType() string {
	return e.EventType
}

// GetAggregateID returns the aggregate ID
func (e *BaseEvent) GetAggregateID() string {
	return e.AggregateID
}

// GetEventData returns the event data
func (e *BaseEvent) GetEventData() map[string]interface{} {
	return e.Data
}

// GetMetadata returns the event metadata
func (e *BaseEvent) GetMetadata() map[string]interface{} {
	return e.Metadata
}

// GetVersion returns the event version
func (e *BaseEvent) GetVersion() int {
	return e.Version
}

// GetTimestamp returns the event timestamp
func (e *BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

// EventStream represents a stream of events for an aggregate
type EventStream struct {
	AggregateID   string         `json:"aggregate_id"`
	AggregateType string         `json:"aggregate_type"`
	Version       int            `json:"version"`
	Events        []*StoredEvent `json:"events"`
	FromSnapshot  bool           `json:"from_snapshot"`
	SnapshotVersion int          `json:"snapshot_version"`
}

// NewEventStore creates a new event store
func NewEventStore(db *gorm.DB, config EventStoreConfig, logger *logrus.Logger) (*EventStore, error) {
	// Set default config values
	if config.TableName == "" {
		config.TableName = "events"
	}
	if config.SnapshotTable == "" {
		config.SnapshotTable = "snapshots"
	}
	if config.SnapshotInterval == 0 {
		config.SnapshotInterval = 100
	}
	if config.RetentionPeriod == 0 {
		config.RetentionPeriod = 365 * 24 * time.Hour // 1 year
	}

	store := &EventStore{
		db:     db,
		logger: logger,
		config: config,
	}

	// Auto-migrate tables
	if err := store.autoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate event store tables: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"table_name":       config.TableName,
		"snapshot_table":   config.SnapshotTable,
		"snapshot_enabled": config.EnableSnapshot,
	}).Info("Event store initialized successfully")

	return store, nil
}

// AppendEvents appends events to an aggregate stream
func (es *EventStore) AppendEvents(ctx context.Context, aggregateID, aggregateType string, expectedVersion int, events []Event) error {
	if len(events) == 0 {
		return fmt.Errorf("no events to append")
	}

	// Start transaction
	tx := es.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Check current version for optimistic concurrency control
	var currentVersion int
	err := tx.Table(es.config.TableName).
		Where("aggregate_id = ? AND aggregate_type = ?", aggregateID, aggregateType).
		Select("COALESCE(MAX(version), 0)").
		Scan(&currentVersion).Error

	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion != expectedVersion {
		tx.Rollback()
		return fmt.Errorf("concurrency conflict: expected version %d, current version %d", expectedVersion, currentVersion)
	}

	// Append events
	for i, event := range events {
		version := currentVersion + i + 1

		eventData, err := json.Marshal(event.GetEventData())
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to serialize event data: %w", err)
		}

		eventMetadata, err := json.Marshal(event.GetMetadata())
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to serialize event metadata: %w", err)
		}

		storedEvent := &StoredEvent{
			ID:            uuid.New().String(),
			AggregateID:   aggregateID,
			AggregateType: aggregateType,
			EventType:     event.GetEventType(),
			EventData:     string(eventData),
			EventMetadata: string(eventMetadata),
			Version:       version,
			Timestamp:     event.GetTimestamp(),
		}

		// Extract common fields from base event if available
		if baseEvent, ok := event.(*BaseEvent); ok {
			storedEvent.CorrelationID = baseEvent.CorrelationID
			storedEvent.CausationID = baseEvent.CausationID
			storedEvent.UserID = baseEvent.UserID
			storedEvent.TenantID = baseEvent.TenantID
		}

		if err := tx.Table(es.config.TableName).Create(storedEvent).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to append event: %w", err)
		}

		es.logger.WithFields(logrus.Fields{
			"aggregate_id":   aggregateID,
			"aggregate_type": aggregateType,
			"event_type":     event.GetEventType(),
			"version":        version,
		}).Debug("Event appended successfully")
	}

	// Check if snapshot should be created
	newVersion := currentVersion + len(events)
	if es.config.EnableSnapshot && newVersion%es.config.SnapshotInterval == 0 {
		es.logger.WithFields(logrus.Fields{
			"aggregate_id":   aggregateID,
			"aggregate_type": aggregateType,
			"version":        newVersion,
		}).Info("Snapshot interval reached, consider creating snapshot")
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit events: %w", err)
	}

	es.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregateID,
		"aggregate_type": aggregateType,
		"event_count":    len(events),
		"new_version":    newVersion,
	}).Info("Events appended successfully")

	return nil
}

// GetEventStream retrieves an event stream for an aggregate
func (es *EventStore) GetEventStream(ctx context.Context, aggregateID, aggregateType string, fromVersion int) (*EventStream, error) {
	var events []*StoredEvent
	var snapshotVersion int
	fromSnapshot := false

	// Check for snapshot if enabled
	if es.config.EnableSnapshot {
		snapshot, err := es.getLatestSnapshot(ctx, aggregateID, aggregateType)
		if err == nil && snapshot != nil && snapshot.Version >= fromVersion {
			fromVersion = snapshot.Version + 1
			snapshotVersion = snapshot.Version
			fromSnapshot = true
		}
	}

	// Get events from version
	err := es.db.WithContext(ctx).
		Table(es.config.TableName).
		Where("aggregate_id = ? AND aggregate_type = ? AND version >= ?", aggregateID, aggregateType, fromVersion).
		Order("version ASC").
		Find(&events).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get event stream: %w", err)
	}

	// Calculate current version
	currentVersion := fromVersion - 1
	if len(events) > 0 {
		currentVersion = events[len(events)-1].Version
	}

	stream := &EventStream{
		AggregateID:     aggregateID,
		AggregateType:   aggregateType,
		Version:         currentVersion,
		Events:          events,
		FromSnapshot:    fromSnapshot,
		SnapshotVersion: snapshotVersion,
	}

	es.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregateID,
		"aggregate_type": aggregateType,
		"event_count":    len(events),
		"version":        currentVersion,
		"from_snapshot":  fromSnapshot,
	}).Debug("Event stream retrieved successfully")

	return stream, nil
}

// CreateSnapshot creates a snapshot of an aggregate
func (es *EventStore) CreateSnapshot(ctx context.Context, aggregateID, aggregateType string, version int, data interface{}) error {
	if !es.config.EnableSnapshot {
		return fmt.Errorf("snapshots are not enabled")
	}

	// Serialize snapshot data
	snapshotData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize snapshot data: %w", err)
	}

	snapshot := &Snapshot{
		ID:            uuid.New().String(),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		Version:       version,
		Data:          string(snapshotData),
		Timestamp:     time.Now(),
	}

	if err := es.db.WithContext(ctx).Table(es.config.SnapshotTable).Create(snapshot).Error; err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Clean up old snapshots (keep only the latest few)
	go es.cleanupOldSnapshots(aggregateID, aggregateType)

	es.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregateID,
		"aggregate_type": aggregateType,
		"version":        version,
	}).Info("Snapshot created successfully")

	return nil
}

// GetLatestSnapshot retrieves the latest snapshot for an aggregate
func (es *EventStore) GetLatestSnapshot(ctx context.Context, aggregateID, aggregateType string) (*Snapshot, error) {
	return es.getLatestSnapshot(ctx, aggregateID, aggregateType)
}

// GetEventsByType retrieves events by event type
func (es *EventStore) GetEventsByType(ctx context.Context, eventType string, limit int, offset int) ([]*StoredEvent, error) {
	var events []*StoredEvent

	query := es.db.WithContext(ctx).
		Table(es.config.TableName).
		Where("event_type = ?", eventType).
		Order("timestamp DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", err)
	}

	return events, nil
}

// GetEventsByTimeRange retrieves events within a time range
func (es *EventStore) GetEventsByTimeRange(ctx context.Context, start, end time.Time, limit int) ([]*StoredEvent, error) {
	var events []*StoredEvent

	query := es.db.WithContext(ctx).
		Table(es.config.TableName).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Order("timestamp ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get events by time range: %w", err)
	}

	return events, nil
}

// GetEventsByCorrelationID retrieves events by correlation ID
func (es *EventStore) GetEventsByCorrelationID(ctx context.Context, correlationID string) ([]*StoredEvent, error) {
	var events []*StoredEvent

	err := es.db.WithContext(ctx).
		Table(es.config.TableName).
		Where("correlation_id = ?", correlationID).
		Order("timestamp ASC").
		Find(&events).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get events by correlation ID: %w", err)
	}

	return events, nil
}

// GetEventCount returns the total number of events
func (es *EventStore) GetEventCount(ctx context.Context) (int64, error) {
	var count int64
	err := es.db.WithContext(ctx).Table(es.config.TableName).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get event count: %w", err)
	}
	return count, nil
}

// GetAggregateVersion returns the current version of an aggregate
func (es *EventStore) GetAggregateVersion(ctx context.Context, aggregateID, aggregateType string) (int, error) {
	var version int
	err := es.db.WithContext(ctx).
		Table(es.config.TableName).
		Where("aggregate_id = ? AND aggregate_type = ?", aggregateID, aggregateType).
		Select("COALESCE(MAX(version), 0)").
		Scan(&version).Error

	if err != nil {
		return 0, fmt.Errorf("failed to get aggregate version: %w", err)
	}

	return version, nil
}

// ReplayEvents replays events to rebuild aggregate state
func (es *EventStore) ReplayEvents(ctx context.Context, aggregateID, aggregateType string, eventHandler func(*StoredEvent) error) error {
	stream, err := es.GetEventStream(ctx, aggregateID, aggregateType, 1)
	if err != nil {
		return fmt.Errorf("failed to get event stream for replay: %w", err)
	}

	for _, event := range stream.Events {
		if err := eventHandler(event); err != nil {
			return fmt.Errorf("event handler failed during replay: %w", err)
		}
	}

	es.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregateID,
		"aggregate_type": aggregateType,
		"events_replayed": len(stream.Events),
	}).Info("Events replayed successfully")

	return nil
}

// Private helper methods

func (es *EventStore) autoMigrate() error {
	// Migrate events table
	if err := es.db.Table(es.config.TableName).AutoMigrate(&StoredEvent{}); err != nil {
		return fmt.Errorf("failed to migrate events table: %w", err)
	}

	// Migrate snapshots table if enabled
	if es.config.EnableSnapshot {
		if err := es.db.Table(es.config.SnapshotTable).AutoMigrate(&Snapshot{}); err != nil {
			return fmt.Errorf("failed to migrate snapshots table: %w", err)
		}
	}

	// Create indexes for better performance
	es.createIndexes()

	return nil
}

func (es *EventStore) createIndexes() {
	// Create composite indexes for common queries
	es.db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_aggregate_version ON %s (aggregate_id, aggregate_type, version)", 
		es.config.TableName, es.config.TableName))
	
	es.db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_timestamp ON %s (timestamp)", 
		es.config.TableName, es.config.TableName))
	
	es.db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_event_type ON %s (event_type)", 
		es.config.TableName, es.config.TableName))

	if es.config.EnableSnapshot {
		es.db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_aggregate ON %s (aggregate_id, aggregate_type, version DESC)", 
			es.config.SnapshotTable, es.config.SnapshotTable))
	}
}

func (es *EventStore) getLatestSnapshot(ctx context.Context, aggregateID, aggregateType string) (*Snapshot, error) {
	var snapshot Snapshot
	err := es.db.WithContext(ctx).
		Table(es.config.SnapshotTable).
		Where("aggregate_id = ? AND aggregate_type = ?", aggregateID, aggregateType).
		Order("version DESC").
		First(&snapshot).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest snapshot: %w", err)
	}

	return &snapshot, nil
}

func (es *EventStore) cleanupOldSnapshots(aggregateID, aggregateType string) {
	// Keep only the latest 5 snapshots
	var snapshots []Snapshot
	es.db.Table(es.config.SnapshotTable).
		Where("aggregate_id = ? AND aggregate_type = ?", aggregateID, aggregateType).
		Order("version DESC").
		Offset(5).
		Find(&snapshots)

	for _, snapshot := range snapshots {
		es.db.Table(es.config.SnapshotTable).Delete(&snapshot)
	}
}

// Close closes the event store
func (es *EventStore) Close() error {
	es.logger.Info("Event store closed")
	return nil
}