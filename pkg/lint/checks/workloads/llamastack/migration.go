package llamastack

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	ConditionTypeRequiresMigration = "RequiresMigration"
)

// MigrationCheck validates LlamaStackDistribution resources for 3.4→3.5 upgrade.
// In 3.5, LlamaStackDistribution CRs are replaced by OGXServer v1beta1.
type MigrationCheck struct {
	check.BaseCheck
}

func NewMigrationCheck() *MigrationCheck {
	return &MigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             "migration",
			CheckID:          "workloads.llamastack.migration",
			CheckName:        "Workloads :: LlamaStack :: CR Migration (3.4 to 3.5)",
			CheckDescription: "Identifies LlamaStackDistribution resources that must be migrated to OGXServer v1beta1 for RHOAI 3.5 upgrade",
			CheckRemediation: "Back up LlamaStack resources using 'odh-cli migrate prepare --migration llamastack.backup', then recreate as OGXServer v1beta1 CRs after upgrade following the OGX migration guide",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 3.4.x TO 3.5.x and LlamaStack Operator is Managed.
func (c *MigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom34To35(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "llamastackoperator", constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *MigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.LlamaStackDistribution).
		Run(ctx, c.validateDistributions)
}

func (c *MigrationCheck) validateDistributions(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) error {
	count := len(req.Items)

	if count == 0 {
		req.Result.SetCondition(check.NewCondition(
			ConditionTypeRequiresMigration,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("No LlamaStackDistribution resources found - no CR migration needed for 3.5 upgrade"),
		))

		return nil
	}

	req.Result.SetCondition(check.NewCondition(
		ConditionTypeRequiresMigration,
		metav1.ConditionFalse,
		check.WithReason("CRTypeMigration"),
		check.WithMessage("Found %d LlamaStackDistribution(s) that must be migrated to OGXServer v1beta1 after RHOAI 3.5 upgrade. The LlamaStackDistribution CRD is removed in 3.5 and replaced by OGXServer.", count),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(c.CheckRemediation),
	))

	req.Result.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(req.Items))

	for _, llsd := range req.Items {
		req.Result.ImpactedObjects = append(req.Result.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.LlamaStackDistribution.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: llsd.GetNamespace(),
				Name:      llsd.GetName(),
				Annotations: map[string]string{
					"upgrade.action": "requires-migration-to-ogxserver",
				},
			},
		})
	}

	return nil
}
