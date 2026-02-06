package trainingoperator

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const checkType = "deprecation"

type DeprecationCheck struct {
	base.BaseCheck
}

func NewDeprecationCheck() *DeprecationCheck {
	return &DeprecationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentTrainingOperator,
			Type:             checkType,
			CheckID:          "components.trainingoperator.deprecation",
			CheckName:        "Components :: TrainingOperator :: Deprecation (3.3+)",
			CheckDescription: "Validates that TrainingOperator (Kubeflow Training Operator v1) deprecation is acknowledged - will be replaced by Trainer v2 in future RHOAI releases",
		},
	}
}

func (c *DeprecationCheck) CanApply(_ context.Context, target check.Target) bool {
	//nolint:mnd // Version numbers 3.3
	return version.IsVersionAtLeast(target.TargetVersion, 3, 3)
}

func (c *DeprecationCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			results.SetCondition(req.Result, check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.ReasonDeprecated,
				"TrainingOperator (Kubeflow Training Operator v1) is enabled (state: %s) but is deprecated in RHOAI 3.3 and will be replaced by Trainer v2 in a future release",
				req.ManagementState,
				check.WithImpact(result.ImpactAdvisory)),
			)

			return nil
		})
}
