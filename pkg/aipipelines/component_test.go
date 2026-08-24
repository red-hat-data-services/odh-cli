package aipipelines_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var componentListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():                resources.DataScienceCluster.ListKind(),
	resources.DataSciencePipelinesApplicationV1.GVR(): resources.DataSciencePipelinesApplicationV1.ListKind(),
}

func TestShouldApplyChecks_ManagedV2(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV2: constants.ManagementStateManaged})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: componentListKinds,
		Objects:   []*unstructured.Unstructured{dsc},
	})

	apply, err := aipipelines.ShouldApplyChecks(t.Context(), dsc, target.Client)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(apply).To(BeTrue())
}

func TestShouldApplyChecks_ManagedV1(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateManaged})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: componentListKinds,
		Objects:   []*unstructured.Unstructured{dsc},
	})

	apply, err := aipipelines.ShouldApplyChecks(t.Context(), dsc, target.Client)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(apply).To(BeTrue())
}

func TestShouldApplyChecks_RemovedWithDSPAs(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateRemoved})
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
		ListKinds: componentListKinds,
		Objects:   []*unstructured.Unstructured{dsc, dspa},
	})

	apply, err := aipipelines.ShouldApplyChecks(t.Context(), dsc, target.Client)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(apply).To(BeTrue())
}

func TestShouldApplyChecks_RemovedWithoutDSPAs(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateRemoved})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: componentListKinds,
		Objects:   []*unstructured.Unstructured{dsc},
	})

	apply, err := aipipelines.ShouldApplyChecks(t.Context(), dsc, target.Client)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(apply).To(BeFalse())
}

func TestHasComponentKey(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateManaged})
	hasV1, err := aipipelines.HasComponentKey(dsc, aipipelines.ComponentKeyV1)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasV1).To(BeTrue())

	hasV2, err := aipipelines.HasComponentKey(dsc, aipipelines.ComponentKeyV2)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasV2).To(BeFalse())
}

func TestUsesComponentKeyV2(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV2: constants.ManagementStateManaged})
	usesV2, err := aipipelines.UsesComponentKeyV2(dsc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(usesV2).To(BeTrue())
}

func TestEffectiveManagementState_PrefersV2(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV2: constants.ManagementStateManaged})
	state, err := aipipelines.EffectiveManagementState(dsc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(state).To(Equal(constants.ManagementStateManaged))
}

func TestEffectiveManagementState_FallsBackToV1(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateManaged})
	state, err := aipipelines.EffectiveManagementState(dsc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(state).To(Equal(constants.ManagementStateManaged))
}
