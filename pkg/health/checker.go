package health

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// NewChecker crea un nuevo health checker agregado
func NewChecker(service, version string, opts ...CheckerOption) *Checker {
	checker := &Checker{
		checkers:  make([]HealthChecker, 0),
		service:   service,
		version:   version,
		startTime: time.Now(),
		systemInfo: SystemInfo{
			Version:   version,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			StartTime: time.Now(),
		},
	}
	
	for _, opt := range opts {
		opt(checker)
	}
	
	return checker
}

// WithEnvironment configura el environment
func WithEnvironment(env string) CheckerOption {
	return func(c *Checker) {
		c.environment = env
		c.systemInfo.Environment = env
	}
}

// WithBuildInfo configura información de build
func WithBuildInfo(buildTime, gitCommit string) CheckerOption {
	return func(c *Checker) {
		c.systemInfo.BuildTime = buildTime
		c.systemInfo.GitCommit = gitCommit
	}
}

// AddChecker añade un health checker
func (c *Checker) AddChecker(checker HealthChecker) {
	c.checkers = append(c.checkers, checker)
}

// Check ejecuta todos los health checks
func (c *Checker) Check(ctx context.Context) HealthResponse {
	startTime := time.Now()
	
	// Actualizar información del sistema
	c.systemInfo.Uptime = time.Since(c.startTime).String()
	
	response := HealthResponse{
		Service:     c.service,
		Version:     c.version,
		Timestamp:   startTime,
		Uptime:      time.Since(c.startTime),
		Components:  make(map[string]ComponentHealth),
		SystemInfo:  c.systemInfo,
		Environment: c.environment,
		RequestID:   uuid.New().String(),
	}
	
	if len(c.checkers) == 0 {
		response.Status = StatusHealthy
		return response
	}
	
	// Ejecutar checks en paralelo
	results := c.runChecksParallel(ctx)
	
	// Procesar resultados
	for _, result := range results {
		response.Components[result.Name] = result.Health
		
		if result.Error != nil {
			logrus.WithFields(logrus.Fields{
				"component": result.Name,
				"error":     result.Error,
				"duration":  result.Health.Duration,
			}).Warn("Health check failed")
		}
	}
	
	// Calcular estado general
	response.Status = OverallStatus(response.Components)
	
	logrus.WithFields(logrus.Fields{
		"service":         c.service,
		"overall_status":  response.Status,
		"components":      len(response.Components),
		"check_duration":  time.Since(startTime),
	}).Debug("Health check completed")
	
	return response
}

// runChecksParallel ejecuta todos los health checks en paralelo
func (c *Checker) runChecksParallel(ctx context.Context) []CheckResult {
	results := make([]CheckResult, len(c.checkers))
	var wg sync.WaitGroup
	
	for i, checker := range c.checkers {
		wg.Add(1)
		go func(index int, hc HealthChecker) {
			defer wg.Done()
			
			// Crear contexto con timeout específico para este checker
			checkCtx, cancel := context.WithTimeout(ctx, hc.Timeout())
			defer cancel()
			
			startTime := time.Now()
			
			// Ejecutar el check con recover para evitar panics
			var health ComponentHealth
			var err error
			
			func() {
				defer func() {
					if r := recover(); r != nil {
						health = NewComponentHealth(StatusUnhealthy, "Health check panicked")
						health = health.WithMetadata("panic", r)
						err = nil // No propagamos el panic como error
					}
				}()
				
				health = hc.Check(checkCtx)
			}()
			
			// Asegurar que la duración esté establecida
			if health.Duration == 0 {
				health.Duration = time.Since(startTime)
			}
			
			results[index] = CheckResult{
				Name:   hc.Name(),
				Health: health,
				Error:  err,
			}
			
		}(i, checker)
	}
	
	wg.Wait()
	return results
}

// IsHealthy verifica si el servicio está saludable
func (c *Checker) IsHealthy(ctx context.Context) bool {
	response := c.Check(ctx)
	return response.Status == StatusHealthy
}

// IsReady verifica si el servicio está listo (readiness probe)
func (c *Checker) IsReady(ctx context.Context) bool {
	response := c.Check(ctx)
	// El servicio está listo si no está completamente unhealthy
	return response.Status != StatusUnhealthy
}

// IsLive verifica si el servicio está vivo (liveness probe)
func (c *Checker) IsLive(ctx context.Context) bool {
	// Liveness probe simple - solo verifica que la aplicación responda
	// No ejecuta health checks complejos para evitar reiniciar por problemas temporales
	return true
}

// GetComponentStatus obtiene el estado de un componente específico
func (c *Checker) GetComponentStatus(ctx context.Context, componentName string) (ComponentHealth, bool) {
	for _, checker := range c.checkers {
		if checker.Name() == componentName {
			health := checker.Check(ctx)
			return health, true
		}
	}
	
	return ComponentHealth{}, false
}

// ListComponents retorna la lista de nombres de componentes
func (c *Checker) ListComponents() []string {
	names := make([]string, len(c.checkers))
	for i, checker := range c.checkers {
		names[i] = checker.Name()
	}
	return names
}

// GetServiceInfo retorna información básica del servicio
func (c *Checker) GetServiceInfo() map[string]interface{} {
	return map[string]interface{}{
		"service":     c.service,
		"version":     c.version,
		"uptime":      time.Since(c.startTime).String(),
		"start_time":  c.startTime,
		"environment": c.environment,
		"components":  len(c.checkers),
	}
}