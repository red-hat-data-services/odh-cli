package modelserving

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	hpIgnorelistActionID          = "modelserving.hardwareprofiles-ignorelist"
	hpIgnorelistActionName        = "Add hardware profile annotations to ignore list"
	hpIgnorelistActionDescription = "Patches inferenceservice-config ConfigMap to set opendatahub.io/managed=false and add hardware-profile annotations to serviceAnnotationDisallowedList before upgrading to RHOAI 3.x"

	msgHPConfigMapNotFound       = "inferenceservice-config ConfigMap not found in namespace %s - no action needed"
	msgHPManagedAnnotationSet    = "Set annotation %s=%s on ConfigMap %s"
	msgHPManagedAnnotationDryRun = "Would set annotation %s=%s on ConfigMap %s"
	msgHPDisallowedListUpdated   = "Added %d annotation(s) to serviceAnnotationDisallowedList: %v"
	msgHPDisallowedListDryRun    = "Would add %d annotation(s) to serviceAnnotationDisallowedList: %v"
	msgHPDisallowedListCurrent   = "serviceAnnotationDisallowedList already contains all required annotations"
	msgHPUpdateFailed            = "Failed to update ConfigMap %s/%s: %v"
	msgHPComplete                = "inferenceservice-config ConfigMap updated successfully"
	msgHPCompleteDryRun          = "Dry-run: no changes applied to inferenceservice-config ConfigMap"
)

// requiredDisallowedAnnotations lists annotations that must be in serviceAnnotationDisallowedList.
//
//nolint:gochecknoglobals // Constant-like list used across action methods.
var requiredDisallowedAnnotations = []string{
	annotationHardwareProfileName,
	annotationHardwareProfileNS,
}

// HardwareProfilesIgnorelistAction patches inferenceservice-config to prevent
// hardware-profile annotations from triggering reconciliation loops during upgrade.
type HardwareProfilesIgnorelistAction struct{}

func (a *HardwareProfilesIgnorelistAction) ID() string {
	return hpIgnorelistActionID
}

func (a *HardwareProfilesIgnorelistAction) Name() string {
	return hpIgnorelistActionName
}

func (a *HardwareProfilesIgnorelistAction) Description() string {
	return hpIgnorelistActionDescription
}

func (a *HardwareProfilesIgnorelistAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *HardwareProfilesIgnorelistAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *HardwareProfilesIgnorelistAction) CanApply(target action.Target) bool {
	return target.CurrentVersion.Major == 2 && target.CurrentVersion.Minor >= 25
}

func (a *HardwareProfilesIgnorelistAction) Prepare() action.Task {
	return &hpIgnorelistPrepareTask{action: a}
}

func (a *HardwareProfilesIgnorelistAction) Run() action.Task {
	return &hpIgnorelistRunTask{action: a}
}

// updateConfigMap sets managed=false and adds missing annotations to the disallowed list.
func (a *HardwareProfilesIgnorelistAction) updateConfigMap(
	ctx context.Context,
	target action.Target,
	namespace string,
) {
	step := target.Recorder.Child(
		"update-inferenceservice-config",
		"Update inferenceservice-config ConfigMap",
	)

	configMap, err := getInferenceServiceConfig(ctx, target, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepSkipped, msgHPConfigMapNotFound, namespace)

			return
		}

		step.Completef(result.StepFailed, msgGetConfigMapFailed, namespace, inferenceServiceConfigName, err)

		return
	}

	// Set managed=false annotation
	a.setManagedAnnotation(target, configMap, step)

	// Add missing annotations to disallowed list
	a.updateDisallowedList(target, configMap, step)

	if target.DryRun {
		step.Completef(result.StepSkipped, msgHPCompleteDryRun)

		return
	}

	// Apply the update
	_, err = target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(namespace).
		Update(ctx, configMap, metav1.UpdateOptions{})

	if err != nil {
		step.Completef(result.StepFailed, msgHPUpdateFailed, namespace, inferenceServiceConfigName, err)

		return
	}

	step.Completef(result.StepCompleted, msgHPComplete)
}

func (a *HardwareProfilesIgnorelistAction) setManagedAnnotation(
	target action.Target,
	configMap *unstructured.Unstructured,
	parentStep action.StepRecorder,
) {
	step := parentStep.Child("set-managed-annotation", "Set managed annotation")

	if target.DryRun {
		step.Completef(result.StepSkipped, msgHPManagedAnnotationDryRun, annotationManaged, managedFalse, inferenceServiceConfigName)

		return
	}

	annotations := configMap.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[annotationManaged] = managedFalse
	configMap.SetAnnotations(annotations)

	step.Completef(result.StepCompleted, msgHPManagedAnnotationSet, annotationManaged, managedFalse, inferenceServiceConfigName)
}

func (a *HardwareProfilesIgnorelistAction) updateDisallowedList(
	target action.Target,
	configMap *unstructured.Unstructured,
	parentStep action.StepRecorder,
) {
	step := parentStep.Child("update-disallowed-list", "Update serviceAnnotationDisallowedList")

	cfg, err := parseISVCConfigData(configMap)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to parse inferenceService config: %v", err)

		return
	}

	currentList, err := cfg.disallowedList()
	if err != nil {
		step.Completef(result.StepFailed, "Failed to read disallowed list: %v", err)

		return
	}

	// Find missing annotations
	var missing []string
	for _, annotation := range requiredDisallowedAnnotations {
		if !slices.Contains(currentList, annotation) {
			missing = append(missing, annotation)
		}
	}

	if len(missing) == 0 {
		step.Completef(result.StepCompleted, msgHPDisallowedListCurrent)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, msgHPDisallowedListDryRun, len(missing), missing)

		return
	}

	// Add missing annotations and serialize back (preserves all other fields)
	if err := cfg.setDisallowedList(append(currentList, missing...)); err != nil {
		step.Completef(result.StepFailed, "Failed to update disallowed list: %v", err)

		return
	}

	updatedJSON, err := json.Marshal(cfg)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to marshal updated config: %v", err)

		return
	}

	if err := jq.Transform(configMap, ".data.%s = %q", inferenceServiceDataKey, string(updatedJSON)); err != nil {
		step.Completef(result.StepFailed, "Failed to update ConfigMap data: %v", err)

		return
	}

	step.Completef(result.StepCompleted, msgHPDisallowedListUpdated, len(missing), missing)
}

// restartKServeController restarts the KServe controller deployment.
func (a *HardwareProfilesIgnorelistAction) restartKServeController(
	ctx context.Context,
	target action.Target,
	namespace string,
) {
	step := target.Recorder.Child(
		"restart-kserve-controller",
		"Restart KServe controller manager",
	)

	restartDeployment(ctx, target, namespace, kserveControllerDeployment, step)
}

// --- Prepare Task ---

type hpIgnorelistPrepareTask struct {
	action *HardwareProfilesIgnorelistAction
}

func (t *hpIgnorelistPrepareTask) Validate(
	_ context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *hpIgnorelistPrepareTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child(
		"backup-inferenceservice-config",
		"Backup inferenceservice-config ConfigMap",
	)

	namespace, err := getApplicationsNamespace(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, msgGetAppNamespaceFailed, err)

		return action.BuildResult(target)
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup ConfigMap %s from namespace %s", inferenceServiceConfigName, namespace)

		return action.BuildResult(target)
	}

	configMap, err := getInferenceServiceConfig(ctx, target, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepSkipped, msgHPConfigMapNotFound, namespace)
		} else {
			step.Completef(result.StepFailed, msgGetConfigMapFailed, namespace, inferenceServiceConfigName, err)
		}

		return action.BuildResult(target)
	}

	outputDir := filepath.Join(target.OutputDir, namespace)

	if err := backup.WriteResourcesToDir(outputDir, resources.ConfigMap.GVR(), []*unstructured.Unstructured{configMap}); err != nil {
		step.Completef(result.StepFailed, "Failed to write ConfigMap backup: %v", err)
	} else {
		step.Completef(result.StepCompleted, "Backed up ConfigMap %s to %s", inferenceServiceConfigName, outputDir)
	}

	return action.BuildResult(target)
}

// --- Run Task ---

type hpIgnorelistRunTask struct {
	action *HardwareProfilesIgnorelistAction
}

func (t *hpIgnorelistRunTask) Validate(
	_ context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *hpIgnorelistRunTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	namespace, err := getApplicationsNamespace(ctx, target)
	if err != nil {
		step := target.Recorder.Child("get-namespace", "Get applications namespace")
		step.Completef(result.StepFailed, msgGetAppNamespaceFailed, err)

		return action.BuildResult(target)
	}

	t.action.updateConfigMap(ctx, target, namespace)
	t.action.restartKServeController(ctx, target, namespace)

	return action.BuildResult(target)
}
