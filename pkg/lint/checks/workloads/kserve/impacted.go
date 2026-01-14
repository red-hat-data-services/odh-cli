package kserve

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	annotationDeploymentMode = "serving.kserve.io/deploymentMode"
	deploymentModeModelMesh  = "ModelMesh"
	deploymentModeServerless = "Serverless"
)

const (
	ConditionTypeServerlessISVCCompatible = "ServerlessInferenceServicesCompatible"
	ConditionTypeModelMeshISVCCompatible  = "ModelMeshInferenceServicesCompatible"
	ConditionTypeModelMeshSRCompatible    = "ModelMeshServingRuntimesCompatible"
)

type impactedInferenceServices struct {
	serverless []types.NamespacedName
	modelMesh  []types.NamespacedName
}

// ImpactedWorkloadsCheck lists InferenceServices and ServingRuntimes using deprecated deployment modes.
type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentKServe,
			CheckType:        check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.impacted-workloads",
			CheckName:        "Workloads :: KServe :: Impacted Workloads (3.x)",
			CheckDescription: "Lists InferenceServices and ServingRuntimes using deprecated deployment modes (ModelMesh, Serverless) that will be impacted in RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x since KServe workloads may be impacted.
func (c *ImpactedWorkloadsCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Find impacted workloads by category
	isvcsByMode, err := c.findImpactedInferenceServices(ctx, target)
	if err != nil {
		return nil, err
	}

	impactedSRs, err := c.findImpactedServingRuntimes(ctx, target)
	if err != nil {
		return nil, err
	}

	// Calculate totals
	totalImpacted := len(isvcsByMode.serverless) + len(isvcsByMode.modelMesh) + len(impactedSRs)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// ALWAYS add all three conditions (even for zero counts per user requirement)
	dr.Status.Conditions = append(dr.Status.Conditions,
		newServerlessISVCCondition(len(isvcsByMode.serverless)),
		newModelMeshISVCCondition(len(isvcsByMode.modelMesh)),
		newModelMeshSRCondition(len(impactedSRs)),
	)

	// Populate ImpactedObjects if any workloads found
	if totalImpacted > 0 {
		populateImpactedObjects(dr, isvcsByMode, impactedSRs)
	}

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedInferenceServices(
	ctx context.Context,
	target check.Target,
) (impactedInferenceServices, error) {
	inferenceServices, err := target.Client.ListMetadata(ctx, resources.InferenceService)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return impactedInferenceServices{}, nil
		}

		return impactedInferenceServices{}, fmt.Errorf("listing InferenceServices: %w", err)
	}

	var isvcs impactedInferenceServices

	for _, isvc := range inferenceServices {
		annotations := isvc.GetAnnotations()
		mode := annotations[annotationDeploymentMode]

		namespacedName := types.NamespacedName{
			Namespace: isvc.GetNamespace(),
			Name:      isvc.GetName(),
		}

		switch mode {
		case deploymentModeServerless:
			isvcs.serverless = append(isvcs.serverless, namespacedName)
		case deploymentModeModelMesh:
			isvcs.modelMesh = append(isvcs.modelMesh, namespacedName)
		}
	}

	return isvcs, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedServingRuntimes(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, error) {
	servingRuntimes, err := target.Client.List(ctx, resources.ServingRuntime)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("listing ServingRuntimes: %w", err)
	}

	impacted := make([]types.NamespacedName, 0, len(servingRuntimes))

	for _, sr := range servingRuntimes {
		// Check for ModelMesh using .spec.multiModel field
		multiModel, err := jq.Query[bool](&sr, ".spec.multiModel")
		if err != nil || !multiModel {
			continue
		}

		impacted = append(impacted, types.NamespacedName{
			Namespace: sr.GetNamespace(),
			Name:      sr.GetName(),
		})
	}

	return impacted, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(NewImpactedWorkloadsCheck())
}
