package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/lukcba-developers/gopherkit/pkg/errors"
	"github.com/lukcba-developers/gopherkit/pkg/cache"
)

// Claims representa los claims del JWT token
type Claims struct {
	UserID       string   `json:"sub"`
	TenantID     string   `json:"tenant_id"`
	Role         string   `json:"role"`
	Permissions  []string `json:"permissions,omitempty"`
	Email        string   `json:"email"`
	SessionID    string   `json:"session_id,omitempty"`
	TokenType    string   `json:"token_type"`
	jwt.RegisteredClaims
}

// JWTConfig configuración del middleware JWT mejorada
type JWTConfig struct {
	// Token configuration
	SecretKey       string
	TokenLookup     string        // "header:Authorization,query:token,cookie:jwt"
	TokenHeadName   string        // "Bearer"
	TokenExpiration time.Duration
	
	// Validation options
	SkipPaths       []string
	RequiredClaims  []string
	AllowedIssuers  []string
	
	// Cache configuration for token blacklist/validation
	CacheClient cache.CacheInterface
	CachePrefix string
	CacheTTL    time.Duration
	
	// Security options
	EnableBlacklist    bool
	MaxTokensPerUser   int
	
	// Custom functions
	SkipperFunc     func(*gin.Context) bool
	ClaimsFunc      func(*gin.Context, *Claims)
	ErrorHandler    func(*gin.Context, error)
}

// DefaultJWTConfig retorna configuración por defecto mejorada
func DefaultJWTConfig(secretKey string) *JWTConfig {
	return &JWTConfig{
		SecretKey:       secretKey,
		TokenLookup:     "header:Authorization",
		TokenHeadName:   "Bearer",
		TokenExpiration: 24 * time.Hour,
		RequiredClaims:  []string{"sub", "tenant_id"},
		CachePrefix:     "jwt:",
		CacheTTL:        24 * time.Hour,
		EnableBlacklist: true,
		MaxTokensPerUser: 5,
	}
}

// JWTMiddleware crea middleware de autenticación JWT mejorado
func JWTMiddleware(config *JWTConfig) gin.HandlerFunc {
	tracer := otel.Tracer("jwt-middleware")
	
	return gin.HandlerFunc(func(c *gin.Context) {
		ctx, span := tracer.Start(c.Request.Context(), "auth.jwt.validate")
		defer span.End()
		
		// Skip authentication for excluded paths
		if shouldSkipPath(c.Request.URL.Path, config.SkipPaths) || 
		   (config.SkipperFunc != nil && config.SkipperFunc(c)) {
			span.SetAttributes(attribute.Bool("skipped", true))
			c.Next()
			return
		}
		
		// Extract token from request
		tokenString, err := extractToken(c, config)
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			handleAuthError(c, config, errors.NewDomainError(
				"TOKEN_EXTRACTION_FAILED",
				"Failed to extract token from request",
				errors.CategorySecurity,
				errors.SeverityMedium,
			).WithCause(err))
			return
		}
		
		if tokenString == "" {
			span.SetAttributes(attribute.Bool("error", true))
			handleAuthError(c, config, errors.NewDomainError(
				"TOKEN_MISSING",
				"Authentication token is required",
				errors.CategorySecurity,
				errors.SeverityMedium,
			))
			return
		}
		
		// Validate token
		claims, err := validateToken(ctx, tokenString, config)
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			handleAuthError(c, config, err)
			return
		}
		
		// Check if token is blacklisted
		if config.EnableBlacklist && config.CacheClient != nil {
			if isBlacklisted(ctx, claims.SessionID, config) {
				span.SetAttributes(attribute.Bool("blacklisted", true))
				handleAuthError(c, config, errors.NewDomainError(
					"TOKEN_BLACKLISTED",
					"Token has been revoked",
					errors.CategorySecurity,
					errors.SeverityHigh,
				))
				return
			}
		}
		
		// Set claims in context
		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("tenant_id", claims.TenantID)
		c.Set("role", claims.Role)
		c.Set("permissions", claims.Permissions)
		c.Set("session_id", claims.SessionID)
		
		// Call custom claims function
		if config.ClaimsFunc != nil {
			config.ClaimsFunc(c, claims)
		}
		
		span.SetAttributes(
			attribute.String("user.id", claims.UserID),
			attribute.String("user.tenant_id", claims.TenantID),
			attribute.String("user.role", claims.Role),
			attribute.Bool("success", true),
		)
		
		c.Next()
	})
}

// extractToken extrae el token de la request según la configuración
func extractToken(c *gin.Context, config *JWTConfig) (string, error) {
	methods := strings.Split(config.TokenLookup, ",")
	
	for _, method := range methods {
		parts := strings.Split(strings.TrimSpace(method), ":")
		if len(parts) != 2 {
			continue
		}
		
		switch parts[0] {
		case "header":
			token := c.GetHeader(parts[1])
			if token != "" {
				if config.TokenHeadName != "" {
					prefix := config.TokenHeadName + " "
					if strings.HasPrefix(token, prefix) {
						return strings.TrimSpace(token[len(prefix):]), nil
					}
				} else {
					return token, nil
				}
			}
			
		case "query":
			token := c.Query(parts[1])
			if token != "" {
				return token, nil
			}
			
		case "cookie":
			token, err := c.Cookie(parts[1])
			if err == nil && token != "" {
				return token, nil
			}
		}
	}
	
	return "", nil
}

// validateToken valida el JWT token y retorna los claims
func validateToken(ctx context.Context, tokenString string, config *JWTConfig) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.SecretKey), nil
	})
	
	if err != nil {
		return nil, errors.NewDomainError(
			"TOKEN_INVALID",
			"Failed to parse JWT token",
			errors.CategorySecurity,
			errors.SeverityMedium,
		).WithCause(err)
	}
	
	if !token.Valid {
		return nil, errors.NewDomainError(
			"TOKEN_INVALID",
			"JWT token is not valid",
			errors.CategorySecurity,
			errors.SeverityMedium,
		)
	}
	
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.NewDomainError(
			"CLAIMS_INVALID",
			"Failed to parse token claims",
			errors.CategorySecurity,
			errors.SeverityMedium,
		)
	}
	
	// Validate required claims
	if err := validateRequiredClaims(claims, config.RequiredClaims); err != nil {
		return nil, err
	}
	
	return claims, nil
}

// validateRequiredClaims verifica que los claims requeridos estén presentes
func validateRequiredClaims(claims *Claims, required []string) error {
	for _, claim := range required {
		switch claim {
		case "sub":
			if claims.UserID == "" {
				return errors.NewDomainError(
					"CLAIM_MISSING_SUB",
					"Subject claim is required",
					errors.CategorySecurity,
					errors.SeverityMedium,
				)
			}
		case "tenant_id":
			if claims.TenantID == "" {
				return errors.NewDomainError(
					"CLAIM_MISSING_TENANT",
					"Tenant ID claim is required",
					errors.CategorySecurity,
					errors.SeverityMedium,
				)
			}
		case "role":
			if claims.Role == "" {
				return errors.NewDomainError(
					"CLAIM_MISSING_ROLE",
					"Role claim is required",
					errors.CategorySecurity,
					errors.SeverityMedium,
				)
			}
		}
	}
	return nil
}

// isBlacklisted verifica si el token está en la blacklist
func isBlacklisted(ctx context.Context, sessionID string, config *JWTConfig) bool {
	if sessionID == "" || config.CacheClient == nil {
		return false
	}
	
	key := fmt.Sprintf("%sblacklist:%s", config.CachePrefix, sessionID)
	var dummy string
	err := config.CacheClient.Get(ctx, key, &dummy)
	if err != nil {
		// If error (key not found), token is not blacklisted
		return false
	}
	
	return true
}

// shouldSkipPath verifica si la ruta debe omitir autenticación
func shouldSkipPath(path string, skipPaths []string) bool {
	for _, skipPath := range skipPaths {
		if path == skipPath || strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// handleAuthError maneja errores de autenticación
func handleAuthError(c *gin.Context, config *JWTConfig, err error) {
	if config.ErrorHandler != nil {
		config.ErrorHandler(c, err)
		return
	}
	
	// Default error handling
	if domainErr, ok := err.(*errors.DomainError); ok {
		response := domainErr.ToHTTPResponse()
		c.AbortWithStatusJSON(http.StatusUnauthorized, response)
	} else {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication failed",
			"message": err.Error(),
			"code": "AUTH_FAILED",
		})
	}
}

// GetClaimsFromContext extrae los claims del contexto de Gin
func GetClaimsFromContext(c *gin.Context) (*Claims, error) {
	value, exists := c.Get("claims")
	if !exists {
		return nil, errors.NewDomainError(
			"CLAIMS_NOT_FOUND",
			"No authentication claims found in context",
			errors.CategorySecurity,
			errors.SeverityMedium,
		)
	}
	
	claims, ok := value.(*Claims)
	if !ok {
		return nil, errors.NewDomainError(
			"CLAIMS_INVALID_TYPE",
			"Authentication claims have invalid type",
			errors.CategoryTechnical,
			errors.SeverityMedium,
		)
	}
	
	return claims, nil
}

// RequireRole middleware que requiere un rol específico
func RequireRole(role string) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		claims, err := GetClaimsFromContext(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code": "AUTH_REQUIRED",
			})
			return
		}
		
		if claims.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": fmt.Sprintf("Role '%s' is required", role),
				"code": "INSUFFICIENT_PRIVILEGES",
				"required_role": role,
				"user_role": claims.Role,
			})
			return
		}
		
		c.Next()
	})
}