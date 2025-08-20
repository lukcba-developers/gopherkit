package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewBasicCacheMetrics(t *testing.T) {
	metrics := NewBasicCacheMetrics()
	
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.operations)
	assert.True(t, metrics.startTime.Before(time.Now()) || metrics.startTime.Equal(time.Now()))
}

func TestCacheMetrics_RecordOperation(t *testing.T) {
	metrics := NewBasicCacheMetrics()

	t.Run("record new operation", func(t *testing.T) {
		metrics.RecordOperation("get", "test_key")
		
		assert.Contains(t, metrics.operations, "get")
		assert.Equal(t, int64(1), metrics.operations["get"].Count)
		assert.Equal(t, int64(0), metrics.operations["get"].Errors)
		assert.True(t, metrics.operations["get"].LastTime.After(time.Time{}))
	})

	t.Run("record existing operation", func(t *testing.T) {
		metrics.RecordOperation("get", "another_key")
		
		assert.Equal(t, int64(2), metrics.operations["get"].Count)
	})

	t.Run("record different operation", func(t *testing.T) {
		metrics.RecordOperation("set", "test_key")
		
		assert.Contains(t, metrics.operations, "set")
		assert.Equal(t, int64(1), metrics.operations["set"].Count)
		
		// Original operation should still exist
		assert.Equal(t, int64(2), metrics.operations["get"].Count)
	})
}

func TestCacheMetrics_RecordOperationWithDuration(t *testing.T) {
	metrics := NewBasicCacheMetrics()

	t.Run("record new operation with duration", func(t *testing.T) {
		duration := 100 * time.Millisecond
		metrics.RecordOperationWithDuration("slow_get", "test_key", duration)
		
		assert.Contains(t, metrics.operations, "slow_get")
		op := metrics.operations["slow_get"]
		assert.Equal(t, int64(1), op.Count)
		assert.Equal(t, duration, op.TotalTime)
		assert.True(t, op.LastTime.After(time.Time{}))
	})

	t.Run("record existing operation with duration", func(t *testing.T) {
		duration := 50 * time.Millisecond
		metrics.RecordOperationWithDuration("slow_get", "another_key", duration)
		
		op := metrics.operations["slow_get"]
		assert.Equal(t, int64(2), op.Count)
		assert.Equal(t, 150*time.Millisecond, op.TotalTime) // 100ms + 50ms
	})
}

func TestCacheMetrics_RecordError(t *testing.T) {
	metrics := NewBasicCacheMetrics()

	t.Run("record error for new operation", func(t *testing.T) {
		metrics.RecordError("failed_get")
		
		assert.Contains(t, metrics.operations, "failed_get")
		assert.Equal(t, int64(1), metrics.operations["failed_get"].Errors)
		assert.Equal(t, int64(0), metrics.operations["failed_get"].Count)
	})

	t.Run("record error for existing operation", func(t *testing.T) {
		// First record a successful operation
		metrics.RecordOperation("mixed_op", "test_key")
		assert.Equal(t, int64(1), metrics.operations["mixed_op"].Count)
		assert.Equal(t, int64(0), metrics.operations["mixed_op"].Errors)
		
		// Then record an error
		metrics.RecordError("mixed_op")
		assert.Equal(t, int64(1), metrics.operations["mixed_op"].Count)
		assert.Equal(t, int64(1), metrics.operations["mixed_op"].Errors)
	})

	t.Run("record multiple errors", func(t *testing.T) {
		metrics.RecordError("failed_get")
		
		assert.Equal(t, int64(2), metrics.operations["failed_get"].Errors)
	})
}

func TestCacheMetrics_GetStats(t *testing.T) {
	metrics := NewBasicCacheMetrics()

	t.Run("empty stats", func(t *testing.T) {
		stats := metrics.GetStats()
		
		assert.Contains(t, stats, "uptime")
		assert.Contains(t, stats, "operations")
		
		operations := stats["operations"].(map[string]interface{})
		assert.Empty(t, operations)
	})

	t.Run("stats with data", func(t *testing.T) {
		// Record some operations
		metrics.RecordOperation("get", "key1")
		metrics.RecordOperation("get", "key2")
		metrics.RecordOperationWithDuration("set", "key1", 50*time.Millisecond)
		metrics.RecordOperationWithDuration("set", "key2", 100*time.Millisecond)
		metrics.RecordError("get")
		
		stats := metrics.GetStats()
		
		assert.Contains(t, stats, "uptime")
		assert.Contains(t, stats, "operations")
		
		operations := stats["operations"].(map[string]interface{})
		assert.Contains(t, operations, "get")
		assert.Contains(t, operations, "set")
		
		// Check get operation stats
		getStats := operations["get"].(map[string]interface{})
		assert.Equal(t, int64(2), getStats["count"])
		assert.Equal(t, int64(1), getStats["errors"])
		assert.Equal(t, "0s", getStats["total_time"]) // No duration recorded for get
		assert.Equal(t, "0s", getStats["avg_duration"])
		assert.Contains(t, getStats, "last_time")
		assert.Equal(t, 1.0/3.0, getStats["error_rate"]) // 1 error out of 3 total (2 success + 1 error)
		
		// Check set operation stats
		setStats := operations["set"].(map[string]interface{})
		assert.Equal(t, int64(2), setStats["count"])
		assert.Equal(t, int64(0), setStats["errors"])
		assert.Equal(t, "150ms", setStats["total_time"]) // 50ms + 100ms
		assert.Equal(t, "75ms", setStats["avg_duration"]) // 150ms / 2
		assert.Contains(t, setStats, "last_time")
		assert.Equal(t, 0.0, setStats["error_rate"])
	})
}

func TestCacheMetrics_CalculateErrorRate(t *testing.T) {

	t.Run("no operations", func(t *testing.T) {
		opMetrics := &OperationMetrics{
			Count:  0,
			Errors: 0,
		}
		rate := calculateErrorRate(opMetrics)
		assert.Equal(t, 0.0, rate)
	})

	t.Run("only successes", func(t *testing.T) {
		opMetrics := &OperationMetrics{
			Count:  10,
			Errors: 0,
		}
		rate := calculateErrorRate(opMetrics)
		assert.Equal(t, 0.0, rate)
	})

	t.Run("only errors", func(t *testing.T) {
		opMetrics := &OperationMetrics{
			Count:  0,
			Errors: 5,
		}
		rate := calculateErrorRate(opMetrics)
		assert.Equal(t, 1.0, rate)
	})

	t.Run("mixed success and errors", func(t *testing.T) {
		opMetrics := &OperationMetrics{
			Count:  7,  // 7 successes
			Errors: 3,  // 3 errors
		}
		rate := calculateErrorRate(opMetrics)
		assert.Equal(t, 0.3, rate) // 3/10
	})
}

func TestCacheMetrics_Reset(t *testing.T) {
	metrics := NewBasicCacheMetrics()
	
	// Record some operations
	metrics.RecordOperation("get", "key1")
	metrics.RecordOperation("set", "key2")
	metrics.RecordError("get")
	
	// Verify data exists
	assert.Len(t, metrics.operations, 2)
	assert.Equal(t, int64(1), metrics.operations["get"].Count)
	assert.Equal(t, int64(1), metrics.operations["get"].Errors)
	assert.Equal(t, int64(1), metrics.operations["set"].Count)
	
	oldStartTime := metrics.startTime
	
	// Wait a moment to ensure reset changes the start time
	time.Sleep(1 * time.Millisecond)
	
	// Reset
	metrics.Reset()
	
	// Verify reset
	assert.Empty(t, metrics.operations)
	assert.True(t, metrics.startTime.After(oldStartTime))
}

func TestCacheMetrics_ConcurrentAccess(t *testing.T) {
	metrics := NewBasicCacheMetrics()
	
	// This test ensures no race conditions exist when accessing metrics concurrently
	// Run multiple goroutines that record operations
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					metrics.RecordOperation("test_op", "key")
				} else {
					metrics.RecordError("test_op")
				}
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify final counts
	stats := metrics.GetStats()
	operations := stats["operations"].(map[string]interface{})
	testOpStats := operations["test_op"].(map[string]interface{})
	
	// Each goroutine records 50 operations and 50 errors (100 total each)
	// 10 goroutines = 500 operations and 500 errors
	assert.Equal(t, int64(500), testOpStats["count"])
	assert.Equal(t, int64(500), testOpStats["errors"])
}

func TestCacheMetrics_AverageDurationCalculation(t *testing.T) {
	metrics := NewBasicCacheMetrics()

	t.Run("single operation with duration", func(t *testing.T) {
		duration := 100 * time.Millisecond
		metrics.RecordOperationWithDuration("single_op", "key", duration)
		
		stats := metrics.GetStats()
		operations := stats["operations"].(map[string]interface{})
		singleOpStats := operations["single_op"].(map[string]interface{})
		
		assert.Equal(t, "100ms", singleOpStats["avg_duration"])
		assert.Equal(t, "100ms", singleOpStats["total_time"])
	})

	t.Run("multiple operations with durations", func(t *testing.T) {
		metrics.RecordOperationWithDuration("multi_op", "key1", 50*time.Millisecond)
		metrics.RecordOperationWithDuration("multi_op", "key2", 150*time.Millisecond)
		metrics.RecordOperationWithDuration("multi_op", "key3", 100*time.Millisecond)
		
		stats := metrics.GetStats()
		operations := stats["operations"].(map[string]interface{})
		multiOpStats := operations["multi_op"].(map[string]interface{})
		
		assert.Equal(t, "100ms", multiOpStats["avg_duration"]) // (50+150+100)/3 = 100
		assert.Equal(t, "300ms", multiOpStats["total_time"])   // 50+150+100 = 300
	})

	t.Run("operations without duration", func(t *testing.T) {
		metrics.RecordOperation("no_duration_op", "key")
		
		stats := metrics.GetStats()
		operations := stats["operations"].(map[string]interface{})
		noDurationStats := operations["no_duration_op"].(map[string]interface{})
		
		assert.Equal(t, "0s", noDurationStats["avg_duration"])
		assert.Equal(t, "0s", noDurationStats["total_time"])
	})
}

func TestCacheMetrics_UptimeTracking(t *testing.T) {
	startTime := time.Now()
	metrics := NewBasicCacheMetrics()
	
	// Wait a small amount to ensure uptime is measurable
	time.Sleep(10 * time.Millisecond)
	
	stats := metrics.GetStats()
	uptime := stats["uptime"].(string)
	
	// Parse the uptime duration
	uptimeDuration, err := time.ParseDuration(uptime)
	assert.NoError(t, err)
	
	// Verify uptime is reasonable (should be > 10ms and < 1 second for this test)
	assert.True(t, uptimeDuration >= 10*time.Millisecond)
	assert.True(t, uptimeDuration < time.Second)
	
	// Verify start time is reasonable
	assert.True(t, metrics.startTime.After(startTime.Add(-time.Second)))
	assert.True(t, metrics.startTime.Before(time.Now()))
}

// Benchmark tests
func BenchmarkCacheMetrics_RecordOperation(b *testing.B) {
	metrics := NewBasicCacheMetrics()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordOperation("benchmark_op", "key")
	}
}

func BenchmarkCacheMetrics_RecordOperationWithDuration(b *testing.B) {
	metrics := NewBasicCacheMetrics()
	duration := 10 * time.Millisecond
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordOperationWithDuration("benchmark_op", "key", duration)
	}
}

func BenchmarkCacheMetrics_RecordError(b *testing.B) {
	metrics := NewBasicCacheMetrics()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordError("benchmark_op")
	}
}

func BenchmarkCacheMetrics_GetStats(b *testing.B) {
	metrics := NewBasicCacheMetrics()
	
	// Pre-populate with some data
	for i := 0; i < 10; i++ {
		metrics.RecordOperation("get", "key")
		metrics.RecordOperation("set", "key")
		metrics.RecordError("get")
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.GetStats()
	}
}