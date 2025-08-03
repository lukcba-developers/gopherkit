package circuitbreaker

import (
	"time"
)

// State representa el estado del circuit breaker
type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

// String implementa Stringer para State
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// Config define la configuración de un circuit breaker
type Config struct {
	// Name es el nombre del circuit breaker para métricas y logs
	Name string `json:"name"`
	
	// MaxRequests es el número máximo de requests permitidas cuando está half-open
	MaxRequests uint32 `json:"max_requests"`
	
	// Interval es el período cíclico del estado closed
	// Si Interval es 0, el circuit breaker no limpia las métricas internas
	Interval time.Duration `json:"interval"`
	
	// Timeout es el período después del cual el breaker pasa de open a half-open
	Timeout time.Duration `json:"timeout"`
	
	// ReadyToTrip es una función que determina si el breaker debe activarse (ir a open)
	// Por defecto, usa FailureThreshold
	ReadyToTrip func(counts Counts) bool `json:"-"`
	
	// OnStateChange es un callback que se ejecuta cuando el estado cambia
	OnStateChange func(name string, from State, to State) `json:"-"`
	
	// IsSuccessful determina si un resultado es exitoso o no
	// Por defecto, considera exitoso cualquier error == nil
	IsSuccessful func(error) bool `json:"-"`
	
	// FailureThreshold es el umbral de fallos para activar el breaker
	// Solo se usa si ReadyToTrip es nil
	FailureThreshold uint32 `json:"failure_threshold"`
	
	// FailureRatio es el ratio de fallos (0.0-1.0) para activar el breaker
	// Solo se usa si ReadyToTrip es nil y FailureThreshold es 0
	FailureRatio float64 `json:"failure_ratio"`
	
	// MinimumRequestThreshold es el número mínimo de requests antes de evaluar el ratio
	MinimumRequestThreshold uint32 `json:"minimum_request_threshold"`
}

// Counts representa las métricas del circuit breaker
type Counts struct {
	Requests             uint32    `json:"requests"`
	TotalSuccesses       uint32    `json:"total_successes"`
	TotalFailures        uint32    `json:"total_failures"`
	ConsecutiveSuccesses uint32    `json:"consecutive_successes"`
	ConsecutiveFailures  uint32    `json:"consecutive_failures"`
	LastRequestTime      time.Time `json:"last_request_time"`
}

// FailureRatio calcula el ratio de fallos
func (c Counts) FailureRatio() float64 {
	if c.Requests == 0 {
		return 0.0
	}
	return float64(c.TotalFailures) / float64(c.Requests)
}

// SuccessRatio calcula el ratio de éxitos
func (c Counts) SuccessRatio() float64 {
	if c.Requests == 0 {
		return 0.0
	}
	return float64(c.TotalSuccesses) / float64(c.Requests)
}

// DefaultConfig retorna una configuración por defecto
func DefaultConfig(name string) *Config {
	return &Config{
		Name:                    name,
		MaxRequests:             1,
		Interval:                60 * time.Second,
		Timeout:                 30 * time.Second,
		FailureThreshold:        5,
		FailureRatio:            0.6,
		MinimumRequestThreshold: 3,
		ReadyToTrip:             nil, // Usa la lógica por defecto
		OnStateChange:           nil, // Sin callback por defecto
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	}
}

// HTTPClientConfig retorna configuración optimizada para clientes HTTP
func HTTPClientConfig(name string) *Config {
	config := DefaultConfig(name)
	config.MaxRequests = 3
	config.Interval = 30 * time.Second
	config.Timeout = 10 * time.Second
	config.FailureThreshold = 3
	config.FailureRatio = 0.5
	config.MinimumRequestThreshold = 5
	return config
}

// DatabaseConfig retorna configuración optimizada para bases de datos
func DatabaseConfig(name string) *Config {
	config := DefaultConfig(name)
	config.MaxRequests = 1
	config.Interval = 120 * time.Second
	config.Timeout = 60 * time.Second
	config.FailureThreshold = 3
	config.FailureRatio = 0.7
	config.MinimumRequestThreshold = 2
	return config
}

// ExternalServiceConfig retorna configuración para servicios externos
func ExternalServiceConfig(name string) *Config {
	config := DefaultConfig(name)
	config.MaxRequests = 2
	config.Interval = 45 * time.Second
	config.Timeout = 20 * time.Second
	config.FailureThreshold = 5
	config.FailureRatio = 0.4
	config.MinimumRequestThreshold = 10
	return config
}

// Validate valida la configuración
func (c *Config) Validate() error {
	if c.Name == "" {
		return ErrInvalidName
	}
	
	if c.MaxRequests == 0 {
		return ErrInvalidMaxRequests
	}
	
	if c.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	
	if c.Interval < 0 {
		return ErrInvalidInterval
	}
	
	if c.FailureRatio < 0 || c.FailureRatio > 1 {
		return ErrInvalidFailureRatio
	}
	
	// Si no hay ReadyToTrip definido, debe haber al menos un criterio
	if c.ReadyToTrip == nil && c.FailureThreshold == 0 && c.FailureRatio == 0 {
		return ErrNoFailureCriteria
	}
	
	return nil
}

// WithStateChangeCallback configura el callback de cambio de estado
func (c *Config) WithStateChangeCallback(callback func(name string, from State, to State)) *Config {
	c.OnStateChange = callback
	return c
}

// WithSuccessFunction configura la función de éxito
func (c *Config) WithSuccessFunction(isSuccessful func(error) bool) *Config {
	c.IsSuccessful = isSuccessful
	return c
}

// WithReadyToTrip configura la función personalizada de activación
func (c *Config) WithReadyToTrip(readyToTrip func(counts Counts) bool) *Config {
	c.ReadyToTrip = readyToTrip
	return c
}