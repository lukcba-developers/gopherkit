package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/lukcba-developers/gopherkit/pkg/domain/service"
)

// ServiceEnabledMiddleware checks if a service is enabled for the organization
type ServiceEnabledMiddleware struct {
	serviceRegistry *service.ServiceRegistry
	logger          *logrus.Logger
	serviceName     string
}

// NewServiceEnabledMiddleware creates a new service enabled middleware
func NewServiceEnabledMiddleware(serviceRegistry *service.ServiceRegistry, serviceName string, logger *logrus.Logger) *ServiceEnabledMiddleware {
	return &ServiceEnabledMiddleware{
		serviceRegistry: serviceRegistry,
		serviceName:     serviceName,
		logger:          logger,
	}
}

// Handle checks if the service is enabled before processing the request
func (sem *ServiceEnabledMiddleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract tenant/organization ID from header
		organizationID := c.GetHeader("X-Tenant-ID")
		if organizationID == "" {
			organizationID = c.GetHeader("X-Organization-ID")
		}

		// Skip check for health endpoints and public endpoints
		if sem.shouldSkipCheck(c.Request.URL.Path) {
			c.Next()
			return
		}

		// If no organization ID is provided, allow core services only
		if organizationID == "" {
			if sem.isCoreService() {
				c.Next()
				return
			}
			
			sem.logger.WithFields(logrus.Fields{
				"service": sem.serviceName,
				"path":    c.Request.URL.Path,
			}).Warn("No organization ID provided for non-core service")
			
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "organization_id_required",
				"message": "Organization ID is required for this service",
				"service": sem.serviceName,
			})
			c.Abort()
			return
		}

		// Check if service is enabled for the organization
		enabled, err := sem.serviceRegistry.IsServiceEnabled(c.Request.Context(), organizationID, sem.serviceName)
		if err != nil {
			sem.logger.WithFields(logrus.Fields{
				"organization_id": organizationID,
				"service":         sem.serviceName,
				"error":          err,
				"path":           c.Request.URL.Path,
			}).Error("Failed to check service enablement")
			
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "service_check_failed",
				"message": "Failed to verify service availability",
			})
			c.Abort()
			return
		}

		if !enabled {
			sem.logger.WithFields(logrus.Fields{
				"organization_id": organizationID,
				"service":         sem.serviceName,
				"path":           c.Request.URL.Path,
			}).Warn("Service not enabled for organization")
			
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "service_not_enabled",
				"message": "This service is not enabled for your organization",
				"service": sem.serviceName,
			})
			c.Abort()
			return
		}

		// Add service info to context for potential use downstream
		c.Set("organization_id", organizationID)
		c.Set("service_name", sem.serviceName)
		c.Set("service_enabled", true)

		c.Next()
	}
}

// shouldSkipCheck determines if the service enablement check should be skipped
func (sem *ServiceEnabledMiddleware) shouldSkipCheck(path string) bool {
	skipPaths := []string{
		"/health",
		"/metrics",
		"/ready",
		"/live",
		"/ping",
		"/swagger",
		"/docs",
		"/favicon.ico",
	}

	pathLower := strings.ToLower(path)
	for _, skipPath := range skipPaths {
		if strings.HasPrefix(pathLower, skipPath) {
			return true
		}
	}

	return false
}

// isCoreService checks if the current service is a core service
func (sem *ServiceEnabledMiddleware) isCoreService() bool {
	coreServices := []string{"auth", "user"}
	for _, coreService := range coreServices {
		if sem.serviceName == coreService {
			return true
		}
	}
	return false
}

// ServiceRegistryMiddleware adds the service registry to the context
type ServiceRegistryMiddleware struct {
	serviceRegistry *service.ServiceRegistry
}

// NewServiceRegistryMiddleware creates a new service registry middleware
func NewServiceRegistryMiddleware(serviceRegistry *service.ServiceRegistry) *ServiceRegistryMiddleware {
	return &ServiceRegistryMiddleware{
		serviceRegistry: serviceRegistry,
	}
}

// Handle adds the service registry to the gin context
func (srm *ServiceRegistryMiddleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("service_registry", srm.serviceRegistry)
		c.Next()
	}
}

// GetServiceRegistry extracts the service registry from gin context
func GetServiceRegistry(c *gin.Context) (*service.ServiceRegistry, bool) {
	registry, exists := c.Get("service_registry")
	if !exists {
		return nil, false
	}
	
	serviceRegistry, ok := registry.(*service.ServiceRegistry)
	return serviceRegistry, ok
}

// OrganizationConfigMiddleware adds organization configuration to the context
type OrganizationConfigMiddleware struct {
	serviceRegistry *service.ServiceRegistry
	logger          *logrus.Logger
}

// NewOrganizationConfigMiddleware creates a new organization config middleware
func NewOrganizationConfigMiddleware(serviceRegistry *service.ServiceRegistry, logger *logrus.Logger) *OrganizationConfigMiddleware {
	return &OrganizationConfigMiddleware{
		serviceRegistry: serviceRegistry,
		logger:          logger,
	}
}

// Handle loads and adds organization configuration to the context
func (ocm *OrganizationConfigMiddleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID := c.GetHeader("X-Tenant-ID")
		if organizationID == "" {
			organizationID = c.GetHeader("X-Organization-ID")
		}

		if organizationID == "" {
			// No organization ID provided, continue without config
			c.Next()
			return
		}

		// Load organization configuration
		config, err := ocm.serviceRegistry.GetOrganizationConfig(c.Request.Context(), organizationID)
		if err != nil {
			ocm.logger.WithFields(logrus.Fields{
				"organization_id": organizationID,
				"error":          err,
			}).Warn("Failed to load organization configuration")
			// Continue without config rather than failing the request
			c.Next()
			return
		}

		// Add config to context
		if config != nil {
			c.Set("organization_config", config)
			c.Set("template_type", config.TemplateType)
			c.Set("enabled_services", config.GetEnabledServices())
		}

		c.Next()
	}
}

// AdminOnlyMiddleware restricts access to admin-only endpoints
type AdminOnlyMiddleware struct {
	logger *logrus.Logger
}

// NewAdminOnlyMiddleware creates a new admin-only middleware
func NewAdminOnlyMiddleware(logger *logrus.Logger) *AdminOnlyMiddleware {
	return &AdminOnlyMiddleware{
		logger: logger,
	}
}

// Handle checks if the user has admin privileges
func (aom *AdminOnlyMiddleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract user role from JWT token or header
		userRole := c.GetHeader("X-User-Role")
		if userRole == "" {
			// Try to get from JWT claims if available
			if claims, exists := c.Get("jwt_claims"); exists {
				if claimsMap, ok := claims.(map[string]interface{}); ok {
					if role, exists := claimsMap["role"]; exists {
						userRole = role.(string)
					}
				}
			}
		}

		// Check if user has admin role
		if userRole != "super_admin" && userRole != "admin" {
			aom.logger.WithFields(logrus.Fields{
				"user_role": userRole,
				"path":      c.Request.URL.Path,
				"method":    c.Request.Method,
			}).Warn("Unauthorized access attempt to admin endpoint")
			
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "admin_access_required",
				"message": "Admin privileges required to access this endpoint",
			})
			c.Abort()
			return
		}

		// Add admin flag to context
		c.Set("is_admin", true)
		c.Set("user_role", userRole)

		c.Next()
	}
}

// SuperAdminOnlyMiddleware restricts access to super admin only endpoints
type SuperAdminOnlyMiddleware struct {
	logger *logrus.Logger
}

// NewSuperAdminOnlyMiddleware creates a new super admin only middleware
func NewSuperAdminOnlyMiddleware(logger *logrus.Logger) *SuperAdminOnlyMiddleware {
	return &SuperAdminOnlyMiddleware{
		logger: logger,
	}
}

// Handle checks if the user has super admin privileges
func (saom *SuperAdminOnlyMiddleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract user role from JWT token or header
		userRole := c.GetHeader("X-User-Role")
		if userRole == "" {
			// Try to get from JWT claims if available
			if claims, exists := c.Get("jwt_claims"); exists {
				if claimsMap, ok := claims.(map[string]interface{}); ok {
					if role, exists := claimsMap["role"]; exists {
						userRole = role.(string)
					}
				}
			}
		}

		// Check if user has super admin role
		if userRole != "super_admin" {
			saom.logger.WithFields(logrus.Fields{
				"user_role": userRole,
				"path":      c.Request.URL.Path,
				"method":    c.Request.Method,
			}).Warn("Unauthorized access attempt to super admin endpoint")
			
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "super_admin_access_required",
				"message": "Super admin privileges required to access this endpoint",
			})
			c.Abort()
			return
		}

		// Add super admin flag to context
		c.Set("is_super_admin", true)
		c.Set("user_role", userRole)

		c.Next()
	}
}