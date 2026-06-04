package aipipelines_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture
var dspaListKinds = map[schema.GroupVersionResource]string{
	resources.DataSciencePipelinesApplicationV1.GVR():       resources.DataSciencePipelinesApplicationV1.ListKind(),
	resources.DataSciencePipelinesApplicationV1Alpha1.GVR(): resources.DataSciencePipelinesApplicationV1Alpha1.ListKind(),
	resources.Pod.GVR():  resources.Pod.ListKind(),
	resources.Role.GVR(): resources.Role.ListKind(),
}

func newFakeClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dspaListKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: fakeDynamic,
	})
}

func makeDSPA(name, namespace, apiVersion string) *unstructured.Unstructured {
	var rt resources.ResourceType

	switch apiVersion {
	case "v1alpha1":
		rt = resources.DataSciencePipelinesApplicationV1Alpha1
	case "v1":
		rt = resources.DataSciencePipelinesApplicationV1
	default:
		panic("unsupported apiVersion in test fixture: " + apiVersion)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{},
		},
	}
}

func TestListDSPAs(t *testing.T) {
	t.Run("lists v1 DSPAs", func(t *testing.T) {
		g := NewWithT(t)

		c := newFakeClient(
			makeDSPA("dspa1", "ns1", "v1"),
			makeDSPA("dspa2", "ns2", "v1"),
		)

		dspas, err := aipipelines.ListDSPAs(context.Background(), c, resources.DataSciencePipelinesApplicationV1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dspas).To(HaveLen(2))
	})

	t.Run("lists v1alpha1 DSPAs", func(t *testing.T) {
		g := NewWithT(t)

		c := newFakeClient(
			makeDSPA("dspa1", "ns1", "v1alpha1"),
		)

		dspas, err := aipipelines.ListDSPAs(context.Background(), c, resources.DataSciencePipelinesApplicationV1Alpha1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dspas).To(HaveLen(1))
	})

	t.Run("returns empty when none exist", func(t *testing.T) {
		g := NewWithT(t)

		c := newFakeClient()

		dspas, err := aipipelines.ListDSPAs(context.Background(), c, resources.DataSciencePipelinesApplicationV1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dspas).To(BeEmpty())
	})
}
