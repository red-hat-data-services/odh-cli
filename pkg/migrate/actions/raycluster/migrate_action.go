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
	migrateActionID          = "raycluster.migrate"
	migrateActionName        = "Migrate RayClusters to RHOAI 3.x"
	migrateActionDescription = "Migrate RayClusters after RHOAI upgrade (live in-place or from backup)"
)

type MigrateAction struct {
	opts *sharedOptions
}

func (a *MigrateAction) ID() string                { return migrateActionID }
func (a *MigrateAction) Name() string              { return migrateActionName }
func (a *MigrateAction) Description() string       { return migrateActionDescription }
func (a *MigrateAction) Group() action.ActionGroup { return action.GroupMigration }
func (a *MigrateAction) Phase() action.ActionPhase { return action.PhasePostUpgrade }

func (a *MigrateAction) CanApply(target action.Target) bool {
	if target.TargetVersion == nil {
		return false
	}

	return target.TargetVersion.Major >= 3 //nolint:mnd // RHOAI 3.x version check
}

func (a *MigrateAction) AddFlags(fs *pflag.FlagSet) {
	addMigrateFlags(a.opts, fs)
}

func (a *MigrateAction) Prepare() action.Task {
	return nil
}

func (a *MigrateAction) Run() action.Task {
	return &migrateRunTask{opts: a.opts}
}

type migrateRunTask struct {
	opts *sharedOptions
}

func (t *migrateRunTask) Validate(_ context.Context, _ action.Target) (*result.ActionResult, error) {
	return nil, nil
}

func (t *migrateRunTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("post-upgrade-migration", "Migrate RayClusters to RHOAI 3.x")

	popts := rcpkg.PostUpgradeOptions{
		ClusterName:  t.opts.ClusterName,
		Namespace:    t.opts.Namespace,
		DryRun:       target.DryRun,
		SkipConfirm:  target.SkipConfirm,
		FromBackup:   t.opts.FromBackup,
		RouteTimeout: t.opts.RouteTimeout,
	}

	res, err := rcpkg.PostUpgrade(ctx, target.Client, popts, target.IO)
	if err != nil {
		step.Completef(result.StepFailed, "Post-upgrade migration failed: %v", err)

		rootRecorder, ok := target.Recorder.(action.RootRecorder)
		if !ok {
			return nil, errors.New("recorder is not a RootRecorder")
		}

		return rootRecorder.Build(), nil
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Dry-run: %d would be migrated, %d skipped", res.Migrated, res.Skipped)
	} else {
		step.Completef(result.StepCompleted, "Migrated: %d, Skipped: %d, Failed: %d", res.Migrated, res.Skipped, res.Failed)
	}

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}
