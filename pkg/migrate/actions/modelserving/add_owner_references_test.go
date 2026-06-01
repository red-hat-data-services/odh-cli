package modelserving_test

import (
	"testing"

	"github.com/blang/semver/v4"

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

func TestAddOwnerReferencesAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.AddOwnerReferencesAction{}
	g.Expect(a.ID()).To(Equal("modelserving.add-owner-references"))
}

func TestAddOwnerReferencesAction_CanApply(t *testing.T) {
	t.Run("should return true for version 2.25", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.AddOwnerReferencesAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.AddOwnerReferencesAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestAddOwnerReferencesAction_Prepare(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.AddOwnerReferencesAction{}
	g.Expect(a.Prepare()).To(BeNil())
}

func TestAddOwnerReferencesAction_RunExecute(t *testing.T) {
	t.Run("should skip when no RawDeployment ISVCs exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.AddOwnerReferencesAction{}
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

	t.Run("should patch auth resources with owner references", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, testISVCName, "RawDeployment")

		sa := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": resources.ServiceAccount.APIVersion(),
				"kind":       resources.ServiceAccount.Kind,
				"metadata": map[string]any{
					"name":      testISVCName + "-sa",
					"namespace": testISVCNamespace,
				},
			},
		}

		role := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": resources.Role.APIVersion(),
				"kind":       resources.Role.Kind,
				"metadata": map[string]any{
					"name":      testISVCName + "-view-role",
					"namespace": testISVCNamespace,
				},
			},
		}

		rb := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": resources.RoleBinding.APIVersion(),
				"kind":       resources.RoleBinding.Kind,
				"metadata": map[string]any{
					"name":      testISVCName + "-view",
					"namespace": testISVCNamespace,
				},
			},
		}

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sa, role, rb,
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

		a := &modelserving.AddOwnerReferencesAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())
		g.Expect(actionResult.Status.Completed).To(BeTrue())
	})

	t.Run("should skip missing auth resources gracefully", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, testISVCName, "RawDeployment")

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

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.AddOwnerReferencesAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())
		g.Expect(actionResult.Status.Completed).To(BeTrue())
	})
}
