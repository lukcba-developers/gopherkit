package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryCacheItem(t *testing.T) {
	t.Run("is expired - no expiration", func(t *testing.T) {
		item := &MemoryCacheItem{
			Value:     "test",
			CreatedAt: time.Now(),
		}
		assert.False(t, item.IsExpired())
	})

	t.Run("is expired - not expired", func(t *testing.T) {
		item := &MemoryCacheItem{
			Value:      "test",
			Expiration: time.Now().Add(time.Hour),
			CreatedAt:  time.Now(),
		}
		assert.False(t, item.IsExpired())
	})

	t.Run("is expired - expired", func(t *testing.T) {
		item := &MemoryCacheItem{
			Value:      "test",
			Expiration: time.Now().Add(-time.Hour),
			CreatedAt:  time.Now().Add(-2 * time.Hour),
		}
		assert.True(t, item.IsExpired())
	})
}

func TestNewMemoryCache(t *testing.T) {
	cache := NewMemoryCache(100)
	
	assert.NotNil(t, cache)
	assert.Equal(t, 100, cache.maxSize)
	assert.NotNil(t, cache.items)
	assert.NotNil(t, cache.stats)
	assert.NotNil(t, cache.stopChan)
	
	// Clean up
	cache.Close()
}

func TestMemoryCache_BasicOperations(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	t.Run("set and get", func(t *testing.T) {
		key := "test_key"
		value := "test_value"
		
		err := cache.Set(key, value, time.Minute)
		assert.NoError(t, err)

		var result string
		err = cache.Get(key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		var result string
		err := cache.Get("non_existent", &result)
		assert.Equal(t, ErrCacheMiss, err)
	})

	t.Run("get expired key", func(t *testing.T) {
		key := "expired_key"
		value := "value"
		
		err := cache.Set(key, value, 1*time.Millisecond)
		assert.NoError(t, err)
		
		// Wait for expiration
		time.Sleep(2 * time.Millisecond)
		
		var result string
		err = cache.Get(key, &result)
		assert.Equal(t, ErrCacheMiss, err)
	})

	t.Run("set without expiration", func(t *testing.T) {
		key := "no_expiration"
		value := "value"
		
		err := cache.Set(key, value, 0)
		assert.NoError(t, err)

		var result string
		err = cache.Get(key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("delete", func(t *testing.T) {
		key := "delete_test"
		value := "value"
		
		err := cache.Set(key, value, time.Minute)
		assert.NoError(t, err)
		
		cache.Delete(key)
		
		var result string
		err = cache.Get(key, &result)
		assert.Equal(t, ErrCacheMiss, err)
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		// Should not panic or error
		cache.Delete("non_existent_key")
	})

	t.Run("exists", func(t *testing.T) {
		key := "exists_test"
		value := "value"
		
		// Key doesn't exist
		assert.False(t, cache.Exists(key))
		
		// Set key
		err := cache.Set(key, value, time.Minute)
		assert.NoError(t, err)
		
		// Key exists
		assert.True(t, cache.Exists(key))
		
		// Delete key
		cache.Delete(key)
		
		// Key doesn't exist anymore
		assert.False(t, cache.Exists(key))
	})

	t.Run("exists expired key", func(t *testing.T) {
		key := "exists_expired"
		value := "value"
		
		err := cache.Set(key, value, 1*time.Millisecond)
		assert.NoError(t, err)
		
		// Wait for expiration
		time.Sleep(2 * time.Millisecond)
		
		// Should return false and clean up expired key
		assert.False(t, cache.Exists(key))
	})
}

func TestMemoryCache_AtomicOperations(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	t.Run("set nx - new key", func(t *testing.T) {
		key := "setnx_new"
		value := "value"
		
		success := cache.SetNX(key, value, time.Minute)
		assert.True(t, success)

		var result string
		err := cache.Get(key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("set nx - existing key", func(t *testing.T) {
		key := "setnx_existing"
		value1 := "value1"
		value2 := "value2"
		
		// Set initial value
		success := cache.SetNX(key, value1, time.Minute)
		assert.True(t, success)
		
		// Try to set again - should fail
		success = cache.SetNX(key, value2, time.Minute)
		assert.False(t, success)

		// Verify original value is still there
		var result string
		err := cache.Get(key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value1, result)
	})

	t.Run("set nx - expired key", func(t *testing.T) {
		key := "setnx_expired"
		value1 := "value1"
		value2 := "value2"
		
		// Set with short expiration
		success := cache.SetNX(key, value1, 1*time.Millisecond)
		assert.True(t, success)
		
		// Wait for expiration
		time.Sleep(2 * time.Millisecond)
		
		// Should be able to set again
		success = cache.SetNX(key, value2, time.Minute)
		assert.True(t, success)

		var result string
		err := cache.Get(key, &result)
		assert.NoError(t, err)
		assert.Equal(t, value2, result)
	})

	t.Run("increment new key", func(t *testing.T) {
		key := "incr_new"
		
		result := cache.Increment(key, 5)
		assert.Equal(t, int64(5), result)
		
		// Verify value was set
		var value int64
		err := cache.Get(key, &value)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), value)
	})

	t.Run("increment existing int64", func(t *testing.T) {
		key := "incr_int64"
		
		err := cache.Set(key, int64(10), time.Minute)
		assert.NoError(t, err)
		
		result := cache.Increment(key, 3)
		assert.Equal(t, int64(13), result)
	})

	t.Run("increment existing int", func(t *testing.T) {
		key := "incr_int"
		
		err := cache.Set(key, 10, time.Minute)
		assert.NoError(t, err)
		
		result := cache.Increment(key, 3)
		assert.Equal(t, int64(13), result)
	})

	t.Run("increment existing float64", func(t *testing.T) {
		key := "incr_float64"
		
		err := cache.Set(key, 10.5, time.Minute)
		assert.NoError(t, err)
		
		result := cache.Increment(key, 3)
		assert.Equal(t, int64(13), result)
	})

	t.Run("increment non-numeric value", func(t *testing.T) {
		key := "incr_string"
		
		err := cache.Set(key, "not_a_number", time.Minute)
		assert.NoError(t, err)
		
		result := cache.Increment(key, 5)
		assert.Equal(t, int64(5), result)
		
		// Verify value was replaced
		var value int64
		err = cache.Get(key, &value)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), value)
	})

	t.Run("increment expired key", func(t *testing.T) {
		key := "incr_expired"
		
		err := cache.Set(key, int64(10), 1*time.Millisecond)
		assert.NoError(t, err)
		
		// Wait for expiration
		time.Sleep(2 * time.Millisecond)
		
		result := cache.Increment(key, 5)
		assert.Equal(t, int64(5), result)
	})
}

func TestMemoryCache_TTL(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	t.Run("get ttl existing key with expiration", func(t *testing.T) {
		key := "ttl_test"
		value := "value"
		expiration := 30 * time.Second
		
		err := cache.Set(key, value, expiration)
		assert.NoError(t, err)
		
		ttl := cache.GetTTL(key)
		assert.True(t, ttl > 0)
		assert.True(t, ttl <= expiration)
	})

	t.Run("get ttl existing key without expiration", func(t *testing.T) {
		key := "ttl_no_expiration"
		value := "value"
		
		err := cache.Set(key, value, 0)
		assert.NoError(t, err)
		
		ttl := cache.GetTTL(key)
		assert.Equal(t, time.Duration(-1), ttl)
	})

	t.Run("get ttl non-existent key", func(t *testing.T) {
		ttl := cache.GetTTL("non_existent")
		assert.Equal(t, time.Duration(0), ttl)
	})

	t.Run("get ttl expired key", func(t *testing.T) {
		key := "ttl_expired"
		value := "value"
		
		err := cache.Set(key, value, 1*time.Millisecond)
		assert.NoError(t, err)
		
		// Wait for expiration
		time.Sleep(2 * time.Millisecond)
		
		ttl := cache.GetTTL(key)
		assert.Equal(t, time.Duration(0), ttl)
	})
}

func TestMemoryCache_PatternMatching(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	// Set up test data
	testData := map[string]string{
		"user:123:profile":  "profile_data",
		"user:123:settings": "settings_data",
		"user:456:profile":  "profile_data_2",
		"cache:data:item1":  "item1_data",
		"cache:data:item2":  "item2_data",
		"exact_match":       "exact_data",
	}

	for key, value := range testData {
		err := cache.Set(key, value, time.Minute)
		assert.NoError(t, err)
	}

	t.Run("exact match pattern", func(t *testing.T) {
		cache.InvalidatePattern("exact_match")
		assert.False(t, cache.Exists("exact_match"))
		
		// Other keys should still exist
		assert.True(t, cache.Exists("user:123:profile"))
	})

	t.Run("prefix wildcard pattern", func(t *testing.T) {
		cache.InvalidatePattern("user:123:*")
		
		// Keys matching pattern should be gone
		assert.False(t, cache.Exists("user:123:profile"))
		assert.False(t, cache.Exists("user:123:settings"))
		
		// Other keys should still exist
		assert.True(t, cache.Exists("user:456:profile"))
		assert.True(t, cache.Exists("cache:data:item1"))
	})

	t.Run("suffix wildcard pattern", func(t *testing.T) {
		cache.InvalidatePattern("*:profile")
		
		// Keys matching pattern should be gone
		assert.False(t, cache.Exists("user:456:profile"))
		
		// Other keys should still exist
		assert.True(t, cache.Exists("cache:data:item1"))
		assert.True(t, cache.Exists("cache:data:item2"))
	})

	t.Run("middle wildcard pattern", func(t *testing.T) {
		cache.InvalidatePattern("cache:*:item1")
		
		// Key matching pattern should be gone
		assert.False(t, cache.Exists("cache:data:item1"))
		
		// Other keys should still exist
		assert.True(t, cache.Exists("cache:data:item2"))
	})
}

func TestMemoryCache_PatternMatchingEdgeCases(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	t.Run("matches pattern helper - exact match", func(t *testing.T) {
		assert.True(t, cache.matchesPattern("exact", "exact"))
		assert.False(t, cache.matchesPattern("exact", "different"))
	})

	t.Run("matches pattern helper - prefix wildcard", func(t *testing.T) {
		assert.True(t, cache.matchesPattern("user:123:profile", "user:123:*"))
		assert.True(t, cache.matchesPattern("user:123:", "user:123:*"))
		assert.False(t, cache.matchesPattern("user:456:profile", "user:123:*"))
	})

	t.Run("matches pattern helper - suffix wildcard", func(t *testing.T) {
		assert.True(t, cache.matchesPattern("user:123:profile", "*:profile"))
		assert.True(t, cache.matchesPattern(":profile", "*:profile"))
		assert.False(t, cache.matchesPattern("user:123:settings", "*:profile"))
	})

	t.Run("matches pattern helper - middle wildcard", func(t *testing.T) {
		assert.True(t, cache.matchesPattern("cache:data:item", "cache:*:item"))
		assert.True(t, cache.matchesPattern("cache::item", "cache:*:item"))
		assert.False(t, cache.matchesPattern("cache:data:other", "cache:*:item"))
		assert.False(t, cache.matchesPattern("other:data:item", "cache:*:item"))
	})

	t.Run("matches pattern helper - complex patterns", func(t *testing.T) {
		// Patterns with multiple wildcards should return false (not supported)
		assert.False(t, cache.matchesPattern("a:b:c:d", "a:*:c:*"))
	})
}

func TestMemoryCache_Eviction(t *testing.T) {
	// Create small cache to test eviction
	cache := NewMemoryCache(3)
	defer cache.Close()

	t.Run("evict oldest when full", func(t *testing.T) {
		// Fill cache to capacity
		err := cache.Set("key1", "value1", time.Minute)
		assert.NoError(t, err)
		
		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
		
		err = cache.Set("key2", "value2", time.Minute)
		assert.NoError(t, err)
		
		time.Sleep(1 * time.Millisecond)
		
		err = cache.Set("key3", "value3", time.Minute)
		assert.NoError(t, err)
		
		// All keys should exist
		assert.True(t, cache.Exists("key1"))
		assert.True(t, cache.Exists("key2"))
		assert.True(t, cache.Exists("key3"))
		
		// Add one more key - should evict the oldest (key1)
		time.Sleep(1 * time.Millisecond)
		err = cache.Set("key4", "value4", time.Minute)
		assert.NoError(t, err)
		
		// key1 should be evicted
		assert.False(t, cache.Exists("key1"))
		assert.True(t, cache.Exists("key2"))
		assert.True(t, cache.Exists("key3"))
		assert.True(t, cache.Exists("key4"))
	})
}

func TestMemoryCache_Stats(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	t.Run("stats tracking", func(t *testing.T) {
		// Perform various operations
		cache.Set("key1", "value1", time.Minute)
		cache.Set("key2", "value2", time.Minute)
		
		var result string
		cache.Get("key1", &result)  // hit
		cache.Get("nonexistent", &result)  // miss
		
		cache.Delete("key2")
		
		// Get stats
		stats := cache.GetStats()
		require.NotNil(t, stats)
		
		assert.Equal(t, int64(2), stats["sets"])
		assert.Equal(t, int64(1), stats["hits"])
		assert.Equal(t, int64(1), stats["misses"])
		assert.Equal(t, int64(1), stats["deletes"])
		assert.Equal(t, int64(0), stats["evictions"])
		assert.Equal(t, 1, stats["size"])
		assert.Equal(t, 100, stats["max_size"])
		assert.Equal(t, 0.5, stats["hit_ratio"])
	})

	t.Run("hit ratio with no requests", func(t *testing.T) {
		newCache := NewMemoryCache(100)
		defer newCache.Close()
		
		stats := newCache.GetStats()
		assert.Equal(t, 0.0, stats["hit_ratio"])
	})

	t.Run("eviction stats", func(t *testing.T) {
		smallCache := NewMemoryCache(2)
		defer smallCache.Close()
		
		// Fill beyond capacity to trigger eviction
		smallCache.Set("key1", "value1", time.Minute)
		time.Sleep(1 * time.Millisecond)
		smallCache.Set("key2", "value2", time.Minute)
		time.Sleep(1 * time.Millisecond)
		smallCache.Set("key3", "value3", time.Minute)  // Should evict key1
		
		stats := smallCache.GetStats()
		assert.Equal(t, int64(1), stats["evictions"])
	})
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	// Add some data
	cache.Set("key1", "value1", time.Minute)
	cache.Set("key2", "value2", time.Minute)
	
	assert.True(t, cache.Exists("key1"))
	assert.True(t, cache.Exists("key2"))
	
	// Clear cache
	cache.Clear()
	
	assert.False(t, cache.Exists("key1"))
	assert.False(t, cache.Exists("key2"))
	
	stats := cache.GetStats()
	assert.Equal(t, 0, stats["size"])
}

func TestMemoryCache_DataTypes(t *testing.T) {
	cache := NewMemoryCache(100)
	defer cache.Close()

	t.Run("string", func(t *testing.T) {
		err := cache.Set("string_key", "test_string", time.Minute)
		assert.NoError(t, err)

		var result string
		err = cache.Get("string_key", &result)
		assert.NoError(t, err)
		assert.Equal(t, "test_string", result)
	})

	t.Run("int", func(t *testing.T) {
		err := cache.Set("int_key", 42, time.Minute)
		assert.NoError(t, err)

		var result int
		err = cache.Get("int_key", &result)
		assert.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("bool", func(t *testing.T) {
		err := cache.Set("bool_key", true, time.Minute)
		assert.NoError(t, err)

		var result bool
		err = cache.Get("bool_key", &result)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("struct", func(t *testing.T) {
		type TestStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		original := TestStruct{Name: "John", Age: 30}
		err := cache.Set("struct_key", original, time.Minute)
		assert.NoError(t, err)

		var result TestStruct
		err = cache.Get("struct_key", &result)
		assert.NoError(t, err)
		assert.Equal(t, original, result)
	})

	t.Run("slice", func(t *testing.T) {
		original := []string{"apple", "banana", "cherry"}
		err := cache.Set("slice_key", original, time.Minute)
		assert.NoError(t, err)

		var result []string
		err = cache.Get("slice_key", &result)
		assert.NoError(t, err)
		assert.Equal(t, original, result)
	})

	t.Run("map", func(t *testing.T) {
		original := map[string]int{"a": 1, "b": 2}
		err := cache.Set("map_key", original, time.Minute)
		assert.NoError(t, err)

		var result map[string]int
		err = cache.Get("map_key", &result)
		assert.NoError(t, err)
		assert.Equal(t, original, result)
	})
}

func TestMemoryCacheStats_Methods(t *testing.T) {
	stats := &MemoryCacheStats{}

	// Test all recording methods
	stats.recordHit()
	stats.recordMiss()
	stats.recordSet()
	stats.recordDelete()
	stats.recordEviction()

	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(1), stats.Sets)
	assert.Equal(t, int64(1), stats.Deletes)
	assert.Equal(t, int64(1), stats.Evictions)
}

// Benchmark tests
func BenchmarkMemoryCache_Set(b *testing.B) {
	cache := NewMemoryCache(10000)
	defer cache.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("benchmark_key", "benchmark_value", time.Minute)
	}
}

func BenchmarkMemoryCache_Get(b *testing.B) {
	cache := NewMemoryCache(10000)
	defer cache.Close()
	
	// Pre-populate
	cache.Set("benchmark_key", "benchmark_value", time.Minute)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result string
		cache.Get("benchmark_key", &result)
	}
}

func BenchmarkMemoryCache_GetMiss(b *testing.B) {
	cache := NewMemoryCache(10000)
	defer cache.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result string
		cache.Get("nonexistent_key", &result)
	}
}

func BenchmarkMemoryCache_SetNX(b *testing.B) {
	cache := NewMemoryCache(10000)
	defer cache.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetNX("benchmark_key", "benchmark_value", time.Minute)
	}
}

func BenchmarkMemoryCache_Increment(b *testing.B) {
	cache := NewMemoryCache(10000)
	defer cache.Close()
	
	// Pre-populate
	cache.Set("counter", int64(0), time.Minute)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Increment("counter", 1)
	}
}