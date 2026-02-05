package guardrails

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

// deprecatedOtelFields lists the otelExporter fields that are deprecated in 3.x.
// These fields need to be migrated to the new configuration format.
//
//nolint:gochecknoglobals // Package-level constant for deprecated field names.
var deprecatedOtelFields = []string{
	"protocol",
	"tracesProtocol",
	"metricsProtocol",
	"otlpEndpoint",
	"tracesEndpoint",
	"metricsEndpoint",
	"otlpExport",
}

func newOtelMigrationCondition(count int) result.Condition {
	if count == 0 {
		return check.NewCondition(
			ConditionTypeOtelConfigCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No GuardrailsOrchestrators found using deprecated otelExporter fields",
		)
	}

	return check.NewCondition(
		ConditionTypeOtelConfigCompatible,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"Found %d GuardrailsOrchestrator(s) using deprecated otelExporter fields - migrate to new format before upgrading",
		count,
		check.WithImpact(result.ImpactAdvisory),
	)
}

func populateImpactedObjects(
	dr *result.DiagnosticResult,
	orchestrators []types.NamespacedName,
) {
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(orchestrators))

	for _, orch := range orchestrators {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.GuardrailsOrchestrator.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: orch.Namespace,
				Name:      orch.Name,
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}
