package components

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/lukcba-developers/gopherkit/pkg/health"
)

// RedisChecker implementa health check para Redis
type RedisChecker struct {
	name     string
	client   redis.UniversalClient
	timeout  time.Duration
	required bool
}

// NewRedisChecker crea un nuevo checker de Redis
func NewRedisChecker(name string, client redis.UniversalClient, opts ...RedisOption) *RedisChecker {
	checker := &RedisChecker{
		name:     name,
		client:   client,
		timeout:  10 * time.Second,
		required: true,
	}
	
	for _, opt := range opts {
		opt(checker)
	}
	
	return checker
}

// RedisOption define opciones para RedisChecker
type RedisOption func(*RedisChecker)

// WithRedisTimeout configura el timeout
func WithRedisTimeout(timeout time.Duration) RedisOption {
	return func(c *RedisChecker) {
		c.timeout = timeout
	}
}

// WithRedisRequired configura si es requerido
func WithRedisRequired(required bool) RedisOption {
	return func(c *RedisChecker) {
		c.required = required
	}
}

// Name implementa HealthChecker
func (r *RedisChecker) Name() string {
	return r.name
}

// Check implementa HealthChecker
func (r *RedisChecker) Check(ctx context.Context) health.ComponentHealth {
	start := time.Now()
	
	// Ping básico
	pong, err := r.client.Ping(ctx).Result()
	if err != nil {
		return health.NewComponentHealth(health.StatusUnhealthy, "Redis ping failed").
			WithDuration(time.Since(start)).
			WithError(err)
	}
	
	if pong != "PONG" {
		return health.NewComponentHealth(health.StatusUnhealthy, "Redis ping returned unexpected response").
			WithDuration(time.Since(start)).
			WithMetadata("ping_response", pong)
	}
	
	// Test básico de escritura/lectura
	testKey := fmt.Sprintf("health_check_%d", time.Now().Unix())
	testValue := "health_check_value"
	
	// SET
	if err := r.client.Set(ctx, testKey, testValue, time.Minute).Err(); err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "Redis SET operation failed").
			WithDuration(time.Since(start)).
			WithError(err)
	}
	
	// GET
	result, err := r.client.Get(ctx, testKey).Result()
	if err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "Redis GET operation failed").
			WithDuration(time.Since(start)).
			WithError(err)
	}
	
	if result != testValue {
		return health.NewComponentHealth(health.StatusDegraded, "Redis GET returned unexpected value").
			WithDuration(time.Since(start)).
			WithMetadata("expected", testValue).
			WithMetadata("actual", result)
	}
	
	// DELETE (cleanup)
	if err := r.client.Del(ctx, testKey).Err(); err != nil {
		// No es crítico si no se puede eliminar, pero lo registramos
		// No retornamos error aquí
	}
	
	// Obtener información del servidor Redis
	info := r.getRedisInfo(ctx)
	
	return health.NewComponentHealth(health.StatusHealthy, "Redis is healthy").
		WithDuration(time.Since(start)).
		WithMetadata("ping_response", pong).
		WithMetadata("info", info)
}

// IsRequired implementa HealthChecker
func (r *RedisChecker) IsRequired() bool {
	return r.required
}

// Timeout implementa HealthChecker
func (r *RedisChecker) Timeout() time.Duration {
	return r.timeout
}

// getRedisInfo obtiene información básica del servidor Redis
func (r *RedisChecker) getRedisInfo(ctx context.Context) map[string]interface{} {
	info := make(map[string]interface{})
	
	// Información del servidor
	if infoResult, err := r.client.Info(ctx, "server").Result(); err == nil {
		info["server_info"] = infoResult
	}
	
	// Información de memoria
	if memInfo, err := r.client.Info(ctx, "memory").Result(); err == nil {
		info["memory_info"] = memInfo
	}
	
	// Número de conexiones
	if clientsInfo, err := r.client.Info(ctx, "clients").Result(); err == nil {
		info["clients_info"] = clientsInfo
	}
	
	// Database size (número de keys)
	if dbSize, err := r.client.DBSize(ctx).Result(); err == nil {
		info["db_size"] = dbSize
	}
	
	// Tiempo de funcionamiento (uptime)
	if uptime, err := r.client.Info(ctx, "stats").Result(); err == nil {
		info["stats"] = uptime
	}
	
	return info
}

// RedisClusterChecker implementa health check para Redis Cluster
type RedisClusterChecker struct {
	name     string
	client   *redis.ClusterClient
	timeout  time.Duration
	required bool
}

// NewRedisClusterChecker crea un nuevo checker para Redis Cluster
func NewRedisClusterChecker(name string, client *redis.ClusterClient, opts ...RedisClusterOption) *RedisClusterChecker {
	checker := &RedisClusterChecker{
		name:     name,
		client:   client,
		timeout:  15 * time.Second, // Más tiempo para clusters
		required: true,
	}
	
	for _, opt := range opts {
		opt(checker)
	}
	
	return checker
}

// RedisClusterOption define opciones para RedisClusterChecker
type RedisClusterOption func(*RedisClusterChecker)

// WithRedisClusterTimeout configura el timeout
func WithRedisClusterTimeout(timeout time.Duration) RedisClusterOption {
	return func(c *RedisClusterChecker) {
		c.timeout = timeout
	}
}

// WithRedisClusterRequired configura si es requerido
func WithRedisClusterRequired(required bool) RedisClusterOption {
	return func(c *RedisClusterChecker) {
		c.required = required
	}
}

// Name implementa HealthChecker
func (rc *RedisClusterChecker) Name() string {
	return rc.name
}

// Check implementa HealthChecker
func (rc *RedisClusterChecker) Check(ctx context.Context) health.ComponentHealth {
	start := time.Now()
	
	// Verificar estado del cluster
	clusterInfo, err := rc.client.ClusterInfo(ctx).Result()
	if err != nil {
		return health.NewComponentHealth(health.StatusUnhealthy, "Redis Cluster info failed").
			WithDuration(time.Since(start)).
			WithError(err)
	}
	
	// Verificar que el cluster esté en estado OK
	if !contains(clusterInfo, "cluster_state:ok") {
		return health.NewComponentHealth(health.StatusUnhealthy, "Redis Cluster is not in OK state").
			WithDuration(time.Since(start)).
			WithMetadata("cluster_info", clusterInfo)
	}
	
	// Obtener nodes del cluster
	clusterNodes, err := rc.client.ClusterNodes(ctx).Result()
	if err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "Redis Cluster nodes info failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("cluster_info", clusterInfo)
	}
	
	// Test básico de escritura/lectura
	testKey := fmt.Sprintf("health_check_%d", time.Now().Unix())
	testValue := "cluster_health_check"
	
	if err := rc.client.Set(ctx, testKey, testValue, time.Minute).Err(); err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "Redis Cluster SET operation failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("cluster_info", clusterInfo)
	}
	
	result, err := rc.client.Get(ctx, testKey).Result()
	if err != nil {
		return health.NewComponentHealth(health.StatusDegraded, "Redis Cluster GET operation failed").
			WithDuration(time.Since(start)).
			WithError(err).
			WithMetadata("cluster_info", clusterInfo)
	}
	
	if result != testValue {
		return health.NewComponentHealth(health.StatusDegraded, "Redis Cluster GET returned unexpected value").
			WithDuration(time.Since(start)).
			WithMetadata("expected", testValue).
			WithMetadata("actual", result).
			WithMetadata("cluster_info", clusterInfo)
	}
	
	// Cleanup
	rc.client.Del(ctx, testKey)
	
	// Contar nodos activos
	activeNodes := countActiveNodes(clusterNodes)
	
	return health.NewComponentHealth(health.StatusHealthy, "Redis Cluster is healthy").
		WithDuration(time.Since(start)).
		WithMetadata("cluster_state", "ok").
		WithMetadata("active_nodes", activeNodes).
		WithMetadata("cluster_info", clusterInfo)
}

// IsRequired implementa HealthChecker
func (rc *RedisClusterChecker) IsRequired() bool {
	return rc.required
}

// Timeout implementa HealthChecker
func (rc *RedisClusterChecker) Timeout() time.Duration {
	return rc.timeout
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && s[:len(substr)+1] == substr+" ") ||
		(len(s) > len(substr) && s[len(s)-len(substr)-1:] == " "+substr) ||
		(len(s) > len(substr)*2 && s[len(s)-len(substr):] == substr))
}

func countActiveNodes(nodesInfo string) int {
	// Implementación simple para contar nodos master activos
	// En producción, esto debería ser más robusto
	lines := strings.Split(nodesInfo, "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "master") && !strings.Contains(line, "fail") {
			count++
		}
	}
	return count
}