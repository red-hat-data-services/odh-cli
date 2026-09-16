package trainer_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/trainer"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Shared immutable test specifications.
var (
	stringNumProcPerNodeSpec = map[string]any{
		"trainer": map[string]any{"numProcPerNode": "auto"},
	}
	numericNumProcPerNodeSpec = map[string]any{
		"trainer": map[string]any{"numProcPerNode": float64(2)},
	}
	nilNumProcPerNodeSpec = map[string]any{
		"trainer": map[string]any{"numProcPerNode": nil},
	}
	missingNumProcPerNodeSpec = map[string]any{}
)

func TestNumProcPerNodeCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := trainer.NewNumProcPerNodeCheck()

	g.Expect(chk.ID()).To(Equal("workloads.trainer.numprocpernode"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Trainer :: numProcPerNode (3.6+)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("trainer"))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestNumProcPerNodeCheck_CanApply(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		targetVersion  string
		managed        string
		want           bool
	}{
		{name: "upgrade to 3.6 with managed Trainer", currentVersion: "3.5.0", targetVersion: "3.6.0", managed: "Managed", want: true},
		{name: "target below 3.6", currentVersion: "3.5.0", targetVersion: "3.5.0", managed: "Managed", want: false},
		{name: "current already 3.6", currentVersion: "3.6.0", targetVersion: "3.6.0", managed: "Managed", want: false},
		{name: "Trainer removed", currentVersion: "3.5.0", targetVersion: "3.6.0", managed: "Removed", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainer": tc.managed})},
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			})

			canApply, err := trainer.NewNumProcPerNodeCheck().CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.want))
		})
	}
}

func TestNumProcPerNodeCheck_StringValueBlocks(t *testing.T) {
	g := NewWithT(t)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			managedTrainerDSC(),
			newTrainJob("legacy-job", "test-ns", stringNumProcPerNodeSpec),
		},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	diagnostic, err := trainer.NewNumProcPerNodeCheck().Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(diagnostic.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": And(ContainSubstring("Found 1 TrainJob(s)"), ContainSubstring("must be deleted")),
	}))
	g.Expect(diagnostic.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(diagnostic.ImpactedObjects).To(HaveLen(1))
}

func TestNumProcPerNodeCheck_NonStringValuesDoNotMatch(t *testing.T) {
	g := NewWithT(t)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			managedTrainerDSC(),
			newTrainJob("numeric-job", "test-ns", numericNumProcPerNodeSpec),
			newTrainJob("nil-job", "test-ns", nilNumProcPerNodeSpec),
			newTrainJob("missing-job", "test-ns", missingNumProcPerNodeSpec),
		},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	diagnostic, err := trainer.NewNumProcPerNodeCheck().Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(diagnostic.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(diagnostic.ImpactedObjects).To(BeEmpty())
}
