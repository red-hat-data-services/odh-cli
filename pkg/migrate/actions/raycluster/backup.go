package raycluster

import (
	"context"
	"errors"

	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	rcpkg "github.com/opendatahub-io/odh-cli/pkg/migrate/raycluster"
)

const (
	backupActionID          = "raycluster.backup"
	backupActionName        = "Backup RayClusters for RHOAI 3.x migration"
	backupActionDescription = "Backup RayCluster configurations and run pre-flight checks before RHOAI upgrade"
)

type BackupAction struct {
	opts *sharedOptions
}

func (a *BackupAction) ID() string                { return backupActionID }
func (a *BackupAction) Name() string              { return backupActionName }
func (a *BackupAction) Description() string       { return backupActionDescription }
func (a *BackupAction) Group() action.ActionGroup { return action.GroupBackup }
func (a *BackupAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *BackupAction) CanApply(target action.Target) bool {
	if target.CurrentVersion == nil {
		return false
	}

	return target.CurrentVersion.Major == 2 //nolint:mnd // RHOAI 2.x version check
}

func (a *BackupAction) AddFlags(fs *pflag.FlagSet) {
	addBackupFlags(a.opts, fs)
}

func (a *BackupAction) Prepare() action.Task {
	return &backupPrepareTask{opts: a.opts}
}

func (a *BackupAction) Run() action.Task {
	return &backupRunTask{opts: a.opts}
}

// backupPrepareTask runs preflight checks without performing the actual backup.
type backupPrepareTask struct {
	opts *sharedOptions
}

func (t *backupPrepareTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	t.runPreflightChecks(ctx, target)

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (t *backupPrepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	t.runPreflightChecks(ctx, target)

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (t *backupPrepareTask) runPreflightChecks(ctx context.Context, target action.Target) {
	step := target.Recorder.Child("preflight-checks", "Run RayCluster pre-upgrade checks")

	checks := rcpkg.RunPreUpgradeChecks(ctx, target.Client)

	allPassed := true
	hasRequiredFailure := false

	for _, chk := range checks {
		status := result.StepCompleted
		if !chk.Passed {
			allPassed = false
			if chk.Required {
				status = result.StepFailed
				hasRequiredFailure = true
			} else {
				status = result.StepSkipped
			}
		}

		step.Recordf(chk.Name, chk.Message, status)
	}

	if hasRequiredFailure {
		step.Completef(result.StepFailed, "Pre-upgrade checks failed — resolve issues before proceeding")
	} else if allPassed {
		step.Completef(result.StepCompleted, "All pre-upgrade checks passed")
	} else {
		step.Completef(result.StepCompleted, "Pre-upgrade checks completed with warnings")
	}
}

// backupRunTask executes the full backup (preflight checks + backup to disk).
type backupRunTask struct {
	opts *sharedOptions
}

func (t *backupRunTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	prep := &backupPrepareTask{opts: t.opts}

	return prep.Validate(ctx, target)
}

func (t *backupRunTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	outputDir := t.opts.OutputDir
	if target.OutputDir != "" {
		outputDir = target.OutputDir
	}

	step := target.Recorder.Child("backup-rayclusters", "Backup RayCluster configurations")

	checks := rcpkg.RunPreUpgradeChecks(ctx, target.Client)

	if target.DryRun {
		clusters, err := rcpkg.GetClusters(ctx, target.Client, t.opts.ClusterName, t.opts.Namespace)
		if err != nil {
			step.Completef(result.StepFailed, "Failed to list RayClusters: %v", err)
		} else if len(clusters) == 0 {
			step.Completef(result.StepSkipped, "Dry-run: no RayClusters found to backup")
		} else {
			step.Completef(result.StepSkipped, "Dry-run: would backup %d RayCluster(s) to %s", len(clusters), outputDir)
		}

		rootRecorder, ok := target.Recorder.(action.RootRecorder)
		if !ok {
			return nil, errors.New("recorder is not a RootRecorder")
		}

		return rootRecorder.Build(), nil
	}

	saved, err := rcpkg.PreUpgrade(ctx, target.Client, outputDir, t.opts.ClusterName, t.opts.Namespace, checks, target.IO)
	if err != nil {
		step.Completef(result.StepFailed, "Pre-upgrade backup failed: %v", err)

		rootRecorder, ok := target.Recorder.(action.RootRecorder)
		if !ok {
			return nil, errors.New("recorder is not a RootRecorder")
		}

		return rootRecorder.Build(), nil
	}

	if len(saved) == 0 {
		step.Completef(result.StepSkipped, "No RayClusters found to backup")
	} else {
		step.Completef(result.StepCompleted, "Backed up %d RayCluster(s) to %s", len(saved), outputDir)
	}

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}
