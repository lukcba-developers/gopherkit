package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Handler maneja las rutas de health check
type Handler struct {
	checker *Checker
	timeout time.Duration
}

// NewHandler crea un nuevo handler de health checks
func NewHandler(checker *Checker, timeout time.Duration) *Handler {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	
	return &Handler{
		checker: checker,
		timeout: timeout,
	}
}

// Health maneja GET /health - health check completo
func (h *Handler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	
	response := h.checker.Check(ctx)
	
	// Determinar código de estado HTTP basado en el estado general
	statusCode := h.getHTTPStatusCode(response.Status)
	
	c.JSON(statusCode, response)
}

// Readiness maneja GET /health/ready - readiness probe
func (h *Handler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	
	isReady := h.checker.IsReady(ctx)
	
	if isReady {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"service":   h.checker.service,
			"timestamp": time.Now(),
		})
	} else {
		response := h.checker.Check(ctx)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":     "not_ready",
			"service":    h.checker.service,
			"timestamp":  time.Now(),
			"components": response.Components,
		})
	}
}

// Liveness maneja GET /health/live - liveness probe
func (h *Handler) Liveness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	
	isLive := h.checker.IsLive(ctx)
	
	if isLive {
		c.JSON(http.StatusOK, gin.H{
			"status":    "alive",
			"service":   h.checker.service,
			"timestamp": time.Now(),
			"uptime":    time.Since(h.checker.startTime).String(),
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":    "not_alive",
			"service":   h.checker.service,
			"timestamp": time.Now(),
		})
	}
}

// Component maneja GET /health/component/{name} - health check de componente específico
func (h *Handler) Component(c *gin.Context) {
	componentName := c.Param("name")
	if componentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "component name is required",
			"message": "Please specify a component name in the URL path",
		})
		return
	}
	
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	
	health, found := h.checker.GetComponentStatus(ctx, componentName)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error":      "component not found",
			"component":  componentName,
			"available":  h.checker.ListComponents(),
		})
		return
	}
	
	statusCode := h.getHTTPStatusCode(health.Status)
	
	c.JSON(statusCode, gin.H{
		"component": componentName,
		"health":    health,
		"service":   h.checker.service,
		"timestamp": time.Now(),
	})
}

// Components maneja GET /health/components - lista todos los componentes
func (h *Handler) Components(c *gin.Context) {
	components := h.checker.ListComponents()
	
	c.JSON(http.StatusOK, gin.H{
		"service":    h.checker.service,
		"components": components,
		"count":      len(components),
		"timestamp":  time.Now(),
	})
}

// Info maneja GET /health/info - información del servicio
func (h *Handler) Info(c *gin.Context) {
	info := h.checker.GetServiceInfo()
	c.JSON(http.StatusOK, info)
}

// getHTTPStatusCode convierte el estado de salud a código HTTP
func (h *Handler) getHTTPStatusCode(status Status) int {
	switch status {
	case StatusHealthy:
		return http.StatusOK
	case StatusDegraded:
		return http.StatusOK // 200 pero con estado degraded
	case StatusUnhealthy:
		return http.StatusServiceUnavailable
	case StatusUnknown:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// RegisterRoutes registra todas las rutas de health check en un router Gin
func (h *Handler) RegisterRoutes(router gin.IRouter) {
	health := router.Group("/health")
	{
		health.GET("", h.Health)
		health.GET("/", h.Health)
		health.GET("/ready", h.Readiness)
		health.GET("/live", h.Liveness)
		health.GET("/info", h.Info)
		health.GET("/components", h.Components)
		health.GET("/component/:name", h.Component)
	}
	
	logrus.WithFields(logrus.Fields{
		"service": h.checker.service,
		"routes": []string{
			"/health",
			"/health/ready", 
			"/health/live",
			"/health/info",
			"/health/components",
			"/health/component/:name",
		},
	}).Info("Health check routes registered")
}

// Middleware crea un middleware que registra health check requests
func (h *Handler) Middleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Solo loggear requests de health check
		if c.Request.URL.Path == "/health" || 
		   c.Request.URL.Path == "/health/" ||
		   c.Request.URL.Path == "/health/ready" ||
		   c.Request.URL.Path == "/health/live" {
			
			start := time.Now()
			c.Next()
			duration := time.Since(start)
			
			logrus.WithFields(logrus.Fields{
				"method":     c.Request.Method,
				"path":       c.Request.URL.Path,
				"status":     c.Writer.Status(),
				"duration":   duration,
				"user_agent": c.Request.Header.Get("User-Agent"),
				"remote_ip":  c.ClientIP(),
			}).Debug("Health check request")
		} else {
			c.Next()
		}
	})
}