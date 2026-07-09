package cmd

import (
	"fmt"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	errCodeWatchWaitExclusive      = "WATCH_WAIT_EXCLUSIVE"
	errCodeWatchRequiresStructured = "WATCH_OUTPUT_FORMAT"
	errCodeWatchMaxErrors          = "WATCH_MAX_ERRORS"

	msgWatchWaitExclusive      = "cannot use --watch and --wait-for together"
	msgWatchRequiresStructured = "--watch requires structured output (-o json or -o yaml)"
	msgWatchMaxErrors          = "watch stopped after %d consecutive errors: %v"

	suggestWatchWaitExclusive  = "Use --watch for streaming output or --wait-for to block until a condition is met, not both"
	suggestWatchRequiresFormat = "Use --watch with -o json or -o yaml for machine-readable streaming output"
	suggestWatchMaxErrors      = "Check cluster connectivity and credentials, then retry"
)

// ErrWatchWaitExclusive creates a structured error when --watch and --wait-for are both set.
func ErrWatchWaitExclusive() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeWatchWaitExclusive,
		Message:    msgWatchWaitExclusive,
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestWatchWaitExclusive,
	}
}

// ErrWatchRequiresStructuredOutput creates a structured error when --watch is used with unsupported output.
func ErrWatchRequiresStructuredOutput() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeWatchRequiresStructured,
		Message:    msgWatchRequiresStructured,
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestWatchRequiresFormat,
	}
}

// ErrWatchMaxErrors creates a structured error when watch exceeds max consecutive errors.
func ErrWatchMaxErrors(lastErr error) *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeWatchMaxErrors,
		Message:    fmt.Sprintf(msgWatchMaxErrors, MaxWatchErrors, lastErr),
		Category:   clierrors.CategoryConnection,
		Retriable:  true,
		Suggestion: suggestWatchMaxErrors,
	}
}
