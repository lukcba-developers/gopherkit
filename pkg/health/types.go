package health

import (
	"context"
	"time"
)

// Status representa el estado de salud de un componente
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
	StatusUnknown   Status = "unknown"
)

// ComponentHealth representa la salud de un componente individual
type ComponentHealth struct {
	Status      Status                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    time.Duration          `json:"duration"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Error       string                 `json:"error,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
}

// SystemInfo representa información del sistema
type SystemInfo struct {
	Version     string    `json:"version"`
	BuildTime   string    `json:"build_time,omitempty"`
	GitCommit   string    `json:"git_commit,omitempty"`
	GoVersion   string    `json:"go_version"`
	Platform    string    `json:"platform"`
	StartTime   time.Time `json:"start_time"`
	Uptime      string    `json:"uptime"`
	Environment string    `json:"environment,omitempty"`
}

// HealthResponse representa la respuesta completa de health check
type HealthResponse struct {
	Status      Status                     `json:"status"`
	Service     string                     `json:"service"`
	Version     string                     `json:"version"`
	Timestamp   time.Time                  `json:"timestamp"`
	Uptime      time.Duration              `json:"uptime"`
	Components  map[string]ComponentHealth `json:"components"`
	SystemInfo  SystemInfo                 `json:"system_info"`
	RequestID   string                     `json:"request_id,omitempty"`
	Environment string                     `json:"environment,omitempty"`
}

// HealthChecker define la interfaz para health checkers
type HealthChecker interface {
	// Name retorna el nombre del componente
	Name() string
	
	// Check ejecuta el health check
	Check(ctx context.Context) ComponentHealth
	
	// IsRequired indica si este componente es requerido para la salud general
	IsRequired() bool
	
	// Timeout retorna el timeout para este health check
	Timeout() time.Duration
}

// Checker agregado que combina múltiples health checkers
type Checker struct {
	checkers    []HealthChecker
	service     string
	version     string
	startTime   time.Time
	systemInfo  SystemInfo
	environment string
}

// CheckerOption define opciones para configurar el Checker
type CheckerOption func(*Checker)

// CheckResult representa el resultado de un health check individual
type CheckResult struct {
	Name   string
	Health ComponentHealth
	Error  error
}

// OverallStatus calcula el estado general basado en los componentes
func OverallStatus(components map[string]ComponentHealth) Status {
	if len(components) == 0 {
		return StatusUnknown
	}
	
	hasUnhealthy := false
	hasDegraded := false
	
	for _, component := range components {
		switch component.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		case StatusUnknown:
			// Si algún componente crítico es unknown, consideramos degraded
			hasDegraded = true
		}
	}
	
	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	
	return StatusHealthy
}

// NewComponentHealth crea un nuevo ComponentHealth
func NewComponentHealth(status Status, message string) ComponentHealth {
	return ComponentHealth{
		Status:      status,
		Message:     message,
		Timestamp:   time.Now(),
		LastChecked: time.Now(),
		Metadata:    make(map[string]interface{}),
	}
}

// WithDuration añade duración al ComponentHealth
func (c ComponentHealth) WithDuration(duration time.Duration) ComponentHealth {
	c.Duration = duration
	return c
}

// WithMetadata añade metadata al ComponentHealth
func (c ComponentHealth) WithMetadata(key string, value interface{}) ComponentHealth {
	if c.Metadata == nil {
		c.Metadata = make(map[string]interface{})
	}
	c.Metadata[key] = value
	return c
}

// WithError añade un error al ComponentHealth
func (c ComponentHealth) WithError(err error) ComponentHealth {
	if err != nil {
		c.Error = err.Error()
		if c.Status == StatusHealthy {
			c.Status = StatusUnhealthy
		}
	}
	return c
}