package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// CacheInterface define la interfaz unificada de cache
type CacheInterface interface {
	// Basic operations
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) bool
	
	// Advanced operations
	GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error)
	SetMultiple(ctx context.Context, items map[string]interface{}, expiration time.Duration) error
	InvalidatePattern(ctx context.Context, pattern string) error
	
	// Atomic operations
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	Increment(ctx context.Context, key string, delta int64) (int64, error)
	
	// Info operations
	GetTTL(ctx context.Context, key string) time.Duration
	Ping(ctx context.Context) error
	GetStats(ctx context.Context) (map[string]interface{}, error)
}

// CacheConfig configuración unificada de cache
type CacheConfig struct {
	// Redis Configuration
	RedisHost           string
	RedisPort           string
	RedisPassword       string
	RedisDB             int
	
	// Connection Pool
	PoolSize            int
	MinIdleConns        int
	MaxConnAge          time.Duration
	PoolTimeout         time.Duration
	IdleTimeout         time.Duration
	
	// Timeouts
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	DialTimeout         time.Duration
	
	// Cache Behavior
	DefaultExpiration   time.Duration
	KeyPrefix           string
	EnableCompression   bool
	EnableMetrics       bool
	
	// Fallback Settings
	EnableMemoryFallback bool
	MemoryCacheSize      int
}

// DefaultCacheConfig retorna configuración por defecto
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		RedisHost:           "localhost",
		RedisPort:           "6379",
		RedisPassword:       "",
		RedisDB:             0,
		PoolSize:            10,
		MinIdleConns:        2,
		MaxConnAge:          30 * time.Minute,
		PoolTimeout:         4 * time.Second,
		IdleTimeout:         5 * time.Minute,
		ReadTimeout:         3 * time.Second,
		WriteTimeout:        3 * time.Second,
		DialTimeout:         5 * time.Second,
		DefaultExpiration:   time.Hour,
		KeyPrefix:           "gopherkit",
		EnableCompression:   false,
		EnableMetrics:       true,
		EnableMemoryFallback: true,
		MemoryCacheSize:     1000,
	}
}

// UnifiedCache implementación unificada de cache con Redis y fallback en memoria
type UnifiedCache struct {
	config       *CacheConfig
	redisClient  *redis.Client
	memoryCache  *MemoryCache
	logger       *logrus.Entry
	metrics      *CacheMetrics
}

// NewUnifiedCache crea una nueva instancia de cache unificado
func NewUnifiedCache(config *CacheConfig, logger *logrus.Logger) (*UnifiedCache, error) {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &UnifiedCache{
		config: config,
		logger: logger.WithField("component", "unified_cache"),
	}

	// Initialize Redis client
	if err := cache.initRedis(); err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Initialize memory cache fallback if enabled
	if config.EnableMemoryFallback {
		cache.memoryCache = NewMemoryCache(config.MemoryCacheSize)
	}

	// Initialize metrics if enabled
	if config.EnableMetrics {
		cache.metrics = NewCacheMetrics()
	}

	logger.WithFields(logrus.Fields{
		"redis_host":           config.RedisHost,
		"redis_port":           config.RedisPort,
		"pool_size":            config.PoolSize,
		"memory_fallback":      config.EnableMemoryFallback,
		"metrics_enabled":      config.EnableMetrics,
		"compression_enabled":  config.EnableCompression,
	}).Info("Unified cache initialized successfully")

	return cache, nil
}

// initRedis inicializa el cliente Redis
func (c *UnifiedCache) initRedis() error {
	rdb := redis.NewClient(&redis.Options{
		Addr:         c.config.RedisHost + ":" + c.config.RedisPort,
		Password:     c.config.RedisPassword,
		DB:           c.config.RedisDB,
		PoolSize:     c.config.PoolSize,
		MinIdleConns: c.config.MinIdleConns,
		MaxConnAge:   c.config.MaxConnAge,
		PoolTimeout:  c.config.PoolTimeout,
		IdleTimeout:  c.config.IdleTimeout,
		ReadTimeout:  c.config.ReadTimeout,
		WriteTimeout: c.config.WriteTimeout,
		DialTimeout:  c.config.DialTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), c.config.DialTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		c.logger.WithError(err).Warn("Redis connection failed, will use memory fallback only")
		if !c.config.EnableMemoryFallback {
			return fmt.Errorf("Redis connection failed and memory fallback is disabled: %w", err)
		}
	} else {
		c.redisClient = rdb
		c.logger.Info("Redis connection established successfully")
	}

	return nil
}

// getKey añade el prefijo a la clave
func (c *UnifiedCache) getKey(key string) string {
	if c.config.KeyPrefix == "" {
		return key
	}
	return c.config.KeyPrefix + ":" + key
}

// Get obtiene un valor del cache
func (c *UnifiedCache) Get(ctx context.Context, key string, dest interface{}) error {
	fullKey := c.getKey(key)
	
	// Try Redis first
	if c.redisClient != nil {
		err := c.getFromRedis(ctx, fullKey, dest)
		if err == nil {
			c.recordMetric("redis_hit", key)
			return nil
		}
		
		if err != ErrCacheMiss {
			c.logger.WithError(err).WithField("key", fullKey).Warn("Redis get error")
		}
		c.recordMetric("redis_miss", key)
	}

	// Fallback to memory cache
	if c.memoryCache != nil {
		err := c.memoryCache.Get(fullKey, dest)
		if err == nil {
			c.recordMetric("memory_hit", key)
			return nil
		}
		c.recordMetric("memory_miss", key)
	}

	return ErrCacheMiss
}

// Set guarda un valor en el cache
func (c *UnifiedCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	fullKey := c.getKey(key)
	
	if expiration == 0 {
		expiration = c.config.DefaultExpiration
	}

	var errors []error

	// Set in Redis
	if c.redisClient != nil {
		if err := c.setInRedis(ctx, fullKey, value, expiration); err != nil {
			c.logger.WithError(err).WithField("key", fullKey).Warn("Redis set error")
			errors = append(errors, err)
		} else {
			c.recordMetric("redis_set", key)
		}
	}

	// Set in memory cache as backup
	if c.memoryCache != nil {
		if err := c.memoryCache.Set(fullKey, value, expiration); err != nil {
			c.logger.WithError(err).WithField("key", fullKey).Warn("Memory cache set error")
			errors = append(errors, err)
		} else {
			c.recordMetric("memory_set", key)
		}
	}

	// Return error only if both failed
	if len(errors) > 0 && len(errors) == c.getAvailableCacheCount() {
		return fmt.Errorf("all cache backends failed: %v", errors)
	}

	return nil
}

// Delete elimina una clave del cache
func (c *UnifiedCache) Delete(ctx context.Context, key string) error {
	fullKey := c.getKey(key)
	
	var errors []error

	// Delete from Redis
	if c.redisClient != nil {
		if err := c.redisClient.Del(ctx, fullKey).Err(); err != nil {
			errors = append(errors, err)
		}
	}

	// Delete from memory cache
	if c.memoryCache != nil {
		c.memoryCache.Delete(fullKey)
	}

	c.recordMetric("delete", key)

	if len(errors) > 0 {
		return fmt.Errorf("errors deleting from cache: %v", errors)
	}

	return nil
}

// Exists verifica si una clave existe en el cache
func (c *UnifiedCache) Exists(ctx context.Context, key string) bool {
	fullKey := c.getKey(key)

	// Check Redis first
	if c.redisClient != nil {
		count, err := c.redisClient.Exists(ctx, fullKey).Result()
		if err == nil && count > 0 {
			return true
		}
	}

	// Check memory cache
	if c.memoryCache != nil {
		return c.memoryCache.Exists(fullKey)
	}

	return false
}

// GetMultiple obtiene múltiples valores del cache
func (c *UnifiedCache) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = c.getKey(key)
	}

	result := make(map[string]interface{})

	// Try Redis first
	if c.redisClient != nil {
		redisResults, err := c.redisClient.MGet(ctx, fullKeys...).Result()
		if err == nil {
			for i, val := range redisResults {
				if val != nil {
					var value interface{}
					if err := json.Unmarshal([]byte(val.(string)), &value); err == nil {
						result[keys[i]] = value
					}
				}
			}
		}
	}

	// Fill missing values from memory cache
	if c.memoryCache != nil {
		for i, key := range keys {
			if _, exists := result[key]; !exists {
				var value interface{}
				if err := c.memoryCache.Get(fullKeys[i], &value); err == nil {
					result[key] = value
				}
			}
		}
	}

	return result, nil
}

// SetMultiple guarda múltiples valores en el cache
func (c *UnifiedCache) SetMultiple(ctx context.Context, items map[string]interface{}, expiration time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	if expiration == 0 {
		expiration = c.config.DefaultExpiration
	}

	var errors []error

	// Set in Redis using pipeline
	if c.redisClient != nil {
		pipe := c.redisClient.Pipeline()
		for key, value := range items {
			fullKey := c.getKey(key)
			jsonValue, err := json.Marshal(value)
			if err != nil {
				continue
			}
			pipe.Set(ctx, fullKey, jsonValue, expiration)
		}
		
		if _, err := pipe.Exec(ctx); err != nil {
			errors = append(errors, err)
		}
	}

	// Set in memory cache
	if c.memoryCache != nil {
		for key, value := range items {
			fullKey := c.getKey(key)
			if err := c.memoryCache.Set(fullKey, value, expiration); err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors in SetMultiple: %v", errors)
	}

	return nil
}

// InvalidatePattern elimina todas las claves que coinciden con un patrón
func (c *UnifiedCache) InvalidatePattern(ctx context.Context, pattern string) error {
	fullPattern := c.getKey(pattern)

	var errors []error

	// Invalidate in Redis
	if c.redisClient != nil {
		keys, err := c.redisClient.Keys(ctx, fullPattern).Result()
		if err != nil {
			errors = append(errors, err)
		} else if len(keys) > 0 {
			if err := c.redisClient.Del(ctx, keys...).Err(); err != nil {
				errors = append(errors, err)
			}
		}
	}

	// Invalidate in memory cache
	if c.memoryCache != nil {
		c.memoryCache.InvalidatePattern(fullPattern)
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors invalidating pattern: %v", errors)
	}

	return nil
}

// SetNX sets a key only if it doesn't exist
func (c *UnifiedCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	fullKey := c.getKey(key)

	// Try Redis first for atomic operation
	if c.redisClient != nil {
		jsonValue, err := json.Marshal(value)
		if err != nil {
			return false, err
		}
		
		result, err := c.redisClient.SetNX(ctx, fullKey, jsonValue, expiration).Result()
		if err != nil {
			c.logger.WithError(err).WithField("key", fullKey).Warn("Redis SetNX error")
		} else {
			return result, nil
		}
	}

	// Fallback to memory cache (not truly atomic across instances)
	if c.memoryCache != nil {
		return c.memoryCache.SetNX(fullKey, value, expiration), nil
	}

	return false, fmt.Errorf("no cache backend available")
}

// Increment atomically increments a numeric value
func (c *UnifiedCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	fullKey := c.getKey(key)

	// Try Redis first for atomic operation
	if c.redisClient != nil {
		result, err := c.redisClient.IncrBy(ctx, fullKey, delta).Result()
		if err != nil {
			c.logger.WithError(err).WithField("key", fullKey).Warn("Redis increment error")
		} else {
			return result, nil
		}
	}

	// Fallback to memory cache (not truly atomic across instances)
	if c.memoryCache != nil {
		return c.memoryCache.Increment(fullKey, delta), nil
	}

	return 0, fmt.Errorf("no cache backend available")
}

// GetTTL obtiene el tiempo de vida restante de una clave
func (c *UnifiedCache) GetTTL(ctx context.Context, key string) time.Duration {
	fullKey := c.getKey(key)

	// Try Redis first
	if c.redisClient != nil {
		ttl, err := c.redisClient.TTL(ctx, fullKey).Result()
		if err == nil {
			return ttl
		}
	}

	// Fallback to memory cache
	if c.memoryCache != nil {
		return c.memoryCache.GetTTL(fullKey)
	}

	return 0
}

// Ping verifica la conectividad del cache
func (c *UnifiedCache) Ping(ctx context.Context) error {
	var errors []error

	// Ping Redis
	if c.redisClient != nil {
		if err := c.redisClient.Ping(ctx).Err(); err != nil {
			errors = append(errors, fmt.Errorf("Redis ping failed: %w", err))
		}
	}

	// Memory cache is always available if initialized
	if c.memoryCache == nil && c.redisClient == nil {
		errors = append(errors, fmt.Errorf("no cache backend available"))
	}

	if len(errors) > 0 {
		return fmt.Errorf("cache ping errors: %v", errors)
	}

	return nil
}

// GetStats obtiene estadísticas del cache
func (c *UnifiedCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Redis stats
	if c.redisClient != nil {
		info, err := c.redisClient.Info(ctx, "memory", "stats").Result()
		if err == nil {
			dbSize, _ := c.redisClient.DBSize(ctx).Result()
			poolStats := c.redisClient.PoolStats()

			stats["redis"] = map[string]interface{}{
				"db_size":    dbSize,
				"info":       info,
				"pool_stats": map[string]interface{}{
					"hits":         poolStats.Hits,
					"misses":       poolStats.Misses,
					"timeouts":     poolStats.Timeouts,
					"total_conns":  poolStats.TotalConns,
					"idle_conns":   poolStats.IdleConns,
					"stale_conns":  poolStats.StaleConns,
				},
			}
		}
	}

	// Memory cache stats
	if c.memoryCache != nil {
		stats["memory"] = c.memoryCache.GetStats()
	}

	// Unified cache metrics
	if c.metrics != nil {
		stats["metrics"] = c.metrics.GetStats()
	}

	return stats, nil
}

// Close cierra todas las conexiones del cache
func (c *UnifiedCache) Close() error {
	var errors []error

	if c.redisClient != nil {
		if err := c.redisClient.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close Redis: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing cache: %v", errors)
	}

	c.logger.Info("Unified cache closed successfully")
	return nil
}

// Helper functions

func (c *UnifiedCache) getFromRedis(ctx context.Context, key string, dest interface{}) error {
	val, err := c.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return ErrCacheMiss
		}
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

func (c *UnifiedCache) setInRedis(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.redisClient.Set(ctx, key, jsonValue, expiration).Err()
}

func (c *UnifiedCache) getAvailableCacheCount() int {
	count := 0
	if c.redisClient != nil {
		count++
	}
	if c.memoryCache != nil {
		count++
	}
	return count
}

func (c *UnifiedCache) recordMetric(operation, key string) {
	if c.metrics != nil {
		c.metrics.RecordOperation(operation, key)
	}
}