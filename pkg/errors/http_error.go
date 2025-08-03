package errors

import (
	"time"
)

// HTTPError representa un error HTTP para respuestas API
type HTTPError struct {
	Code          string                 `json:"code"`
	Message       string                 `json:"message"`
	Category      string                 `json:"category"`
	Severity      string                 `json:"severity"`
	Details       map[string]interface{} `json:"details,omitempty"`
	Field         string                 `json:"field,omitempty"`
	Value         interface{}            `json:"value,omitempty"`
	RequestID     string                 `json:"request_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	Retryable     bool                   `json:"retryable"`
	Transient     bool                   `json:"transient"`
	HelpURL       string                 `json:"help_url,omitempty"`
}

// HTTPErrorResponse representa la respuesta de error completa
type HTTPErrorResponse struct {
	Error      HTTPError `json:"error"`
	StatusCode int       `json:"-"`
}

// ValidationError representa un error de validación específico
type ValidationError struct {
	Field   string      `json:"field"`
	Value   interface{} `json:"value"`
	Tag     string      `json:"tag"`
	Message string      `json:"message"`
}

// ValidationErrorResponse representa una respuesta con múltiples errores de validación
type ValidationErrorResponse struct {
	Error  HTTPError         `json:"error"`
	Errors []ValidationError `json:"validation_errors"`
}

// BusinessError representa un error de lógica de negocio
type BusinessError struct {
	Rule        string                 `json:"rule"`
	Description string                 `json:"description"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// SecurityError representa un error de seguridad
type SecurityError struct {
	Threat      string `json:"threat"`
	Action      string `json:"action"`
	Severity    string `json:"severity"`
	RemoteIP    string `json:"remote_ip,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
	BlockedTime *time.Time `json:"blocked_until,omitempty"`
}

// IntegrationError representa un error de integración con servicios externos
type IntegrationError struct {
	Service     string                 `json:"service"`
	Endpoint    string                 `json:"endpoint,omitempty"`
	Method      string                 `json:"method,omitempty"`
	StatusCode  int                    `json:"status_code,omitempty"`
	Response    string                 `json:"response,omitempty"`
	Timeout     bool                   `json:"timeout"`
	NetworkError bool                  `json:"network_error"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}