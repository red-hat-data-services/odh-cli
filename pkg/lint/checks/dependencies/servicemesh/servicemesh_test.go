package servicemesh_test

import (
	"testing"

	"github.com/blang/semver/v4"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/servicemesh"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestServiceMeshV3Check_NotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:            operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": And(
			ContainSubstring("not installed"),
			ContainSubstring("latest 3.x"),
			ContainSubstring("3.1.0"),
			ContainSubstring("disconnected install"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_InstalledVersionTooOld(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator3",
			Namespace: "openshift-operators",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "servicemeshoperator3.v3.0.0",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("3.0.0"),
			ContainSubstring("3.1.0"),
			ContainSubstring("disconnected install"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "servicemeshoperator3.v3.0.0"))
}

func TestServiceMeshV3Check_InstalledVersionMeetsMinimum(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator3",
			Namespace: "openshift-operators",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "servicemeshoperator3.v3.1.0",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("3.1.0"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "servicemeshoperator3.v3.1.0"))
}

func TestServiceMeshV3Check_InstalledVersionAboveMinimum(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator3",
			Namespace: "openshift-operators",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "servicemeshoperator3.v3.2.0",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("3.2.0"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "servicemeshoperator3.v3.2.0"))
}

func TestServiceMeshV3Check_InstalledVersionUnparseable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator3",
			Namespace: "openshift-operators",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "servicemeshoperator3-badversion",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("could not be determined"), ContainSubstring("servicemeshoperator3-badversion")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_CanApply_2xTo3x(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := smCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestServiceMeshV3Check_CanApply_2xTo2x(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("2.18.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := smCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestServiceMeshV3Check_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.1.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := smCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestServiceMeshV3Check_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	canApply, err := smCheck.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestServiceMeshV3Check_Metadata(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	g.Expect(smCheck.ID()).To(Equal("dependencies.servicemesh.installed"))
	g.Expect(smCheck.Name()).To(Equal("Dependencies :: Service Mesh v3 :: Installed"))
	g.Expect(smCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(smCheck.Description()).ToNot(BeEmpty())
}
