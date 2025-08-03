package circuitbreaker

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// CircuitBreaker representa un circuit breaker thread-safe
type CircuitBreaker struct {
	name   string
	config *Config
	mutex  sync.RWMutex
	
	state      State
	generation uint64
	counts     Counts
	expiry     time.Time
	
	logger *logrus.Entry
}

// New crea un nuevo circuit breaker
func New(config *Config) (*CircuitBreaker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	
	// Configurar función ReadyToTrip por defecto si no está definida
	if config.ReadyToTrip == nil {
		config.ReadyToTrip = func(counts Counts) bool {
			// Primero verificar si tenemos suficientes requests
			if counts.Requests < config.MinimumRequestThreshold {
				return false
			}
			
			// Si hay threshold de fallos definido, usarlo
			if config.FailureThreshold > 0 {
				return counts.ConsecutiveFailures >= config.FailureThreshold
			}
			
			// Usar failure ratio
			return counts.FailureRatio() >= config.FailureRatio
		}
	}
	
	cb := &CircuitBreaker{
		name:   config.Name,
		config: config,
		state:  StateClosed,
		logger: logrus.WithFields(logrus.Fields{
			"component": "circuit_breaker",
			"name":      config.Name,
		}),
	}
	
	cb.logger.WithFields(logrus.Fields{
		"max_requests":              config.MaxRequests,
		"interval":                  config.Interval,
		"timeout":                   config.Timeout,
		"failure_threshold":         config.FailureThreshold,
		"failure_ratio":             config.FailureRatio,
		"minimum_request_threshold": config.MinimumRequestThreshold,
	}).Debug("Circuit breaker created")
	
	return cb, nil
}

// Execute ejecuta una función con protección del circuit breaker
func (cb *CircuitBreaker) Execute(fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}
	
	defer func() {
		e := recover()
		if e != nil {
			cb.afterRequest(generation, false, nil)
			panic(e)
		}
	}()
	
	result := fn()
	cb.afterRequest(generation, cb.config.IsSuccessful(result), result)
	return result
}

// ExecuteWithContext ejecuta una función con contexto
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}
	
	defer func() {
		e := recover()
		if e != nil {
			cb.afterRequest(generation, false, nil)
			panic(e)
		}
	}()
	
	result := fn(ctx)
	cb.afterRequest(generation, cb.config.IsSuccessful(result), result)
	return result
}

// Call es un alias para Execute para compatibilidad
func (cb *CircuitBreaker) Call(fn func() error) error {
	return cb.Execute(fn)
}

// beforeRequest verifica si la request puede proceder
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	now := time.Now()
	state, generation := cb.currentState(now)
	
	if state == StateOpen {
		cb.logger.WithFields(logrus.Fields{
			"state":      state.String(),
			"expiry":     cb.expiry,
			"timeout":    cb.config.Timeout,
			"generation": generation,
		}).Debug("Request rejected - circuit breaker is open")
		
		return generation, NewOpenStateError(cb.name, cb.config.Timeout, cb.counts)
	}
	
	if state == StateHalfOpen && cb.counts.Requests >= cb.config.MaxRequests {
		cb.logger.WithFields(logrus.Fields{
			"state":        state.String(),
			"requests":     cb.counts.Requests,
			"max_requests": cb.config.MaxRequests,
			"generation":   generation,
		}).Debug("Request rejected - too many requests in half-open state")
		
		return generation, NewTooManyRequestsError(cb.name, cb.config.MaxRequests, cb.counts)
	}
	
	cb.counts.onRequest()
	cb.logger.WithFields(logrus.Fields{
		"state":       state.String(),
		"requests":    cb.counts.Requests,
		"generation":  generation,
	}).Debug("Request allowed")
	
	return generation, nil
}

// afterRequest procesa el resultado de la request
func (cb *CircuitBreaker) afterRequest(before uint64, success bool, err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	now := time.Now()
	state, generation := cb.currentState(now)
	
	if generation != before {
		// La generación cambió durante la request, ignorar el resultado
		cb.logger.WithFields(logrus.Fields{
			"before_generation": before,
			"current_generation": generation,
			"success": success,
		}).Debug("Request result ignored - generation changed")
		return
	}
	
	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now, err)
	}
}

// onSuccess maneja un resultado exitoso
func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	cb.counts.onSuccess()
	
	if state == StateHalfOpen && cb.counts.ConsecutiveSuccesses >= cb.config.MaxRequests {
		cb.setState(StateClosed, now)
	}
	
	cb.logger.WithFields(logrus.Fields{
		"state":                 state.String(),
		"consecutive_successes": cb.counts.ConsecutiveSuccesses,
		"total_successes":       cb.counts.TotalSuccesses,
		"requests":              cb.counts.Requests,
	}).Debug("Request succeeded")
}

// onFailure maneja un resultado fallido
func (cb *CircuitBreaker) onFailure(state State, now time.Time, err error) {
	cb.counts.onFailure()
	
	if cb.config.ReadyToTrip(cb.counts) {
		cb.setState(StateOpen, now)
		
		cb.logger.WithFields(logrus.Fields{
			"previous_state":         state.String(),
			"new_state":             StateOpen.String(),
			"consecutive_failures":   cb.counts.ConsecutiveFailures,
			"total_failures":        cb.counts.TotalFailures,
			"failure_ratio":         cb.counts.FailureRatio(),
			"requests":              cb.counts.Requests,
			"error":                 err,
		}).Warn("Circuit breaker opened due to failures")
	} else {
		cb.logger.WithFields(logrus.Fields{
			"state":                 state.String(),
			"consecutive_failures":  cb.counts.ConsecutiveFailures,
			"total_failures":        cb.counts.TotalFailures,
			"failure_ratio":         cb.counts.FailureRatio(),
			"requests":              cb.counts.Requests,
			"error":                 err,
		}).Debug("Request failed")
	}
}

// currentState obtiene el estado actual y actualiza si es necesario
func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

// setState cambia el estado del circuit breaker
func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}
	
	prev := cb.state
	cb.state = state
	cb.toNewGeneration(now)
	
	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.name, prev, state)
	}
	
	cb.logger.WithFields(logrus.Fields{
		"previous_state": prev.String(),
		"new_state":      state.String(),
		"generation":     cb.generation,
	}).Info("Circuit breaker state changed")
}

// toNewGeneration inicia una nueva generación
func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts.clear()
	
	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.config.Interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.config.Interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.config.Timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

// State retorna el estado actual del circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	
	state, _ := cb.currentState(time.Now())
	return state
}

// Counts retorna las métricas actuales
func (cb *CircuitBreaker) Counts() Counts {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	
	return cb.counts
}

// Name retorna el nombre del circuit breaker
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Config retorna la configuración (solo lectura)
func (cb *CircuitBreaker) Config() Config {
	return *cb.config
}

// Reset reinicia el circuit breaker al estado cerrado
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.logger.Info("Circuit breaker manually reset")
	cb.setState(StateClosed, time.Now())
}

// ForceOpen fuerza el circuit breaker al estado abierto
func (cb *CircuitBreaker) ForceOpen() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.logger.Warn("Circuit breaker manually forced open")
	cb.setState(StateOpen, time.Now())
}

// Métodos helper para Counts

func (c *Counts) onRequest() {
	c.Requests++
	c.LastRequestTime = time.Now()
}

func (c *Counts) onSuccess() {
	c.TotalSuccesses++
	c.ConsecutiveSuccesses++
	c.ConsecutiveFailures = 0
}

func (c *Counts) onFailure() {
	c.TotalFailures++
	c.ConsecutiveFailures++
	c.ConsecutiveSuccesses = 0
}

func (c *Counts) clear() {
	c.Requests = 0
	c.TotalSuccesses = 0
	c.TotalFailures = 0
	c.ConsecutiveSuccesses = 0
	c.ConsecutiveFailures = 0
	c.LastRequestTime = time.Time{}
}