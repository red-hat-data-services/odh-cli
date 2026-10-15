package dscinitialization_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/dscinitialization"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// Test constants for DSCInitialization readiness check.
//
//nolint:gochecknoglobals // Test fixtures shared across test functions in this file.
var dsciReadinessListKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
}

func newDSCIWithPhase(phase string) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": resources.DSCInitialization.APIVersion(),
		"kind":       resources.DSCInitialization.Kind,
		"metadata": map[string]any{
			"name": "default-dsci",
		},
	}
	if phase != "" {
		obj["status"] = map[string]any{
			"phase": phase,
		}
	}

	return &unstructured.Unstructured{Object: obj}
}

func newDSCIWithEmptyPhase() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "",
			},
		},
	}
}

func newDSCIWithoutPhase() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{},
		},
	}
}

// Operator condition messages for unconfigured optional observability features.
const (
	msgMetricsNotConfigured  = "Metrics not configured in DSCI CR"
	msgTracesNotConfigured   = "Traces not configured in DSCI CR"
	msgAlertingNotConfigured = "Alerting not configured in DSCI CR"
	msgMonitoringNotEnabled  = "Monitoring is not enabled"
	msgPersesUnconfigured    = "Perses requires at least Metrics or Traces to be configured"
)

// falseDSCICondition returns a False status condition with the given reason and message.
func falseDSCICondition(condType, reason, message string) map[string]any {
	return map[string]any{"type": condType, "status": "False", "reason": reason, "message": message}
}

// healthyCoreDSCIConditions returns the core DSCI conditions in their expected healthy state.
func healthyCoreDSCIConditions() []any {
	return []any{
		map[string]any{"type": "ReconcileComplete", "status": "True", "reason": "ReconcileCompleted", "message": "reconcile completed"},
		map[string]any{"type": "Available", "status": "True", "reason": "Available", "message": "available"},
		map[string]any{"type": "Degraded", "status": "False", "reason": "NotDegraded", "message": "not degraded"},
		map[string]any{"type": "Progressing", "status": "False", "reason": "Done", "message": "not progressing"},
	}
}

// withHealthyCoreConditions appends the given conditions to the healthy core set.
func withHealthyCoreConditions(extra ...map[string]any) []any {
	conditions := healthyCoreDSCIConditions()
	for _, cond := range extra {
		conditions = append(conditions, cond)
	}

	return conditions
}

// newDSCIWithMonitoring builds a DSCI with the given phase, spec.monitoring, and status conditions.
func newDSCIWithMonitoring(phase string, monitoring map[string]any, conditions []any) *unstructured.Unstructured {
	status := map[string]any{}
	if phase != "" {
		status["phase"] = phase
	}

	if conditions != nil {
		status["conditions"] = conditions
	}

	obj := map[string]any{
		"apiVersion": resources.DSCInitialization.APIVersion(),
		"kind":       resources.DSCInitialization.Kind,
		"metadata": map[string]any{
			"name": "default-dsci",
		},
		"status": status,
	}

	if monitoring != nil {
		obj["spec"] = map[string]any{"monitoring": monitoring}
	}

	return &unstructured.Unstructured{Object: obj}
}

func TestDSCInitializationReadinessCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
}

func TestDSCInitializationReadinessCheck_PhaseMissing(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithoutPhase()},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("phase field is missing"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_PhaseEmpty(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithEmptyPhase()},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("phase is empty"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_NotReady(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithPhase("Degraded")},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": And(ContainSubstring("DSCInitialization"), ContainSubstring("Degraded")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_NotReadyWithUnknownCondition(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Ready",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "Unknown", "message": "reconcile state unknown"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": ContainSubstring("ReconcileComplete: reconcile state unknown"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_NotReadyWithUnhappyConditions(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Degraded",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "False", "message": "reconcile failed: some error"},
					map[string]any{"type": "Degraded", "status": "True", "message": "operator is degraded"},
					map[string]any{"type": "Progressing", "status": "False", "message": "Reconcile completed successfully"},
					map[string]any{"type": "Available", "status": "True", "message": "Reconcile completed successfully"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeReady),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceUnavailable),
		"Message": And(
			ContainSubstring("DSCInitialization"),
			ContainSubstring("Degraded"),
			ContainSubstring("ReconcileComplete: reconcile failed: some error"),
			ContainSubstring("Degraded: operator is degraded"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_CapabilityConditions(t *testing.T) {
	tests := []struct {
		name            string
		conditionType   string
		reason          string
		message         string
		expectedStatus  metav1.ConditionStatus
		expectedReason  string
		expectedImpact  resultpkg.Impact
		expectedMessage string
	}{
		{
			name:            "ServiceMesh/Removed",
			conditionType:   "CapabilityServiceMesh",
			reason:          "Removed",
			message:         "ServiceMesh removed",
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "ServiceMesh/Unmanaged",
			conditionType:   "CapabilityServiceMesh",
			reason:          "Unmanaged",
			message:         "ServiceMesh is set to Unmanaged",
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "ServiceMesh/NotReady",
			conditionType:   "CapabilityServiceMesh",
			reason:          "NotReady",
			message:         "ServiceMeshControlPlane is not ready",
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "CapabilityServiceMesh: ServiceMeshControlPlane is not ready",
		},
		{
			name:            "ServiceMesh/MissingOperator",
			conditionType:   "CapabilityServiceMesh",
			reason:          "MissingOperator",
			message:         "ServiceMesh operator not found",
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "CapabilityServiceMesh: ServiceMesh operator not found",
		},
		{
			name:            "ServiceMeshAuth/Unmanaged",
			conditionType:   "CapabilityServiceMeshAuthorization",
			reason:          "Unmanaged",
			message:         "ServiceMesh is set to Unmanaged",
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "ServiceMeshAuth/Removed",
			conditionType:   "CapabilityServiceMeshAuthorization",
			reason:          "Removed",
			message:         "ServiceMesh authorization removed",
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "ServiceMeshAuth/MissingOperator",
			conditionType:   "CapabilityServiceMeshAuthorization",
			reason:          "MissingOperator",
			message:         "Authorino operator not found",
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "ServiceMeshAuth/NotReady",
			conditionType:   "CapabilityServiceMeshAuthorization",
			reason:          "NotReady",
			message:         "Authorino resource not ready",
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "CapabilityServiceMeshAuthorization: Authorino resource not ready",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			dsci := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": resources.DSCInitialization.APIVersion(),
					"kind":       resources.DSCInitialization.Kind,
					"metadata": map[string]any{
						"name": "default-dsci",
					},
					"status": map[string]any{
						"phase": "Ready",
						"conditions": []any{
							map[string]any{"type": "ReconcileComplete", "status": "True", "reason": "ReconcileCompleted", "message": "reconcile completed"},
							map[string]any{"type": tc.conditionType, "status": "False", "reason": tc.reason, "message": tc.message},
						},
					},
				},
			}

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds: dsciReadinessListKinds,
				Objects:   []*unstructured.Unstructured{dsci},
			})

			chk := dscinitialization.NewDSCInitializationReadinessCheck()
			result, err := chk.Validate(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Status.Conditions).To(HaveLen(1))
			g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
				"Type":    Equal(check.ConditionTypeReady),
				"Status":  Equal(tc.expectedStatus),
				"Reason":  Equal(tc.expectedReason),
				"Message": ContainSubstring(tc.expectedMessage),
			}))
			g.Expect(result.Status.Conditions[0].Impact).To(Equal(tc.expectedImpact))
		})
	}
}

func TestDSCInitializationReadinessCheck_ObservabilityConditions(t *testing.T) {
	tests := []struct {
		name              string
		phase             string
		monitoring        map[string]any
		conditions        []any
		expectedStatus    metav1.ConditionStatus
		expectedReason    string
		expectedImpact    resultpkg.Impact
		expectedMessage   string
		unexpectedMessage string
	}{
		{
			name:            "Managed with unconfigured metrics",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with unconfigured traces",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("TempoAvailable", "TracesNotConfigured", msgTracesNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with unconfigured metrics and traces",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("PersesAvailable", "MetricsAndTracesNotConfigured", msgPersesUnconfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with unconfigured alerting",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("AlertingAvailable", "AlertingNotConfigured", msgAlertingNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with bare metrics block without storage",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "metrics": map[string]any{"replicas": int64(2)}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with empty exporters map",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "metrics": map[string]any{"exporters": map[string]any{}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with exporter-only metrics reported unconfigured",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "metrics": map[string]any{"exporters": map[string]any{"remote": map[string]any{"endpoint": "http://example.com/metrics"}}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "MonitoringStackAvailable: Metrics not configured in DSCI CR",
		},
		{
			name:            "Managed with exporter-only traces reported unconfigured",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "traces": map[string]any{"exporters": map[string]any{"remote": map[string]any{"endpoint": "http://example.com/traces"}}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("TempoAvailable", "TracesNotConfigured", msgTracesNotConfigured)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "TempoAvailable: Traces not configured in DSCI CR",
		},
		{
			name:            "Managed with exporter-only metrics and Perses reported unconfigured",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "metrics": map[string]any{"exporters": map[string]any{"remote": map[string]any{"endpoint": "http://example.com/metrics"}}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("PersesAvailable", "MetricsAndTracesNotConfigured", msgPersesUnconfigured)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "PersesAvailable: Perses requires at least Metrics or Traces to be configured",
		},
		{
			name:            "Managed with metrics configured but reported unconfigured",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "metrics": map[string]any{"storage": map[string]any{"size": "5Gi"}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "MonitoringStackAvailable: Metrics not configured in DSCI CR",
		},
		{
			name:            "Managed with traces configured but reported unconfigured",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "traces": map[string]any{"storage": map[string]any{"backend": "pv", "size": "5Gi"}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("TempoAvailable", "TracesNotConfigured", msgTracesNotConfigured)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "TempoAvailable: Traces not configured in DSCI CR",
		},
		{
			name:            "Managed with alerting configured but reported unconfigured",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "alerting": map[string]any{"enabled": true}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("AlertingAvailable", "AlertingNotConfigured", msgAlertingNotConfigured)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "AlertingAvailable: Alerting not configured in DSCI CR",
		},
		{
			name:            "Removed management state ignores feature spec",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Removed", "metrics": map[string]any{"storage": map[string]any{"size": "5Gi"}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Missing management state defaults to removed",
			phase:           "Ready",
			monitoring:      map[string]any{"namespace": "redhat-ods-monitoring"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Removed with monitoring not enabled",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Removed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringReady", "Removed", msgMonitoringNotEnabled)),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with monitoring reported removed",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringReady", "Removed", msgMonitoringNotEnabled)),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "MonitoringReady: Monitoring is not enabled",
		},
		{
			name:            "Managed with failure reason and unconfigured feature",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed"},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "NotReady", "monitoring stack is not ready")),
			expectedStatus:  metav1.ConditionTrue,
			expectedReason:  check.ReasonResourceAvailable,
			expectedImpact:  resultpkg.ImpactNone,
			expectedMessage: "DSCInitialization is ready",
		},
		{
			name:            "Managed with NotReady reason and configured metrics",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "metrics": map[string]any{"storage": map[string]any{"size": "5Gi"}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "NotReady", "monitoring stack is not ready")),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "MonitoringStackAvailable: monitoring stack is not ready",
		},
		{
			name:            "Managed with MissingOperator reason and configured traces",
			phase:           "Ready",
			monitoring:      map[string]any{"managementState": "Managed", "traces": map[string]any{"storage": map[string]any{"backend": "pv", "size": "5Gi"}}},
			conditions:      withHealthyCoreConditions(falseDSCICondition("TempoAvailable", "MissingOperator", "Tempo operator must be installed for traces configuration")),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "TempoAvailable: Tempo operator must be installed for traces configuration",
		},
		{
			name:       "Unknown status with unconfigured reason",
			phase:      "Ready",
			monitoring: map[string]any{"managementState": "Managed"},
			conditions: withHealthyCoreConditions(map[string]any{
				"type": "PersesAvailable", "status": "Unknown", "reason": "MetricsAndTracesNotConfigured", "message": msgPersesUnconfigured,
			}),
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  check.ReasonResourceUnavailable,
			expectedImpact:  resultpkg.ImpactBlocking,
			expectedMessage: "PersesAvailable: Perses requires at least Metrics or Traces to be configured",
		},
		{
			name:              "Not ready phase filters unconfigured conditions",
			phase:             "Degraded",
			monitoring:        map[string]any{"managementState": "Managed"},
			conditions:        withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)),
			expectedStatus:    metav1.ConditionFalse,
			expectedReason:    check.ReasonResourceUnavailable,
			expectedImpact:    resultpkg.ImpactBlocking,
			expectedMessage:   "phase: Degraded",
			unexpectedMessage: msgMetricsNotConfigured,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			dsci := newDSCIWithMonitoring(tc.phase, tc.monitoring, tc.conditions)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds: dsciReadinessListKinds,
				Objects:   []*unstructured.Unstructured{dsci},
			})

			chk := dscinitialization.NewDSCInitializationReadinessCheck()
			result, err := chk.Validate(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Status.Conditions).To(HaveLen(1))
			g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(check.ConditionTypeReady),
				"Status": Equal(tc.expectedStatus),
				"Reason": Equal(tc.expectedReason),
			}))
			g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring(tc.expectedMessage))
			if tc.unexpectedMessage != "" {
				g.Expect(result.Status.Conditions[0].Message).ToNot(ContainSubstring(tc.unexpectedMessage))
			}
			g.Expect(result.Status.Conditions[0].Impact).To(Equal(tc.expectedImpact))
		})
	}
}

// TestDSCInitializationReadinessCheck_ReadyWithUnconfiguredObservability replicates a real
// cluster report: DSCI phase Ready with every optional observability condition False
// because metrics, traces, and alerting are not configured in the DSCI CR.
func TestDSCInitializationReadinessCheck_ReadyWithUnconfiguredObservability(t *testing.T) {
	g := NewWithT(t)

	dsci := newDSCIWithMonitoring("Ready",
		map[string]any{"managementState": "Managed"},
		withHealthyCoreConditions(
			falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured),
			falseDSCICondition("TempoAvailable", "TracesNotConfigured", msgTracesNotConfigured),
			falseDSCICondition("OpenTelemetryCollectorAvailable", "MetricsAndTracesNotConfigured", msgMetricsNotConfigured+"\n"+msgTracesNotConfigured),
			falseDSCICondition("AlertingAvailable", "AlertingNotConfigured", msgAlertingNotConfigured),
			falseDSCICondition("ThanosQuerierAvailable", "MetricsNotConfigured", msgMetricsNotConfigured),
			falseDSCICondition("PersesAvailable", "MetricsAndTracesNotConfigured", msgPersesUnconfigured),
			falseDSCICondition("NodeMetricsEndpointAvailable", "MetricsNotConfigured", msgMetricsNotConfigured),
		))

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("DSCInitialization is ready"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
}

// TestDSCInitializationReadinessCheck_ConditionLastTransitionTime documents how the
// readiness check handles lastTransitionTime on DSCI status conditions: a missing
// timestamp is tolerated, while a malformed one fails the check with an error rather
// than silently misreporting readiness.
func TestDSCInitializationReadinessCheck_ConditionLastTransitionTime(t *testing.T) {
	t.Run("missing lastTransitionTime is tolerated", func(t *testing.T) {
		g := NewWithT(t)

		dsci := newDSCIWithMonitoring("Ready",
			map[string]any{"managementState": "Managed"},
			withHealthyCoreConditions(falseDSCICondition("MonitoringStackAvailable", "MetricsNotConfigured", msgMetricsNotConfigured)))

		target := testutil.NewTarget(t, testutil.TargetConfig{
			ListKinds: dsciReadinessListKinds,
			Objects:   []*unstructured.Unstructured{dsci},
		})

		chk := dscinitialization.NewDSCInitializationReadinessCheck()
		result, err := chk.Validate(t.Context(), target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status.Conditions).To(HaveLen(1))
		g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	})

	t.Run("malformed lastTransitionTime fails the check", func(t *testing.T) {
		g := NewWithT(t)

		dsci := newDSCIWithMonitoring("Ready",
			map[string]any{"managementState": "Managed"},
			withHealthyCoreConditions(map[string]any{
				"type": "ReconcileComplete", "status": "True", "reason": "ReconcileCompleted", "message": "reconcile completed",
				"lastTransitionTime": "not-a-timestamp",
			}))

		target := testutil.NewTarget(t, testutil.TargetConfig{
			ListKinds: dsciReadinessListKinds,
			Objects:   []*unstructured.Unstructured{dsci},
		})

		chk := dscinitialization.NewDSCInitializationReadinessCheck()
		_, err := chk.Validate(t.Context(), target)

		g.Expect(err).To(HaveOccurred())
	})
}

func TestDSCInitializationReadinessCheck_Ready(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithPhase("Ready")},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("DSCInitialization is ready"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
}

func TestDSCInitializationReadinessCheck_ReadyWithUnhappyConditions(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Ready",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "False", "message": "reconcile failed: some error"},
					map[string]any{"type": "Degraded", "status": "False", "message": "not degraded"},
					map[string]any{"type": "Progressing", "status": "False", "message": "not progressing"},
					map[string]any{"type": "Available", "status": "True", "message": "available"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": And(ContainSubstring("phase is Ready"), ContainSubstring("ReconcileComplete: reconcile failed: some error")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dscinitialization.NewDSCInitializationReadinessCheck()

	g.Expect(chk.ID()).To(Equal("platform.dsci.readiness"))
	g.Expect(chk.Name()).To(Equal("Platform :: DSCI :: Readiness Check"))
	g.Expect(chk.Group()).To(Equal(check.GroupPlatform))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
