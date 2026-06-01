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

func TestServerlessToRawAction_PrepareExecute(t *testing.T) {
	t.Run("should backup Serverless ISVCs to output dir", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)
		target.OutputDir = t.TempDir()

		a := &modelserving.ServerlessToRawAction{}
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

	t.Run("should skip when no Serverless ISVCs exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)
		target.OutputDir = t.TempDir()

		a := &modelserving.ServerlessToRawAction{}
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

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)
		target.OutputDir = t.TempDir()

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Prepare().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify no files were written
		entries, err := os.ReadDir(target.OutputDir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(BeEmpty())
	})
}
