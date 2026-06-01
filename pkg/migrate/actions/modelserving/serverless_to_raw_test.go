package modelserving_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

func TestServerlessToRawAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.ServerlessToRawAction{}
	g.Expect(a.ID()).To(Equal("modelserving.serverless-to-raw"))
}

func TestServerlessToRawAction_CanApply(t *testing.T) {
	t.Run("should return true for version 2.25", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ServerlessToRawAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ServerlessToRawAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestServerlessToRawAction_RunValidate(t *testing.T) {
	t.Run("should report Serverless ISVCs found", func(t *testing.T) {
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Validate(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		hasCompleted := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepCompleted {
				hasCompleted = true
			}
		}

		g.Expect(hasCompleted).To(BeTrue())
	})

	t.Run("should report no Serverless ISVCs when none exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Validate(ctx, target)

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
}

func TestServerlessToRawAction_RunExecute(t *testing.T) {
	t.Run("should convert Serverless ISVCs to RawDeployment", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		v := semver.MustParse("2.25.0")
		tv := semver.MustParse("3.0.0")

		target := action.Target{
			Client:         testClient,
			CurrentVersion: &v,
			TargetVersion:  &tv,
			DryRun:         false,
			SkipConfirm:    true,
			Recorder:       action.NewRootRecorder(),
		}

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was patched to RawDeployment
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "serverless-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

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

	t.Run("should not mutate in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was NOT patched
		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "serverless-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "Serverless"))
	})
}
