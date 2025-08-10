package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
	"github.com/lukcba-developers/gopherkit/pkg/observability"
)

// RedisClient provides Redis caching operations
type RedisClient struct {
	client redis.UniversalClient
	logger logger.Logger
	config config.CacheConfig
}

// RedisOptions holds options for Redis client
type RedisOptions struct {
	Config config.CacheConfig
	Logger logger.Logger
}

// NewRedisClient creates a new Redis client
func NewRedisClient(opts RedisOptions) (*RedisClient, error) {
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	if !opts.Config.Enabled {
		return &RedisClient{logger: opts.Logger, config: opts.Config}, nil
	}

	// Configure Redis client options
	options := &redis.UniversalOptions{
		Addrs:        []string{opts.Config.Host + ":" + opts.Config.Port},
		Password:     opts.Config.Password,
		DB:           opts.Config.DB,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolTimeout:  5 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	}

	client := redis.NewUniversalClient(options)

	redisClient := &RedisClient{
		client: client,
		logger: opts.Logger,
		config: opts.Config,
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	opts.Logger.LogBusinessEvent(context.Background(), "cache_connected", map[string]interface{}{
		"type": "redis",
		"host": opts.Config.Host,
		"port": opts.Config.Port,
		"db":   opts.Config.DB,
	})

	return redisClient, nil
}

// Ping tests Redis connection
func (rc *RedisClient) Ping(ctx context.Context) error {
	if !rc.config.Enabled || rc.client == nil {
		return fmt.Errorf("cache is disabled")
	}

	cmd := rc.client.Ping(ctx)
	return cmd.Err()
}

// Set stores a value with key
func (rc *RedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !rc.config.Enabled {
		return nil // Silently skip if cache is disabled
	}

	start := time.Now()
	fullKey := rc.buildKey(key)

	// Serialize value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		rc.logger.LogError(ctx, err, "failed to serialize cache value", map[string]interface{}{
			"key": fullKey,
		})
		return err
	}

	// Use default TTL if not specified
	if ttl == 0 {
		ttl = rc.config.TTL
	}

	cmd := rc.client.Set(ctx, fullKey, data, ttl)
	err = cmd.Err()

	duration := time.Since(start)
	rc.logger.LogPerformanceEvent(ctx, "cache_set", duration, map[string]interface{}{
		"key": fullKey,
		"ttl": ttl.String(),
	})

	if err != nil {
		rc.logger.LogError(ctx, err, "failed to set cache value", map[string]interface{}{
			"key": fullKey,
			"ttl": ttl.String(),
		})
	}

	return err
}

// Get retrieves a value by key
func (rc *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	start := time.Now()
	fullKey := rc.buildKey(key)

	cmd := rc.client.Get(ctx, fullKey)
	data, err := cmd.Bytes()

	duration := time.Since(start)

	if err == redis.Nil {
		// Cache miss
		rc.logger.LogPerformanceEvent(ctx, "cache_miss", duration, map[string]interface{}{
			"key": fullKey,
		})
		return ErrCacheMiss
	}

	if err != nil {
		rc.logger.LogError(ctx, err, "failed to get cache value", map[string]interface{}{
			"key": fullKey,
		})
		return err
	}

	// Cache hit - deserialize
	err = json.Unmarshal(data, dest)
	if err != nil {
		rc.logger.LogError(ctx, err, "failed to deserialize cache value", map[string]interface{}{
			"key": fullKey,
		})
		return err
	}

	rc.logger.LogPerformanceEvent(ctx, "cache_hit", duration, map[string]interface{}{
		"key": fullKey,
	})

	return nil
}

// Delete removes a key from cache
func (rc *RedisClient) Delete(ctx context.Context, key string) error {
	if !rc.config.Enabled {
		return nil
	}

	start := time.Now()
	fullKey := rc.buildKey(key)

	cmd := rc.client.Del(ctx, fullKey)
	err := cmd.Err()

	duration := time.Since(start)
	rc.logger.LogPerformanceEvent(ctx, "cache_delete", duration, map[string]interface{}{
		"key": fullKey,
	})

	if err != nil {
		rc.logger.LogError(ctx, err, "failed to delete cache key", map[string]interface{}{
			"key": fullKey,
		})
	}

	return err
}

// Exists checks if a key exists
func (rc *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	if !rc.config.Enabled {
		return false, nil
	}

	fullKey := rc.buildKey(key)
	cmd := rc.client.Exists(ctx, fullKey)
	count, err := cmd.Result()
	
	if err != nil {
		rc.logger.LogError(ctx, err, "failed to check cache key existence", map[string]interface{}{
			"key": fullKey,
		})
		return false, err
	}

	return count > 0, nil
}

// SetNX sets a key only if it doesn't exist (atomic operation)
func (rc *RedisClient) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	if !rc.config.Enabled {
		return false, ErrCacheDisabled
	}

	fullKey := rc.buildKey(key)
	
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	if ttl == 0 {
		ttl = rc.config.TTL
	}

	cmd := rc.client.SetNX(ctx, fullKey, data, ttl)
	return cmd.Result()
}

// Increment atomically increments a key
func (rc *RedisClient) Increment(ctx context.Context, key string) (int64, error) {
	if !rc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	fullKey := rc.buildKey(key)
	cmd := rc.client.Incr(ctx, fullKey)
	return cmd.Result()
}

// IncrementBy atomically increments a key by a given amount
func (rc *RedisClient) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	if !rc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	fullKey := rc.buildKey(key)
	cmd := rc.client.IncrBy(ctx, fullKey, value)
	return cmd.Result()
}

// Expire sets TTL for a key
func (rc *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if !rc.config.Enabled {
		return nil
	}

	fullKey := rc.buildKey(key)
	cmd := rc.client.Expire(ctx, fullKey, ttl)
	return cmd.Err()
}

// GetTTL gets the remaining TTL for a key
func (rc *RedisClient) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	if !rc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	fullKey := rc.buildKey(key)
	cmd := rc.client.TTL(ctx, fullKey)
	return cmd.Result()
}

// FlushPattern deletes all keys matching a pattern
func (rc *RedisClient) FlushPattern(ctx context.Context, pattern string) error {
	if !rc.config.Enabled {
		return nil
	}

	fullPattern := rc.buildKey(pattern)
	
	// Use SCAN to find matching keys
	iter := rc.client.Scan(ctx, 0, fullPattern, 0).Iterator()
	var keys []string
	
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	
	if err := iter.Err(); err != nil {
		return err
	}

	if len(keys) > 0 {
		cmd := rc.client.Del(ctx, keys...)
		return cmd.Err()
	}

	return nil
}

// GetMultiple gets multiple keys at once
func (rc *RedisClient) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if !rc.config.Enabled {
		return make(map[string]interface{}), nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rc.buildKey(key)
	}

	cmd := rc.client.MGet(ctx, fullKeys...)
	values, err := cmd.Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for i, value := range values {
		if value != nil {
			var dest interface{}
			if data, ok := value.(string); ok {
				if err := json.Unmarshal([]byte(data), &dest); err == nil {
					result[keys[i]] = dest
				}
			}
		}
	}

	return result, nil
}

// Close closes the Redis connection
func (rc *RedisClient) Close() error {
	if rc.client != nil {
		return rc.client.Close()
	}
	return nil
}

// HealthCheck returns a health check for Redis
func (rc *RedisClient) HealthCheck() observability.HealthCheck {
	return observability.NewRedisHealthCheck("redis", rc.client)
}

// GetClient returns the underlying Redis client for advanced operations
func (rc *RedisClient) GetClient() redis.UniversalClient {
	return rc.client
}

// buildKey creates a full key with prefix
func (rc *RedisClient) buildKey(key string) string {
	if rc.config.Prefix != "" {
		return rc.config.Prefix + key
	}
	return key
}

// Cache errors
var (
	ErrCacheDisabled = fmt.Errorf("cache is disabled")
)

// CacheService provides high-level caching operations
type CacheService struct {
	client *RedisClient
	logger logger.Logger
}

// NewCacheService creates a new cache service
func NewCacheService(client *RedisClient, logger logger.Logger) *CacheService {
	return &CacheService{
		client: client,
		logger: logger,
	}
}

// GetOrSet gets a value from cache, or sets it if not found
func (cs *CacheService) GetOrSet(ctx context.Context, key string, dest interface{}, setter func() (interface{}, error), ttl time.Duration) error {
	// Try to get from cache first
	err := cs.client.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	if err != ErrCacheMiss && err != ErrCacheDisabled {
		cs.logger.LogError(ctx, err, "cache error during get", map[string]interface{}{
			"key": key,
		})
		// Continue with setter even if cache fails
	}

	// Cache miss or error - call setter
	value, err := setter()
	if err != nil {
		return err
	}

	// Try to set in cache (async, don't block on cache errors)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if setErr := cs.client.Set(cacheCtx, key, value, ttl); setErr != nil {
			cs.logger.LogError(cacheCtx, setErr, "failed to set cache after miss", map[string]interface{}{
				"key": key,
			})
		}
	}()

	// Set the destination with the fetched value
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(data, dest)
}