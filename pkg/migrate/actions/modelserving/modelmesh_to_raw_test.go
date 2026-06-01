package modelserving_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func newModelMeshISVC(namespace, name, runtimeName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"uid":       "test-uid-mm-123",
				"annotations": map[string]any{
					"serving.kserve.io/deploymentMode": "ModelMesh",
				},
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": runtimeName,
					},
				},
			},
		},
	}
}

func newServingRuntime(namespace, name string, multiModel bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"multiModel": multiModel,
			},
		},
	}
}

func TestModelMeshToRawAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.ModelMeshToRawAction{}
	g.Expect(a.ID()).To(Equal("modelserving.modelmesh-to-raw"))
}

func TestModelMeshToRawAction_CanApply(t *testing.T) {
	t.Run("should return true for version 2.25", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ModelMeshToRawAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ModelMeshToRawAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestModelMeshToRawAction_RunValidate(t *testing.T) {
	t.Run("should report ModelMesh ISVCs found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
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
}

func TestModelMeshToRawAction_RunExecute(t *testing.T) {
	t.Run("should convert ModelMesh ISVCs to RawDeployment", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr,
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

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was patched to RawDeployment
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "mm-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
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

		a := &modelserving.ModelMeshToRawAction{}
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

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was NOT patched
		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "mm-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
	})
}
