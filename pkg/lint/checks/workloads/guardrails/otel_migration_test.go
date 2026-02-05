package guardrails_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/guardrails"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.GuardrailsOrchestrator.GVR(): resources.GuardrailsOrchestrator.ListKind(),
}

func toPartialObjectMetadata(objs ...*unstructured.Unstructured) []runtime.Object {
	result := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		pom := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        obj.GetName(),
				Namespace:   obj.GetNamespace(),
				Labels:      obj.GetLabels(),
				Annotations: obj.GetAnnotations(),
				Finalizers:  obj.GetFinalizers(),
			},
		}
		result = append(result, pom)
	}

	return result
}

func TestOtelMigrationCheck_NoOrchestrators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No GuardrailsOrchestrators found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestOtelMigrationCheck_OrchestratorWithNewFields(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Orchestrator using new field format only
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "test-orchestrator",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"otlpProtocol":        "grpc",
					"otlpTracesEndpoint":  "http://traces:4317",
					"otlpMetricsEndpoint": "http://metrics:4317",
					"enableMetrics":       true,
					"enableTracing":       true,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, orch)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(orch)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestOtelMigrationCheck_OrchestratorWithoutOtelExporter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Orchestrator without any otelExporter configuration
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "no-otel-orch",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"replicas": int64(1),
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, orch)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(orch)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestOtelMigrationCheck_OrchestratorWithDeprecatedFields(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Orchestrator using deprecated fields
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "deprecated-orch",
				"namespace": "prod-ns",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"protocol":     "grpc",
					"otlpEndpoint": "http://collector:4317",
					"otlpExport":   true,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, orch)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(orch)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("Found 1 GuardrailsOrchestrator"), ContainSubstring("deprecated")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("deprecated-orch"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("prod-ns"))
}

func TestOtelMigrationCheck_MultipleOrchestratorsWithDeprecatedFields(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	orch1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-1",
				"namespace": "ns1",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"tracesProtocol":  "http",
					"tracesEndpoint":  "http://traces:4318",
					"metricsProtocol": "grpc",
					"metricsEndpoint": "http://metrics:4317",
				},
			},
		},
	}

	orch2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-2",
				"namespace": "ns2",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"protocol":     "grpc",
					"otlpEndpoint": "http://otel:4317",
				},
			},
		},
	}

	// Orchestrator with new config only - should not be impacted
	orch3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-3",
				"namespace": "ns3",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"otlpProtocol":        "grpc",
					"otlpTracesEndpoint":  "http://traces:4317",
					"otlpMetricsEndpoint": "http://metrics:4317",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, orch1, orch2, orch3)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(orch1, orch2, orch3)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 2 GuardrailsOrchestrator"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestOtelMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	otelCheck := guardrails.NewOtelMigrationCheck()

	g.Expect(otelCheck.ID()).To(Equal("workloads.guardrails.otel-config-migration"))
	g.Expect(otelCheck.Name()).To(Equal("Workloads :: Guardrails :: OTEL Config Migration (3.x)"))
	g.Expect(otelCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(otelCheck.Description()).ToNot(BeEmpty())
}

func TestOtelMigrationCheck_CanApply_LintMode(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("2.17.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &currentVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	canApply := otelCheck.CanApply(target)

	// Lint mode at 2.x should not apply
	g.Expect(canApply).To(BeFalse())
}

func TestOtelMigrationCheck_CanApply_LintMode3x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &currentVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	canApply := otelCheck.CanApply(target)

	// Lint mode at 3.x should apply
	g.Expect(canApply).To(BeTrue())
}

func TestOtelMigrationCheck_CanApply_UpgradeTo3x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	canApply := otelCheck.CanApply(target)

	g.Expect(canApply).To(BeTrue())
}

func TestOtelMigrationCheck_CanApply_UpgradeTo33(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.3.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	canApply := otelCheck.CanApply(target)

	g.Expect(canApply).To(BeTrue())
}

func TestOtelMigrationCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	target := check.Target{
		CurrentVersion: nil,
		TargetVersion:  nil,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	canApply := otelCheck.CanApply(target)

	g.Expect(canApply).To(BeFalse())
}

func TestOtelMigrationCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
