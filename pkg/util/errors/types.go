package errors

import (
	"errors"
	"fmt"
)

// ErrAlreadyHandled is a sentinel error indicating that the error has already
// been rendered to the output (e.g. as structured JSON/YAML) and should not
// be printed again by the caller.
var ErrAlreadyHandled = errors.New("error already rendered to output")

// ErrorCategory represents the classification of a structured error.
type ErrorCategory string

const (
	CategoryAuthentication ErrorCategory = "authentication"
	CategoryAuthorization  ErrorCategory = "authorization"
	CategoryConnection     ErrorCategory = "connection"
	CategoryNotFound       ErrorCategory = "not_found"
	CategoryValidation     ErrorCategory = "validation"
	CategoryConflict       ErrorCategory = "conflict"
	CategoryServer         ErrorCategory = "server"
	CategoryTimeout        ErrorCategory = "timeout"
	CategoryInternal       ErrorCategory = "internal"
)

// StructuredError provides machine-readable error information for programmatic
// consumption by agents and automation tools.
type StructuredError struct {
	Code       string        `json:"code"       yaml:"code"`
	Message    string        `json:"message"    yaml:"message"`
	Category   ErrorCategory `json:"category"   yaml:"category"`
	Retriable  bool          `json:"retriable"  yaml:"retriable"`
	Suggestion string        `json:"suggestion" yaml:"suggestion"`

	cause error
}

// Error implements the error interface.
func (e *StructuredError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Category, e.Message)
}

// Unwrap returns the underlying error, preserving the error chain
// for use with errors.Is and errors.As.
func (e *StructuredError) Unwrap() error {
	return e.cause
}

// NewAlreadyHandledError wraps the original error with ErrAlreadyHandled,
// preserving the full error chain for callers that inspect the cause.
func NewAlreadyHandledError(err error) error {
	return fmt.Errorf("%w: %w", ErrAlreadyHandled, err)
}

// ConfigError indicates a configuration problem such as an invalid kubeconfig,
// missing context, or unreachable cluster entry. Wrapping errors with this type
// allows Classify to distinguish user configuration mistakes from internal bugs.
type ConfigError struct {
	cause error
}

func (e *ConfigError) Error() string { return e.cause.Error() }
func (e *ConfigError) Unwrap() error { return e.cause }

// NewConfigError wraps err as a ConfigError.
func NewConfigError(err error) *ConfigError {
	return &ConfigError{cause: err}
}

// errorEnvelope wraps a StructuredError for JSON/YAML output rendering.
type errorEnvelope struct {
	Error *StructuredError `json:"error" yaml:"error"`
}
