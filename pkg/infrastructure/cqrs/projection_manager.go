package cqrs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ProjectionManager manages event projections to read models
type ProjectionManager struct {
	projections map[string]Projection
	eventStore  EventStore
	readStore   *ReadModelStore
	logger      *logrus.Logger
	config      ProjectionConfig
	mu          sync.RWMutex
	running     bool
	stopChan    chan struct{}
}

// ProjectionConfig holds projection manager configuration
type ProjectionConfig struct {
	BatchSize       int           `json:"batch_size"`
	PollInterval    time.Duration `json:"poll_interval"`
	ErrorRetryCount int           `json:"error_retry_count"`
	ErrorRetryDelay time.Duration `json:"error_retry_delay"`
	EnableMetrics   bool          `json:"enable_metrics"`
}

// Projection defines the interface for event projections
type Projection interface {
	GetName() string
	GetEventTypes() []string
	Project(ctx context.Context, event *StoredEvent) (*ProjectionResult, error)
	Reset(ctx context.Context) error
	GetPosition() int64
	SetPosition(position int64)
}

// ProjectionResult represents the result of a projection
type ProjectionResult struct {
	ReadModels []ReadModel `json:"read_models"`
	Success    bool        `json:"success"`
	Message    string      `json:"message,omitempty"`
	Position   int64       `json:"position"`
}

// BaseProjection provides a base implementation for projections
type BaseProjection struct {
	Name        string   `json:"name"`
	EventTypes  []string `json:"event_types"`
	Position    int64    `json:"position"`
	Logger      *logrus.Logger
	ReadStore   *ReadModelStore
}

// GetName returns the projection name
func (bp *BaseProjection) GetName() string {
	return bp.Name
}

// GetEventTypes returns the event types this projection handles
func (bp *BaseProjection) GetEventTypes() []string {
	return bp.EventTypes
}

// GetPosition returns the current projection position
func (bp *BaseProjection) GetPosition() int64 {
	return bp.Position
}

// SetPosition sets the projection position
func (bp *BaseProjection) SetPosition(position int64) {
	bp.Position = position
}

// ProjectionState represents the state of a projection
type ProjectionState struct {
	Name         string    `json:"name"`
	Position     int64     `json:"position"`
	LastUpdated  time.Time `json:"last_updated"`
	EventsCount  int64     `json:"events_count"`
	ErrorsCount  int64     `json:"errors_count"`
	IsRunning    bool      `json:"is_running"`
	LastError    string    `json:"last_error,omitempty"`
}

// EventStore interface is now defined in types.go

// NewProjectionManager creates a new projection manager
func NewProjectionManager(eventStore EventStore, readStore *ReadModelStore, config ProjectionConfig, logger *logrus.Logger) *ProjectionManager {
	// Set default config values
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.PollInterval == 0 {
		config.PollInterval = 5 * time.Second
	}
	if config.ErrorRetryCount == 0 {
		config.ErrorRetryCount = 3
	}
	if config.ErrorRetryDelay == 0 {
		config.ErrorRetryDelay = 1 * time.Second
	}

	return &ProjectionManager{
		projections: make(map[string]Projection),
		eventStore:  eventStore,
		readStore:   readStore,
		logger:      logger,
		config:      config,
		stopChan:    make(chan struct{}),
	}
}

// RegisterProjection registers a projection
func (pm *ProjectionManager) RegisterProjection(projection Projection) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	name := projection.GetName()
	if name == "" {
		return fmt.Errorf("projection name cannot be empty")
	}

	if _, exists := pm.projections[name]; exists {
		return fmt.Errorf("projection with name %s already registered", name)
	}

	pm.projections[name] = projection

	pm.logger.WithFields(logrus.Fields{
		"projection":   name,
		"event_types":  projection.GetEventTypes(),
	}).Info("Projection registered successfully")

	return nil
}

// UnregisterProjection removes a projection
func (pm *ProjectionManager) UnregisterProjection(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.projections[name]; !exists {
		return fmt.Errorf("projection %s not found", name)
	}

	delete(pm.projections, name)

	pm.logger.WithField("projection", name).Info("Projection unregistered")

	return nil
}

// Start starts the projection manager
func (pm *ProjectionManager) Start(ctx context.Context) error {
	pm.mu.Lock()
	if pm.running {
		pm.mu.Unlock()
		return fmt.Errorf("projection manager is already running")
	}
	pm.running = true
	pm.mu.Unlock()

	pm.logger.Info("Starting projection manager")

	// Start projection workers for each registered projection
	for name, projection := range pm.projections {
		go pm.runProjection(ctx, name, projection)
	}

	pm.logger.WithField("projection_count", len(pm.projections)).Info("Projection manager started")

	return nil
}

// Stop stops the projection manager
func (pm *ProjectionManager) Stop() error {
	pm.mu.Lock()
	if !pm.running {
		pm.mu.Unlock()
		return fmt.Errorf("projection manager is not running")
	}
	pm.running = false
	pm.mu.Unlock()

	pm.logger.Info("Stopping projection manager")

	close(pm.stopChan)

	pm.logger.Info("Projection manager stopped")

	return nil
}

// ProjectEvents manually projects events for a specific projection
func (pm *ProjectionManager) ProjectEvents(ctx context.Context, projectionName string, fromPosition int64) error {
	pm.mu.RLock()
	projection, exists := pm.projections[projectionName]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("projection %s not found", projectionName)
	}

	pm.logger.WithFields(logrus.Fields{
		"projection":    projectionName,
		"from_position": fromPosition,
	}).Info("Starting manual event projection")

	return pm.processEventsForProjection(ctx, projection, fromPosition)
}

// ResetProjection resets a projection to start from the beginning
func (pm *ProjectionManager) ResetProjection(ctx context.Context, projectionName string) error {
	pm.mu.RLock()
	projection, exists := pm.projections[projectionName]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("projection %s not found", projectionName)
	}

	// Reset the projection
	if err := projection.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset projection %s: %w", projectionName, err)
	}

	// Set position to 0
	projection.SetPosition(0)

	pm.logger.WithField("projection", projectionName).Info("Projection reset successfully")

	return nil
}

// GetProjectionState returns the state of a projection
func (pm *ProjectionManager) GetProjectionState(projectionName string) (*ProjectionState, error) {
	pm.mu.RLock()
	projection, exists := pm.projections[projectionName]
	pm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("projection %s not found", projectionName)
	}

	return &ProjectionState{
		Name:        projection.GetName(),
		Position:    projection.GetPosition(),
		LastUpdated: time.Now(),
		IsRunning:   pm.running,
	}, nil
}

// GetAllProjectionStates returns the state of all projections
func (pm *ProjectionManager) GetAllProjectionStates() ([]*ProjectionState, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	states := make([]*ProjectionState, 0, len(pm.projections))
	for _, projection := range pm.projections {
		state := &ProjectionState{
			Name:        projection.GetName(),
			Position:    projection.GetPosition(),
			LastUpdated: time.Now(),
			IsRunning:   pm.running,
		}
		states = append(states, state)
	}

	return states, nil
}

// runProjection runs a projection in a loop
func (pm *ProjectionManager) runProjection(ctx context.Context, name string, projection Projection) {
	pm.logger.WithField("projection", name).Info("Starting projection worker")

	ticker := time.NewTicker(pm.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			pm.logger.WithField("projection", name).Info("Projection worker stopped due to context cancellation")
			return
		case <-pm.stopChan:
			pm.logger.WithField("projection", name).Info("Projection worker stopped")
			return
		case <-ticker.C:
			if err := pm.processEventsForProjection(ctx, projection, projection.GetPosition()); err != nil {
				pm.logger.WithFields(logrus.Fields{
					"projection": name,
					"error":      err,
				}).Error("Failed to process events for projection")
			}
		}
	}
}

// processEventsForProjection processes events for a specific projection
func (pm *ProjectionManager) processEventsForProjection(ctx context.Context, projection Projection, fromPosition int64) error {
	eventTypes := projection.GetEventTypes()
	processedCount := 0

	for _, eventType := range eventTypes {
		events, err := pm.eventStore.GetEventsByType(ctx, eventType, pm.config.BatchSize, int(fromPosition))
		if err != nil {
			return fmt.Errorf("failed to get events for type %s: %w", eventType, err)
		}

		for _, event := range events {
			// Skip events we've already processed
			if int64(event.Version) <= fromPosition {
				continue
			}

			// Project the event
			result, err := pm.projectEventWithRetry(ctx, projection, event)
			if err != nil {
				pm.logger.WithFields(logrus.Fields{
					"projection":  projection.GetName(),
					"event_type":  event.EventType,
					"event_id":    event.ID,
					"error":       err,
				}).Error("Failed to project event after retries")
				continue
			}

			// Save read models if projection succeeded
			if result.Success && len(result.ReadModels) > 0 {
				for _, readModel := range result.ReadModels {
					if err := pm.readStore.Save(ctx, readModel); err != nil {
						pm.logger.WithFields(logrus.Fields{
							"projection":    projection.GetName(),
							"read_model_id": readModel.GetID(),
							"error":         err,
						}).Error("Failed to save read model")
					}
				}
			}

			// Update projection position
			projection.SetPosition(int64(event.Version))
			processedCount++
		}
	}

	if processedCount > 0 {
		pm.logger.WithFields(logrus.Fields{
			"projection":      projection.GetName(),
			"processed_count": processedCount,
			"new_position":    projection.GetPosition(),
		}).Debug("Events processed for projection")
	}

	return nil
}

// projectEventWithRetry projects an event with retry logic
func (pm *ProjectionManager) projectEventWithRetry(ctx context.Context, projection Projection, event *StoredEvent) (*ProjectionResult, error) {
	var lastErr error

	for attempt := 0; attempt <= pm.config.ErrorRetryCount; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(pm.config.ErrorRetryDelay * time.Duration(attempt))
		}

		result, err := projection.Project(ctx, event)
		if err == nil {
			return result, nil
		}

		lastErr = err
		pm.logger.WithFields(logrus.Fields{
			"projection": projection.GetName(),
			"event_id":   event.ID,
			"attempt":    attempt + 1,
			"error":      err,
		}).Warn("Projection attempt failed, retrying")
	}

	return nil, fmt.Errorf("projection failed after %d attempts: %w", pm.config.ErrorRetryCount+1, lastErr)
}

// RebuildProjection rebuilds a projection from scratch
func (pm *ProjectionManager) RebuildProjection(ctx context.Context, projectionName string) error {
	pm.logger.WithField("projection", projectionName).Info("Starting projection rebuild")

	// Reset the projection first
	if err := pm.ResetProjection(ctx, projectionName); err != nil {
		return fmt.Errorf("failed to reset projection for rebuild: %w", err)
	}

	// Project all events from the beginning
	if err := pm.ProjectEvents(ctx, projectionName, 0); err != nil {
		return fmt.Errorf("failed to project events during rebuild: %w", err)
	}

	pm.logger.WithField("projection", projectionName).Info("Projection rebuild completed")

	return nil
}

// GetProjectionCount returns the number of registered projections
func (pm *ProjectionManager) GetProjectionCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.projections)
}

// IsRunning returns whether the projection manager is running
func (pm *ProjectionManager) IsRunning() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.running
}

// HealthCheck performs a health check on the projection manager
func (pm *ProjectionManager) HealthCheck(ctx context.Context) error {
	pm.mu.RLock()
	projectionCount := len(pm.projections)
	running := pm.running
	pm.mu.RUnlock()

	if projectionCount == 0 {
		return fmt.Errorf("no projections registered")
	}

	if !running {
		return fmt.Errorf("projection manager is not running")
	}

	pm.logger.WithField("projection_count", projectionCount).Debug("Projection manager health check passed")
	return nil
}

// Close closes the projection manager
func (pm *ProjectionManager) Close() error {
	if pm.running {
		pm.Stop()
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear projections
	pm.projections = make(map[string]Projection)

	pm.logger.Info("Projection manager closed")
	return nil
}