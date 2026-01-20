package servicemeshoperator

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	checkID          = "dependencies.servicemeshoperator2.upgrade"
	checkName        = "Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)"
	checkDescription = "Validates that servicemeshoperator2 is not installed when upgrading to RHOAI 3.x (requires servicemeshoperator3)"
)

// Check validates that Service Mesh Operator v2 is not installed when upgrading to 3.x.
type Check struct{}

// NewCheck creates a new Service Mesh Operator v2 upgrade check.
func NewCheck() *Check {
	return &Check{}
}

func (c *Check) ID() string {
	return checkID
}

func (c *Check) Name() string {
	return checkName
}

func (c *Check) Description() string {
	return checkDescription
}

func (c *Check) Group() check.CheckGroup {
	return check.GroupDependency
}

func (c *Check) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	res, err := operators.CheckOperatorPresence(
		ctx,
		target.Client,
		check.DependencyServiceMeshOperatorV2,
		operators.WithDescription(checkDescription),
		operators.WithMatcher(func(subscription *operatorsv1alpha1.Subscription) bool {
			// Check if this is servicemeshoperator on v2.x channel
			op := operators.GetOperator(subscription)
			if op.Name != "servicemeshoperator" {
				return false
			}

			// Check if it's on v2.x channel (stable or v2.x)
			channelStr := subscription.Spec.Channel
			if channelStr == "" {
				return false
			}

			return channelStr == "stable" || channelStr == "v2.x"
		}),
		operators.WithConditionBuilder(func(found bool, version string) result.Condition {
			// Inverted logic: NOT finding the operator is good
			if !found {
				return results.NewCompatibilitySuccess("Service Mesh Operator v2 is not installed - ready for RHOAI 3.x upgrade")
			}

			return results.NewCompatibilityFailure("Service Mesh Operator v2 (%s) is installed but RHOAI 3.x requires v3", version)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("checking servicemesh-operator v2 presence: %w", err)
	}

	return res, nil
}
