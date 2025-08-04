package circuitbreaker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateHalfOpen, "half-open"},
		{StateOpen, "open"},
		{State(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestCounts_FailureRatio(t *testing.T) {
	tests := []struct {
		name     string
		counts   Counts
		expected float64
	}{
		{
			name:     "no requests",
			counts:   Counts{},
			expected: 0.0,
		},
		{
			name: "all successes",
			counts: Counts{
				Requests:       10,
				TotalSuccesses: 10,
				TotalFailures:  0,
			},
			expected: 0.0,
		},
		{
			name: "all failures",
			counts: Counts{
				Requests:       10,
				TotalSuccesses: 0,
				TotalFailures:  10,
			},
			expected: 1.0,
		},
		{
			name: "mixed results",
			counts: Counts{
				Requests:       10,
				TotalSuccesses: 3,
				TotalFailures:  7,
			},
			expected: 0.7,
		},
		{
			name: "partial requests",
			counts: Counts{
				Requests:       5,
				TotalSuccesses: 2,
				TotalFailures:  3,
			},
			expected: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := tt.counts.FailureRatio()
			assert.InDelta(t, tt.expected, ratio, 0.001)
		})
	}
}

func TestCounts_SuccessRatio(t *testing.T) {
	tests := []struct {
		name     string
		counts   Counts
		expected float64
	}{
		{
			name:     "no requests",
			counts:   Counts{},
			expected: 0.0,
		},
		{
			name: "all successes",
			counts: Counts{
				Requests:       10,
				TotalSuccesses: 10,
				TotalFailures:  0,
			},
			expected: 1.0,
		},
		{
			name: "all failures",
			counts: Counts{
				Requests:       10,
				TotalSuccesses: 0,
				TotalFailures:  10,
			},
			expected: 0.0,
		},
		{
			name: "mixed results",
			counts: Counts{
				Requests:       10,
				TotalSuccesses: 3,
				TotalFailures:  7,
			},
			expected: 0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := tt.counts.SuccessRatio()
			assert.InDelta(t, tt.expected, ratio, 0.001)
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("test-service")

	assert.Equal(t, "test-service", config.Name)
	assert.Equal(t, uint32(1), config.MaxRequests)
	assert.Equal(t, 60*time.Second, config.Interval)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, uint32(5), config.FailureThreshold)
	assert.Equal(t, 0.6, config.FailureRatio)
	assert.Equal(t, uint32(3), config.MinimumRequestThreshold)
	assert.Nil(t, config.ReadyToTrip)
	assert.Nil(t, config.OnStateChange)
	assert.NotNil(t, config.IsSuccessful)

	// Test default IsSuccessful function
	assert.True(t, config.IsSuccessful(nil))
	assert.False(t, config.IsSuccessful(errors.New("error")))
}

func TestHTTPClientConfig(t *testing.T) {
	config := HTTPClientConfig("http-client")

	assert.Equal(t, "http-client", config.Name)
	assert.Equal(t, uint32(3), config.MaxRequests)
	assert.Equal(t, 30*time.Second, config.Interval)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, uint32(3), config.FailureThreshold)
	assert.Equal(t, 0.5, config.FailureRatio)
	assert.Equal(t, uint32(5), config.MinimumRequestThreshold)
	assert.NotNil(t, config.IsSuccessful)
}

func TestDatabaseConfig(t *testing.T) {
	config := DatabaseConfig("database")

	assert.Equal(t, "database", config.Name)
	assert.Equal(t, uint32(1), config.MaxRequests)
	assert.Equal(t, 120*time.Second, config.Interval)
	assert.Equal(t, 60*time.Second, config.Timeout)
	assert.Equal(t, uint32(3), config.FailureThreshold)
	assert.Equal(t, 0.7, config.FailureRatio)
	assert.Equal(t, uint32(2), config.MinimumRequestThreshold)
	assert.NotNil(t, config.IsSuccessful)
}

func TestExternalServiceConfig(t *testing.T) {
	config := ExternalServiceConfig("external-api")

	assert.Equal(t, "external-api", config.Name)
	assert.Equal(t, uint32(2), config.MaxRequests)
	assert.Equal(t, 45*time.Second, config.Interval)
	assert.Equal(t, 20*time.Second, config.Timeout)
	assert.Equal(t, uint32(5), config.FailureThreshold)
	assert.Equal(t, 0.4, config.FailureRatio)
	assert.Equal(t, uint32(10), config.MinimumRequestThreshold)
	assert.NotNil(t, config.IsSuccessful)
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		config := DefaultConfig("test")
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("empty name", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Name = ""
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidName, err)
	})

	t.Run("zero max requests", func(t *testing.T) {
		config := DefaultConfig("test")
		config.MaxRequests = 0
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidMaxRequests, err)
	})

	t.Run("zero timeout", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Timeout = 0
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidTimeout, err)
	})

	t.Run("negative timeout", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Timeout = -1 * time.Second
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidTimeout, err)
	})

	t.Run("negative interval", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Interval = -1 * time.Second
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidInterval, err)
	})

	t.Run("zero interval is valid", func(t *testing.T) {
		config := DefaultConfig("test")
		config.Interval = 0
		
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid failure ratio - negative", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureRatio = -0.1
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidFailureRatio, err)
	})

	t.Run("invalid failure ratio - greater than 1", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureRatio = 1.1
		
		err := config.Validate()
		assert.Equal(t, ErrInvalidFailureRatio, err)
	})

	t.Run("failure ratio 0.0 is valid", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureRatio = 0.0
		config.FailureThreshold = 5 // Need some criteria
		
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("failure ratio 1.0 is valid", func(t *testing.T) {
		config := DefaultConfig("test")
		config.FailureRatio = 1.0
		
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("no failure criteria", func(t *testing.T) {
		config := DefaultConfig("test")
		config.ReadyToTrip = nil
		config.FailureThreshold = 0
		config.FailureRatio = 0
		
		err := config.Validate()
		assert.Equal(t, ErrNoFailureCriteria, err)
	})

	t.Run("custom ReadyToTrip with no other criteria", func(t *testing.T) {
		config := DefaultConfig("test")
		config.ReadyToTrip = func(counts Counts) bool { return false }
		config.FailureThreshold = 0
		config.FailureRatio = 0
		
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("only failure threshold", func(t *testing.T) {
		config := DefaultConfig("test")
		config.ReadyToTrip = nil
		config.FailureThreshold = 3
		config.FailureRatio = 0
		
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("only failure ratio", func(t *testing.T) {
		config := DefaultConfig("test")
		config.ReadyToTrip = nil
		config.FailureThreshold = 0
		config.FailureRatio = 0.5
		
		err := config.Validate()
		assert.NoError(t, err)
	})
}

func TestConfig_WithStateChangeCallback(t *testing.T) {
	config := DefaultConfig("test")
	assert.Nil(t, config.OnStateChange)

	called := false
	callback := func(name string, from State, to State) {
		called = true
	}

	result := config.WithStateChangeCallback(callback)
	
	// Should return same config instance
	assert.Equal(t, config, result)
	assert.NotNil(t, config.OnStateChange)

	// Test callback is working
	config.OnStateChange("test", StateClosed, StateOpen)
	assert.True(t, called)
}

func TestConfig_WithSuccessFunction(t *testing.T) {
	config := DefaultConfig("test")
	
	// Default function
	assert.True(t, config.IsSuccessful(nil))
	assert.False(t, config.IsSuccessful(errors.New("error")))

	// Custom function
	customFunc := func(err error) bool {
		return err == nil || err.Error() == "acceptable"
	}

	result := config.WithSuccessFunction(customFunc)
	
	// Should return same config instance
	assert.Equal(t, config, result)
	
	// Test custom function
	assert.True(t, config.IsSuccessful(nil))
	assert.True(t, config.IsSuccessful(errors.New("acceptable")))
	assert.False(t, config.IsSuccessful(errors.New("unacceptable")))
}

func TestConfig_WithReadyToTrip(t *testing.T) {
	config := DefaultConfig("test")
	assert.Nil(t, config.ReadyToTrip)

	customTrip := func(counts Counts) bool {
		return counts.ConsecutiveFailures >= 2
	}

	result := config.WithReadyToTrip(customTrip)
	
	// Should return same config instance
	assert.Equal(t, config, result)
	assert.NotNil(t, config.ReadyToTrip)

	// Test custom function
	counts1 := Counts{ConsecutiveFailures: 1}
	counts2 := Counts{ConsecutiveFailures: 2}
	
	assert.False(t, config.ReadyToTrip(counts1))
	assert.True(t, config.ReadyToTrip(counts2))
}

func TestConfig_ChainedMethods(t *testing.T) {
	called := false
	
	config := DefaultConfig("test").
		WithStateChangeCallback(func(name string, from State, to State) {
			called = true
		}).
		WithSuccessFunction(func(err error) bool {
			return err == nil
		}).
		WithReadyToTrip(func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3
		})

	assert.NotNil(t, config.OnStateChange)
	assert.NotNil(t, config.IsSuccessful)
	assert.NotNil(t, config.ReadyToTrip)

	// Test all functions work
	config.OnStateChange("test", StateClosed, StateOpen)
	assert.True(t, called)
	
	assert.True(t, config.IsSuccessful(nil))
	assert.False(t, config.IsSuccessful(errors.New("error")))
	
	assert.False(t, config.ReadyToTrip(Counts{ConsecutiveFailures: 2}))
	assert.True(t, config.ReadyToTrip(Counts{ConsecutiveFailures: 3}))
}

func TestCounts_Methods(t *testing.T) {
	t.Run("onRequest", func(t *testing.T) {
		counts := &Counts{}
		before := time.Now()
		
		counts.onRequest()
		
		assert.Equal(t, uint32(1), counts.Requests)
		assert.True(t, counts.LastRequestTime.After(before) || counts.LastRequestTime.Equal(before))
	})

	t.Run("onSuccess", func(t *testing.T) {
		counts := &Counts{
			ConsecutiveFailures: 3,
		}
		
		counts.onSuccess()
		
		assert.Equal(t, uint32(1), counts.TotalSuccesses)
		assert.Equal(t, uint32(1), counts.ConsecutiveSuccesses)
		assert.Equal(t, uint32(0), counts.ConsecutiveFailures)
	})

	t.Run("onFailure", func(t *testing.T) {
		counts := &Counts{
			ConsecutiveSuccesses: 3,
		}
		
		counts.onFailure()
		
		assert.Equal(t, uint32(1), counts.TotalFailures)
		assert.Equal(t, uint32(1), counts.ConsecutiveFailures)
		assert.Equal(t, uint32(0), counts.ConsecutiveSuccesses)
	})

	t.Run("clear", func(t *testing.T) {
		counts := &Counts{
			Requests:             10,
			TotalSuccesses:       5,
			TotalFailures:        5,
			ConsecutiveSuccesses: 2,
			ConsecutiveFailures:  3,
			LastRequestTime:      time.Now(),
		}
		
		counts.clear()
		
		assert.Equal(t, uint32(0), counts.Requests)
		assert.Equal(t, uint32(0), counts.TotalSuccesses)
		assert.Equal(t, uint32(0), counts.TotalFailures)
		assert.Equal(t, uint32(0), counts.ConsecutiveSuccesses)
		assert.Equal(t, uint32(0), counts.ConsecutiveFailures)
		assert.True(t, counts.LastRequestTime.IsZero())
	})
}

func TestCounts_Sequential(t *testing.T) {
	counts := &Counts{}

	// Simulate requests and responses
	counts.onRequest()
	counts.onSuccess()
	
	counts.onRequest()
	counts.onFailure()
	
	counts.onRequest()
	counts.onFailure()
	
	counts.onRequest()
	counts.onSuccess()

	assert.Equal(t, uint32(4), counts.Requests)
	assert.Equal(t, uint32(2), counts.TotalSuccesses)
	assert.Equal(t, uint32(2), counts.TotalFailures)
	assert.Equal(t, uint32(1), counts.ConsecutiveSuccesses)
	assert.Equal(t, uint32(0), counts.ConsecutiveFailures)
	
	assert.Equal(t, 0.5, counts.FailureRatio())
	assert.Equal(t, 0.5, counts.SuccessRatio())
}

// Benchmark tests
func BenchmarkCounts_FailureRatio(b *testing.B) {
	counts := Counts{
		Requests:       1000,
		TotalSuccesses: 700,
		TotalFailures:  300,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counts.FailureRatio()
	}
}

func BenchmarkCounts_SuccessRatio(b *testing.B) {
	counts := Counts{
		Requests:       1000,
		TotalSuccesses: 700,
		TotalFailures:  300,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counts.SuccessRatio()
	}
}

func BenchmarkConfig_Validate(b *testing.B) {
	config := DefaultConfig("benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.Validate()
	}
}