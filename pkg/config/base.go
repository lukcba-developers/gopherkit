package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/lukcba-developers/gopherkit/pkg/errors"
)

// BaseConfig holds common configuration for all services
type BaseConfig struct {
	Server        ServerConfig        `json:"server" validate:"required"`
	Database      DatabaseConfig      `json:"database" validate:"required"`
	Observability ObservabilityConfig `json:"observability"`
	Security      SecurityConfig      `json:"security"`
	Cache         CacheConfig         `json:"cache"`
	External      ExternalConfig      `json:"external"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port            string        `json:"port" validate:"required,min=4,max=5"`
	Environment     string        `json:"environment" validate:"required,oneof=development staging production"`
	MetricsPort     string        `json:"metrics_port" validate:"required,min=4,max=5"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	MaxRequestSize  int64         `json:"max_request_size"`
}

// DatabaseConfig holds database-related configuration
type DatabaseConfig struct {
	URL             string        `json:"url"`
	Host            string        `json:"host"`
	Port            string        `json:"port"`
	Database        string        `json:"database"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"`
}

// ObservabilityConfig holds observability-related configuration
type ObservabilityConfig struct {
	TracingEnabled bool   `json:"tracing_enabled"`
	MetricsEnabled bool   `json:"metrics_enabled"`
	LogLevel       string `json:"log_level" validate:"oneof=debug info warn error"`
	ServiceName    string `json:"service_name" validate:"required"`
	ServiceVersion string `json:"service_version"`
	JaegerEndpoint string `json:"jaeger_endpoint"`
	OTLPEndpoint   string `json:"otlp_endpoint"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	RateLimit      RateLimitConfig      `json:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
	CORS           CORSConfig           `json:"cors"`
	JWT            JWTConfig            `json:"jwt"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled       bool          `json:"enabled"`
	RequestsPerMinute int       `json:"requests_per_minute"`
	BurstSize     int           `json:"burst_size"`
	WindowSize    time.Duration `json:"window_size"`
	RedisEnabled  bool          `json:"redis_enabled"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled             bool          `json:"enabled"`
	MaxRequests         uint32        `json:"max_requests"`
	Interval            time.Duration `json:"interval"`
	Timeout             time.Duration `json:"timeout"`
	ReadyToTripRequests uint32        `json:"ready_to_trip_requests"`
	ReadyToTripRatio    float64       `json:"ready_to_trip_ratio"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	ExposedHeaders   []string `json:"exposed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age"`
}

// JWTConfig holds JWT-related configuration
type JWTConfig struct {
	Secret          string        `json:"secret" validate:"required,min=32"`
	RefreshSecret   string        `json:"refresh_secret"`
	AccessDuration  time.Duration `json:"access_duration"`
	RefreshDuration time.Duration `json:"refresh_duration"`
	Issuer          string        `json:"issuer"`
}

// CacheConfig holds cache-related configuration
type CacheConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	Prefix   string `json:"prefix"`
	TTL      time.Duration `json:"ttl"`
}

// ExternalConfig holds external service configuration
type ExternalConfig struct {
	AuthAPIURL        string        `json:"auth_api_url"`
	UserAPIURL        string        `json:"user_api_url"`
	NotificationURL   string        `json:"notification_url"`
	CalendarAPIURL    string        `json:"calendar_api_url"`
	ChampionshipAPIURL string       `json:"championship_api_url"`
	Timeout           time.Duration `json:"timeout"`
	RetryAttempts     int           `json:"retry_attempts"`
	RetryDelay        time.Duration `json:"retry_delay"`
}

// LoadBaseConfig loads base configuration from environment variables
func LoadBaseConfig(serviceName string) (*BaseConfig, error) {
	// Load .env file if it exists (development)
	_ = godotenv.Load()

	config := &BaseConfig{
		Server: ServerConfig{
			Port:            getEnvOrDefault("PORT", "8080"),
			Environment:     getEnvOrDefault("ENVIRONMENT", "development"),
			MetricsPort:     getEnvOrDefault("METRICS_PORT", "9090"),
			ReadTimeout:     getEnvDurationOrDefault("READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getEnvDurationOrDefault("WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:     getEnvDurationOrDefault("IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getEnvDurationOrDefault("SHUTDOWN_TIMEOUT", 30*time.Second),
			MaxRequestSize:  getEnvInt64OrDefault("MAX_REQUEST_SIZE", 10<<20), // 10MB
		},
		Database: DatabaseConfig{
			URL:             getEnvOrDefault("DATABASE_URL", ""),
			Host:            getEnvOrDefault("DB_HOST", "localhost"),
			Port:            getEnvOrDefault("DB_PORT", "5432"),
			Database:        getEnvOrDefault("DB_NAME", serviceName+"_db"),
			User:            getEnvOrDefault("DB_USER", "postgres"),
			Password:        getEnvOrDefault("DB_PASSWORD", "postgres"),
			SSLMode:         getEnvOrDefault("DB_SSL_MODE", "disable"),
			MaxOpenConns:    getEnvIntOrDefault("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvIntOrDefault("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDurationOrDefault("DB_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getEnvDurationOrDefault("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		},
		Observability: ObservabilityConfig{
			TracingEnabled: getEnvBoolOrDefault("TRACING_ENABLED", true),
			MetricsEnabled: getEnvBoolOrDefault("METRICS_ENABLED", true),
			LogLevel:       getEnvOrDefault("LOG_LEVEL", "info"),
			ServiceName:    serviceName,
			ServiceVersion: getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
			JaegerEndpoint: getEnvOrDefault("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
			OTLPEndpoint:   getEnvOrDefault("OTLP_ENDPOINT", "http://localhost:4318/v1/traces"),
		},
		Security: SecurityConfig{
			RateLimit: RateLimitConfig{
				Enabled:           getEnvBoolOrDefault("RATE_LIMIT_ENABLED", true),
				RequestsPerMinute: getEnvIntOrDefault("RATE_LIMIT_RPS", 100),
				BurstSize:         getEnvIntOrDefault("RATE_LIMIT_BURST", 10),
				WindowSize:        getEnvDurationOrDefault("RATE_LIMIT_WINDOW", time.Minute),
				RedisEnabled:      getEnvBoolOrDefault("RATE_LIMIT_REDIS_ENABLED", false),
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:             getEnvBoolOrDefault("CIRCUIT_BREAKER_ENABLED", true),
				MaxRequests:         uint32(getEnvIntOrDefault("CB_MAX_REQUESTS", 10)),
				Interval:            getEnvDurationOrDefault("CB_INTERVAL", 60*time.Second),
				Timeout:             getEnvDurationOrDefault("CB_TIMEOUT", 30*time.Second),
				ReadyToTripRequests: uint32(getEnvIntOrDefault("CB_READY_TO_TRIP_REQUESTS", 5)),
				ReadyToTripRatio:    getEnvFloatOrDefault("CB_READY_TO_TRIP_RATIO", 0.6),
			},
			CORS: CORSConfig{
				AllowedOrigins:   getEnvSliceOrDefault("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
				AllowedMethods:   getEnvSliceOrDefault("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
				AllowedHeaders:   getEnvSliceOrDefault("CORS_ALLOWED_HEADERS", []string{"*"}),
				ExposedHeaders:   getEnvSliceOrDefault("CORS_EXPOSED_HEADERS", []string{}),
				AllowCredentials: getEnvBoolOrDefault("CORS_ALLOW_CREDENTIALS", true),
				MaxAge:           getEnvIntOrDefault("CORS_MAX_AGE", 86400),
			},
			JWT: JWTConfig{
				Secret:          getEnvOrDefault("JWT_SECRET", ""),
				RefreshSecret:   getEnvOrDefault("REFRESH_SECRET", ""),
				AccessDuration:  getEnvDurationOrDefault("ACCESS_DURATION", 15*time.Minute),
				RefreshDuration: getEnvDurationOrDefault("REFRESH_DURATION", 24*time.Hour),
				Issuer:          getEnvOrDefault("JWT_ISSUER", serviceName),
			},
		},
		Cache: CacheConfig{
			Enabled:  getEnvBoolOrDefault("CACHE_ENABLED", true),
			Host:     getEnvOrDefault("REDIS_HOST", "localhost"),
			Port:     getEnvOrDefault("REDIS_PORT", "6379"),
			Password: getEnvOrDefault("REDIS_PASSWORD", ""),
			DB:       getEnvIntOrDefault("REDIS_DB", 0),
			Prefix:   getEnvOrDefault("CACHE_PREFIX", serviceName+":"),
			TTL:      getEnvDurationOrDefault("CACHE_TTL", 5*time.Minute),
		},
		External: ExternalConfig{
			AuthAPIURL:         getEnvOrDefault("AUTH_API_URL", "http://localhost:8083"),
			UserAPIURL:         getEnvOrDefault("USER_API_URL", "http://localhost:8081"),
			NotificationURL:    getEnvOrDefault("NOTIFICATION_API_URL", "http://localhost:8090"),
			CalendarAPIURL:     getEnvOrDefault("CALENDAR_API_URL", "http://localhost:8087"),
			ChampionshipAPIURL: getEnvOrDefault("CHAMPIONSHIP_API_URL", "http://localhost:8084"),
			Timeout:            getEnvDurationOrDefault("EXTERNAL_TIMEOUT", 30*time.Second),
			RetryAttempts:      getEnvIntOrDefault("EXTERNAL_RETRY_ATTEMPTS", 3),
			RetryDelay:         getEnvDurationOrDefault("EXTERNAL_RETRY_DELAY", time.Second),
		},
	}

	if err := ValidateConfig(config); err != nil {
		return nil, errors.NewValidationError("configuration validation failed", err)
	}

	return config, nil
}

// ValidateConfig validates the configuration using struct tags
func ValidateConfig(config *BaseConfig) error {
	validator := validator.New()
	if err := validator.Struct(config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Additional custom validations
	if config.Server.Environment == "production" {
		if config.Security.JWT.Secret == "" {
			return fmt.Errorf("JWT_SECRET is required in production")
		}
		if len(config.Security.JWT.Secret) < 32 {
			return fmt.Errorf("JWT_SECRET must be at least 32 characters in production")
		}
		if config.Database.Password == "postgres" {
			return fmt.Errorf("default database password not allowed in production")
		}
	}

	return nil
}

// IsDevelopment returns true if the environment is development
func (c *BaseConfig) IsDevelopment() bool {
	return c.Server.Environment == "development"
}

// IsProduction returns true if the environment is production
func (c *BaseConfig) IsProduction() bool {
	return c.Server.Environment == "production"
}

// IsStaging returns true if the environment is staging
func (c *BaseConfig) IsStaging() bool {
	return c.Server.Environment == "staging"
}

// GetDSN returns the database connection string
func (c *DatabaseConfig) GetDSN() string {
	if c.URL != "" {
		return c.URL
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode)
}

// Utility functions for environment variable parsing
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvInt64OrDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

func getEnvFloatOrDefault(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSliceOrDefault(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}