package errors

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrorSeverity define la severidad del error
type ErrorSeverity string

const (
	SeverityLow      ErrorSeverity = "low"
	SeverityMedium   ErrorSeverity = "medium"
	SeverityHigh     ErrorSeverity = "high"
	SeverityCritical ErrorSeverity = "critical"
)

// ErrorCategory define la categoría del error
type ErrorCategory string

const (
	CategoryValidation   ErrorCategory = "validation"
	CategoryBusiness     ErrorCategory = "business"
	CategoryTechnical    ErrorCategory = "technical"
	CategorySecurity     ErrorCategory = "security"
	CategoryIntegration  ErrorCategory = "integration"
	CategoryPerformance  ErrorCategory = "performance"
	CategoryInfrastructure ErrorCategory = "infrastructure"
)

// StackFrame representa un frame en el stack trace
type StackFrame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Package  string `json:"package"`
}

// StackTrace representa un stack trace completo
type StackTrace struct {
	Frames []StackFrame `json:"frames"`
	Raw    string       `json:"raw"`
}

// ErrorContext proporciona contexto rico para errores
type ErrorContext struct {
	Service       string                 `json:"service"`
	Version       string                 `json:"version"`
	RequestID     string                 `json:"request_id"`
	CorrelationID string                 `json:"correlation_id"`
	UserID        string                 `json:"user_id,omitempty"`
	TenantID      string                 `json:"tenant_id,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	Environment   string                 `json:"environment"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	SpanID        string                 `json:"span_id,omitempty"`
}

// DomainError representa un error de dominio con información rica para debugging
type DomainError struct {
	// Información básica del error
	ID       string        `json:"id"`
	Code     string        `json:"code"`
	Message  string        `json:"message"`
	Category ErrorCategory `json:"category"`
	Severity ErrorSeverity `json:"severity"`

	// Información de contexto
	Context *ErrorContext `json:"context"`

	// Stack trace y debugging
	StackTrace *StackTrace `json:"stack_trace,omitempty"`
	Cause      error       `json:"-"` // Error original (no serializable)
	CauseMsg   string      `json:"cause,omitempty"`

	// Información HTTP
	StatusCode int `json:"-"`

	// Información adicional
	Details    map[string]interface{} `json:"details,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Retryable  bool                   `json:"retryable"`
	Transient  bool                   `json:"transient"`
	
	// Información de debugging
	Fingerprint string `json:"fingerprint,omitempty"` // Para agrupar errores similares
	Tags        []string `json:"tags,omitempty"`
}

// Error implementa la interfaz error
func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %s (caused by: %s)", e.Code, e.Message, e.Details, e.Cause.Error())
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Details)
}

// Unwrap permite unwrapping del error original
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// Is permite comparación de errores
func (e *DomainError) Is(target error) bool {
	if target == nil {
		return false
	}
	
	if domainErr, ok := target.(*DomainError); ok {
		return e.Code == domainErr.Code
	}
	
	return false
}

// WithContext añade contexto al error
func (e *DomainError) WithContext(ctx *ErrorContext) *DomainError {
	e.Context = ctx
	return e
}

// WithMetadata añade metadata al error
func (e *DomainError) WithMetadata(key string, value interface{}) *DomainError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithCause añade la causa del error
func (e *DomainError) WithCause(cause error) *DomainError {
	e.Cause = cause
	if cause != nil {
		e.CauseMsg = cause.Error()
	}
	return e
}

// WithStackTrace añade stack trace al error
func (e *DomainError) WithStackTrace(skip int) *DomainError {
	e.StackTrace = captureStackTrace(skip + 1)
	return e
}

// WithTags añade tags para clasificación
func (e *DomainError) WithTags(tags ...string) *DomainError {
	e.Tags = append(e.Tags, tags...)
	return e
}

// IsRetryable indica si el error puede ser reintentado
func (e *DomainError) IsRetryable() bool {
	return e.Retryable
}

// IsTransient indica si el error es transitorio
func (e *DomainError) IsTransient() bool {
	return e.Transient
}

// GenerateFingerprint genera un fingerprint único para agrupar errores similares
func (e *DomainError) GenerateFingerprint() string {
	if e.Fingerprint != "" {
		return e.Fingerprint
	}
	
	// Generar fingerprint basado en código, mensaje y primer frame del stack
	components := []string{e.Code, e.Message}
	
	if e.StackTrace != nil && len(e.StackTrace.Frames) > 0 {
		frame := e.StackTrace.Frames[0]
		components = append(components, fmt.Sprintf("%s:%d", frame.File, frame.Line))
	}
	
	e.Fingerprint = fmt.Sprintf("%x", strings.Join(components, "|"))
	return e.Fingerprint
}

// ToJSON serializa el error a JSON
func (e *DomainError) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ToHTTPResponse convierte el error a respuesta HTTP
func (e *DomainError) ToHTTPResponse() HTTPErrorResponse {
	response := HTTPErrorResponse{
		Error: HTTPError{
			Code:       e.Code,
			Message:    e.Message,
			Category:   string(e.Category),
			Severity:   string(e.Severity),
			Timestamp:  e.Timestamp,
			Retryable:  e.Retryable,
			Transient:  e.Transient,
		},
		StatusCode: e.StatusCode,
	}
	
	if e.Context != nil {
		response.Error.RequestID = e.Context.RequestID
		response.Error.CorrelationID = e.Context.CorrelationID
		response.Error.TraceID = e.Context.TraceID
	}
	
	if e.Details != nil {
		response.Error.Details = e.Details
	}
	
	return response
}

// captureStackTrace captura el stack trace actual
func captureStackTrace(skip int) *StackTrace {
	const maxFrames = 50
	frames := make([]StackFrame, 0, maxFrames)
	
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip+2, pcs) // +2 para saltar esta función y runtime.Callers
	
	if n == 0 {
		return &StackTrace{Frames: frames}
	}
	
	callersFrames := runtime.CallersFrames(pcs[:n])
	rawTrace := make([]string, 0, n)
	
	for {
		frame, more := callersFrames.Next()
		
		// Extraer nombre del paquete del nombre de la función
		funcParts := strings.Split(frame.Function, ".")
		var pkg string
		if len(funcParts) > 1 {
			pkg = strings.Join(funcParts[:len(funcParts)-1], ".")
		}
		
		stackFrame := StackFrame{
			Function: frame.Function,
			File:     frame.File,
			Line:     frame.Line,
			Package:  pkg,
		}
		
		frames = append(frames, stackFrame)
		rawTrace = append(rawTrace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		
		if !more {
			break
		}
	}
	
	return &StackTrace{
		Frames: frames,
		Raw:    strings.Join(rawTrace, "\n"),
	}
}

// NewDomainError crea un nuevo error de dominio
func NewDomainError(code, message string, category ErrorCategory, severity ErrorSeverity) *DomainError {
	return &DomainError{
		ID:        uuid.New().String(),
		Code:      code,
		Message:   message,
		Category:  category,
		Severity:  severity,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
		StatusCode: 500, // Default internal server error
	}
}