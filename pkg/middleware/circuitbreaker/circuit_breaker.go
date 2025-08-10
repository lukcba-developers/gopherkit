package circuitbreaker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/lukcba-developers/gopherkit/pkg/errors"
)

// State representa el estado del circuit breaker
type State int

const (
	// StateClosed - circuit breaker cerrado, permite todas las requests
	StateClosed State = iota
	// StateOpen - circuit breaker abierto, rechaza todas las requests
	StateOpen
	// StateHalfOpen - circuit breaker semi-abierto, permite requests limitadas para test
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config configuración del circuit breaker
type Config struct {
	// Threshold de fallos para abrir el circuit
	FailureThreshold int
	
	// Umbral de tasa de fallos (0.0 - 1.0)
	FailureRateThreshold float64
	
	// Número mínimo de requests para evaluar la tasa de fallos
	MinimumRequestThreshold int
	
	// Tiempo que permanece abierto antes de pasar a half-open
	OpenTimeout time.Duration
	
	// Timeout para requests individuales
	RequestTimeout time.Duration
	
	// Número máximo de requests permitidas en estado half-open
	MaxHalfOpenRequests int
	
	// Ventana de tiempo para estadísticas
	StatisticsWindow time.Duration
	
	// Función para determinar si una respuesta es un fallo
	IsFailure func(*http.Response, error) bool
	
	// Función para determinar si se debe aplicar circuit breaker
	ShouldTrip func(State, Metrics) bool
	
	// Callbacks
	OnStateChange func(State, State)
	OnSuccess     func()
	OnFailure     func(error)
}

// DefaultConfig retorna configuración por defecto
func DefaultConfig() *Config {
	return &Config{
		FailureThreshold:         5,
		FailureRateThreshold:     0.6,
		MinimumRequestThreshold:  10,
		OpenTimeout:              30 * time.Second,
		RequestTimeout:           10 * time.Second,
		MaxHalfOpenRequests:      3,
		StatisticsWindow:         60 * time.Second,
		IsFailure:                defaultIsFailure,
		ShouldTrip:               defaultShouldTrip,
	}
}

// defaultIsFailure implementación por defecto para determinar fallos
func defaultIsFailure(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	
	if resp != nil {
		// Considerar códigos 5xx y timeouts como fallos
		return resp.StatusCode >= 500 || resp.StatusCode == 408
	}
	
	return false
}

// defaultShouldTrip implementación por defecto para determinar cuándo abrir el circuit
func defaultShouldTrip(state State, metrics Metrics) bool {
	if state == StateOpen {
		return false
	}
	
	// Verificar umbral de fallos consecutivos
	if metrics.ConsecutiveFailures >= 5 {
		return true
	}
	
	// Verificar tasa de fallos si hay suficientes requests
	if metrics.TotalRequests >= 10 {
		failureRate := float64(metrics.TotalFailures) / float64(metrics.TotalRequests)
		return failureRate >= 0.6
	}
	
	return false
}

// Metrics estadísticas del circuit breaker
type Metrics struct {
	TotalRequests        int64
	TotalSuccesses       int64
	TotalFailures        int64
	ConsecutiveFailures  int64
	ConsecutiveSuccesses int64
	LastFailureTime      time.Time
	LastSuccessTime      time.Time
}

// CircuitBreaker implementa el patrón circuit breaker
type CircuitBreaker struct {
	name    string
	config  *Config
	state   State
	metrics Metrics
	mutex   sync.RWMutex
	tracer  trace.Tracer
	
	// Para controlar requests en estado half-open
	halfOpenRequests int64
	lastStateChange  time.Time
	
	// Window para estadísticas deslizantes
	requestWindow []requestRecord
	windowMutex   sync.Mutex
}

type requestRecord struct {
	timestamp time.Time
	success   bool
}

// NewCircuitBreaker crea una nueva instancia de circuit breaker
func NewCircuitBreaker(name string, config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &CircuitBreaker{
		name:            name,
		config:          config,
		state:           StateClosed,
		tracer:          otel.Tracer("circuit-breaker"),
		lastStateChange: time.Now(),
		requestWindow:   make([]requestRecord, 0),
	}
}

// Execute ejecuta una función con protección del circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	ctx, span := cb.tracer.Start(ctx, fmt.Sprintf("circuit_breaker.%s.execute", cb.name))
	defer span.End()
	
	span.SetAttributes(
		attribute.String("circuit_breaker.name", cb.name),
		attribute.String("circuit_breaker.state", cb.GetState().String()),
	)
	
	// Verificar si podemos proceder
	if !cb.canProceed() {
		span.SetAttributes(attribute.Bool("circuit_breaker.rejected", true))
		return cb.createCircuitOpenError()
	}
	
	// Ejecutar con timeout si está configurado
	if cb.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.config.RequestTimeout)
		defer cancel()
	}
	
	// Ejecutar función
	err := fn()
	
	// Registrar resultado
	cb.recordResult(err == nil)
	
	if err != nil {
		span.SetAttributes(attribute.Bool("circuit_breaker.failure", true))
		if cb.config.OnFailure != nil {
			cb.config.OnFailure(err)
		}
		return err
	}
	
	span.SetAttributes(attribute.Bool("circuit_breaker.success", true))
	if cb.config.OnSuccess != nil {
		cb.config.OnSuccess()
	}
	
	return nil
}

// ExecuteHTTP ejecuta una request HTTP con protección del circuit breaker
func (cb *CircuitBreaker) ExecuteHTTP(ctx context.Context, fn func() (*http.Response, error)) (*http.Response, error) {
	ctx, span := cb.tracer.Start(ctx, fmt.Sprintf("circuit_breaker.%s.execute_http", cb.name))
	defer span.End()
	
	span.SetAttributes(
		attribute.String("circuit_breaker.name", cb.name),
		attribute.String("circuit_breaker.state", cb.GetState().String()),
	)
	
	if !cb.canProceed() {
		span.SetAttributes(attribute.Bool("circuit_breaker.rejected", true))
		return nil, cb.createCircuitOpenError()
	}
	
	if cb.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.config.RequestTimeout)
		defer cancel()
	}
	
	resp, err := fn()
	
	// Determinar si es un fallo usando la función configurada
	isFailure := cb.config.IsFailure(resp, err)
	cb.recordResult(!isFailure)
	
	if isFailure {
		span.SetAttributes(attribute.Bool("circuit_breaker.failure", true))
		if cb.config.OnFailure != nil {
			cb.config.OnFailure(err)
		}
	} else {
		span.SetAttributes(attribute.Bool("circuit_breaker.success", true))
		if cb.config.OnSuccess != nil {
			cb.config.OnSuccess()
		}
	}
	
	return resp, err
}

// canProceed verifica si una request puede proceder
func (cb *CircuitBreaker) canProceed() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	now := time.Now()
	
	switch cb.state {
	case StateClosed:
		return true
		
	case StateOpen:
		// Verificar si es tiempo de pasar a half-open
		if now.Sub(cb.lastStateChange) >= cb.config.OpenTimeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests = 0
			return true
		}
		return false
		
	case StateHalfOpen:
		// Permitir requests limitadas
		if cb.halfOpenRequests < int64(cb.config.MaxHalfOpenRequests) {
			cb.halfOpenRequests++
			return true
		}
		return false
		
	default:
		return false
	}
}

// recordResult registra el resultado de una request
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	now := time.Now()
	cb.metrics.TotalRequests++
	
	// Actualizar estadísticas deslizantes
	cb.updateSlidingWindow(success)
	
	if success {
		cb.metrics.TotalSuccesses++
		cb.metrics.ConsecutiveSuccesses++
		cb.metrics.ConsecutiveFailures = 0
		cb.metrics.LastSuccessTime = now
		
		// Si estamos en half-open y tenemos suficientes éxitos, cerrar circuit
		if cb.state == StateHalfOpen {
			if cb.metrics.ConsecutiveSuccesses >= int64(cb.config.MaxHalfOpenRequests) {
				cb.setState(StateClosed)
			}
		}
	} else {
		cb.metrics.TotalFailures++
		cb.metrics.ConsecutiveFailures++
		cb.metrics.ConsecutiveSuccesses = 0
		cb.metrics.LastFailureTime = now
		
		// Verificar si debemos abrir el circuit
		if cb.config.ShouldTrip(cb.state, cb.metrics) {
			cb.setState(StateOpen)
		}
	}
}

// updateSlidingWindow actualiza la ventana deslizante de estadísticas
func (cb *CircuitBreaker) updateSlidingWindow(success bool) {
	cb.windowMutex.Lock()
	defer cb.windowMutex.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-cb.config.StatisticsWindow)
	
	// Remover registros antiguos
	for i, record := range cb.requestWindow {
		if record.timestamp.After(cutoff) {
			cb.requestWindow = cb.requestWindow[i:]
			break
		}
	}
	
	// Si todos los registros son antiguos
	if len(cb.requestWindow) > 0 && cb.requestWindow[0].timestamp.Before(cutoff) {
		cb.requestWindow = cb.requestWindow[:0]
	}
	
	// Añadir nuevo registro
	cb.requestWindow = append(cb.requestWindow, requestRecord{
		timestamp: now,
		success:   success,
	})
}

// setState cambia el estado del circuit breaker
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}
	
	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()
	
	// Reset estadísticas según el estado
	switch newState {
	case StateClosed:
		cb.metrics.ConsecutiveFailures = 0
		cb.halfOpenRequests = 0
	case StateOpen:
		cb.halfOpenRequests = 0
	case StateHalfOpen:
		cb.halfOpenRequests = 0
	}
	
	// Callback de cambio de estado
	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(oldState, newState)
	}
}

// GetState retorna el estado actual del circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetMetrics retorna las métricas actuales
func (cb *CircuitBreaker) GetMetrics() Metrics {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.metrics
}

// Reset reinicia el circuit breaker al estado cerrado
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.setState(StateClosed)
	cb.metrics = Metrics{}
	cb.halfOpenRequests = 0
	
	cb.windowMutex.Lock()
	cb.requestWindow = cb.requestWindow[:0]
	cb.windowMutex.Unlock()
}

// createCircuitOpenError crea un error cuando el circuit está abierto
func (cb *CircuitBreaker) createCircuitOpenError() error {
	return errors.NewDomainError(
		"CIRCUIT_BREAKER_OPEN",
		fmt.Sprintf("Circuit breaker '%s' is open", cb.name),
		errors.CategoryInfrastructure,
		errors.SeverityHigh,
	).WithMetadata("circuit_breaker", cb.name).WithMetadata("state", cb.state.String())
}

// Middleware de Gin para circuit breaker

// HTTPConfig configuración del middleware HTTP
type HTTPConfig struct {
	// Nombre del circuit breaker
	Name string
	
	// Configuración del circuit breaker
	CircuitConfig *Config
	
	// Función para generar key única por endpoint/método
	KeyGenerator func(*gin.Context) string
	
	// Función para determinar si aplicar circuit breaker
	Skipper func(*gin.Context) bool
	
	// Handler para cuando el circuit está abierto
	FallbackHandler gin.HandlerFunc
}

// CircuitBreakerManager gestiona múltiples circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mutex    sync.RWMutex
	config   *Config
}

// NewCircuitBreakerManager crea un nuevo manager
func NewCircuitBreakerManager(config *Config) *CircuitBreakerManager {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetOrCreateCircuitBreaker obtiene o crea un circuit breaker
func (cbm *CircuitBreakerManager) GetOrCreateCircuitBreaker(name string) *CircuitBreaker {
	cbm.mutex.RLock()
	if cb, exists := cbm.breakers[name]; exists {
		cbm.mutex.RUnlock()
		return cb
	}
	cbm.mutex.RUnlock()
	
	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()
	
	// Double-check después del lock exclusivo
	if cb, exists := cbm.breakers[name]; exists {
		return cb
	}
	
	cb := NewCircuitBreaker(name, cbm.config)
	cbm.breakers[name] = cb
	return cb
}

// HTTPMiddleware crea middleware de circuit breaker para Gin
func HTTPMiddleware(config *HTTPConfig) gin.HandlerFunc {
	if config == nil {
		config = &HTTPConfig{
			Name:          "default",
			CircuitConfig: DefaultConfig(),
		}
	}
	
	if config.KeyGenerator == nil {
		config.KeyGenerator = func(c *gin.Context) string {
			return fmt.Sprintf("%s_%s", c.Request.Method, c.FullPath())
		}
	}
	
	manager := NewCircuitBreakerManager(config.CircuitConfig)
	
	return gin.HandlerFunc(func(c *gin.Context) {
		// Skip si está configurado
		if config.Skipper != nil && config.Skipper(c) {
			c.Next()
			return
		}
		
		// Obtener o crear circuit breaker para esta ruta
		key := config.KeyGenerator(c)
		cb := manager.GetOrCreateCircuitBreaker(key)
		
		// Verificar si podemos proceder
		if !cb.canProceed() {
			if config.FallbackHandler != nil {
				config.FallbackHandler(c)
			} else {
				// Respuesta por defecto cuando circuit está abierto
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error":   "Service temporarily unavailable",
					"message": "Circuit breaker is open",
					"code":    "CIRCUIT_BREAKER_OPEN",
					"circuit": key,
				})
			}
			c.Abort()
			return
		}
		
		// Procesar request
		c.Next()
		
		// Registrar resultado basado en código de respuesta
		success := c.Writer.Status() < 500
		cb.recordResult(success)
	})
}