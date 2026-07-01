package status

import (
	"fmt"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	errCodeInvalidSection      = "INVALID_SECTION"
	errCodeInvalidLayer        = "INVALID_LAYER"
	errCodeInvalidOutputFormat = "INVALID_OUTPUT_FORMAT"
	errCodeInvalidTimeout      = "INVALID_TIMEOUT"
	errCodeDSCINotFound        = "DSCI_NOT_FOUND"

	msgInvalidSection      = "invalid section %q"
	msgInvalidLayer        = "invalid layer %q"
	msgInvalidOutputFormat = "invalid output format %q"
	msgInvalidTimeout      = "timeout must be greater than 0"
	msgDSCINotFound        = "no DSCInitialization found"
	msgHealthCheckRetry    = "Warning: health check failed (retrying): %v\n"
	msgWaitProgress        = "Waiting for %s status... (%s elapsed)\n"

	suggestValidSections   = "Valid sections: nodes, deployments, pods, events, quotas, operator, dsci, dsc"
	suggestValidLayers     = "Valid layers: infrastructure, workload, operator"
	suggestValidFormats    = "Use --output with one of: table, json, yaml"
	suggestValidTimeout    = "Use --timeout with a positive duration (e.g., 30s, 1m)"
	suggestInstallODHRHOAI = "Ensure ODH/RHOAI is installed on the cluster"
)

// ErrInvalidSection creates a structured error for invalid section names.
func ErrInvalidSection(section string) *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeInvalidSection,
		Message:    fmt.Sprintf(msgInvalidSection, section),
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestValidSections,
	}
}

// ErrInvalidLayer creates a structured error for invalid layer names.
func ErrInvalidLayer(layer string) *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeInvalidLayer,
		Message:    fmt.Sprintf(msgInvalidLayer, layer),
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestValidLayers,
	}
}

// ErrInvalidOutputFormat creates a structured error for invalid output formats.
func ErrInvalidOutputFormat(format string) *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeInvalidOutputFormat,
		Message:    fmt.Sprintf(msgInvalidOutputFormat, format),
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestValidFormats,
	}
}

// ErrInvalidTimeout creates a structured error for invalid timeout values.
func ErrInvalidTimeout() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeInvalidTimeout,
		Message:    msgInvalidTimeout,
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: suggestValidTimeout,
	}
}

// ErrNoDSCIFound creates a structured error when DSCInitialization is not found.
func ErrNoDSCIFound() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       errCodeDSCINotFound,
		Message:    msgDSCINotFound,
		Category:   clierrors.CategoryNotFound,
		Retriable:  false,
		Suggestion: suggestInstallODHRHOAI,
	}
}
