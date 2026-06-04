package aipipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// PostUpgradeCheckAction verifies pipeline server pods are healthy post-upgrade.
type PostUpgradeCheckAction struct{}

func (a *PostUpgradeCheckAction) ID() string          { return postUpgradeCheckID }
func (a *PostUpgradeCheckAction) Name() string        { return postUpgradeCheckName }
func (a *PostUpgradeCheckAction) Description() string { return postUpgradeCheckDescription }

func (a *PostUpgradeCheckAction) Group() action.ActionGroup { return action.GroupValidation }
func (a *PostUpgradeCheckAction) Phase() action.ActionPhase { return action.PhasePostUpgrade }

func (a *PostUpgradeCheckAction) CanApply(target action.Target) bool {
	return target.CurrentVersion != nil && target.CurrentVersion.Major == 2
}

func (a *PostUpgradeCheckAction) Prepare() action.Task { return nil }
func (a *PostUpgradeCheckAction) Run() action.Task     { return &postUpgradeCheckRunTask{} }

type postUpgradeCheckRunTask struct{}

func (t *postUpgradeCheckRunTask) Validate(_ context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	step := recorder.Child("check-state-file", "Check for pre-upgrade state file")

	statePath := defaultStatePath()

	if _, err := loadPodHealthState(statePath); err != nil {
		step.Complete(result.StepFailed,
			"Pre-upgrade state not found at %s. Run 'migrate prepare -m %s' before upgrading",
			statePath, preUpgradeCheckID)
	} else {
		step.Complete(result.StepCompleted, "Pre-upgrade state file found at %s", statePath)
	}

	return recorder.Build(), nil
}

func (t *postUpgradeCheckRunTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	// Load pre-upgrade state
	loadStep := recorder.Child("load-state", "Load pre-upgrade state")

	statePath := defaultStatePath()

	preState, err := loadPodHealthState(statePath)
	if err != nil {
		loadStep.Complete(result.StepFailed,
			"Pre-upgrade state not found at %s. Run 'migrate prepare -m %s' before upgrading",
			statePath, preUpgradeCheckID)

		return recorder.Build(), fmt.Errorf("loading pre-upgrade state: %w", err)
	}

	loadStep.Complete(result.StepCompleted, "Loaded state captured at %s", preState.CapturedAt)

	if len(preState.DSPAs) == 0 {
		recorder.Record("no-dspas", "No DSPAs were tracked pre-upgrade", result.StepCompleted)

		return recorder.Build(), nil
	}

	// Initial check — if no degradation, return immediately
	comparisons, err := comparePodHealth(ctx, target.Client, preState)
	if err != nil {
		recorder.Record("compare-failed", "Failed to compare pod health: %v", result.StepFailed, err)

		return recorder.Build(), fmt.Errorf("comparing pod health: %w", err)
	}

	if !hasAnyDegradation(comparisons) {
		t.recordComparisons(recorder, comparisons)

		return recorder.Build(), nil
	}

	// Degradation detected — poll for recovery
	target.IO.Errorf("Degradation detected. Waiting up to %s for pods to recover...\n",
		postUpgradeTimeout)

	comparisons, err = t.pollForRecovery(ctx, target, preState)
	if err != nil {
		recorder.Record("poll-failed", "Failed during recovery polling: %v", result.StepFailed, err)

		return recorder.Build(), fmt.Errorf("polling for recovery: %w", err)
	}

	t.recordComparisons(recorder, comparisons)

	return recorder.Build(), nil
}

func (t *postUpgradeCheckRunTask) pollForRecovery(
	ctx context.Context,
	target action.Target,
	preState PodHealthState,
) ([]podGroupComparison, error) {
	ticker := time.NewTicker(postUpgradePollInterval)
	defer ticker.Stop()

	deadline := time.After(postUpgradeTimeout)

	for {
		select {
		case <-ctx.Done():
			comparisons, err := comparePodHealthFreshCtx(target.Client, preState) //nolint:contextcheck // parent ctx is cancelled
			if err != nil {
				return nil, fmt.Errorf("final comparison after context cancellation: %w", err)
			}

			return comparisons, nil
		case <-deadline:
			target.IO.Errorf("Wait timeout reached (%s). Running final comparison...\n",
				postUpgradeTimeout)

			comparisons, err := comparePodHealth(ctx, target.Client, preState)
			if err != nil {
				return nil, fmt.Errorf("final comparison after timeout: %w", err)
			}

			return comparisons, nil
		case <-ticker.C:
			comparisons, err := comparePodHealth(ctx, target.Client, preState)
			if err != nil {
				target.IO.Errorf("Comparison failed: %v, retrying...\n", err)

				continue
			}

			if !hasAnyDegradation(comparisons) {
				target.IO.Errorf("All previously-healthy pods recovered\n")

				return comparisons, nil
			}

			target.IO.Errorf("Not yet fully recovered, retrying...\n")
		}
	}
}

func (t *postUpgradeCheckRunTask) recordComparisons(
	recorder action.RootRecorder,
	comparisons []podGroupComparison,
) {
	step := recorder.Child("comparison", "Pre vs Post-Upgrade Comparison")

	for _, comp := range comparisons {
		var status result.StepStatus

		var msg string

		switch comp.Status {
		case podGroupOK:
			status = result.StepCompleted
			msg = "[OK] " + comp.Prefix
		case podGroupImproved:
			status = result.StepCompleted
			msg = fmt.Sprintf("[IMPROVED] %s (was unhealthy, now healthy)", comp.Prefix)
		case podGroupWarn:
			status = result.StepCompleted
			msg = fmt.Sprintf("[WARN] %s (unchanged unhealthy state, pre-existing issue)", comp.Prefix)
		case podGroupFail:
			status = result.StepFailed
			msg = fmt.Sprintf("[FAIL] %s (was healthy, now unhealthy)", comp.Prefix)
		}

		groupStep := step.Child(comp.Prefix, msg)

		if comp.Status == podGroupFail {
			recordFailedPodGroup(groupStep, comp.Current)
		}

		groupStep.Complete(status, msg)
	}

	if hasAnyDegradation(comparisons) {
		step.Complete(result.StepFailed, "One or more DSPA pods degraded compared to pre-upgrade state")
	} else {
		step.Complete(result.StepCompleted, "All DSPA pods are in an equal or better state than before upgrade")
	}
}

func comparePodHealthFreshCtx(c client.Client, preState PodHealthState) ([]podGroupComparison, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postUpgradePollInterval)
	defer cancel()

	return comparePodHealth(ctx, c, preState)
}

func recordFailedPodGroup(groupStep action.StepRecorder, current PodGroup) {
	if !current.PodsFound {
		groupStep.Record("missing", "[MISSING] No pods found post-upgrade", result.StepFailed)

		return
	}

	for _, pod := range current.Pods {
		if pod.Healthy {
			groupStep.Record(pod.Name, "[OK] %s (Running, Ready)", result.StepCompleted, pod.Name)
		} else {
			groupStep.Record(pod.Name, "[FAIL] %s (Phase: %s, Ready: %s)", result.StepFailed, pod.Name, pod.Phase, pod.Ready)
		}
	}
}
