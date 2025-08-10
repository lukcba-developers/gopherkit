package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/lukcba-developers/gopherkit/pkg/config"
	"github.com/lukcba-developers/gopherkit/pkg/logger"
)

// JWTClaims represents the JWT claims structure
type JWTClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TenantID  string `json:"tenant_id"`
	SessionID string `json:"session_id"`
	jwt.RegisteredClaims
}

// JWTMiddleware provides JWT authentication middleware
type JWTMiddleware struct {
	config config.JWTConfig
	logger logger.Logger
}

// NewJWTMiddleware creates a new JWT middleware
func NewJWTMiddleware(config config.JWTConfig, logger logger.Logger) *JWTMiddleware {
	return &JWTMiddleware{
		config: config,
		logger: logger,
	}
}

// RequireAuth middleware that requires valid JWT token
func (jm *JWTMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := jm.extractToken(c)
		if err != nil {
			jm.logger.LogSecurityEvent(c.Request.Context(), "missing_auth_token", map[string]interface{}{
				"error":     err.Error(),
				"client_ip": c.ClientIP(),
				"endpoint":  c.FullPath(),
			})
			c.JSON(401, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		claims, err := jm.validateToken(token)
		if err != nil {
			jm.logger.LogSecurityEvent(c.Request.Context(), "invalid_auth_token", map[string]interface{}{
				"error":     err.Error(),
				"client_ip": c.ClientIP(),
				"endpoint":  c.FullPath(),
			})
			c.JSON(401, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Set user context
		jm.setUserContext(c, claims)
		c.Next()
	}
}

// OptionalAuth middleware that validates JWT token if present
func (jm *JWTMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := jm.extractToken(c)
		if err != nil {
			// No token present, continue without auth
			c.Next()
			return
		}

		claims, err := jm.validateToken(token)
		if err != nil {
			jm.logger.LogSecurityEvent(c.Request.Context(), "invalid_optional_token", map[string]interface{}{
				"error":     err.Error(),
				"client_ip": c.ClientIP(),
			})
			// Invalid token, continue without auth
			c.Next()
			return
		}

		// Set user context if token is valid
		jm.setUserContext(c, claims)
		c.Next()
	}
}

// RequireRole middleware that requires specific role
func (jm *JWTMiddleware) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("user_role")
		if !exists {
			jm.logger.LogSecurityEvent(c.Request.Context(), "missing_user_role", map[string]interface{}{
				"client_ip": c.ClientIP(),
				"endpoint":  c.FullPath(),
			})
			c.JSON(403, gin.H{"error": "Role information missing"})
			c.Abort()
			return
		}

		role, ok := userRole.(string)
		if !ok {
			c.JSON(403, gin.H{"error": "Invalid role format"})
			c.Abort()
			return
		}

		// Check if user has required role
		hasRole := false
		for _, requiredRole := range roles {
			if role == requiredRole {
				hasRole = true
				break
			}
		}

		if !hasRole {
			jm.logger.LogSecurityEvent(c.Request.Context(), "insufficient_permissions", map[string]interface{}{
				"user_role":      role,
				"required_roles": roles,
				"user_id":        c.GetString("user_id"),
				"client_ip":      c.ClientIP(),
				"endpoint":       c.FullPath(),
			})
			c.JSON(403, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireTenant middleware that ensures request has tenant context
func (jm *JWTMiddleware) RequireTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, exists := c.Get("tenant_id")
		if !exists || tenantID == "" {
			jm.logger.LogSecurityEvent(c.Request.Context(), "missing_tenant_context", map[string]interface{}{
				"user_id":   c.GetString("user_id"),
				"client_ip": c.ClientIP(),
				"endpoint":  c.FullPath(),
			})
			c.JSON(400, gin.H{"error": "Tenant context required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// extractToken extracts JWT token from Authorization header
func (jm *JWTMiddleware) extractToken(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header missing")
	}

	// Check for Bearer token
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", errors.New("invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return "", errors.New("empty token")
	}

	return token, nil
}

// validateToken validates JWT token and returns claims
func (jm *JWTMiddleware) validateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jm.config.Secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// Check if token is expired
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, errors.New("token expired")
	}

	// Check issuer if configured
	if jm.config.Issuer != "" && claims.Issuer != jm.config.Issuer {
		return nil, errors.New("invalid token issuer")
	}

	return claims, nil
}

// setUserContext sets user information in Gin context
func (jm *JWTMiddleware) setUserContext(c *gin.Context, claims *JWTClaims) {
	// Set Gin context values
	c.Set("user_id", claims.UserID)
	c.Set("user_email", claims.Email)
	c.Set("user_role", claims.Role)
	c.Set("tenant_id", claims.TenantID)
	c.Set("session_id", claims.SessionID)

	// Set request context for logger
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "user_id", claims.UserID)
	ctx = context.WithValue(ctx, "tenant_id", claims.TenantID)
	c.Request = c.Request.WithContext(ctx)

	// Log authentication event
	jm.logger.LogBusinessEvent(c.Request.Context(), "user_authenticated", map[string]interface{}{
		"user_id":   claims.UserID,
		"email":     claims.Email,
		"role":      claims.Role,
		"tenant_id": claims.TenantID,
	})
}

// JWTService provides JWT token generation and validation
type JWTService struct {
	config config.JWTConfig
	logger logger.Logger
}

// NewJWTService creates a new JWT service
func NewJWTService(config config.JWTConfig, logger logger.Logger) *JWTService {
	return &JWTService{
		config: config,
		logger: logger,
	}
}

// UserInfo represents user information for token generation
type UserInfo struct {
	ID       string
	Email    string
	Role     string
	TenantID string
}

// GenerateTokens generates access and refresh tokens
func (js *JWTService) GenerateTokens(ctx context.Context, user UserInfo, sessionID string) (accessToken, refreshToken string, expiresAt time.Time, err error) {
	now := time.Now()
	
	// Access token claims
	accessClaims := JWTClaims{
		UserID:    user.ID,
		Email:     user.Email,
		Role:      user.Role,
		TenantID:  user.TenantID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    js.config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(js.config.AccessDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	// Generate access token
	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessTokenObj.SignedString([]byte(js.config.Secret))
	if err != nil {
		js.logger.LogError(ctx, err, "failed to generate access token", map[string]interface{}{
			"user_id": user.ID,
		})
		return "", "", time.Time{}, err
	}

	// Refresh token claims (longer expiry, minimal claims)
	refreshClaims := JWTClaims{
		UserID:    user.ID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    js.config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(js.config.RefreshDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	// Generate refresh token
	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshTokenObj.SignedString([]byte(js.config.RefreshSecret))
	if err != nil {
		js.logger.LogError(ctx, err, "failed to generate refresh token", map[string]interface{}{
			"user_id": user.ID,
		})
		return "", "", time.Time{}, err
	}

	expiresAt = now.Add(js.config.AccessDuration)
	
	js.logger.LogBusinessEvent(ctx, "tokens_generated", map[string]interface{}{
		"user_id":    user.ID,
		"session_id": sessionID,
		"expires_at": expiresAt,
	})

	return accessToken, refreshToken, expiresAt, nil
}

// RefreshTokens validates refresh token and generates new access token
func (js *JWTService) RefreshTokens(ctx context.Context, refreshTokenString string) (accessToken string, expiresAt time.Time, err error) {
	// Validate refresh token
	token, err := jwt.ParseWithClaims(refreshTokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(js.config.RefreshSecret), nil
	})

	if err != nil {
		js.logger.LogSecurityEvent(ctx, "invalid_refresh_token", map[string]interface{}{
			"error": err.Error(),
		})
		return "", time.Time{}, fmt.Errorf("invalid refresh token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return "", time.Time{}, errors.New("invalid refresh token claims")
	}

	// Check expiry
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		js.logger.LogSecurityEvent(ctx, "expired_refresh_token", map[string]interface{}{
			"user_id": claims.UserID,
		})
		return "", time.Time{}, errors.New("refresh token expired")
	}

	// Generate new access token (you'd typically fetch fresh user data here)
	now := time.Now()
	newAccessClaims := JWTClaims{
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    js.config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(js.config.AccessDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, newAccessClaims)
	accessToken, err = accessTokenObj.SignedString([]byte(js.config.Secret))
	if err != nil {
		js.logger.LogError(ctx, err, "failed to generate new access token", map[string]interface{}{
			"user_id": claims.UserID,
		})
		return "", time.Time{}, err
	}

	expiresAt = now.Add(js.config.AccessDuration)
	
	js.logger.LogBusinessEvent(ctx, "tokens_refreshed", map[string]interface{}{
		"user_id":    claims.UserID,
		"session_id": claims.SessionID,
	})

	return accessToken, expiresAt, nil
}

// ValidateAccessToken validates access token and returns claims
func (js *JWTService) ValidateAccessToken(ctx context.Context, tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(js.config.Secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse access token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid access token claims")
	}

	return claims, nil
}