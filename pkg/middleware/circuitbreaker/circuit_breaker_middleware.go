package circuitbreaker

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CircuitBreakerState representa el estado del circuit breaker
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig configuración del circuit breaker
type CircuitBreakerConfig struct {
	// Failure threshold - número de fallos consecutivos para abrir el circuito
	FailureThreshold int
	
	// Recovery timeout - tiempo en estado abierto antes de intentar recovery
	RecoveryTimeout time.Duration
	
	// Recovery threshold - número de requests exitosas para cerrar el circuito
	RecoveryThreshold int
	
	// Request timeout - timeout individual para requests
	RequestTimeout time.Duration
	
	// Status codes que se consideran fallos
	FailureStatusCodes []int
	
	// Paths que deben ser protegidos por el circuit breaker
	ProtectedPaths []string
	
	// Métodos HTTP que deben ser protegidos
	ProtectedMethods []string
	
	// Callback para eventos del circuit breaker
	OnStateChange func(from, to CircuitBreakerState)
}

// DefaultCircuitBreakerConfig retorna configuración por defecto
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:   5,
		RecoveryTimeout:    30 * time.Second,
		RecoveryThreshold:  3,
		RequestTimeout:     10 * time.Second,
		FailureStatusCodes: []int{500, 502, 503, 504},
		ProtectedPaths:     []string{}, // Empty means all paths
		ProtectedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
		OnStateChange:      nil,
	}
}

// CircuitBreaker implementa el patrón circuit breaker
type CircuitBreaker struct {
	config           *CircuitBreakerConfig
	state            CircuitBreakerState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	logger           *logrus.Entry
	mutex            sync.RWMutex
}

// NewCircuitBreaker crea un nuevo circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig, logger *logrus.Logger) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
		logger: logger.WithField("component", "circuit_breaker"),
	}
}

// CircuitBreakerMiddleware middleware que implementa circuit breaker
func CircuitBreakerMiddleware(cb *CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this path/method should be protected
		if !cb.shouldProtect(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		// Check circuit breaker state
		if !cb.allowRequest() {
			cb.logger.WithFields(logrus.Fields{
				"path":         c.Request.URL.Path,
				"method":       c.Request.Method,
				"state":        cb.getState().String(),
				"failure_count": cb.getFailureCount(),
			}).Warn("Circuit breaker is open, rejecting request")

			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "service_unavailable",
				"message": "Service temporarily unavailable due to circuit breaker",
				"code":    "CIRCUIT_BREAKER_OPEN",
				"retry_after": cb.config.RecoveryTimeout.Seconds(),
			})
			c.Abort()
			return
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), cb.config.RequestTimeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		// Record start time
		start := time.Now()

		// Execute request
		c.Next()

		// Record result
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		if cb.isFailureStatusCode(statusCode) || ctx.Err() != nil {
			cb.recordFailure()
			cb.logger.WithFields(logrus.Fields{
				"path":        c.Request.URL.Path,
				"method":      c.Request.Method,
				"status_code": statusCode,
				"duration":    duration.String(),
				"error":       ctx.Err(),
				"state":       cb.getState().String(),
			}).Warn("Request failed, recorded by circuit breaker")
		} else {
			cb.recordSuccess()
			cb.logger.WithFields(logrus.Fields{
				"path":        c.Request.URL.Path,
				"method":      c.Request.Method,
				"status_code": statusCode,
				"duration":    duration.String(),
				"state":       cb.getState().String(),
			}).Debug("Request succeeded, recorded by circuit breaker")
		}
	}
}

// allowRequest verifica si se debe permitir la request
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
			// Transition to half-open
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
				cb.setState(StateHalfOpen)
				cb.successCount = 0
			}
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return cb.state == StateHalfOpen
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// recordSuccess registra una request exitosa
func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount = 0

	if cb.state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.config.RecoveryThreshold {
			cb.setState(StateClosed)
			cb.successCount = 0
		}
	}
}

// recordFailure registra una request fallida
func (cb *CircuitBreaker) recordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.state == StateClosed && cb.failureCount >= cb.config.FailureThreshold {
		cb.setState(StateOpen)
	} else if cb.state == StateHalfOpen {
		cb.setState(StateOpen)
		cb.successCount = 0
	}
}

// setState cambia el estado del circuit breaker
func (cb *CircuitBreaker) setState(newState CircuitBreakerState) {
	oldState := cb.state
	cb.state = newState

	cb.logger.WithFields(logrus.Fields{
		"from_state":     oldState.String(),
		"to_state":       newState.String(),
		"failure_count":  cb.failureCount,
		"success_count":  cb.successCount,
	}).Info("Circuit breaker state changed")

	// Call callback if configured
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(oldState, newState)
	}
}

// getState retorna el estado actual de forma thread-safe
func (cb *CircuitBreaker) getState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// getFailureCount retorna el número de fallos actual
func (cb *CircuitBreaker) getFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failureCount
}

// shouldProtect verifica si el path/método debe ser protegido
func (cb *CircuitBreaker) shouldProtect(method, path string) bool {
	// Check methods
	if len(cb.config.ProtectedMethods) > 0 {
		methodProtected := false
		for _, protectedMethod := range cb.config.ProtectedMethods {
			if method == protectedMethod {
				methodProtected = true
				break
			}
		}
		if !methodProtected {
			return false
		}
	}

	// Check paths - if empty, protect all paths
	if len(cb.config.ProtectedPaths) == 0 {
		return true
	}

	for _, protectedPath := range cb.config.ProtectedPaths {
		if path == protectedPath {
			return true
		}
	}

	return false
}

// isFailureStatusCode verifica si el código de estado es un fallo
func (cb *CircuitBreaker) isFailureStatusCode(statusCode int) bool {
	for _, failureCode := range cb.config.FailureStatusCodes {
		if statusCode == failureCode {
			return true
		}
	}
	return false
}

// GetStats retorna estadísticas del circuit breaker
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return map[string]interface{}{
		"state":             cb.state.String(),
		"failure_count":     cb.failureCount,
		"success_count":     cb.successCount,
		"last_failure_time": cb.lastFailureTime.Format(time.RFC3339),
		"config": map[string]interface{}{
			"failure_threshold":    cb.config.FailureThreshold,
			"recovery_timeout":     cb.config.RecoveryTimeout.String(),
			"recovery_threshold":   cb.config.RecoveryThreshold,
			"request_timeout":      cb.config.RequestTimeout.String(),
			"failure_status_codes": cb.config.FailureStatusCodes,
		},
	}
}

// Reset resetea el circuit breaker al estado inicial
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	oldState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastFailureTime = time.Time{}

	cb.logger.WithFields(logrus.Fields{
		"from_state": oldState.String(),
		"to_state":   StateClosed.String(),
	}).Info("Circuit breaker manually reset")
}

// ForceOpen fuerza el circuit breaker al estado abierto
func (cb *CircuitBreaker) ForceOpen() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	oldState := cb.state
	cb.setState(StateOpen)
	cb.lastFailureTime = time.Now()

	cb.logger.WithFields(logrus.Fields{
		"from_state": oldState.String(),
		"to_state":   StateOpen.String(),
	}).Warn("Circuit breaker manually forced open")
}

// CircuitBreakerManager maneja múltiples circuit breakers
type CircuitBreakerManager struct {
	circuitBreakers map[string]*CircuitBreaker
	mutex           sync.RWMutex
	logger          *logrus.Logger
}

// NewCircuitBreakerManager crea un nuevo manager
func NewCircuitBreakerManager(logger *logrus.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		circuitBreakers: make(map[string]*CircuitBreaker),
		logger:          logger,
	}
}

// AddCircuitBreaker añade un circuit breaker
func (cbm *CircuitBreakerManager) AddCircuitBreaker(name string, config *CircuitBreakerConfig) *CircuitBreaker {
	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()

	cb := NewCircuitBreaker(config, cbm.logger)
	cbm.circuitBreakers[name] = cb
	return cb
}

// GetCircuitBreaker obtiene un circuit breaker por nombre
func (cbm *CircuitBreakerManager) GetCircuitBreaker(name string) *CircuitBreaker {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()
	return cbm.circuitBreakers[name]
}

// GetAllStats obtiene estadísticas de todos los circuit breakers
func (cbm *CircuitBreakerManager) GetAllStats() map[string]interface{} {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	stats := make(map[string]interface{})
	for name, cb := range cbm.circuitBreakers {
		stats[name] = cb.GetStats()
	}
	return stats
}

// HealthCheck verifica el estado de todos los circuit breakers
func (cbm *CircuitBreakerManager) HealthCheck() map[string]string {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	health := make(map[string]string)
	for name, cb := range cbm.circuitBreakers {
		health[name] = cb.getState().String()
	}
	return health
}

// MultiCircuitBreakerMiddleware middleware que usa el manager
func MultiCircuitBreakerMiddleware(cbm *CircuitBreakerManager, circuitBreakerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cb := cbm.GetCircuitBreaker(circuitBreakerName)
		if cb == nil {
			// If circuit breaker doesn't exist, create a default one
			cb = cbm.AddCircuitBreaker(circuitBreakerName, DefaultCircuitBreakerConfig())
		}

		// Use the circuit breaker middleware
		CircuitBreakerMiddleware(cb)(c)
	}
}

// CircuitBreakerHealthHandler handler para verificar el estado del circuit breaker
func CircuitBreakerHealthHandler(cbm *CircuitBreakerManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		health := cbm.HealthCheck()
		stats := cbm.GetAllStats()

		response := gin.H{
			"circuit_breakers": health,
			"stats":           stats,
			"timestamp":       time.Now().Format(time.RFC3339),
		}

		// Determine overall health
		allHealthy := true
		for _, state := range health {
			if state == StateOpen.String() {
				allHealthy = false
				break
			}
		}

		if allHealthy {
			c.JSON(http.StatusOK, response)
		} else {
			c.JSON(http.StatusServiceUnavailable, response)
		}
	}
}