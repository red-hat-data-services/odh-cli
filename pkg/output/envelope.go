package output

import (
	"time"

	"github.com/opendatahub-io/odh-cli/internal/version"
)

// APIVersion is the shared schema version for all CLI structured output.
// Consumers can branch on this value to handle format changes across CLI releases.
const APIVersion = "cli.opendatahub.io/v1"

// Status result constants.
const (
	StatusSuccess = "success"
	StatusWarning = "warning"
	StatusFailure = "failure"
)

// Metadata contains contextual information about when and how the output was produced.
// Shared across all command output types for consistent self-describing envelopes.
type Metadata struct {
	// GeneratedAt is the RFC3339 timestamp of when the output was produced
	GeneratedAt string `json:"generatedAt" yaml:"generatedAt"`

	// Command identifies which CLI command generated this output
	Command string `json:"command" yaml:"command"`

	// CLIVersion is the semantic version of the CLI binary that produced this output
	CLIVersion string `json:"cliVersion" yaml:"cliVersion"`
}

// Status provides a summary of the command execution result.
// Included in output envelope for quick pass/fail determination.
type Status struct {
	// Result indicates overall outcome: "success", "warning", or "failure"
	Result string `json:"result" yaml:"result"`

	// Warnings count of non-blocking issues found
	Warnings int `json:"warnings" yaml:"warnings"`

	// Errors count of blocking issues found
	Errors int `json:"errors" yaml:"errors"`
}

// NewMetadata creates a Metadata with the current timestamp and CLI version.
// Each command passes its own name (e.g., "lint", "backup", "status").
func NewMetadata(command string) Metadata {
	return Metadata{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Command:     command,
		CLIVersion:  version.GetVersion(),
	}
}

// NewStatus creates a Status with auto-determined result based on counts.
// Result is "failure" if errors > 0, "warning" if warnings > 0, otherwise "success".
// Negative inputs are clamped to zero.
func NewStatus(warnings, errors int) Status {
	if warnings < 0 {
		warnings = 0
	}
	if errors < 0 {
		errors = 0
	}

	result := StatusSuccess
	if errors > 0 {
		result = StatusFailure
	} else if warnings > 0 {
		result = StatusWarning
	}

	return Status{
		Result:   result,
		Warnings: warnings,
		Errors:   errors,
	}
}

// Envelope contains the common envelope fields for all command outputs.
// Embed this struct in command-specific output types to get consistent
// self-describing envelopes without duplicating field definitions.
//
// Example usage:
//
//	type ComponentList struct {
//	    output.Envelope
//	    Components []ComponentInfo `json:"components"`
//	}
type Envelope struct {
	APIVersion string   `json:"apiVersion"       yaml:"apiVersion"`
	Kind       string   `json:"kind"             yaml:"kind"`
	Metadata   Metadata `json:"metadata"         yaml:"metadata"`
	Status     *Status  `json:"status,omitempty" yaml:"status,omitempty"`
}

// NewEnvelope creates an Envelope with standard fields populated.
// The kind parameter should be the output type name (e.g., "ComponentList").
// The command parameter identifies which CLI command produced the output.
func NewEnvelope(kind, command string) Envelope {
	return Envelope{
		APIVersion: APIVersion,
		Kind:       kind,
		Metadata:   NewMetadata(command),
	}
}

// SetStatus sets the status on the envelope based on warning/error counts.
func (e *Envelope) SetStatus(warnings, errors int) {
	status := NewStatus(warnings, errors)
	e.Status = &status
}
