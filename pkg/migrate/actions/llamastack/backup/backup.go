package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	actionID          = "llamastack.backup"
	actionName        = "Backup LlamaStack Resources"
	actionDescription = "Backup all LlamaStack resources before upgrade (LlamaStack->OGX rename in 3.5)"

	dirPerms  = 0o700
	filePerms = 0o600

	podDataPath = "/opt/app-root/src/.llama/distributions/rh"

	msgErrCreateBackupDir    = "Failed to create backup directory: %v"
	msgErrListDistributions  = "Failed to list LlamaStackDistributions: %v"
	msgErrCreateLLSDDir      = "Failed to create directory: %v"
	msgErrBackupLLSD         = "Failed to write LlamaStackDistribution %s/%s: %v"
	msgErrBackupConfigMap    = "Failed to write ConfigMap %s/%s: %v"
	msgErrGetConfigMap       = "Failed to get ConfigMap: %v"
	msgErrGetConfigMapData   = "Failed to read ConfigMap data: %v"
	msgErrGetDeployment      = "Failed to get Deployment: %v"
	msgErrListPods           = "Failed to list pods in %s: %v"
	msgErrBackupPod          = "Failed to write Pod %s/%s: %v"
	msgErrCreatePodDataDir   = "Failed to create pod data directory: %v"
	msgErrWriteExtractedYAML = "Failed to write extracted %s: %v"
	msgErrBackupDeployment   = "Failed to write Deployment %s/%s: %v"
	msgErrBackupFailedFor    = "Backup failed for %s"
	msgErrCLIPath            = "Failed to find 'oc' or 'kubectl' in PATH for pod data backup: %v"
	msgErrTarPath            = "Failed to find 'tar' in PATH for pod data backup: %v"
	msgErrTarStart           = "Failed to start local tar process: %v"
	msgErrExecRun            = "Failed to execute 'oc exec' in pod %s: %v\nstderr: %s"
	msgErrTarWait            = "Failed to complete local tar extraction: %v\nstderr: %s"
	msgErrStdoutPipe         = "Failed to create stdout pipe: %v"
	msgErrEnforcePerms       = "Failed to set permissions on extracted pod data: %v"
	msgErrDirCheck           = "Failed to check data directory in pod %s: %v"
	msgInfoPodDataDirMissing = "Data directory not found in pod %s (may be empty or using different path)"
	msgInfoConfigMapNotFound = "ConfigMap not found"
	msgInfoBackedUpConfigMap = "Backed up ConfigMap"
	msgInfoBackedUpPodData   = "Backed up pod data"
	msgStepBackupAll         = "Backup LlamaStack resources"
	msgStepBackupLLSD        = "Backup %s/%s"
	msgStepBackupConfigMap   = "Backup ConfigMap %s"
	msgStepBackupPodData     = "Backup pod data for %s"
	msgInfoNoCRDPresent      = "No LlamaStackDistribution resources found (CRD not present)"
	msgInfoNoResourcesFound  = "No LlamaStackDistribution resources found"
	msgInfoBackedUpLLSD      = "Backed up %s"
	msgInfoBackedUpCount     = "Backed up %d LlamaStackDistributions"
	msgPreRenameNotice       = "Note: Backing up pre-rename resources (LlamaStack to OGX in 3.5)"
	msgCustomConfigWarning   = "\n⚠️  CUSTOM CONFIG WARNINGS"
	msgCustomConfigLine1     = "▸ %s\n"
	msgCustomConfigLine2     = "  This distribution was using a custom config.\n"
	msgCustomConfigLine3     = "  State was backed up from the default location: /opt/app-root/src/.llama/distributions/rh\n"
	msgCustomConfigLine4     = "  VERIFY in the config that this location was correct, as it may have been altered in the custom config.\n\n"
)

type LlamaStackBackupAction struct{}

func (a *LlamaStackBackupAction) ID() string                { return actionID }
func (a *LlamaStackBackupAction) Name() string              { return actionName }
func (a *LlamaStackBackupAction) Description() string       { return actionDescription }
func (a *LlamaStackBackupAction) Group() action.ActionGroup { return action.GroupBackup }
func (a *LlamaStackBackupAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *LlamaStackBackupAction) CanApply(target action.Target) bool {
	if target.TargetVersion == nil {
		return false
	}

	// LlamaStack→OGX rename lands in 3.5; after that the CRDs no longer exist.
	return target.TargetVersion.Major == 3 && target.TargetVersion.Minor <= 5
}

func (a *LlamaStackBackupAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *LlamaStackBackupAction) Run() action.Task {
	return nil
}

type prepareTask struct {
	action *LlamaStackBackupAction
}

func (t *prepareTask) Validate(_ context.Context, target action.Target) (*result.ActionResult, error) {
	return action.BuildResult(target)
}

func (t *prepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("llamastack-backup", msgStepBackupAll)

	// Create main backup directory
	if !target.DryRun {
		if err := os.MkdirAll(target.OutputDir, dirPerms); err != nil {
			step.Completef(result.StepFailed, msgErrCreateBackupDir, err)

			return action.BuildResult(target)
		}
	}

	llsdList, err := target.Client.Dynamic().Resource(resources.LlamaStackDistribution.GVR()).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			step.Completef(result.StepCompleted, msgInfoNoCRDPresent)

			return action.BuildResult(target)
		}
		step.Completef(result.StepFailed, msgErrListDistributions, err)

		return action.BuildResult(target)
	}

	if len(llsdList.Items) == 0 {
		step.Completef(result.StepCompleted, msgInfoNoResourcesFound)

		return action.BuildResult(target)
	}

	target.IO.Errorf(msgPreRenameNotice)
	target.IO.Errorln()

	customConfigs := []string{}

	for _, llsd := range llsdList.Items {
		if cfg, ok := processLLSD(ctx, target, llsd, step); ok {
			customConfigs = append(customConfigs, cfg)
		}
	}

	if len(customConfigs) > 0 {
		target.IO.Errorf(msgCustomConfigWarning)
		target.IO.Errorln()
		for _, cfg := range customConfigs {
			target.IO.Errorf(msgCustomConfigLine1, cfg)
			target.IO.Errorf(msgCustomConfigLine2)
			target.IO.Errorf(msgCustomConfigLine3)
			target.IO.Errorf(msgCustomConfigLine4)
		}
	}

	step.Completef(result.StepCompleted, msgInfoBackedUpCount, len(llsdList.Items))

	return action.BuildResult(target)
}

//nolint:nonamedreturns // named returns clarify the two-purpose return value
func processLLSD(ctx context.Context, target action.Target, llsd unstructured.Unstructured, step action.StepRecorder) (customConfig string, hasCustomConfig bool) {
	name := llsd.GetName()
	ns := llsd.GetNamespace()

	llsdStep := step.Child(fmt.Sprintf("backup-%s-%s", ns, name), fmt.Sprintf(msgStepBackupLLSD, ns, name))

	llsdDir := filepath.Join(target.OutputDir, ns, name)
	if !target.DryRun {
		if err := os.MkdirAll(llsdDir, dirPerms); err != nil {
			llsdStep.Completef(result.StepFailed, msgErrCreateLLSDDir, err)

			return "", false
		}

		// 1. Backup YAML
		if err := backup.WriteResourceFlat(llsdDir, resources.LlamaStackDistribution.GVR(), &llsd); err != nil {
			llsdStep.Completef(result.StepFailed, msgErrBackupLLSD, ns, name, err)

			return "", false
		}
	}

	// ConfigMap and pod data backups are independent; attempt both even if one
	// fails so we capture as much data as possible. Individual failures are
	// recorded immediately via child steps and the parent step is marked failed
	// after both have been attempted.
	failed := false

	// 2. ConfigMap
	isCustomConfig, ok := backupConfigMap(ctx, target, llsd, llsdDir, ns, llsdStep)
	if !ok {
		failed = true
	}

	// 3. Pod Data
	if !backupPodData(ctx, target, llsdDir, ns, name, llsdStep) {
		failed = true
	}

	if failed {
		llsdStep.Completef(result.StepFailed, msgErrBackupFailedFor, name)

		return "", false
	}

	llsdStep.Completef(result.StepCompleted, msgInfoBackedUpLLSD, name)

	if isCustomConfig {
		return fmt.Sprintf("%s/%s", ns, name), true
	}

	return "", false
}

//nolint:nonamedreturns // required to avoid revive complaining about identical unnamed boolean returns
func backupConfigMap(ctx context.Context, target action.Target, llsd unstructured.Unstructured, llsdDir, ns string, step action.StepRecorder) (hasCustomConfig, ok bool) {
	configMapName, _ := jq.Query[string](&llsd, ".spec.server.userConfig.configMapName")
	if configMapName == "" {
		return false, true
	}

	cmStep := step.Child("configmap-"+configMapName, fmt.Sprintf(msgStepBackupConfigMap, configMapName))

	cm, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).Namespace(ns).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			cmStep.Completef(result.StepSkipped, msgInfoConfigMapNotFound)

			return true, true
		}
		cmStep.Completef(result.StepFailed, msgErrGetConfigMap, err)

		return true, false
	}

	if target.DryRun {
		cmStep.Completef(result.StepCompleted, msgInfoBackedUpConfigMap)

		return true, true
	}

	if err := backup.WriteResourceFlat(llsdDir, resources.ConfigMap.GVR(), cm); err != nil {
		cmStep.Completef(result.StepFailed, msgErrBackupConfigMap, ns, configMapName, err)

		return true, false
	}

	data, err := jq.Query[map[string]any](cm, ".data")
	if err != nil && !errors.Is(err, jq.ErrNotFound) {
		cmStep.Completef(result.StepFailed, msgErrGetConfigMapData, err)

		return true, false
	}

	if runYaml, ok := data["run.yaml"].(string); ok {
		if err := os.WriteFile(filepath.Join(llsdDir, "run.yaml"), []byte(runYaml), filePerms); err != nil {
			cmStep.Completef(result.StepFailed, msgErrWriteExtractedYAML, "run.yaml", err)

			return true, false
		}
	}
	if configYaml, ok := data["config.yaml"].(string); ok {
		if err := os.WriteFile(filepath.Join(llsdDir, "config.yaml"), []byte(configYaml), filePerms); err != nil {
			cmStep.Completef(result.StepFailed, msgErrWriteExtractedYAML, "config.yaml", err)

			return true, false
		}
	}

	cmStep.Completef(result.StepCompleted, msgInfoBackedUpConfigMap)

	return true, true
}

func backupPodData(ctx context.Context, target action.Target, llsdDir, ns, name string, step action.StepRecorder) bool {
	pods, err := target.Client.Dynamic().Resource(resources.Pod.GVR()).Namespace(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=" + name,
	})
	if err != nil {
		podStep := step.Child("pod-data-"+name, fmt.Sprintf(msgStepBackupPodData, name))
		podStep.Completef(result.StepFailed, msgErrListPods, ns, err)

		return false
	}

	if len(pods.Items) == 0 {
		return true
	}

	pod := selectReadyPod(pods.Items)
	podName := pod.GetName()

	podStep := step.Child("pod-data-"+name, fmt.Sprintf(msgStepBackupPodData, name))

	if !target.DryRun {
		if err := backup.WriteResourceFlat(llsdDir, resources.Pod.GVR(), &pod); err != nil {
			podStep.Completef(result.StepFailed, msgErrBackupPod, ns, podName, err)

			return false
		}
	}

	if !backupDeploymentForPod(ctx, target, pod, llsdDir, ns, podStep) {
		return false
	}

	if !backupPodDataExecTar(ctx, target, podName, ns, llsdDir, podStep) {
		return false
	}

	podStep.Completef(result.StepCompleted, msgInfoBackedUpPodData)

	return true
}

func backupDeploymentForPod(ctx context.Context, target action.Target, pod unstructured.Unstructured, llsdDir, ns string, step action.StepRecorder) bool {
	ownerRefs := pod.GetOwnerReferences()
	for _, ref := range ownerRefs {
		if ref.Kind != "ReplicaSet" {
			continue
		}

		// Infer deployment name by removing hash
		rsName := ref.Name
		lastDash := strings.LastIndex(rsName, "-")
		if lastDash <= 0 {
			continue
		}

		deployName := rsName[:lastDash]
		deploy, err := target.Client.Dynamic().Resource(resources.Deployment.GVR()).Namespace(ns).Get(ctx, deployName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			step.Completef(result.StepFailed, msgErrGetDeployment, err)

			return false
		}

		if !target.DryRun {
			if err := backup.WriteResourceFlat(llsdDir, resources.Deployment.GVR(), deploy); err != nil {
				step.Completef(result.StepFailed, msgErrBackupDeployment, ns, deployName, err)

				return false
			}
		}
	}

	return true
}

func backupPodDataExecTar(ctx context.Context, target action.Target, podName, ns, llsdDir string, step action.StepRecorder) bool {
	if target.DryRun {
		return true
	}

	podDataDir := filepath.Join(llsdDir, "pod-data")
	if err := os.MkdirAll(podDataDir, dirPerms); err != nil {
		step.Completef(result.StepFailed, msgErrCreatePodDataDir, err)

		return false
	}

	cliBin, err := findCLI()
	if err != nil {
		step.Completef(result.StepFailed, msgErrCLIPath, err)

		return false
	}

	if _, err := exec.LookPath("tar"); err != nil {
		step.Completef(result.StepFailed, msgErrTarPath, err)

		return false
	}

	exists, err := checkPodDataDir(ctx, cliBin, ns, podName)
	if err != nil {
		step.Completef(result.StepFailed, msgErrDirCheck, podName, err)

		return false
	}

	if !exists {
		step.Completef(result.StepSkipped, msgInfoPodDataDirMissing, podName)

		return true
	}

	var execStderr, tarStderr bytes.Buffer

	//nolint:gosec // cliBin is resolved from LookPath, not user input
	cmdExec := exec.CommandContext(ctx, cliBin, "exec", "-n", ns, podName, "-c", "llama-stack", "--", "tar", "--warning=no-file-changed", "--ignore-failed-read", "-czf", "-", "-C", podDataPath, ".")
	cmdExec.Stderr = &execStderr

	//nolint:gosec // controlled inputs for podDataDir
	cmdTar := exec.CommandContext(ctx, "tar", "-xzf", "-", "-C", podDataDir)
	cmdTar.Stderr = &tarStderr

	stdout, err := cmdExec.StdoutPipe()
	if err != nil {
		step.Completef(result.StepFailed, msgErrStdoutPipe, err)

		return false
	}

	if err := cmdExec.Start(); err != nil {
		step.Completef(result.StepFailed, msgErrExecRun, podName, err, execStderr.String())

		return false
	}

	cmdTar.Stdin = stdout
	if err := cmdTar.Start(); err != nil {
		// Prevent leaving cmdExec as a zombie process when tar fails to start.
		_ = cmdExec.Process.Kill()
		_ = cmdExec.Wait()

		step.Completef(result.StepFailed, msgErrTarStart, err)

		return false
	}

	// Wait for the reader to finish reading from the pipe before waiting for the writer.
	// This prevents cmdExec.Wait() from closing the pipe while tar is still reading.
	tarErr := cmdTar.Wait()
	execErr := cmdExec.Wait()

	if execErr != nil {
		step.Completef(result.StepFailed, msgErrExecRun, podName, execErr, execStderr.String())

		return false
	}

	if tarErr != nil {
		step.Completef(result.StepFailed, msgErrTarWait, tarErr, tarStderr.String())

		return false
	}

	if err := enforcePermissions(podDataDir); err != nil {
		step.Completef(result.StepFailed, msgErrEnforcePerms, err)

		return false
	}

	return true
}

// enforcePermissions walks a directory tree and sets restrictive permissions
// on all files and directories. Symlinks are skipped to prevent os.Chmod from
// following them and modifying files outside the tree (CWE-59).
func enforcePermissions(dir string) error {
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks to prevent os.Chmod from following them and modifying
		// files outside the directory tree (CWE-59).
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			return os.Chmod(path, dirPerms)
		}

		return os.Chmod(path, filePerms)
	}); err != nil {
		return fmt.Errorf("walking directory %s: %w", dir, err)
	}

	return nil
}

// selectReadyPod returns the first Running+Ready pod from the list.
// Falls back to the first pod if none are Ready.
func selectReadyPod(pods []unstructured.Unstructured) unstructured.Unstructured {
	for i := range pods {
		phase, _ := jq.Query[string](&pods[i], ".status.phase")
		if phase != "Running" {
			continue
		}

		conditions, err := jq.Query[[]any](&pods[i], ".status.conditions")
		if err != nil {
			continue
		}

		for _, c := range conditions {
			cond, ok := c.(map[string]any)
			if !ok {
				continue
			}

			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)

			if condType == "Ready" && condStatus == "True" {
				return pods[i]
			}
		}
	}

	return pods[0]
}

// findCLI returns "oc" or "kubectl", preferring "oc".
func findCLI() (string, error) {
	if _, err := exec.LookPath("oc"); err == nil {
		return "oc", nil
	}

	if _, err := exec.LookPath("kubectl"); err != nil {
		return "", fmt.Errorf("looking up kubectl: %w", err)
	}

	return "kubectl", nil
}

// checkPodDataDir checks whether podDataPath exists inside a pod.
// Returns (true, nil) if the directory exists, (false, nil) if it does not,
// and (false, err) if the exec command itself failed.
func checkPodDataDir(ctx context.Context, cliBin, ns, podName string) (bool, error) {
	cmd := exec.CommandContext(ctx, cliBin, "exec", "-n", ns, podName, "-c", "llama-stack", "--", "test", "-d", podDataPath)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}

		return false, fmt.Errorf("exec in pod %s: %w", podName, err)
	}

	return true, nil
}
