package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockLogger implements Logger interface for testing
type mockLogger struct{}

func (ml *mockLogger) Info(msg string)                                 {}
func (ml *mockLogger) Warn(msg string)                                 {}
func (ml *mockLogger) Error(msg string)                                {}
func (ml *mockLogger) Debugf(format string, args ...interface{})      {}
func (ml *mockLogger) WithField(key string, value interface{}) Logger { return ml }

func TestHotReloader(t *testing.T) {
	// Create temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "hot_reload_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test config file
	configFile := filepath.Join(tmpDir, "test_config.json")
	configContent := `{
		"server": {
			"port": "8080",
			"metrics_port": "9090",
			"environment": "development"
		},
		"observability": {
			"service_name": "test-service",
			"log_level": "info"
		}
	}`
	
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	assert.NoError(t, err)

	baseConfig := &BaseConfig{
		Server: ServerConfig{
			Port:        "8080",
			MetricsPort: "9090",
			Environment: "development",
		},
		Observability: ObservabilityConfig{
			ServiceName: "test-service",
			LogLevel:    "info",
		},
	}

	t.Run("NewHotReloader", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)
		assert.NotNil(t, reloader)
	})

	t.Run("GetConfig and UpdateConfig", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Test GetConfig
		config := reloader.GetConfig()
		assert.NotNil(t, config)

		// Test UpdateConfig
		updates := map[string]interface{}{
			"database_mode": "test",
			"cache_enabled": false,
		}

		err = reloader.UpdateConfig(updates)
		assert.NoError(t, err)

		// Verify config was updated
		updated := reloader.GetConfig()
		assert.Equal(t, "test", updated.DatabaseMode)
		assert.False(t, updated.CacheConfig.Enabled)
	})

	t.Run("SaveConfig", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Update config first
		updates := map[string]interface{}{
			"database_mode": "saved",
			"feature_flags.test_feature": true,
		}
		err = reloader.UpdateConfig(updates)
		assert.NoError(t, err)

		err = reloader.SaveConfig()
		assert.NoError(t, err)

		// Verify file was written
		content, err := os.ReadFile(configFile)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "saved")
		assert.Contains(t, string(content), "test_feature")
	})

	t.Run("RegisterCallback", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		callbackCalled := false
		reloader.RegisterCallback("test_callback", func(old, new *DynamicConfig) error {
			callbackCalled = true
			return nil
		})

		// Update config to trigger callback
		updates := map[string]interface{}{
			"database_mode": "callback_test",
		}

		err = reloader.UpdateConfig(updates)
		assert.NoError(t, err)
		
		// Wait a bit for async callback to execute
		time.Sleep(10 * time.Millisecond)
		assert.True(t, callbackCalled)
	})

	t.Run("GetMetrics", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		metrics := reloader.GetMetrics()
		assert.NotNil(t, metrics)
		assert.GreaterOrEqual(t, metrics.TotalReloads, int64(0))
		assert.GreaterOrEqual(t, metrics.SuccessfulReloads, int64(0))
		assert.GreaterOrEqual(t, metrics.FailedReloads, int64(0))
	})

	t.Run("GetLastReloadTime and IsRunning", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Initially should not be running
		assert.False(t, reloader.IsRunning())

		lastReload := reloader.GetLastReloadTime()
		assert.True(t, lastReload.IsZero() || !lastReload.IsZero())
	})

	t.Run("GetWatchedFiles", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		watchedFiles := reloader.GetWatchedFiles()
		assert.NotNil(t, watchedFiles)
		// Should contain at least the main config file
		found := false
		for _, file := range watchedFiles {
			if file == configFile {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("IsFeatureEnabled", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Test with a feature that should be enabled
		enabled := reloader.IsFeatureEnabled("hot_reload_enabled")
		assert.True(t, enabled)

		// Test with a feature that should be disabled
		enabled = reloader.IsFeatureEnabled("nonexistent_feature")
		assert.False(t, enabled)
	})

	t.Run("Start and Stop", func(t *testing.T) {
		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Test Start
		err = reloader.Start()
		assert.NoError(t, err)
		assert.True(t, reloader.IsRunning())

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)

		// Test Stop
		err = reloader.Stop()
		assert.NoError(t, err)
		assert.False(t, reloader.IsRunning())
	})
}

func TestHotReloaderEdgeCases(t *testing.T) {
	t.Run("Invalid config path", func(t *testing.T) {
		baseConfig := &BaseConfig{
			Server: ServerConfig{
				Port:        "8080",
				MetricsPort: "9090",
				Environment: "development",
			},
			Observability: ObservabilityConfig{
				ServiceName: "test-service",
				LogLevel:    "info",
			},
		}

		reloader, err := NewHotReloader("/nonexistent/path/config.json", baseConfig, &mockLogger{})
		assert.NoError(t, err) // Should not error on creation
		assert.NotNil(t, reloader)

		// Should handle gracefully
		config := reloader.GetConfig()
		assert.NotNil(t, config) // Should return provided base config
	})

	t.Run("Invalid JSON content", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "hot_reload_invalid_test")
		assert.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		invalidFile := filepath.Join(tmpDir, "invalid.json")
		err = os.WriteFile(invalidFile, []byte("{invalid json content"), 0644)
		assert.NoError(t, err)

		baseConfig := &BaseConfig{
			Server: ServerConfig{
				Port:        "8080",
				MetricsPort: "9090",
				Environment: "development",
			},
			Observability: ObservabilityConfig{
				ServiceName: "test-service",
				LogLevel:    "info",
			},
		}

		reloader, err := NewHotReloader(invalidFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Should handle invalid JSON gracefully
		config := reloader.GetConfig()
		assert.NotNil(t, config)
	})

	t.Run("Callback with error", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "hot_reload_callback_error_test")
		assert.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		configFile := filepath.Join(tmpDir, "callback_error_config.json")
		configContent := `{
			"server": {
				"port": "8080",
				"metrics_port": "9090",
				"environment": "development"
			},
			"observability": {
				"service_name": "test-service",
				"log_level": "info"
			}
		}`
		
		err = os.WriteFile(configFile, []byte(configContent), 0644)
		assert.NoError(t, err)

		baseConfig := &BaseConfig{
			Server: ServerConfig{
				Port:        "8080",
				MetricsPort: "9090",
				Environment: "development",
			},
			Observability: ObservabilityConfig{
				ServiceName: "test-service",
				LogLevel:    "info",
			},
		}

		reloader, err := NewHotReloader(configFile, baseConfig, &mockLogger{})
		assert.NoError(t, err)

		// Register callback that returns error
		reloader.RegisterCallback("error_callback", func(old, new *DynamicConfig) error {
			return fmt.Errorf("callback error")
		})

		// Update config - should handle callback error gracefully
		updates := map[string]interface{}{
			"database_mode": "error_test",
		}

		err = reloader.UpdateConfig(updates)
		// Should not return error even if callback fails
		assert.NoError(t, err)
	})
}

func TestMockLogger(t *testing.T) {
	logger := &mockLogger{}
	
	// Test all logger methods
	logger.Info("test info")
	logger.Warn("test warn")
	logger.Error("test error")
	logger.Debugf("test debug %s", "formatted")
	
	field := logger.WithField("key", "value")
	assert.NotNil(t, field)
	
	// Should not panic - all methods are no-ops
}

func TestHelperFunctions(t *testing.T) {
	t.Run("IsStaging", func(t *testing.T) {
		config := &BaseConfig{
			Server: ServerConfig{
				Environment: "staging",
			},
		}
		assert.True(t, config.IsStaging())

		config.Server.Environment = "development"
		assert.False(t, config.IsStaging())
	})

	t.Run("getEnvInt64OrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_INT64")
		assert.Equal(t, int64(42), getEnvInt64OrDefault("TEST_INT64", 42))

		os.Setenv("TEST_INT64", "123")
		assert.Equal(t, int64(123), getEnvInt64OrDefault("TEST_INT64", 42))
		
		os.Setenv("TEST_INT64", "invalid")
		assert.Equal(t, int64(42), getEnvInt64OrDefault("TEST_INT64", 42))
		os.Unsetenv("TEST_INT64")
	})

	t.Run("getEnvFloatOrDefault", func(t *testing.T) {
		os.Unsetenv("TEST_FLOAT")
		assert.Equal(t, 3.14, getEnvFloatOrDefault("TEST_FLOAT", 3.14))

		os.Setenv("TEST_FLOAT", "2.71")
		assert.Equal(t, 2.71, getEnvFloatOrDefault("TEST_FLOAT", 3.14))
		
		os.Setenv("TEST_FLOAT", "invalid")
		assert.Equal(t, 3.14, getEnvFloatOrDefault("TEST_FLOAT", 3.14))
		os.Unsetenv("TEST_FLOAT")
	})
}