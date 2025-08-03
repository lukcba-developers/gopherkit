package cors

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Middleware crea el middleware CORS con la configuración especificada
func Middleware(config *Config) gin.HandlerFunc {
	// Validar configuración
	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("Invalid CORS configuration: %v", err))
	}

	return gin.HandlerFunc(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		method := c.Request.Method

		// Si no hay Origin header, continuar sin CORS headers
		if origin == "" {
			if method == "OPTIONS" && !config.OptionsPassthrough {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.Next()
			return
		}

		// Verificar si el origen está permitido
		if !config.IsOriginAllowed(origin) {
			logrus.WithFields(logrus.Fields{
				"origin":     origin,
				"user_agent": c.Request.Header.Get("User-Agent"),
				"remote_ip":  c.ClientIP(),
			}).Warn("CORS: Origin not allowed")
			
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "CORS policy violation",
				"message": "Origin not allowed",
				"code":    "CORS_ORIGIN_NOT_ALLOWED",
			})
			return
		}

		// Establecer headers CORS
		setCORSHeaders(c, config, origin)

		// Manejar preflight requests (OPTIONS)
		if method == "OPTIONS" {
			handlePreflightRequest(c, config)
			if !config.PreflightContinue {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}

		// Log de CORS para debugging en desarrollo
		if config.IsDevelopment {
			logrus.WithFields(logrus.Fields{
				"origin": origin,
				"method": method,
				"path":   c.Request.URL.Path,
			}).Debug("CORS: Request allowed")
		}

		c.Next()
	})
}

// setCORSHeaders establece los headers CORS apropiados
func setCORSHeaders(c *gin.Context, config *Config, origin string) {
	// Access-Control-Allow-Origin
	if config.AllowAllOrigins && config.IsDevelopment {
		c.Header("Access-Control-Allow-Origin", "*")
	} else {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
	}

	// Access-Control-Allow-Credentials
	if config.AllowCredentials {
		c.Header("Access-Control-Allow-Credentials", "true")
	}

	// Access-Control-Expose-Headers
	if len(config.ExposedHeaders) > 0 {
		c.Header("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
	}
}

// handlePreflightRequest maneja las solicitudes preflight OPTIONS
func handlePreflightRequest(c *gin.Context, config *Config) {
	// Access-Control-Allow-Methods
	if len(config.AllowedMethods) > 0 {
		c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
	}

	// Access-Control-Allow-Headers
	requestedHeaders := c.Request.Header.Get("Access-Control-Request-Headers")
	if requestedHeaders != "" {
		// Verificar headers solicitados contra headers permitidos
		if isHeadersAllowed(requestedHeaders, config.AllowedHeaders) {
			c.Header("Access-Control-Allow-Headers", requestedHeaders)
		} else {
			// Si no todos los headers están permitidos, solo permitir los configurados
			c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
		}
	} else if len(config.AllowedHeaders) > 0 {
		c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
	}

	// Access-Control-Max-Age
	if config.MaxAge > 0 {
		c.Header("Access-Control-Max-Age", strconv.Itoa(int(config.MaxAge.Seconds())))
	}

	logrus.WithFields(logrus.Fields{
		"origin":            c.Request.Header.Get("Origin"),
		"requested_method":  c.Request.Header.Get("Access-Control-Request-Method"),
		"requested_headers": requestedHeaders,
	}).Debug("CORS: Preflight request handled")
}

// isHeadersAllowed verifica si todos los headers solicitados están permitidos
func isHeadersAllowed(requestedHeaders string, allowedHeaders []string) bool {
	if requestedHeaders == "" {
		return true
	}

	// Crear mapa de headers permitidos para búsqueda rápida
	allowedMap := make(map[string]bool)
	for _, header := range allowedHeaders {
		allowedMap[strings.ToLower(strings.TrimSpace(header))] = true
	}

	// Verificar cada header solicitado
	requested := strings.Split(requestedHeaders, ",")
	for _, header := range requested {
		normalized := strings.ToLower(strings.TrimSpace(header))
		if normalized != "" && !allowedMap[normalized] {
			return false
		}
	}

	return true
}

// Default crea middleware CORS con configuración por defecto
func Default() gin.HandlerFunc {
	return Middleware(DefaultConfig())
}

// Development crea middleware CORS para desarrollo
func Development() gin.HandlerFunc {
	return Middleware(DevelopmentConfig())
}

// Production crea middleware CORS para producción con dominios específicos
func Production(domains []string) gin.HandlerFunc {
	return Middleware(ProductionConfig(domains))
}

// New es un alias para Middleware para compatibilidad
func New(config *Config) gin.HandlerFunc {
	return Middleware(config)
}