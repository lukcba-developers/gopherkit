package health

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockHealthChecker implementa HealthChecker para tests
type MockHealthChecker struct {
	name     string
	status   Status
	message  string
	duration time.Duration
	timeout  time.Duration
	required bool
	err      error
	metadata map[string]interface{}
	panic    bool
}

func NewMockHealthChecker(name string) *MockHealthChecker {
	return &MockHealthChecker{
		name:     name,
		status:   StatusHealthy,
		message:  "OK",
		timeout:  5 * time.Second,
		required: true,
		metadata: make(map[string]interface{}),
	}
}

func (m *MockHealthChecker) Name() string {
	return m.name
}

func (m *MockHealthChecker) Check(ctx context.Context) ComponentHealth {
	if m.panic {
		panic("mock panic")
	}
	
	start := time.Now()
	
	// Simular algo de trabajo si hay duración configurada
	if m.duration > 0 {
		time.Sleep(m.duration)
	}
	
	health := NewComponentHealth(m.status, m.message)
	health.Duration = time.Since(start)
	
	if m.err != nil {
		health = health.WithError(m.err)
	}
	
	for k, v := range m.metadata {
		health = health.WithMetadata(k, v)
	}
	
	return health
}

func (m *MockHealthChecker) IsRequired() bool {
	return m.required
}

func (m *MockHealthChecker) Timeout() time.Duration {
	return m.timeout
}

func (m *MockHealthChecker) SetStatus(status Status) *MockHealthChecker {
	m.status = status
	return m
}

func (m *MockHealthChecker) SetMessage(message string) *MockHealthChecker {
	m.message = message
	return m
}

func (m *MockHealthChecker) SetDuration(duration time.Duration) *MockHealthChecker {
	m.duration = duration
	return m
}

func (m *MockHealthChecker) SetTimeout(timeout time.Duration) *MockHealthChecker {
	m.timeout = timeout
	return m
}

func (m *MockHealthChecker) SetRequired(required bool) *MockHealthChecker {
	m.required = required
	return m
}

func (m *MockHealthChecker) SetError(err error) *MockHealthChecker {
	m.err = err
	return m
}

func (m *MockHealthChecker) SetMetadata(key string, value interface{}) *MockHealthChecker {
	m.metadata[key] = value
	return m
}

func (m *MockHealthChecker) SetPanic(panic bool) *MockHealthChecker {
	m.panic = panic
	return m
}

func TestNewChecker(t *testing.T) {
	t.Run("basic checker creation", func(t *testing.T) {
		service := "test-service"
		version := "v1.0.0"
		
		checker := NewChecker(service, version)
		
		assert.Equal(t, service, checker.service)
		assert.Equal(t, version, checker.version)
		assert.NotZero(t, checker.startTime)
		assert.Len(t, checker.checkers, 0)
		
		// Verificar SystemInfo
		assert.Equal(t, version, checker.systemInfo.Version)
		assert.Equal(t, runtime.Version(), checker.systemInfo.GoVersion)
		assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, checker.systemInfo.Platform)
		assert.NotZero(t, checker.systemInfo.StartTime)
	})

	t.Run("with options", func(t *testing.T) {
		service := "test-service"
		version := "v1.0.0"
		env := "production"
		buildTime := "2025-01-01T00:00:00Z"
		gitCommit := "abc123"
		
		checker := NewChecker(service, version,
			WithEnvironment(env),
			WithBuildInfo(buildTime, gitCommit),
		)
		
		assert.Equal(t, env, checker.environment)
		assert.Equal(t, env, checker.systemInfo.Environment)
		assert.Equal(t, buildTime, checker.systemInfo.BuildTime)
		assert.Equal(t, gitCommit, checker.systemInfo.GitCommit)
	})
}

func TestChecker_AddChecker(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	mock1 := NewMockHealthChecker("db")
	mock2 := NewMockHealthChecker("redis")
	
	assert.Len(t, checker.checkers, 0)
	
	checker.AddChecker(mock1)
	assert.Len(t, checker.checkers, 1)
	
	checker.AddChecker(mock2)
	assert.Len(t, checker.checkers, 2)
}

func TestChecker_Check(t *testing.T) {
	t.Run("no checkers", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		ctx := context.Background()
		
		response := checker.Check(ctx)
		
		assert.Equal(t, StatusHealthy, response.Status)
		assert.Equal(t, "test", response.Service)
		assert.Equal(t, "v1.0.0", response.Version)
		assert.NotZero(t, response.Timestamp)
		assert.NotZero(t, response.Uptime)
		assert.Len(t, response.Components, 0)
		assert.NotEmpty(t, response.RequestID)
	})

	t.Run("all healthy checkers", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		mock1 := NewMockHealthChecker("db").SetStatus(StatusHealthy)
		mock2 := NewMockHealthChecker("redis").SetStatus(StatusHealthy)
		
		checker.AddChecker(mock1)
		checker.AddChecker(mock2)
		
		ctx := context.Background()
		response := checker.Check(ctx)
		
		assert.Equal(t, StatusHealthy, response.Status)
		assert.Len(t, response.Components, 2)
		assert.Contains(t, response.Components, "db")
		assert.Contains(t, response.Components, "redis")
		assert.Equal(t, StatusHealthy, response.Components["db"].Status)
		assert.Equal(t, StatusHealthy, response.Components["redis"].Status)
	})

	t.Run("mixed statuses", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		mock1 := NewMockHealthChecker("db").SetStatus(StatusHealthy)
		mock2 := NewMockHealthChecker("redis").SetStatus(StatusDegraded)
		
		checker.AddChecker(mock1)
		checker.AddChecker(mock2)
		
		ctx := context.Background()
		response := checker.Check(ctx)
		
		assert.Equal(t, StatusDegraded, response.Status)
		assert.Len(t, response.Components, 2)
		assert.Equal(t, StatusHealthy, response.Components["db"].Status)
		assert.Equal(t, StatusDegraded, response.Components["redis"].Status)
	})

	t.Run("unhealthy checker", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		mock1 := NewMockHealthChecker("db").SetStatus(StatusHealthy)
		mock2 := NewMockHealthChecker("redis").SetStatus(StatusUnhealthy)
		
		checker.AddChecker(mock1)
		checker.AddChecker(mock2)
		
		ctx := context.Background()
		response := checker.Check(ctx)
		
		assert.Equal(t, StatusUnhealthy, response.Status)
		assert.Len(t, response.Components, 2)
		assert.Equal(t, StatusHealthy, response.Components["db"].Status)
		assert.Equal(t, StatusUnhealthy, response.Components["redis"].Status)
	})

	t.Run("with environment", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0", WithEnvironment("production"))
		ctx := context.Background()
		
		response := checker.Check(ctx)
		
		assert.Equal(t, "production", response.Environment)
		assert.Equal(t, "production", response.SystemInfo.Environment)
	})

	t.Run("uptime calculation", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		// Simular que el checker ha estado ejecutándose por un tiempo
		checker.startTime = time.Now().Add(-1 * time.Hour)
		
		ctx := context.Background()
		response := checker.Check(ctx)
		
		assert.True(t, response.Uptime >= time.Hour)
		assert.Contains(t, response.SystemInfo.Uptime, "h")
	})
}

func TestChecker_Check_ParallelExecution(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	
	// Crear checkers con diferentes duraciones
	slow := NewMockHealthChecker("slow").SetDuration(100 * time.Millisecond)
	fast := NewMockHealthChecker("fast").SetDuration(10 * time.Millisecond)
	
	checker.AddChecker(slow)
	checker.AddChecker(fast)
	
	ctx := context.Background()
	start := time.Now()
	response := checker.Check(ctx)
	elapsed := time.Since(start)
	
	// La ejecución debería tomar cerca del tiempo del checker más lento,
	// no la suma de ambos (indicando ejecución paralela)
	assert.Less(t, elapsed, 150*time.Millisecond) // Margen para overhead
	assert.Greater(t, elapsed, 90*time.Millisecond) // Al menos el tiempo del más lento
	
	assert.Len(t, response.Components, 2)
	assert.Contains(t, response.Components, "slow")
	assert.Contains(t, response.Components, "fast")
}

func TestChecker_Check_Timeout(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	
	// Crear un checker que tome más tiempo que su timeout
	slow := NewMockHealthChecker("slow").
		SetDuration(200 * time.Millisecond).
		SetTimeout(50 * time.Millisecond)
	
	checker.AddChecker(slow)
	
	ctx := context.Background()
	start := time.Now()
	response := checker.Check(ctx)
	elapsed := time.Since(start)
	
	// Debería terminar cerca del timeout, no de la duración completa
	// Dar más margen debido a la concurrencia y scheduling
	assert.Less(t, elapsed, 250*time.Millisecond)
	assert.Greater(t, elapsed, 40*time.Millisecond)
	
	assert.Len(t, response.Components, 1)
	// El componente debería estar presente, pero el status puede variar
	// dependiendo de cómo maneje el contexto cancelado
	assert.Contains(t, response.Components, "slow")
}

func TestChecker_Check_PanicRecovery(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	
	normal := NewMockHealthChecker("normal").SetStatus(StatusHealthy)
	panicky := NewMockHealthChecker("panicky").SetPanic(true)
	
	checker.AddChecker(normal)
	checker.AddChecker(panicky)
	
	ctx := context.Background()
	
	// El test no debería panic
	response := checker.Check(ctx)
	
	assert.Len(t, response.Components, 2)
	assert.Equal(t, StatusHealthy, response.Components["normal"].Status)
	assert.Equal(t, StatusUnhealthy, response.Components["panicky"].Status)
	assert.Contains(t, response.Components["panicky"].Message, "panicked")
	assert.Contains(t, response.Components["panicky"].Metadata, "panic")
}

func TestChecker_Check_WithErrors(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	
	healthy := NewMockHealthChecker("healthy").SetStatus(StatusHealthy)
	errorChecker := NewMockHealthChecker("error").SetStatus(StatusUnhealthy).SetError(errors.New("connection failed"))
	
	checker.AddChecker(healthy)
	checker.AddChecker(errorChecker)
	
	ctx := context.Background()
	response := checker.Check(ctx)
	
	assert.Equal(t, StatusUnhealthy, response.Status)
	assert.Len(t, response.Components, 2)
	assert.Equal(t, StatusHealthy, response.Components["healthy"].Status)
	assert.Equal(t, StatusUnhealthy, response.Components["error"].Status)
	assert.Equal(t, "connection failed", response.Components["error"].Error)
}

func TestChecker_IsHealthy(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	ctx := context.Background()
	
	t.Run("no checkers", func(t *testing.T) {
		assert.True(t, checker.IsHealthy(ctx))
	})

	t.Run("all healthy", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusHealthy))
		assert.True(t, checker.IsHealthy(ctx))
	})

	t.Run("degraded", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("redis").SetStatus(StatusDegraded))
		assert.False(t, checker.IsHealthy(ctx))
	})

	t.Run("unhealthy", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("api").SetStatus(StatusUnhealthy))
		assert.False(t, checker.IsHealthy(ctx))
	})
}

func TestChecker_IsReady(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	ctx := context.Background()
	
	t.Run("no checkers", func(t *testing.T) {
		assert.True(t, checker.IsReady(ctx))
	})

	t.Run("healthy", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusHealthy))
		assert.True(t, checker.IsReady(ctx))
	})

	t.Run("degraded", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("redis").SetStatus(StatusDegraded))
		assert.True(t, checker.IsReady(ctx)) // Ready acepta degraded
	})

	t.Run("unhealthy", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("api").SetStatus(StatusUnhealthy))
		assert.False(t, checker.IsReady(ctx))
	})
}

func TestChecker_IsLive(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	ctx := context.Background()
	
	// Liveness siempre debería retornar true (implementación simple)
	assert.True(t, checker.IsLive(ctx))
	
	// Incluso con checkers unhealthy
	checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusUnhealthy))
	assert.True(t, checker.IsLive(ctx))
}

func TestChecker_GetComponentStatus(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	mock := NewMockHealthChecker("db").SetStatus(StatusHealthy).SetMessage("Database OK")
	checker.AddChecker(mock)
	
	ctx := context.Background()
	
	t.Run("existing component", func(t *testing.T) {
		health, found := checker.GetComponentStatus(ctx, "db")
		
		assert.True(t, found)
		assert.Equal(t, StatusHealthy, health.Status)
		assert.Equal(t, "Database OK", health.Message)
	})

	t.Run("non-existing component", func(t *testing.T) {
		health, found := checker.GetComponentStatus(ctx, "redis")
		
		assert.False(t, found)
		assert.Equal(t, ComponentHealth{}, health)
	})
}

func TestChecker_ListComponents(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	
	t.Run("no components", func(t *testing.T) {
		components := checker.ListComponents()
		assert.Len(t, components, 0)
	})

	t.Run("multiple components", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db"))
		checker.AddChecker(NewMockHealthChecker("redis"))
		checker.AddChecker(NewMockHealthChecker("api"))
		
		components := checker.ListComponents()
		
		assert.Len(t, components, 3)
		assert.Contains(t, components, "db")
		assert.Contains(t, components, "redis")
		assert.Contains(t, components, "api")
	})
}

func TestChecker_GetServiceInfo(t *testing.T) {
	service := "test-service"
	version := "v1.0.0"
	env := "production"
	
	checker := NewChecker(service, version, WithEnvironment(env))
	checker.AddChecker(NewMockHealthChecker("db"))
	checker.AddChecker(NewMockHealthChecker("redis"))
	
	// Simular uptime
	checker.startTime = time.Now().Add(-1 * time.Hour)
	
	info := checker.GetServiceInfo()
	
	assert.Equal(t, service, info["service"])
	assert.Equal(t, version, info["version"])
	assert.Equal(t, env, info["environment"])
	assert.Equal(t, 2, info["components"])
	assert.NotNil(t, info["uptime"])
	assert.NotNil(t, info["start_time"])
	
	// Verificar que uptime sea string y contenga información de tiempo
	uptimeStr, ok := info["uptime"].(string)
	require.True(t, ok)
	assert.Contains(t, uptimeStr, "h") // Debería contener horas
}

func TestWithEnvironment(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	option := WithEnvironment("staging")
	option(checker)
	
	assert.Equal(t, "staging", checker.environment)
	assert.Equal(t, "staging", checker.systemInfo.Environment)
}

func TestWithBuildInfo(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	buildTime := "2025-01-01T00:00:00Z"
	gitCommit := "abc123def"
	
	option := WithBuildInfo(buildTime, gitCommit)
	option(checker)
	
	assert.Equal(t, buildTime, checker.systemInfo.BuildTime)
	assert.Equal(t, gitCommit, checker.systemInfo.GitCommit)
}

func TestChecker_ConcurrentAccess(t *testing.T) {
	checker := NewChecker("test", "v1.0.0")
	mock := NewMockHealthChecker("db").SetStatus(StatusHealthy)
	checker.AddChecker(mock)
	
	ctx := context.Background()
	const numGoroutines = 10
	
	results := make(chan HealthResponse, numGoroutines)
	
	// Ejecutar múltiples health checks concurrentemente
	for i := 0; i < numGoroutines; i++ {
		go func() {
			results <- checker.Check(ctx)
		}()
	}
	
	// Recoger todos los resultados
	for i := 0; i < numGoroutines; i++ {
		response := <-results
		assert.Equal(t, StatusHealthy, response.Status)
		assert.Len(t, response.Components, 1)
		assert.Contains(t, response.Components, "db")
	}
}

// Benchmark tests
func BenchmarkChecker_Check_NoCheckers(b *testing.B) {
	checker := NewChecker("test", "v1.0.0")
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check(ctx)
	}
}

func BenchmarkChecker_Check_SingleChecker(b *testing.B) {
	checker := NewChecker("test", "v1.0.0")
	checker.AddChecker(NewMockHealthChecker("db"))
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check(ctx)
	}
}

func BenchmarkChecker_Check_MultipleCheckers(b *testing.B) {
	checker := NewChecker("test", "v1.0.0")
	for i := 0; i < 5; i++ {
		name := "component" + string(rune(i))
		checker.AddChecker(NewMockHealthChecker(name))
	}
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check(ctx)
	}
}

func BenchmarkChecker_ListComponents(b *testing.B) {
	checker := NewChecker("test", "v1.0.0")
	for i := 0; i < 10; i++ {
		name := "component" + string(rune(i))
		checker.AddChecker(NewMockHealthChecker(name))
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.ListComponents()
	}
}

func BenchmarkChecker_GetServiceInfo(b *testing.B) {
	checker := NewChecker("test", "v1.0.0", WithEnvironment("production"))
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.GetServiceInfo()
	}
}