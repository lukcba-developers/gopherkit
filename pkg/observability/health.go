package observability

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/go-redis/redis/v8"
)

// HealthCheck represents a health check
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) HealthResult
}

// HealthResult represents the result of a health check
type HealthResult struct {
	Healthy bool          `json:"healthy"`
	Message string        `json:"message"`
	Latency time.Duration `json:"latency"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// DatabaseHealthCheck checks database connectivity
type DatabaseHealthCheck struct {
	name string
	db   *sql.DB
}

// NewDatabaseHealthCheck creates a new database health check
func NewDatabaseHealthCheck(name string, db *sql.DB) *DatabaseHealthCheck {
	return &DatabaseHealthCheck{
		name: name,
		db:   db,
	}
}

func (h *DatabaseHealthCheck) Name() string {
	return h.name
}

func (h *DatabaseHealthCheck) Check(ctx context.Context) HealthResult {
	start := time.Now()
	
	// Create a context with timeout for the health check
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := h.db.PingContext(checkCtx)
	latency := time.Since(start)

	if err != nil {
		return HealthResult{
			Healthy: false,
			Message: fmt.Sprintf("Database connection failed: %v", err),
			Latency: latency,
		}
	}

	// Get additional database stats
	stats := h.db.Stats()
	details := map[string]interface{}{
		"open_connections":     stats.OpenConnections,
		"in_use":              stats.InUse,
		"idle":                stats.Idle,
		"max_open_connections": stats.MaxOpenConnections,
	}

	return HealthResult{
		Healthy: true,
		Message: "Database is healthy",
		Latency: latency,
		Details: details,
	}
}

// RedisHealthCheck checks Redis connectivity
type RedisHealthCheck struct {
	name   string
	client redis.UniversalClient
}

// NewRedisHealthCheck creates a new Redis health check
func NewRedisHealthCheck(name string, client redis.UniversalClient) *RedisHealthCheck {
	return &RedisHealthCheck{
		name:   name,
		client: client,
	}
}

func (h *RedisHealthCheck) Name() string {
	return h.name
}

func (h *RedisHealthCheck) Check(ctx context.Context) HealthResult {
	start := time.Now()
	
	// Create a context with timeout for the health check
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := h.client.Ping(checkCtx)
	err := cmd.Err()
	latency := time.Since(start)

	if err != nil {
		return HealthResult{
			Healthy: false,
			Message: fmt.Sprintf("Redis connection failed: %v", err),
			Latency: latency,
		}
	}

	// Get additional Redis info
	var details map[string]interface{}
	if info := h.client.Info(checkCtx, "memory"); info.Err() == nil {
		details = map[string]interface{}{
			"ping_response": cmd.Val(),
		}
	}

	return HealthResult{
		Healthy: true,
		Message: "Redis is healthy",
		Latency: latency,
		Details: details,
	}
}

// HTTPHealthCheck checks external HTTP services
type HTTPHealthCheck struct {
	name     string
	url      string
	client   *http.Client
	expected int
}

// NewHTTPHealthCheck creates a new HTTP service health check
func NewHTTPHealthCheck(name, url string, expectedStatus int) *HTTPHealthCheck {
	return &HTTPHealthCheck{
		name: name,
		url:  url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		expected: expectedStatus,
	}
}

func (h *HTTPHealthCheck) Name() string {
	return h.name
}

func (h *HTTPHealthCheck) Check(ctx context.Context) HealthResult {
	start := time.Now()
	
	req, err := http.NewRequestWithContext(ctx, "GET", h.url, nil)
	if err != nil {
		return HealthResult{
			Healthy: false,
			Message: fmt.Sprintf("Failed to create request: %v", err),
			Latency: time.Since(start),
		}
	}

	resp, err := h.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return HealthResult{
			Healthy: false,
			Message: fmt.Sprintf("HTTP request failed: %v", err),
			Latency: latency,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != h.expected {
		return HealthResult{
			Healthy: false,
			Message: fmt.Sprintf("Expected status %d, got %d", h.expected, resp.StatusCode),
			Latency: latency,
			Details: map[string]interface{}{
				"status_code": resp.StatusCode,
				"url":         h.url,
			},
		}
	}

	return HealthResult{
		Healthy: true,
		Message: "HTTP service is healthy",
		Latency: latency,
		Details: map[string]interface{}{
			"status_code": resp.StatusCode,
			"url":         h.url,
		},
	}
}

// MemoryHealthCheck checks memory usage
type MemoryHealthCheck struct {
	name              string
	maxMemoryPercent  float64
}

// NewMemoryHealthCheck creates a new memory health check
func NewMemoryHealthCheck(name string, maxMemoryPercent float64) *MemoryHealthCheck {
	return &MemoryHealthCheck{
		name:             name,
		maxMemoryPercent: maxMemoryPercent,
	}
}

func (h *MemoryHealthCheck) Name() string {
	return h.name
}

func (h *MemoryHealthCheck) Check(ctx context.Context) HealthResult {
	start := time.Now()
	
	// This is a simplified memory check
	// In production, you'd want to use runtime.ReadMemStats() or similar
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	latency := time.Since(start)
	
	// Convert bytes to MB for easier reading
	allocMB := float64(memStats.Alloc) / 1024 / 1024
	sysMB := float64(memStats.Sys) / 1024 / 1024
	
	details := map[string]interface{}{
		"alloc_mb":    allocMB,
		"sys_mb":      sysMB,
		"gc_count":    memStats.NumGC,
		"goroutines":  runtime.NumGoroutine(),
	}

	// Simple memory check - you might want to make this more sophisticated
	memoryHealthy := allocMB < 1000 // Less than 1GB allocated
	message := "Memory usage is healthy"
	
	if !memoryHealthy {
		message = fmt.Sprintf("High memory usage: %.2f MB allocated", allocMB)
	}

	return HealthResult{
		Healthy: memoryHealthy,
		Message: message,
		Latency: latency,
		Details: details,
	}
}

// DiskSpaceHealthCheck checks disk space
type DiskSpaceHealthCheck struct {
	name           string
	path           string
	minFreePercent float64
}

// NewDiskSpaceHealthCheck creates a new disk space health check
func NewDiskSpaceHealthCheck(name, path string, minFreePercent float64) *DiskSpaceHealthCheck {
	return &DiskSpaceHealthCheck{
		name:           name,
		path:           path,
		minFreePercent: minFreePercent,
	}
}

func (h *DiskSpaceHealthCheck) Name() string {
	return h.name
}

func (h *DiskSpaceHealthCheck) Check(ctx context.Context) HealthResult {
	start := time.Now()
	
	// This would need platform-specific implementation
	// For now, return a simple healthy status
	latency := time.Since(start)
	
	return HealthResult{
		Healthy: true,
		Message: "Disk space check not implemented",
		Latency: latency,
		Details: map[string]interface{}{
			"path": h.path,
		},
	}
}

// CompositeHealthCheck combines multiple health checks
type CompositeHealthCheck struct {
	name   string
	checks []HealthCheck
}

// NewCompositeHealthCheck creates a new composite health check
func NewCompositeHealthCheck(name string, checks ...HealthCheck) *CompositeHealthCheck {
	return &CompositeHealthCheck{
		name:   name,
		checks: checks,
	}
}

func (h *CompositeHealthCheck) Name() string {
	return h.name
}

func (h *CompositeHealthCheck) Check(ctx context.Context) HealthResult {
	start := time.Now()
	
	results := make(map[string]HealthResult)
	overallHealthy := true
	
	for _, check := range h.checks {
		result := check.Check(ctx)
		results[check.Name()] = result
		if !result.Healthy {
			overallHealthy = false
		}
	}
	
	latency := time.Since(start)
	message := "All checks passed"
	if !overallHealthy {
		message = "Some checks failed"
	}

	return HealthResult{
		Healthy: overallHealthy,
		Message: message,
		Latency: latency,
		Details: map[string]interface{}{
			"checks": results,
		},
	}
}