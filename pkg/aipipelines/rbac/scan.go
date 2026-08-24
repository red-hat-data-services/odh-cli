package rbac

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// FindRolesNeedingFix scans cluster Roles and returns those missing the
// datasciencepipelinesapplications/api subresource while granting route.openshift.io access.
func FindRolesNeedingFix(ctx context.Context, r client.Reader) ([]Classification, error) {
	roleList, err := r.List(ctx, resources.Role)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}

	needsFix := make([]Classification, 0)

	for _, role := range roleList {
		classification := ClassifyRole(role)
		if classification.NeedsFix {
			needsFix = append(needsFix, classification)
		}
	}

	return needsFix, nil
}
