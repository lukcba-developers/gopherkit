package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBaseConfig(t *testing.T) {
	// Clean environment
	envVars := []string{
		"PORT", "ENVIRONMENT", "DATABASE_URL", "JWT_SECRET",
		"REDIS_HOST", "LOG_LEVEL", "SERVICE_VERSION",
	}
	
	originalEnv := make(map[string]string)
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	
	// Restore environment after test
	defer func() {
		for key, value := range originalEnv {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	tests := []struct {
		name        string
		envVars     map[string]string
		serviceName string
		wantError   bool
		validate    func(t *testing.T, cfg *BaseConfig)
	}{
		{
			name: "default configuration",
			envVars: map[string]string{
				"JWT_SECRET": "test-secret-key-with-more-than-32-characters",
			},
			serviceName: "test-service",
			wantError:   false,
			validate: func(t *testing.T, cfg *BaseConfig) {
				assert.Equal(t, "8080", cfg.Server.Port)
				assert.Equal(t, "development", cfg.Server.Environment)
				assert.Equal(t, "test-service", cfg.Observability.ServiceName)
				assert.True(t, cfg.Cache.Enabled)
				assert.Equal(t, "localhost", cfg.Cache.Host)
			},
		},
		{
			name: "production configuration",
			envVars: map[string]string{
				"ENVIRONMENT":    "production",
				"PORT":           "9000",
				"DATABASE_URL":   "postgresql://prod_user:prod_pass@prod_host:5432/prod_db",
				"JWT_SECRET":     "super-secure-production-secret-key-that-is-long-enough",
				"DB_PASSWORD":    "production-password",
				"LOG_LEVEL":      "warn",
			},
			serviceName: "prod-service",
			wantError:   false,
			validate: func(t *testing.T, cfg *BaseConfig) {
				assert.Equal(t, "9000", cfg.Server.Port)
				assert.Equal(t, "production", cfg.Server.Environment)
				assert.True(t, cfg.IsProduction())
				assert.False(t, cfg.IsDevelopment())
				assert.Equal(t, "warn", cfg.Observability.LogLevel)
			},
		},
		{
			name: "invalid JWT secret - too short",
			envVars: map[string]string{
				"JWT_SECRET": "short",
			},
			serviceName: "test-service",
			wantError:   true,
		},
		{
			name: "invalid JWT secret - default in production",
			envVars: map[string]string{
				"ENVIRONMENT": "production",
				"JWT_SECRET":  "",
			},
			serviceName: "test-service",
			wantError:   true,
		},
		{
			name: "cache disabled",
			envVars: map[string]string{
				"CACHE_ENABLED": "false",
				"JWT_SECRET":    "test-secret-key-with-more-than-32-characters",
			},
			serviceName: "test-service",
			wantError:   false,
			validate: func(t *testing.T, cfg *BaseConfig) {
				assert.False(t, cfg.Cache.Enabled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg, err := LoadBaseConfig(tt.serviceName)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}

			// Clean up environment variables for next test
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestDatabaseConfigGetDSN(t *testing.T) {
	tests := []struct {
		name   string
		config DatabaseConfig
		want   string
	}{
		{
			name: "with URL",
			config: DatabaseConfig{
				URL: "postgresql://user:pass@host:5432/db",
			},
			want: "postgresql://user:pass@host:5432/db",
		},
		{
			name: "without URL - build from components",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
				Database: "testdb",
				SSLMode:  "disable",
			},
			want: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetDSN()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnvHelperFunctions(t *testing.T) {
	t.Run("getEnvOrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_VAR")
		assert.Equal(t, "default", getEnvOrDefault("TEST_VAR", "default"))

		os.Setenv("TEST_VAR", "value")
		assert.Equal(t, "value", getEnvOrDefault("TEST_VAR", "default"))
		os.Unsetenv("TEST_VAR")
	})

	t.Run("getEnvIntOrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_INT")
		assert.Equal(t, 42, getEnvIntOrDefault("TEST_INT", 42))

		os.Setenv("TEST_INT", "100")
		assert.Equal(t, 100, getEnvIntOrDefault("TEST_INT", 42))

		os.Setenv("TEST_INT", "invalid")
		assert.Equal(t, 42, getEnvIntOrDefault("TEST_INT", 42))
		os.Unsetenv("TEST_INT")
	})

	t.Run("getEnvBoolOrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_BOOL")
		assert.True(t, getEnvBoolOrDefault("TEST_BOOL", true))

		os.Setenv("TEST_BOOL", "true")
		assert.True(t, getEnvBoolOrDefault("TEST_BOOL", false))

		os.Setenv("TEST_BOOL", "1")
		assert.True(t, getEnvBoolOrDefault("TEST_BOOL", false))

		os.Setenv("TEST_BOOL", "false")
		assert.False(t, getEnvBoolOrDefault("TEST_BOOL", true))

		os.Setenv("TEST_BOOL", "0")
		assert.False(t, getEnvBoolOrDefault("TEST_BOOL", true))
		os.Unsetenv("TEST_BOOL")
	})

	t.Run("getEnvDurationOrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_DURATION")
		assert.Equal(t, 5*time.Second, getEnvDurationOrDefault("TEST_DURATION", 5*time.Second))

		os.Setenv("TEST_DURATION", "10s")
		assert.Equal(t, 10*time.Second, getEnvDurationOrDefault("TEST_DURATION", 5*time.Second))

		os.Setenv("TEST_DURATION", "invalid")
		assert.Equal(t, 5*time.Second, getEnvDurationOrDefault("TEST_DURATION", 5*time.Second))
		os.Unsetenv("TEST_DURATION")
	})

	t.Run("getEnvSliceOrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_SLICE")
		assert.Equal(t, []string{"a", "b"}, getEnvSliceOrDefault("TEST_SLICE", []string{"a", "b"}))

		os.Setenv("TEST_SLICE", "x,y,z")
		assert.Equal(t, []string{"x", "y", "z"}, getEnvSliceOrDefault("TEST_SLICE", []string{"a", "b"}))
		os.Unsetenv("TEST_SLICE")
	})
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *BaseConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid development config",
			config: &BaseConfig{
				Server: ServerConfig{
					Port:        "8080",
					Environment: "development",
				},
				Security: SecurityConfig{
					JWT: JWTConfig{
						Secret: "development-secret-key-with-32-chars-minimum",
					},
				},
				Observability: ObservabilityConfig{
					ServiceName: "test-service",
				},
			},
			wantError: false,
		},
		{
			name: "valid production config",
			config: &BaseConfig{
				Server: ServerConfig{
					Port:        "8080",
					Environment: "production",
				},
				Database: DatabaseConfig{
					Password: "production-password",
				},
				Security: SecurityConfig{
					JWT: JWTConfig{
						Secret: "production-secret-key-with-32-chars-minimum",
					},
				},
				Observability: ObservabilityConfig{
					ServiceName: "test-service",
				},
			},
			wantError: false,
		},
		{
			name: "invalid - missing JWT secret",
			config: &BaseConfig{
				Server: ServerConfig{
					Port:        "8080",
					Environment: "development",
				},
				Security: SecurityConfig{
					JWT: JWTConfig{
						Secret: "",
					},
				},
				Observability: ObservabilityConfig{
					ServiceName: "test-service",
				},
			},
			wantError: true,
		},
		{
			name: "invalid - JWT secret too short",
			config: &BaseConfig{
				Server: ServerConfig{
					Port:        "8080",
					Environment: "development",
				},
				Security: SecurityConfig{
					JWT: JWTConfig{
						Secret: "short",
					},
				},
				Observability: ObservabilityConfig{
					ServiceName: "test-service",
				},
			},
			wantError: true,
		},
		{
			name: "invalid - production with default DB password",
			config: &BaseConfig{
				Server: ServerConfig{
					Port:        "8080",
					Environment: "production",
				},
				Database: DatabaseConfig{
					Password: "postgres",
				},
				Security: SecurityConfig{
					JWT: JWTConfig{
						Secret: "production-secret-key-with-32-chars-minimum",
					},
				},
				Observability: ObservabilityConfig{
					ServiceName: "test-service",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}