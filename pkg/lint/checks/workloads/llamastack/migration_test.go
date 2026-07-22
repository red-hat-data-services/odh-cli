package llamastack_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/llamastack"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestMigrationCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name           string
		currentVersion string
		targetVersion  string
		componentState string
		expected       bool
	}{
		{
			name:           "3.4 to 3.5 upgrade with component Managed",
			currentVersion: "3.4.0",
			targetVersion:  "3.5.0",
			componentState: "Managed",
			expected:       true,
		},
		{
			name:           "3.4.1 to 3.5.0 upgrade with component Managed",
			currentVersion: "3.4.1",
			targetVersion:  "3.5.0",
			componentState: "Managed",
			expected:       true,
		},
		{
			name:           "3.4 to 3.5 upgrade with component Removed",
			currentVersion: "3.4.0",
			targetVersion:  "3.5.0",
			componentState: "Removed",
			expected:       false,
		},
		{
			name:           "3.4 to 3.5 upgrade with component Unmanaged",
			currentVersion: "3.4.0",
			targetVersion:  "3.5.0",
			componentState: "Unmanaged",
			expected:       false,
		},
		{
			name:           "3.3 to 3.5 upgrade — wrong source version",
			currentVersion: "3.3.0",
			targetVersion:  "3.5.0",
			componentState: "Managed",
			expected:       false,
		},
		{
			name:           "3.5 to 3.6 upgrade — wrong target version",
			currentVersion: "3.5.0",
			targetVersion:  "3.6.0",
			componentState: "Managed",
			expected:       false,
		},
		{
			name:           "2.25 to 3.5 upgrade — wrong source major",
			currentVersion: "2.25.0",
			targetVersion:  "3.5.0",
			componentState: "Managed",
			expected:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := newDSC(map[string]string{"llamastackoperator": tc.componentState})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			})

			chk := llamastack.NewMigrationCheck()
			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestMigrationCheck_NoWorkloads(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewMigrationCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))
	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RequiresMigration"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("No LlamaStackDistribution resources found"))
	g.Expect(res.Status.Conditions[0].Impact).To(Equal(result.ImpactNone))
	g.Expect(res.ImpactedObjects).To(BeEmpty())
}

func TestMigrationCheck_WorkloadsFound(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	llsd1 := newLLSD("llsd-1", "ns-a")
	llsd2 := newLLSD("llsd-2", "ns-b")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd1, llsd2},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewMigrationCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))

	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RequiresMigration"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal("CRTypeMigration"),
	}))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("Found 2 LlamaStackDistribution(s)"))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("migrated to OGXServer v1beta1"))
	g.Expect(res.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
	g.Expect(res.Status.Conditions[0].Remediation).To(ContainSubstring("odh-cli migrate prepare"))

	g.Expect(res.ImpactedObjects).To(HaveLen(2))
	g.Expect(res.ImpactedObjects[0].Name).To(Equal("llsd-1"))
	g.Expect(res.ImpactedObjects[0].Namespace).To(Equal("ns-a"))
	g.Expect(res.ImpactedObjects[0].Annotations).To(HaveKeyWithValue("upgrade.action", "requires-migration-to-ogxserver"))
	g.Expect(res.ImpactedObjects[1].Name).To(Equal("llsd-2"))
	g.Expect(res.ImpactedObjects[1].Namespace).To(Equal("ns-b"))
}

func TestMigrationCheck_SingleWorkload(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	llsd := newLLSD("my-distribution", "test-ns")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewMigrationCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("Found 1 LlamaStackDistribution(s)"))
	g.Expect(res.ImpactedObjects).To(HaveLen(1))
	g.Expect(res.ImpactedObjects[0].Name).To(Equal("my-distribution"))
}

func TestMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := llamastack.NewMigrationCheck()

	g.Expect(chk.ID()).To(Equal("workloads.llamastack.migration"))
	g.Expect(chk.Name()).To(Equal("Workloads :: LlamaStack :: CR Migration (3.4 to 3.5)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.CheckKind()).To(Equal("llamastackdistribution"))
	g.Expect(chk.CheckType()).To(Equal("migration"))
}
