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

func TestManagedISVCConfigAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.ManagedISVCConfigAction{}
	g.Expect(a.ID()).To(Equal("modelserving.managed-isvc-config"))
}

func TestManagedISVCConfigAction_CanApply(t *testing.T) {
	t.Run("should return true for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ManagedISVCConfigAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return true for version 3.5", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ManagedISVCConfigAction{}
		v := semver.MustParse("3.5.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 2.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ManagedISVCConfigAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestManagedISVCConfigAction_Prepare(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.ManagedISVCConfigAction{}
	g.Expect(a.Prepare()).To(BeNil())
}

func TestManagedISVCConfigAction_RunExecute(t *testing.T) {
	t.Run("should set managed=true on ConfigMap", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsci := newDSCI(testApplicationsNamespace)
		configMap := newISVCConfigMap(testApplicationsNamespace,
			map[string]string{"opendatahub.io/managed": "false"}, "")
		deployment := newDeployment(testApplicationsNamespace, "kserve-controller-manager")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
			resources.ConfigMap.GVR():         resources.ConfigMap.ListKind(),
			resources.Deployment.GVR():        resources.Deployment.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, dsci, configMap, deployment,
		)

		v := semver.MustParse("3.0.0")

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := action.Target{
			Client:         testClient,
			CurrentVersion: &v,
			DryRun:         false,
			SkipConfirm:    true,
			Recorder:       action.NewRootRecorder(),
		}

		a := &modelserving.ManagedISVCConfigAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ConfigMap was updated with managed=true
		updated, err := dynamicClient.Resource(resources.ConfigMap.GVR()).
			Namespace(testApplicationsNamespace).
			Get(ctx, testConfigMapName, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("opendatahub.io/managed", "true"))
	})

	t.Run("should skip when ConfigMap not found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsci := newDSCI(testApplicationsNamespace)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
			resources.ConfigMap.GVR():         resources.ConfigMap.ListKind(),
			resources.Deployment.GVR():        resources.Deployment.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, dsci,
		)

		v := semver.MustParse("3.0.0")

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := action.Target{
			Client:         testClient,
			CurrentVersion: &v,
			DryRun:         false,
			SkipConfirm:    true,
			Recorder:       action.NewRootRecorder(),
		}

		a := &modelserving.ManagedISVCConfigAction{}
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

		dsci := newDSCI(testApplicationsNamespace)
		configMap := newISVCConfigMap(testApplicationsNamespace,
			map[string]string{"opendatahub.io/managed": "false"}, "")
		deployment := newDeployment(testApplicationsNamespace, "kserve-controller-manager")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
			resources.ConfigMap.GVR():         resources.ConfigMap.ListKind(),
			resources.Deployment.GVR():        resources.Deployment.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, dsci, configMap, deployment,
		)

		v := semver.MustParse("3.0.0")

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := action.Target{
			Client:         testClient,
			CurrentVersion: &v,
			DryRun:         true,
			SkipConfirm:    true,
			Recorder:       action.NewRootRecorder(),
		}

		a := &modelserving.ManagedISVCConfigAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ConfigMap was NOT updated
		original, err := dynamicClient.Resource(resources.ConfigMap.GVR()).
			Namespace(testApplicationsNamespace).
			Get(ctx, testConfigMapName, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("opendatahub.io/managed", "false"))
	})
}
