package httpclient

import (
	"net/http"
	"time"
)

// Config define la configuración de un HTTP client resiliente
type Config struct {
	// Name es el nombre del cliente para logs y métricas
	Name string `json:"name"`
	
	// BaseURL es la URL base para todas las requests
	BaseURL string `json:"base_url"`
	
	// Timeout es el timeout total para requests
	Timeout time.Duration `json:"timeout"`
	
	// RetryConfig configura el comportamiento de retry
	RetryConfig RetryConfig `json:"retry_config"`
	
	// CircuitBreakerEnabled habilita circuit breaker
	CircuitBreakerEnabled bool `json:"circuit_breaker_enabled"`
	
	// CircuitBreakerConfig configuración del circuit breaker
	CircuitBreakerConfig CircuitBreakerConfig `json:"circuit_breaker_config"`
	
	// Headers por defecto para todas las requests
	DefaultHeaders map[string]string `json:"default_headers"`
	
	// UserAgent personalizado
	UserAgent string `json:"user_agent"`
	
	// TLS Configuration
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
	
	// Connection pooling
	MaxIdleConns        int           `json:"max_idle_conns"`
	MaxIdleConnsPerHost int           `json:"max_idle_conns_per_host"`
	IdleConnTimeout     time.Duration `json:"idle_conn_timeout"`
	
	// Timeouts específicos
	DialTimeout           time.Duration `json:"dial_timeout"`
	KeepAlive            time.Duration `json:"keep_alive"`
	TLSHandshakeTimeout  time.Duration `json:"tls_handshake_timeout"`
	ResponseHeaderTimeout time.Duration `json:"response_header_timeout"`
	ExpectContinueTimeout time.Duration `json:"expect_continue_timeout"`
	
	// Logging
	LogRequests  bool `json:"log_requests"`
	LogResponses bool `json:"log_responses"`
	LogBody      bool `json:"log_body"`
	
	// Metrics
	EnableMetrics bool `json:"enable_metrics"`
	
	// Authentication
	AuthConfig AuthConfig `json:"auth_config"`
}

// RetryConfig configura el comportamiento de retry
type RetryConfig struct {
	Enabled     bool          `json:"enabled"`
	MaxRetries  int           `json:"max_retries"`
	InitialWait time.Duration `json:"initial_wait"`
	MaxWait     time.Duration `json:"max_wait"`
	Multiplier  float64       `json:"multiplier"`
	Jitter      bool          `json:"jitter"`
	
	// RetryableStatusCodes códigos de estado que permiten retry
	RetryableStatusCodes []int `json:"retryable_status_codes"`
	
	// RetryableErrors errores que permiten retry
	RetryableErrors []string `json:"retryable_errors"`
	
	// OnRetry callback ejecutado en cada retry
	OnRetry func(attempt int, err error) `json:"-"`
}

// CircuitBreakerConfig configura el circuit breaker
type CircuitBreakerConfig struct {
	MaxRequests             uint32        `json:"max_requests"`
	Interval                time.Duration `json:"interval"`
	Timeout                 time.Duration `json:"timeout"`
	FailureThreshold        uint32        `json:"failure_threshold"`
	FailureRatio            float64       `json:"failure_ratio"`
	MinimumRequestThreshold uint32        `json:"minimum_request_threshold"`
}

// AuthConfig configura la autenticación
type AuthConfig struct {
	Type     AuthType `json:"type"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Token    string   `json:"token,omitempty"`
	APIKey   string   `json:"api_key,omitempty"`
	Header   string   `json:"header,omitempty"` // Para API key custom header
}

// AuthType define el tipo de autenticación
type AuthType string

const (
	AuthNone       AuthType = "none"
	AuthBasic      AuthType = "basic"
	AuthBearer     AuthType = "bearer"
	AuthAPIKey     AuthType = "api_key"
	AuthCustom     AuthType = "custom"
)

// DefaultConfig retorna configuración por defecto
func DefaultConfig(name, baseURL string) *Config {
	return &Config{
		Name:    name,
		BaseURL: baseURL,
		Timeout: 30 * time.Second,
		RetryConfig: RetryConfig{
			Enabled:     true,
			MaxRetries:  3,
			InitialWait: 100 * time.Millisecond,
			MaxWait:     5 * time.Second,
			Multiplier:  2.0,
			Jitter:      true,
			RetryableStatusCodes: []int{
				http.StatusRequestTimeout,      // 408
				http.StatusTooManyRequests,     // 429
				http.StatusInternalServerError, // 500
				http.StatusBadGateway,          // 502
				http.StatusServiceUnavailable,  // 503
				http.StatusGatewayTimeout,      // 504
			},
			RetryableErrors: []string{
				"connection refused",
				"connection reset",
				"timeout",
				"temporary failure",
				"network unreachable",
			},
		},
		CircuitBreakerEnabled: true,
		CircuitBreakerConfig: CircuitBreakerConfig{
			MaxRequests:             3,
			Interval:                30 * time.Second,
			Timeout:                 10 * time.Second,
			FailureThreshold:        5,
			FailureRatio:            0.6,
			MinimumRequestThreshold: 3,
		},
		DefaultHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		UserAgent:               "gopherkit-httpclient/1.0",
		InsecureSkipVerify:      false,
		MaxIdleConns:            100,
		MaxIdleConnsPerHost:     10,
		IdleConnTimeout:         90 * time.Second,
		DialTimeout:             10 * time.Second,
		KeepAlive:              30 * time.Second,
		TLSHandshakeTimeout:     10 * time.Second,
		ResponseHeaderTimeout:   10 * time.Second,
		ExpectContinueTimeout:   1 * time.Second,
		LogRequests:             false,
		LogResponses:            false,
		LogBody:                 false,
		EnableMetrics:           true,
		AuthConfig: AuthConfig{
			Type: AuthNone,
		},
	}
}

// FastConfig retorna configuración optimizada para velocidad
func FastConfig(name, baseURL string) *Config {
	config := DefaultConfig(name, baseURL)
	config.Timeout = 5 * time.Second
	config.RetryConfig.MaxRetries = 1
	config.RetryConfig.InitialWait = 50 * time.Millisecond
	config.RetryConfig.MaxWait = 1 * time.Second
	config.CircuitBreakerConfig.Timeout = 5 * time.Second
	config.DialTimeout = 2 * time.Second
	config.ResponseHeaderTimeout = 3 * time.Second
	return config
}

// ResilientConfig retorna configuración optimizada para resiliencia
func ResilientConfig(name, baseURL string) *Config {
	config := DefaultConfig(name, baseURL)
	config.Timeout = 60 * time.Second
	config.RetryConfig.MaxRetries = 5
	config.RetryConfig.InitialWait = 200 * time.Millisecond
	config.RetryConfig.MaxWait = 10 * time.Second
	config.CircuitBreakerConfig.Timeout = 30 * time.Second
	config.CircuitBreakerConfig.FailureThreshold = 3
	config.CircuitBreakerConfig.MinimumRequestThreshold = 5
	return config
}

// InternalServiceConfig retorna configuración para servicios internos
func InternalServiceConfig(name, baseURL string) *Config {
	config := DefaultConfig(name, baseURL)
	config.Timeout = 15 * time.Second
	config.RetryConfig.MaxRetries = 2
	config.CircuitBreakerConfig.FailureThreshold = 3
	config.CircuitBreakerConfig.Timeout = 15 * time.Second
	config.LogRequests = true // Log interno más detallado
	return config
}

// ExternalServiceConfig retorna configuración para servicios externos
func ExternalServiceConfig(name, baseURL string) *Config {
	config := ResilientConfig(name, baseURL)
	config.CircuitBreakerConfig.FailureRatio = 0.4 // Más tolerante con servicios externos
	config.CircuitBreakerConfig.MinimumRequestThreshold = 10
	return config
}

// WithAuth configura autenticación
func (c *Config) WithAuth(authType AuthType, credentials map[string]string) *Config {
	c.AuthConfig.Type = authType
	
	switch authType {
	case AuthBasic:
		c.AuthConfig.Username = credentials["username"]
		c.AuthConfig.Password = credentials["password"]
	case AuthBearer:
		c.AuthConfig.Token = credentials["token"]
	case AuthAPIKey:
		c.AuthConfig.APIKey = credentials["api_key"]
		c.AuthConfig.Header = credentials["header"]
		if c.AuthConfig.Header == "" {
			c.AuthConfig.Header = "X-API-Key"
		}
	}
	
	return c
}

// WithHeaders añade headers por defecto
func (c *Config) WithHeaders(headers map[string]string) *Config {
	if c.DefaultHeaders == nil {
		c.DefaultHeaders = make(map[string]string)
	}
	
	for k, v := range headers {
		c.DefaultHeaders[k] = v
	}
	
	return c
}

// WithTimeout configura timeout
func (c *Config) WithTimeout(timeout time.Duration) *Config {
	c.Timeout = timeout
	return c
}

// WithRetries configura número de retries
func (c *Config) WithRetries(maxRetries int) *Config {
	c.RetryConfig.MaxRetries = maxRetries
	return c
}

// WithCircuitBreaker habilita/deshabilita circuit breaker
func (c *Config) WithCircuitBreaker(enabled bool) *Config {
	c.CircuitBreakerEnabled = enabled
	return c
}

// WithLogging configura logging
func (c *Config) WithLogging(requests, responses, body bool) *Config {
	c.LogRequests = requests
	c.LogResponses = responses
	c.LogBody = body
	return c
}

// Validate valida la configuración
func (c *Config) Validate() error {
	if c.Name == "" {
		return ErrInvalidName
	}
	
	if c.BaseURL == "" {
		return ErrInvalidBaseURL
	}
	
	if c.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	
	if c.RetryConfig.MaxRetries < 0 {
		return ErrInvalidMaxRetries
	}
	
	if c.RetryConfig.Multiplier <= 0 {
		return ErrInvalidMultiplier
	}
	
	if c.CircuitBreakerConfig.FailureRatio < 0 || c.CircuitBreakerConfig.FailureRatio > 1 {
		return ErrInvalidFailureRatio
	}
	
	return nil
}