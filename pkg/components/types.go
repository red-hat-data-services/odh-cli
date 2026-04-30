package components

import (
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/output"
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

// ComponentList wraps a slice of ComponentInfo with a self-describing envelope.
type ComponentList struct {
	output.Envelope

	Components []ComponentInfo `json:"components" yaml:"components"`
}

// NewComponentList creates a new ComponentList with envelope fields populated.
func NewComponentList(components []ComponentInfo) *ComponentList {
	list := &ComponentList{
		Envelope:   output.NewEnvelope("ComponentList", "components-list"),
		Components: components,
	}
	list.computeStatus()

	return list
}

// computeStatus calculates the Status based on component health.
func (l *ComponentList) computeStatus() {
	var warnings, errs int
	for _, c := range l.Components {
		if !c.IsActive() {
			continue
		}
		if c.Ready == nil {
			warnings++
		} else if !*c.Ready {
			errs++
		}
	}
	l.SetStatus(warnings, errs)
}

// IsActive returns true if the component is Managed or Unmanaged (not Removed).
func (c ComponentInfo) IsActive() bool {
	return c.ManagementState == constants.ManagementStateManaged ||
		c.ManagementState == constants.ManagementStateUnmanaged
}

// ComponentDetailsResult wraps ComponentDetails with a self-describing envelope.
type ComponentDetailsResult struct {
	output.Envelope

	Component ComponentDetails `json:"component" yaml:"component"`
}

// NewComponentDetailsResult creates a new ComponentDetailsResult with envelope fields populated.
func NewComponentDetailsResult(details ComponentDetails) *ComponentDetailsResult {
	result := &ComponentDetailsResult{
		Envelope:  output.NewEnvelope("ComponentDetails", "components-describe"),
		Component: details,
	}
	result.computeStatus()

	return result
}

// computeStatus calculates the Status based on component health.
func (r *ComponentDetailsResult) computeStatus() {
	// Skip status computation for inactive (Removed) components
	if r.Component.ManagementState != constants.ManagementStateManaged &&
		r.Component.ManagementState != constants.ManagementStateUnmanaged {
		r.SetStatus(0, 0)

		return
	}

	var warnings, errs int
	if r.Component.Ready == nil {
		warnings++
	} else if !*r.Component.Ready {
		errs++
	}
	r.SetStatus(warnings, errs)
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
