package kueueoperator

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "dependencies.kueueoperator.installed"
	checkName        = "Dependencies :: KueueOperator :: Installed"
	checkDescription = "Reports the kueue-operator installation status and version"
)

type Check struct{}

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

// CanApply returns whether this check should run for the given target.
// This check only applies when the kueue component is enabled in DataScienceCluster.
func (c *Check) CanApply(target check.Target) bool {
	if target.Client == nil {
		return false
	}

	// Query DataScienceCluster to check if kueue component is enabled
	ctx := context.Background()
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	if err != nil {
		// If DSC doesn't exist or error querying, don't apply check
		return false
	}

	// Check kueue management state using JQ
	managementState, err := jq.Query[string](dsc, ".spec.components.kueue.managementState")
	if err != nil || managementState == "" {
		// Kueue not configured, check doesn't apply
		return false
	}

	// Check applies if kueue is Managed or Unmanaged (not Removed)
	return managementState == check.ManagementStateManaged ||
		managementState == check.ManagementStateUnmanaged
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	// kueue-operator check uses all defaults from operators.CheckOperatorPresence
	// since the subscription name matches the operator kind ("kueue-operator")
	res, err := operators.CheckOperatorPresence(
		ctx,
		target.Client,
		"kueue-operator",
		operators.WithDescription(checkDescription),
		operators.WithMatcher(func(subscription *operatorsv1alpha1.Subscription) bool {
			op := operators.GetOperator(subscription)

			return op.Name == "kueue-operator"
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("checking kueue-operator presence: %w", err)
	}

	return res, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(&Check{})
}
