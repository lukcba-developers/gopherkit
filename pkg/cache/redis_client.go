package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/lukcba-developers/gopherkit/pkg/errors"
)

// Client interface para operaciones de cache
type Client interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	GetJSON(ctx context.Context, key string, dest interface{}) error
	SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	Expire(ctx context.Context, key string, expiration time.Duration) error
	Keys(ctx context.Context, pattern string) ([]string, error)
	FlushDB(ctx context.Context) error
	Ping(ctx context.Context) error
	Close() error
	
	// Operaciones avanzadas
	Increment(ctx context.Context, key string) (int64, error)
	Decrement(ctx context.Context, key string) (int64, error)
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	GetSet(ctx context.Context, key string, value interface{}) (string, error)
	
	// Operaciones de lista
	LPush(ctx context.Context, key string, values ...interface{}) error
	RPush(ctx context.Context, key string, values ...interface{}) error
	LPop(ctx context.Context, key string) (string, error)
	RPop(ctx context.Context, key string) (string, error)
	LLen(ctx context.Context, key string) (int64, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	
	// Operaciones de conjunto
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SMembers(ctx context.Context, key string) ([]string, error)
	SIsMember(ctx context.Context, key string, member interface{}) (bool, error)
	SRem(ctx context.Context, key string, members ...interface{}) error
	
	// Hash operations
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	HExists(ctx context.Context, key, field string) (bool, error)
}

// RedisConfig configuración para cliente Redis
type RedisConfig struct {
	Host               string
	Port               int
	Password           string
	DB                 int
	PoolSize           int
	MinIdleConns       int
	MaxConnAge         time.Duration
	PoolTimeout        time.Duration
	IdleTimeout        time.Duration
	IdleCheckFrequency time.Duration
	
	// Clustering
	ClusterMode bool
	ClusterAddrs []string
	
	// Security
	TLSEnabled bool
	TLSSkipVerify bool
	
	// Retry configuration
	MaxRetries      int
	MinRetryBackoff time.Duration
	MaxRetryBackoff time.Duration
	
	// Monitoring
	EnableMetrics bool
	SlowLogEnabled bool
	SlowLogThreshold time.Duration
}

// DefaultRedisConfig retorna configuración por defecto
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Host:               "localhost",
		Port:               6379,
		DB:                 0,
		PoolSize:           10,
		MinIdleConns:       5,
		MaxConnAge:         30 * time.Minute,
		PoolTimeout:        10 * time.Second,
		IdleTimeout:        5 * time.Minute,
		IdleCheckFrequency: time.Minute,
		MaxRetries:         3,
		MinRetryBackoff:    8 * time.Millisecond,
		MaxRetryBackoff:    512 * time.Millisecond,
		EnableMetrics:      true,
		SlowLogEnabled:     true,
		SlowLogThreshold:   100 * time.Millisecond,
	}
}

// redisClient implementación de Client usando go-redis
type redisClient struct {
	client *redis.Client
	cluster *redis.ClusterClient
	config *RedisConfig
	tracer trace.Tracer
	isCluster bool
}

// NewRedisClient crea un nuevo cliente Redis
func NewRedisClient(config *RedisConfig) (Client, error) {
	if config == nil {
		config = DefaultRedisConfig()
	}
	
	tracer := otel.Tracer("redis-client")
	
	rc := &redisClient{
		config: config,
		tracer: tracer,
		isCluster: config.ClusterMode,
	}
	
	if config.ClusterMode {
		return rc.initClusterClient()
	}
	
	return rc.initSingleClient()
}

// initSingleClient inicializa cliente Redis single-node
func (r *redisClient) initSingleClient() (Client, error) {
	opts := &redis.Options{
		Addr:               fmt.Sprintf("%s:%d", r.config.Host, r.config.Port),
		Password:           r.config.Password,
		DB:                 r.config.DB,
		PoolSize:           r.config.PoolSize,
		MinIdleConns:       r.config.MinIdleConns,
		MaxConnAge:         r.config.MaxConnAge,
		PoolTimeout:        r.config.PoolTimeout,
		IdleTimeout:        r.config.IdleTimeout,
		IdleCheckFrequency: r.config.IdleCheckFrequency,
		MaxRetries:         r.config.MaxRetries,
		MinRetryBackoff:    r.config.MinRetryBackoff,
		MaxRetryBackoff:    r.config.MaxRetryBackoff,
	}
	
	r.client = redis.NewClient(opts)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := r.client.Ping(ctx).Err(); err != nil {
		return nil, errors.NewDomainError(
			"REDIS_CONNECTION_FAILED",
			"Failed to connect to Redis",
			errors.CategoryInfrastructure,
			errors.SeverityCritical,
		).WithCause(err).WithMetadata("host", r.config.Host).WithMetadata("port", r.config.Port)
	}
	
	return r, nil
}

// initClusterClient inicializa cliente Redis cluster
func (r *redisClient) initClusterClient() (Client, error) {
	opts := &redis.ClusterOptions{
		Addrs:              r.config.ClusterAddrs,
		Password:           r.config.Password,
		PoolSize:           r.config.PoolSize,
		MinIdleConns:       r.config.MinIdleConns,
		MaxConnAge:         r.config.MaxConnAge,
		PoolTimeout:        r.config.PoolTimeout,
		IdleTimeout:        r.config.IdleTimeout,
		IdleCheckFrequency: r.config.IdleCheckFrequency,
		MaxRetries:         r.config.MaxRetries,
		MinRetryBackoff:    r.config.MinRetryBackoff,
		MaxRetryBackoff:    r.config.MaxRetryBackoff,
	}
	
	r.cluster = redis.NewClusterClient(opts)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := r.cluster.Ping(ctx).Err(); err != nil {
		return nil, errors.NewDomainError(
			"REDIS_CLUSTER_CONNECTION_FAILED",
			"Failed to connect to Redis cluster",
			errors.CategoryInfrastructure,
			errors.SeverityCritical,
		).WithCause(err).WithMetadata("addrs", r.config.ClusterAddrs)
	}
	
	return r, nil
}

// getClient retorna el cliente apropiado (single o cluster)
func (r *redisClient) getClient() redis.Cmdable {
	if r.isCluster {
		return r.cluster
	}
	return r.client
}

// Set almacena un valor con expiración
func (r *redisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	ctx, span := r.tracer.Start(ctx, "cache.set")
	defer span.End()
	
	span.SetAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.expiration", expiration.String()),
	)
	
	client := r.getClient()
	err := client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.wrapError("SET", err)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// Get obtiene un valor como string
func (r *redisClient) Get(ctx context.Context, key string) (string, error) {
	ctx, span := r.tracer.Start(ctx, "cache.get")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.key", key))
	
	client := r.getClient()
	result, err := client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			span.SetAttributes(attribute.Bool("cache.miss", true))
			return "", errors.NewDomainError(
				"CACHE_KEY_NOT_FOUND",
				"Key not found in cache",
				errors.CategoryBusiness,
				errors.SeverityLow,
			).WithMetadata("key", key)
		}
		span.SetAttributes(attribute.Bool("error", true))
		return "", r.wrapError("GET", err)
	}
	
	span.SetAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Bool("success", true),
	)
	return result, nil
}

// GetJSON obtiene un valor y lo deserializa como JSON
func (r *redisClient) GetJSON(ctx context.Context, key string, dest interface{}) error {
	ctx, span := r.tracer.Start(ctx, "cache.get_json")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.key", key))
	
	value, err := r.Get(ctx, key)
	if err != nil {
		return err
	}
	
	if err := json.Unmarshal([]byte(value), dest); err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return errors.NewDomainError(
			"CACHE_JSON_UNMARSHAL_FAILED",
			"Failed to unmarshal cached JSON",
			errors.CategoryTechnical,
			errors.SeverityMedium,
		).WithCause(err).WithMetadata("key", key)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// SetJSON serializa un valor como JSON y lo almacena
func (r *redisClient) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	ctx, span := r.tracer.Start(ctx, "cache.set_json")
	defer span.End()
	
	span.SetAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.expiration", expiration.String()),
	)
	
	jsonValue, err := json.Marshal(value)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return errors.NewDomainError(
			"CACHE_JSON_MARSHAL_FAILED",
			"Failed to marshal value to JSON",
			errors.CategoryTechnical,
			errors.SeverityMedium,
		).WithCause(err).WithMetadata("key", key)
	}
	
	return r.Set(ctx, key, string(jsonValue), expiration)
}

// Delete elimina una o más keys
func (r *redisClient) Delete(ctx context.Context, keys ...string) error {
	ctx, span := r.tracer.Start(ctx, "cache.delete")
	defer span.End()
	
	span.SetAttributes(
		attribute.StringSlice("cache.keys", keys),
		attribute.Int("cache.key_count", len(keys)),
	)
	
	client := r.getClient()
	err := client.Del(ctx, keys...).Err()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.wrapError("DELETE", err)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// Exists verifica si una key existe
func (r *redisClient) Exists(ctx context.Context, key string) (bool, error) {
	ctx, span := r.tracer.Start(ctx, "cache.exists")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.key", key))
	
	client := r.getClient()
	result, err := client.Exists(ctx, key).Result()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return false, r.wrapError("EXISTS", err)
	}
	
	exists := result > 0
	span.SetAttributes(
		attribute.Bool("cache.exists", exists),
		attribute.Bool("success", true),
	)
	return exists, nil
}

// Expire establece expiración para una key
func (r *redisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	ctx, span := r.tracer.Start(ctx, "cache.expire")
	defer span.End()
	
	span.SetAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.expiration", expiration.String()),
	)
	
	client := r.getClient()
	err := client.Expire(ctx, key, expiration).Err()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.wrapError("EXPIRE", err)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// Keys obtiene keys que coincidan con un patrón
func (r *redisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "cache.keys")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.pattern", pattern))
	
	client := r.getClient()
	result, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return nil, r.wrapError("KEYS", err)
	}
	
	span.SetAttributes(
		attribute.Int("cache.key_count", len(result)),
		attribute.Bool("success", true),
	)
	return result, nil
}

// FlushDB elimina todas las keys de la base de datos actual
func (r *redisClient) FlushDB(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "cache.flush_db")
	defer span.End()
	
	client := r.getClient()
	err := client.FlushDB(ctx).Err()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.wrapError("FLUSHDB", err)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// Ping verifica la conexión
func (r *redisClient) Ping(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "cache.ping")
	defer span.End()
	
	client := r.getClient()
	err := client.Ping(ctx).Err()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return r.wrapError("PING", err)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return nil
}

// Close cierra la conexión
func (r *redisClient) Close() error {
	if r.isCluster && r.cluster != nil {
		return r.cluster.Close()
	}
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Operaciones avanzadas

// Increment incrementa un contador
func (r *redisClient) Increment(ctx context.Context, key string) (int64, error) {
	ctx, span := r.tracer.Start(ctx, "cache.increment")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.key", key))
	
	client := r.getClient()
	result, err := client.Incr(ctx, key).Result()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return 0, r.wrapError("INCR", err)
	}
	
	span.SetAttributes(
		attribute.Int64("cache.value", result),
		attribute.Bool("success", true),
	)
	return result, nil
}

// Decrement decrementa un contador
func (r *redisClient) Decrement(ctx context.Context, key string) (int64, error) {
	ctx, span := r.tracer.Start(ctx, "cache.decrement")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.key", key))
	
	client := r.getClient()
	result, err := client.Decr(ctx, key).Result()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return 0, r.wrapError("DECR", err)
	}
	
	span.SetAttributes(
		attribute.Int64("cache.value", result),
		attribute.Bool("success", true),
	)
	return result, nil
}

// SetNX establece una key solo si no existe
func (r *redisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	ctx, span := r.tracer.Start(ctx, "cache.set_nx")
	defer span.End()
	
	span.SetAttributes(
		attribute.String("cache.key", key),
		attribute.String("cache.expiration", expiration.String()),
	)
	
	client := r.getClient()
	result, err := client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return false, r.wrapError("SETNX", err)
	}
	
	span.SetAttributes(
		attribute.Bool("cache.set", result),
		attribute.Bool("success", true),
	)
	return result, nil
}

// GetSet establece un nuevo valor y retorna el anterior
func (r *redisClient) GetSet(ctx context.Context, key string, value interface{}) (string, error) {
	ctx, span := r.tracer.Start(ctx, "cache.get_set")
	defer span.End()
	
	span.SetAttributes(attribute.String("cache.key", key))
	
	client := r.getClient()
	result, err := client.GetSet(ctx, key, value).Result()
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		return "", r.wrapError("GETSET", err)
	}
	
	span.SetAttributes(attribute.Bool("success", true))
	return result, nil
}

// Operaciones de lista - implementaciones básicas
func (r *redisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	client := r.getClient()
	return client.LPush(ctx, key, values...).Err()
}

func (r *redisClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	client := r.getClient()
	return client.RPush(ctx, key, values...).Err()
}

func (r *redisClient) LPop(ctx context.Context, key string) (string, error) {
	client := r.getClient()
	return client.LPop(ctx, key).Result()
}

func (r *redisClient) RPop(ctx context.Context, key string) (string, error) {
	client := r.getClient()
	return client.RPop(ctx, key).Result()
}

func (r *redisClient) LLen(ctx context.Context, key string) (int64, error) {
	client := r.getClient()
	return client.LLen(ctx, key).Result()
}

func (r *redisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	client := r.getClient()
	return client.LRange(ctx, key, start, stop).Result()
}

// Operaciones de conjunto
func (r *redisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	client := r.getClient()
	return client.SAdd(ctx, key, members...).Err()
}

func (r *redisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	client := r.getClient()
	return client.SMembers(ctx, key).Result()
}

func (r *redisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	client := r.getClient()
	return client.SIsMember(ctx, key, member).Result()
}

func (r *redisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	client := r.getClient()
	return client.SRem(ctx, key, members...).Err()
}

// Operaciones de hash
func (r *redisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	client := r.getClient()
	return client.HSet(ctx, key, values...).Err()
}

func (r *redisClient) HGet(ctx context.Context, key, field string) (string, error) {
	client := r.getClient()
	return client.HGet(ctx, key, field).Result()
}

func (r *redisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	client := r.getClient()
	return client.HGetAll(ctx, key).Result()
}

func (r *redisClient) HDel(ctx context.Context, key string, fields ...string) error {
	client := r.getClient()
	return client.HDel(ctx, key, fields...).Err()
}

func (r *redisClient) HExists(ctx context.Context, key, field string) (bool, error) {
	client := r.getClient()
	return client.HExists(ctx, key, field).Result()
}

// wrapError convierte errores de Redis en domain errors
func (r *redisClient) wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	
	return errors.NewDomainError(
		"REDIS_OPERATION_FAILED",
		fmt.Sprintf("Redis %s operation failed", operation),
		errors.CategoryInfrastructure,
		errors.SeverityHigh,
	).WithCause(err).WithMetadata("operation", operation)
}