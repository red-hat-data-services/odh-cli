package cleanup

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

type runTask struct {
	action *CleanupOAuthAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.action.Scope.Validate(); err != nil {
		return nil, fmt.Errorf("invalid flags: %w", err)
	}

	step := target.Recorder.Child(
		"validate-cleanup-readiness",
		"Validate notebooks are ready for OAuth cleanup",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found, nothing to clean up")

		return action.BuildResult(target)
	}

	passCount := 0
	failCount := 0

	for _, nb := range notebooks {
		passed, failures := checkMigrationState(nb)
		if passed {
			passCount++
		} else {
			failCount++
			step.Recordf(
				fmt.Sprintf("precheck-%s-%s", nb.GetNamespace(), nb.GetName()),
				"Pre-check failed for %s/%s: %s",
				result.StepFailed,
				nb.GetNamespace(), nb.GetName(), joinFailures(failures))
		}
	}

	if failCount > 0 {
		step.Completef(result.StepFailed,
			"%d/%d Notebook(s) failed migration pre-checks; resolve before cleanup",
			failCount, len(notebooks))
	} else {
		step.Completef(result.StepCompleted,
			"All %d Notebook(s) passed migration pre-checks and are ready for cleanup",
			passCount)
	}

	return action.BuildResult(target)
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.action.Scope.Validate(); err != nil {
		return nil, fmt.Errorf("invalid flags: %w", err)
	}

	discoverStep := target.Recorder.Child(
		"discover-notebooks",
		"Discover notebooks for OAuth cleanup",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		discoverStep.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(notebooks) == 0 {
		discoverStep.Completef(result.StepCompleted,
			"No Notebook instances found, nothing to clean up")

		return action.BuildResult(target)
	}

	discoverStep.Completef(result.StepCompleted,
		"Found %d Notebook(s) for cleanup", len(notebooks))

	if !target.DryRun && !t.action.promptBeforeCleanup(target, len(notebooks)) {
		target.Recorder.Recordf("cleanup-cancelled",
			"User cancelled cleanup", result.StepSkipped)

		return action.BuildResult(target)
	}

	cleanedCount := 0
	skippedCount := 0
	failedCount := 0

	for _, nb := range notebooks {
		r := t.cleanupWorkbench(ctx, target, nb)

		switch r {
		case cleanupResultCleaned:
			cleanedCount++
		case cleanupResultSkipped:
			skippedCount++
		case cleanupResultFailed:
			failedCount++
		}
	}

	summaryStep := target.Recorder.Child("cleanup-summary", "Cleanup summary")

	if failedCount > 0 {
		summaryStep.Completef(result.StepFailed,
			"Cleaned %d, skipped %d, failed %d out of %d Notebook(s)",
			cleanedCount, skippedCount, failedCount, len(notebooks))
	} else if target.DryRun {
		summaryStep.Completef(result.StepSkipped,
			"Would clean up OAuth resources for %d Notebook(s)", len(notebooks)-skippedCount)
	} else {
		summaryStep.Completef(result.StepCompleted,
			"Cleaned %d, skipped %d out of %d Notebook(s)",
			cleanedCount, skippedCount, len(notebooks))
	}

	return action.BuildResult(target)
}

type cleanupResult int

const (
	cleanupResultCleaned cleanupResult = iota
	cleanupResultSkipped
	cleanupResultFailed
)

func (t *runTask) cleanupWorkbench(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
) cleanupResult {
	name := nb.GetName()
	namespace := nb.GetNamespace()

	step := target.Recorder.Child(
		fmt.Sprintf("cleanup-%s-%s", namespace, name),
		fmt.Sprintf("Clean up OAuth resources for %s/%s", namespace, name),
	)

	passed, failures := checkMigrationState(nb)
	if !passed {
		step.Recordf("precheck",
			"Pre-check failed: %s",
			result.StepFailed,
			joinFailures(failures))

		if !t.action.promptCleanupContinueOrSkip(target, name, namespace, failures) {
			step.Completef(result.StepSkipped,
				"Skipped cleanup for %s/%s (pre-check failed)", namespace, name)

			return cleanupResultSkipped
		}

		step.Recordf("precheck-override",
			"Continuing cleanup despite failed pre-checks", result.StepCompleted)
	}

	hasFailed := !deleteResourceIfPresent(ctx, target,
		resources.Route.GVR(), name, namespace, step)

	if !deleteResourceIfPresent(ctx, target,
		resources.Service.GVR(), name+"-tls", namespace, step) {
		hasFailed = true
	}

	if !deleteResourceIfPresent(ctx, target,
		resources.Secret.GVR(), name+"-oauth-client", namespace, step) {
		hasFailed = true
	}

	if !deleteResourceIfPresent(ctx, target,
		resources.Secret.GVR(), name+"-oauth-config", namespace, step) {
		hasFailed = true
	}

	if !deleteResourceIfPresent(ctx, target,
		resources.Secret.GVR(), name+"-tls", namespace, step) {
		hasFailed = true
	}

	oauthClientName := fmt.Sprintf("%s-%s-oauth-client", name, namespace)
	if !deleteResourceIfPresent(ctx, target,
		resources.OAuthClient.GVR(), oauthClientName, "", step) {
		hasFailed = true
	}

	if hasFailed {
		step.Completef(result.StepFailed,
			"Cleanup partially failed for %s/%s", namespace, name)

		return cleanupResultFailed
	}

	if target.DryRun {
		step.Completef(result.StepSkipped,
			"Would clean up OAuth resources for %s/%s", namespace, name)
	} else {
		step.Completef(result.StepCompleted,
			"Cleaned up OAuth resources for %s/%s", namespace, name)
	}

	return cleanupResultCleaned
}

func joinFailures(failures []string) string {
	return strings.Join(failures, "; ")
}
