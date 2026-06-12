package data

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	podexec "github.com/opendatahub-io/odh-cli/pkg/util/exec"
)

const backupTimestampFmt = "20060102-150405"

type prepareTask struct {
	action *DataAction
}

func (t *prepareTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-trustyai-data", "Check TrustyAI data services")

	services, err := discoverTrustyAIServices(ctx, target, t.action.ServiceName)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover TrustyAI services: %v", err)

		return action.BuildResult(target)
	}

	if len(services) == 0 {
		step.Completef(result.StepSkipped, "No TrustyAIService CRs found")
	} else {
		step.Completef(result.StepCompleted, "Found %d TrustyAIService(s)", len(services))
	}

	return action.BuildResult(target)
}

func (t *prepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("backup-trustyai-data", "Backup TrustyAI data storage")

	services, err := discoverTrustyAIServices(ctx, target, t.action.ServiceName)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover TrustyAI services: %v", err)

		return action.BuildResult(target)
	}

	if len(services) == 0 {
		step.Completef(result.StepSkipped, "No TrustyAIService CRs found")

		return action.BuildResult(target)
	}

	var executor podexec.Executor
	if target.RESTConfig != nil {
		executor = podexec.NewSPDYExecutor(target.RESTConfig, target.Client.CoreV1())
	}

	outputDir := t.action.BackupDir
	if target.OutputDir != "" {
		outputDir = filepath.Join(target.OutputDir, "trustyai-data")
	}

	for _, svc := range services {
		svcStep := step.Child(
			fmt.Sprintf("backup-%s-%s", svc.namespace, svc.name),
			fmt.Sprintf("Backup %s/%s (%s)", svc.namespace, svc.name, svc.storageFormat),
		)

		switch svc.storageFormat {
		case storageFormatPVC:
			t.backupPVC(ctx, target, executor, svc, outputDir, svcStep)
		case storageFormatDatabase:
			t.backupDatabase(ctx, target, executor, svc, outputDir, svcStep)
		default:
			svcStep.Completef(result.StepSkipped, "Unsupported storage format %q", svc.storageFormat)
		}
	}

	step.Completef(result.StepCompleted, "Data backup complete")

	return action.BuildResult(target)
}

func (t *prepareTask) backupPVC(
	ctx context.Context,
	target action.Target,
	executor podexec.Executor,
	svc serviceInfo,
	outputDir string,
	step action.StepRecorder,
) {
	pod, err := findServicePod(ctx, target, svc.namespace, svc.name)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find TrustyAI pod: %v", err)

		return
	}

	mount, err := findMountPath(pod, svc.cr, svc.name)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find mount path: %v", err)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup PVC data from %s:%s", pod.Name, mount.mountPath)

		return
	}

	if executor == nil {
		step.Completef(result.StepFailed, "No SPDY executor available (RESTConfig not set)")

		return
	}

	timestamp := time.Now().Format(backupTimestampFmt)
	backupDirName := fmt.Sprintf("trustyai-data-%s-%s", svc.namespace, timestamp)
	backupPath := filepath.Join(outputDir, backupDirName)
	dataPath := filepath.Join(backupPath, dataSubDir)

	if err := os.MkdirAll(dataPath, backupDirPermission); err != nil {
		step.Completef(result.StepFailed, "Failed to create backup directory: %v", err)

		return
	}

	if err := podexec.CopyFromPod(ctx, executor, podexec.CopyOptions{
		Namespace:     svc.namespace,
		PodName:       pod.Name,
		ContainerName: trustyaiContainer,
		PodPath:       mount.mountPath,
		LocalPath:     dataPath,
	}); err != nil {
		step.Completef(result.StepFailed, "Failed to copy data from pod: %v", err)

		return
	}

	fileCount, _ := countFiles(dataPath)

	metadata := DataBackupMetadata{
		Timestamp:       timestamp,
		Namespace:       svc.namespace,
		TrustyAIService: svc.name,
		StorageFormat:   storageFormatPVC,
		PVCName:         mount.pvcName,
		MountPath:       mount.mountPath,
		SourcePod:       pod.Name,
		FileCount:       fileCount,
	}

	if err := writeMetadata(backupPath, metadata); err != nil {
		step.Completef(result.StepFailed, "Failed to write backup metadata: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Backed up %d file(s) to %s", fileCount, backupPath)
}

func (t *prepareTask) backupDatabase(
	ctx context.Context,
	target action.Target,
	executor podexec.Executor,
	svc serviceInfo,
	outputDir string,
	step action.StepRecorder,
) {
	creds, err := findDBCredentials(ctx, target, svc.namespace, svc.cr, svc.name)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find DB credentials: %v", err)

		return
	}

	secretObj, err := target.Client.GetResource(ctx, resources.Secret, creds.secretName,
		client.InNamespace(svc.namespace))
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get secret: %v", err)

		return
	}

	secretData := extractSecretData(secretObj)

	mariadbPod, err := findMariaDBPod(ctx, target, svc.namespace, svc.name, secretData)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find MariaDB pod: %v", err)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would dump database %s from pod %s", creds.database, mariadbPod.Name)

		return
	}

	if executor == nil {
		step.Completef(result.StepFailed, "No SPDY executor available (RESTConfig not set)")

		return
	}

	dumpCmd, err := detectDumpCommand(ctx, executor, svc.namespace, mariadbPod)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to detect dump command: %v", err)

		return
	}

	timestamp := time.Now().Format(backupTimestampFmt)
	backupDirName := fmt.Sprintf("trustyai-db-%s-%s", svc.namespace, timestamp)
	backupPath := filepath.Join(outputDir, backupDirName)

	if err := os.MkdirAll(backupPath, backupDirPermission); err != nil {
		step.Completef(result.StepFailed, "Failed to create backup directory: %v", err)

		return
	}

	dumpPath := filepath.Join(backupPath, dumpFileName)

	dumpFile, err := os.Create(dumpPath) //nolint:gosec // Path built from controlled backupDir + timestamp.
	if err != nil {
		step.Completef(result.StepFailed, "Failed to create dump file: %v", err)

		return
	}

	var stderrBuf bytes.Buffer

	execErr := executor.Exec(ctx, podexec.ExecOptions{
		Namespace:     svc.namespace,
		PodName:       mariadbPod.Name,
		ContainerName: mariadbContainer,
		Command: []string{
			dumpCmd, "--skip-lock-tables",
			"-u" + creds.username,
			"-p" + creds.password,
			creds.database,
		},
		Stdout: dumpFile,
		Stderr: &stderrBuf,
	})

	if closeErr := dumpFile.Close(); closeErr != nil {
		step.Completef(result.StepFailed, "Failed to close dump file: %v", closeErr)

		return
	}

	if execErr != nil {
		fi, statErr := os.Stat(dumpPath)
		if statErr != nil || fi.Size() == 0 {
			_ = os.RemoveAll(backupPath)
			step.Completef(result.StepFailed, "Database dump failed: %v", execErr)

			return
		}
	}

	dumpLines, _ := countLines(dumpPath)

	if dumpLines <= 1 {
		_ = os.RemoveAll(backupPath)
		step.Completef(result.StepFailed, "Database dump appears empty")

		return
	}

	metadata := DataBackupMetadata{
		Timestamp:         timestamp,
		Namespace:         svc.namespace,
		TrustyAIService:   svc.name,
		StorageFormat:     storageFormatDatabase,
		MariaDBPod:        mariadbPod.Name,
		CredentialsSecret: creds.secretName,
		DatabaseName:      creds.database,
		DatabaseUser:      creds.username,
		DumpCommand:       dumpCmd,
		DumpLines:         dumpLines,
	}

	if err := writeMetadata(backupPath, metadata); err != nil {
		step.Completef(result.StepFailed, "Failed to write backup metadata: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Dumped %d lines from database %s to %s", dumpLines, creds.database, backupPath)
}
