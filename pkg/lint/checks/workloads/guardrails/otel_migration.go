package guardrails

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/inspect"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypeOtelConfigCompatible = "OtelConfigCompatible"

	// minTargetMajorVersion is the minimum major version for this check to apply.
	minTargetMajorVersion = 3
)

// OtelMigrationCheck detects GuardrailsOrchestrator CRs using deprecated otelExporter configuration fields.
type OtelMigrationCheck struct {
	base.BaseCheck
}

func NewOtelMigrationCheck() *OtelMigrationCheck {
	return &OtelMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentGuardrails,
			CheckType:        check.CheckTypeConfigMigration,
			CheckID:          "workloads.guardrails.otel-config-migration",
			CheckName:        "Workloads :: Guardrails :: OTEL Config Migration (3.x)",
			CheckDescription: "Detects GuardrailsOrchestrator CRs using deprecated otelExporter configuration fields that need migration",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading to 3.x or later.
func (c *OtelMigrationCheck) CanApply(target check.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, minTargetMajorVersion, 0)
}

// Validate executes the check against the provided target.
func (c *OtelMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Find orchestrators with deprecated OTEL configuration
	impacted, err := c.findOrchestratorWithDeprecatedConfig(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// Add condition
	dr.Status.Conditions = append(dr.Status.Conditions,
		newOtelMigrationCondition(totalImpacted),
	)

	// Populate ImpactedObjects if any orchestrators found
	if totalImpacted > 0 {
		results.PopulateImpactedObjects(dr, resources.GuardrailsOrchestrator, impacted)
	}

	return dr, nil
}

func (c *OtelMigrationCheck) findOrchestratorWithDeprecatedConfig(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, error) {
	orchestrators, err := target.Client.ListResources(ctx, resources.GuardrailsOrchestrator.GVR())
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("listing GuardrailsOrchestrators: %w", err)
	}

	impacted := make([]types.NamespacedName, 0)

	for _, orch := range orchestrators {
		found, err := inspect.HasFields(orch.Object, deprecatedOtelExpressions...)
		if err != nil {
			return nil, fmt.Errorf("checking deprecated fields for %s/%s: %w",
				orch.GetNamespace(), orch.GetName(), err)
		}

		if len(found) > 0 {
			impacted = append(impacted, types.NamespacedName{
				Namespace: orch.GetNamespace(),
				Name:      orch.GetName(),
			})
		}
	}

	return impacted, nil
}
