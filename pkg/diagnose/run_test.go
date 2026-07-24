package diagnose_test

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/opendatahub-io/odh-cli/pkg/diagnose"
)

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(objs...).Build()
}

func makeDeployment(ns, name string, ready, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: ready, Replicas: replicas},
	}
}

func makePod(ns, name, phase string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status:     corev1.PodStatus{Phase: corev1.PodPhase(phase)},
	}
}

func baseConfig(cl client.Client) diagnose.Config {
	return diagnose.Config{
		Client:       cl,
		AppsNS:       "opendatahub",
		OperatorNS:   "opendatahub-operator-system",
		OperatorName: "opendatahub-operator-controller-manager",
	}
}

// TestRunReturnsReport verifies Run completes without error and returns a populated report.
// DSCI/DSC CRDs don't exist in fake clients so the cluster always reports unhealthy —
// but the report structure must be populated correctly.
func TestRunReturnsReport(t *testing.T) {
	cl := newFakeClient(
		makeDeployment("opendatahub-operator-system", "opendatahub-operator-controller-manager", 1, 1),
		makePod("opendatahub-operator-system", "op-pod", "Running"),
	)

	report, err := diagnose.Run(context.Background(), baseConfig(cl))
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if report == nil {
		t.Fatal("Run() returned nil report")
	}

	if report.Health == nil {
		t.Error("report.Health should not be nil")
	}

	// Classification is populated when cluster is unhealthy.
	if !report.Healthy && report.Classification == nil {
		t.Error("expected Classification to be set when cluster is unhealthy")
	}
}

// TestRunComponentScope verifies that TargetComponent limits Correlate to one component.
func TestRunComponentScope(t *testing.T) {
	cl := newFakeClient(
		makeDeployment("opendatahub-operator-system", "opendatahub-operator-controller-manager", 1, 1),
		makePod("opendatahub-operator-system", "op-pod", "Running"),
	)

	cfg := baseConfig(cl)
	cfg.TargetComponent = "dashboard"

	report, err := diagnose.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(report.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(report.Components))
	}

	if report.Components[0].Component != "dashboard" {
		t.Errorf("expected component=dashboard, got %s", report.Components[0].Component)
	}
}

// TestRunNilClient verifies Run returns an error when no client is provided.
func TestRunNilClient(t *testing.T) {
	_, err := diagnose.Run(context.Background(), diagnose.Config{})
	if err == nil {
		t.Error("expected error with nil client, got nil")
	}
}
