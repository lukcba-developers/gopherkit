package httpclient

import "errors"

var (
	ErrInvalidName         = errors.New("client name cannot be empty")
	ErrInvalidBaseURL      = errors.New("base URL cannot be empty")
	ErrInvalidTimeout      = errors.New("timeout must be greater than 0")
	ErrInvalidMaxRetries   = errors.New("max retries cannot be negative")
	ErrInvalidMultiplier   = errors.New("multiplier must be greater than 0")
	ErrInvalidFailureRatio = errors.New("failure ratio must be between 0.0 and 1.0")
)