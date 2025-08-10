package cqrs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event represents a domain event
type Event interface {
	GetEventType() string
	GetAggregateID() string
	GetAggregateType() string
	GetEventData() map[string]interface{}
	GetMetadata() map[string]interface{}
	GetTimestamp() time.Time
	GetVersion() int
}

// BaseEvent provides a basic implementation of Event
type BaseEvent struct {
	EventType     string                 `json:"event_type"`
	AggregateID   string                 `json:"aggregate_id"`
	AggregateType string                 `json:"aggregate_type"`
	EventData     map[string]interface{} `json:"event_data"`
	Metadata      map[string]interface{} `json:"metadata"`
	Timestamp     time.Time              `json:"timestamp"`
	Version       int                    `json:"version"`
	Data          map[string]interface{} `json:"data"`
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

// GetAggregateType returns the aggregate type
func (e *BaseEvent) GetAggregateType() string {
	return e.AggregateType
}

// GetEventData returns the event data
func (e *BaseEvent) GetEventData() map[string]interface{} {
	return e.EventData
}

// GetMetadata returns the event metadata
func (e *BaseEvent) GetMetadata() map[string]interface{} {
	return e.Metadata
}

// GetTimestamp returns the event timestamp
func (e *BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

// GetVersion returns the event version
func (e *BaseEvent) GetVersion() int {
	return e.Version
}

// StoredEvent represents an event as stored in the event store
type StoredEvent struct {
	ID            string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	EventType     string    `gorm:"not null;index" json:"event_type"`
	AggregateID   string    `gorm:"not null;index" json:"aggregate_id"`
	AggregateType string    `gorm:"not null;index" json:"aggregate_type"`
	EventData     string    `gorm:"type:jsonb" json:"event_data"`
	Metadata      string    `gorm:"type:jsonb" json:"metadata"`
	Version       int       `gorm:"not null;index" json:"version"`
	Timestamp     time.Time `gorm:"not null;index" json:"timestamp"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	CorrelationID string    `gorm:"index" json:"correlation_id"`
	CausationID   string    `gorm:"index" json:"causation_id"`
	UserID        string    `gorm:"index" json:"user_id"`
	TenantID      string    `gorm:"index" json:"tenant_id"`
}

// ToEvent converts a StoredEvent to an Event
func (se *StoredEvent) ToEvent() (Event, error) {
	var eventData map[string]interface{}
	if err := json.Unmarshal([]byte(se.EventData), &eventData); err != nil {
		return nil, err
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(se.Metadata), &metadata); err != nil {
		return nil, err
	}

	return &BaseEvent{
		EventType:     se.EventType,
		AggregateID:   se.AggregateID,
		AggregateType: se.AggregateType,
		EventData:     eventData,
		Metadata:      metadata,
		Timestamp:     se.Timestamp,
		Version:       se.Version,
	}, nil
}

// FromEvent creates a StoredEvent from an Event
func FromEvent(event Event) (*StoredEvent, error) {
	eventDataBytes, err := json.Marshal(event.GetEventData())
	if err != nil {
		return nil, err
	}

	metadataBytes, err := json.Marshal(event.GetMetadata())
	if err != nil {
		return nil, err
	}

	return &StoredEvent{
		ID:            uuid.New().String(),
		EventType:     event.GetEventType(),
		AggregateID:   event.GetAggregateID(),
		AggregateType: event.GetAggregateType(),
		EventData:     string(eventDataBytes),
		Metadata:      string(metadataBytes),
		Version:       event.GetVersion(),
		Timestamp:     event.GetTimestamp(),
		CreatedAt:     time.Now(),
	}, nil
}

// EventStream represents a stream of events
type EventStream struct {
	AggregateID   string         `json:"aggregate_id"`
	AggregateType string         `json:"aggregate_type"`
	Events        []*StoredEvent `json:"events"`
	Version       int            `json:"version"`
	Timestamp     time.Time      `json:"timestamp"`
}

// GetEvents returns the events as Event interfaces
func (es *EventStream) GetEvents() ([]Event, error) {
	events := make([]Event, len(es.Events))
	for i, storedEvent := range es.Events {
		event, err := storedEvent.ToEvent()
		if err != nil {
			return nil, err
		}
		events[i] = event
	}
	return events, nil
}

// NewEvent creates a new BaseEvent
func NewEvent(eventType, aggregateID, aggregateType string, data map[string]interface{}, version int) Event {
	return &BaseEvent{
		EventType:     eventType,
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventData:     data,
		Metadata:      make(map[string]interface{}),
		Timestamp:     time.Now(),
		Version:       version,
	}
}

// WithMetadata adds metadata to an event
func (e *BaseEvent) WithMetadata(key string, value interface{}) *BaseEvent {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// Snapshot represents an aggregate snapshot
type Snapshot struct {
	ID            string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AggregateID   string    `gorm:"not null;index" json:"aggregate_id"`
	AggregateType string    `gorm:"not null;index" json:"aggregate_type"`
	Version       int       `gorm:"not null" json:"version"`
	Data          string    `gorm:"type:jsonb" json:"data"`
	Timestamp     time.Time `gorm:"not null" json:"timestamp"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}