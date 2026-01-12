package trainingoperator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

func newPyTorchJobCondition(
	activeCount int,
	completedCount int,
) metav1.Condition {
	totalCount := activeCount + completedCount

	if totalCount == 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No PyTorchJob(s) found - no workloads impacted by TrainingOperator deprecation",
		)
	}

	if activeCount > 0 && completedCount > 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionFalse,
			check.ReasonVersionIncompatible,
			"Found %d PyTorchJob(s) (%d active, %d completed) - workloads use deprecated TrainingOperator (Kubeflow v1) which will be replaced by Trainer v2",
			totalCount,
			activeCount,
			completedCount,
		)
	}

	if activeCount > 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionFalse,
			check.ReasonVersionIncompatible,
			"Found %d active PyTorchJob(s) - workloads use deprecated TrainingOperator (Kubeflow v1) which will be replaced by Trainer v2",
			activeCount,
		)
	}

	return check.NewCondition(
		ConditionTypePyTorchJobsCompatible,
		metav1.ConditionTrue,
		check.ReasonVersionCompatible,
		"Found %d completed PyTorchJob(s) - workloads previously used deprecated TrainingOperator (Kubeflow v1)",
		completedCount,
	)
}

func populateImpactedObjects(
	dr *result.DiagnosticResult,
	activeJobs []types.NamespacedName,
	completedJobs []types.NamespacedName,
) {
	totalCount := len(activeJobs) + len(completedJobs)
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, totalCount)

	for _, job := range activeJobs {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.PyTorchJob.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: job.Namespace,
				Name:      job.Name,
				Annotations: map[string]string{
					"status": "active",
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}

	for _, job := range completedJobs {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.PyTorchJob.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: job.Namespace,
				Name:      job.Name,
				Annotations: map[string]string{
					"status": "completed",
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}

func isJobCompleted(job *unstructured.Unstructured) bool {
	conditions, err := jq.Query[[]any](job, ".status.conditions")
	if err != nil || len(conditions) == 0 {
		return false
	}

	for _, conditionAny := range conditions {
		condition, ok := conditionAny.(map[string]any)
		if !ok {
			continue
		}

		condType, _ := condition["type"].(string)
		status, _ := condition["status"].(string)

		if (condType == "Succeeded" || condType == "Failed") && status == "True" {
			return true
		}
	}

	return false
}
