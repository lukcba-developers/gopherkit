package errors

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDomainError(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		message  string
		category ErrorCategory
		severity ErrorSeverity
	}{
		{
			name:     "simple error",
			code:     "TEST_ERROR",
			message:  "Test error message",
			category: CategoryBusiness,
			severity: SeverityMedium,
		},
		{
			name:     "validation error",
			code:     "VALIDATION_ERROR",
			message:  "Validation failed",
			category: CategoryValidation,
			severity: SeverityLow,
		},
		{
			name:     "critical security error",
			code:     "SECURITY_BREACH",
			message:  "Security breach detected",
			category: CategorySecurity,
			severity: SeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewDomainError(tt.code, tt.message, tt.category, tt.severity)

			assert.Equal(t, tt.code, err.Code)
			assert.Equal(t, tt.message, err.Message)
			assert.Equal(t, tt.category, err.Category)
			assert.Equal(t, tt.severity, err.Severity)
			assert.NotEmpty(t, err.ID)
			assert.NotEmpty(t, err.Timestamp)
			assert.NotNil(t, err.Details)
			assert.Equal(t, 500, err.StatusCode) // Default status code
		})
	}
}

func TestDomainError_Error(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	result := err.Error()
	
	assert.Contains(t, result, "TEST_ERROR")
	assert.Contains(t, result, "Test message")
}

func TestDomainError_WithCause(t *testing.T) {
	originalErr := errors.New("original error")
	domainErr := NewDomainError("WRAPPER_ERROR", "Wrapped error", CategoryTechnical, SeverityHigh)
	
	wrappedErr := domainErr.WithCause(originalErr)
	
	assert.Equal(t, originalErr, wrappedErr.Cause)
	assert.Equal(t, originalErr.Error(), wrappedErr.CauseMsg)
	assert.Equal(t, domainErr.Code, wrappedErr.Code)
	assert.Equal(t, domainErr.Message, wrappedErr.Message)
}

func TestDomainError_WithMetadata(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	updatedErr := err.WithMetadata("key1", "value1")
	assert.Equal(t, "value1", updatedErr.Details["key1"])
	
	// Test chaining
	chainedErr := updatedErr.WithMetadata("key2", "value2")
	assert.Equal(t, "value1", chainedErr.Details["key1"])
	assert.Equal(t, "value2", chainedErr.Details["key2"])
}

func TestDomainError_WithStackTrace(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	updatedErr := err.WithStackTrace(0)
	assert.NotNil(t, updatedErr.StackTrace)
	assert.NotEmpty(t, updatedErr.StackTrace.Frames)
	assert.NotEmpty(t, updatedErr.StackTrace.Raw)
}

func TestDomainError_WithTags(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	updatedErr := err.WithTags("urgent", "user-facing")
	assert.Contains(t, updatedErr.Tags, "urgent")
	assert.Contains(t, updatedErr.Tags, "user-facing")
	
	// Test adding more tags
	finalErr := updatedErr.WithTags("database")
	assert.Contains(t, finalErr.Tags, "urgent")
	assert.Contains(t, finalErr.Tags, "user-facing")
	assert.Contains(t, finalErr.Tags, "database")
}

func TestDomainError_WithContext(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	context := &ErrorContext{
		Service:   "user-service",
		Version:   "1.0.0",
		RequestID: "req-123",
		UserID:    "user-456",
	}
	
	updatedErr := err.WithContext(context)
	assert.Equal(t, context, updatedErr.Context)
	assert.Equal(t, "user-service", updatedErr.Context.Service)
	assert.Equal(t, "req-123", updatedErr.Context.RequestID)
}

func TestDomainError_IsRetryable(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	// Default should be false
	assert.False(t, err.IsRetryable())
	
	// Set retryable
	err.Retryable = true
	assert.True(t, err.IsRetryable())
}

func TestDomainError_IsTransient(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	// Default should be false
	assert.False(t, err.IsTransient())
	
	// Set transient
	err.Transient = true
	assert.True(t, err.IsTransient())
}

func TestDomainError_Is(t *testing.T) {
	err1 := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	err2 := NewDomainError("TEST_ERROR", "Different message", CategoryTechnical, SeverityHigh)
	err3 := NewDomainError("DIFFERENT_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	// Same code should match
	assert.True(t, err1.Is(err2))
	
	// Different code should not match
	assert.False(t, err1.Is(err3))
	
	// Nil should not match
	assert.False(t, err1.Is(nil))
	
	// Non-DomainError should not match
	standardErr := errors.New("standard error")
	assert.False(t, err1.Is(standardErr))
}

func TestDomainError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	domainErr := NewDomainError("WRAPPER_ERROR", "Wrapped error", CategoryTechnical, SeverityHigh)
	wrappedErr := domainErr.WithCause(originalErr)
	
	unwrapped := wrappedErr.Unwrap()
	assert.Equal(t, originalErr, unwrapped)
	
	// Test unwrap on error without cause
	noCauseErr := NewDomainError("NO_CAUSE", "No cause error", CategoryBusiness, SeverityMedium)
	assert.Nil(t, noCauseErr.Unwrap())
}

func TestDomainError_GenerateFingerprint(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	err = err.WithStackTrace(0)
	
	fingerprint1 := err.GenerateFingerprint()
	assert.NotEmpty(t, fingerprint1)
	
	// Should return same fingerprint on subsequent calls
	fingerprint2 := err.GenerateFingerprint()
	assert.Equal(t, fingerprint1, fingerprint2)
	
	// Different error should have different fingerprint
	err2 := NewDomainError("DIFFERENT_ERROR", "Different message", CategoryBusiness, SeverityMedium)
	err2 = err2.WithStackTrace(0)
	fingerprint3 := err2.GenerateFingerprint()
	assert.NotEqual(t, fingerprint1, fingerprint3)
}

func TestDomainError_ToJSON(t *testing.T) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	err.WithMetadata("user_id", "123")
	err.WithMetadata("action", "create")
	
	// Test JSON marshaling
	jsonData, marshalErr := err.ToJSON()
	require.NoError(t, marshalErr)
	assert.NotEmpty(t, jsonData)
	
	// Test that JSON is valid
	var unmarshaled map[string]interface{}
	unmarshalErr := json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, unmarshalErr)
	
	assert.Equal(t, err.Code, unmarshaled["code"])
	assert.Equal(t, err.Message, unmarshaled["message"])
	assert.Equal(t, string(err.Category), unmarshaled["category"])
	assert.Equal(t, string(err.Severity), unmarshaled["severity"])
}

func TestDomainError_Chain(t *testing.T) {
	// Test error chaining with multiple levels
	rootErr := errors.New("root cause")
	level1Err := NewDomainError("LEVEL1_ERROR", "Level 1 error", CategoryTechnical, SeverityHigh).WithCause(rootErr)
	level2Err := NewDomainError("LEVEL2_ERROR", "Level 2 error", CategoryBusiness, SeverityMedium).WithCause(level1Err)
	
	// Test that unwrapping works through the chain
	assert.Equal(t, level1Err, level2Err.Unwrap())
	assert.Equal(t, rootErr, level1Err.Unwrap())
}

func TestDomainError_EmptyValues(t *testing.T) {
	// Test with empty code
	err1 := NewDomainError("", "message", CategoryBusiness, SeverityMedium)
	assert.Equal(t, "", err1.Code)
	assert.Equal(t, "message", err1.Message)
	
	// Test with empty message
	err2 := NewDomainError("CODE", "", CategoryBusiness, SeverityMedium)
	assert.Equal(t, "CODE", err2.Code)
	assert.Equal(t, "", err2.Message)
}

func TestDomainError_ErrorCategories(t *testing.T) {
	tests := []struct {
		name     string
		category ErrorCategory
	}{
		{"validation", CategoryValidation},
		{"business", CategoryBusiness},
		{"technical", CategoryTechnical},
		{"security", CategorySecurity},
		{"integration", CategoryIntegration},
		{"performance", CategoryPerformance},
		{"infrastructure", CategoryInfrastructure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewDomainError("TEST", "Test", tt.category, SeverityMedium)
			assert.Equal(t, tt.category, err.Category)
		})
	}
}

func TestDomainError_ErrorSeverities(t *testing.T) {
	tests := []struct {
		name     string
		severity ErrorSeverity
	}{
		{"low", SeverityLow},
		{"medium", SeverityMedium},
		{"high", SeverityHigh},
		{"critical", SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewDomainError("TEST", "Test", CategoryBusiness, tt.severity)
			assert.Equal(t, tt.severity, err.Severity)
		})
	}
}

func TestDomainError_ComplexScenario(t *testing.T) {
	// Test a complex scenario with all features
	originalErr := errors.New("database connection failed")
	
	err := NewDomainError("DATABASE_ERROR", "Failed to save user", CategoryInfrastructure, SeverityCritical)
	err = err.WithCause(originalErr)
	err = err.WithMetadata("user_id", "123")
	err = err.WithMetadata("table", "users")
	err = err.WithMetadata("operation", "INSERT")
	err = err.WithTags("database", "critical", "user-operation")
	err = err.WithStackTrace(0)
	
	context := &ErrorContext{
		Service:       "user-service",
		Version:       "1.2.3",
		RequestID:     "req-456",
		CorrelationID: "corr-789",
		UserID:        "user-123",
		Environment:   "production",
	}
	err = err.WithContext(context)
	
	err.Retryable = true
	err.Transient = false
	err.StatusCode = 500
	
	// Verify all properties
	assert.Equal(t, "DATABASE_ERROR", err.Code)
	assert.Equal(t, "Failed to save user", err.Message)
	assert.Equal(t, CategoryInfrastructure, err.Category)
	assert.Equal(t, SeverityCritical, err.Severity)
	assert.Equal(t, originalErr, err.Cause)
	assert.Equal(t, "123", err.Details["user_id"])
	assert.Equal(t, "users", err.Details["table"])
	assert.Equal(t, "INSERT", err.Details["operation"])
	assert.Contains(t, err.Tags, "database")
	assert.Contains(t, err.Tags, "critical")
	assert.Contains(t, err.Tags, "user-operation")
	assert.NotNil(t, err.StackTrace)
	assert.Equal(t, context, err.Context)
	assert.True(t, err.Retryable)
	assert.False(t, err.Transient)
	assert.Equal(t, 500, err.StatusCode)
	
	// Test JSON serialization of complex error
	jsonData, err2 := err.ToJSON()
	require.NoError(t, err2)
	assert.NotEmpty(t, jsonData)
	
	// Test error message contains relevant information
	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "DATABASE_ERROR")
	assert.Contains(t, errorMsg, "Failed to save user")
	assert.Contains(t, errorMsg, "database connection failed")
}

// Benchmark tests
func BenchmarkNewDomainError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	}
}

func BenchmarkDomainError_WithMetadata(b *testing.B) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err.WithMetadata("key", "value")
	}
}

func BenchmarkDomainError_WithStackTrace(b *testing.B) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err.WithStackTrace(0)
	}
}

func BenchmarkDomainError_ToJSON(b *testing.B) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	err = err.WithMetadata("user_id", "123")
	err = err.WithMetadata("action", "create")
	err = err.WithTags("urgent", "user-facing")
	err = err.WithStackTrace(0)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err.ToJSON()
	}
}

func BenchmarkDomainError_GenerateFingerprint(b *testing.B) {
	err := NewDomainError("TEST_ERROR", "Test message", CategoryBusiness, SeverityMedium)
	err = err.WithStackTrace(0)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err.GenerateFingerprint()
	}
}