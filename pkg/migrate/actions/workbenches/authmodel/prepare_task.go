package authmodel

import (
	"context"
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

type prepareTask struct {
	action *PatchAuthModelAction
}

func (t *prepareTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.action.Scope.Validate(); err != nil {
		return nil, fmt.Errorf("invalid flags: %w", err)
	}

	step := target.Recorder.Child(
		"validate-patch-readiness",
		"Validate notebooks for auth model patch",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found")

		return action.BuildResult(target)
	}

	needsPatch := 0
	alreadyPatched := 0

	for _, nb := range notebooks {
		if needsAuthModelPatch(nb) {
			needsPatch++
		} else {
			alreadyPatched++
		}
	}

	if needsPatch == 0 {
		step.Completef(result.StepCompleted,
			"All %d Notebook(s) already patched for 3.x auth model", alreadyPatched)
	} else {
		step.Completef(result.StepCompleted,
			"Found %d Notebook(s) needing auth model patch, %d already patched",
			needsPatch, alreadyPatched)
	}

	return action.BuildResult(target)
}

func (t *prepareTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.action.Scope.Validate(); err != nil {
		return nil, fmt.Errorf("invalid flags: %w", err)
	}

	step := target.Recorder.Child(
		"backup-notebooks",
		"Backup Notebook resources before auth model patch",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found, nothing to backup")

		return action.BuildResult(target)
	}

	if err := backupNotebooks(target, notebooks, step); err != nil {
		return nil, fmt.Errorf("notebook backup failed: %w", err)
	}

	return action.BuildResult(target)
}

func backupNotebooks(
	target action.Target,
	notebooks []*unstructured.Unstructured,
	step action.StepRecorder,
) error {
	if target.DryRun {
		step.Completef(result.StepSkipped,
			"Would backup %d Notebook(s) to %s", len(notebooks), target.OutputDir)

		return nil
	}

	byNamespace := make(map[string][]*unstructured.Unstructured)
	for _, nb := range notebooks {
		ns := nb.GetNamespace()
		byNamespace[ns] = append(byNamespace[ns], nb)
	}

	totalBacked := 0

	for ns, nbs := range byNamespace {
		outputDir := filepath.Join(target.OutputDir, ns)

		if err := backup.WriteResourcesToDir(outputDir, resources.Notebook.GVR(), nbs); err != nil {
			step.Completef(result.StepFailed,
				"Failed to write Notebooks in namespace %s: %v", ns, err)

			return fmt.Errorf("backup failed for namespace %s: %w", ns, err)
		}

		totalBacked += len(nbs)
	}

	step.Completef(result.StepCompleted,
		"Backed up %d Notebook(s) across %d namespace(s) to %s",
		totalBacked, len(byNamespace), target.OutputDir)

	return nil
}
