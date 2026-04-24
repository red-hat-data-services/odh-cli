package components

import (
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	readyYes     = "Yes"
	readyNo      = "No"
	readyUnknown = "?"
)

// ComponentInfo holds the state and health information for a DSC component.
type ComponentInfo struct {
	Name            string `json:"name"`
	ManagementState string `json:"managementState"`
	Ready           *bool  `json:"ready,omitempty"`
	Message         string `json:"message,omitempty"`
}

// ComponentList wraps a slice of ComponentInfo for JSON/YAML output.
type ComponentList struct {
	Components []ComponentInfo `json:"components"`
}

// IsActive returns true if the component is Managed or Unmanaged (not Removed).
func (c ComponentInfo) IsActive() bool {
	return c.ManagementState == constants.ManagementStateManaged ||
		c.ManagementState == constants.ManagementStateUnmanaged
}

// ErrComponentNotFound creates a structured error for unknown components.
func ErrComponentNotFound(name string, available []string) *clierrors.StructuredError {
	suggestion := "Available components: " + strings.Join(available, ", ")

	return &clierrors.StructuredError{
		Code:       "COMPONENT_NOT_FOUND",
		Message:    fmt.Sprintf("component %q not found in DataScienceCluster", name),
		Category:   clierrors.CategoryNotFound,
		Retriable:  false,
		Suggestion: suggestion,
	}
}

// ErrInvalidOutputFormat creates a structured error for invalid output formats.
func ErrInvalidOutputFormat(format string) *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       "INVALID_OUTPUT_FORMAT",
		Message:    fmt.Sprintf("invalid output format %q (must be one of: table, json, yaml)", format),
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: "Use --output with one of: table, json, yaml",
	}
}

// ErrUserAborted creates a structured error when user cancels an operation.
func ErrUserAborted() *clierrors.StructuredError {
	return &clierrors.StructuredError{
		Code:       "USER_ABORTED",
		Message:    "aborted by user",
		Category:   clierrors.CategoryValidation,
		Retriable:  false,
		Suggestion: "Use --yes flag to skip confirmation",
	}
}
