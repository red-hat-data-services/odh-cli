package datasciencepipelines_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var customRBACListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():                resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR():              resources.DataScienceClusterV1.ListKind(),
	resources.Role.GVR():                              resources.Role.ListKind(),
	resources.DataSciencePipelinesApplicationV1.GVR(): resources.DataSciencePipelinesApplicationV1.ListKind(),
}

func makeCustomRBACRole(name, namespace string, rules []map[string]any) *unstructured.Unstructured {
	rulesAny := make([]any, len(rules))
	for i, r := range rules {
		rulesAny[i] = r
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Role.APIVersion(),
			"kind":       resources.Role.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"rules": rulesAny,
		},
	}
}

func TestCustomRBACAPISubresourceCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := datasciencepipelines.NewCustomRBACAPISubresourceCheck()

	g.Expect(chk.ID()).To(Equal("workloads.datasciencepipelines.custom-rbac-api-subresource"))
	g.Expect(chk.Name()).To(Equal("Workloads :: DataSciencePipelines :: Custom RBAC API Subresource (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestCustomRBACAPISubresourceCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := datasciencepipelines.NewCustomRBACAPISubresourceCheck()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      customRBACListKinds,
		CurrentVersion: "2.25.0",
		TargetVersion:  "2.25.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateManaged})
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      customRBACListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	dsc = testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV2: constants.ManagementStateManaged})
	dsc.SetAPIVersion(resources.DataScienceCluster.APIVersion())
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      customRBACListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	dsc = testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateRemoved})
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      customRBACListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestCustomRBACAPISubresourceCheck_Validate_NoRolesNeedingFix(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := datasciencepipelines.NewCustomRBACAPISubresourceCheck()
	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateManaged})
	role := makeCustomRBACRole("already-fixed", "team-ns", []map[string]any{
		{
			"apiGroups": []any{"route.openshift.io"},
			"resources": []any{"routes"},
			"verbs":     []any{"get"},
		},
		{
			"apiGroups": []any{"datasciencepipelinesapplications.opendatahub.io"},
			"resources": []any{"datasciencepipelinesapplications/api"},
			"verbs":     []any{"get"},
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      customRBACListKinds,
		Objects:        []*unstructured.Unstructured{dsc, role},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	dr, err := chk.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionTrue),
		"Message": ContainSubstring("ready for RHOAI 3.5 upgrade"),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactNone))
}

func TestCustomRBACAPISubresourceCheck_Validate_RolesNeedingFix(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := datasciencepipelines.NewCustomRBACAPISubresourceCheck()
	dsc := testutil.NewDSC(map[string]string{aipipelines.ComponentKeyV1: constants.ManagementStateManaged})
	role := makeCustomRBACRole("custom-role", "team-ns", []map[string]any{
		{
			"apiGroups": []any{"route.openshift.io"},
			"resources": []any{"routes"},
			"verbs":     []any{"get", "list"},
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      customRBACListKinds,
		Objects:        []*unstructured.Unstructured{dsc, role},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	dr, err := chk.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionFalse),
		"Message": ContainSubstring(
			"custom Role(s) with route.openshift.io permissions missing the datasciencepipelinesapplications/api subresource",
		),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
	g.Expect(dr.ImpactedObjects).To(HaveLen(1))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("custom-role"))
	g.Expect(dr.ImpactedObjects[0].Namespace).To(Equal("team-ns"))
}
