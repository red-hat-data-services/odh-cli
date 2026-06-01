package rbac

import (
	"context"
	"errors"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// PermissionCheck describes a single Kubernetes RBAC permission to verify.
type PermissionCheck struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string // empty = cluster-scoped
}

func (p PermissionCheck) String() string {
	scope := "cluster"
	if p.Namespace != "" {
		scope = p.Namespace
	}

	groupSuffix := ""
	if p.Group != "" {
		groupSuffix = "." + p.Group
	}

	return fmt.Sprintf("%s %s%s [%s]", p.Verb, p.Resource, groupSuffix, scope)
}

// CheckPermissions verifies that the current user has all the specified permissions.
// Returns the list of denied checks. An empty slice means all permissions are granted.
func CheckPermissions(
	ctx context.Context,
	authClient authorizationv1client.AuthorizationV1Interface,
	checks []PermissionCheck,
) ([]PermissionCheck, error) {
	if authClient == nil {
		return nil, errors.New("authorization client is nil")
	}

	var denied []PermissionCheck

	for _, check := range checks {
		review := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:      check.Verb,
					Group:     check.Group,
					Resource:  check.Resource,
					Namespace: check.Namespace,
				},
			},
		}

		result, err := authClient.SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to check permission %s: %w", check, err)
		}

		if !result.Status.Allowed {
			denied = append(denied, check)
		}
	}

	return denied, nil
}
