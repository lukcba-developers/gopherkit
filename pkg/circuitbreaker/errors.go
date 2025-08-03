package circuitbreaker

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrOpenState se retorna cuando el circuit breaker está abierto
	ErrOpenState = errors.New("circuit breaker is open")
	
	// ErrTooManyRequests se retorna cuando hay demasiadas requests en estado half-open
	ErrTooManyRequests = errors.New("too many requests in half-open state")
	
	// Configuration validation errors
	ErrInvalidName = errors.New("circuit breaker name cannot be empty")
	ErrInvalidMaxRequests = errors.New("max requests must be greater than 0")
	ErrInvalidTimeout = errors.New("timeout must be greater than 0")
	ErrInvalidInterval = errors.New("interval cannot be negative")
	ErrInvalidFailureRatio = errors.New("failure ratio must be between 0.0 and 1.0")
	ErrNoFailureCriteria = errors.New("must specify either failure threshold, failure ratio, or custom ReadyToTrip function")
)

// CircuitBreakerError representa un error específico del circuit breaker
type CircuitBreakerError struct {
	Name    string        `json:"name"`
	State   State         `json:"state"`
	Message string        `json:"message"`
	Cause   error         `json:"-"`
	Timeout time.Duration `json:"timeout,omitempty"`
	Counts  Counts        `json:"counts,omitempty"`
}

// Error implementa la interfaz error
func (e *CircuitBreakerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("circuit breaker '%s' (%s): %s (caused by: %s)", 
			e.Name, e.State.String(), e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("circuit breaker '%s' (%s): %s", 
		e.Name, e.State.String(), e.Message)
}

// Unwrap permite unwrapping del error original
func (e *CircuitBreakerError) Unwrap() error {
	return e.Cause
}

// IsOpenError verifica si el error es por estado abierto
func (e *CircuitBreakerError) IsOpenError() bool {
	return e.State == StateOpen && errors.Is(e.Cause, ErrOpenState)
}

// IsTooManyRequestsError verifica si el error es por demasiadas requests
func (e *CircuitBreakerError) IsTooManyRequestsError() bool {
	return e.State == StateHalfOpen && errors.Is(e.Cause, ErrTooManyRequests)
}

// NewOpenStateError crea un error para estado abierto
func NewOpenStateError(name string, timeout time.Duration, counts Counts) *CircuitBreakerError {
	return &CircuitBreakerError{
		Name:    name,
		State:   StateOpen,
		Message: fmt.Sprintf("circuit breaker is open, retry after %v", timeout),
		Cause:   ErrOpenState,
		Timeout: timeout,
		Counts:  counts,
	}
}

// NewTooManyRequestsError crea un error para demasiadas requests
func NewTooManyRequestsError(name string, maxRequests uint32, counts Counts) *CircuitBreakerError {
	return &CircuitBreakerError{
		Name:    name,
		State:   StateHalfOpen,
		Message: fmt.Sprintf("too many requests in half-open state (max: %d)", maxRequests),
		Cause:   ErrTooManyRequests,
		Counts:  counts,
	}
}

// IsCircuitBreakerError verifica si un error es de circuit breaker
func IsCircuitBreakerError(err error) bool {
	_, ok := err.(*CircuitBreakerError)
	return ok
}

// IsOpenStateError verifica si un error es por estado abierto
func IsOpenStateError(err error) bool {
	if cbErr, ok := err.(*CircuitBreakerError); ok {
		return cbErr.IsOpenError()
	}
	return errors.Is(err, ErrOpenState)
}

// IsTooManyRequestsError verifica si un error es por demasiadas requests
func IsTooManyRequestsError(err error) bool {
	if cbErr, ok := err.(*CircuitBreakerError); ok {
		return cbErr.IsTooManyRequestsError()
	}
	return errors.Is(err, ErrTooManyRequests)
}