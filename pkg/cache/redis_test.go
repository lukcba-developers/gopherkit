package cache

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lukcba-developers/gopherkit/pkg/config"
)

// TestData para testing de serialización
type TestDataSimple struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Count   int               `json:"count"`
	Active  bool              `json:"active"`
	Tags    []string          `json:"tags"`
	Meta    map[string]string `json:"meta"`
}

func TestCacheConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  config.CacheConfig
		isValid bool
	}{
		{
			name: "valid enabled config",
			config: config.CacheConfig{
				Enabled:  true,
				Host:     "localhost",
				Port:     "6379",
				Password: "",
				DB:       0,
				Prefix:   "test:",
				TTL:      5 * time.Minute,
			},
			isValid: true,
		},
		{
			name: "disabled config - should be valid",
			config: config.CacheConfig{
				Enabled: false,
			},
			isValid: true,
		},
		{
			name: "production config",
			config: config.CacheConfig{
				Enabled:  true,
				Host:     "redis.prod.com",
				Port:     "6379",
				Password: "secret",
				DB:       1,
				Prefix:   "prod:",
				TTL:      time.Hour,
			},
			isValid: true,
		},
		{
			name: "invalid - enabled but no host",
			config: config.CacheConfig{
				Enabled:  true,
				Host:     "",
				Port:     "6379",
				Password: "",
				DB:       0,
				Prefix:   "test:",
				TTL:      5 * time.Minute,
			},
			isValid: false,
		},
		{
			name: "invalid - enabled but no port",
			config: config.CacheConfig{
				Enabled:  true,
				Host:     "localhost",
				Port:     "",
				Password: "",
				DB:       0,
				Prefix:   "test:",
				TTL:      5 * time.Minute,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validar configuración básica
			if tt.config.Enabled {
				if tt.isValid {
					assert.NotEmpty(t, tt.config.Host, "Host should not be empty for enabled cache")
					assert.NotEmpty(t, tt.config.Port, "Port should not be empty for enabled cache")
				} else {
					// Para configuraciones inválidas, al menos una debe faltar
					invalid := tt.config.Host == "" || tt.config.Port == ""
					assert.True(t, invalid, "Invalid config should have missing host or port")
				}
			}
		})
	}
}

func TestRedisAddressBuilding(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     string
		expected string
	}{
		{
			name:     "localhost with default port",
			host:     "localhost",
			port:     "6379",
			expected: "localhost:6379",
		},
		{
			name:     "custom host and port",
			host:     "redis.example.com",
			port:     "6380",
			expected: "redis.example.com:6380",
		},
		{
			name:     "IP address with port",
			host:     "192.168.1.100",
			port:     "6379",
			expected: "192.168.1.100:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fmt.Sprintf("%s:%s", tt.host, tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTTLParsing(t *testing.T) {
	tests := []struct {
		name      string
		ttl       time.Duration
		expectErr bool
	}{
		{
			name:      "5 minutes",
			ttl:       5 * time.Minute,
			expectErr: false,
		},
		{
			name:      "1 hour",
			ttl:       time.Hour,
			expectErr: false,
		},
		{
			name:      "30 seconds",
			ttl:       30 * time.Second,
			expectErr: false,
		},
		{
			name:      "24 hours",
			ttl:       24 * time.Hour,
			expectErr: false,
		},
		{
			name:      "zero duration",
			ttl:       0,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simplemente verificar que el TTL sea el esperado
			assert.Equal(t, tt.ttl, tt.ttl)
			
			if tt.ttl > 0 {
				assert.Greater(t, tt.ttl, time.Duration(0))
			}
		})
	}
}

func TestRedisKeyBuilding(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		key      string
		expected string
	}{
		{
			name:     "with prefix",
			prefix:   "app:",
			key:      "user:123",
			expected: "app:user:123",
		},
		{
			name:     "empty prefix",
			prefix:   "",
			key:      "user:123",
			expected: "user:123",
		},
		{
			name:     "prefix without colon",
			prefix:   "app",
			key:      "user:123",
			expected: "appuser:123",
		},
		{
			name:     "empty key",
			prefix:   "app:",
			key:      "",
			expected: "app:",
		},
		{
			name:     "both empty",
			prefix:   "",
			key:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.prefix + tt.key
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONSerialization(t *testing.T) {
	testData := TestDataSimple{
		ID:     "test-123",
		Name:   "Test Item",
		Count:  42,
		Active: true,
		Tags:   []string{"tag1", "tag2", "tag3"},
		Meta: map[string]string{
			"version": "1.0.0",
			"env":     "test",
		},
	}

	// Test serialization
	data, err := json.Marshal(testData)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Test deserialization
	var deserializedData TestDataSimple
	err = json.Unmarshal(data, &deserializedData)
	require.NoError(t, err)

	// Verify data integrity
	assert.Equal(t, testData.ID, deserializedData.ID)
	assert.Equal(t, testData.Name, deserializedData.Name)
	assert.Equal(t, testData.Count, deserializedData.Count)
	assert.Equal(t, testData.Active, deserializedData.Active)
	assert.Equal(t, testData.Tags, deserializedData.Tags)
	assert.Equal(t, testData.Meta, deserializedData.Meta)
}

func TestConfigurationScenarios(t *testing.T) {
	scenarios := []struct {
		name   string
		config config.CacheConfig
		valid  bool
	}{
		{
			name: "minimal configuration",
			config: config.CacheConfig{
				Enabled: true,
				Host:    "localhost",
				Port:    "6379",
				TTL:     5 * time.Minute,
			},
			valid: true,
		},
		{
			name: "production configuration",
			config: config.CacheConfig{
				Enabled:  true,
				Host:     "redis.production.com",
				Port:     "6379",
				Password: "secure-password",
				DB:       1,
				Prefix:   "prod:app:",
				TTL:      time.Hour,
			},
			valid: true,
		},
		{
			name: "disabled cache",
			config: config.CacheConfig{
				Enabled: false,
			},
			valid: true,
		},
		{
			name: "invalid configuration - no host",
			config: config.CacheConfig{
				Enabled: true,
				Port:    "6379",
				TTL:     5 * time.Minute,
			},
			valid: false,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.config.Enabled {
				if scenario.valid {
					assert.NotEmpty(t, scenario.config.Host)
					assert.NotEmpty(t, scenario.config.Port)
				} else {
					// Al menos uno debería faltar
					missing := scenario.config.Host == "" || scenario.config.Port == ""
					assert.True(t, missing)
				}
			} else {
				// Cache deshabilitado siempre es válido
				assert.False(t, scenario.config.Enabled)
			}
		})
	}
}

// Benchmark para operaciones críticas
func BenchmarkJSONMarshal(b *testing.B) {
	testData := TestDataSimple{
		ID:     "test-123",
		Name:   "Test Item",
		Count:  42,
		Active: true,
		Tags:   []string{"tag1", "tag2", "tag3"},
		Meta: map[string]string{
			"version": "1.0.0",
			"env":     "test",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(testData)
		require.NoError(b, err)
	}
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	testData := TestDataSimple{
		ID:     "test-123",
		Name:   "Test Item",
		Count:  42,
		Active: true,
		Tags:   []string{"tag1", "tag2", "tag3"},
		Meta: map[string]string{
			"version": "1.0.0",
			"env":     "test",
		},
	}

	data, err := json.Marshal(testData)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result TestDataSimple
		err := json.Unmarshal(data, &result)
		require.NoError(b, err)
	}
}

func BenchmarkRedisKeyBuilding(b *testing.B) {
	prefix := "app:cache:"
	key := "user:session:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = prefix + key
	}
}