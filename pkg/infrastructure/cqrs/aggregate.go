package cqrs

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Aggregate represents a domain aggregate root
type Aggregate interface {
	GetID() string
	GetType() string
	GetVersion() int
	GetUncommittedEvents() []Event
	MarkEventsAsCommitted()
	LoadFromHistory(events []*StoredEvent) error
	ApplyEvent(event Event) error
}

// AggregateRepository handles aggregate persistence and retrieval
type AggregateRepository struct {
	eventStore EventStore
	logger     *logrus.Logger
	config     AggregateConfig
}

// AggregateConfig holds aggregate repository configuration
type AggregateConfig struct {
	SnapshotFrequency int  `json:"snapshot_frequency"`
	EnableSnapshots   bool `json:"enable_snapshots"`
	MaxEventCount     int  `json:"max_event_count"`
}

// BaseAggregate provides a base implementation for aggregates
type BaseAggregate struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	Version           int       `json:"version"`
	UncommittedEvents []Event   `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	TenantID          string    `json:"tenant_id"`
}

// GetID returns the aggregate ID
func (ba *BaseAggregate) GetID() string {
	return ba.ID
}

// GetType returns the aggregate type
func (ba *BaseAggregate) GetType() string {
	return ba.Type
}

// GetVersion returns the aggregate version
func (ba *BaseAggregate) GetVersion() int {
	return ba.Version
}

// GetUncommittedEvents returns uncommitted events
func (ba *BaseAggregate) GetUncommittedEvents() []Event {
	return ba.UncommittedEvents
}

// MarkEventsAsCommitted marks all uncommitted events as committed
func (ba *BaseAggregate) MarkEventsAsCommitted() {
	ba.UncommittedEvents = make([]Event, 0)
}

// AddEvent adds an event to the uncommitted events list
func (ba *BaseAggregate) AddEvent(event Event) {
	ba.UncommittedEvents = append(ba.UncommittedEvents, event)
	ba.Version++
	ba.UpdatedAt = time.Now()
}

// LoadFromHistory loads the aggregate from historical events
func (ba *BaseAggregate) LoadFromHistory(events []*StoredEvent) error {
	for _, storedEvent := range events {
		// Create domain event from stored event
		event, err := ba.createDomainEvent(storedEvent)
		if err != nil {
			return fmt.Errorf("failed to create domain event: %w", err)
		}

		// Apply the event
		if err := ba.ApplyEvent(event); err != nil {
			return fmt.Errorf("failed to apply historical event: %w", err)
		}

		ba.Version = storedEvent.Version
	}

	// Clear uncommitted events as these are historical
	ba.UncommittedEvents = make([]Event, 0)

	return nil
}

// ApplyEvent applies an event to the aggregate (to be overridden by concrete aggregates)
func (ba *BaseAggregate) ApplyEvent(event Event) error {
	// Base implementation - concrete aggregates should override this
	ba.UpdatedAt = time.Now()
	return nil
}

// createDomainEvent creates a domain event from a stored event
func (ba *BaseAggregate) createDomainEvent(storedEvent *StoredEvent) (Event, error) {
	// This would typically use a factory or registry to create the appropriate event type
	// For now, return a base event
	return &BaseEvent{
		AggregateID:   storedEvent.AggregateID,
		EventType:     storedEvent.EventType,
		Data:          map[string]interface{}{}, // Would parse from JSON
		Version:       storedEvent.Version,
		Timestamp:     storedEvent.Timestamp,
		CorrelationID: storedEvent.CorrelationID,
		CausationID:   storedEvent.CausationID,
		UserID:        storedEvent.UserID,
		TenantID:      storedEvent.TenantID,
	}, nil
}

// NewAggregateRepository creates a new aggregate repository
func NewAggregateRepository(eventStore EventStore, config AggregateConfig, logger *logrus.Logger) *AggregateRepository {
	// Set default config values
	if config.SnapshotFrequency == 0 {
		config.SnapshotFrequency = 100
	}
	if config.MaxEventCount == 0 {
		config.MaxEventCount = 1000
	}

	return &AggregateRepository{
		eventStore: eventStore,
		logger:     logger,
		config:     config,
	}
}

// Save saves an aggregate by persisting its uncommitted events
func (ar *AggregateRepository) Save(ctx context.Context, aggregate Aggregate) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		ar.logger.WithFields(logrus.Fields{
			"aggregate_id":   aggregate.GetID(),
			"aggregate_type": aggregate.GetType(),
		}).Debug("No uncommitted events to save")
		return nil
	}

	// Calculate expected version (current version minus uncommitted events)
	expectedVersion := aggregate.GetVersion() - len(uncommittedEvents)

	// Append events to event store
	err := ar.eventStore.AppendEvents(
		ctx,
		aggregate.GetID(),
		aggregate.GetType(),
		expectedVersion,
		uncommittedEvents,
	)
	if err != nil {
		ar.logger.WithFields(logrus.Fields{
			"aggregate_id":     aggregate.GetID(),
			"aggregate_type":   aggregate.GetType(),
			"expected_version": expectedVersion,
			"events_count":     len(uncommittedEvents),
			"error":           err,
		}).Error("Failed to save aggregate events")
		return fmt.Errorf("failed to save aggregate: %w", err)
	}

	// Mark events as committed
	aggregate.MarkEventsAsCommitted()

	// Create snapshot if needed
	if ar.config.EnableSnapshots && aggregate.GetVersion()%ar.config.SnapshotFrequency == 0 {
		if err := ar.createSnapshot(ctx, aggregate); err != nil {
			ar.logger.WithFields(logrus.Fields{
				"aggregate_id":   aggregate.GetID(),
				"aggregate_type": aggregate.GetType(),
				"version":        aggregate.GetVersion(),
				"error":          err,
			}).Warn("Failed to create snapshot, continuing")
		}
	}

	ar.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregate.GetID(),
		"aggregate_type": aggregate.GetType(),
		"version":        aggregate.GetVersion(),
		"events_saved":   len(uncommittedEvents),
	}).Info("Aggregate saved successfully")

	return nil
}

// Load loads an aggregate from the event store
func (ar *AggregateRepository) Load(ctx context.Context, aggregateID, aggregateType string, aggregate Aggregate) error {
	// Try to load from snapshot first if enabled
	var fromVersion int = 1
	if ar.config.EnableSnapshots {
		snapshot, err := ar.eventStore.GetLatestSnapshot(ctx, aggregateID, aggregateType)
		if err == nil && snapshot != nil {
			// Load aggregate state from snapshot
			if err := ar.loadFromSnapshot(aggregate, snapshot); err != nil {
				ar.logger.WithFields(logrus.Fields{
					"aggregate_id":   aggregateID,
					"aggregate_type": aggregateType,
					"snapshot_version": snapshot.Version,
					"error":          err,
				}).Warn("Failed to load from snapshot, loading from events")
			} else {
				fromVersion = snapshot.Version + 1
			}
		}
	}

	// Load events from the determined version
	eventStream, err := ar.eventStore.GetEventStream(ctx, aggregateID, aggregateType, fromVersion)
	if err != nil {
		return fmt.Errorf("failed to get event stream: %w", err)
	}

	if len(eventStream.Events) == 0 && fromVersion == 1 {
		return fmt.Errorf("aggregate not found: %s", aggregateID)
	}

	// Load aggregate from historical events
	if len(eventStream.Events) > 0 {
		if err := aggregate.LoadFromHistory(eventStream.Events); err != nil {
			return fmt.Errorf("failed to load aggregate from history: %w", err)
		}
	}

	ar.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregateID,
		"aggregate_type": aggregateType,
		"version":        aggregate.GetVersion(),
		"events_loaded":  len(eventStream.Events),
		"from_snapshot":  eventStream.FromSnapshot,
	}).Debug("Aggregate loaded successfully")

	return nil
}

// Exists checks if an aggregate exists
func (ar *AggregateRepository) Exists(ctx context.Context, aggregateID, aggregateType string) (bool, error) {
	version, err := ar.eventStore.GetAggregateVersion(ctx, aggregateID, aggregateType)
	if err != nil {
		return false, fmt.Errorf("failed to check aggregate existence: %w", err)
	}

	return version > 0, nil
}

// Delete marks an aggregate as deleted (typically by adding a deletion event)
func (ar *AggregateRepository) Delete(ctx context.Context, aggregate Aggregate, deletionEvent Event) error {
	// Add the deletion event
	if deletionEvent == nil {
		return fmt.Errorf("deletion event is required")
	}

	// Apply the deletion event to the aggregate
	if err := aggregate.ApplyEvent(deletionEvent); err != nil {
		return fmt.Errorf("failed to apply deletion event: %w", err)
	}

	// Save the aggregate with the deletion event
	if err := ar.Save(ctx, aggregate); err != nil {
		return fmt.Errorf("failed to save aggregate with deletion event: %w", err)
	}

	ar.logger.WithFields(logrus.Fields{
		"aggregate_id":   aggregate.GetID(),
		"aggregate_type": aggregate.GetType(),
		"version":        aggregate.GetVersion(),
	}).Info("Aggregate marked as deleted")

	return nil
}

// GetVersion returns the current version of an aggregate
func (ar *AggregateRepository) GetVersion(ctx context.Context, aggregateID, aggregateType string) (int, error) {
	return ar.eventStore.GetAggregateVersion(ctx, aggregateID, aggregateType)
}

// createSnapshot creates a snapshot of an aggregate
func (ar *AggregateRepository) createSnapshot(ctx context.Context, aggregate Aggregate) error {
	// In a real implementation, this would serialize the aggregate state
	// For now, we'll create a simple snapshot representation
	snapshotData := map[string]interface{}{
		"id":         aggregate.GetID(),
		"type":       aggregate.GetType(),
		"version":    aggregate.GetVersion(),
		"created_at": time.Now(),
	}

	return ar.eventStore.CreateSnapshot(
		ctx,
		aggregate.GetID(),
		aggregate.GetType(),
		aggregate.GetVersion(),
		snapshotData,
	)
}

// loadFromSnapshot loads aggregate state from a snapshot
func (ar *AggregateRepository) loadFromSnapshot(aggregate Aggregate, snapshot *Snapshot) error {
	// In a real implementation, this would deserialize the snapshot data
	// and restore the aggregate state
	// For now, this is a placeholder implementation
	
	if baseAggregate, ok := aggregate.(*BaseAggregate); ok {
		baseAggregate.Version = snapshot.Version
		baseAggregate.UpdatedAt = snapshot.Timestamp
	}

	ar.logger.WithFields(logrus.Fields{
		"aggregate_id":     aggregate.GetID(),
		"aggregate_type":   aggregate.GetType(),
		"snapshot_version": snapshot.Version,
	}).Debug("Aggregate loaded from snapshot")

	return nil
}

// AggregateFactory creates aggregate instances
type AggregateFactory struct {
	types  map[string]reflect.Type
	logger *logrus.Logger
}

// NewAggregateFactory creates a new aggregate factory
func NewAggregateFactory(logger *logrus.Logger) *AggregateFactory {
	return &AggregateFactory{
		types:  make(map[string]reflect.Type),
		logger: logger,
	}
}

// RegisterType registers an aggregate type
func (af *AggregateFactory) RegisterType(aggregateType string, aggregateStruct interface{}) {
	t := reflect.TypeOf(aggregateStruct)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	af.types[aggregateType] = t

	af.logger.WithFields(logrus.Fields{
		"aggregate_type": aggregateType,
		"struct_type":    t.String(),
	}).Debug("Aggregate type registered")
}

// Create creates a new aggregate instance
func (af *AggregateFactory) Create(aggregateType string) (Aggregate, error) {
	t, exists := af.types[aggregateType]
	if !exists {
		return nil, fmt.Errorf("unknown aggregate type: %s", aggregateType)
	}

	// Create new instance
	instance := reflect.New(t).Interface()
	
	aggregate, ok := instance.(Aggregate)
	if !ok {
		return nil, fmt.Errorf("type %s does not implement Aggregate interface", aggregateType)
	}

	return aggregate, nil
}

// CreateWithID creates a new aggregate instance with a specific ID
func (af *AggregateFactory) CreateWithID(aggregateType, id string) (Aggregate, error) {
	aggregate, err := af.Create(aggregateType)
	if err != nil {
		return nil, err
	}

	// Set the ID if it's a BaseAggregate
	if baseAggregate, ok := aggregate.(*BaseAggregate); ok {
		baseAggregate.ID = id
		baseAggregate.Type = aggregateType
		baseAggregate.CreatedAt = time.Now()
		baseAggregate.UpdatedAt = time.Now()
		baseAggregate.UncommittedEvents = make([]Event, 0)
	}

	return aggregate, nil
}

// CreateNew creates a new aggregate with a generated UUID
func (af *AggregateFactory) CreateNew(aggregateType string) (Aggregate, error) {
	id := uuid.New().String()
	return af.CreateWithID(aggregateType, id)
}

// GetRegisteredTypes returns all registered aggregate types
func (af *AggregateFactory) GetRegisteredTypes() []string {
	types := make([]string, 0, len(af.types))
	for aggregateType := range af.types {
		types = append(types, aggregateType)
	}
	return types
}

// Example aggregate implementations

// UserAggregate example aggregate
type UserAggregate struct {
	*BaseAggregate
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	IsActive bool      `json:"is_active"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// NewUserAggregate creates a new user aggregate
func NewUserAggregate(id, email, name string) *UserAggregate {
	return &UserAggregate{
		BaseAggregate: &BaseAggregate{
			ID:                id,
			Type:              "user",
			Version:           0,
			UncommittedEvents: make([]Event, 0),
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
		Email:    email,
		Name:     name,
		IsActive: true,
	}
}

// ApplyEvent applies events to the user aggregate
func (ua *UserAggregate) ApplyEvent(event Event) error {
	switch event.GetEventType() {
	case "user.created":
		// Handle user creation
		data := event.GetEventData()
		if email, ok := data["email"].(string); ok {
			ua.Email = email
		}
		if name, ok := data["name"].(string); ok {
			ua.Name = name
		}
		ua.IsActive = true
	case "user.email_changed":
		data := event.GetEventData()
		if email, ok := data["new_email"].(string); ok {
			ua.Email = email
		}
	case "user.deactivated":
		ua.IsActive = false
	case "user.deleted":
		now := time.Now()
		ua.DeletedAt = &now
		ua.IsActive = false
	}

	// Call base implementation
	return ua.BaseAggregate.ApplyEvent(event)
}

// ChangeEmail changes the user's email
func (ua *UserAggregate) ChangeEmail(newEmail string, userID string) {
	if ua.Email == newEmail {
		return // No change needed
	}

	event := &BaseEvent{
		AggregateID: ua.ID,
		EventType:   "user.email_changed",
		Data: map[string]interface{}{
			"old_email": ua.Email,
			"new_email": newEmail,
		},
		Metadata: map[string]interface{}{
			"changed_by": userID,
		},
		Version:   ua.Version + 1,
		Timestamp: time.Now(),
		UserID:    userID,
		TenantID:  ua.TenantID,
	}

	ua.AddEvent(event)
	ua.ApplyEvent(event)
}

// Deactivate deactivates the user
func (ua *UserAggregate) Deactivate(userID string, reason string) {
	if !ua.IsActive {
		return // Already deactivated
	}

	event := &BaseEvent{
		AggregateID: ua.ID,
		EventType:   "user.deactivated",
		Data: map[string]interface{}{
			"reason": reason,
		},
		Metadata: map[string]interface{}{
			"deactivated_by": userID,
		},
		Version:   ua.Version + 1,
		Timestamp: time.Now(),
		UserID:    userID,
		TenantID:  ua.TenantID,
	}

	ua.AddEvent(event)
	ua.ApplyEvent(event)
}