package datasciencepipelines

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	pipelinescomponent "github.com/opendatahub-io/odh-cli/pkg/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/inspect"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind                        = "datasciencepipelines"
	checkTypeInstructLabRemoval = "instructlab-removal"

	msgDeprecatedManagedPipelineFieldsFound = "Found %d DataSciencePipelinesApplication(s) with deprecated apiServer managed-pipelines fields removed in RHOAI %s"
	msgDeprecatedManagedPipelineFieldsClear = "No DataSciencePipelinesApplications found using deprecated apiServer managed-pipelines fields - ready for RHOAI %s upgrade"
)

//nolint:gochecknoglobals // Deprecated DSPA apiServer field paths removed in RHOAI 3.x
var deprecatedAPIServerFieldPaths = []string{
	".spec.apiServer.managedPipelines.instructLab",
	".spec.apiServer.runtimeGenericImage",
	".spec.apiServer.toolboxImage",
	".spec.apiServer.rhelAIImage",
	".spec.apiServer.initResources",
}

type InstructLabRemovalCheck struct {
	check.BaseCheck
}

func NewInstructLabRemovalCheck() *InstructLabRemovalCheck {
	return &InstructLabRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             checkTypeInstructLabRemoval,
			CheckID:          "workloads.datasciencepipelines.instructlab-removal",
			CheckName:        "Workloads :: DataSciencePipelines :: Deprecated apiServer Managed-Pipelines Fields (3.x)",
			CheckDescription: "Validates that DSPA objects do not use deprecated apiServer fields before upgrading to RHOAI 3.x",
			CheckRemediation: "Remove .spec.apiServer.managedPipelines.instructLab and the deprecated .spec.apiServer fields runtimeGenericImage, toolboxImage, rhelAIImage, and initResources from affected DSPA objects before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check applies when upgrading FROM 2.x TO 3.x and AI Pipelines is Managed
// under either DSC component key, or when DSPAs remain on the cluster.
func (c *InstructLabRemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
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

func (c *InstructLabRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()
	tv := version.MajorMinorLabel(target.TargetVersion)

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	state, err := pipelinescomponent.EffectiveManagementState(dsc)
	if err != nil {
		return nil, fmt.Errorf("resolving AI Pipelines management state: %w", err)
	}

	dr.Annotations[check.AnnotationComponentManagementState] = state

	dspas, usedResourceType, err := pipelinescomponent.ListDSPAs(ctx, target.Client)
	if err != nil {
		return nil, fmt.Errorf("listing DataSciencePipelinesApplications: %w", err)
	}

	impactedDSPAs := make([]types.NamespacedName, 0)

	for i := range dspas {
		dspa := dspas[i]

		found, err := inspect.HasFields(dspa, deprecatedAPIServerFieldPaths...)
		if err != nil {
			return nil, fmt.Errorf("querying deprecated apiServer fields for DSPA %s/%s: %w",
				dspa.GetNamespace(), dspa.GetName(), err)
		}

		if len(found) == 0 {
			continue
		}

		impactedDSPAs = append(impactedDSPAs, types.NamespacedName{
			Namespace: dspa.GetNamespace(),
			Name:      dspa.GetName(),
		})
	}

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(impactedDSPAs))

	if len(impactedDSPAs) > 0 {
		dr.SetImpactedObjects(usedResourceType, impactedDSPAs)
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonFeatureRemoved),
			check.WithMessage(msgDeprecatedManagedPipelineFieldsFound, len(impactedDSPAs), tv),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage(msgDeprecatedManagedPipelineFieldsClear, tv),
	))

	return dr, nil
}
