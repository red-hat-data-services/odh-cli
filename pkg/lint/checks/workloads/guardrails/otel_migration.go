package guardrails

import (
	"context"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypeOtelConfigCompatible = "OtelConfigCompatible"
)

// OtelMigrationCheck detects GuardrailsOrchestrator CRs using deprecated otelExporter configuration fields.
type OtelMigrationCheck struct {
	check.BaseCheck
}

func NewOtelMigrationCheck() *OtelMigrationCheck {
	return &OtelMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeConfigMigration,
			CheckID:          "workloads.guardrails.otel-config-migration",
			CheckName:        "Workloads :: Guardrails :: OTEL Config Migration (3.x)",
			CheckDescription: "Detects GuardrailsOrchestrator CRs using deprecated otelExporter configuration fields that need migration",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x and TrustyAI is Managed.
func (c *OtelMigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "trustyai", check.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *OtelMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.GuardrailsOrchestrator).
		Filter(hasDeprecatedOtelFields).
		Complete(ctx, c.newOtelMigrationCondition)
}
