package health

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusHealthy, "healthy"},
		{StatusUnhealthy, "unhealthy"},
		{StatusDegraded, "degraded"},
		{StatusUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestOverallStatus(t *testing.T) {
	t.Run("empty components", func(t *testing.T) {
		components := make(map[string]ComponentHealth)
		status := OverallStatus(components)
		assert.Equal(t, StatusUnknown, status)
	})

	t.Run("all healthy", func(t *testing.T) {
		components := map[string]ComponentHealth{
			"db":    {Status: StatusHealthy},
			"redis": {Status: StatusHealthy},
		}
		status := OverallStatus(components)
		assert.Equal(t, StatusHealthy, status)
	})

	t.Run("one degraded", func(t *testing.T) {
		components := map[string]ComponentHealth{
			"db":    {Status: StatusHealthy},
			"redis": {Status: StatusDegraded},
		}
		status := OverallStatus(components)
		assert.Equal(t, StatusDegraded, status)
	})

	t.Run("one unhealthy", func(t *testing.T) {
		components := map[string]ComponentHealth{
			"db":    {Status: StatusHealthy},
			"redis": {Status: StatusUnhealthy},
		}
		status := OverallStatus(components)
		assert.Equal(t, StatusUnhealthy, status)
	})

	t.Run("unhealthy takes precedence over degraded", func(t *testing.T) {
		components := map[string]ComponentHealth{
			"db":    {Status: StatusDegraded},
			"redis": {Status: StatusUnhealthy},
		}
		status := OverallStatus(components)
		assert.Equal(t, StatusUnhealthy, status)
	})

	t.Run("unknown considered degraded", func(t *testing.T) {
		components := map[string]ComponentHealth{
			"db":    {Status: StatusHealthy},
			"redis": {Status: StatusUnknown},
		}
		status := OverallStatus(components)
		assert.Equal(t, StatusDegraded, status)
	})

	t.Run("complex scenario", func(t *testing.T) {
		components := map[string]ComponentHealth{
			"db":      {Status: StatusHealthy},
			"redis":   {Status: StatusDegraded},
			"api":     {Status: StatusHealthy},
			"storage": {Status: StatusUnknown},
		}
		status := OverallStatus(components)
		assert.Equal(t, StatusDegraded, status)
	})
}

func TestNewComponentHealth(t *testing.T) {
	status := StatusHealthy
	message := "All systems operational"
	
	component := NewComponentHealth(status, message)
	
	assert.Equal(t, status, component.Status)
	assert.Equal(t, message, component.Message)
	assert.NotZero(t, component.Timestamp)
	assert.NotZero(t, component.LastChecked)
	assert.NotNil(t, component.Metadata)
	assert.Len(t, component.Metadata, 0)
}

func TestComponentHealth_WithDuration(t *testing.T) {
	component := NewComponentHealth(StatusHealthy, "test")
	duration := 100 * time.Millisecond
	
	result := component.WithDuration(duration)
	
	assert.Equal(t, duration, result.Duration)
	assert.Equal(t, component.Status, result.Status)
	assert.Equal(t, component.Message, result.Message)
}

func TestComponentHealth_WithMetadata(t *testing.T) {
	component := NewComponentHealth(StatusHealthy, "test")
	
	result := component.WithMetadata("key1", "value1").
		WithMetadata("key2", 42).
		WithMetadata("key3", true)
	
	assert.Len(t, result.Metadata, 3)
	assert.Equal(t, "value1", result.Metadata["key1"])
	assert.Equal(t, 42, result.Metadata["key2"])
	assert.Equal(t, true, result.Metadata["key3"])
}

func TestComponentHealth_WithMetadata_InitializesMap(t *testing.T) {
	component := ComponentHealth{
		Status:  StatusHealthy,
		Message: "test",
	}
	// Metadata is nil initially
	assert.Nil(t, component.Metadata)
	
	result := component.WithMetadata("key", "value")
	
	assert.NotNil(t, result.Metadata)
	assert.Equal(t, "value", result.Metadata["key"])
}

func TestComponentHealth_WithError(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		component := NewComponentHealth(StatusHealthy, "test")
		testErr := errors.New("connection failed")
		
		result := component.WithError(testErr)
		
		assert.Equal(t, "connection failed", result.Error)
		assert.Equal(t, StatusUnhealthy, result.Status) // Status changed from healthy
	})

	t.Run("with nil error", func(t *testing.T) {
		component := NewComponentHealth(StatusHealthy, "test")
		
		result := component.WithError(nil)
		
		assert.Empty(t, result.Error)
		assert.Equal(t, StatusHealthy, result.Status) // Status unchanged
	})

	t.Run("error doesn't override unhealthy status", func(t *testing.T) {
		component := NewComponentHealth(StatusUnhealthy, "already unhealthy")
		testErr := errors.New("additional error")
		
		result := component.WithError(testErr)
		
		assert.Equal(t, "additional error", result.Error)
		assert.Equal(t, StatusUnhealthy, result.Status) // Status remains unhealthy
	})

	t.Run("error doesn't override degraded status", func(t *testing.T) {
		component := NewComponentHealth(StatusDegraded, "degraded")
		testErr := errors.New("additional error")
		
		result := component.WithError(testErr)
		
		assert.Equal(t, "additional error", result.Error)
		assert.Equal(t, StatusDegraded, result.Status) // Status remains degraded
	})
}

func TestComponentHealth_ChainedMethods(t *testing.T) {
	testErr := errors.New("test error")
	duration := 50 * time.Millisecond
	
	component := NewComponentHealth(StatusHealthy, "test").
		WithDuration(duration).
		WithMetadata("key1", "value1").
		WithMetadata("key2", 123).
		WithError(testErr)
	
	assert.Equal(t, StatusUnhealthy, component.Status)
	assert.Equal(t, "test", component.Message)
	assert.Equal(t, duration, component.Duration)
	assert.Equal(t, "test error", component.Error)
	assert.Len(t, component.Metadata, 2)
	assert.Equal(t, "value1", component.Metadata["key1"])
	assert.Equal(t, 123, component.Metadata["key2"])
}

func TestSystemInfo_Fields(t *testing.T) {
	systemInfo := SystemInfo{
		Version:     "v1.0.0",
		BuildTime:   "2025-01-01T00:00:00Z",
		GitCommit:   "abc123",
		GoVersion:   "go1.21",
		Platform:    "linux/amd64",
		StartTime:   time.Now(),
		Uptime:      "1h30m",
		Environment: "production",
	}
	
	assert.Equal(t, "v1.0.0", systemInfo.Version)
	assert.Equal(t, "2025-01-01T00:00:00Z", systemInfo.BuildTime)
	assert.Equal(t, "abc123", systemInfo.GitCommit)
	assert.Equal(t, "go1.21", systemInfo.GoVersion)
	assert.Equal(t, "linux/amd64", systemInfo.Platform)
	assert.Equal(t, "1h30m", systemInfo.Uptime)
	assert.Equal(t, "production", systemInfo.Environment)
	assert.NotZero(t, systemInfo.StartTime)
}

func TestHealthResponse_Fields(t *testing.T) {
	timestamp := time.Now()
	uptime := 2 * time.Hour
	components := map[string]ComponentHealth{
		"db": NewComponentHealth(StatusHealthy, "OK"),
	}
	systemInfo := SystemInfo{Version: "v1.0.0"}
	
	response := HealthResponse{
		Status:      StatusHealthy,
		Service:     "test-service",
		Version:     "v1.0.0",
		Timestamp:   timestamp,
		Uptime:      uptime,
		Components:  components,
		SystemInfo:  systemInfo,
		RequestID:   "req-123",
		Environment: "test",
	}
	
	assert.Equal(t, StatusHealthy, response.Status)
	assert.Equal(t, "test-service", response.Service)
	assert.Equal(t, "v1.0.0", response.Version)
	assert.Equal(t, timestamp, response.Timestamp)
	assert.Equal(t, uptime, response.Uptime)
	assert.Len(t, response.Components, 1)
	assert.Contains(t, response.Components, "db")
	assert.Equal(t, "v1.0.0", response.SystemInfo.Version)
	assert.Equal(t, "req-123", response.RequestID)
	assert.Equal(t, "test", response.Environment)
}

func TestCheckResult_Fields(t *testing.T) {
	health := NewComponentHealth(StatusHealthy, "OK")
	testErr := errors.New("check failed")
	
	result := CheckResult{
		Name:   "database",
		Health: health,
		Error:  testErr,
	}
	
	assert.Equal(t, "database", result.Name)
	assert.Equal(t, health, result.Health)
	assert.Equal(t, testErr, result.Error)
}

// Benchmark tests
func BenchmarkOverallStatus_SmallComponents(b *testing.B) {
	components := map[string]ComponentHealth{
		"db":    {Status: StatusHealthy},
		"redis": {Status: StatusHealthy},
		"api":   {Status: StatusDegraded},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		OverallStatus(components)
	}
}

func BenchmarkOverallStatus_LargeComponents(b *testing.B) {
	components := make(map[string]ComponentHealth)
	for i := 0; i < 100; i++ {
		name := "component" + string(rune(i))
		status := StatusHealthy
		if i%10 == 0 {
			status = StatusDegraded
		}
		components[name] = ComponentHealth{Status: status}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		OverallStatus(components)
	}
}

func BenchmarkNewComponentHealth(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewComponentHealth(StatusHealthy, "test message")
	}
}

func BenchmarkComponentHealth_WithMetadata(b *testing.B) {
	component := NewComponentHealth(StatusHealthy, "test")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		component.WithMetadata("key", "value")
	}
}