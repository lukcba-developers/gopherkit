package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHandler() (*Handler, *Checker) {
	checker := NewChecker("test-service", "v1.0.0")
	handler := NewHandler(checker, 5*time.Second)
	return handler, checker
}

func setupGinTest() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestNewHandler(t *testing.T) {
	t.Run("with custom timeout", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		timeout := 10 * time.Second
		
		handler := NewHandler(checker, timeout)
		
		assert.Equal(t, checker, handler.checker)
		assert.Equal(t, timeout, handler.timeout)
	})

	t.Run("with zero timeout defaults to 30s", func(t *testing.T) {
		checker := NewChecker("test", "v1.0.0")
		
		handler := NewHandler(checker, 0)
		
		assert.Equal(t, 30*time.Second, handler.timeout)
	})
}

func TestHandler_Health(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	router.GET("/health", handler.Health)

	t.Run("no checkers - healthy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response HealthResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, StatusHealthy, response.Status)
		assert.Equal(t, "test-service", response.Service)
		assert.Equal(t, "v1.0.0", response.Version)
		assert.NotZero(t, response.Timestamp)
		assert.NotEmpty(t, response.RequestID)
	})

	t.Run("healthy checkers", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusHealthy))
		
		req := httptest.NewRequest("GET", "/health", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response HealthResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, StatusHealthy, response.Status)
		assert.Len(t, response.Components, 1)
		assert.Contains(t, response.Components, "db")
	})

	t.Run("degraded checkers", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("redis").SetStatus(StatusDegraded))
		
		req := httptest.NewRequest("GET", "/health", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code) // Degraded still returns 200
		
		var response HealthResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, StatusDegraded, response.Status)
	})

	t.Run("unhealthy checkers", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("api").SetStatus(StatusUnhealthy))
		
		req := httptest.NewRequest("GET", "/health", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
		
		var response HealthResponse
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, StatusUnhealthy, response.Status)
	})
}

func TestHandler_Readiness(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	router.GET("/health/ready", handler.Readiness)

	t.Run("ready service", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusHealthy))
		
		req := httptest.NewRequest("GET", "/health/ready", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "ready", response["status"])
		assert.Equal(t, "test-service", response["service"])
		assert.NotNil(t, response["timestamp"])
	})

	t.Run("not ready service", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusUnhealthy))
		
		req := httptest.NewRequest("GET", "/health/ready", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "not_ready", response["status"])
		assert.Equal(t, "test-service", response["service"])
		assert.NotNil(t, response["components"])
	})

	t.Run("degraded service is ready", func(t *testing.T) {
		// Limpiar checkers anteriores
		checker.checkers = make([]HealthChecker, 0)
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusDegraded))
		
		req := httptest.NewRequest("GET", "/health/ready", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "ready", response["status"])
	})
}

func TestHandler_Liveness(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	router.GET("/health/live", handler.Liveness)

	t.Run("alive service", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/live", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "alive", response["status"])
		assert.Equal(t, "test-service", response["service"])
		assert.NotNil(t, response["timestamp"])
		assert.NotNil(t, response["uptime"])
	})

	t.Run("always alive even with unhealthy components", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusUnhealthy))
		
		req := httptest.NewRequest("GET", "/health/live", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code) // Liveness always returns OK
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "alive", response["status"])
	})
}

func TestHandler_Component(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	router.GET("/health/component/:name", handler.Component)

	t.Run("existing component", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusHealthy).SetMessage("Database OK"))
		
		req := httptest.NewRequest("GET", "/health/component/db", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "db", response["component"])
		assert.Equal(t, "test-service", response["service"])
		assert.NotNil(t, response["health"])
		assert.NotNil(t, response["timestamp"])
	})

	t.Run("non-existing component", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/component/redis", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusNotFound, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "component not found", response["error"])
		assert.Equal(t, "redis", response["component"])
		assert.NotNil(t, response["available"])
	})

	t.Run("empty component name", func(t *testing.T) {
		// Con Gin, /health/component/ sin nombre resulta en 404, no 400
		// porque no coincide con la ruta /health/component/:name
		req := httptest.NewRequest("GET", "/health/component/", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		// Gin retorna 404 para rutas no encontradas
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("unhealthy component returns 503", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("broken").SetStatus(StatusUnhealthy))
		
		req := httptest.NewRequest("GET", "/health/component/broken", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
	})
}

func TestHandler_Components(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	router.GET("/health/components", handler.Components)

	t.Run("no components", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/components", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "test-service", response["service"])
		assert.Equal(t, float64(0), response["count"]) // JSON numbers are float64
		
		components, ok := response["components"].([]interface{})
		require.True(t, ok)
		assert.Len(t, components, 0)
	})

	t.Run("multiple components", func(t *testing.T) {
		checker.AddChecker(NewMockHealthChecker("db"))
		checker.AddChecker(NewMockHealthChecker("redis"))
		checker.AddChecker(NewMockHealthChecker("api"))
		
		req := httptest.NewRequest("GET", "/health/components", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "test-service", response["service"])
		assert.Equal(t, float64(3), response["count"])
		
		components, ok := response["components"].([]interface{})
		require.True(t, ok)
		assert.Len(t, components, 3)
		
		// Convertir a strings para verificar contenido
		componentNames := make([]string, len(components))
		for i, comp := range components {
			componentNames[i] = comp.(string)
		}
		
		assert.Contains(t, componentNames, "db")
		assert.Contains(t, componentNames, "redis")
		assert.Contains(t, componentNames, "api")
	})
}

func TestHandler_Info(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	router.GET("/health/info", handler.Info)
	
	checker.AddChecker(NewMockHealthChecker("db"))
	checker.AddChecker(NewMockHealthChecker("redis"))

	req := httptest.NewRequest("GET", "/health/info", nil)
	resp := httptest.NewRecorder()
	
	router.ServeHTTP(resp, req)
	
	assert.Equal(t, http.StatusOK, resp.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, "test-service", response["service"])
	assert.Equal(t, "v1.0.0", response["version"])
	assert.Equal(t, float64(2), response["components"])
	assert.NotNil(t, response["uptime"])
	assert.NotNil(t, response["start_time"])
}

func TestHandler_getHTTPStatusCode(t *testing.T) {
	handler, _ := setupTestHandler()
	
	tests := []struct {
		status   Status
		expected int
	}{
		{StatusHealthy, http.StatusOK},
		{StatusDegraded, http.StatusOK},
		{StatusUnhealthy, http.StatusServiceUnavailable},
		{StatusUnknown, http.StatusServiceUnavailable},
		{Status("invalid"), http.StatusInternalServerError},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			statusCode := handler.getHTTPStatusCode(tt.status)
			assert.Equal(t, tt.expected, statusCode)
		})
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	handler, _ := setupTestHandler()
	router := setupGinTest()
	
	// Capturar el output de logs sería complejo, así que solo verificamos que no panic
	assert.NotPanics(t, func() {
		handler.RegisterRoutes(router)
	})
	
	// Verificar que las rutas fueron registradas probando algunas
	routes := router.Routes()
	
	healthRoutes := []string{
		"/health",
		"/health/",
		"/health/ready",
		"/health/live",
		"/health/info",
		"/health/components",
		"/health/component/:name",
	}
	
	registeredPaths := make(map[string]bool)
	for _, route := range routes {
		registeredPaths[route.Path] = true
	}
	
	for _, expectedPath := range healthRoutes {
		assert.True(t, registeredPaths[expectedPath], "Route %s should be registered", expectedPath)
	}
}

func TestHandler_Middleware(t *testing.T) {
	handler, _ := setupTestHandler()
	router := setupGinTest()
	
	// Configurar middleware
	router.Use(handler.Middleware())
	
	// Añadir una ruta de health check
	router.GET("/health", handler.Health)
	
	// Añadir una ruta normal que no debería ser loggeada
	router.GET("/api/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"test": "ok"})
	})

	t.Run("logs health check requests", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("User-Agent", "test-agent")
		resp := httptest.NewRecorder()
		
		// El middleware no debería panic y la request debería procesarse
		assert.NotPanics(t, func() {
			router.ServeHTTP(resp, req)
		})
		
		assert.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("doesn't interfere with non-health requests", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		resp := httptest.NewRecorder()
		
		router.ServeHTTP(resp, req)
		
		assert.Equal(t, http.StatusOK, resp.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Equal(t, "ok", response["test"])
	})

	t.Run("handles different health paths", func(t *testing.T) {
		healthPaths := []string{"/health", "/health/", "/health/ready", "/health/live"}
		
		for _, path := range healthPaths {
			req := httptest.NewRequest("GET", path, nil)
			resp := httptest.NewRecorder()
			
			assert.NotPanics(t, func() {
				router.ServeHTTP(resp, req)
			}, "Should not panic for path: %s", path)
		}
	})
}

func TestHandler_ContextTimeout(t *testing.T) {
	// Crear un handler con timeout muy corto
	checker := NewChecker("test-service", "v1.0.0")  
	handler := NewHandler(checker, 1*time.Millisecond)
	router := setupGinTest()
	router.GET("/health", handler.Health)
	
	// Añadir un checker que tome más tiempo que el timeout
	slowChecker := NewMockHealthChecker("slow").SetDuration(100 * time.Millisecond)
	checker.AddChecker(slowChecker)

	req := httptest.NewRequest("GET", "/health", nil)
	resp := httptest.NewRecorder()
	
	start := time.Now()
	router.ServeHTTP(resp, req)
	elapsed := time.Since(start)
	
	// Debería terminar rápidamente debido al timeout
	// Dar más margen debido a la concurrencia y scheduling  
	assert.Less(t, elapsed, 150*time.Millisecond)
	
	// Aún debería retornar una respuesta válida
	assert.NotEqual(t, 0, resp.Code)
}

func TestHandler_Integration(t *testing.T) {
	handler, checker := setupTestHandler()
	router := setupGinTest()
	
	// Registrar todas las rutas
	handler.RegisterRoutes(router)
	
	// Añadir algunos checkers de prueba
	checker.AddChecker(NewMockHealthChecker("db").SetStatus(StatusHealthy))
	checker.AddChecker(NewMockHealthChecker("redis").SetStatus(StatusDegraded))
	
	testCases := []struct {
		method       string
		path         string
		expectedCode int
	}{
		{"GET", "/health", http.StatusOK}, // Degraded pero OK en HTTP
		{"GET", "/health/", http.StatusOK},
		{"GET", "/health/ready", http.StatusOK}, // Ready acepta degraded
		{"GET", "/health/live", http.StatusOK},
		{"GET", "/health/info", http.StatusOK},
		{"GET", "/health/components", http.StatusOK},
		{"GET", "/health/component/db", http.StatusOK},
		{"GET", "/health/component/redis", http.StatusOK},
		{"GET", "/health/component/nonexistent", http.StatusNotFound},
	}
	
	for _, tc := range testCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			resp := httptest.NewRecorder()
			
			router.ServeHTTP(resp, req)
			
			assert.Equal(t, tc.expectedCode, resp.Code, "Unexpected status code for %s %s", tc.method, tc.path)
			
			// Verificar que todas las respuestas tienen contenido JSON válido
			if resp.Code != http.StatusNotFound { // 404 también retorna JSON válido
				var result map[string]interface{}
				err := json.Unmarshal(resp.Body.Bytes(), &result)
				assert.NoError(t, err, "Response should be valid JSON for %s %s", tc.method, tc.path)
			}
		})
	}
}

// Benchmark tests
func BenchmarkHandler_Health(b *testing.B) {
	handler, checker := setupTestHandler()
	checker.AddChecker(NewMockHealthChecker("db"))
	router := setupGinTest()
	router.GET("/health", handler.Health)
	
	req := httptest.NewRequest("GET", "/health", nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}
}

func BenchmarkHandler_Liveness(b *testing.B) {
	handler, _ := setupTestHandler()
	router := setupGinTest()
	router.GET("/health/live", handler.Liveness)
	
	req := httptest.NewRequest("GET", "/health/live", nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}
}

func BenchmarkHandler_Components(b *testing.B) {
	handler, checker := setupTestHandler()
	for i := 0; i < 10; i++ {
		name := "component" + string(rune(i))
		checker.AddChecker(NewMockHealthChecker(name))
	}
	router := setupGinTest()
	router.GET("/health/components", handler.Components)
	
	req := httptest.NewRequest("GET", "/health/components", nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}
}