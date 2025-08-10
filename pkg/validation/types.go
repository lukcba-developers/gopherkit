package validation

import (
	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
)

// UnifiedValidator validador unificado para todos los servicios
type UnifiedValidator struct {
	validator *validator.Validate
	logger    *logrus.Entry
	config    *ValidationConfig
}

// ValidationResult represents the result of a validation operation
type ValidationResult struct {
	IsValid bool               `json:"is_valid"`
	Errors  []ValidationError  `json:"errors,omitempty"`
	Value   interface{}        `json:"value,omitempty"`
	Field   string            `json:"field,omitempty"`
}

// ValidationError represents a single validation error
type ValidationError struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	Code     string `json:"code"`
	Value    string `json:"value"`
	Severity string `json:"severity"`
}

// NewUnifiedValidator creates a new unified validator instance
func NewUnifiedValidator(config *ValidationConfig, logger *logrus.Entry) *UnifiedValidator {
	if config == nil {
		config = DefaultValidationConfig()
	}
	
	if logger == nil {
		logger = logrus.NewEntry(logrus.New())
	}
	
	v := &UnifiedValidator{
		validator: validator.New(),
		logger:    logger,
		config:    config,
	}
	
	if config.EnableCustomValidators {
		v.registerCustomValidators()
	}
	
	return v
}