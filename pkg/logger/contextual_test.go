package logger

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	// Test development environment
	os.Setenv("ENVIRONMENT", "development")
	defer os.Unsetenv("ENVIRONMENT")

	logger := New("test-service")
	require.NotNil(t, logger)
	assert.Equal(t, "test-service", logger.serviceName)
	assert.NotNil(t, logger.Logger)
}

func TestContextualLogger_WithContext(t *testing.T) {
	logger := New("test-service")
	
	// Create context with values
	ctx := context.Background()
	ctx = context.WithValue(ctx, "tenant_id", "tenant123")
	ctx = context.WithValue(ctx, "correlation_id", "corr456")
	ctx = context.WithValue(ctx, "user_id", "user789")
	ctx = context.WithValue(ctx, "request_id", "req101")

	entry := logger.WithContext(ctx)
	require.NotNil(t, entry)

	// Test that entry has the context fields
	// This is hard to test directly without inspecting internal state,
	// but we can verify the entry is created without panicking
	entry.Info("test message")
}

func TestContextualLogger_WithMethods(t *testing.T) {
	logger := New("test-service")

	tests := []struct {
		name   string
		method func() LoggerEntry
	}{
		{
			name:   "WithTenant",
			method: func() LoggerEntry { return logger.WithTenant("tenant123") },
		},
		{
			name:   "WithUser",
			method: func() LoggerEntry { return logger.WithUser("user456") },
		},
		{
			name:   "WithCorrelation",
			method: func() LoggerEntry { return logger.WithCorrelation("corr789") },
		},
		{
			name:   "WithOperation",
			method: func() LoggerEntry { return logger.WithOperation("test_op") },
		},
		{
			name: "WithFields",
			method: func() LoggerEntry {
				return logger.WithFields(map[string]interface{}{
					"key1": "value1",
					"key2": 42,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := tt.method()
			require.NotNil(t, entry)
			
			// Test that we can log without panic
			entry.Info("test message")
		})
	}
}

func TestContextualLogger_BusinessMethods(t *testing.T) {
	logger := New("test-service")
	ctx := context.Background()

	// Test business event logging
	t.Run("LogBusinessEvent", func(t *testing.T) {
		// This should not panic
		logger.LogBusinessEvent(ctx, "user_created", map[string]interface{}{
			"user_id": "123",
			"email":   "test@example.com",
		})
	})

	t.Run("LogSecurityEvent", func(t *testing.T) {
		logger.LogSecurityEvent(ctx, "failed_login", map[string]interface{}{
			"email":     "attacker@example.com",
			"client_ip": "192.168.1.100",
		})
	})

	t.Run("LogPerformanceEvent", func(t *testing.T) {
		logger.LogPerformanceEvent(ctx, "database_query", 250*time.Millisecond, map[string]interface{}{
			"query": "SELECT * FROM users",
			"rows":  100,
		})
	})

	t.Run("LogError", func(t *testing.T) {
		err := assert.AnError
		logger.LogError(ctx, err, "test error", map[string]interface{}{
			"component": "test",
		})
	})

	t.Run("LogDatabaseOperation", func(t *testing.T) {
		logger.LogDatabaseOperation(ctx, "SELECT * FROM users WHERE id = ?", 50*time.Millisecond, 1)
	})

	t.Run("LogExternalAPICall", func(t *testing.T) {
		logger.LogExternalAPICall(ctx, "user-service", "GET", "/api/users/123", 200, 100*time.Millisecond)
		logger.LogExternalAPICall(ctx, "payment-service", "POST", "/api/payments", 500, 2*time.Second)
	})
}

func TestContextExtractionFunctions(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		extractor func(context.Context) string
	}{
		{
			name:      "getTenantFromContext with tenant_id",
			key:       "tenant_id",
			value:     "tenant123",
			extractor: getTenantFromContext,
		},
		{
			name:      "getTenantFromContext with tenantID",
			key:       "tenantID",
			value:     "tenant456",
			extractor: getTenantFromContext,
		},
		{
			name:      "getCorrelationIDFromContext with correlation_id",
			key:       "correlation_id",
			value:     "corr123",
			extractor: getCorrelationIDFromContext,
		},
		{
			name:      "getCorrelationIDFromContext with X-Correlation-ID",
			key:       "X-Correlation-ID",
			value:     "corr456",
			extractor: getCorrelationIDFromContext,
		},
		{
			name:      "getUserIDFromContext with user_id",
			key:       "user_id",
			value:     "user123",
			extractor: getUserIDFromContext,
		},
		{
			name:      "getRequestIDFromContext with request_id",
			key:       "request_id",
			value:     "req123",
			extractor: getRequestIDFromContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), tt.key, tt.value)
			result := tt.extractor(ctx)
			assert.Equal(t, tt.value, result)
		})
	}

	// Test with empty context
	t.Run("empty context returns empty string", func(t *testing.T) {
		ctx := context.Background()
		assert.Empty(t, getTenantFromContext(ctx))
		assert.Empty(t, getCorrelationIDFromContext(ctx))
		assert.Empty(t, getUserIDFromContext(ctx))
		assert.Empty(t, getRequestIDFromContext(ctx))
	})
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("isProduction", func(t *testing.T) {
		// Test production environments
		prodEnvs := []string{"production", "prod"}
		for _, env := range prodEnvs {
			os.Setenv("ENVIRONMENT", env)
			assert.True(t, isProduction(), "Expected production for env: %s", env)
		}

		// Test non-production environments  
		nonProdEnvs := []string{"development", "dev", "staging", "test", ""}
		for _, env := range nonProdEnvs {
			os.Setenv("ENVIRONMENT", env)
			assert.False(t, isProduction(), "Expected non-production for env: %s", env)
		}

		os.Unsetenv("ENVIRONMENT")
	})

	t.Run("getLogLevel", func(t *testing.T) {
		// Test valid log levels
		validLevels := map[string]interface{}{
			"debug": nil,
			"info":  nil,
			"warn":  nil,
			"error": nil,
			"fatal": nil,
			"panic": nil,
		}

		for level := range validLevels {
			os.Setenv("LOG_LEVEL", level)
			result := getLogLevel()
			assert.NotNil(t, result)
		}

		// Test invalid log level (should default to InfoLevel)
		os.Setenv("LOG_LEVEL", "invalid")
		result := getLogLevel()
		assert.NotNil(t, result)

		// Test empty log level (should default to InfoLevel)
		os.Unsetenv("LOG_LEVEL")
		result = getLogLevel()
		assert.NotNil(t, result)
	})
}

func TestMaskSensitiveData(t *testing.T) {
	// Currently maskSensitiveData is a placeholder
	// This test ensures it doesn't panic and returns something
	testCases := []string{
		"password=secret123",
		"SELECT * FROM users WHERE email = 'test@example.com'",
		"normal log message",
		"",
	}

	for _, input := range testCases {
		result := maskSensitiveData(input)
		assert.NotEmpty(t, result, "maskSensitiveData should not return empty string")
		// For now, it just returns the input as-is
		assert.Equal(t, input, result)
	}
}

func TestLoggerInterface(t *testing.T) {
	// Test that our ContextualLogger implements the Logger interface
	var logger Logger = New("test-service")
	require.NotNil(t, logger)

	ctx := context.Background()

	// Test interface methods don't panic
	entry := logger.WithContext(ctx)
	assert.NotNil(t, entry)

	entry = logger.WithTenant("tenant123")
	assert.NotNil(t, entry)

	entry = logger.WithUser("user456")
	assert.NotNil(t, entry)

	// Test business methods don't panic
	logger.LogBusinessEvent(ctx, "test_event", nil)
	logger.LogSecurityEvent(ctx, "test_security", nil)
	logger.LogPerformanceEvent(ctx, "test_perf", time.Millisecond, nil)
	logger.LogError(ctx, assert.AnError, "test error", nil)
}