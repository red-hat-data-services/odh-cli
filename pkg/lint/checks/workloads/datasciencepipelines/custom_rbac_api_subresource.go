package datasciencepipelines

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	pipelinescomponent "github.com/opendatahub-io/odh-cli/pkg/aipipelines"
	pipelinesrbac "github.com/opendatahub-io/odh-cli/pkg/aipipelines/rbac"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	checkTypeCustomRBACAPISubresource = "custom-rbac-api-subresource"

	msgCustomRBACFound = "Found %d custom Role(s) with route.openshift.io permissions missing the datasciencepipelinesapplications/api subresource required in RHOAI %s"
	msgCustomRBACClear = "No custom Roles missing datasciencepipelinesapplications/api subresource - ready for RHOAI %s upgrade"
)

// CustomRBACAPISubresourceCheck validates that custom Roles grant the DSPA API subresource
// required after the kube-rbac-proxy migration in RHOAI 3.x.
type CustomRBACAPISubresourceCheck struct {
	check.BaseCheck
}

// NewCustomRBACAPISubresourceCheck creates a new CustomRBACAPISubresourceCheck.
func NewCustomRBACAPISubresourceCheck() *CustomRBACAPISubresourceCheck {
	return &CustomRBACAPISubresourceCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             checkTypeCustomRBACAPISubresource,
			CheckID:          "workloads.datasciencepipelines.custom-rbac-api-subresource",
			CheckName:        "Workloads :: DataSciencePipelines :: Custom RBAC API Subresource (3.x)",
			CheckDescription: "Validates that custom Roles with route.openshift.io permissions also grant the datasciencepipelinesapplications/api subresource required after the kube-rbac-proxy migration in RHOAI 3.x",
			CheckRemediation: "Run 'kubectl odh migrate run -m ai-pipelines.update-dsp-role' to patch affected Roles",
		},
	}
}

// CanApply returns whether this check should run for the given target.
func (c *CustomRBACAPISubresourceCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	apply, err := pipelinescomponent.ShouldApplyChecks(ctx, dsc, target.Client)
	if err != nil {
		return false, fmt.Errorf("checking AI Pipelines component applicability: %w", err)
	}

	return apply, nil
}

// Validate scans cluster Roles for missing datasciencepipelinesapplications/api permissions.
func (c *CustomRBACAPISubresourceCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()
	tv := version.MajorMinorLabel(target.TargetVersion)

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	roles, err := pipelinesrbac.FindRolesNeedingFix(ctx, target.Client)
	if err != nil {
		return nil, fmt.Errorf("scanning roles for DSPA API subresource gaps: %w", err)
	}

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(roles))

	if len(roles) > 0 {
		impacted := make([]types.NamespacedName, 0, len(roles))
		for _, role := range roles {
			impacted = append(impacted, types.NamespacedName{
				Namespace: role.Namespace,
				Name:      role.RoleName,
			})
		}

		dr.SetImpactedObjects(resources.Role, impacted)
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage(msgCustomRBACFound, len(roles), tv),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage(msgCustomRBACClear, tv),
	))

	return dr, nil
}
