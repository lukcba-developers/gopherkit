package cache

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	assert.Equal(t, "localhost", config.RedisHost)
	assert.Equal(t, "6379", config.RedisPort)
	assert.Equal(t, "", config.RedisPassword)
	assert.Equal(t, 0, config.RedisDB)
	assert.Equal(t, 10, config.PoolSize)
	assert.Equal(t, 2, config.MinIdleConns)
	assert.Equal(t, 30*time.Minute, config.MaxConnAge)
	assert.Equal(t, 4*time.Second, config.PoolTimeout)
	assert.Equal(t, 5*time.Minute, config.IdleTimeout)
	assert.Equal(t, 3*time.Second, config.ReadTimeout)
	assert.Equal(t, 3*time.Second, config.WriteTimeout)
	assert.Equal(t, 5*time.Second, config.DialTimeout)
	assert.Equal(t, time.Hour, config.DefaultExpiration)
	assert.Equal(t, "gopherkit", config.KeyPrefix)
	assert.False(t, config.EnableCompression)
	assert.True(t, config.EnableMetrics)
	assert.True(t, config.EnableMemoryFallback)
	assert.Equal(t, 1000, config.MemoryCacheSize)
}

func TestNewUnifiedCache(t *testing.T) {
	logger := logrus.New()

	t.Run("with default config", func(t *testing.T) {
		// Use memory-only config to avoid Redis dependency
		config := DefaultCacheConfig()
		config.EnableMemoryFallback = true
		
		cache, err := NewUnifiedCache(config, logger)
		require.NoError(t, err)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.config)
		assert.NotNil(t, cache.memoryCache)
		assert.NotNil(t, cache.metrics)
		
		// Clean up
		cache.Close()
	})

	t.Run("with nil config", func(t *testing.T) {
		cache, err := NewUnifiedCache(nil, logger)
		require.NoError(t, err)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.config)
		
		// Clean up
		cache.Close()
	})

	t.Run("with memory fallback disabled and no Redis", func(t *testing.T) {
		config := DefaultCacheConfig()
		config.EnableMemoryFallback = false
		config.RedisHost = "nonexistent-host"
		config.DialTimeout = 100 * time.Millisecond
		
		cache, err := NewUnifiedCache(config, logger)
		assert.Error(t, err)
		assert.Nil(t, cache)
	})
}

func TestUnifiedCache_KeyPrefixing(t *testing.T) {
	config := DefaultCacheConfig()
	config.KeyPrefix = "test"
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()

	assert.Equal(t, "test:mykey", cache.getKey("mykey"))
	
	// Test with empty prefix
	cache.config.KeyPrefix = ""
	assert.Equal(t, "mykey", cache.getKey("mykey"))
}

func TestUnifiedCache_MemoryOperations(t *testing.T) {
	// Create memory-only cache
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	t.Run("set and get", func(t *testing.T) {
		key := "test_key"
		value := "test_value"
		
		err := cache.Set(ctx, key, value, time.Minute)
		assert.NoError(t, err)

		var result string
		err = cache.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		var result string
		err := cache.Get(ctx, "non_existent", &result)
		assert.Equal(t, ErrCacheMiss, err)
	})

	t.Run("exists", func(t *testing.T) {
		key := "exists_test"
		value := "value"
		
		// Key doesn't exist initially
		assert.False(t, cache.Exists(ctx, key))
		
		// Set key
		err := cache.Set(ctx, key, value, time.Minute)
		assert.NoError(t, err)
		
		// Key exists now
		assert.True(t, cache.Exists(ctx, key))
	})

	t.Run("delete", func(t *testing.T) {
		key := "delete_test"
		value := "value"
		
		// Set key
		err := cache.Set(ctx, key, value, time.Minute)
		assert.NoError(t, err)
		
		// Verify it exists
		assert.True(t, cache.Exists(ctx, key))
		
		// Delete key
		err = cache.Delete(ctx, key)
		assert.NoError(t, err)
		
		// Verify it's gone
		assert.False(t, cache.Exists(ctx, key))
	})

	t.Run("set with default expiration", func(t *testing.T) {
		key := "default_expiration_test"
		value := "value"
		
		// Set with 0 expiration (should use default)
		err := cache.Set(ctx, key, value, 0)
		assert.NoError(t, err)

		var result string
		err = cache.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})
}

func TestUnifiedCache_MultipleOperations(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	t.Run("get multiple", func(t *testing.T) {
		// Set some test data
		testData := map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		}

		for key, value := range testData {
			err := cache.Set(ctx, key, value, time.Minute)
			assert.NoError(t, err)
		}

		// Get multiple keys
		keys := []string{"key1", "key2", "key3", "nonexistent"}
		results, err := cache.GetMultiple(ctx, keys)
		assert.NoError(t, err)
		
		assert.Equal(t, "value1", results["key1"])
		assert.Equal(t, float64(42), results["key2"]) // JSON unmarshaling converts to float64
		assert.Equal(t, true, results["key3"])
		assert.NotContains(t, results, "nonexistent")
	})

	t.Run("get multiple empty keys", func(t *testing.T) {
		results, err := cache.GetMultiple(ctx, []string{})
		assert.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("set multiple", func(t *testing.T) {
		items := map[string]interface{}{
			"multi1": "value1",
			"multi2": 123,
			"multi3": []string{"a", "b", "c"},
		}

		err := cache.SetMultiple(ctx, items, time.Minute)
		assert.NoError(t, err)

		// Verify all items were set
		for key, expectedValue := range items {
			var result interface{}
			err := cache.Get(ctx, key, &result)
			assert.NoError(t, err)
			
			// Handle different types appropriately
			switch expectedValue.(type) {
			case int:
				assert.Equal(t, float64(expectedValue.(int)), result)
			case []string:
				// JSON unmarshaling converts []string to []interface{}
				resultSlice, ok := result.([]interface{})
				assert.True(t, ok)
				expectedSlice := expectedValue.([]string)
				assert.Len(t, resultSlice, len(expectedSlice))
				for i, expected := range expectedSlice {
					assert.Equal(t, expected, resultSlice[i])
				}
			default:
				assert.Equal(t, expectedValue, result)
			}
		}
	})

	t.Run("set multiple empty", func(t *testing.T) {
		err := cache.SetMultiple(ctx, map[string]interface{}{}, time.Minute)
		assert.NoError(t, err)
	})
}

func TestUnifiedCache_AtomicOperations(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	t.Run("set nx - new key", func(t *testing.T) {
		key := "setnx_new"
		value := "value"
		
		success, err := cache.SetNX(ctx, key, value, time.Minute)
		assert.NoError(t, err)
		assert.True(t, success)

		var result string
		err = cache.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("set nx - existing key", func(t *testing.T) {
		key := "setnx_existing"
		value1 := "value1"
		value2 := "value2"
		
		// Set initial value
		success, err := cache.SetNX(ctx, key, value1, time.Minute)
		assert.NoError(t, err)
		assert.True(t, success)
		
		// Try to set again - should fail
		success, err = cache.SetNX(ctx, key, value2, time.Minute)
		assert.NoError(t, err)
		assert.False(t, success)

		// Verify original value is still there
		var result string
		err = cache.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value1, result)
	})

	t.Run("increment new key", func(t *testing.T) {
		key := "incr_new"
		
		result, err := cache.Increment(ctx, key, 5)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), result)
	})

	t.Run("increment existing key", func(t *testing.T) {
		key := "incr_existing"
		
		// Set initial value
		err := cache.Set(ctx, key, int64(10), time.Minute)
		assert.NoError(t, err)
		
		// Increment
		result, err := cache.Increment(ctx, key, 3)
		assert.NoError(t, err)
		assert.Equal(t, int64(13), result)
	})
}

func TestUnifiedCache_PatternOperations(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	config.KeyPrefix = ""  // Disable prefix for easier pattern testing
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	t.Run("invalidate pattern", func(t *testing.T) {
		// Set some test keys
		testKeys := []string{
			"user:123:profile",
			"user:123:settings",
			"user:456:profile",
			"cache:data:item1",
			"cache:data:item2",
		}

		for _, key := range testKeys {
			err := cache.Set(ctx, key, "value", time.Minute)
			assert.NoError(t, err)
		}

		// Verify all keys exist
		for _, key := range testKeys {
			assert.True(t, cache.Exists(ctx, key))
		}

		// Invalidate user:123:* pattern
		err := cache.InvalidatePattern(ctx, "user:123:*")
		assert.NoError(t, err)

		// Verify pattern-matched keys are gone
		assert.False(t, cache.Exists(ctx, "user:123:profile"))
		assert.False(t, cache.Exists(ctx, "user:123:settings"))
		
		// Verify other keys still exist
		assert.True(t, cache.Exists(ctx, "user:456:profile"))
		assert.True(t, cache.Exists(ctx, "cache:data:item1"))
		assert.True(t, cache.Exists(ctx, "cache:data:item2"))
	})
}

func TestUnifiedCache_InfoOperations(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	t.Run("get ttl", func(t *testing.T) {
		key := "ttl_test"
		value := "value"
		expiration := 30 * time.Second
		
		// Set with expiration
		err := cache.Set(ctx, key, value, expiration)
		assert.NoError(t, err)
		
		// Get TTL
		ttl := cache.GetTTL(ctx, key)
		assert.True(t, ttl > 0)
		assert.True(t, ttl <= expiration)
		
		// Non-existent key should return 0
		ttl = cache.GetTTL(ctx, "nonexistent")
		assert.Equal(t, time.Duration(0), ttl)
	})

	t.Run("ping", func(t *testing.T) {
		err := cache.Ping(ctx)
		assert.NoError(t, err)
	})

	t.Run("get stats", func(t *testing.T) {
		// Perform some operations to generate stats
		err := cache.Set(ctx, "stats_test", "value", time.Minute)
		assert.NoError(t, err)
		
		var result string
		err = cache.Get(ctx, "stats_test", &result)
		assert.NoError(t, err)

		stats, err := cache.GetStats(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
		
		// Should have memory stats
		assert.Contains(t, stats, "memory")
		
		// Should have metrics if enabled
		if config.EnableMetrics {
			assert.Contains(t, stats, "metrics")
		}
	})
}

func TestUnifiedCache_ErrorScenarios(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("no cache backends available", func(t *testing.T) {
		// Create cache with no backends
		emptyCache := &UnifiedCache{
			config: config,
			logger: logrus.NewEntry(logger),
		}

		// SetNX should fail
		success, err := emptyCache.SetNX(ctx, "key", "value", time.Minute)
		assert.Error(t, err)
		assert.False(t, success)

		// Increment should fail
		result, err := emptyCache.Increment(ctx, "key", 1)
		assert.Error(t, err)
		assert.Equal(t, int64(0), result)

		// Ping should fail
		err = emptyCache.Ping(ctx)
		assert.Error(t, err)
	})
}

func TestUnifiedCache_AvailableCacheCount(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()

	// With memory cache only (Redis connection will likely fail in test)
	count := cache.getAvailableCacheCount()
	assert.True(t, count >= 1) // At least memory cache should be available

	// Test with no caches
	cache.redisClient = nil
	cache.memoryCache = nil
	count = cache.getAvailableCacheCount()
	assert.Equal(t, 0, count)
}

func TestUnifiedCache_MetricsRecording(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	config.EnableMetrics = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	// Perform operations that should record metrics
	cache.Set(ctx, "metrics_test", "value", time.Minute)
	
	var result string
	cache.Get(ctx, "metrics_test", &result)
	cache.Get(ctx, "nonexistent", &result)
	cache.Delete(ctx, "metrics_test")

	// Get stats to verify metrics were recorded
	stats, err := cache.GetStats(ctx)
	assert.NoError(t, err)
	assert.Contains(t, stats, "metrics")
}

func TestUnifiedCache_ComplexDataTypes(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(t, err)
	defer cache.Close()
	
	// Force memory-only by setting redisClient to nil
	cache.redisClient = nil

	ctx := context.Background()

	t.Run("struct data", func(t *testing.T) {
		type TestStruct struct {
			Name  string `json:"name"`
			Age   int    `json:"age"`
			Email string `json:"email"`
		}

		original := TestStruct{
			Name:  "John Doe",
			Age:   30,
			Email: "john@example.com",
		}

		err := cache.Set(ctx, "struct_test", original, time.Minute)
		assert.NoError(t, err)

		var result TestStruct
		err = cache.Get(ctx, "struct_test", &result)
		assert.NoError(t, err)
		assert.Equal(t, original, result)
	})

	t.Run("slice data", func(t *testing.T) {
		original := []string{"apple", "banana", "cherry"}

		err := cache.Set(ctx, "slice_test", original, time.Minute)
		assert.NoError(t, err)

		var result []string
		err = cache.Get(ctx, "slice_test", &result)
		assert.NoError(t, err)
		assert.Equal(t, original, result)
	})

	t.Run("map data", func(t *testing.T) {
		original := map[string]int{
			"apple":  1,
			"banana": 2,
			"cherry": 3,
		}

		err := cache.Set(ctx, "map_test", original, time.Minute)
		assert.NoError(t, err)

		var result map[string]int
		err = cache.Get(ctx, "map_test", &result)
		assert.NoError(t, err)
		assert.Equal(t, original, result)
	})
}

// Benchmark tests
func BenchmarkUnifiedCache_Set(b *testing.B) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(b, err)
	defer cache.Close()
	
	// Force memory-only
	cache.redisClient = nil

	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(ctx, "benchmark_key", "benchmark_value", time.Minute)
	}
}

func BenchmarkUnifiedCache_Get(b *testing.B) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(b, err)
	defer cache.Close()
	
	// Force memory-only
	cache.redisClient = nil

	ctx := context.Background()
	
	// Pre-populate cache
	cache.Set(ctx, "benchmark_key", "benchmark_value", time.Minute)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result string
		cache.Get(ctx, "benchmark_key", &result)
	}
}

func BenchmarkUnifiedCache_SetMultiple(b *testing.B) {
	config := DefaultCacheConfig()
	config.EnableMemoryFallback = true
	
	logger := logrus.New()
	cache, err := NewUnifiedCache(config, logger)
	require.NoError(b, err)
	defer cache.Close()
	
	// Force memory-only
	cache.redisClient = nil

	ctx := context.Background()
	
	items := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetMultiple(ctx, items, time.Minute)
	}
}