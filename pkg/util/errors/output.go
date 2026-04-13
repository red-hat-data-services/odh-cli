package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"
)

// WriteSuggestion classifies an error and renders only the suggestion line.
func WriteSuggestion(w io.Writer, err error) {
	if err == nil {
		return
	}

	var structErr *StructuredError
	if !errors.As(err, &structErr) {
		structErr = Classify(err)
	}

	_, _ = fmt.Fprintf(w, "Suggestion: %s\n", structErr.Suggestion)
}

// WriteTextError classifies an error and renders it as human-readable plain
// text with the suggestion included. Returns true if the error was written.
func WriteTextError(w io.Writer, err error) bool {
	if err == nil {
		return false
	}

	var structErr *StructuredError
	if !errors.As(err, &structErr) {
		structErr = Classify(err)
	}

	if _, writeErr := fmt.Fprintf(w, "%s\nSuggestion: %s\n", structErr.Message, structErr.Suggestion); writeErr != nil {
		return false
	}

	return true
}

// WriteStructuredError renders a structured error as JSON or YAML to the
// provided writer. Returns true if the error was rendered, false if the
// format is not json/yaml (caller should fall back to plain text).
func WriteStructuredError(w io.Writer, err error, format string) bool {
	if err == nil {
		return false
	}

	var structErr *StructuredError
	if !errors.As(err, &structErr) {
		structErr = Classify(err)
	}

	envelope := errorEnvelope{Error: structErr}

	switch format {
	case "json":
		data, marshalErr := json.MarshalIndent(envelope, "", "  ")
		if marshalErr != nil {
			if _, writeErr := fmt.Fprintf(w, "Error: %v\n", err); writeErr != nil {
				return false
			}

			return true
		}

		if _, writeErr := fmt.Fprintln(w, string(data)); writeErr != nil {
			return false
		}

		return true
	case "yaml":
		data, marshalErr := yaml.Marshal(envelope)
		if marshalErr != nil {
			if _, writeErr := fmt.Fprintf(w, "Error: %v\n", err); writeErr != nil {
				return false
			}

			return true
		}

		if _, writeErr := fmt.Fprint(w, string(data)); writeErr != nil {
			return false
		}

		return true
	default:
		return false
	}
}
