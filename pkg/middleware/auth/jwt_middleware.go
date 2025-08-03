package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"github.com/lukcba-developers/gopherkit/pkg/errors"
)

// JWTConfig configura el middleware JWT
type JWTConfig struct {
	SecretKey      string
	TokenLookup    string // "header:Authorization" o "query:token"
	TokenHeadName  string // "Bearer"
	SkipperFunc    func(*gin.Context) bool
	ClaimsFunc     func(*gin.Context, jwt.MapClaims)
}

// DefaultJWTConfig retorna configuración por defecto
func DefaultJWTConfig(secretKey string) *JWTConfig {
	return &JWTConfig{
		SecretKey:     secretKey,
		TokenLookup:   "header:Authorization",
		TokenHeadName: "Bearer",
		SkipperFunc:   nil,
		ClaimsFunc:    nil,
	}
}

// JWTMiddleware crea middleware de autenticación JWT
func JWTMiddleware(config *JWTConfig) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Skipper function
		if config.SkipperFunc != nil && config.SkipperFunc(c) {
			c.Next()
			return
		}

		// Extract token
		token := extractToken(c, config)
		if token == "" {
			c.JSON(http.StatusUnauthorized, errors.NewAuthenticationError("missing token").ToHTTPResponse())
			c.Abort()
			return
		}

		// Parse and validate token
		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(config.SecretKey), nil
		})

		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
				"token": token[:min(len(token), 20)] + "...",
			}).Warn("JWT validation failed")
			
			c.JSON(http.StatusUnauthorized, errors.NewInvalidTokenError("JWT").ToHTTPResponse())
			c.Abort()
			return
		}

		if !parsedToken.Valid {
			c.JSON(http.StatusUnauthorized, errors.NewInvalidTokenError("JWT").ToHTTPResponse())
			c.Abort()
			return
		}

		// Extract claims
		if claims, ok := parsedToken.Claims.(jwt.MapClaims); ok {
			// Store claims in context
			c.Set("jwt_claims", claims)
			
			// Optional custom claims function
			if config.ClaimsFunc != nil {
				config.ClaimsFunc(c, claims)
			}
			
			// Extract common fields
			if userID, ok := claims["user_id"].(string); ok {
				c.Set("user_id", userID)
			}
			if tenantID, ok := claims["tenant_id"].(string); ok {
				c.Set("tenant_id", tenantID)
			}
		}

		c.Next()
	})
}

func extractToken(c *gin.Context, config *JWTConfig) string {
	parts := strings.Split(config.TokenLookup, ":")
	if len(parts) != 2 {
		return ""
	}

	switch parts[0] {
	case "header":
		authHeader := c.GetHeader(parts[1])
		if authHeader == "" {
			return ""
		}
		
		if config.TokenHeadName != "" {
			prefix := config.TokenHeadName + " "
			if strings.HasPrefix(authHeader, prefix) {
				return authHeader[len(prefix):]
			}
			return ""
		}
		return authHeader
		
	case "query":
		return c.Query(parts[1])
		
	default:
		return ""
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}