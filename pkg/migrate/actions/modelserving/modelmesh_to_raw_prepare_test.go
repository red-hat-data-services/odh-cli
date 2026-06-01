package modelserving_test

import (
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestModelMeshToRawAction_PrepareExecute(t *testing.T) {
	t.Run("should backup ModelMesh ISVCs and ServingRuntimes", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)
		target.OutputDir = t.TempDir()

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Prepare().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())
		g.Expect(actionResult.Status.Completed).To(BeTrue())

		hasCompleted := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepCompleted {
				hasCompleted = true
			}
		}

		g.Expect(hasCompleted).To(BeTrue())
	})

	t.Run("should skip when no ModelMesh ISVCs exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)
		target.OutputDir = t.TempDir()

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Prepare().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		hasSkipped := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepSkipped {
				hasSkipped = true
			}
		}

		g.Expect(hasSkipped).To(BeTrue())
	})

	t.Run("should not write files in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)
		target.OutputDir = t.TempDir()

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Prepare().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify no files were written
		entries, err := os.ReadDir(target.OutputDir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(BeEmpty())
	})
}
