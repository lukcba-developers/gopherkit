package validation

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

// ValidateStruct validates a complete struct
func (v *UnifiedValidator) ValidateStruct(s interface{}) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	err := v.validator.Struct(s)
	if err != nil {
		result.IsValid = false
		
		// Handle validation errors
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, fieldError := range validationErrors {
				validationError := ValidationError{
					Field:    fieldError.Field(),
					Value:    fmt.Sprintf("%v", fieldError.Value()),
					Code:     v.getErrorCode(fieldError),
					Message:  v.getErrorMessage(fieldError),
					Severity: v.getErrorSeverity(fieldError),
				}
				result.Errors = append(result.Errors, validationError)
			}
		}
	}

	// Perform additional security validations if enabled
	if v.config.EnableXSSProtection || v.config.EnableSQLInjection || v.config.EnableScriptCheck {
		securityResult := v.validateSecurity(s)
		result.Errors = append(result.Errors, securityResult.Errors...)
		if len(securityResult.Errors) > 0 {
			result.IsValid = false
		}
	}

	return result
}

// ValidateString validates a string value
func (v *UnifiedValidator) ValidateString(value, fieldName string, rules ...string) *ValidationResult {
	result := &ValidationResult{
		Field:   fieldName,
		Value:   value,
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	// Basic validations
	if len(value) > v.config.MaxStringLength {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    fieldName,
			Value:    value,
			Code:     "MAX_LENGTH_ERROR",
			Message:  fmt.Sprintf("exceeds maximum length of %d", v.config.MaxStringLength),
			Severity: "medium",
		})
	}

	// UTF-8 validation
	if !utf8.ValidString(value) {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    fieldName,
			Value:    value,
			Code:     "INVALID_UTF8_ERROR",
			Message:  "contains invalid UTF-8 characters",
			Severity: "high",
		})
	}

	// Security validations
	if v.containsSuspiciousPatterns(value) {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    fieldName,
			Value:    value,
			Code:     "SECURITY_THREAT_ERROR",
			Message:  "contains potentially dangerous content",
			Severity: "high",
		})
	}

	// Apply custom rules
	for _, rule := range rules {
		if !v.applyStringRule(value, rule) {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:    fieldName,
				Value:    value,
				Code:     "RULE_VIOLATION_ERROR",
				Message:  fmt.Sprintf("failed rule: %s", rule),
				Severity: "medium",
			})
		}
	}

	return result
}

// ValidateEmail validates an email address
func (v *UnifiedValidator) ValidateEmail(email string) *ValidationResult {
	result := &ValidationResult{
		Field:   "email",
		Value:   email,
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	// Basic checks
	if len(strings.TrimSpace(email)) == 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    email,
			Code:     "REQUIRED_ERROR",
			Message:  "email is required",
			Severity: "medium",
		})
		return result
	}

	if len(email) > v.config.MaxEmailLength {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    email,
			Code:     "MAX_LENGTH_ERROR",
			Message:  fmt.Sprintf("email exceeds maximum length of %d", v.config.MaxEmailLength),
			Severity: "medium",
		})
	}

	// Standard email validation
	_, err := mail.ParseAddress(email)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    email,
			Code:     "INVALID_FORMAT_ERROR",
			Message:  "invalid email format",
			Severity: "medium",
		})
	}

	// Security validation
	if v.containsSuspiciousPatterns(email) {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    email,
			Code:     "SECURITY_THREAT_ERROR",
			Message:  "email contains suspicious patterns",
			Severity: "high",
		})
	}

	return result
}

// ValidateName validates a name field
func (v *UnifiedValidator) ValidateName(name string) *ValidationResult {
	result := &ValidationResult{
		Field:   "name",
		Value:   name,
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	name = strings.TrimSpace(name)

	if len(name) == 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "name",
			Value:    name,
			Code:     "REQUIRED_ERROR",
			Message:  "name is required",
			Severity: "medium",
		})
		return result
	}

	if len(name) > v.config.MaxNameLength {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "name",
			Value:    name,
			Code:     "MAX_LENGTH_ERROR",
			Message:  fmt.Sprintf("name exceeds maximum length of %d", v.config.MaxNameLength),
			Severity: "medium",
		})
	}

	if !v.isValidName(name) {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "name",
			Value:    name,
			Code:     "INVALID_FORMAT_ERROR",
			Message:  "name contains invalid characters",
			Severity: "medium",
		})
	}

	if v.containsSuspiciousPatterns(name) {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "name",
			Value:    name,
			Code:     "SECURITY_THREAT_ERROR",
			Message:  "name contains suspicious patterns",
			Severity: "high",
		})
	}

	return result
}

// ValidatePassword validates a password
func (v *UnifiedValidator) ValidatePassword(password string) *ValidationResult {
	result := &ValidationResult{
		Field:   "password",
		Value:   "[REDACTED]", // Don't store actual password
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	if len(password) < v.config.MinPasswordLength {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "password",
			Value:    "[REDACTED]",
			Code:     "MIN_LENGTH_ERROR",
			Message:  fmt.Sprintf("password must be at least %d characters", v.config.MinPasswordLength),
			Severity: "medium",
		})
	}

	if len(password) > v.config.MaxPasswordLength {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "password",
			Value:    "[REDACTED]",
			Code:     "MAX_LENGTH_ERROR",
			Message:  fmt.Sprintf("password exceeds maximum length of %d", v.config.MaxPasswordLength),
			Severity: "medium",
		})
	}

	if !v.isStrongPassword(password) {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "password",
			Value:    "[REDACTED]",
			Code:     "WEAK_PASSWORD_ERROR",
			Message:  "password must contain uppercase, lowercase, number and special character",
			Severity: "high",
		})
	}

	return result
}

// ValidateUUID validates a UUID string
func (v *UnifiedValidator) ValidateUUID(uuid, fieldName string) *ValidationResult {
	result := &ValidationResult{
		Field:   fieldName,
		Value:   uuid,
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	if len(strings.TrimSpace(uuid)) == 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    fieldName,
			Value:    uuid,
			Code:     "REQUIRED_ERROR",
			Message:  "UUID is required",
			Severity: "medium",
		})
		return result
	}

	// UUID v4 pattern
	uuidPattern := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`
	matched, err := regexp.MatchString(uuidPattern, uuid)
	
	if err != nil || !matched {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    fieldName,
			Value:    uuid,
			Code:     "INVALID_FORMAT_ERROR",
			Message:  "invalid UUID format",
			Severity: "medium",
		})
	}

	return result
}

// ValidateSport validates a sport name
func (v *UnifiedValidator) ValidateSport(sport string) *ValidationResult {
	result := &ValidationResult{
		Field:   "sport",
		Value:   sport,
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	sport = strings.TrimSpace(strings.ToLower(sport))

	if len(sport) == 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "sport",
			Value:    sport,
			Code:     "REQUIRED_ERROR",
			Message:  "sport is required",
			Severity: "medium",
		})
		return result
	}

	// Check against allowed sports
	isValid := false
	for _, allowedSport := range v.config.AllowedSports {
		if sport == strings.ToLower(allowedSport) {
			isValid = true
			break
		}
	}

	if !isValid {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "sport",
			Value:    sport,
			Code:     "INVALID_VALUE_ERROR",
			Message:  "sport is not supported",
			Severity: "medium",
		})
	}

	return result
}

// ValidateCurrency validates a currency code
func (v *UnifiedValidator) ValidateCurrency(currency string) *ValidationResult {
	result := &ValidationResult{
		Field:   "currency",
		Value:   currency,
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	currency = strings.TrimSpace(strings.ToUpper(currency))

	if len(currency) == 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "currency",
			Value:    currency,
			Code:     "REQUIRED_ERROR",
			Message:  "currency is required",
			Severity: "medium",
		})
		return result
	}

	// Check against allowed currencies
	isValid := false
	for _, allowedCurrency := range v.config.AllowedCurrencies {
		if currency == allowedCurrency {
			isValid = true
			break
		}
	}

	if !isValid {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "currency",
			Value:    currency,
			Code:     "INVALID_VALUE_ERROR",
			Message:  "currency is not supported",
			Severity: "medium",
		})
	}

	return result
}