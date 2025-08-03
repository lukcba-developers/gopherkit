package cors

import (
	"errors"
	"fmt"
)

var (
	// ErrUnsafeProductionConfig se lanza cuando se intenta usar configuración insegura en producción
	ErrUnsafeProductionConfig = errors.New("unsafe CORS configuration detected in production mode: AllowAllOrigins=true is not allowed")
	
	// ErrNoOriginsAllowed se lanza cuando no hay orígenes permitidos
	ErrNoOriginsAllowed = errors.New("no origins allowed: must specify at least one origin when AllowAllOrigins is false")
	
	// ErrOriginNotAllowed se lanza cuando un origen no está permitido
	ErrOriginNotAllowed = errors.New("origin not allowed by CORS policy")
)

// ErrInvalidMethod error para métodos HTTP inválidos
type ErrInvalidMethod struct {
	Method string
}

func (e *ErrInvalidMethod) Error() string {
	return fmt.Sprintf("invalid HTTP method: %s", e.Method)
}

// NewErrInvalidMethod crea un nuevo error de método inválido
func NewErrInvalidMethod(method string) *ErrInvalidMethod {
	return &ErrInvalidMethod{Method: method}
}

// ErrInvalidOrigin error para orígenes inválidos
type ErrInvalidOrigin struct {
	Origin string
	Reason string
}

func (e *ErrInvalidOrigin) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("invalid origin %s: %s", e.Origin, e.Reason)
	}
	return fmt.Sprintf("invalid origin: %s", e.Origin)
}

// NewErrInvalidOrigin crea un nuevo error de origen inválido
func NewErrInvalidOrigin(origin, reason string) *ErrInvalidOrigin {
	return &ErrInvalidOrigin{Origin: origin, Reason: reason}
}