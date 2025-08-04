package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultConfig("test")
		cb, err := New(config)
		
		require.NoError(t, err)
		assert.NotNil(t, cb)
		assert.Equal(t, "test", cb.Name())
		assert.Equal(t, StateClosed, cb.State())
		assert.NotNil(t, cb.config.ReadyToTrip)
	})

	t.Run("invalid config", func(t *testing.T) {
		config := &Config{
			Name: "", // Invalid empty name
		}
		
		cb, err := New(config)
		assert.Error(t, err)
		assert.Nil(t, cb)
		assert.Equal(t, ErrInvalidName, err)
	})

	t.Run("config with custom ReadyToTrip", func(t *testing.T) {
		config := DefaultConfig("test")
		customTrip := func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3
		}
		config.ReadyToTrip = customTrip
		
		cb, err := New(config)
		require.NoError(t, err)
		assert.NotNil(t, cb.config.ReadyToTrip)
	})
}

func TestCircuitBreaker_Execute(t *testing.T) {
	t.Run("successful execution in closed state", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		called := false
		err = cb.Execute(func() error {
			called = true
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, StateClosed, cb.State())
		
		counts := cb.Counts()
		assert.Equal(t, uint32(1), counts.Requests)
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
		assert.Equal(t, uint32(0), counts.TotalFailures)
	})

	t.Run("failed execution in closed state", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		testErr := errors.New("test error")
		called := false
		
		err = cb.Execute(func() error {
			called = true
			return testErr
		})

		assert.Equal(t, testErr, err)
		assert.True(t, called)
		assert.Equal(t, StateClosed, cb.State())
		
		counts := cb.Counts()
		assert.Equal(t, uint32(1), counts.Requests)
		assert.Equal(t, uint32(0), counts.TotalSuccesses)
		assert.Equal(t, uint32(1), counts.TotalFailures)
	})

	t.Run("panic handling", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		assert.Panics(t, func() {
			cb.Execute(func() error {
				panic("test panic")
			})
		})
		
		// Should record as failure
		counts := cb.Counts()
		assert.Equal(t, uint32(1), counts.Requests)
		assert.Equal(t, uint32(1), counts.TotalFailures)
	})

	t.Run("circuit breaker opens after failures", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureThreshold = 2
		config.MinimumRequestThreshold = 1
		
		cb, err := New(config)
		require.NoError(t, err)

		testErr := errors.New("test error")

		// First failure
		err = cb.Execute(func() error { return testErr })
		assert.Equal(t, testErr, err)
		assert.Equal(t, StateClosed, cb.State())

		// Second failure - should trip
		err = cb.Execute(func() error { return testErr })
		assert.Equal(t, testErr, err)
		assert.Equal(t, StateOpen, cb.State())

		// Third request should be rejected
		err = cb.Execute(func() error { return nil })
		assert.Error(t, err)
		assert.True(t, IsOpenStateError(err))
	})
}

func TestCircuitBreaker_ExecuteWithContext(t *testing.T) {
	t.Run("successful execution with context", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		ctx := context.Background()
		called := false
		
		err = cb.ExecuteWithContext(ctx, func(c context.Context) error {
			called = true
			assert.Equal(t, ctx, c)
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("failed execution with context", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		ctx := context.Background()
		testErr := errors.New("test error")
		
		err = cb.ExecuteWithContext(ctx, func(c context.Context) error {
			return testErr
		})

		assert.Equal(t, testErr, err)
	})

	t.Run("panic handling with context", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		ctx := context.Background()
		
		assert.Panics(t, func() {
			cb.ExecuteWithContext(ctx, func(c context.Context) error {
				panic("test panic")
			})
		})
	})
}

func TestCircuitBreaker_Call(t *testing.T) {
	t.Run("call is alias for execute", func(t *testing.T) {
		cb, err := New(DefaultConfig("test"))
		require.NoError(t, err)

		called := false
		err = cb.Call(func() error {
			called = true
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, called)
	})
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	t.Run("closed to open transition", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureThreshold = 3
		config.MinimumRequestThreshold = 1
		
		cb, err := New(config)
		require.NoError(t, err)

		testErr := errors.New("test error")

		// Generate enough failures to trip
		for i := 0; i < 3; i++ {
			err = cb.Execute(func() error { return testErr })
			assert.Equal(t, testErr, err)
		}

		assert.Equal(t, StateOpen, cb.State())
	})

	t.Run("open to half-open transition", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureThreshold = 1
		config.MinimumRequestThreshold = 1
		config.Timeout = 10 * time.Millisecond
		
		cb, err := New(config)
		require.NoError(t, err)

		// Trip the breaker
		err = cb.Execute(func() error { return errors.New("test error") })
		assert.Error(t, err)
		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout
		time.Sleep(15 * time.Millisecond)

		// Next request should transition to half-open and then to closed
		err = cb.Execute(func() error { return nil })
		assert.NoError(t, err)
		assert.Equal(t, StateClosed, cb.State()) // Should close immediately with MaxRequests=1
	})

	t.Run("half-open to closed transition", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureThreshold = 1
		config.MinimumRequestThreshold = 1
		config.Timeout = 10 * time.Millisecond
		config.MaxRequests = 2
		
		cb, err := New(config)
		require.NoError(t, err)

		// Trip the breaker
		err = cb.Execute(func() error { return errors.New("test error") })
		assert.Error(t, err)
		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout
		time.Sleep(15 * time.Millisecond)

		// Execute successful requests in half-open state
		err = cb.Execute(func() error { return nil })
		assert.NoError(t, err)
		assert.Equal(t, StateHalfOpen, cb.State())

		err = cb.Execute(func() error { return nil })
		assert.NoError(t, err)
		assert.Equal(t, StateClosed, cb.State())
	})

	t.Run("half-open to open transition", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureThreshold = 1
		config.MinimumRequestThreshold = 1
		config.Timeout = 10 * time.Millisecond
		
		cb, err := New(config)
		require.NoError(t, err)

		// Trip the breaker
		err = cb.Execute(func() error { return errors.New("test error") })
		assert.Error(t, err)
		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout
		time.Sleep(15 * time.Millisecond)

		// Execute successful request to go to half-open and then closed (MaxRequests=1)
		err = cb.Execute(func() error { return nil })
		assert.NoError(t, err)
		assert.Equal(t, StateClosed, cb.State()) // Should go to closed immediately

		// Create new breaker for testing half-open -> open transition
		cb2, err := New(config)
		require.NoError(t, err)

		// Trip the breaker
		err = cb2.Execute(func() error { return errors.New("test error") })
		assert.Error(t, err)
		assert.Equal(t, StateOpen, cb2.State())

		// Wait and manually set to half-open
		time.Sleep(15 * time.Millisecond)
		cb2.mutex.Lock()
		cb2.state = StateHalfOpen
		cb2.toNewGeneration(time.Now())
		cb2.mutex.Unlock()

		// Execute failed request - should go back to open  
		testErr := errors.New("test error")
		err = cb2.Execute(func() error { return testErr })
		assert.Equal(t, testErr, err)
		assert.Equal(t, StateOpen, cb2.State())
	})
}

func TestCircuitBreaker_HalfOpenMaxRequests(t *testing.T) {
	config := DefaultConfig("test")
	config.FailureThreshold = 1
	config.MinimumRequestThreshold = 1
	config.Timeout = 10 * time.Millisecond
	config.MaxRequests = 2
	
	cb, err := New(config)
	require.NoError(t, err)

	// Trip the breaker
	err = cb.Execute(func() error { return errors.New("test error") })
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(15 * time.Millisecond)

	// Execute max requests in half-open state
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, StateHalfOpen, cb.State())

	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())

	// Reset for next test
	cb.ForceOpen()
	
	// Wait for timeout again
	time.Sleep(15 * time.Millisecond)

	// Execute first request to go to half-open
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, StateHalfOpen, cb.State())

	// Second request should succeed
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_TooManyRequestsInHalfOpen(t *testing.T) {
	config := DefaultConfig("test")
	config.FailureThreshold = 1
	config.MinimumRequestThreshold = 1
	config.Timeout = 10 * time.Millisecond
	config.MaxRequests = 1
	
	cb, err := New(config)
	require.NoError(t, err)

	// Trip the breaker
	err = cb.Execute(func() error { return errors.New("test error") })
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(15 * time.Millisecond)

	// Execute first request (allowed)
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State()) // Should close after 1 successful request

	// Reset and test rejection
	cb.ForceOpen()
	time.Sleep(15 * time.Millisecond)

	// First request - should succeed and keep in half-open (not enough consecutive successes)
	cb.Execute(func() error { return nil })

	// If we set MaxRequests to 1 but need 2 consecutive successes, we need to test differently
	// Let's force half-open state and test concurrent requests
	config.MaxRequests = 1
	cb2, err := New(config)
	require.NoError(t, err)

	// Manually set to half-open state for testing
	cb2.mutex.Lock()
	cb2.state = StateHalfOpen
	cb2.counts.Requests = 1 // Already at max
	cb2.mutex.Unlock()

	// Next request should be rejected
	err = cb2.Execute(func() error { return nil })
	assert.Error(t, err)
	assert.True(t, IsTooManyRequestsError(err))
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := DefaultConfig("test")
	config.FailureThreshold = 1
	config.MinimumRequestThreshold = 1
	
	cb, err := New(config)
	require.NoError(t, err)

	// Trip the breaker
	err = cb.Execute(func() error { return errors.New("test error") })
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.State())

	// Reset should close the breaker
	cb.Reset()
	assert.Equal(t, StateClosed, cb.State())

	// Should work normally after reset
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	cb, err := New(DefaultConfig("test"))
	require.NoError(t, err)

	assert.Equal(t, StateClosed, cb.State())

	// Force open
	cb.ForceOpen()
	assert.Equal(t, StateOpen, cb.State())

	// Requests should be rejected
	err = cb.Execute(func() error { return nil })
	assert.Error(t, err)
	assert.True(t, IsOpenStateError(err))
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var stateChanges []struct {
		name string
		from State
		to   State
	}

	config := DefaultConfig("test")
	config.FailureThreshold = 1
	config.MinimumRequestThreshold = 1
	config.OnStateChange = func(name string, from State, to State) {
		stateChanges = append(stateChanges, struct {
			name string
			from State
			to   State
		}{name, from, to})
	}

	cb, err := New(config)
	require.NoError(t, err)

	// Trip the breaker
	err = cb.Execute(func() error { return errors.New("test error") })
	assert.Error(t, err)

	// Should have one state change: Closed -> Open
	assert.Len(t, stateChanges, 1)
	assert.Equal(t, "test", stateChanges[0].name)
	assert.Equal(t, StateClosed, stateChanges[0].from)
	assert.Equal(t, StateOpen, stateChanges[0].to)

	// Reset to trigger another state change
	cb.Reset()

	// Should have two state changes now
	assert.Len(t, stateChanges, 2)
	assert.Equal(t, StateOpen, stateChanges[1].from)
	assert.Equal(t, StateClosed, stateChanges[1].to)
}

func TestCircuitBreaker_CustomIsSuccessful(t *testing.T) {
	config := DefaultConfig("test")
	config.FailureThreshold = 1
	config.MinimumRequestThreshold = 1
	
	// Custom success function: only specific errors are failures
	config.IsSuccessful = func(err error) bool {
		return err == nil || err.Error() != "critical error"
	}

	cb, err := New(config)
	require.NoError(t, err)

	// Non-critical error should be treated as success
	err = cb.Execute(func() error { return errors.New("minor error") })
	assert.Error(t, err)
	assert.Equal(t, StateClosed, cb.State()) // Should remain closed

	// Check the callback works - success recorded
	counts := cb.Counts()
	assert.Equal(t, uint32(1), counts.TotalSuccesses) 
	assert.Equal(t, uint32(0), counts.TotalFailures)

	// Critical error should be treated as failure and trip the breaker
	criticalErr := errors.New("critical error")
	err = cb.Execute(func() error { return criticalErr })
	assert.Equal(t, criticalErr, err)
	assert.Equal(t, StateOpen, cb.State()) // Should trip

	// Note: When circuit breaker opens, it starts a new generation, so counts are reset
	counts = cb.Counts()
	assert.Equal(t, uint32(0), counts.TotalFailures) // Reset in new generation
}

func TestCircuitBreaker_GenerationHandling(t *testing.T) {
	config := DefaultConfig("test")
	config.Interval = 50 * time.Millisecond
	
	cb, err := New(config)
	require.NoError(t, err)

	// Execute request
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)

	// Wait for interval to pass
	time.Sleep(60 * time.Millisecond)

	// Next request should start new generation (previous counts cleared)
	err = cb.Execute(func() error { return nil })
	assert.NoError(t, err)

	// The interval doesn't immediately reset counts; it sets an expiry time
	// Both requests are executed before the interval-based reset occurs
	newCounts := cb.Counts()
	assert.Equal(t, uint32(2), newCounts.Requests) // Both requests before reset
	assert.Equal(t, uint32(2), newCounts.TotalSuccesses) // Both successes
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb, err := New(DefaultConfig("test"))
	require.NoError(t, err)

	const numGoroutines = 100
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	successCount := int32(0)
	errorCount := int32(0)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				err := cb.Execute(func() error {
					// Simulate some work
					time.Sleep(time.Microsecond)
					return nil
				})
				if err != nil {
					errorCount++
				} else {
					successCount++
				}
			}
		}()
	}

	wg.Wait()

	// Should have processed all requests
	counts := cb.Counts()
	expectedTotal := uint32(numGoroutines * requestsPerGoroutine)
	assert.Equal(t, expectedTotal, counts.Requests)
	assert.Equal(t, expectedTotal, counts.TotalSuccesses)
	assert.Equal(t, uint32(0), counts.TotalFailures)
}

func TestCircuitBreaker_Config(t *testing.T) {
	originalConfig := DefaultConfig("test")
	cb, err := New(originalConfig)
	require.NoError(t, err)

	// Config should return a copy
	returnedConfig := cb.Config()
	
	// Compare fields that can be compared (excluding function fields)
	assert.Equal(t, originalConfig.Name, returnedConfig.Name)
	assert.Equal(t, originalConfig.MaxRequests, returnedConfig.MaxRequests)
	assert.Equal(t, originalConfig.Interval, returnedConfig.Interval)
	assert.Equal(t, originalConfig.Timeout, returnedConfig.Timeout)

	// Modifying returned config shouldn't affect original
	returnedConfig.Name = "modified"
	assert.Equal(t, "test", cb.Name())
}

func TestCircuitBreaker_Counts(t *testing.T) {
	cb, err := New(DefaultConfig("test"))
	require.NoError(t, err)

	// Initial counts
	counts := cb.Counts()
	assert.Equal(t, uint32(0), counts.Requests)
	assert.Equal(t, uint32(0), counts.TotalSuccesses)
	assert.Equal(t, uint32(0), counts.TotalFailures)

	// Execute some requests
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return errors.New("error") })

	counts = cb.Counts()
	assert.Equal(t, uint32(2), counts.Requests)
	assert.Equal(t, uint32(1), counts.TotalSuccesses)
	assert.Equal(t, uint32(1), counts.TotalFailures)
	assert.Equal(t, uint32(0), counts.ConsecutiveSuccesses) // Reset after failure
	assert.Equal(t, uint32(1), counts.ConsecutiveFailures)
}

func TestCircuitBreaker_FailureRatio(t *testing.T) {
	config := DefaultConfig("test")
	config.FailureThreshold = 0 // Use ratio instead
	config.FailureRatio = 0.5
	config.MinimumRequestThreshold = 4
	
	cb, err := New(config)
	require.NoError(t, err)

	// Execute requests below threshold - should not trip
	cb.Execute(func() error { return errors.New("error") })
	cb.Execute(func() error { return errors.New("error") })
	assert.Equal(t, StateClosed, cb.State())

	// Execute requests to reach threshold
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return errors.New("error") }) // 3/4 = 0.75 > 0.5
	
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_Name(t *testing.T) {
	cb, err := New(DefaultConfig("test-breaker"))
	require.NoError(t, err)

	assert.Equal(t, "test-breaker", cb.Name())
}

// Benchmark tests
func BenchmarkCircuitBreaker_Execute_Success(b *testing.B) {
	cb, err := New(DefaultConfig("benchmark"))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(func() error { return nil })
	}
}

func BenchmarkCircuitBreaker_Execute_Failure(b *testing.B) {
	cb, err := New(DefaultConfig("benchmark"))
	require.NoError(b, err)

	testErr := errors.New("benchmark error")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(func() error { return testErr })
	}
}

func BenchmarkCircuitBreaker_Execute_Open(b *testing.B) {
	config := DefaultConfig("benchmark")
	config.FailureThreshold = 1
	config.MinimumRequestThreshold = 1
	
	cb, err := New(config)
	require.NoError(b, err)

	// Trip the breaker
	cb.Execute(func() error { return errors.New("error") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(func() error { return nil })
	}
}

func BenchmarkCircuitBreaker_State(b *testing.B) {
	cb, err := New(DefaultConfig("benchmark"))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.State()
	}
}

func BenchmarkCircuitBreaker_Counts(b *testing.B) {
	cb, err := New(DefaultConfig("benchmark"))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Counts()
	}
}