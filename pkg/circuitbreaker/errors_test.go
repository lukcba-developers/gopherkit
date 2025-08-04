package circuitbreaker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreakerError_Error(t *testing.T) {
	t.Run("error with cause", func(t *testing.T) {
		cause := errors.New("original error")
		cbErr := &CircuitBreakerError{
			Name:    "test-breaker",
			State:   StateOpen,
			Message: "circuit breaker is open",
			Cause:   cause,
		}

		expected := "circuit breaker 'test-breaker' (open): circuit breaker is open (caused by: original error)"
		assert.Equal(t, expected, cbErr.Error())
	})

	t.Run("error without cause", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			Name:    "test-breaker",
			State:   StateHalfOpen,
			Message: "too many requests",
		}

		expected := "circuit breaker 'test-breaker' (half-open): too many requests"
		assert.Equal(t, expected, cbErr.Error())
	})
}

func TestCircuitBreakerError_Unwrap(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("original error")
		cbErr := &CircuitBreakerError{
			Cause: cause,
		}

		assert.Equal(t, cause, cbErr.Unwrap())
	})

	t.Run("without cause", func(t *testing.T) {
		cbErr := &CircuitBreakerError{}
		assert.Nil(t, cbErr.Unwrap())
	})
}

func TestCircuitBreakerError_IsOpenError(t *testing.T) {
	t.Run("is open error", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
			Cause: ErrOpenState,
		}

		assert.True(t, cbErr.IsOpenError())
	})

	t.Run("wrong state", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateHalfOpen,
			Cause: ErrOpenState,
		}

		assert.False(t, cbErr.IsOpenError())
	})

	t.Run("wrong cause", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
			Cause: ErrTooManyRequests,
		}

		assert.False(t, cbErr.IsOpenError())
	})

	t.Run("no cause", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
		}

		assert.False(t, cbErr.IsOpenError())
	})
}

func TestCircuitBreakerError_IsTooManyRequestsError(t *testing.T) {
	t.Run("is too many requests error", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateHalfOpen,
			Cause: ErrTooManyRequests,
		}

		assert.True(t, cbErr.IsTooManyRequestsError())
	})

	t.Run("wrong state", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
			Cause: ErrTooManyRequests,
		}

		assert.False(t, cbErr.IsTooManyRequestsError())
	})

	t.Run("wrong cause", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateHalfOpen,
			Cause: ErrOpenState,
		}

		assert.False(t, cbErr.IsTooManyRequestsError())
	})
}

func TestNewOpenStateError(t *testing.T) {
	timeout := 30 * time.Second
	counts := Counts{
		Requests:            10,
		TotalFailures:       7,
		ConsecutiveFailures: 5,
	}

	err := NewOpenStateError("test-breaker", timeout, counts)

	assert.Equal(t, "test-breaker", err.Name)
	assert.Equal(t, StateOpen, err.State)
	assert.Equal(t, "circuit breaker is open, retry after 30s", err.Message)
	assert.Equal(t, ErrOpenState, err.Cause)
	assert.Equal(t, timeout, err.Timeout)
	assert.Equal(t, counts, err.Counts)
	assert.True(t, err.IsOpenError())
	assert.False(t, err.IsTooManyRequestsError())
}

func TestNewTooManyRequestsError(t *testing.T) {
	maxRequests := uint32(3)
	counts := Counts{
		Requests:       3,
		TotalSuccesses: 2,
		TotalFailures:  1,
	}

	err := NewTooManyRequestsError("test-breaker", maxRequests, counts)

	assert.Equal(t, "test-breaker", err.Name)
	assert.Equal(t, StateHalfOpen, err.State)
	assert.Equal(t, "too many requests in half-open state (max: 3)", err.Message)
	assert.Equal(t, ErrTooManyRequests, err.Cause)
	assert.Equal(t, time.Duration(0), err.Timeout) // Should be zero for this error type
	assert.Equal(t, counts, err.Counts)
	assert.False(t, err.IsOpenError())
	assert.True(t, err.IsTooManyRequestsError())
}

func TestIsCircuitBreakerError(t *testing.T) {
	t.Run("is circuit breaker error", func(t *testing.T) {
		cbErr := &CircuitBreakerError{}
		assert.True(t, IsCircuitBreakerError(cbErr))
	})

	t.Run("is not circuit breaker error", func(t *testing.T) {
		regularErr := errors.New("regular error")
		assert.False(t, IsCircuitBreakerError(regularErr))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsCircuitBreakerError(nil))
	})
}

func TestIsOpenStateError(t *testing.T) {
	t.Run("circuit breaker error - open state", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
			Cause: ErrOpenState,
		}
		assert.True(t, IsOpenStateError(cbErr))
	})

	t.Run("circuit breaker error - not open state", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateHalfOpen,
			Cause: ErrTooManyRequests,
		}
		assert.False(t, IsOpenStateError(cbErr))
	})

	t.Run("direct ErrOpenState", func(t *testing.T) {
		assert.True(t, IsOpenStateError(ErrOpenState))
	})

	t.Run("wrapped ErrOpenState", func(t *testing.T) {
		wrapped := errors.New("wrapped: " + ErrOpenState.Error())
		// This will return false because errors.Is() does exact matching
		assert.False(t, IsOpenStateError(wrapped))
	})

	t.Run("other error", func(t *testing.T) {
		regularErr := errors.New("regular error")
		assert.False(t, IsOpenStateError(regularErr))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsOpenStateError(nil))
	})
}

func TestIsTooManyRequestsError(t *testing.T) {
	t.Run("circuit breaker error - too many requests", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateHalfOpen,
			Cause: ErrTooManyRequests,
		}
		assert.True(t, IsTooManyRequestsError(cbErr))
	})

	t.Run("circuit breaker error - not too many requests", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
			Cause: ErrOpenState,
		}
		assert.False(t, IsTooManyRequestsError(cbErr))
	})

	t.Run("direct ErrTooManyRequests", func(t *testing.T) {
		assert.True(t, IsTooManyRequestsError(ErrTooManyRequests))
	})

	t.Run("other error", func(t *testing.T) {
		regularErr := errors.New("regular error")
		assert.False(t, IsTooManyRequestsError(regularErr))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsTooManyRequestsError(nil))
	})
}

func TestErrorConstants(t *testing.T) {
	t.Run("error messages", func(t *testing.T) {
		assert.Equal(t, "circuit breaker is open", ErrOpenState.Error())
		assert.Equal(t, "too many requests in half-open state", ErrTooManyRequests.Error())
		assert.Equal(t, "circuit breaker name cannot be empty", ErrInvalidName.Error())
		assert.Equal(t, "max requests must be greater than 0", ErrInvalidMaxRequests.Error())
		assert.Equal(t, "timeout must be greater than 0", ErrInvalidTimeout.Error())
		assert.Equal(t, "interval cannot be negative", ErrInvalidInterval.Error())
		assert.Equal(t, "failure ratio must be between 0.0 and 1.0", ErrInvalidFailureRatio.Error())
		assert.Equal(t, "must specify either failure threshold, failure ratio, or custom ReadyToTrip function", ErrNoFailureCriteria.Error())
	})

	t.Run("error identity", func(t *testing.T) {
		// Test that errors maintain their identity for errors.Is() comparisons
		assert.True(t, errors.Is(ErrOpenState, ErrOpenState))
		assert.True(t, errors.Is(ErrTooManyRequests, ErrTooManyRequests))
		assert.False(t, errors.Is(ErrOpenState, ErrTooManyRequests))
	})
}

func TestCircuitBreakerError_ComplexScenarios(t *testing.T) {
	t.Run("error with all fields", func(t *testing.T) {
		cause := errors.New("database connection failed")
		counts := Counts{
			Requests:             15,
			TotalSuccesses:       5,
			TotalFailures:        10,
			ConsecutiveFailures:  8,
			LastRequestTime:      time.Now(),
		}

		cbErr := &CircuitBreakerError{
			Name:    "database-breaker",
			State:   StateOpen,
			Message: "circuit breaker tripped due to high failure rate",
			Cause:   cause,
			Timeout: 60 * time.Second,
			Counts:  counts,
		}

		// Test error message
		errorMsg := cbErr.Error()
		assert.Contains(t, errorMsg, "database-breaker")
		assert.Contains(t, errorMsg, "open")
		assert.Contains(t, errorMsg, "circuit breaker tripped due to high failure rate")
		assert.Contains(t, errorMsg, "database connection failed")

		// Test unwrapping
		assert.Equal(t, cause, cbErr.Unwrap())

		// Test state checks
		assert.False(t, cbErr.IsOpenError()) // Cause is not ErrOpenState
		assert.False(t, cbErr.IsTooManyRequestsError())
	})

	t.Run("error chaining with errors.Is", func(t *testing.T) {
		cbErr := NewOpenStateError("test", 30*time.Second, Counts{})
		
		// Should work with errors.Is
		assert.True(t, errors.Is(cbErr, ErrOpenState))
		assert.False(t, errors.Is(cbErr, ErrTooManyRequests))
	})

	t.Run("error chaining with errors.As", func(t *testing.T) {
		cbErr := NewTooManyRequestsError("test", 5, Counts{})
		
		var target *CircuitBreakerError
		assert.True(t, errors.As(cbErr, &target))
		assert.Equal(t, cbErr, target)
	})
}

func TestErrorHelperFunctions_EdgeCases(t *testing.T) {
	t.Run("IsCircuitBreakerError with nil pointer", func(t *testing.T) {
		var cbErr *CircuitBreakerError = nil
		// Go's type assertion with nil pointer of correct type returns true
		assert.True(t, IsCircuitBreakerError(cbErr))
	})

	t.Run("IsOpenStateError with CircuitBreakerError but nil cause", func(t *testing.T) {
		cbErr := &CircuitBreakerError{
			State: StateOpen,
			Cause: nil,
		}
		assert.False(t, IsOpenStateError(cbErr))
	})

	t.Run("error type assertions", func(t *testing.T) {
		cbErr := NewOpenStateError("test", 30*time.Second, Counts{})
		
		// Test that it satisfies error interface
		var err error = cbErr
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "test")
		
		// Test that it can be cast back
		if circuitErr, ok := err.(*CircuitBreakerError); ok {
			assert.Equal(t, "test", circuitErr.Name)
			assert.Equal(t, StateOpen, circuitErr.State)
		} else {
			t.Fatal("Failed to cast back to CircuitBreakerError")
		}
	})
}

// Benchmark tests
func BenchmarkCircuitBreakerError_Error(b *testing.B) {
	cbErr := &CircuitBreakerError{
		Name:    "benchmark-breaker",
		State:   StateOpen,
		Message: "circuit breaker is open",
		Cause:   ErrOpenState,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cbErr.Error()
	}
}

func BenchmarkNewOpenStateError(b *testing.B) {
	timeout := 30 * time.Second
	counts := Counts{
		Requests:       10,
		TotalFailures:  7,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewOpenStateError("benchmark", timeout, counts)
	}
}

func BenchmarkNewTooManyRequestsError(b *testing.B) {
	maxRequests := uint32(5)
	counts := Counts{
		Requests: 3,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewTooManyRequestsError("benchmark", maxRequests, counts)
	}
}

func BenchmarkIsCircuitBreakerError(b *testing.B) {
	cbErr := NewOpenStateError("benchmark", 30*time.Second, Counts{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsCircuitBreakerError(cbErr)
	}
}

func BenchmarkIsOpenStateError(b *testing.B) {
	cbErr := NewOpenStateError("benchmark", 30*time.Second, Counts{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsOpenStateError(cbErr)
	}
}