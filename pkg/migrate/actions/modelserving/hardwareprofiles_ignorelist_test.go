package modelserving_test

import (
	"encoding/json"
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

	. "github.com/onsi/gomega"
)

func TestHardwareProfilesIgnorelistAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.HardwareProfilesIgnorelistAction{}
	g.Expect(a.ID()).To(Equal("modelserving.hardwareprofiles-ignorelist"))
}

func TestHardwareProfilesIgnorelistAction_CanApply(t *testing.T) {
	t.Run("should return true for version 2.25", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 1.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		v := semver.MustParse("1.10.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})

	t.Run("should return false for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})

	t.Run("should return false for version 2.24", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		v := semver.MustParse("2.24.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestHardwareProfilesIgnorelistAction_RunExecute(t *testing.T) {
	t.Run("should update ConfigMap with managed=false and disallowed annotations", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvcConfig := map[string]any{
			"serviceAnnotationDisallowedList": []string{},
		}
		isvcConfigJSON, _ := json.Marshal(isvcConfig)

		dsci := newDSCI(testApplicationsNamespace)
		configMap := newISVCConfigMap(testApplicationsNamespace, nil, string(isvcConfigJSON))
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

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())
		g.Expect(actionResult.Status.Completed).To(BeTrue())

		// Verify ConfigMap was updated
		updated, err := dynamicClient.Resource(resources.ConfigMap.GVR()).
			Namespace(testApplicationsNamespace).
			Get(ctx, testConfigMapName, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		// Verify managed annotation
		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("opendatahub.io/managed", "false"))
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

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Should have a skipped step
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

		isvcConfig := map[string]any{
			"serviceAnnotationDisallowedList": []string{},
		}
		isvcConfigJSON, _ := json.Marshal(isvcConfig)

		dsci := newDSCI(testApplicationsNamespace)
		configMap := newISVCConfigMap(testApplicationsNamespace, nil, string(isvcConfigJSON))
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

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.HardwareProfilesIgnorelistAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ConfigMap was NOT updated (no managed annotation)
		original, err := dynamicClient.Resource(resources.ConfigMap.GVR()).
			Namespace(testApplicationsNamespace).
			Get(ctx, testConfigMapName, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).ToNot(HaveKey("opendatahub.io/managed"))
	})
}
