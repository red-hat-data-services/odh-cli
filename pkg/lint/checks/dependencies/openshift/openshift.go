package openshift

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	checkID          = "dependencies.openshift.version-requirement"
	checkName        = "Dependencies :: OpenShift :: Version Requirement (3.x)"
	checkDescription = "Validates that OpenShift is at least version 4.19 when upgrading to RHOAI 3.x"

	minMajorVersion = 4
	minMinorVersion = 19
)

// Check validates OpenShift version requirements for RHOAI 3.x upgrades.
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
			fmt.Sprintf("Unable to detect OpenShift version: %s. RHOAI 3.x requires OpenShift 4.19 or later", err.Error()),
		)
		dr.Status.Conditions = []result.Condition{condition}

		return dr, nil
	}

	dr.Annotations["platform.opendatahub.io/openshift-version"] = openshiftVersion.String()

	if version.IsVersionAtLeast(openshiftVersion, minMajorVersion, minMinorVersion) {
		condition := results.NewCompatibilitySuccess(
			"OpenShift %s meets RHOAI 3.x minimum version requirement (4.19+)",
			openshiftVersion.String(),
		)
		dr.Status.Conditions = []result.Condition{condition}
	} else {
		condition := results.NewCompatibilityFailure(
			"OpenShift %s does not meet RHOAI 3.x minimum version requirement (4.19+). Upgrade OpenShift to 4.19 or later before upgrading RHOAI",
			openshiftVersion.String(),
		)
		dr.Status.Conditions = []result.Condition{condition}
	}

	return dr, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(&Check{})
}
