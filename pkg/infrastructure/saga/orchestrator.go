package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// DefaultOrchestrator provides the default implementation of saga orchestration
type DefaultOrchestrator struct {
	handlers map[string]StepHandler
	logger   *logrus.Logger
	mu       sync.RWMutex
}

// DefaultCompensator provides the default implementation of saga compensation
type DefaultCompensator struct {
	handlers map[string]CompensationHandler
	logger   *logrus.Logger
	mu       sync.RWMutex
}

// NewDefaultOrchestrator creates a new default orchestrator
func NewDefaultOrchestrator(logger *logrus.Logger) *DefaultOrchestrator {
	return &DefaultOrchestrator{
		handlers: make(map[string]StepHandler),
		logger:   logger,
	}
}

// RegisterStepHandler registers a step handler
func (do *DefaultOrchestrator) RegisterStepHandler(handler StepHandler) error {
	do.mu.Lock()
	defer do.mu.Unlock()

	name := handler.GetName()
	if name == "" {
		return fmt.Errorf("step handler name cannot be empty")
	}

	if _, exists := do.handlers[name]; exists {
		return fmt.Errorf("step handler with name %s already registered", name)
	}

	do.handlers[name] = handler

	do.logger.WithFields(logrus.Fields{
		"handler_name": name,
		"handler_type": reflect.TypeOf(handler).String(),
	}).Info("Step handler registered")

	return nil
}

// ExecuteStep executes a saga step
func (do *DefaultOrchestrator) ExecuteStep(ctx context.Context, instance *SagaInstance, step *SagaStep) (*SagaStepResult, error) {
	handler, err := do.GetStepHandler(step.Action)
	if err != nil {
		return nil, fmt.Errorf("failed to get step handler: %w", err)
	}

	// Parse saga data
	sagaData := make(map[string]interface{})
	if instance.Data != "" {
		if err := json.Unmarshal([]byte(instance.Data), &sagaData); err != nil {
			return nil, fmt.Errorf("failed to parse saga data: %w", err)
		}
	}

	// Merge step parameters with saga data
	stepData := make(map[string]interface{})
	for k, v := range sagaData {
		stepData[k] = v
	}
	for k, v := range step.Parameters {
		stepData[k] = v
	}

	// Add metadata
	stepData["saga_id"] = instance.ID
	stepData["saga_type"] = instance.SagaType
	stepData["step_name"] = step.Name
	stepData["tenant_id"] = instance.TenantID
	stepData["correlation_id"] = instance.CorrelationID
	stepData["created_by"] = instance.CreatedBy

	do.logger.WithFields(logrus.Fields{
		"saga_id":   instance.ID,
		"step_name": step.Name,
		"action":    step.Action,
		"handler":   handler.GetName(),
	}).Debug("Executing saga step")

	// Set timeout if specified
	stepCtx := ctx
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// Execute the step
	startTime := time.Now()
	result, err := handler.Execute(stepCtx, stepData)
	duration := time.Since(startTime)

	if err != nil {
		do.logger.WithFields(logrus.Fields{
			"saga_id":   instance.ID,
			"step_name": step.Name,
			"action":    step.Action,
			"error":     err,
			"duration":  duration,
		}).Error("Saga step execution failed")

		return &SagaStepResult{
			StepName: step.Name,
			Success:  false,
			Error:    err,
			Duration: duration,
		}, nil
	}

	// Update saga data with step result
	if len(result) > 0 {
		// Merge result back into saga data
		for k, v := range result {
			sagaData[k] = v
		}

		// Update instance data
		updatedData, err := json.Marshal(sagaData)
		if err != nil {
			do.logger.WithError(err).Warn("Failed to update saga data")
		} else {
			instance.Data = string(updatedData)
		}
	}

	do.logger.WithFields(logrus.Fields{
		"saga_id":   instance.ID,
		"step_name": step.Name,
		"action":    step.Action,
		"duration":  duration,
	}).Info("Saga step executed successfully")

	return &SagaStepResult{
		StepName: step.Name,
		Success:  true,
		Data:     result,
		Duration: duration,
	}, nil
}

// GetStepHandler retrieves a step handler by name
func (do *DefaultOrchestrator) GetStepHandler(action string) (StepHandler, error) {
	do.mu.RLock()
	defer do.mu.RUnlock()

	handler, exists := do.handlers[action]
	if !exists {
		return nil, fmt.Errorf("step handler not found: %s", action)
	}

	return handler, nil
}

// GetRegisteredHandlers returns all registered step handlers
func (do *DefaultOrchestrator) GetRegisteredHandlers() []string {
	do.mu.RLock()
	defer do.mu.RUnlock()

	handlers := make([]string, 0, len(do.handlers))
	for name := range do.handlers {
		handlers = append(handlers, name)
	}

	return handlers
}

// NewDefaultCompensator creates a new default compensator
func NewDefaultCompensator(logger *logrus.Logger) *DefaultCompensator {
	return &DefaultCompensator{
		handlers: make(map[string]CompensationHandler),
		logger:   logger,
	}
}

// RegisterCompensationHandler registers a compensation handler
func (dc *DefaultCompensator) RegisterCompensationHandler(handler CompensationHandler) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	name := handler.GetName()
	if name == "" {
		return fmt.Errorf("compensation handler name cannot be empty")
	}

	if _, exists := dc.handlers[name]; exists {
		return fmt.Errorf("compensation handler with name %s already registered", name)
	}

	dc.handlers[name] = handler

	dc.logger.WithFields(logrus.Fields{
		"handler_name": name,
		"handler_type": reflect.TypeOf(handler).String(),
	}).Info("Compensation handler registered")

	return nil
}

// CompensateStep executes compensation for a saga step
func (dc *DefaultCompensator) CompensateStep(ctx context.Context, instance *SagaInstance, step *SagaStep) (*SagaStepResult, error) {
	if step.Compensation == "" {
		return &SagaStepResult{
			StepName: step.Name,
			Success:  true,
			Data:     map[string]interface{}{"message": "No compensation required"},
		}, nil
	}

	handler, err := dc.GetCompensationHandler(step.Compensation)
	if err != nil {
		return nil, fmt.Errorf("failed to get compensation handler: %w", err)
	}

	// Parse saga data
	sagaData := make(map[string]interface{})
	if instance.Data != "" {
		if err := json.Unmarshal([]byte(instance.Data), &sagaData); err != nil {
			return nil, fmt.Errorf("failed to parse saga data: %w", err)
		}
	}

	// Merge step parameters with saga data
	compensationData := make(map[string]interface{})
	for k, v := range sagaData {
		compensationData[k] = v
	}
	for k, v := range step.Parameters {
		compensationData[k] = v
	}

	// Add metadata
	compensationData["saga_id"] = instance.ID
	compensationData["saga_type"] = instance.SagaType
	compensationData["step_name"] = step.Name
	compensationData["tenant_id"] = instance.TenantID
	compensationData["correlation_id"] = instance.CorrelationID
	compensationData["created_by"] = instance.CreatedBy

	dc.logger.WithFields(logrus.Fields{
		"saga_id":      instance.ID,
		"step_name":    step.Name,
		"compensation": step.Compensation,
		"handler":      handler.GetName(),
	}).Debug("Executing saga step compensation")

	// Execute compensation
	startTime := time.Now()
	err = handler.Compensate(ctx, compensationData)
	duration := time.Since(startTime)

	if err != nil {
		dc.logger.WithFields(logrus.Fields{
			"saga_id":      instance.ID,
			"step_name":    step.Name,
			"compensation": step.Compensation,
			"error":        err,
			"duration":     duration,
		}).Error("Saga step compensation failed")

		return &SagaStepResult{
			StepName:    step.Name,
			Success:     false,
			Error:       err,
			Compensated: false,
			Duration:    duration,
		}, nil
	}

	dc.logger.WithFields(logrus.Fields{
		"saga_id":      instance.ID,
		"step_name":    step.Name,
		"compensation": step.Compensation,
		"duration":     duration,
	}).Info("Saga step compensation executed successfully")

	return &SagaStepResult{
		StepName:    step.Name,
		Success:     true,
		Compensated: true,
		Duration:    duration,
	}, nil
}

// GetCompensationHandler retrieves a compensation handler by name
func (dc *DefaultCompensator) GetCompensationHandler(compensation string) (CompensationHandler, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	handler, exists := dc.handlers[compensation]
	if !exists {
		return nil, fmt.Errorf("compensation handler not found: %s", compensation)
	}

	return handler, nil
}

// GetRegisteredHandlers returns all registered compensation handlers
func (dc *DefaultCompensator) GetRegisteredHandlers() []string {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	handlers := make([]string, 0, len(dc.handlers))
	for name := range dc.handlers {
		handlers = append(handlers, name)
	}

	return handlers
}

// Example Step Handlers

// CreateUserStepHandler handles user creation
type CreateUserStepHandler struct {
	logger *logrus.Logger
}

// NewCreateUserStepHandler creates a new create user step handler
func NewCreateUserStepHandler(logger *logrus.Logger) *CreateUserStepHandler {
	return &CreateUserStepHandler{
		logger: logger,
	}
}

// Execute executes the create user step
func (cush *CreateUserStepHandler) Execute(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	email, ok := data["email"].(string)
	if !ok || email == "" {
		return nil, fmt.Errorf("email is required")
	}

	name, ok := data["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name is required")
	}

	cush.logger.WithFields(logrus.Fields{
		"email": email,
		"name":  name,
	}).Info("Creating user")

	// Simulate user creation (in real implementation, call user service)
	userID := fmt.Sprintf("user_%d", time.Now().Unix())

	// Return created user data
	return map[string]interface{}{
		"user_id":      userID,
		"email":        email,
		"name":         name,
		"created_at":   time.Now(),
		"user_created": true,
	}, nil
}

// GetName returns the handler name
func (cush *CreateUserStepHandler) GetName() string {
	return "create_user"
}

// DeleteUserCompensationHandler handles user deletion compensation
type DeleteUserCompensationHandler struct {
	logger *logrus.Logger
}

// NewDeleteUserCompensationHandler creates a new delete user compensation handler
func NewDeleteUserCompensationHandler(logger *logrus.Logger) *DeleteUserCompensationHandler {
	return &DeleteUserCompensationHandler{
		logger: logger,
	}
}

// Compensate executes user deletion compensation
func (duch *DeleteUserCompensationHandler) Compensate(ctx context.Context, data map[string]interface{}) error {
	userID, ok := data["user_id"].(string)
	if !ok || userID == "" {
		return fmt.Errorf("user_id is required for compensation")
	}

	duch.logger.WithField("user_id", userID).Info("Compensating user creation by deleting user")

	// Simulate user deletion (in real implementation, call user service)
	// This would typically mark the user as deleted or remove them from the system

	return nil
}

// GetName returns the compensation handler name
func (duch *DeleteUserCompensationHandler) GetName() string {
	return "delete_user"
}

// SendWelcomeEmailStepHandler handles sending welcome emails
type SendWelcomeEmailStepHandler struct {
	logger *logrus.Logger
}

// NewSendWelcomeEmailStepHandler creates a new send welcome email step handler
func NewSendWelcomeEmailStepHandler(logger *logrus.Logger) *SendWelcomeEmailStepHandler {
	return &SendWelcomeEmailStepHandler{
		logger: logger,
	}
}

// Execute executes the send welcome email step
func (swesh *SendWelcomeEmailStepHandler) Execute(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	email, ok := data["email"].(string)
	if !ok || email == "" {
		return nil, fmt.Errorf("email is required")
	}

	name, ok := data["name"].(string)
	if !ok || name == "" {
		name = "User"
	}

	swesh.logger.WithFields(logrus.Fields{
		"email": email,
		"name":  name,
	}).Info("Sending welcome email")

	// Simulate email sending (in real implementation, call notification service)
	emailID := fmt.Sprintf("email_%d", time.Now().Unix())

	return map[string]interface{}{
		"email_id":   emailID,
		"email_sent": true,
		"sent_at":    time.Now(),
	}, nil
}

// GetName returns the handler name
func (swesh *SendWelcomeEmailStepHandler) GetName() string {
	return "send_welcome_email"
}

// CreateBookingStepHandler handles booking creation
type CreateBookingStepHandler struct {
	logger *logrus.Logger
}

// NewCreateBookingStepHandler creates a new create booking step handler
func NewCreateBookingStepHandler(logger *logrus.Logger) *CreateBookingStepHandler {
	return &CreateBookingStepHandler{
		logger: logger,
	}
}

// Execute executes the create booking step
func (cbsh *CreateBookingStepHandler) Execute(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	userID, ok := data["user_id"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	facilityID, ok := data["facility_id"].(string)
	if !ok || facilityID == "" {
		return nil, fmt.Errorf("facility_id is required")
	}

	cbsh.logger.WithFields(logrus.Fields{
		"user_id":     userID,
		"facility_id": facilityID,
	}).Info("Creating booking")

	// Simulate booking creation (in real implementation, call booking service)
	bookingID := fmt.Sprintf("booking_%d", time.Now().Unix())

	return map[string]interface{}{
		"booking_id":      bookingID,
		"user_id":         userID,
		"facility_id":     facilityID,
		"booking_created": true,
		"created_at":      time.Now(),
	}, nil
}

// GetName returns the handler name
func (cbsh *CreateBookingStepHandler) GetName() string {
	return "create_booking"
}

// CancelBookingCompensationHandler handles booking cancellation compensation
type CancelBookingCompensationHandler struct {
	logger *logrus.Logger
}

// NewCancelBookingCompensationHandler creates a new cancel booking compensation handler
func NewCancelBookingCompensationHandler(logger *logrus.Logger) *CancelBookingCompensationHandler {
	return &CancelBookingCompensationHandler{
		logger: logger,
	}
}

// Compensate executes booking cancellation compensation
func (cbch *CancelBookingCompensationHandler) Compensate(ctx context.Context, data map[string]interface{}) error {
	bookingID, ok := data["booking_id"].(string)
	if !ok || bookingID == "" {
		return fmt.Errorf("booking_id is required for compensation")
	}

	cbch.logger.WithField("booking_id", bookingID).Info("Compensating booking creation by cancelling booking")

	// Simulate booking cancellation (in real implementation, call booking service)
	// This would typically cancel the booking and free up the time slot

	return nil
}

// GetName returns the compensation handler name
func (cbch *CancelBookingCompensationHandler) GetName() string {
	return "cancel_booking"
}

// ProcessPaymentStepHandler handles payment processing
type ProcessPaymentStepHandler struct {
	logger *logrus.Logger
}

// NewProcessPaymentStepHandler creates a new process payment step handler
func NewProcessPaymentStepHandler(logger *logrus.Logger) *ProcessPaymentStepHandler {
	return &ProcessPaymentStepHandler{
		logger: logger,
	}
}

// Execute executes the process payment step
func (ppsh *ProcessPaymentStepHandler) Execute(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	amount, ok := data["amount"].(float64)
	if !ok || amount <= 0 {
		return nil, fmt.Errorf("valid amount is required")
	}

	userID, ok := data["user_id"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	ppsh.logger.WithFields(logrus.Fields{
		"user_id": userID,
		"amount":  amount,
	}).Info("Processing payment")

	// Simulate payment processing (in real implementation, call payment service)
	paymentID := fmt.Sprintf("payment_%d", time.Now().Unix())

	// Simulate random failure (10% chance)
	if time.Now().Unix()%10 == 0 {
		return nil, fmt.Errorf("payment processing failed")
	}

	return map[string]interface{}{
		"payment_id":        paymentID,
		"amount":            amount,
		"payment_processed": true,
		"processed_at":      time.Now(),
	}, nil
}

// GetName returns the handler name
func (ppsh *ProcessPaymentStepHandler) GetName() string {
	return "process_payment"
}

// RefundPaymentCompensationHandler handles payment refund compensation
type RefundPaymentCompensationHandler struct {
	logger *logrus.Logger
}

// NewRefundPaymentCompensationHandler creates a new refund payment compensation handler
func NewRefundPaymentCompensationHandler(logger *logrus.Logger) *RefundPaymentCompensationHandler {
	return &RefundPaymentCompensationHandler{
		logger: logger,
	}
}

// Compensate executes payment refund compensation
func (rpch *RefundPaymentCompensationHandler) Compensate(ctx context.Context, data map[string]interface{}) error {
	paymentID, ok := data["payment_id"].(string)
	if !ok || paymentID == "" {
		return fmt.Errorf("payment_id is required for compensation")
	}

	amount, ok := data["amount"].(float64)
	if !ok || amount <= 0 {
		return fmt.Errorf("valid amount is required for compensation")
	}

	rpch.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
		"amount":     amount,
	}).Info("Compensating payment by issuing refund")

	// Simulate payment refund (in real implementation, call payment service)
	// This would typically refund the payment to the user

	return nil
}

// GetName returns the compensation handler name
func (rpch *RefundPaymentCompensationHandler) GetName() string {
	return "refund_payment"
}