package servicemesh

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const kind = "servicemesh-v3"

const displayName = "Red Hat Service Mesh v3"

const disconnectedNote = "If performing a disconnected install, ensure the appropriate images are mirrored into your environment."

//nolint:gochecknoglobals
var minVersion = semver.MustParse("3.1.0")

// Check validates Red Hat Service Mesh v3 operator installation and version.
type Check struct {
	check.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             kind,
			Type:             check.CheckTypeInstalled,
			CheckID:          "dependencies.servicemesh.installed",
			CheckName:        "Dependencies :: Service Mesh v3 :: Installed",
			CheckDescription: "Reports the Red Hat Service Mesh v3 operator installation status and validates minimum version requirement",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Operator(c, target).
		WithNames("servicemeshoperator3").
		WithConditionBuilder(func(found bool, installedCSV string) result.Condition {
			if !found {
				return check.NewCondition(
					check.ConditionTypeAvailable,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonResourceNotFound),
					check.WithMessage("%s is not installed. Install the latest 3.x version (minimum required is %s). %s",
						displayName, minVersion.String(), disconnectedNote),
					check.WithImpact(result.ImpactBlocking),
				)
			}

			ver, err := parseInstalledCSVVersion(installedCSV)
			if err != nil {
				return check.NewCondition(
					check.ConditionTypeAvailable,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("%s is installed but version could not be determined from %q; minimum required is %s",
						displayName, installedCSV, minVersion.String()),
					check.WithImpact(result.ImpactBlocking),
				)
			}

			if ver.LT(minVersion) {
				return check.NewCondition(
					check.ConditionTypeAvailable,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("%s version %s is below minimum required version %s. %s",
						displayName, ver.String(), minVersion.String(), disconnectedNote),
					check.WithImpact(result.ImpactBlocking),
				)
			}

			return check.NewCondition(
				check.ConditionTypeAvailable,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonResourceFound),
				check.WithMessage("%s installed: %s", displayName, ver.String()),
			)
		}).
		Run(ctx)
}

// parseInstalledCSVVersion extracts a semver.Version from an InstalledCSV string.
// The expected format is "<name>.v<semver>", e.g. "servicemeshoperator3.v3.1.0".
func parseInstalledCSVVersion(csv string) (semver.Version, error) {
	idx := strings.LastIndex(csv, ".v")
	if idx < 0 {
		return semver.Version{}, fmt.Errorf("no version segment found in %q", csv)
	}

	raw := csv[idx+2:] // skip ".v"

	ver, err := semver.ParseTolerant(raw)
	if err != nil {
		return semver.Version{}, fmt.Errorf("parsing version %q from %q: %w", raw, csv, err)
	}

	return ver, nil
}
