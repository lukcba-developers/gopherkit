package errors

import (
	"fmt"
	"net/http"
)

// Validation Errors

// NewValidationError crea un error de validación
func NewValidationError(message string, field string, value interface{}) *DomainError {
	err := NewDomainError("VALIDATION_ERROR", message, CategoryValidation, SeverityMedium)
	err.StatusCode = http.StatusBadRequest
	err.WithMetadata("field", field)
	err.WithMetadata("value", value)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewRequiredFieldError crea un error de campo requerido
func NewRequiredFieldError(field string) *DomainError {
	message := fmt.Sprintf("Field '%s' is required", field)
	return NewValidationError(message, field, nil)
}

// NewInvalidFormatError crea un error de formato inválido
func NewInvalidFormatError(field string, value interface{}, expectedFormat string) *DomainError {
	message := fmt.Sprintf("Field '%s' has invalid format. Expected: %s", field, expectedFormat)
	err := NewValidationError(message, field, value)
	err.WithMetadata("expected_format", expectedFormat)
	return err
}

// NewInvalidValueError crea un error de valor inválido
func NewInvalidValueError(field string, value interface{}, validValues []interface{}) *DomainError {
	message := fmt.Sprintf("Field '%s' has invalid value", field)
	err := NewValidationError(message, field, value)
	err.WithMetadata("valid_values", validValues)
	return err
}

// Business Logic Errors

// NewBusinessError crea un error de lógica de negocio
func NewBusinessError(code, message string) *DomainError {
	err := NewDomainError(code, message, CategoryBusiness, SeverityMedium)
	err.StatusCode = http.StatusUnprocessableEntity
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewNotFoundError crea un error de recurso no encontrado
func NewNotFoundError(resource string, identifier interface{}) *DomainError {
	message := fmt.Sprintf("%s not found", resource)
	err := NewDomainError("RESOURCE_NOT_FOUND", message, CategoryBusiness, SeverityLow)
	err.StatusCode = http.StatusNotFound
	err.WithMetadata("resource", resource)
	err.WithMetadata("identifier", identifier)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewAlreadyExistsError crea un error de recurso ya existente
func NewAlreadyExistsError(resource string, identifier interface{}) *DomainError {
	message := fmt.Sprintf("%s already exists", resource)
	err := NewDomainError("RESOURCE_ALREADY_EXISTS", message, CategoryBusiness, SeverityMedium)
	err.StatusCode = http.StatusConflict
	err.WithMetadata("resource", resource)
	err.WithMetadata("identifier", identifier)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewInsufficientPermissionsError crea un error de permisos insuficientes
func NewInsufficientPermissionsError(action string, resource string) *DomainError {
	message := fmt.Sprintf("Insufficient permissions to %s %s", action, resource)
	err := NewDomainError("INSUFFICIENT_PERMISSIONS", message, CategorySecurity, SeverityHigh)
	err.StatusCode = http.StatusForbidden
	err.WithMetadata("action", action)
	err.WithMetadata("resource", resource)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// Technical Errors

// NewInternalError crea un error interno del servidor
func NewInternalError(message string, cause error) *DomainError {
	err := NewDomainError("INTERNAL_ERROR", message, CategoryTechnical, SeverityCritical)
	err.StatusCode = http.StatusInternalServerError
	err.WithCause(cause)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// NewDatabaseError crea un error de base de datos
func NewDatabaseError(operation string, cause error) *DomainError {
	message := fmt.Sprintf("Database error during %s", operation)
	err := NewDomainError("DATABASE_ERROR", message, CategoryInfrastructure, SeverityHigh)
	err.StatusCode = http.StatusInternalServerError
	err.WithCause(cause)
	err.WithMetadata("operation", operation)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// NewExternalServiceError crea un error de servicio externo
func NewExternalServiceError(service string, cause error) *DomainError {
	message := fmt.Sprintf("External service error: %s", service)
	err := NewDomainError("EXTERNAL_SERVICE_ERROR", message, CategoryIntegration, SeverityHigh)
	err.StatusCode = http.StatusBadGateway
	err.WithCause(cause)
	err.WithMetadata("service", service)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// NewTimeoutError crea un error de timeout
func NewTimeoutError(operation string, timeoutSeconds int) *DomainError {
	message := fmt.Sprintf("Operation timed out: %s (timeout: %ds)", operation, timeoutSeconds)
	err := NewDomainError("TIMEOUT_ERROR", message, CategoryTechnical, SeverityMedium)
	err.StatusCode = http.StatusRequestTimeout
	err.WithMetadata("operation", operation)
	err.WithMetadata("timeout_seconds", timeoutSeconds)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// NewRateLimitExceededError crea un error de rate limit excedido
func NewRateLimitExceededError(limit int, windowSeconds int) *DomainError {
	message := fmt.Sprintf("Rate limit exceeded: %d requests per %d seconds", limit, windowSeconds)
	err := NewDomainError("RATE_LIMIT_EXCEEDED", message, CategorySecurity, SeverityMedium)
	err.StatusCode = http.StatusTooManyRequests
	err.WithMetadata("limit", limit)
	err.WithMetadata("window_seconds", windowSeconds)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// Security Errors

// NewAuthenticationError crea un error de autenticación
func NewAuthenticationError(reason string) *DomainError {
	message := "Authentication failed"
	if reason != "" {
		message = fmt.Sprintf("Authentication failed: %s", reason)
	}
	err := NewDomainError("AUTHENTICATION_FAILED", message, CategorySecurity, SeverityHigh)
	err.StatusCode = http.StatusUnauthorized
	err.WithMetadata("reason", reason)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewAuthorizationError crea un error de autorización
func NewAuthorizationError(resource string, action string) *DomainError {
	message := fmt.Sprintf("Access denied to %s %s", action, resource)
	err := NewDomainError("ACCESS_DENIED", message, CategorySecurity, SeverityHigh)
	err.StatusCode = http.StatusForbidden
	err.WithMetadata("resource", resource)
	err.WithMetadata("action", action)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewInvalidTokenError crea un error de token inválido
func NewInvalidTokenError(tokenType string) *DomainError {
	message := fmt.Sprintf("Invalid %s token", tokenType)
	err := NewDomainError("INVALID_TOKEN", message, CategorySecurity, SeverityMedium)
	err.StatusCode = http.StatusUnauthorized
	err.WithMetadata("token_type", tokenType)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// NewExpiredTokenError crea un error de token expirado
func NewExpiredTokenError(tokenType string) *DomainError {
	message := fmt.Sprintf("%s token has expired", tokenType)
	err := NewDomainError("TOKEN_EXPIRED", message, CategorySecurity, SeverityMedium)
	err.StatusCode = http.StatusUnauthorized
	err.WithMetadata("token_type", tokenType)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}

// Circuit Breaker Errors

// NewCircuitBreakerOpenError crea un error de circuit breaker abierto
func NewCircuitBreakerOpenError(service string) *DomainError {
	message := fmt.Sprintf("Circuit breaker is open for service: %s", service)
	err := NewDomainError("CIRCUIT_BREAKER_OPEN", message, CategoryTechnical, SeverityHigh)
	err.StatusCode = http.StatusServiceUnavailable
	err.WithMetadata("service", service)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// Integration Errors

// NewServiceUnavailableError crea un error de servicio no disponible
func NewServiceUnavailableError(service string) *DomainError {
	message := fmt.Sprintf("Service unavailable: %s", service)
	err := NewDomainError("SERVICE_UNAVAILABLE", message, CategoryIntegration, SeverityHigh)
	err.StatusCode = http.StatusServiceUnavailable
	err.WithMetadata("service", service)
	err.Retryable = true
	err.Transient = true
	return err.WithStackTrace(1)
}

// NewConfigurationError crea un error de configuración
func NewConfigurationError(setting string, reason string) *DomainError {
	message := fmt.Sprintf("Configuration error for %s: %s", setting, reason)
	err := NewDomainError("CONFIGURATION_ERROR", message, CategoryTechnical, SeverityCritical)
	err.StatusCode = http.StatusInternalServerError
	err.WithMetadata("setting", setting)
	err.WithMetadata("reason", reason)
	err.Retryable = false
	err.Transient = false
	return err.WithStackTrace(1)
}