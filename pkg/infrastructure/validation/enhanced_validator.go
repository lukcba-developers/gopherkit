package validation

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
)

// EnhancedValidator provides comprehensive input validation with security features
type EnhancedValidator struct {
	validator *validator.Validate
	logger    *logrus.Logger
	config    ValidationConfig
}

// ValidationConfig holds validation configuration
type ValidationConfig struct {
	EnableSQLInjectionCheck bool     `json:"enable_sql_injection_check"`
	EnableXSSCheck          bool     `json:"enable_xss_check"`
	EnableCSRFCheck         bool     `json:"enable_csrf_check"`
	MaxStringLength         int      `json:"max_string_length"`
	AllowedFileTypes        []string `json:"allowed_file_types"`
	MaxFileSize             int64    `json:"max_file_size"`
	EnableRateLimiting      bool     `json:"enable_rate_limiting"`
}

// ValidationError represents a validation error with additional context
type ValidationError struct {
	Field       string                 `json:"field"`
	Value       interface{}            `json:"value,omitempty"`
	Tag         string                 `json:"tag"`
	Message     string                 `json:"message"`
	Severity    ValidationSeverity     `json:"severity"`
	SecurityRisk bool                  `json:"security_risk"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ValidationSeverity indicates the severity of a validation error
type ValidationSeverity string

const (
	SeverityLow      ValidationSeverity = "low"
	SeverityMedium   ValidationSeverity = "medium"
	SeverityHigh     ValidationSeverity = "high"
	SeverityCritical ValidationSeverity = "critical"
)

// ValidationResult contains the result of validation
type ValidationResult struct {
	IsValid      bool              `json:"is_valid"`
	Errors       []ValidationError `json:"errors,omitempty"`
	Warnings     []ValidationError `json:"warnings,omitempty"`
	SecurityRisk bool              `json:"security_risk"`
	Score        int               `json:"score"` // 0-100, higher is better
}

// NewEnhancedValidator creates a new enhanced validator
func NewEnhancedValidator(config ValidationConfig, logger *logrus.Logger) *EnhancedValidator {
	v := validator.New()

	// Set default config values
	if config.MaxStringLength == 0 {
		config.MaxStringLength = 1000
	}
	if config.MaxFileSize == 0 {
		config.MaxFileSize = 10 * 1024 * 1024 // 10MB
	}
	if len(config.AllowedFileTypes) == 0 {
		config.AllowedFileTypes = []string{".jpg", ".jpeg", ".png", ".gif", ".pdf", ".doc", ".docx"}
	}

	ev := &EnhancedValidator{
		validator: v,
		logger:    logger,
		config:    config,
	}

	// Register custom validators
	ev.registerCustomValidators()

	return ev
}

// ValidateStruct validates a struct with comprehensive security checks
func (ev *EnhancedValidator) ValidateStruct(s interface{}) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
		Warnings: make([]ValidationError, 0),
		Score:   100,
	}

	// Standard validation
	err := ev.validator.Struct(s)
	if err != nil {
		result.IsValid = false
		
		for _, validationErr := range err.(validator.ValidationErrors) {
			valErr := ValidationError{
				Field:   validationErr.Field(),
				Value:   validationErr.Value(),
				Tag:     validationErr.Tag(),
				Message: ev.getErrorMessage(validationErr),
				Severity: ev.getSeverity(validationErr.Tag()),
			}

			// Check for security risks
			valErr.SecurityRisk = ev.isSecurityRisk(validationErr.Tag(), validationErr.Value())
			if valErr.SecurityRisk {
				result.SecurityRisk = true
			}

			result.Errors = append(result.Errors, valErr)
		}
	}

	// Additional security validations
	ev.performSecurityValidations(s, result)

	// Calculate security score
	result.Score = ev.calculateSecurityScore(result)

	ev.logger.WithFields(logrus.Fields{
		"is_valid":      result.IsValid,
		"error_count":   len(result.Errors),
		"warning_count": len(result.Warnings),
		"security_risk": result.SecurityRisk,
		"score":         result.Score,
	}).Debug("Validation completed")

	return result
}

// ValidateString validates a string with comprehensive security checks
func (ev *EnhancedValidator) ValidateString(value string, rules ...string) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
		Warnings: make([]ValidationError, 0),
		Score:   100,
	}

	// Length check
	if len(value) > ev.config.MaxStringLength {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "string",
			Value:    len(value),
			Tag:      "max_length",
			Message:  fmt.Sprintf("String length exceeds maximum allowed (%d)", ev.config.MaxStringLength),
			Severity: SeverityHigh,
		})
	}

	// SQL Injection check
	if ev.config.EnableSQLInjectionCheck && ev.containsSQLInjection(value) {
		result.IsValid = false
		result.SecurityRisk = true
		result.Errors = append(result.Errors, ValidationError{
			Field:        "string",
			Value:        "[REDACTED]",
			Tag:          "sql_injection",
			Message:      "Potential SQL injection detected",
			Severity:     SeverityCritical,
			SecurityRisk: true,
		})
	}

	// XSS check
	if ev.config.EnableXSSCheck && ev.containsXSS(value) {
		result.IsValid = false
		result.SecurityRisk = true
		result.Errors = append(result.Errors, ValidationError{
			Field:        "string",
			Value:        "[REDACTED]",
			Tag:          "xss",
			Message:      "Potential XSS attack detected",
			Severity:     SeverityCritical,
			SecurityRisk: true,
		})
	}

	// Path traversal check
	if ev.containsPathTraversal(value) {
		result.IsValid = false
		result.SecurityRisk = true
		result.Errors = append(result.Errors, ValidationError{
			Field:        "string",
			Value:        "[REDACTED]",
			Tag:          "path_traversal",
			Message:      "Potential path traversal attack detected",
			Severity:     SeverityCritical,
			SecurityRisk: true,
		})
	}

	// Apply custom rules
	for _, rule := range rules {
		if err := ev.applyStringRule(value, rule); err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:    "string",
				Value:    value,
				Tag:      rule,
				Message:  err.Error(),
				Severity: SeverityMedium,
			})
		}
	}

	result.Score = ev.calculateSecurityScore(result)
	return result
}

// ValidateEmail validates an email address with enhanced checks
func (ev *EnhancedValidator) ValidateEmail(email string) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
		Score:   100,
	}

	// Basic email validation
	_, err := mail.ParseAddress(email)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    email,
			Tag:      "email",
			Message:  "Invalid email format",
			Severity: SeverityMedium,
		})
		return result
	}

	// Additional validations
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    email,
			Tag:      "email_format",
			Message:  "Invalid email format",
			Severity: SeverityMedium,
		})
		return result
	}

	domain := parts[1]
	
	// Check for suspicious domains
	if ev.isSuspiciousDomain(domain) {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:    "email",
			Value:    domain,
			Tag:      "suspicious_domain",
			Message:  "Email domain appears suspicious",
			Severity: SeverityMedium,
		})
	}

	// Check domain length
	if len(domain) > 253 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "email",
			Value:    len(domain),
			Tag:      "domain_length",
			Message:  "Email domain too long",
			Severity: SeverityMedium,
		})
	}

	return result
}

// ValidateURL validates a URL with security checks
func (ev *EnhancedValidator) ValidateURL(urlStr string) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
		Score:   100,
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "url",
			Value:    urlStr,
			Tag:      "url",
			Message:  "Invalid URL format",
			Severity: SeverityMedium,
		})
		return result
	}

	// Check scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:    "url",
			Value:    parsedURL.Scheme,
			Tag:      "url_scheme",
			Message:  "Only HTTP and HTTPS schemes are allowed",
			Severity: SeverityHigh,
		})
	}

	// Check for localhost/private IPs (potential SSRF)
	if ev.isPrivateOrLocalhost(parsedURL.Hostname()) {
		result.SecurityRisk = true
		result.Errors = append(result.Errors, ValidationError{
			Field:        "url",
			Value:        parsedURL.Hostname(),
			Tag:          "ssrf",
			Message:      "URL points to private/localhost address",
			Severity:     SeverityCritical,
			SecurityRisk: true,
		})
	}

	return result
}

// RegisterCustomValidators registers custom validation tags
func (ev *EnhancedValidator) registerCustomValidators() {
	// Strong password validator
	ev.validator.RegisterValidation("strong_password", ev.validateStrongPassword)
	
	// Phone number validator
	ev.validator.RegisterValidation("phone", ev.validatePhoneNumber)
	
	// UUID validator
	ev.validator.RegisterValidation("uuid4", ev.validateUUID4)
	
	// Safe filename validator
	ev.validator.RegisterValidation("safe_filename", ev.validateSafeFilename)
	
	// No SQL injection validator
	ev.validator.RegisterValidation("no_sql_injection", ev.validateNoSQLInjection)
	
	// No XSS validator
	ev.validator.RegisterValidation("no_xss", ev.validateNoXSS)
}

// Custom validation functions

func (ev *EnhancedValidator) validateStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	
	if len(password) < 8 {
		return false
	}
	
	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false
	
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	
	return hasUpper && hasLower && hasDigit && hasSpecial
}

func (ev *EnhancedValidator) validatePhoneNumber(fl validator.FieldLevel) bool {
	phone := fl.Field().String()
	
	// Remove common separators
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	
	// Check if it starts with + and contains only digits
	if len(cleaned) < 10 || cleaned[0] != '+' {
		return false
	}
	
	for i := 1; i < len(cleaned); i++ {
		if cleaned[i] < '0' || cleaned[i] > '9' {
			return false
		}
	}
	
	return len(cleaned) >= 10 && len(cleaned) <= 15
}

func (ev *EnhancedValidator) validateUUID4(fl validator.FieldLevel) bool {
	uuid := fl.Field().String()
	uuidPattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	matched, _ := regexp.MatchString(uuidPattern, uuid)
	return matched
}

func (ev *EnhancedValidator) validateSafeFilename(fl validator.FieldLevel) bool {
	filename := fl.Field().String()
	
	// Check for dangerous characters
	dangerousChars := []string{"..", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range dangerousChars {
		if strings.Contains(filename, char) {
			return false
		}
	}
	
	// Check file extension
	if len(ev.config.AllowedFileTypes) > 0 {
		hasValidExtension := false
		for _, ext := range ev.config.AllowedFileTypes {
			if strings.HasSuffix(strings.ToLower(filename), ext) {
				hasValidExtension = true
				break
			}
		}
		return hasValidExtension
	}
	
	return true
}

func (ev *EnhancedValidator) validateNoSQLInjection(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return !ev.containsSQLInjection(value)
}

func (ev *EnhancedValidator) validateNoXSS(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return !ev.containsXSS(value)
}

// Security check methods

func (ev *EnhancedValidator) containsSQLInjection(value string) bool {
	sqlPatterns := []string{
		`(?i)\b(SELECT|INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|EXEC|UNION)\b`,
		`(?i)\b(OR|AND)\s+\d+\s*=\s*\d+`,
		`(?i)\b(OR|AND)\s+['"]\w*['"]?\s*=\s*['"]\w*['"]?`,
		`(?i)\b(UNION\s+SELECT|UNION\s+ALL)`,
		`(?i)(--|#|\*\/|\/\*)`,
		`(?i)\b(sp_|xp_|fn_)`,
		`(?i)\b(LOAD_FILE|INTO\s+OUTFILE|INTO\s+DUMPFILE)`,
	}
	
	for _, pattern := range sqlPatterns {
		matched, _ := regexp.MatchString(pattern, value)
		if matched {
			return true
		}
	}
	
	return false
}

func (ev *EnhancedValidator) containsXSS(value string) bool {
	xssPatterns := []string{
		`(?i)<script[^>]*>.*?</script>`,
		`(?i)<iframe[^>]*>.*?</iframe>`,
		`(?i)<object[^>]*>.*?</object>`,
		`(?i)<embed[^>]*>`,
		`(?i)<link[^>]*>`,
		`(?i)<meta[^>]*>`,
		`(?i)javascript:`,
		`(?i)vbscript:`,
		`(?i)onload\s*=`,
		`(?i)onerror\s*=`,
		`(?i)onclick\s*=`,
		`(?i)onmouseover\s*=`,
		`(?i)onfocus\s*=`,
		`(?i)onblur\s*=`,
		`(?i)expression\s*\(`,
		`(?i)@import`,
	}
	
	for _, pattern := range xssPatterns {
		matched, _ := regexp.MatchString(pattern, value)
		if matched {
			return true
		}
	}
	
	return false
}

func (ev *EnhancedValidator) containsPathTraversal(value string) bool {
	pathTraversalPatterns := []string{
		`\.\.\/`,
		`\.\.\\`,
		`\.\.%2F`,
		`\.\.%2f`,
		`\.\.%5C`,
		`\.\.%5c`,
		`%2e%2e%2f`,
		`%2e%2e%5c`,
	}
	
	for _, pattern := range pathTraversalPatterns {
		matched, _ := regexp.MatchString(pattern, value)
		if matched {
			return true
		}
	}
	
	return false
}

func (ev *EnhancedValidator) isPrivateOrLocalhost(hostname string) bool {
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return true
	}
	
	ip := net.ParseIP(hostname)
	if ip == nil {
		return false
	}
	
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

func (ev *EnhancedValidator) isSuspiciousDomain(domain string) bool {
	suspiciousDomains := []string{
		"tempmail", "10minutemail", "mailinator", "guerrillamail",
		"temp-mail", "throwaway", "disposable", "fake",
	}
	
	lowerDomain := strings.ToLower(domain)
	for _, suspicious := range suspiciousDomains {
		if strings.Contains(lowerDomain, suspicious) {
			return true
		}
	}
	
	return false
}

// Helper methods

func (ev *EnhancedValidator) getErrorMessage(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", err.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email address", err.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters long", err.Field(), err.Param())
	case "max":
		return fmt.Sprintf("%s must not exceed %s characters", err.Field(), err.Param())
	case "strong_password":
		return fmt.Sprintf("%s must contain at least 8 characters with uppercase, lowercase, digit, and special character", err.Field())
	case "phone":
		return fmt.Sprintf("%s must be a valid phone number", err.Field())
	case "uuid4":
		return fmt.Sprintf("%s must be a valid UUID", err.Field())
	case "safe_filename":
		return fmt.Sprintf("%s contains invalid characters or file type", err.Field())
	case "no_sql_injection":
		return fmt.Sprintf("%s contains potentially dangerous SQL patterns", err.Field())
	case "no_xss":
		return fmt.Sprintf("%s contains potentially dangerous XSS patterns", err.Field())
	default:
		return fmt.Sprintf("%s failed validation for %s", err.Field(), err.Tag())
	}
}

func (ev *EnhancedValidator) getSeverity(tag string) ValidationSeverity {
	switch tag {
	case "no_sql_injection", "no_xss", "ssrf":
		return SeverityCritical
	case "strong_password", "safe_filename":
		return SeverityHigh
	case "email", "phone", "uuid4":
		return SeverityMedium
	default:
		return SeverityLow
	}
}

func (ev *EnhancedValidator) isSecurityRisk(tag string, value interface{}) bool {
	securityTags := []string{"no_sql_injection", "no_xss", "ssrf", "path_traversal"}
	for _, secTag := range securityTags {
		if tag == secTag {
			return true
		}
	}
	return false
}

func (ev *EnhancedValidator) performSecurityValidations(s interface{}, result *ValidationResult) {
	// This would be implemented based on the specific struct type
	// For now, it's a placeholder for additional security validations
}

func (ev *EnhancedValidator) calculateSecurityScore(result *ValidationResult) int {
	score := 100
	
	for _, err := range result.Errors {
		switch err.Severity {
		case SeverityCritical:
			score -= 40
		case SeverityHigh:
			score -= 20
		case SeverityMedium:
			score -= 10
		case SeverityLow:
			score -= 5
		}
		
		if err.SecurityRisk {
			score -= 20
		}
	}
	
	for _, warning := range result.Warnings {
		switch warning.Severity {
		case SeverityHigh:
			score -= 5
		case SeverityMedium:
			score -= 3
		case SeverityLow:
			score -= 1
		}
	}
	
	if score < 0 {
		score = 0
	}
	
	return score
}

func (ev *EnhancedValidator) applyStringRule(value, rule string) error {
	switch rule {
	case "alphanumeric":
		for _, char := range value {
			if !unicode.IsLetter(char) && !unicode.IsNumber(char) {
				return fmt.Errorf("string must contain only alphanumeric characters")
			}
		}
	case "no_spaces":
		if strings.Contains(value, " ") {
			return fmt.Errorf("string must not contain spaces")
		}
	case "lowercase":
		if value != strings.ToLower(value) {
			return fmt.Errorf("string must be lowercase")
		}
	case "uppercase":
		if value != strings.ToUpper(value) {
			return fmt.Errorf("string must be uppercase")
		}
	}
	return nil
}

// Close closes the validator
func (ev *EnhancedValidator) Close() error {
	ev.logger.Info("Enhanced validator closed")
	return nil
}