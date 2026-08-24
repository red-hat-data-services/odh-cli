package aipipelines

import (
	"context"
	"fmt"

	pipelinesrbac "github.com/opendatahub-io/odh-cli/pkg/aipipelines/rbac"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

func findRolesNeedingFix(
	ctx context.Context,
	c client.Client,
	recorder action.StepRecorder,
) ([]pipelinesrbac.Classification, error) {
	step := recorder.Child("scan-roles", "Scan custom roles for RBAC gaps")

	roles, err := pipelinesrbac.FindRolesNeedingFix(ctx, c)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list roles: %v", err)

		return nil, fmt.Errorf("scanning roles for RBAC gaps: %w", err)
	}

	if len(roles) == 0 {
		step.Completef(result.StepCompleted, "No custom roles need updating")
	} else {
		for _, r := range roles {
			step.Recordf(
				r.RoleName,
				"Role %s in namespace %s needs datasciencepipelinesapplications/api subresource",
				result.StepCompleted,
				r.RoleName,
				r.Namespace,
			)
		}

		step.Completef(result.StepCompleted, "Found %d role(s) needing update", len(roles))
	}

	return roles, nil
}
