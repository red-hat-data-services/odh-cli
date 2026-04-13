package errors

import (
	"context"
	"errors"
	"io/fs"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	suggestionAuthentication  = "Refresh your kubeconfig credentials with 'oc login' or 'kubectl config'"
	suggestionAuthorization   = "Verify your RBAC permissions for the required resources"
	suggestionConnection      = "Check if the API server is reachable"
	suggestionNotFound        = "Verify the resource exists in the cluster"
	suggestionAlreadyExists   = "Resource already exists, use update or delete first"
	suggestionConflict        = "Retry the operation (resource was modified concurrently)"
	suggestionValidation      = "Check the request parameters and resource spec"
	suggestionGone            = "Resource version expired, retry with a fresh list/watch"
	suggestionServer          = "API server error, retry later"
	suggestionTimeout         = "Increase --timeout value or retry the operation"
	suggestionRateLimited     = "Too many requests, retry after a brief wait"
	suggestionRequestTooLarge = "Reduce the size of the request payload"
	suggestionInternal        = "Unexpected error, please report a bug"
	suggestionCanceled        = "Operation was canceled"
	suggestionFilePath        = "Verify the file path exists and is readable (e.g. --kubeconfig)"
	suggestionConfig          = "Check your kubeconfig: verify the --context, --cluster, and --kubeconfig flags are correct"
)

// apiErrorEntry maps an apierrors check function to its structured error fields.
type apiErrorEntry struct {
	check      func(error) bool
	code       string
	category   ErrorCategory
	retriable  bool
	suggestion string
}

// apiErrorTable defines the classification for every Kubernetes API error type.
// Order matters: more specific checks (e.g. IsUnexpectedServerError) must
// appear before broader ones (e.g. IsInternalError) that match the same status code.
//
//nolint:gochecknoglobals // package-level lookup table is intentional
var apiErrorTable = []apiErrorEntry{
	{apierrors.IsUnauthorized, "AUTH_FAILED", CategoryAuthentication, false, suggestionAuthentication},
	{apierrors.IsForbidden, "AUTHZ_DENIED", CategoryAuthorization, false, suggestionAuthorization},
	{apierrors.IsNotFound, "NOT_FOUND", CategoryNotFound, false, suggestionNotFound},
	{apierrors.IsAlreadyExists, "ALREADY_EXISTS", CategoryConflict, false, suggestionAlreadyExists},
	{apierrors.IsConflict, "CONFLICT", CategoryConflict, true, suggestionConflict},
	{apierrors.IsInvalid, "INVALID", CategoryValidation, false, suggestionValidation},
	{apierrors.IsBadRequest, "BAD_REQUEST", CategoryValidation, false, suggestionValidation},
	{apierrors.IsMethodNotSupported, "METHOD_NOT_SUPPORTED", CategoryValidation, false, suggestionValidation},
	{apierrors.IsNotAcceptable, "NOT_ACCEPTABLE", CategoryValidation, false, suggestionValidation},
	{apierrors.IsUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", CategoryValidation, false, suggestionValidation},
	{apierrors.IsRequestEntityTooLargeError, "REQUEST_TOO_LARGE", CategoryValidation, false, suggestionRequestTooLarge},
	{apierrors.IsGone, "GONE", CategoryServer, true, suggestionGone},
	{apierrors.IsResourceExpired, "RESOURCE_EXPIRED", CategoryServer, true, suggestionGone},
	{apierrors.IsServerTimeout, "SERVER_TIMEOUT", CategoryTimeout, true, suggestionTimeout},
	{apierrors.IsServiceUnavailable, "SERVER_UNAVAILABLE", CategoryServer, true, suggestionServer},
	{apierrors.IsUnexpectedServerError, "UNEXPECTED_SERVER_ERROR", CategoryServer, true, suggestionServer},
	{apierrors.IsInternalError, "SERVER_ERROR", CategoryServer, true, suggestionServer},
	{apierrors.IsTimeout, "GATEWAY_TIMEOUT", CategoryTimeout, true, suggestionTimeout},
	{apierrors.IsTooManyRequests, "RATE_LIMITED", CategoryServer, true, suggestionRateLimited},
	{apierrors.IsUnexpectedObjectError, "UNEXPECTED_OBJECT", CategoryServer, false, suggestionServer},
	{apierrors.IsStoreReadError, "STORE_READ_ERROR", CategoryServer, true, suggestionServer},
}

// Classify inspects an error and returns a StructuredError with the
// appropriate category, error code, retriable flag, and suggestion.
func Classify(err error) *StructuredError {
	if err == nil {
		return nil
	}

	var structuredErr *StructuredError
	if errors.As(err, &structuredErr) && structuredErr != nil {
		return structuredErr
	}

	for _, entry := range apiErrorTable {
		if entry.check(err) {
			return &StructuredError{
				Code:       entry.code,
				Message:    err.Error(),
				Category:   entry.category,
				Retriable:  entry.retriable,
				Suggestion: entry.suggestion,
				cause:      err,
			}
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &StructuredError{
			Code:       "TIMEOUT",
			Message:    err.Error(),
			Category:   CategoryTimeout,
			Retriable:  true,
			Suggestion: suggestionTimeout,
			cause:      err,
		}

	case errors.Is(err, context.Canceled):
		return &StructuredError{
			Code:       "CANCELED",
			Message:    err.Error(),
			Category:   CategoryInternal,
			Retriable:  false,
			Suggestion: suggestionCanceled,
			cause:      err,
		}

	case isFilesystemError(err):
		return &StructuredError{
			Code:       "CONFIG_INVALID",
			Message:    err.Error(),
			Category:   CategoryValidation,
			Retriable:  false,
			Suggestion: suggestionFilePath,
			cause:      err,
		}

	case isConfigError(err):
		return &StructuredError{
			Code:       "CONFIG_INVALID",
			Message:    err.Error(),
			Category:   CategoryValidation,
			Retriable:  false,
			Suggestion: suggestionConfig,
			cause:      err,
		}

	case isNetworkTimeout(err):
		return &StructuredError{
			Code:       "NET_TIMEOUT",
			Message:    err.Error(),
			Category:   CategoryTimeout,
			Retriable:  true,
			Suggestion: suggestionTimeout,
			cause:      err,
		}

	case isNetworkError(err):
		return &StructuredError{
			Code:       "CONN_FAILED",
			Message:    err.Error(),
			Category:   CategoryConnection,
			Retriable:  true,
			Suggestion: suggestionConnection,
			cause:      err,
		}

	default:
		return &StructuredError{
			Code:       "INTERNAL",
			Message:    err.Error(),
			Category:   CategoryInternal,
			Retriable:  false,
			Suggestion: suggestionInternal,
			cause:      err,
		}
	}
}

func isFilesystemError(err error) bool {
	var pathErr *fs.PathError

	return errors.As(err, &pathErr)
}

func isConfigError(err error) bool {
	var cfgErr *ConfigError

	return errors.As(err, &cfgErr)
}

func isNetworkError(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr)
}

func isNetworkTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}
