package certmanager

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
)

// Check validates cert-manager operator installation.
type Check struct {
	base.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             check.DependencyCertManager,
			CheckType:        check.CheckTypeInstalled,
			CheckID:          "dependencies.certmanager.installed",
			CheckName:        "Dependencies :: CertManager :: Installed",
			CheckDescription: "Reports the cert-manager operator installation status and version",
		},
	}
}

func (c *Check) CanApply(_ *semver.Version, _ *semver.Version) bool {
	return true
}

func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
	res, err := operators.CheckOperatorPresence(
		ctx,
		target.Client,
		"cert-manager",
		operators.WithDescription(c.Description()),
		operators.WithMatcher(func(subscription *operatorsv1alpha1.Subscription) bool {
			op := operators.GetOperator(subscription)

			return op.Name == "cert-manager" || op.Name == "openshift-cert-manager-operator"
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("checking cert-manager operator presence: %w", err)
	}

	return res, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(NewCheck())
}
