package modelserving

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	managedISVCConfigActionID          = "modelserving.managed-isvc-config"
	managedISVCConfigActionName        = "Restore managed inferenceservice-config"
	managedISVCConfigActionDescription = "Sets opendatahub.io/managed=true on inferenceservice-config ConfigMap and restarts KServe controller after upgrading to RHOAI 3.x"

	majorVersion3 = 3

	msgManagedAnnotationSet    = "Set annotation %s=%s on ConfigMap %s"
	msgManagedAnnotationDryRun = "Would set annotation %s=%s on ConfigMap %s"
	msgManagedUpdateFailed     = "Failed to update ConfigMap %s/%s: %v"
	msgManagedComplete         = "inferenceservice-config ConfigMap restored to managed state"
	msgManagedCompleteDryRun   = "Dry-run: no changes applied to inferenceservice-config ConfigMap"
)

// ManagedISVCConfigAction restores the managed annotation on inferenceservice-config
// after upgrading to RHOAI 3.x.
type ManagedISVCConfigAction struct{}

func (a *ManagedISVCConfigAction) ID() string {
	return managedISVCConfigActionID
}

func (a *ManagedISVCConfigAction) Name() string {
	return managedISVCConfigActionName
}

func (a *ManagedISVCConfigAction) Description() string {
	return managedISVCConfigActionDescription
}

func (a *ManagedISVCConfigAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *ManagedISVCConfigAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *ManagedISVCConfigAction) CanApply(target action.Target) bool {
	return target.CurrentVersion.Major == majorVersion3
}

func (a *ManagedISVCConfigAction) Prepare() action.Task {
	return nil
}

func (a *ManagedISVCConfigAction) Run() action.Task {
	return &managedISVCConfigRunTask{action: a}
}

func (a *ManagedISVCConfigAction) setManagedTrue(
	ctx context.Context,
	target action.Target,
	namespace string,
) bool {
	step := target.Recorder.Child(
		"set-managed-true",
		"Set managed=true on inferenceservice-config",
	)

	configMap, err := getInferenceServiceConfig(ctx, target, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepSkipped, msgConfigMapNotFound, inferenceServiceConfigName, namespace)

			return false
		}

		step.Completef(result.StepFailed, msgGetConfigMapFailed, namespace, inferenceServiceConfigName, err)

		return false
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, msgManagedAnnotationDryRun, annotationManaged, managedTrue, inferenceServiceConfigName)

		return false
	}

	annotations := configMap.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[annotationManaged] = managedTrue
	configMap.SetAnnotations(annotations)

	_, err = target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(namespace).
		Update(ctx, configMap, metav1.UpdateOptions{})

	if err != nil {
		step.Completef(result.StepFailed, msgManagedUpdateFailed, namespace, inferenceServiceConfigName, err)

		return false
	}

	step.Completef(result.StepCompleted, msgManagedAnnotationSet, annotationManaged, managedTrue, inferenceServiceConfigName)

	return true
}

// --- Run Task ---

type managedISVCConfigRunTask struct {
	action *ManagedISVCConfigAction
}

func (t *managedISVCConfigRunTask) Validate(
	_ context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *managedISVCConfigRunTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	namespace, err := getApplicationsNamespace(ctx, target)
	if err != nil {
		step := target.Recorder.Child("get-namespace", "Get applications namespace")
		step.Completef(result.StepFailed, msgGetAppNamespaceFailed, err)

		return action.BuildResult(target)
	}

	changed := t.action.setManagedTrue(ctx, target, namespace)

	if changed {
		restartStep := target.Recorder.Child("restart-kserve-controller", "Restart KServe controller manager")
		restartDeployment(ctx, target, namespace, kserveControllerDeployment, restartStep)
	}

	return action.BuildResult(target)
}
