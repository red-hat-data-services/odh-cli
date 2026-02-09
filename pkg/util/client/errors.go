package client

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

// IsUnrecoverableError checks if an error is unrecoverable and should not be retried.
// Returns true for errors like Forbidden, Unauthorized, Invalid, MethodNotSupported, and NotAcceptable.
func IsUnrecoverableError(err error) bool {
	if apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) ||
		apierrors.IsInvalid(err) ||
		apierrors.IsMethodNotSupported(err) ||
		apierrors.IsNotAcceptable(err) {
		return true
	}

	return false
}

// IsResourceTypeNotFound checks if an error indicates the resource type/CRD doesn't exist.
// Catches both:
//   - meta.NoResourceMatchError from the REST mapper when the GVK/GVR is unknown
//   - 404 NotFound from the dynamic/metadata clients, which bypass the REST mapper
//     and hit the API server directly (returns 404 when the resource endpoint doesn't exist)
func IsResourceTypeNotFound(err error) bool {
	return meta.IsNoMatchError(err) || apierrors.IsNotFound(err)
}

// IsPermissionError checks if an error is due to insufficient permissions.
// Returns true for Forbidden (403) and Unauthorized (401) errors.
func IsPermissionError(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
}
