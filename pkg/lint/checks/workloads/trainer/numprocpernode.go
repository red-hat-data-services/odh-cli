package trainer

import (
	"context"
	"errors"
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
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const conditionTypeNumProcPerNodeCompatible = "NumProcPerNodeCompatible"

// NumProcPerNodeCheck blocks upgrades when legacy TrainJobs use a string
// value for spec.trainer.numProcPerNode, which Trainer 2.2 no longer accepts.
type NumProcPerNodeCheck struct {
	check.BaseCheck
}

func NewNumProcPerNodeCheck() *NumProcPerNodeCheck {
	return &NumProcPerNodeCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup: check.GroupWorkload,
			Kind:       constants.ComponentTrainer,
			Type:       check.CheckTypeImpactedWorkloads,
			CheckID:    "workloads.trainer.numprocpernode",
			CheckName:  "Workloads :: Trainer :: numProcPerNode (3.6+)",
			CheckDescription: "Detects TrainJobs with a string numProcPerNode value that prevents " +
				"the Trainer controller from starting after an OpenShift AI 3.6 upgrade",
			CheckRemediation: "Delete TrainJobs with a string spec.trainer.numProcPerNode value before upgrading. " +
				"The field is immutable. Consult the TrainJob owner before deletion; recreate the TrainJob after the upgrade, if desired, with a nil value, which is equivalent to auto.",
		},
	}
}

// CanApply applies to upgrades from before 3.6 to 3.6 or newer when Trainer is managed.
func (c *NumProcPerNodeCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	//nolint:mnd // Version numbers 3.6
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 6) || version.IsVersionAtLeast(target.CurrentVersion, 3, 6) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentTrainer, constants.ManagementStateManaged), nil
}

func (c *NumProcPerNodeCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.TrainJob).
		ForComponent(constants.ComponentTrainer).
		Filter(hasStringNumProcPerNode).
		Complete(ctx, c.newNumProcPerNodeCondition)
}

func hasStringNumProcPerNode(obj *unstructured.Unstructured) (bool, error) {
	value, err := jq.Query[any](obj, ".spec.trainer.numProcPerNode")
	if errors.Is(err, jq.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	_, ok := value.(string)

	return ok, nil
}

func (c *NumProcPerNodeCheck) newNumProcPerNodeCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	if len(req.Items) == 0 {
		return []result.Condition{check.NewCondition(
			conditionTypeNumProcPerNodeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No TrainJob(s) with a string numProcPerNode value found - ready for OpenShift AI 3.6 Trainer upgrade"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		conditionTypeNumProcPerNodeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage("Found %d TrainJob(s) with a string spec.trainer.numProcPerNode value. These TrainJobs must be deleted before upgrading to OpenShift AI 3.6 or the Trainer controller will not start. Consult the TrainJob owner before deleting them.", len(req.Items)),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
