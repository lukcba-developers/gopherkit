package validation

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

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxStringLength:      500,
		MaxEmailLength:       255,
		MaxNameLength:        100,
		MaxPasswordLength:    128,
		MinPasswordLength:    8,
		MaxDescriptionLength: 2000,
		
		EnableXSSProtection:  true,
		EnableSQLInjection:   true,
		EnableScriptCheck:    true,
		EnablePathTraversal:  true,
		
		AllowedFileTypes:     []string{"jpg", "jpeg", "png", "gif", "pdf", "doc", "docx"},
		MaxFileSize:          10 * 1024 * 1024, // 10MB
		MaxFileNameLength:    255,
		
		AllowedSports: []string{
			"football", "soccer", "basketball", "tennis", "padel", "golf",
			"swimming", "running", "cycling", "volleyball", "baseball",
			"hockey", "cricket", "rugby", "badminton", "table-tennis",
		},
		
		AllowedCurrencies: []string{
			"USD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "CNY",
			"SEK", "NZD", "MXN", "SGD", "HKD", "NOK", "TRY", "RUB",
			"INR", "BRL", "ZAR", "ARS", "CLP", "COP", "PEN", "UYU",
		},
		
		AllowedCountries: []string{
			"US", "CA", "GB", "DE", "FR", "IT", "ES", "NL", "BE", "CH",
			"AT", "SE", "NO", "DK", "FI", "IE", "PT", "GR", "PL", "CZ",
			"HU", "RO", "BG", "HR", "SI", "SK", "LT", "LV", "EE", "LU",
			"MT", "CY", "AU", "NZ", "JP", "KR", "SG", "HK", "TW", "MY",
			"TH", "PH", "ID", "VN", "IN", "CN", "MX", "BR", "AR", "CL",
			"CO", "PE", "UY", "PY", "BO", "EC", "VE", "GY", "SR", "FK",
		},
		
		EnableUnicodeNormalization: true,
		StrictMode:                 false,
		EnableCustomValidators:     true,
	}
}