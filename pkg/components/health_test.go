package components_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

const componentCRGroup = "components.platform.opendatahub.io"

func newDashboardCR(ready bool, message string) *unstructured.Unstructured {
	conditions := []any{
		map[string]any{
			"type":    "Ready",
			"status":  boolToConditionStatus(ready),
			"reason":  "ComponentReady",
			"message": message,
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": componentCRGroup + "/v1alpha1",
			"kind":       "Dashboard",
			"metadata": map[string]any{
				"name": "default",
			},
			"status": map[string]any{
				"conditions": conditions,
			},
		},
	}
}

func boolToConditionStatus(b bool) string {
	if b {
		return string(metav1.ConditionTrue)
	}

	return string(metav1.ConditionFalse)
}

func newHealthTestClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()

	listKinds := map[schema.GroupVersionResource]string{
		resources.DataScienceCluster.GVR():                                     resources.DataScienceCluster.ListKind(),
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "dashboards"}: "DashboardList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "kserves"}:    "KserveList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func TestEnrichWithHealth(t *testing.T) {
	t.Run("enriches active components with health info", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dashboardCR := newDashboardCR(true, "")
		k8sClient := newHealthTestClient(dashboardCR)

		comps := []components.ComponentInfo{
			{Name: "dashboard", ManagementState: "Managed"},
		}

		result := components.EnrichWithHealth(ctx, k8sClient, comps)

		g.Expect(result).To(HaveLen(1))
		g.Expect(result[0].Ready).ToNot(BeNil())
		g.Expect(*result[0].Ready).To(BeTrue())
	})

	t.Run("skips health enrichment for Removed components", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		k8sClient := newHealthTestClient()

		comps := []components.ComponentInfo{
			{Name: "dashboard", ManagementState: "Removed"},
		}

		result := components.EnrichWithHealth(ctx, k8sClient, comps)

		g.Expect(result).To(HaveLen(1))
		g.Expect(result[0].Ready).To(BeNil())
	})

	t.Run("preserves original slice when enrichment fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// No CR exists, so health fetch will fail gracefully
		k8sClient := newHealthTestClient()

		comps := []components.ComponentInfo{
			{Name: "dashboard", ManagementState: "Managed"},
		}

		result := components.EnrichWithHealth(ctx, k8sClient, comps)

		g.Expect(result).To(HaveLen(1))
		g.Expect(result[0].Name).To(Equal("dashboard"))
		g.Expect(result[0].Ready).To(BeNil())
		g.Expect(result[0].Message).To(BeEmpty())
	})
}

func TestGetComponentHealth(t *testing.T) {
	t.Run("returns health info for known component", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dashboardCR := newDashboardCR(true, "All systems operational")
		k8sClient := newHealthTestClient(dashboardCR)

		health, err := components.GetComponentHealth(ctx, k8sClient, "dashboard")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(health).ToNot(BeNil())
		g.Expect(health.Ready).ToNot(BeNil())
		g.Expect(*health.Ready).To(BeTrue())
	})

	t.Run("returns not ready with message", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dashboardCR := newDashboardCR(false, "Deployment pending")
		k8sClient := newHealthTestClient(dashboardCR)

		health, err := components.GetComponentHealth(ctx, k8sClient, "dashboard")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(health.Ready).ToNot(BeNil())
		g.Expect(*health.Ready).To(BeFalse())
		g.Expect(health.Message).To(Equal("Deployment pending"))
	})

	t.Run("returns error for unknown component", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		k8sClient := newHealthTestClient()

		_, err := components.GetComponentHealth(ctx, k8sClient, "unknown-component")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unknown component CR"))
	})

	t.Run("returns empty health when no CR instances exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		k8sClient := newHealthTestClient()

		health, err := components.GetComponentHealth(ctx, k8sClient, "dashboard")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(health).ToNot(BeNil())
		g.Expect(health.Ready).To(BeNil())
	})
}
