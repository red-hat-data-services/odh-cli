package openshift

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	checkID          = "dependencies.openshift.version-requirement"
	checkName        = "Dependencies :: OpenShift :: Version Requirement (3.x)"
	checkDescription = "Validates that OpenShift is at least version 4.19.9 when upgrading to RHOAI 3.x"
)

//nolint:gochecknoglobals
var minVersion = semver.MustParse("4.19.9")

// Check validates OpenShift version requirements for RHOAI 3.x upgrades.
type Check struct{}

// NewCheck creates a new OpenShift version requirement check.
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

func (c *Check) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(check.GroupDependency),
		check.DependencyOpenShiftPlatform,
		check.CheckTypeVersionRequirement,
		checkDescription,
	)

	openshiftVersion, err := version.DetectOpenShiftVersion(ctx, target.Client)
	if err != nil {
		condition := check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.ReasonInsufficientData,
			fmt.Sprintf("Unable to detect OpenShift version: %s. RHOAI 3.x requires OpenShift %s or later", err.Error(), minVersion.String()),
		)
		dr.Status.Conditions = []result.Condition{condition}

		return dr, nil
	}

	dr.Annotations["platform.opendatahub.io/openshift-version"] = openshiftVersion.String()

	if openshiftVersion.GTE(minVersion) {
		condition := results.NewCompatibilitySuccess(
			"OpenShift %s meets RHOAI 3.x minimum version requirement (%s+)",
			openshiftVersion.String(),
			minVersion.String(),
		)
		dr.Status.Conditions = []result.Condition{condition}
	} else {
		condition := results.NewCompatibilityFailure(
			"OpenShift %s does not meet RHOAI 3.x minimum version requirement (%s+). Upgrade OpenShift to %s or later before upgrading RHOAI",
			openshiftVersion.String(),
			minVersion.String(),
			minVersion.String(),
		)
		dr.Status.Conditions = []result.Condition{condition}
	}

	return dr, nil
}
