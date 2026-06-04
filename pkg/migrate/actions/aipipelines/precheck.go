package aipipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

// PreUpgradeCheckAction captures DSPA pod health, migrates v1alpha1→v1, and detects RBAC gaps.
type PreUpgradeCheckAction struct{}

func (a *PreUpgradeCheckAction) ID() string          { return preUpgradeCheckID }
func (a *PreUpgradeCheckAction) Name() string        { return preUpgradeCheckName }
func (a *PreUpgradeCheckAction) Description() string { return preUpgradeCheckDescription }

func (a *PreUpgradeCheckAction) Group() action.ActionGroup { return action.GroupMigration }
func (a *PreUpgradeCheckAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *PreUpgradeCheckAction) CanApply(target action.Target) bool {
	return target.CurrentVersion != nil && target.CurrentVersion.Major == 2
}

func (a *PreUpgradeCheckAction) Prepare() action.Task { return &preUpgradeCheckPrepareTask{} }
func (a *PreUpgradeCheckAction) Run() action.Task     { return &preUpgradeCheckRunTask{} }

// --- Prepare task: capture pod health state, detect v1alpha1, detect RBAC gaps ---

type preUpgradeCheckPrepareTask struct{}

func (t *preUpgradeCheckPrepareTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	validateTarget := target
	validateTarget.DryRun = true

	return t.Execute(ctx, validateTarget)
}

func (t *preUpgradeCheckPrepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	t.discover(ctx, target, recorder)

	return recorder.Build(), nil
}

func (t *preUpgradeCheckPrepareTask) discover(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) {
	// Step 1: Capture pod health state for v1 DSPAs
	healthStep := recorder.Child("capture-pod-health", "Capture pre-upgrade DSPA pod health")

	v1DSPAs, err := listDSPAs(ctx, target.Client, resources.DataSciencePipelinesApplicationV1)
	if err != nil {
		healthStep.Complete(result.StepFailed, "Failed to list v1 DSPAs: %v", err)

		return
	}

	var state PodHealthState

	if len(v1DSPAs) == 0 {
		healthStep.Complete(result.StepCompleted, "No v1 DSPAs found")

		state = PodHealthState{
			CapturedAt: time.Now().UTC().Format(time.RFC3339),
			DSPAs:      []DSPAState{},
		}
	} else {
		var captureErr error

		state, captureErr = capturePodHealth(ctx, target.Client, healthStep, v1DSPAs)
		if captureErr != nil {
			healthStep.Complete(result.StepFailed, "Failed to capture pod health: %v", captureErr)

			return
		}

		healthStep.Complete(result.StepCompleted, "Captured pod health for %d DSPA(s)", len(v1DSPAs))
	}

	if !target.DryRun {
		t.saveState(target, recorder, state)
	}

	// Step 2: Detect v1alpha1 DSPAs
	v1alpha1Step := recorder.Child("detect-v1alpha1", "Detect deprecated v1alpha1 DSPAs")

	v1alpha1DSPAs, err := listDSPAs(ctx, target.Client, resources.DataSciencePipelinesApplicationV1Alpha1)
	if err != nil {
		v1alpha1Step.Complete(result.StepFailed, "Failed to list v1alpha1 DSPAs: %v", err)

		return
	}

	if len(v1alpha1DSPAs) == 0 {
		v1alpha1Step.Complete(result.StepCompleted, "No deprecated v1alpha1 DSPAs found")
	} else {
		for _, dspa := range v1alpha1DSPAs {
			v1alpha1Step.Record(dspa.Name, "v1alpha1 DSPA %s/%s needs migration", result.StepCompleted, dspa.Namespace, dspa.Name)
		}

		v1alpha1Step.Complete(result.StepCompleted, "Found %d v1alpha1 DSPA(s) requiring migration", len(v1alpha1DSPAs))
	}

	// Step 3: Detect custom roles needing RBAC update
	_, _ = findRolesNeedingFix(ctx, target.Client, recorder)
}

func (t *preUpgradeCheckPrepareTask) saveState(
	target action.Target,
	recorder action.StepRecorder,
	state PodHealthState,
) {
	saveStep := recorder.Child("save-state", "Save pre-upgrade state")

	if target.DryRun {
		saveStep.Complete(result.StepSkipped, "Would save pre-upgrade pod health state")

		return
	}

	statePath := defaultStatePath()

	if err := savePodHealthState(state, statePath); err != nil {
		saveStep.Complete(result.StepFailed, "Failed to save state: %v", err)

		return
	}

	saveStep.Complete(result.StepCompleted, "State saved to %s", statePath)
}

// --- Run task: migrate v1alpha1 DSPAs to v1 ---

type preUpgradeCheckRunTask struct{}

func (t *preUpgradeCheckRunTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	step := recorder.Child("check-v1alpha1", "Check for v1alpha1 DSPAs")

	v1alpha1DSPAs, err := listDSPAs(ctx, target.Client, resources.DataSciencePipelinesApplicationV1Alpha1)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to list v1alpha1 DSPAs: %v", err)

		return recorder.Build(), nil
	}

	if len(v1alpha1DSPAs) == 0 {
		step.Complete(result.StepCompleted, "No v1alpha1 DSPAs found — nothing to migrate")
	} else {
		step.Complete(result.StepCompleted, "Found %d v1alpha1 DSPA(s) to migrate", len(v1alpha1DSPAs))
	}

	return recorder.Build(), nil
}

func (t *preUpgradeCheckRunTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	if err := migrateAllDSPAsToV1(ctx, target.Client, recorder, migrateOpts{DryRun: target.DryRun}); err != nil {
		return recorder.Build(), fmt.Errorf("v1alpha1 migration failed: %w", err)
	}

	// Verify no v1alpha1 DSPAs remain (skip in dry-run — we didn't actually migrate)
	if !target.DryRun {
		verifyStep := recorder.Child("verify-migration", "Verify no v1alpha1 DSPAs remain")

		remaining, err := listDSPAs(ctx, target.Client, resources.DataSciencePipelinesApplicationV1Alpha1)
		if err != nil {
			verifyStep.Complete(result.StepFailed, "Failed to verify: %v", err)
		} else if len(remaining) > 0 {
			verifyStep.Complete(result.StepFailed, "%d v1alpha1 DSPA(s) still remain", len(remaining))
		} else {
			verifyStep.Complete(result.StepCompleted, "All DSPAs are now v1")
		}
	}

	return recorder.Build(), nil
}
