package aipipelines_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var dspaListKinds = map[schema.GroupVersionResource]string{
	resources.DataSciencePipelinesApplicationV1.GVR():       resources.DataSciencePipelinesApplicationV1.ListKind(),
	resources.DataSciencePipelinesApplicationV1Alpha1.GVR(): resources.DataSciencePipelinesApplicationV1Alpha1.ListKind(),
}

func TestListDSPAs_V1(t *testing.T) {
	g := NewWithT(t)

	dspa := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataSciencePipelinesApplicationV1.APIVersion(),
			"kind":       resources.DataSciencePipelinesApplicationV1.Kind,
			"metadata": map[string]any{
				"name":      "dspa",
				"namespace": "team-ns",
			},
		},
	}
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dspaListKinds,
		Objects:   []*unstructured.Unstructured{dspa},
	})

	objs, usedType, err := aipipelines.ListDSPAs(t.Context(), target.Client)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(usedType).To(Equal(resources.DataSciencePipelinesApplicationV1))
	g.Expect(objs).To(HaveLen(1))
}

func TestHasDSPAs(t *testing.T) {
	g := NewWithT(t)

	dspa := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataSciencePipelinesApplicationV1.APIVersion(),
			"kind":       resources.DataSciencePipelinesApplicationV1.Kind,
			"metadata": map[string]any{
				"name":      "dspa",
				"namespace": "team-ns",
			},
		},
	}
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dspaListKinds,
		Objects:   []*unstructured.Unstructured{dspa},
	})

	exists, err := aipipelines.HasDSPAs(t.Context(), target.Client)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(exists).To(BeTrue())
}
