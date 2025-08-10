package validation

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

// registerCustomValidators registers custom validation rules
func (v *UnifiedValidator) registerCustomValidators() {
	// Register custom validators here
	v.validator.RegisterValidation("strong_password", v.validateStrongPasswordRule)
	v.validator.RegisterValidation("safe_string", v.validateSafeStringRule)
	v.validator.RegisterValidation("alphanumeric_spaces", v.validateAlphanumericSpacesRule)
	v.validator.RegisterValidation("sport", v.validateSportRule)
	v.validator.RegisterValidation("currency", v.validateCurrencyRule)
}

// validateStrongPasswordRule custom validator for strong passwords
func (v *UnifiedValidator) validateStrongPasswordRule(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	return v.isStrongPassword(password)
}

// validateSafeStringRule custom validator for safe strings
func (v *UnifiedValidator) validateSafeStringRule(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return !v.containsSuspiciousPatterns(value)
}

// validateAlphanumericSpacesRule custom validator for alphanumeric with spaces
func (v *UnifiedValidator) validateAlphanumericSpacesRule(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' {
			return false
		}
	}
	return true
}

// validateSportRule custom validator for sport names
func (v *UnifiedValidator) validateSportRule(fl validator.FieldLevel) bool {
	sport := strings.TrimSpace(strings.ToLower(fl.Field().String()))
	if len(sport) == 0 {
		return false
	}
	
	for _, allowedSport := range v.config.AllowedSports {
		if sport == strings.ToLower(allowedSport) {
			return true
		}
	}
	return false
}

// validateCurrencyRule custom validator for currency codes
func (v *UnifiedValidator) validateCurrencyRule(fl validator.FieldLevel) bool {
	currency := strings.TrimSpace(strings.ToUpper(fl.Field().String()))
	if len(currency) == 0 {
		return false
	}
	
	for _, allowedCurrency := range v.config.AllowedCurrencies {
		if currency == allowedCurrency {
			return true
		}
	}
	return false
}

// getErrorMessage returns a human-readable error message for a validation error
func (v *UnifiedValidator) getErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Please enter a valid email address"
	case "min":
		return "Value is too short"
	case "max":
		return "Value is too long"
	case "strong_password":
		return "Password must contain at least 8 characters with uppercase, lowercase, number and special character"
	case "safe_string":
		return "Input contains potentially dangerous content"
	case "alphanumeric_spaces":
		return "Only letters, numbers and spaces are allowed"
	default:
		return "Invalid value"
	}
}

// getErrorCode returns an error code for a validation error
func (v *UnifiedValidator) getErrorCode(fe validator.FieldError) string {
	return strings.ToUpper(fe.Tag()) + "_ERROR"
}

// getErrorSeverity returns the severity level for a validation error
func (v *UnifiedValidator) getErrorSeverity(fe validator.FieldError) string {
	switch fe.Tag() {
	case "safe_string":
		return "high"
	case "strong_password":
		return "medium"
	case "required", "email":
		return "medium"
	default:
		return "low"
	}
}

// applyStringRule applies a specific validation rule to a string value
func (v *UnifiedValidator) applyStringRule(value, rule string) bool {
	switch rule {
	case "alphanumeric":
		return v.isAlphanumeric(value)
	case "alpha":
		return v.isAlpha(value)
	case "numeric":
		return v.isNumeric(value)
	case "safe":
		return !v.containsSuspiciousPatterns(value)
	case "name":
		return v.isValidName(value)
	case "no_spaces":
		return !strings.Contains(value, " ")
	default:
		return true
	}
}

// isValidName checks if a string is a valid name
func (v *UnifiedValidator) isValidName(name string) bool {
	name = strings.TrimSpace(name)
	if len(name) == 0 || len(name) > v.config.MaxNameLength {
		return false
	}
	
	for _, r := range name {
		if !unicode.IsLetter(r) && r != ' ' && r != '\'' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// isStrongPassword checks if a password meets strength requirements
func (v *UnifiedValidator) isStrongPassword(password string) bool {
	if len(password) < v.config.MinPasswordLength || len(password) > v.config.MaxPasswordLength {
		return false
	}
	
	var (
		hasUpper   = false
		hasLower   = false
		hasNumber  = false
		hasSpecial = false
	)
	
	for _, char := range password {
		if unicode.IsUpper(char) {
			hasUpper = true
		} else if unicode.IsLower(char) {
			hasLower = true
		} else if unicode.IsDigit(char) {
			hasNumber = true
		} else if unicode.IsPunct(char) || unicode.IsSymbol(char) {
			hasSpecial = true
		}
	}
	
	return hasUpper && hasLower && hasNumber && hasSpecial
}

// isAlphanumeric checks if string contains only alphanumeric characters
func (v *UnifiedValidator) isAlphanumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// isAlpha checks if string contains only alphabetic characters
func (v *UnifiedValidator) isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// isNumeric checks if string contains only numeric characters
func (v *UnifiedValidator) isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// SanitizeInput sanitizes input by removing/escaping dangerous characters
func (v *UnifiedValidator) SanitizeInput(input string) string {
	// Remove null bytes
	sanitized := strings.ReplaceAll(input, "\x00", "")
	
	// Remove other control characters except allowed ones
	var result strings.Builder
	for _, r := range sanitized {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		result.WriteRune(r)
	}
	
	sanitized = result.String()
	
	// Escape HTML if needed
	if v.config.EnableXSSProtection {
		sanitized = v.escapeHTML(sanitized)
	}
	
	// Trim spaces and ensure valid UTF-8
	sanitized = strings.TrimSpace(sanitized)
	if !utf8.ValidString(sanitized) {
		sanitized = strings.ToValidUTF8(sanitized, "")
	}
	
	return sanitized
}