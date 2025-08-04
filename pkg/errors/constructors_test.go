package errors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Validation Error Tests

func TestNewValidationError(t *testing.T) {
	message := "Invalid email format"
	field := "email"
	value := "invalid-email"
	
	err := NewValidationError(message, field, value)
	
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, message, err.Message)
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
	assert.Equal(t, field, err.Details["field"])
	assert.Equal(t, value, err.Details["value"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewRequiredFieldError(t *testing.T) {
	field := "email"
	
	err := NewRequiredFieldError(field)
	
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "Field 'email' is required", err.Message)
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
	assert.Equal(t, field, err.Details["field"])
	assert.Nil(t, err.Details["value"])
}

func TestNewInvalidFormatError(t *testing.T) {
	field := "email"
	value := "invalid-email"
	expectedFormat := "user@domain.com"
	
	err := NewInvalidFormatError(field, value, expectedFormat)
	
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "Field 'email' has invalid format. Expected: user@domain.com", err.Message)
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
	assert.Equal(t, field, err.Details["field"])
	assert.Equal(t, value, err.Details["value"])
	assert.Equal(t, expectedFormat, err.Details["expected_format"])
}

func TestNewInvalidValueError(t *testing.T) {
	field := "status"
	value := "invalid"
	validValues := []interface{}{"active", "inactive", "pending"}
	
	err := NewInvalidValueError(field, value, validValues)
	
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "Field 'status' has invalid value", err.Message)
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
	assert.Equal(t, field, err.Details["field"])
	assert.Equal(t, value, err.Details["value"])
	assert.Equal(t, validValues, err.Details["valid_values"])
}

// Business Logic Error Tests

func TestNewBusinessError(t *testing.T) {
	code := "INSUFFICIENT_FUNDS"
	message := "Insufficient funds for transaction"
	
	err := NewBusinessError(code, message)
	
	assert.Equal(t, http.StatusUnprocessableEntity, err.StatusCode)
	assert.Equal(t, message, err.Message)
	assert.Equal(t, code, err.Code)
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewNotFoundError(t *testing.T) {
	resource := "user"
	id := "123"
	
	err := NewNotFoundError(resource, id)
	
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
	assert.Equal(t, "user not found", err.Message)
	assert.Equal(t, "RESOURCE_NOT_FOUND", err.Code)
	assert.Equal(t, resource, err.Details["resource"])
	assert.Equal(t, id, err.Details["identifier"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewAlreadyExistsError(t *testing.T) {
	resource := "user"
	id := "test@example.com"
	
	err := NewAlreadyExistsError(resource, id)
	
	assert.Equal(t, http.StatusConflict, err.StatusCode)
	assert.Equal(t, "user already exists", err.Message)
	assert.Equal(t, "RESOURCE_ALREADY_EXISTS", err.Code)
	assert.Equal(t, resource, err.Details["resource"])
	assert.Equal(t, id, err.Details["identifier"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewInsufficientPermissionsError(t *testing.T) {
	action := "delete"
	resource := "user"
	
	err := NewInsufficientPermissionsError(action, resource)
	
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, "Insufficient permissions to delete user", err.Message)
	assert.Equal(t, "INSUFFICIENT_PERMISSIONS", err.Code)
	assert.Equal(t, action, err.Details["action"])
	assert.Equal(t, resource, err.Details["resource"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

// Technical Error Tests

func TestNewInternalError(t *testing.T) {
	message := "Database connection failed"
	cause := errors.New("connection timeout")
	
	err := NewInternalError(message, cause)
	
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.Equal(t, message, err.Message)
	assert.Equal(t, "INTERNAL_ERROR", err.Code)
	assert.Equal(t, cause, err.Cause)
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

func TestNewDatabaseError(t *testing.T) {
	operation := "SELECT users"
	cause := errors.New("connection lost")
	
	err := NewDatabaseError(operation, cause)
	
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.Equal(t, "Database error during SELECT users", err.Message)
	assert.Equal(t, "DATABASE_ERROR", err.Code)
	assert.Equal(t, cause, err.Cause)
	assert.Equal(t, operation, err.Details["operation"])
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

func TestNewExternalServiceError(t *testing.T) {
	service := "payment-gateway"
	cause := errors.New("service unavailable")
	
	err := NewExternalServiceError(service, cause)
	
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
	assert.Equal(t, "External service error: payment-gateway", err.Message)
	assert.Equal(t, "EXTERNAL_SERVICE_ERROR", err.Code)
	assert.Equal(t, cause, err.Cause)
	assert.Equal(t, service, err.Details["service"])
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

func TestNewTimeoutError(t *testing.T) {
	operation := "database query"
	timeoutSeconds := 30
	
	err := NewTimeoutError(operation, timeoutSeconds)
	
	assert.Equal(t, http.StatusRequestTimeout, err.StatusCode)
	assert.Equal(t, "Operation timed out: database query (timeout: 30s)", err.Message)
	assert.Equal(t, "TIMEOUT_ERROR", err.Code)
	assert.Equal(t, operation, err.Details["operation"])
	assert.Equal(t, timeoutSeconds, err.Details["timeout_seconds"])
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

func TestNewRateLimitExceededError(t *testing.T) {
	limit := 100
	windowSeconds := 3600
	
	err := NewRateLimitExceededError(limit, windowSeconds)
	
	assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	assert.Equal(t, "Rate limit exceeded: 100 requests per 3600 seconds", err.Message)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", err.Code)
	assert.Equal(t, limit, err.Details["limit"])
	assert.Equal(t, windowSeconds, err.Details["window_seconds"])
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

// Security Error Tests

func TestNewAuthenticationError(t *testing.T) {
	reason := "Invalid credentials"
	
	err := NewAuthenticationError(reason)
	
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "Authentication failed: Invalid credentials", err.Message)
	assert.Equal(t, "AUTHENTICATION_FAILED", err.Code)
	assert.Equal(t, reason, err.Details["reason"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewAuthenticationError_EmptyReason(t *testing.T) {
	err := NewAuthenticationError("")
	
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "Authentication failed", err.Message)
	assert.Equal(t, "AUTHENTICATION_FAILED", err.Code)
	assert.Equal(t, "", err.Details["reason"])
}

func TestNewAuthorizationError(t *testing.T) {
	resource := "admin panel"
	action := "access"
	
	err := NewAuthorizationError(resource, action)
	
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, "Access denied to access admin panel", err.Message)
	assert.Equal(t, "ACCESS_DENIED", err.Code)
	assert.Equal(t, resource, err.Details["resource"])
	assert.Equal(t, action, err.Details["action"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewInvalidTokenError(t *testing.T) {
	tokenType := "JWT"
	
	err := NewInvalidTokenError(tokenType)
	
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "Invalid JWT token", err.Message)
	assert.Equal(t, "INVALID_TOKEN", err.Code)
	assert.Equal(t, tokenType, err.Details["token_type"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

func TestNewExpiredTokenError(t *testing.T) {
	tokenType := "refresh"
	
	err := NewExpiredTokenError(tokenType)
	
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "refresh token has expired", err.Message)
	assert.Equal(t, "TOKEN_EXPIRED", err.Code)
	assert.Equal(t, tokenType, err.Details["token_type"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

// Circuit Breaker Error Tests

func TestNewCircuitBreakerOpenError(t *testing.T) {
	service := "user-service"
	
	err := NewCircuitBreakerOpenError(service)
	
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, "Circuit breaker is open for service: user-service", err.Message)
	assert.Equal(t, "CIRCUIT_BREAKER_OPEN", err.Code)
	assert.Equal(t, service, err.Details["service"])
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

// Integration Error Tests

func TestNewServiceUnavailableError(t *testing.T) {
	service := "payment-service"
	
	err := NewServiceUnavailableError(service)
	
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, "Service unavailable: payment-service", err.Message)
	assert.Equal(t, "SERVICE_UNAVAILABLE", err.Code)
	assert.Equal(t, service, err.Details["service"])
	assert.True(t, err.Retryable)
	assert.True(t, err.Transient)
}

func TestNewConfigurationError(t *testing.T) {
	setting := "database.host"
	reason := "host is required"
	
	err := NewConfigurationError(setting, reason)
	
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.Equal(t, "Configuration error for database.host: host is required", err.Message)
	assert.Equal(t, "CONFIGURATION_ERROR", err.Code)
	assert.Equal(t, setting, err.Details["setting"])
	assert.Equal(t, reason, err.Details["reason"])
	assert.False(t, err.Retryable)
	assert.False(t, err.Transient)
}

// Edge cases and error scenarios

func TestConstructors_WithNilCause(t *testing.T) {
	err := NewInternalError("Test error", nil)
	assert.Equal(t, "Test error", err.Message)
	assert.Nil(t, err.Cause)
}

func TestConstructors_WithEmptyStrings(t *testing.T) {
	// Test with empty resource name
	err1 := NewNotFoundError("", "123")
	assert.Equal(t, " not found", err1.Message)
	
	// Test with empty validation message
	err2 := NewValidationError("", "field", "value")
	assert.Equal(t, "", err2.Message)
	
	// Test with empty authentication reason
	err3 := NewAuthenticationError("")
	assert.Equal(t, "Authentication failed", err3.Message)
}

func TestConstructors_StackTracePresent(t *testing.T) {
	err := NewNotFoundError("user", "123")
	assert.NotNil(t, err.StackTrace)
	assert.NotEmpty(t, err.StackTrace.Frames)
	assert.Contains(t, err.StackTrace.Raw, "constructors_test.go")
}

func TestConstructors_Categories(t *testing.T) {
	// Test different error categories
	validationErr := NewValidationError("test", "field", "value")
	assert.Equal(t, CategoryValidation, validationErr.Category)
	
	businessErr := NewBusinessError("TEST", "test")
	assert.Equal(t, CategoryBusiness, businessErr.Category)
	
	technicalErr := NewInternalError("test", nil)
	assert.Equal(t, CategoryTechnical, technicalErr.Category)
	
	securityErr := NewAuthenticationError("test")
	assert.Equal(t, CategorySecurity, securityErr.Category)
	
	integrationErr := NewExternalServiceError("test", nil)
	assert.Equal(t, CategoryIntegration, integrationErr.Category)
	
	infrastructureErr := NewDatabaseError("test", nil)
	assert.Equal(t, CategoryInfrastructure, infrastructureErr.Category)
}

func TestConstructors_Severities(t *testing.T) {
	// Test different error severities
	lowSeverityErr := NewNotFoundError("user", "123")
	assert.Equal(t, SeverityLow, lowSeverityErr.Severity)
	
	mediumSeverityErr := NewValidationError("test", "field", "value")
	assert.Equal(t, SeverityMedium, mediumSeverityErr.Severity)
	
	highSeverityErr := NewAuthenticationError("test")
	assert.Equal(t, SeverityHigh, highSeverityErr.Severity)
	
	criticalSeverityErr := NewInternalError("test", nil)
	assert.Equal(t, SeverityCritical, criticalSeverityErr.Severity)
}

// Benchmark tests
func BenchmarkNewValidationError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewValidationError("Invalid email", "email", "invalid@")
	}
}

func BenchmarkNewNotFoundError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewNotFoundError("user", "123")
	}
}

func BenchmarkNewInternalError(b *testing.B) {
	cause := errors.New("test error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewInternalError("Internal error", cause)
	}
}

func BenchmarkNewAuthenticationError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewAuthenticationError("Invalid credentials")
	}
}