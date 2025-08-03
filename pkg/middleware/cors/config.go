package cors

import (
	"strings"
	"time"
)

// Config configura las opciones de CORS de manera segura
type Config struct {
	AllowedOrigins     []string      `json:"allowed_origins"`
	AllowedMethods     []string      `json:"allowed_methods"`
	AllowedHeaders     []string      `json:"allowed_headers"`
	ExposedHeaders     []string      `json:"exposed_headers"`
	AllowCredentials   bool          `json:"allow_credentials"`
	MaxAge             time.Duration `json:"max_age"`
	IsDevelopment      bool          `json:"is_development"`
	AllowAllOrigins    bool          `json:"allow_all_origins"` // Solo para desarrollo
	PreflightContinue  bool          `json:"preflight_continue"`
	OptionsPassthrough bool          `json:"options_passthrough"`
}

// DefaultConfig retorna configuración CORS segura por defecto
func DefaultConfig() *Config {
	return &Config{
		AllowedOrigins: []string{
			"https://app.clubpulse.com",
			"https://admin.clubpulse.com",
			"https://api.clubpulse.com",
		},
		AllowedMethods: []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH",
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"Content-Length",
			"X-CSRF-Token",
			"X-Requested-With",
			"X-Correlation-ID",
			"X-Tenant-ID",
			"X-Request-ID",
			"Cache-Control",
			"Accept-Encoding",
			"Origin",
		},
		ExposedHeaders: []string{
			"X-Correlation-ID",
			"X-Request-ID",
			"X-Total-Count",
			"X-Page-Count",
		},
		AllowCredentials:   true,
		MaxAge:             12 * time.Hour,
		IsDevelopment:      false,
		AllowAllOrigins:    false,
		PreflightContinue:  false,
		OptionsPassthrough: false,
	}
}

// DevelopmentConfig retorna configuración CORS para desarrollo
func DevelopmentConfig() *Config {
	config := DefaultConfig()
	config.IsDevelopment = true
	config.AllowAllOrigins = true
	config.AllowedOrigins = []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:5173",
		"http://127.0.0.1:3000",
		"http://127.0.0.1:3001",
		"http://127.0.0.1:5173",
	}
	return config
}

// ProductionConfig retorna configuración CORS estricta para producción
func ProductionConfig(domains []string) *Config {
	config := DefaultConfig()
	config.IsDevelopment = false
	config.AllowAllOrigins = false
	
	if len(domains) > 0 {
		config.AllowedOrigins = domains
	}
	
	// Configuración más estricta para producción
	config.MaxAge = 1 * time.Hour
	config.AllowCredentials = true
	
	return config
}

// IsOriginAllowed verifica si un origen está permitido
func (c *Config) IsOriginAllowed(origin string) bool {
	if c.AllowAllOrigins && c.IsDevelopment {
		return true
	}
	
	origin = strings.ToLower(strings.TrimSpace(origin))
	
	for _, allowedOrigin := range c.AllowedOrigins {
		if strings.ToLower(allowedOrigin) == origin {
			return true
		}
		
		// Soporte para wildcards (solo en desarrollo)
		if c.IsDevelopment && strings.Contains(allowedOrigin, "*") {
			if matchWildcard(allowedOrigin, origin) {
				return true
			}
		}
	}
	
	return false
}

// matchWildcard verifica si un patrón con wildcard coincide con el origen
func matchWildcard(pattern, origin string) bool {
	if !strings.Contains(pattern, "*") {
		return strings.ToLower(pattern) == strings.ToLower(origin)
	}
	
	// Implementación simple de wildcard matching
	parts := strings.Split(strings.ToLower(pattern), "*")
	if len(parts) != 2 {
		return false
	}
	
	prefix, suffix := parts[0], parts[1]
	origin = strings.ToLower(origin)
	
	return strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix)
}

// Validate valida la configuración CORS
func (c *Config) Validate() error {
	// En producción, no permitir AllowAllOrigins
	if !c.IsDevelopment && c.AllowAllOrigins {
		return ErrUnsafeProductionConfig
	}
	
	// Verificar que hay al menos un origen permitido
	if !c.AllowAllOrigins && len(c.AllowedOrigins) == 0 {
		return ErrNoOriginsAllowed
	}
	
	// Verificar métodos válidos
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true,
		"HEAD": true, "OPTIONS": true, "PATCH": true, "TRACE": true,
	}
	
	for _, method := range c.AllowedMethods {
		if !validMethods[strings.ToUpper(method)] {
			return NewErrInvalidMethod(method)
		}
	}
	
	return nil
}