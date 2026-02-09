package notebook

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
)

func newNotebookCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No Notebooks found - no workloads impacted by deprecation",
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypeNotebooksCompatible,
		metav1.ConditionFalse,
		check.ReasonWorkloadsImpacted,
		"Found %d Notebook(s) - workloads will be impacted in RHOAI 3.x",
		count,
		check.WithImpact(result.ImpactAdvisory),
	)}, nil
}
