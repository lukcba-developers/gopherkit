package httptest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequest representa una petición HTTP de prueba
type TestRequest struct {
	Method      string
	URL         string
	Body        interface{}
	Headers     map[string]string
	QueryParams map[string]string
	TenantID    string
	UserID      string
}

// TestResponse representa una respuesta HTTP esperada
type TestResponse struct {
	StatusCode  int
	Body        interface{}
	Headers     map[string]string
	BodyPartial interface{} // Para verificaciones parciales del body
}

// HTTPTestHelper ayuda con las pruebas HTTP
type HTTPTestHelper struct {
	router   *gin.Engine
	server   *httptest.Server
	tenantID string
	userID   string
	logger   *logrus.Logger
}

// NewHTTPTestHelper crea un nuevo helper HTTP
func NewHTTPTestHelper(router *gin.Engine, tenantID, userID string) *HTTPTestHelper {
	return &HTTPTestHelper{
		router:   router,
		tenantID: tenantID,
		userID:   userID,
		logger:   logrus.New(),
	}
}

// StartServer inicia el servidor HTTP de prueba
func (h *HTTPTestHelper) StartServer() {
	h.server = httptest.NewServer(h.router)
}

// StopServer detiene el servidor HTTP de prueba
func (h *HTTPTestHelper) StopServer() {
	if h.server != nil {
		h.server.Close()
		h.server = nil
	}
}

// GetServerURL retorna la URL del servidor de prueba
func (h *HTTPTestHelper) GetServerURL() string {
	if h.server == nil {
		h.StartServer()
	}
	return h.server.URL
}

// MakeRequest realiza una petición HTTP y retorna la respuesta
func (h *HTTPTestHelper) MakeRequest(t *testing.T, req *TestRequest) *http.Response {
	// Prepare body
	var bodyReader io.Reader
	if req.Body != nil {
		if bodyBytes, ok := req.Body.([]byte); ok {
			bodyReader = bytes.NewReader(bodyBytes)
		} else if bodyString, ok := req.Body.(string); ok {
			bodyReader = strings.NewReader(bodyString)
		} else {
			// Marshal to JSON
			jsonBody, err := json.Marshal(req.Body)
			require.NoError(t, err, "Failed to marshal request body")
			bodyReader = bytes.NewReader(jsonBody)
		}
	}

	// Create request
	httpReq, err := http.NewRequest(req.Method, h.GetServerURL()+req.URL, bodyReader)
	require.NoError(t, err, "Failed to create HTTP request")

	// Add default headers
	if req.Body != nil && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Add tenant ID header
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = h.tenantID
	}
	if tenantID != "" {
		httpReq.Header.Set("X-Tenant-ID", tenantID)
	}

	// Add user ID header if available
	userID := req.UserID
	if userID == "" {
		userID = h.userID
	}
	if userID != "" {
		httpReq.Header.Set("X-User-ID", userID)
	}

	// Add custom headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Add query parameters
	if len(req.QueryParams) > 0 {
		q := httpReq.URL.Query()
		for key, value := range req.QueryParams {
			q.Add(key, value)
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	require.NoError(t, err, "Failed to make HTTP request")

	return resp
}

// AssertResponse verifica que la respuesta coincida con las expectativas
func (h *HTTPTestHelper) AssertResponse(t *testing.T, resp *http.Response, expected *TestResponse) {
	// Check status code
	assert.Equal(t, expected.StatusCode, resp.StatusCode, 
		"Expected status code %d, got %d", expected.StatusCode, resp.StatusCode)

	// Check headers
	for key, expectedValue := range expected.Headers {
		actualValue := resp.Header.Get(key)
		assert.Equal(t, expectedValue, actualValue, 
			"Expected header %s to be %s, got %s", key, expectedValue, actualValue)
	}

	// Check body if specified
	if expected.Body != nil || expected.BodyPartial != nil {
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Failed to read response body")
		resp.Body.Close()

		if expected.Body != nil {
			// Full body comparison
			if expectedBytes, ok := expected.Body.([]byte); ok {
				assert.Equal(t, expectedBytes, body)
			} else if expectedString, ok := expected.Body.(string); ok {
				assert.Equal(t, expectedString, string(body))
			} else {
				// JSON comparison
				expectedJSON, err := json.Marshal(expected.Body)
				require.NoError(t, err, "Failed to marshal expected body")
				
				assert.JSONEq(t, string(expectedJSON), string(body), 
					"Response body doesn't match expected JSON")
			}
		}

		if expected.BodyPartial != nil {
			// Partial body comparison - check if expected fields are present
			var actualJSON map[string]interface{}
			err := json.Unmarshal(body, &actualJSON)
			require.NoError(t, err, "Failed to unmarshal response body")

			expectedJSON, err := json.Marshal(expected.BodyPartial)
			require.NoError(t, err, "Failed to marshal expected partial body")

			var expectedPartial map[string]interface{}
			err = json.Unmarshal(expectedJSON, &expectedPartial)
			require.NoError(t, err, "Failed to unmarshal expected partial body")

			// Check each expected field
			for key, expectedValue := range expectedPartial {
				actualValue, exists := actualJSON[key]
				assert.True(t, exists, "Expected field %s not found in response", key)
				assert.Equal(t, expectedValue, actualValue, 
					"Expected field %s to be %v, got %v", key, expectedValue, actualValue)
			}
		}
	}
}

// GET realiza una petición GET
func (h *HTTPTestHelper) GET(t *testing.T, url string, queryParams map[string]string) *http.Response {
	return h.MakeRequest(t, &TestRequest{
		Method:      "GET",
		URL:         url,
		QueryParams: queryParams,
	})
}

// POST realiza una petición POST
func (h *HTTPTestHelper) POST(t *testing.T, url string, body interface{}) *http.Response {
	return h.MakeRequest(t, &TestRequest{
		Method: "POST", 
		URL:    url,
		Body:   body,
	})
}

// PUT realiza una petición PUT
func (h *HTTPTestHelper) PUT(t *testing.T, url string, body interface{}) *http.Response {
	return h.MakeRequest(t, &TestRequest{
		Method: "PUT",
		URL:    url, 
		Body:   body,
	})
}

// DELETE realiza una petición DELETE
func (h *HTTPTestHelper) DELETE(t *testing.T, url string) *http.Response {
	return h.MakeRequest(t, &TestRequest{
		Method: "DELETE",
		URL:    url,
	})
}

// PATCH realiza una petición PATCH
func (h *HTTPTestHelper) PATCH(t *testing.T, url string, body interface{}) *http.Response {
	return h.MakeRequest(t, &TestRequest{
		Method: "PATCH",
		URL:    url,
		Body:   body,
	})
}

// PostJSON realiza una petición POST con JSON y verifica la respuesta
func (h *HTTPTestHelper) PostJSON(t *testing.T, url string, requestBody, expectedResponse interface{}, expectedStatus int) {
	resp := h.POST(t, url, requestBody)
	defer resp.Body.Close()

	h.AssertResponse(t, resp, &TestResponse{
		StatusCode: expectedStatus,
		Body:       expectedResponse,
	})
}

// GetJSON realiza una petición GET y verifica la respuesta JSON
func (h *HTTPTestHelper) GetJSON(t *testing.T, url string, expectedResponse interface{}, expectedStatus int) {
	resp := h.GET(t, url, nil)
	defer resp.Body.Close()

	h.AssertResponse(t, resp, &TestResponse{
		StatusCode: expectedStatus,
		Body:       expectedResponse,
	})
}

// AssertErrorResponse verifica que la respuesta sea un error con el código esperado
func (h *HTTPTestHelper) AssertErrorResponse(t *testing.T, resp *http.Response, expectedStatus int, expectedErrorCode string) {
	assert.Equal(t, expectedStatus, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	var errorResp map[string]interface{}
	err = json.Unmarshal(body, &errorResp)
	require.NoError(t, err, "Response should be valid JSON")

	if expectedErrorCode != "" {
		errorCode, exists := errorResp["code"]
		assert.True(t, exists, "Error response should have 'code' field")
		assert.Equal(t, expectedErrorCode, errorCode)
	}
}

// AssertHealthCheck verifica que el endpoint de health check funcione
func (h *HTTPTestHelper) AssertHealthCheck(t *testing.T, url string) {
	resp := h.GET(t, url, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Health check should return 200")
	
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var healthResp map[string]interface{}
	err = json.Unmarshal(body, &healthResp)
	require.NoError(t, err, "Health response should be valid JSON")

	status, exists := healthResp["status"]
	assert.True(t, exists, "Health response should have 'status' field")
	assert.Equal(t, "healthy", status, "Health status should be 'healthy'")
}

// WithTenantID configura el tenant ID para las siguientes peticiones
func (h *HTTPTestHelper) WithTenantID(tenantID string) *HTTPTestHelper {
	h.tenantID = tenantID
	return h
}

// WithUserID configura el user ID para las siguientes peticiones
func (h *HTTPTestHelper) WithUserID(userID string) *HTTPTestHelper {
	h.userID = userID
	return h
}

// CreateTestContext crea un contexto Gin para pruebas unitarias
func CreateTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	return c, w
}

// SetupTestRouter configura un router Gin para pruebas
func SetupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Add basic middleware for testing
	router.Use(gin.Recovery())
	
	return router
}

// MockResponseWriter implementa http.ResponseWriter para pruebas
type MockResponseWriter struct {
	headers    http.Header
	body       *bytes.Buffer
	statusCode int
}

// NewMockResponseWriter crea un nuevo MockResponseWriter
func NewMockResponseWriter() *MockResponseWriter {
	return &MockResponseWriter{
		headers:    make(http.Header),
		body:       &bytes.Buffer{},
		statusCode: http.StatusOK,
	}
}

func (m *MockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *MockResponseWriter) Write(data []byte) (int, error) {
	return m.body.Write(data)
}

func (m *MockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func (m *MockResponseWriter) GetStatusCode() int {
	return m.statusCode
}

func (m *MockResponseWriter) GetBody() string {
	return m.body.String()
}

func (m *MockResponseWriter) GetHeaders() http.Header {
	return m.headers
}

// TestAuthHelper ayuda con la autenticación en pruebas
type TestAuthHelper struct {
	tenantID string
	userID   string
}

// NewTestAuthHelper crea un nuevo helper de autenticación
func NewTestAuthHelper(tenantID, userID string) *TestAuthHelper {
	return &TestAuthHelper{
		tenantID: tenantID,
		userID:   userID,
	}
}

// CreateJWTToken crea un token JWT de prueba (mock)
func (h *TestAuthHelper) CreateJWTToken() string {
	// This is a mock token for testing - not a real JWT
	return fmt.Sprintf("Bearer test-token-%s-%s", h.tenantID, h.userID)
}

// GetAuthHeaders retorna headers de autenticación para pruebas
func (h *TestAuthHelper) GetAuthHeaders() map[string]string {
	return map[string]string{
		"Authorization": h.CreateJWTToken(),
		"X-Tenant-ID":   h.tenantID,
		"X-User-ID":     h.userID,
	}
}

// CreateTestUser crea un usuario de prueba con datos básicos
func CreateTestUser(tenantID string) map[string]interface{} {
	userID := uuid.New().String()
	return map[string]interface{}{
		"id":        userID,
		"tenant_id": tenantID,
		"name":      "Test User",
		"email":     fmt.Sprintf("user_%s@test.com", userID[:8]),
		"sport":     "tennis",
		"is_active": true,
		"created_at": time.Now().Format(time.RFC3339),
		"updated_at": time.Now().Format(time.RFC3339),
	}
}

// CreateTestOrganization crea una organización de prueba
func CreateTestOrganization(tenantID string) map[string]interface{} {
	orgID := uuid.New().String()
	return map[string]interface{}{
		"id":        orgID,
		"tenant_id": tenantID,
		"name":      "Test Organization",
		"type":      "club",
		"settings":  map[string]interface{}{"theme": "default", "language": "es"},
		"is_active": true,
		"created_at": time.Now().Format(time.RFC3339),
		"updated_at": time.Now().Format(time.RFC3339),
	}
}

// AssertJSONContains verifica que el JSON contenga los campos esperados
func AssertJSONContains(t *testing.T, jsonBody []byte, expectedFields map[string]interface{}) {
	var actualData map[string]interface{}
	err := json.Unmarshal(jsonBody, &actualData)
	require.NoError(t, err, "Failed to unmarshal JSON body")

	for key, expectedValue := range expectedFields {
		actualValue, exists := actualData[key]
		assert.True(t, exists, "Expected field %s not found in JSON", key)
		assert.Equal(t, expectedValue, actualValue, 
			"Expected field %s to be %v, got %v", key, expectedValue, actualValue)
	}
}

// ParseJSONResponse parses JSON response into a map
func ParseJSONResponse(t *testing.T, resp *http.Response) map[string]interface{} {
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")
	resp.Body.Close()

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	require.NoError(t, err, "Failed to parse JSON response")

	return data
}

// CreateRequestWithContext crea una request HTTP con contexto personalizado
func CreateRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	
	// Add correlation ID to context
	if correlationID := ctx.Value("correlation_id"); correlationID != nil {
		if id, ok := correlationID.(string); ok {
			req.Header.Set("X-Correlation-ID", id)
		}
	}
	
	return req, nil
}