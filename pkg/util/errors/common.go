package errors

import "fmt"

// Common error codes.
const (
	CodeConfigFailed    = "CONFIG_FAILED"
	CodeClientFailed    = "CLIENT_FAILED"
	CodeNamespaceFailed = "NAMESPACE_FAILED"
	CodeDSCIFailed      = "DSCI_FAILED"
	CodeNotFound        = "NOT_FOUND"
	CodeRenderFailed    = "RENDER_FAILED"
)

// Common suggestions.
const (
	SuggestCheckKubeconfig = "Verify your kubeconfig is valid and the cluster is reachable"
	SuggestCheckCluster    = "Ensure the cluster is running and accessible"
	SuggestCheckODH        = "Ensure ODH/RHOAI is properly installed"
	SuggestRetry           = "Try again; if the problem persists, check cluster connectivity"
)

// classifyOrWrap attempts to classify the error. If classification succeeds,
// it prepends the context message. Otherwise, it creates a new StructuredError.
// Returns a copy to avoid mutating the original error in the chain.
func classifyOrWrap(err error, code, context, suggestion string, category ErrorCategory, retriable bool) *StructuredError {
	if classified := Classify(err); classified != nil {
		wrapped := *classified
		wrapped.Message = fmt.Sprintf("%s: %s", context, classified.Message)

		return &wrapped
	}

	return &StructuredError{
		Code:       code,
		Message:    fmt.Sprintf("%s: %s", context, err.Error()),
		Category:   category,
		Retriable:  retriable,
		Suggestion: suggestion,
	}
}

// ErrConfigFailed creates a structured error for REST config creation failures.
func ErrConfigFailed(err error) *StructuredError {
	return classifyOrWrap(err, CodeConfigFailed, "failed to create REST config",
		SuggestCheckKubeconfig, CategoryValidation, false)
}

// ErrClientFailed creates a structured error for Kubernetes client creation failures.
func ErrClientFailed(err error) *StructuredError {
	return classifyOrWrap(err, CodeClientFailed, "failed to create Kubernetes client",
		SuggestCheckCluster, CategoryConnection, true)
}

// ErrCRClientFailed creates a structured error for controller-runtime client creation failures.
func ErrCRClientFailed(err error) *StructuredError {
	return classifyOrWrap(err, CodeClientFailed, "failed to create controller-runtime client",
		SuggestCheckCluster, CategoryConnection, true)
}

// ErrNamespaceFailed creates a structured error for namespace determination failures.
func ErrNamespaceFailed(err error) *StructuredError {
	return classifyOrWrap(err, CodeNamespaceFailed, "failed to determine namespace from kubeconfig",
		SuggestCheckKubeconfig, CategoryValidation, false)
}

// ErrDSCIFailed creates a structured error for DSCInitialization retrieval failures.
func ErrDSCIFailed(err error) *StructuredError {
	return classifyOrWrap(err, CodeDSCIFailed, "failed to get DSCInitialization",
		SuggestCheckODH, CategoryInternal, true)
}

// ErrNoNamespacesDiscovered creates a structured error when no ODH namespaces could be found.
func ErrNoNamespacesDiscovered() *StructuredError {
	return &StructuredError{
		Code:       CodeNotFound,
		Message:    "no ODH namespaces discovered",
		Category:   CategoryNotFound,
		Retriable:  false,
		Suggestion: SuggestCheckODH,
	}
}

// ErrOperatorNamespaceNotFound creates a structured error when operator namespace cannot be discovered.
func ErrOperatorNamespaceNotFound() *StructuredError {
	return &StructuredError{
		Code:       CodeNotFound,
		Message:    "could not discover operator namespace",
		Category:   CategoryNotFound,
		Retriable:  false,
		Suggestion: "Use --operator-namespace to specify the operator namespace",
	}
}

// ErrRenderFailed creates a structured error for output rendering failures.
func ErrRenderFailed(format string, err error) *StructuredError {
	return &StructuredError{
		Code:       CodeRenderFailed,
		Message:    fmt.Sprintf("failed to render %s output: %s", format, err.Error()),
		Category:   CategoryInternal,
		Retriable:  false,
		Suggestion: SuggestRetry,
	}
}

// ErrEventsFetchFailed creates a structured error when fetching events fails.
func ErrEventsFetchFailed(err error) *StructuredError {
	return classifyOrWrap(err, "EVENTS_FETCH_FAILED", "failed to fetch events",
		SuggestRetry, CategoryInternal, true)
}
