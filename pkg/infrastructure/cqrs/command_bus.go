package cqrs

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/sirupsen/logrus"
)

// CommandBus handles command dispatching and execution
type CommandBus struct {
	handlers map[string]CommandHandler
	logger   *logrus.Logger
	mu       sync.RWMutex
	config   CommandBusConfig
}

// CommandBusConfig holds command bus configuration
type CommandBusConfig struct {
	EnableMetrics    bool `json:"enable_metrics"`
	EnableTracing    bool `json:"enable_tracing"`
	MaxConcurrent    int  `json:"max_concurrent"`
	TimeoutDuration  int  `json:"timeout_duration"` // seconds
}

// Command represents a command that can be executed
type Command interface {
	GetCommandType() string
	GetAggregateID() string
	GetMetadata() map[string]interface{}
}

// CommandHandler defines the interface for command handlers
type CommandHandler interface {
	Handle(ctx context.Context, command Command) (*CommandResult, error)
	GetCommandType() string
}

// CommandResult represents the result of a command execution
type CommandResult struct {
	AggregateID   string                 `json:"aggregate_id"`
	EventsEmitted []interface{}          `json:"events_emitted"`
	Success       bool                   `json:"success"`
	Message       string                 `json:"message,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Version       int                    `json:"version"`
}

// BaseCommand provides a base implementation for commands
type BaseCommand struct {
	CommandType string                 `json:"command_type"`
	AggregateID string                 `json:"aggregate_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	UserID      string                 `json:"user_id"`
	TenantID    string                 `json:"tenant_id"`
	CorrelationID string               `json:"correlation_id"`
}

// GetCommandType returns the command type
func (bc *BaseCommand) GetCommandType() string {
	return bc.CommandType
}

// GetAggregateID returns the aggregate ID
func (bc *BaseCommand) GetAggregateID() string {
	return bc.AggregateID
}

// GetMetadata returns the command metadata
func (bc *BaseCommand) GetMetadata() map[string]interface{} {
	return bc.Metadata
}

// NewCommandBus creates a new command bus
func NewCommandBus(config CommandBusConfig, logger *logrus.Logger) *CommandBus {
	// Set default config values
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 100
	}
	if config.TimeoutDuration == 0 {
		config.TimeoutDuration = 30
	}

	return &CommandBus{
		handlers: make(map[string]CommandHandler),
		logger:   logger,
		config:   config,
	}
}

// RegisterHandler registers a command handler
func (cb *CommandBus) RegisterHandler(handler CommandHandler) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	commandType := handler.GetCommandType()
	if commandType == "" {
		return fmt.Errorf("command type cannot be empty")
	}

	if _, exists := cb.handlers[commandType]; exists {
		return fmt.Errorf("handler for command type %s already registered", commandType)
	}

	cb.handlers[commandType] = handler

	cb.logger.WithFields(logrus.Fields{
		"command_type": commandType,
		"handler":      reflect.TypeOf(handler).String(),
	}).Info("Command handler registered successfully")

	return nil
}

// Execute executes a command
func (cb *CommandBus) Execute(ctx context.Context, command Command) (*CommandResult, error) {
	commandType := command.GetCommandType()
	if commandType == "" {
		return nil, fmt.Errorf("command type cannot be empty")
	}

	// Get handler
	cb.mu.RLock()
	handler, exists := cb.handlers[commandType]
	cb.mu.RUnlock()

	if !exists {
		cb.logger.WithFields(logrus.Fields{
			"command_type": commandType,
			"aggregate_id": command.GetAggregateID(),
		}).Error("No handler found for command type")
		return nil, fmt.Errorf("no handler registered for command type: %s", commandType)
	}

	// Log command execution start
	cb.logger.WithFields(logrus.Fields{
		"command_type": commandType,
		"aggregate_id": command.GetAggregateID(),
		"handler":      reflect.TypeOf(handler).String(),
	}).Debug("Executing command")

	// Execute command
	result, err := handler.Handle(ctx, command)
	if err != nil {
		cb.logger.WithFields(logrus.Fields{
			"command_type": commandType,
			"aggregate_id": command.GetAggregateID(),
			"error":        err,
		}).Error("Command execution failed")
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	// Log successful execution
	cb.logger.WithFields(logrus.Fields{
		"command_type":   commandType,
		"aggregate_id":   command.GetAggregateID(),
		"success":        result.Success,
		"events_count":   len(result.EventsEmitted),
		"result_version": result.Version,
	}).Info("Command executed successfully")

	return result, nil
}

// ExecuteAsync executes a command asynchronously
func (cb *CommandBus) ExecuteAsync(ctx context.Context, command Command) <-chan *AsyncCommandResult {
	resultChan := make(chan *AsyncCommandResult, 1)

	go func() {
		defer close(resultChan)

		result, err := cb.Execute(ctx, command)
		asyncResult := &AsyncCommandResult{
			Result: result,
			Error:  err,
		}

		select {
		case resultChan <- asyncResult:
		case <-ctx.Done():
			cb.logger.WithFields(logrus.Fields{
				"command_type": command.GetCommandType(),
				"aggregate_id": command.GetAggregateID(),
			}).Warn("Async command execution cancelled")
		}
	}()

	return resultChan
}

// AsyncCommandResult represents the result of an async command execution
type AsyncCommandResult struct {
	Result *CommandResult
	Error  error
}

// GetRegisteredCommands returns a list of registered command types
func (cb *CommandBus) GetRegisteredCommands() []string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	commands := make([]string, 0, len(cb.handlers))
	for commandType := range cb.handlers {
		commands = append(commands, commandType)
	}

	return commands
}

// UnregisterHandler removes a command handler
func (cb *CommandBus) UnregisterHandler(commandType string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if _, exists := cb.handlers[commandType]; !exists {
		return fmt.Errorf("no handler registered for command type: %s", commandType)
	}

	delete(cb.handlers, commandType)

	cb.logger.WithField("command_type", commandType).Info("Command handler unregistered")

	return nil
}

// GetHandlerCount returns the number of registered handlers
func (cb *CommandBus) GetHandlerCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return len(cb.handlers)
}

// ValidateCommand validates a command before execution
func (cb *CommandBus) ValidateCommand(command Command) error {
	if command == nil {
		return fmt.Errorf("command cannot be nil")
	}

	if command.GetCommandType() == "" {
		return fmt.Errorf("command type cannot be empty")
	}

	if command.GetAggregateID() == "" {
		return fmt.Errorf("aggregate ID cannot be empty")
	}

	return nil
}

// HealthCheck performs a health check on the command bus
func (cb *CommandBus) HealthCheck(ctx context.Context) error {
	cb.mu.RLock()
	handlerCount := len(cb.handlers)
	cb.mu.RUnlock()

	if handlerCount == 0 {
		return fmt.Errorf("no command handlers registered")
	}

	cb.logger.WithField("handler_count", handlerCount).Debug("Command bus health check passed")
	return nil
}

// Close closes the command bus
func (cb *CommandBus) Close() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Clear handlers
	cb.handlers = make(map[string]CommandHandler)

	cb.logger.Info("Command bus closed")
	return nil
}

// Batch execution functionality

// BatchCommand represents a batch of commands to execute
type BatchCommand struct {
	Commands    []Command `json:"commands"`
	Transaction bool      `json:"transaction"` // If true, all commands must succeed
}

// BatchResult represents the result of batch command execution
type BatchResult struct {
	Results    []*CommandResult `json:"results"`
	Success    bool             `json:"success"`
	FailedAt   int              `json:"failed_at,omitempty"` // Index of failed command
	TotalCount int              `json:"total_count"`
}

// ExecuteBatch executes multiple commands in a batch
func (cb *CommandBus) ExecuteBatch(ctx context.Context, batch *BatchCommand) (*BatchResult, error) {
	if len(batch.Commands) == 0 {
		return nil, fmt.Errorf("batch cannot be empty")
	}

	results := make([]*CommandResult, 0, len(batch.Commands))
	batchResult := &BatchResult{
		Results:    results,
		Success:    true,
		TotalCount: len(batch.Commands),
	}

	cb.logger.WithFields(logrus.Fields{
		"command_count": len(batch.Commands),
		"transaction":   batch.Transaction,
	}).Info("Executing command batch")

	for i, command := range batch.Commands {
		result, err := cb.Execute(ctx, command)
		if err != nil {
			batchResult.Success = false
			batchResult.FailedAt = i

			if batch.Transaction {
				// In transaction mode, stop on first failure
				cb.logger.WithFields(logrus.Fields{
					"failed_at":    i,
					"command_type": command.GetCommandType(),
					"error":        err,
				}).Error("Batch command failed in transaction mode")
				return batchResult, fmt.Errorf("batch failed at command %d: %w", i, err)
			}

			// In non-transaction mode, continue with next command
			cb.logger.WithFields(logrus.Fields{
				"failed_at":    i,
				"command_type": command.GetCommandType(),
				"error":        err,
			}).Warn("Command failed in batch, continuing with next")

			// Add a failed result
			failedResult := &CommandResult{
				Success: false,
				Message: err.Error(),
			}
			batchResult.Results = append(batchResult.Results, failedResult)
		} else {
			batchResult.Results = append(batchResult.Results, result)
		}
	}

	cb.logger.WithFields(logrus.Fields{
		"total_commands":    len(batch.Commands),
		"successful_commands": len(batchResult.Results),
		"batch_success":     batchResult.Success,
	}).Info("Batch command execution completed")

	return batchResult, nil
}