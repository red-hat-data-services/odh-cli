package components_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture
var dscListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newDSC(componentsSpec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": componentsSpec,
			},
		},
	}
}

func newTestClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func TestDiscoverComponents(t *testing.T) {
	t.Run("discovers components from DSC", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
			"kserve": map[string]any{
				"managementState": "Removed",
			},
			"ray": map[string]any{
				"managementState": "Unmanaged",
			},
		})

		k8sClient := newTestClient(dsc)

		result, err := components.DiscoverComponents(ctx, k8sClient)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(HaveLen(3))

		// Results should be sorted alphabetically
		g.Expect(result[0].Name).To(Equal("dashboard"))
		g.Expect(result[0].ManagementState).To(Equal("Managed"))
		g.Expect(result[1].Name).To(Equal("kserve"))
		g.Expect(result[1].ManagementState).To(Equal("Removed"))
		g.Expect(result[2].Name).To(Equal("ray"))
		g.Expect(result[2].ManagementState).To(Equal("Unmanaged"))
	})

	t.Run("returns empty list when components map is empty", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{})

		k8sClient := newTestClient(dsc)

		result, err := components.DiscoverComponents(ctx, k8sClient)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeEmpty())
	})

	t.Run("defaults to Removed when managementState is missing", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{},
		})

		k8sClient := newTestClient(dsc)

		result, err := components.DiscoverComponents(ctx, k8sClient)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(HaveLen(1))
		g.Expect(result[0].ManagementState).To(Equal("Removed"))
	})
}

func TestGetComponent(t *testing.T) {
	t.Run("returns component info for valid component", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"kserve": map[string]any{
				"managementState": "Managed",
			},
		})

		k8sClient := newTestClient(dsc)

		result, err := components.GetComponent(ctx, k8sClient, "kserve")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.Name).To(Equal("kserve"))
		g.Expect(result.ManagementState).To(Equal("Managed"))
	})

	t.Run("returns error for non-existent component", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		k8sClient := newTestClient(dsc)

		_, err := components.GetComponent(ctx, k8sClient, "unknown-component")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not found"))
	})
}

func TestGetComponent_NoDSC(t *testing.T) {
	t.Run("returns error when DSC does not exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.Background()

		k8sClient := newTestClient()

		_, err := components.GetComponent(ctx, k8sClient, "dashboard")

		g.Expect(err).To(HaveOccurred())
	})
}
