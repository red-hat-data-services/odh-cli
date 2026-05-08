package errors

import "errors"

// ExitCode represents a CLI process exit code.
type ExitCode int

const (
	ExitSuccess    ExitCode = 0 // Process completed without issues
	ExitError      ExitCode = 1 // General runtime/unexpected errors
	ExitWarning    ExitCode = 2 // Process finished, but advisory warnings were found
	ExitValidation ExitCode = 3 // Invalid user input or configuration errors
	ExitAuth       ExitCode = 4 // Authentication or authorization failures
	ExitConnection ExitCode = 5 // Network issues, timeouts, or downstream service unavailability
)

const (
	prioritySuccess    = iota // lowest
	priorityWarning           // advisory findings
	priorityError             // runtime errors
	priorityValidation        // input / config errors
	priorityAuth              // authentication / authorization
	priorityConnection        // network / timeout (highest)
)

// exitCodePriority defines the precedence of each exit code.
// Higher value = higher priority. Used by HigherPriority to resolve
// conflicts when multiple error categories occur.
//
//nolint:gochecknoglobals // package-level lookup table is intentional
var exitCodePriority = map[ExitCode]int{
	ExitSuccess:    prioritySuccess,
	ExitWarning:    priorityWarning,
	ExitError:      priorityError,
	ExitValidation: priorityValidation,
	ExitAuth:       priorityAuth,
	ExitConnection: priorityConnection,
}

// ExitCodeFromCategory maps an ErrorCategory to its corresponding ExitCode.
func ExitCodeFromCategory(category ErrorCategory) ExitCode {
	switch category {
	case CategoryAuthentication, CategoryAuthorization:
		return ExitAuth
	case CategoryConnection, CategoryTimeout:
		return ExitConnection
	case CategoryValidation:
		return ExitValidation
	case CategoryServer, CategoryNotFound, CategoryConflict, CategoryInternal:
		return ExitError
	}

	return ExitError
}

// ExitCodeFromError inspects the error chain and returns the appropriate ExitCode.
// It checks for ExitCodeError first (explicit exit code), then falls back to
// classifying the error via Classify().
func ExitCodeFromError(err error) ExitCode {
	if err == nil {
		return ExitSuccess
	}

	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}

	var structErr *StructuredError
	if errors.As(err, &structErr) {
		// Honor explicit ExitCode if set; otherwise derive from Category.
		if structErr.ExitCode != 0 {
			return ExitCode(structErr.ExitCode)
		}

		return ExitCodeFromCategory(structErr.Category)
	}

	classified := Classify(err)

	return ExitCodeFromCategory(classified.Category)
}

// priority returns the precedence of code. Unknown codes default to
// priorityError so they are never silently treated as success.
func priority(code ExitCode) int {
	if p, ok := exitCodePriority[code]; ok {
		return p
	}

	return priorityError
}

// IsHigherPriority reports whether a has strictly higher precedence than b.
// Precedence order: Connection(5) > Auth(4) > Validation(3) > Error(1) > Warning(2) > Success(0).
func IsHigherPriority(a, b ExitCode) bool {
	return priority(a) > priority(b)
}
