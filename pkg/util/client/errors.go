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
func IsResourceTypeNotFound(err error) bool {
	return meta.IsNoMatchError(err)
}
