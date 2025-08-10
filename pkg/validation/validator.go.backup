package validation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
)

// UnifiedValidator validador unificado para todos los servicios
type UnifiedValidator struct {
	validator *validator.Validate
	logger    *logrus.Entry
	config    *ValidationConfig
}

// ValidationConfig configuración del validador
type ValidationConfig struct {
	// String limits
	MaxStringLength     int
	MaxEmailLength      int
	MaxNameLength       int
	MaxPasswordLength   int
	MinPasswordLength   int
	MaxDescriptionLength int
	
	// Security settings
	EnableXSSProtection  bool
	EnableSQLInjection   bool
	EnableScriptCheck    bool
	EnablePathTraversal  bool
	
	// File validation
	AllowedFileTypes     []string
	MaxFileSize          int64
	MaxFileNameLength    int
	
	// Business rules
	AllowedSports        []string
	AllowedCurrencies    []string
	AllowedCountries     []string
	
	// Advanced settings
	EnableUnicodeNormalization bool
	StrictMode                 bool
	EnableCustomValidators     bool
}

// DefaultValidationConfig retorna configuración por defecto
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxStringLength:      255,
		MaxEmailLength:       320,
		MaxNameLength:        100,
		MaxPasswordLength:    128,
		MinPasswordLength:    8,
		MaxDescriptionLength: 1000,
		EnableXSSProtection:  true,
		EnableSQLInjection:   true,
		EnableScriptCheck:    true,
		EnablePathTraversal:  true,
		AllowedFileTypes:     []string{"jpg", "jpeg", "png", "gif", "pdf", "doc", "docx"},
		MaxFileSize:          10 * 1024 * 1024, // 10MB
		MaxFileNameLength:    255,
		AllowedSports:        []string{"tennis", "padel", "football", "basketball", "volleyball", "swimming", "golf", "running"},
		AllowedCurrencies:    []string{"EUR", "USD", "GBP", "JPY"},
		AllowedCountries:     []string{"ES", "US", "GB", "FR", "DE", "IT"},
		EnableUnicodeNormalization: true,
		StrictMode:                 false,
		EnableCustomValidators:     true,
	}
}

// NewUnifiedValidator crea un nuevo validador unificado
func NewUnifiedValidator(config *ValidationConfig, logger *logrus.Entry) *UnifiedValidator {
	if config == nil {
		config = DefaultValidationConfig()
	}

	v := validator.New()
	
	validator := &UnifiedValidator{
		validator: v,
		logger:    logger,
		config:    config,
	}

	// Register custom validators if enabled
	if config.EnableCustomValidators {
		validator.registerCustomValidators()
	}

	return validator
}

// ValidationResult resultado de validación
type ValidationResult struct {
	IsValid bool                    `json:"is_valid"`
	Errors  []ValidationError       `json:"errors,omitempty"`
	Field   string                  `json:"field,omitempty"`
	Value   interface{}             `json:"value,omitempty"`
	Issues  []string               `json:"issues,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ValidationError error de validación
type ValidationError struct {
	Field    string      `json:"field"`
	Value    interface{} `json:"value"`
	Tag      string      `json:"tag"`
	Message  string      `json:"message"`
	Code     string      `json:"code"`
	Severity string      `json:"severity"`
}

// ValidateStruct valida una estructura completa
func (v *UnifiedValidator) ValidateStruct(s interface{}) *ValidationResult {
	result := &ValidationResult{
		IsValid:  true,
		Errors:   make([]ValidationError, 0),
		Metadata: make(map[string]interface{}),
	}

	err := v.validator.Struct(s)
	if err != nil {
		result.IsValid = false
		
		// Handle validation errors
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, fieldError := range validationErrors {
				validationError := ValidationError{
					Field:    fieldError.Field(),
					Value:    fieldError.Value(),
					Tag:      fieldError.Tag(),
					Message:  v.getErrorMessage(fieldError),
					Code:     v.getErrorCode(fieldError),
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

// ValidateString valida una cadena de texto
func (v *UnifiedValidator) ValidateString(value, fieldName string, rules ...string) *ValidationResult {
	result := &ValidationResult{
		Field:   fieldName,
		Value:   value,
		IsValid: true,
		Issues:  make([]string, 0),
		Metadata: make(map[string]interface{}),
	}

	// Basic validations
	if len(value) > v.config.MaxStringLength {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("exceeds maximum length of %d", v.config.MaxStringLength))
	}

	// UTF-8 validation
	if !utf8.ValidString(value) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains invalid UTF-8 characters")
	}

	// Security validations
	if v.config.EnableXSSProtection && v.containsXSSPatterns(value) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains potential XSS patterns")
	}

	if v.config.EnableSQLInjection && v.containsSQLInjectionPatterns(value) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains potential SQL injection patterns")
	}

	if v.config.EnableScriptCheck && v.containsScriptPatterns(value) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains potential script injection patterns")
	}

	if v.config.EnablePathTraversal && v.containsPathTraversalPatterns(value) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains potential path traversal patterns")
	}

	// Apply custom rules
	for _, rule := range rules {
		if !v.applyStringRule(value, rule) {
			result.IsValid = false
			result.Issues = append(result.Issues, fmt.Sprintf("failed rule: %s", rule))
		}
	}

	return result
}

// ValidateEmail valida una dirección de email
func (v *UnifiedValidator) ValidateEmail(email string) *ValidationResult {
	result := &ValidationResult{
		Field:   "email",
		Value:   email,
		IsValid: true,
		Issues:  make([]string, 0),
	}

	// Basic checks
	if email == "" {
		result.IsValid = false
		result.Issues = append(result.Issues, "email is required")
		return result
	}

	if len(email) > v.config.MaxEmailLength {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("exceeds maximum length of %d", v.config.MaxEmailLength))
	}

	// Email format validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		result.IsValid = false
		result.Issues = append(result.Issues, "invalid email format")
	}

	// Security checks
	if v.containsSuspiciousPatterns(email) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains suspicious patterns")
	}

	return result
}

// ValidateName valida un nombre de persona
func (v *UnifiedValidator) ValidateName(name string) *ValidationResult {
	result := &ValidationResult{
		Field:   "name",
		Value:   name,
		IsValid: true,
		Issues:  make([]string, 0),
	}

	if name == "" {
		result.IsValid = false
		result.Issues = append(result.Issues, "name is required")
		return result
	}

	if len(name) > v.config.MaxNameLength {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("exceeds maximum length of %d", v.config.MaxNameLength))
	}

	// Check for valid name characters
	if !v.isValidName(name) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains invalid characters for a name")
	}

	// Security checks
	if v.config.EnableXSSProtection && v.containsXSSPatterns(name) {
		result.IsValid = false
		result.Issues = append(result.Issues, "contains potential XSS patterns")
	}

	return result
}

// ValidatePassword valida una contraseña
func (v *UnifiedValidator) ValidatePassword(password string) *ValidationResult {
	result := &ValidationResult{
		Field:   "password",
		Value:   "[REDACTED]", // Never expose password in results
		IsValid: true,
		Issues:  make([]string, 0),
	}

	if password == "" {
		result.IsValid = false
		result.Issues = append(result.Issues, "password is required")
		return result
	}

	if len(password) < v.config.MinPasswordLength {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("password must be at least %d characters", v.config.MinPasswordLength))
	}

	if len(password) > v.config.MaxPasswordLength {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("password cannot exceed %d characters", v.config.MaxPasswordLength))
	}

	// Password strength check
	if !v.isStrongPassword(password) {
		result.IsValid = false
		result.Issues = append(result.Issues, "password does not meet strength requirements")
	}

	return result
}

// ValidateUUID valida un UUID
func (v *UnifiedValidator) ValidateUUID(uuid, fieldName string) *ValidationResult {
	result := &ValidationResult{
		Field:   fieldName,
		Value:   uuid,
		IsValid: true,
		Issues:  make([]string, 0),
	}

	if uuid == "" {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("%s is required", fieldName))
		return result
	}

	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidRegex.MatchString(strings.ToLower(uuid)) {
		result.IsValid = false
		result.Issues = append(result.Issues, "invalid UUID format")
	}

	return result
}

// ValidateSport valida un deporte
func (v *UnifiedValidator) ValidateSport(sport string) *ValidationResult {
	result := &ValidationResult{
		Field:   "sport",
		Value:   sport,
		IsValid: true,
		Issues:  make([]string, 0),
	}

	if sport == "" {
		result.IsValid = false
		result.Issues = append(result.Issues, "sport is required")
		return result
	}

	sportLower := strings.ToLower(sport)
	allowed := false
	for _, allowedSport := range v.config.AllowedSports {
		if sportLower == allowedSport {
			allowed = true
			break
		}
	}

	if !allowed {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("sport '%s' is not supported. Allowed sports: %v", sport, v.config.AllowedSports))
	}

	return result
}

// ValidateCurrency valida una moneda
func (v *UnifiedValidator) ValidateCurrency(currency string) *ValidationResult {
	result := &ValidationResult{
		Field:   "currency",
		Value:   currency,
		IsValid: true,
		Issues:  make([]string, 0),
	}

	if currency == "" {
		result.IsValid = false
		result.Issues = append(result.Issues, "currency is required")
		return result
	}

	currencyUpper := strings.ToUpper(currency)
	allowed := false
	for _, allowedCurrency := range v.config.AllowedCurrencies {
		if currencyUpper == allowedCurrency {
			allowed = true
			break
		}
	}

	if !allowed {
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("currency '%s' is not supported. Allowed currencies: %v", currency, v.config.AllowedCurrencies))
	}

	return result
}

// SanitizeInput sanitiza una entrada removiendo caracteres peligrosos
func (v *UnifiedValidator) SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Remove or escape HTML tags
	input = v.escapeHTML(input)

	// Remove control characters except tab, newline, and carriage return
	result := strings.Builder{}
	for _, r := range input {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

// Helper methods

func (v *UnifiedValidator) registerCustomValidators() {
	// Register custom UUID validator
	v.validator.RegisterValidation("uuid", func(fl validator.FieldLevel) bool {
		uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
		return uuidRegex.MatchString(strings.ToLower(fl.Field().String()))
	})

	// Register custom sport validator
	v.validator.RegisterValidation("sport", func(fl validator.FieldLevel) bool {
		sport := strings.ToLower(fl.Field().String())
		for _, allowedSport := range v.config.AllowedSports {
			if sport == allowedSport {
				return true
			}
		}
		return false
	})

	// Register custom currency validator
	v.validator.RegisterValidation("currency", func(fl validator.FieldLevel) bool {
		currency := strings.ToUpper(fl.Field().String())
		for _, allowedCurrency := range v.config.AllowedCurrencies {
			if currency == allowedCurrency {
				return true
			}
		}
		return false
	})

	// Register strong password validator
	v.validator.RegisterValidation("strong_password", func(fl validator.FieldLevel) bool {
		return v.isStrongPassword(fl.Field().String())
	})
}

func (v *UnifiedValidator) getErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters long", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s cannot be longer than %s characters", fe.Field(), fe.Param())
	case "uuid":
		return fmt.Sprintf("%s must be a valid UUID", fe.Field())
	case "sport":
		return fmt.Sprintf("%s must be a valid sport", fe.Field())
	case "currency":
		return fmt.Sprintf("%s must be a valid currency", fe.Field())
	case "strong_password":
		return fmt.Sprintf("%s must meet password strength requirements", fe.Field())
	default:
		return fmt.Sprintf("%s failed validation for %s", fe.Field(), fe.Tag())
	}
}

func (v *UnifiedValidator) getErrorCode(fe validator.FieldError) string {
	return fmt.Sprintf("VALIDATION_%s_%s", strings.ToUpper(fe.Field()), strings.ToUpper(fe.Tag()))
}

func (v *UnifiedValidator) getErrorSeverity(fe validator.FieldError) string {
	securityTags := []string{"xss", "sql_injection", "script_injection", "path_traversal"}
	for _, tag := range securityTags {
		if fe.Tag() == tag {
			return "high"
		}
	}
	
	if fe.Tag() == "required" {
		return "medium"
	}
	
	return "low"
}

func (v *UnifiedValidator) validateSecurity(s interface{}) *ValidationResult {
	// This would perform deep security validation on the struct
	// For now, return empty result
	return &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}
}

func (v *UnifiedValidator) applyStringRule(value, rule string) bool {
	switch rule {
	case "no_spaces":
		return !strings.Contains(value, " ")
	case "alphanumeric":
		return v.isAlphanumeric(value)
	case "alpha":
		return v.isAlpha(value)
	case "numeric":
		return v.isNumeric(value)
	default:
		return true
	}
}

func (v *UnifiedValidator) isValidName(name string) bool {
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsSpace(r) && r != '-' && r != '\'' && r != '.' {
			return false
		}
	}
	return true
}

func (v *UnifiedValidator) isStrongPassword(password string) bool {
	if len(password) < 8 {
		return false
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasDigit && hasSpecial
}

func (v *UnifiedValidator) isAlphanumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (v *UnifiedValidator) isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func (v *UnifiedValidator) isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (v *UnifiedValidator) containsXSSPatterns(s string) bool {
	xssPatterns := []string{
		"<script", "</script>", "<iframe", "</iframe>",
		"<object", "</object>", "<embed", "</embed>",
		"<form", "</form>", "<input", "<button",
		"javascript:", "vbscript:", "data:",
		"onload=", "onerror=", "onclick=", "onmouseover=", 
		"document.cookie", "document.write", "innerHTML",
		"eval(", "setTimeout(", "setInterval(",
	}

	sLower := strings.ToLower(s)
	for _, pattern := range xssPatterns {
		if strings.Contains(sLower, pattern) {
			return true
		}
	}
	return false
}

func (v *UnifiedValidator) containsSQLInjectionPatterns(s string) bool {
	sqlPatterns := []string{
		"' or '1'='1", "' or 1=1", "' or true",
		"union select", "insert into", "delete from",
		"update set", "drop table", "drop database",
		"exec(", "execute(", "sp_", "xp_",
		"--", "/*", "*/", "@@",
		"char(", "nchar(", "varchar(", "nvarchar(",
		"waitfor delay", "benchmark(", "sleep(",
	}

	sLower := strings.ToLower(s)
	for _, pattern := range sqlPatterns {
		if strings.Contains(sLower, pattern) {
			return true
		}
	}
	return false
}

func (v *UnifiedValidator) containsScriptPatterns(s string) bool {
	scriptPatterns := []string{
		"<script", "</script>", "javascript:", "vbscript:",
		"<iframe", "<object", "<embed", "<form",
		"eval(", "setTimeout(", "setInterval(",
		"Function(", "constructor(", "prototype.",
		"__proto__", "document.", "window.",
		"location.", "history.", "navigator.",
	}

	sLower := strings.ToLower(s)
	for _, pattern := range scriptPatterns {
		if strings.Contains(sLower, pattern) {
			return true
		}
	}
	return false
}

func (v *UnifiedValidator) containsPathTraversalPatterns(s string) bool {
	pathPatterns := []string{
		"../", "..\\", "./", ".\\",
		"%2e%2e", "%2e%2e%2f", "%2e%2e%5c",
		"..%2f", "..%5c", "%252e%252e",
	}

	sLower := strings.ToLower(s)
	for _, pattern := range pathPatterns {
		if strings.Contains(sLower, pattern) {
			return true
		}
	}
	return false
}

func (v *UnifiedValidator) containsSuspiciousPatterns(s string) bool {
	return v.containsXSSPatterns(s) || 
		   v.containsSQLInjectionPatterns(s) || 
		   v.containsScriptPatterns(s) || 
		   v.containsPathTraversalPatterns(s)
}

func (v *UnifiedValidator) escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}