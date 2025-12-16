package migrate

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/cmd"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*RunCommand)(nil)

type RunCommand struct {
	*SharedOptions

	DryRun        bool
	Prepare       bool
	Yes           bool
	BackupPath    string
	MigrationIDs  []string
	TargetVersion string

	parsedTargetVersion *semver.Version
}

func NewRunCommand(streams genericiooptions.IOStreams) *RunCommand {
	shared := NewSharedOptions(streams)

	return &RunCommand{
		SharedOptions: shared,
		BackupPath:    "./backups",
	}
}

func (c *RunCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescRunVerbose)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescRunTimeout)
	fs.BoolVar(&c.DryRun, "dry-run", false, flagDescRunDryRun)
	fs.BoolVar(&c.Prepare, "prepare", false, flagDescRunPrepare)
	fs.BoolVarP(&c.Yes, "yes", "y", false, flagDescRunYes)
	fs.StringVar(&c.BackupPath, "backup-path", c.BackupPath, flagDescRunBackupPath)
	fs.StringArrayVarP(&c.MigrationIDs, "migration", "m", []string{}, flagDescRunMigration)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescRunTargetVersion)
}

func (c *RunCommand) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Always enable verbose for migrate run (both dry-run and actual execution)
	c.Verbose = true

	if c.TargetVersion != "" {
		targetVer, err := semver.Parse(c.TargetVersion)
		if err != nil {
			return fmt.Errorf("invalid target version %q: %w", c.TargetVersion, err)
		}
		c.parsedTargetVersion = &targetVer
	}

	return nil
}

func (c *RunCommand) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	if len(c.MigrationIDs) == 0 {
		return errors.New("--migration flag is required")
	}

	if c.TargetVersion == "" {
		return errors.New("--target-version flag is required")
	}

	return nil
}

func (c *RunCommand) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	currentVersion, err := version.Detect(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("detecting cluster version: %w", err)
	}

	targetVersionInfo := &version.ClusterVersion{
		Version:    c.TargetVersion,
		Source:     version.SourceManual,
		Confidence: version.ConfidenceHigh,
	}

	registry := action.GetGlobalRegistry()

	if c.Prepare {
		return c.runPrepareMode(ctx, currentVersion, targetVersionInfo, registry)
	}

	return c.runMigrationMode(ctx, currentVersion, targetVersionInfo, registry)
}

func (c *RunCommand) runPrepareMode(
	ctx context.Context,
	currentVersion *version.ClusterVersion,
	targetVersionInfo *version.ClusterVersion,
	registry *action.ActionRegistry,
) error {
	for _, migrationID := range c.MigrationIDs {
		c.IO.Errorf("Running pre-flight checks for migration: %s\n", migrationID)

		selectedAction, ok := registry.Get(migrationID)
		if !ok {
			return fmt.Errorf("migration %q not found", migrationID)
		}

		// Use verbose recorder for real-time streaming output
		recorder := action.NewVerboseRootRecorder(c.IO)
		c.IO.Errorf("\n%s:\n", migrationID)

		target := &action.ActionTarget{
			Client:         c.Client,
			CurrentVersion: currentVersion,
			TargetVersion:  targetVersionInfo,
			DryRun:         c.DryRun,
			BackupPath:     c.BackupPath,
			SkipConfirm:    c.Yes,
			Recorder:       recorder,
			IO:             c.IO,
		}

		_, err := selectedAction.Validate(ctx, target)
		if err != nil {
			return fmt.Errorf("pre-flight validation failed: %w", err)
		}

		// Output has already been streamed during validation
		c.IO.Fprintln()
	}

	c.IO.Errorf("Preparation complete. Run without --prepare to execute migration.")

	return nil
}

func (c *RunCommand) runMigrationMode(
	ctx context.Context,
	currentVersion *version.ClusterVersion,
	targetVersionInfo *version.ClusterVersion,
	registry *action.ActionRegistry,
) error {
	c.IO.Errorf("Current OpenShift AI version: %s", currentVersion.Version)
	c.IO.Errorf("Target OpenShift AI version: %s\n", targetVersionInfo.Version)

	for idx, migrationID := range c.MigrationIDs {
		if len(c.MigrationIDs) > 1 {
			c.IO.Errorf("\n=== Migration %d/%d: %s ===\n", idx+1, len(c.MigrationIDs), migrationID)
		}

		selectedAction, ok := registry.Get(migrationID)
		if !ok {
			return fmt.Errorf("migration %q not found", migrationID)
		}

		// Use verbose recorder for real-time streaming output
		recorder := action.NewVerboseRootRecorder(c.IO)
		c.IO.Errorf("\n%s:\n", migrationID)

		target := &action.ActionTarget{
			Client:         c.Client,
			CurrentVersion: currentVersion,
			TargetVersion:  targetVersionInfo,
			DryRun:         c.DryRun,
			BackupPath:     c.BackupPath,
			SkipConfirm:    c.Yes,
			Recorder:       recorder,
			IO:             c.IO,
		}

		if c.DryRun {
			c.IO.Errorf("DRY RUN MODE: No changes will be made to the cluster\n")
		} else if c.Yes {
			c.IO.Errorf("Running migration: %s (confirmations skipped)\n", migrationID)
		} else {
			c.IO.Errorf("Preparing migration: %s\n", migrationID)
		}

		actionResult, err := selectedAction.Execute(ctx, target)
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		// Output has already been streamed during execution, no need to render again
		c.IO.Fprintln()
		if !actionResult.Status.Completed {
			c.IO.Errorf("Migration %s incomplete - please review the output above", migrationID)

			return fmt.Errorf("migration halted: %s", migrationID)
		}
		c.IO.Errorf("Migration %s completed successfully!", migrationID)
	}

	c.IO.Fprintln()
	c.IO.Errorf("All migrations completed successfully!")

	return nil
}
