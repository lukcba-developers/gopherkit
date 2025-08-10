package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// SagaManager manages distributed transactions using the Saga pattern
type SagaManager struct {
	repository *SagaRepository
	orchestrator Orchestrator
	compensator Compensator
	logger     *logrus.Logger
	config     SagaConfig
	registry   *SagaDefinitionRegistry
	mu         sync.RWMutex
}

// SagaConfig holds saga manager configuration
type SagaConfig struct {
	TimeoutDuration     time.Duration `json:"timeout_duration"`
	RetryAttempts       int           `json:"retry_attempts"`
	RetryDelay          time.Duration `json:"retry_delay"`
	CompensationTimeout time.Duration `json:"compensation_timeout"`
	EnableMetrics       bool          `json:"enable_metrics"`
	BatchSize           int           `json:"batch_size"`
}

// SagaDefinition defines the structure of a saga
type SagaDefinition struct {
	Name  string      `json:"name"`
	Steps []SagaStep  `json:"steps"`
}

// SagaStep represents a single step in a saga
type SagaStep struct {
	Name           string                 `json:"name"`
	Action         string                 `json:"action"`
	Compensation   string                 `json:"compensation"`
	Parameters     map[string]interface{} `json:"parameters"`
	Timeout        time.Duration          `json:"timeout"`
	RetryPolicy    RetryPolicy            `json:"retry_policy"`
	DependsOn      []string               `json:"depends_on"`
	Critical       bool                   `json:"critical"`
}

// RetryPolicy defines retry behavior for saga steps
type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	Delay       time.Duration `json:"delay"`
	Backoff     string        `json:"backoff"` // linear, exponential
}

// SagaInstance represents an instance of a running saga
type SagaInstance struct {
	ID                string                 `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	SagaType          string                 `gorm:"type:varchar(255);not null;index" json:"saga_type"`
	Status            SagaStatus             `gorm:"type:varchar(50);not null" json:"status"`
	CurrentStep       int                    `gorm:"not null;default:0" json:"current_step"`
	Data              string                 `gorm:"type:jsonb" json:"data"`
	CompletedSteps    string                 `gorm:"type:jsonb" json:"completed_steps"`
	FailedSteps       string                 `gorm:"type:jsonb" json:"failed_steps"`
	CompensatedSteps  string                 `gorm:"type:jsonb" json:"compensated_steps"`
	StartedAt         time.Time              `gorm:"not null" json:"started_at"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	LastError         string                 `json:"last_error,omitempty"`
	TenantID          string                 `gorm:"type:uuid;index" json:"tenant_id"`
	CorrelationID     string                 `gorm:"type:uuid;index" json:"correlation_id"`
	CreatedBy         string                 `gorm:"type:uuid" json:"created_by"`
	UpdatedAt         time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
}

// SagaStatus represents the status of a saga instance
type SagaStatus string

const (
	SagaStatusPending      SagaStatus = "pending"
	SagaStatusRunning      SagaStatus = "running"
	SagaStatusCompleted    SagaStatus = "completed"
	SagaStatusFailed       SagaStatus = "failed"
	SagaStatusCompensating SagaStatus = "compensating"
	SagaStatusCompensated  SagaStatus = "compensated"
	SagaStatusTimeout      SagaStatus = "timeout"
	SagaStatusCancelled    SagaStatus = "cancelled"
)

// SagaStepResult represents the result of a saga step execution
type SagaStepResult struct {
	StepName    string                 `json:"step_name"`
	Success     bool                   `json:"success"`
	Error       error                  `json:"error,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Compensated bool                   `json:"compensated"`
	Duration    time.Duration          `json:"duration"`
	Attempt     int                    `json:"attempt"`
}

// Orchestrator defines the interface for saga orchestration
type Orchestrator interface {
	ExecuteStep(ctx context.Context, instance *SagaInstance, step *SagaStep) (*SagaStepResult, error)
	GetStepHandler(action string) (StepHandler, error)
}

// Compensator defines the interface for saga compensation
type Compensator interface {
	CompensateStep(ctx context.Context, instance *SagaInstance, step *SagaStep) (*SagaStepResult, error)
	GetCompensationHandler(compensation string) (CompensationHandler, error)
}

// StepHandler defines the interface for step execution
type StepHandler interface {
	Execute(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)
	GetName() string
}

// CompensationHandler defines the interface for compensation execution
type CompensationHandler interface {
	Compensate(ctx context.Context, data map[string]interface{}) error
	GetName() string
}

// SagaDefinitionRegistry manages saga definitions
type SagaDefinitionRegistry struct {
	definitions map[string]*SagaDefinition
	mu          sync.RWMutex
}

// NewSagaDefinitionRegistry creates a new saga definition registry
func NewSagaDefinitionRegistry() *SagaDefinitionRegistry {
	return &SagaDefinitionRegistry{
		definitions: make(map[string]*SagaDefinition),
	}
}

// Register registers a saga definition
func (sdr *SagaDefinitionRegistry) Register(definition *SagaDefinition) error {
	sdr.mu.Lock()
	defer sdr.mu.Unlock()

	if definition.Name == "" {
		return fmt.Errorf("saga definition name cannot be empty")
	}

	sdr.definitions[definition.Name] = definition
	return nil
}

// Get retrieves a saga definition by name
func (sdr *SagaDefinitionRegistry) Get(name string) (*SagaDefinition, error) {
	sdr.mu.RLock()
	defer sdr.mu.RUnlock()

	definition, exists := sdr.definitions[name]
	if !exists {
		return nil, fmt.Errorf("saga definition not found: %s", name)
	}

	return definition, nil
}

// GetAll returns all registered saga definitions
func (sdr *SagaDefinitionRegistry) GetAll() map[string]*SagaDefinition {
	sdr.mu.RLock()
	defer sdr.mu.RUnlock()

	definitions := make(map[string]*SagaDefinition)
	for name, def := range sdr.definitions {
		definitions[name] = def
	}

	return definitions
}

// NewSagaManager creates a new saga manager
func NewSagaManager(
	repository *SagaRepository,
	orchestrator Orchestrator,
	compensator Compensator,
	config SagaConfig,
	logger *logrus.Logger,
) *SagaManager {
	// Set default config values
	if config.TimeoutDuration == 0 {
		config.TimeoutDuration = 30 * time.Minute
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 5 * time.Second
	}
	if config.CompensationTimeout == 0 {
		config.CompensationTimeout = 10 * time.Minute
	}
	if config.BatchSize == 0 {
		config.BatchSize = 10
	}

	return &SagaManager{
		repository:   repository,
		orchestrator: orchestrator,
		compensator:  compensator,
		logger:       logger,
		config:       config,
		registry:     NewSagaDefinitionRegistry(),
	}
}

// StartSaga starts a new saga instance
func (sm *SagaManager) StartSaga(ctx context.Context, sagaType string, data map[string]interface{}, tenantID, userID string) (*SagaInstance, error) {
	// Get saga definition
	definition, err := sm.registry.Get(sagaType)
	if err != nil {
		return nil, fmt.Errorf("failed to get saga definition: %w", err)
	}

	// Create saga instance
	instance := &SagaInstance{
		ID:            uuid.New().String(),
		SagaType:      sagaType,
		Status:        SagaStatusPending,
		CurrentStep:   0,
		StartedAt:     time.Now(),
		TenantID:      tenantID,
		CorrelationID: uuid.New().String(),
		CreatedBy:     userID,
	}

	// Serialize data
	if len(data) > 0 {
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize saga data: %w", err)
		}
		instance.Data = string(dataBytes)
	}

	// Initialize step tracking
	instance.CompletedSteps = "[]"
	instance.FailedSteps = "[]"
	instance.CompensatedSteps = "[]"

	// Save instance
	if err := sm.repository.Save(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to save saga instance: %w", err)
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":        instance.ID,
		"saga_type":      sagaType,
		"tenant_id":      tenantID,
		"correlation_id": instance.CorrelationID,
		"steps_count":    len(definition.Steps),
	}).Info("Saga started")

	// Start execution asynchronously
	go sm.executeSaga(context.Background(), instance)

	return instance, nil
}

// ContinueSaga continues an existing saga instance
func (sm *SagaManager) ContinueSaga(ctx context.Context, sagaID string) error {
	instance, err := sm.repository.GetByID(ctx, sagaID)
	if err != nil {
		return fmt.Errorf("failed to get saga instance: %w", err)
	}

	if instance.Status != SagaStatusRunning && instance.Status != SagaStatusFailed {
		return fmt.Errorf("saga cannot be continued, current status: %s", instance.Status)
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":   sagaID,
		"saga_type": instance.SagaType,
		"status":    instance.Status,
	}).Info("Continuing saga execution")

	// Continue execution asynchronously
	go sm.executeSaga(context.Background(), instance)

	return nil
}

// CancelSaga cancels a running saga and triggers compensation
func (sm *SagaManager) CancelSaga(ctx context.Context, sagaID string, reason string) error {
	instance, err := sm.repository.GetByID(ctx, sagaID)
	if err != nil {
		return fmt.Errorf("failed to get saga instance: %w", err)
	}

	if instance.Status == SagaStatusCompleted || instance.Status == SagaStatusCancelled {
		return fmt.Errorf("saga cannot be cancelled, current status: %s", instance.Status)
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":   sagaID,
		"saga_type": instance.SagaType,
		"reason":    reason,
	}).Info("Cancelling saga")

	// Update status
	instance.Status = SagaStatusCancelled
	instance.LastError = reason
	if err := sm.repository.Update(ctx, instance); err != nil {
		return fmt.Errorf("failed to update saga status: %w", err)
	}

	// Start compensation asynchronously
	go sm.compensateSaga(context.Background(), instance)

	return nil
}

// GetSagaStatus returns the status of a saga instance
func (sm *SagaManager) GetSagaStatus(ctx context.Context, sagaID string) (*SagaInstance, error) {
	return sm.repository.GetByID(ctx, sagaID)
}

// ListSagas lists saga instances with optional filtering
func (sm *SagaManager) ListSagas(ctx context.Context, filter SagaFilter, limit, offset int) ([]*SagaInstance, int64, error) {
	return sm.repository.List(ctx, filter, limit, offset)
}

// RegisterDefinition registers a saga definition
func (sm *SagaManager) RegisterDefinition(definition *SagaDefinition) error {
	if err := sm.validateDefinition(definition); err != nil {
		return fmt.Errorf("invalid saga definition: %w", err)
	}

	if err := sm.registry.Register(definition); err != nil {
		return fmt.Errorf("failed to register saga definition: %w", err)
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_name":   definition.Name,
		"steps_count": len(definition.Steps),
	}).Info("Saga definition registered")

	return nil
}

// executeSaga executes a saga instance
func (sm *SagaManager) executeSaga(ctx context.Context, instance *SagaInstance) {
	// Set timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, sm.config.TimeoutDuration)
	defer cancel()

	// Get saga definition
	definition, err := sm.registry.Get(instance.SagaType)
	if err != nil {
		sm.handleSagaError(ctx, instance, fmt.Errorf("failed to get saga definition: %w", err))
		return
	}

	// Update status to running
	instance.Status = SagaStatusRunning
	if err := sm.repository.Update(ctx, instance); err != nil {
		sm.logger.WithError(err).Error("Failed to update saga status to running")
		return
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":      instance.ID,
		"saga_type":    instance.SagaType,
		"current_step": instance.CurrentStep,
		"total_steps":  len(definition.Steps),
	}).Info("Starting saga execution")

	// Execute steps
	for i := instance.CurrentStep; i < len(definition.Steps); i++ {
		step := &definition.Steps[i]

		// Check if step dependencies are satisfied
		if !sm.checkStepDependencies(instance, step) {
			sm.handleSagaError(ctx, instance, fmt.Errorf("step dependencies not satisfied: %s", step.Name))
			return
		}

		// Execute step with retry
		result := sm.executeStepWithRetry(timeoutCtx, instance, step)
		
		// Update step tracking
		if err := sm.updateStepTracking(ctx, instance, step, result); err != nil {
			sm.logger.WithError(err).Error("Failed to update step tracking")
		}

		if !result.Success {
			sm.logger.WithFields(logrus.Fields{
				"saga_id":   instance.ID,
				"step_name": step.Name,
				"error":     result.Error,
				"attempt":   result.Attempt,
			}).Error("Saga step failed")

			if step.Critical {
				// Critical step failed, start compensation
				sm.handleSagaError(ctx, instance, result.Error)
				return
			} else {
				// Non-critical step failed, continue to next step
				sm.logger.WithField("step_name", step.Name).Warn("Non-critical step failed, continuing")
			}
		}

		// Update current step
		instance.CurrentStep = i + 1
		if err := sm.repository.Update(ctx, instance); err != nil {
			sm.logger.WithError(err).Error("Failed to update saga current step")
		}

		sm.logger.WithFields(logrus.Fields{
			"saga_id":      instance.ID,
			"step_name":    step.Name,
			"step_result":  result.Success,
			"current_step": instance.CurrentStep,
		}).Debug("Saga step completed")
	}

	// All steps completed successfully
	instance.Status = SagaStatusCompleted
	now := time.Now()
	instance.CompletedAt = &now

	if err := sm.repository.Update(ctx, instance); err != nil {
		sm.logger.WithError(err).Error("Failed to update saga completion status")
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":      instance.ID,
		"saga_type":    instance.SagaType,
		"duration":     time.Since(instance.StartedAt),
		"steps_count":  len(definition.Steps),
	}).Info("Saga completed successfully")
}

// executeStepWithRetry executes a step with retry logic
func (sm *SagaManager) executeStepWithRetry(ctx context.Context, instance *SagaInstance, step *SagaStep) *SagaStepResult {
	maxAttempts := step.RetryPolicy.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = sm.config.RetryAttempts
	}

	delay := step.RetryPolicy.Delay
	if delay == 0 {
		delay = sm.config.RetryDelay
	}

	var lastResult *SagaStepResult

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return &SagaStepResult{
					StepName: step.Name,
					Success:  false,
					Error:    ctx.Err(),
					Attempt:  attempt,
				}
			case <-time.After(sm.calculateRetryDelay(delay, attempt, step.RetryPolicy.Backoff)):
			}
		}

		startTime := time.Now()
		result, err := sm.orchestrator.ExecuteStep(ctx, instance, step)
		duration := time.Since(startTime)

		if result == nil {
			result = &SagaStepResult{
				StepName: step.Name,
				Success:  false,
				Error:    err,
				Duration: duration,
				Attempt:  attempt,
			}
		} else {
			result.Duration = duration
			result.Attempt = attempt
		}

		lastResult = result

		if result.Success {
			sm.logger.WithFields(logrus.Fields{
				"saga_id":   instance.ID,
				"step_name": step.Name,
				"attempt":   attempt,
				"duration":  duration,
			}).Debug("Saga step executed successfully")
			return result
		}

		sm.logger.WithFields(logrus.Fields{
			"saga_id":   instance.ID,
			"step_name": step.Name,
			"attempt":   attempt,
			"error":     result.Error,
			"duration":  duration,
		}).Warn("Saga step execution failed, retrying")
	}

	return lastResult
}

// compensateSaga executes compensation for completed steps
func (sm *SagaManager) compensateSaga(ctx context.Context, instance *SagaInstance) {
	timeoutCtx, cancel := context.WithTimeout(ctx, sm.config.CompensationTimeout)
	defer cancel()

	instance.Status = SagaStatusCompensating
	if err := sm.repository.Update(ctx, instance); err != nil {
		sm.logger.WithError(err).Error("Failed to update saga status to compensating")
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":   instance.ID,
		"saga_type": instance.SagaType,
	}).Info("Starting saga compensation")

	// Get saga definition
	definition, err := sm.registry.Get(instance.SagaType)
	if err != nil {
		sm.logger.WithError(err).Error("Failed to get saga definition for compensation")
		return
	}

	// Get completed steps
	var completedSteps []string
	if instance.CompletedSteps != "" && instance.CompletedSteps != "[]" {
		if err := json.Unmarshal([]byte(instance.CompletedSteps), &completedSteps); err != nil {
			sm.logger.WithError(err).Error("Failed to parse completed steps")
			return
		}
	}

	// Compensate in reverse order
	compensatedSteps := make([]string, 0)
	for i := len(completedSteps) - 1; i >= 0; i-- {
		stepName := completedSteps[i]
		
		// Find step definition
		var step *SagaStep
		for j := range definition.Steps {
			if definition.Steps[j].Name == stepName {
				step = &definition.Steps[j]
				break
			}
		}

		if step == nil {
			sm.logger.WithField("step_name", stepName).Warn("Step definition not found for compensation")
			continue
		}

		if step.Compensation == "" {
			sm.logger.WithField("step_name", stepName).Debug("No compensation defined for step")
			continue
		}

		// Execute compensation
		result, err := sm.compensator.CompensateStep(timeoutCtx, instance, step)
		if err != nil || (result != nil && !result.Success) {
			sm.logger.WithFields(logrus.Fields{
				"saga_id":   instance.ID,
				"step_name": stepName,
				"error":     err,
			}).Error("Failed to compensate step")
			continue
		}

		compensatedSteps = append(compensatedSteps, stepName)
		sm.logger.WithField("step_name", stepName).Debug("Step compensated successfully")
	}

	// Update compensated steps
	compensatedBytes, _ := json.Marshal(compensatedSteps)
	instance.CompensatedSteps = string(compensatedBytes)
	instance.Status = SagaStatusCompensated
	now := time.Now()
	instance.CompletedAt = &now

	if err := sm.repository.Update(ctx, instance); err != nil {
		sm.logger.WithError(err).Error("Failed to update saga compensation status")
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":           instance.ID,
		"compensated_count": len(compensatedSteps),
		"duration":          time.Since(instance.StartedAt),
	}).Info("Saga compensation completed")
}

// Helper methods

func (sm *SagaManager) handleSagaError(ctx context.Context, instance *SagaInstance, err error) {
	instance.Status = SagaStatusFailed
	instance.LastError = err.Error()
	now := time.Now()
	instance.CompletedAt = &now

	if updateErr := sm.repository.Update(ctx, instance); updateErr != nil {
		sm.logger.WithError(updateErr).Error("Failed to update saga error status")
	}

	sm.logger.WithFields(logrus.Fields{
		"saga_id":   instance.ID,
		"saga_type": instance.SagaType,
		"error":     err,
	}).Error("Saga failed")

	// Start compensation
	go sm.compensateSaga(context.Background(), instance)
}

func (sm *SagaManager) checkStepDependencies(instance *SagaInstance, step *SagaStep) bool {
	if len(step.DependsOn) == 0 {
		return true
	}

	// Parse completed steps
	var completedSteps []string
	if instance.CompletedSteps != "" && instance.CompletedSteps != "[]" {
		if err := json.Unmarshal([]byte(instance.CompletedSteps), &completedSteps); err != nil {
			return false
		}
	}

	// Check if all dependencies are satisfied
	for _, dependency := range step.DependsOn {
		found := false
		for _, completed := range completedSteps {
			if completed == dependency {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (sm *SagaManager) updateStepTracking(ctx context.Context, instance *SagaInstance, step *SagaStep, result *SagaStepResult) error {
	if result.Success {
		// Add to completed steps
		var completedSteps []string
		if instance.CompletedSteps != "" && instance.CompletedSteps != "[]" {
			if err := json.Unmarshal([]byte(instance.CompletedSteps), &completedSteps); err != nil {
				return err
			}
		}
		completedSteps = append(completedSteps, step.Name)
		completedBytes, err := json.Marshal(completedSteps)
		if err != nil {
			return err
		}
		instance.CompletedSteps = string(completedBytes)
	} else {
		// Add to failed steps
		var failedSteps []string
		if instance.FailedSteps != "" && instance.FailedSteps != "[]" {
			if err := json.Unmarshal([]byte(instance.FailedSteps), &failedSteps); err != nil {
				return err
			}
		}
		failedSteps = append(failedSteps, step.Name)
		failedBytes, err := json.Marshal(failedSteps)
		if err != nil {
			return err
		}
		instance.FailedSteps = string(failedBytes)
	}

	return nil
}

func (sm *SagaManager) calculateRetryDelay(baseDelay time.Duration, attempt int, backoff string) time.Duration {
	switch backoff {
	case "exponential":
		return baseDelay * time.Duration(1<<uint(attempt-1))
	case "linear":
		return baseDelay * time.Duration(attempt)
	default:
		return baseDelay
	}
}

func (sm *SagaManager) validateDefinition(definition *SagaDefinition) error {
	if definition.Name == "" {
		return fmt.Errorf("saga name is required")
	}

	if len(definition.Steps) == 0 {
		return fmt.Errorf("saga must have at least one step")
	}

	stepNames := make(map[string]bool)
	for _, step := range definition.Steps {
		if step.Name == "" {
			return fmt.Errorf("step name is required")
		}

		if stepNames[step.Name] {
			return fmt.Errorf("duplicate step name: %s", step.Name)
		}
		stepNames[step.Name] = true

		if step.Action == "" {
			return fmt.Errorf("step action is required for step: %s", step.Name)
		}

		// Validate dependencies
		for _, dep := range step.DependsOn {
			if !stepNames[dep] {
				return fmt.Errorf("invalid dependency '%s' for step '%s'", dep, step.Name)
			}
		}
	}

	return nil
}

// HealthCheck performs a health check on the saga manager
func (sm *SagaManager) HealthCheck(ctx context.Context) error {
	// Check repository connectivity
	if err := sm.repository.HealthCheck(ctx); err != nil {
		return fmt.Errorf("saga repository health check failed: %w", err)
	}

	sm.logger.Debug("Saga manager health check passed")
	return nil
}

// Close closes the saga manager
func (sm *SagaManager) Close() error {
	sm.logger.Info("Saga manager closed")
	return nil
}