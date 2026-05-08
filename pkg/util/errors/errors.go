package errors

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"os"

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
	suggestionTLS             = "Verify the certificate authority bundle, check the server certificate validity, and ensure the hostname matches the certificate"
	suggestionDNS             = "Verify the server hostname in your kubeconfig is correct and DNS is configured"
	suggestionPermission      = "Check file and directory permissions for the target path"
	suggestionLintAdvisory    = "Review the advisory findings before proceeding with the upgrade"
	suggestionLintBlocked     = "Address the prohibited or blocking findings before proceeding with the upgrade"
	suggestionInputValidation = "Check the command arguments and flags"
)

// errorEntry maps an error-check function to its structured error fields.
type errorEntry struct {
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
var apiErrorTable = []errorEntry{
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

// localErrorTable classifies non-Kubernetes errors by inspecting the error
// chain for well-known Go types (context, filesystem, TLS, DNS, net).
// Order matters: specific checks (e.g. isTLSError, isDNSError) must appear
// before broader ones (e.g. isNetworkError) that match the same underlying types.
//
//nolint:gochecknoglobals // package-level lookup table is intentional
var localErrorTable = []errorEntry{
	{isDeadlineExceeded, "TIMEOUT", CategoryTimeout, true, suggestionTimeout},
	{isContextCanceled, "CANCELED", CategoryInternal, false, suggestionCanceled},
	{isPermissionError, "PERMISSION_DENIED", CategoryValidation, false, suggestionPermission},
	{isFilesystemError, "CONFIG_INVALID", CategoryValidation, false, suggestionFilePath},
	{isConfigError, "CONFIG_INVALID", CategoryValidation, false, suggestionConfig},
	{isTLSError, "TLS_CERT_INVALID", CategoryAuthentication, false, suggestionTLS},
	{isDNSError, "DNS_FAILED", CategoryConnection, true, suggestionDNS},
	{isNetworkTimeout, "NET_TIMEOUT", CategoryTimeout, true, suggestionTimeout},
	{isNetworkError, "CONN_FAILED", CategoryConnection, true, suggestionConnection},
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

	if result := matchEntry(apiErrorTable, err); result != nil {
		return result
	}

	if result := matchEntry(localErrorTable, err); result != nil {
		return result
	}

	// When an ExitCodeError wraps a plain (unclassifiable) error, derive the
	// structured fields from the explicit exit code so the JSON/YAML output
	// reflects the intended category rather than falling through to INTERNAL.
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return classifyFromExitCode(exitErr.Code, err)
	}

	return &StructuredError{
		Code:       "INTERNAL",
		Message:    err.Error(),
		Category:   CategoryInternal,
		ExitCode:   int(ExitCodeFromCategory(CategoryInternal)),
		Retriable:  false,
		Suggestion: suggestionInternal,
		cause:      err,
	}
}

// classifyFromExitCode builds a StructuredError from an explicit ExitCode
// when the wrapped error itself is not classifiable.
func classifyFromExitCode(code ExitCode, err error) *StructuredError {
	switch code { //nolint:exhaustive // ExitSuccess is not an error
	case ExitWarning:
		return &StructuredError{
			Code: "LINT_ADVISORY", Message: err.Error(),
			Category: CategoryValidation, ExitCode: int(ExitWarning),
			Suggestion: suggestionLintAdvisory, cause: err,
		}
	case ExitValidation:
		return &StructuredError{
			Code: "VALIDATION_FAILED", Message: err.Error(),
			Category: CategoryValidation, ExitCode: int(ExitValidation),
			Suggestion: suggestionInputValidation, cause: err,
		}
	case ExitAuth:
		return &StructuredError{
			Code: "AUTH_FAILED", Message: err.Error(),
			Category: CategoryAuthentication, ExitCode: int(ExitAuth),
			Suggestion: suggestionAuthentication, cause: err,
		}
	case ExitConnection:
		return &StructuredError{
			Code: "CONN_FAILED", Message: err.Error(),
			Category: CategoryConnection, ExitCode: int(ExitConnection),
			Retriable: true, Suggestion: suggestionConnection, cause: err,
		}
	case ExitError:
		// Check if this is a lint finding (prohibited/blocking) vs general error
		if errors.Is(err, ErrLintBlocked) {
			return &StructuredError{
				Code: "LINT_BLOCKED", Message: err.Error(),
				Category: CategoryValidation, ExitCode: int(ExitError),
				Suggestion: suggestionLintBlocked, cause: err,
			}
		}

		fallthrough
	default:
		return &StructuredError{
			Code: "INTERNAL", Message: err.Error(),
			Category: CategoryInternal, ExitCode: int(ExitError),
			Suggestion: suggestionInternal, cause: err,
		}
	}
}

func matchEntry(table []errorEntry, err error) *StructuredError {
	for _, entry := range table {
		if entry.check(err) {
			return &StructuredError{
				Code:       entry.code,
				Message:    err.Error(),
				Category:   entry.category,
				ExitCode:   int(ExitCodeFromCategory(entry.category)),
				Retriable:  entry.retriable,
				Suggestion: entry.suggestion,
				cause:      err,
			}
		}
	}

	return nil
}

func isDeadlineExceeded(err error) bool { return errors.Is(err, context.DeadlineExceeded) }
func isContextCanceled(err error) bool  { return errors.Is(err, context.Canceled) }

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

func isTLSError(err error) bool {
	var unknownAuth x509.UnknownAuthorityError
	var certInvalid x509.CertificateInvalidError
	var hostnameErr x509.HostnameError
	var recordHeader tls.RecordHeaderError

	return errors.As(err, &unknownAuth) ||
		errors.As(err, &certInvalid) ||
		errors.As(err, &hostnameErr) ||
		errors.As(err, &recordHeader)
}

func isDNSError(err error) bool {
	var dnsErr *net.DNSError

	return errors.As(err, &dnsErr)
}

func isPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission)
}

func isNetworkTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}
