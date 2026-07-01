package cmd

import (
	"fmt"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	errCodeInvalidPollInterval = "INVALID_POLL_INTERVAL"
	errCodeInvalidWaitTimeout  = "INVALID_TIMEOUT"
	errCodeWaitTimeout         = "WAIT_TIMEOUT"

	msgInvalidPollInterval = "poll interval must be between 1s and 5m"
	msgInvalidWaitTimeout  = "timeout must be between 0 and 30m (0 means no timeout)"
	msgWaitTimeout         = "timed out waiting for condition %q"

	suggestValidPollInterval = "Use --poll-interval with a duration between 1s and 5m (e.g., 5s, 10s, 30s)"
	suggestValidWaitTimeout  = "Use --timeout=0 for no timeout, or a duration up to 30m (e.g., 30s, 5m)"
	suggestWaitTimeout       = "Increase --timeout or check platform health with: kubectl odh status"
)

// ErrInvalidPollInterval creates a structured error for invalid poll interval values.
func ErrInvalidPollInterval() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeInvalidPollInterval,
		Message:    msgInvalidPollInterval,
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestValidPollInterval,
	}
}

// ErrInvalidWaitTimeout creates a structured error for negative timeout in wait mode.
func ErrInvalidWaitTimeout() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeInvalidWaitTimeout,
		Message:    msgInvalidWaitTimeout,
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestValidWaitTimeout,
	}
}

// ErrWaitTimeout creates a structured error when --wait-for times out.
func ErrWaitTimeout(condition string) *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeWaitTimeout,
		Message:    fmt.Sprintf(msgWaitTimeout, condition),
		Category:   clierrors.CategoryTimeout,
		Retriable:  true,
		Suggestion: suggestWaitTimeout,
	}
}
