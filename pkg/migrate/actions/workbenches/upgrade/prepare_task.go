package upgrade

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
)

type prepareTask struct {
	action *WorkbenchUpgradeAction
}

func (t *prepareTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child(
		"validate-notebooks",
		"Check for Notebook resources",
	)

	notebooks, err := t.action.listNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)
	} else if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found")
	} else {
		step.Completef(result.StepCompleted, "Found %d Notebook(s) across the cluster", len(notebooks))
	}

	return buildResult(target.Recorder)
}

func (t *prepareTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child(
		"prepare-backup",
		"Prepare workbench backup",
	)

	notebooks, err := t.action.listNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return buildResult(target.Recorder)
	}

	if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found, nothing to back up")

		return buildResult(target.Recorder)
	}

	if err := t.action.backupNotebooks(ctx, target, notebooks, step); err != nil {
		r, buildErr := buildResult(target.Recorder)
		if buildErr != nil {
			return nil, buildErr
		}

		return r, fmt.Errorf("prepare backup failed: %w", err)
	}

	step.Completef(result.StepCompleted, "Backup phase complete")

	return buildResult(target.Recorder)
}
