package kueue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
)

type runTask struct {
	action *AttachKueueLabelAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if t.action.WorkbenchName != "" && t.action.WorkbenchNamespace == "" {
		return nil, errors.New("--workbench-name requires --workbench-namespace")
	}

	if errs := validation.IsValidLabelValue(t.action.queueName()); len(errs) > 0 {
		return nil, fmt.Errorf("invalid --queue-name %q: %s", t.action.queueName(), strings.Join(errs, "; "))
	}

	step := target.Recorder.Child(
		"validate-kueue-labels",
		"Validate Kueue label requirements",
	)

	eligible, err := t.findEligibleNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to discover eligible notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(eligible) == 0 {
		step.Completef(result.StepCompleted, "No notebooks require the Kueue queue-name label")

		return action.BuildResult(target)
	}

	step.Completef(result.StepCompleted,
		"Found %d notebook(s) in Kueue-managed namespaces missing the queue-name label", len(eligible))

	return action.BuildResult(target)
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if t.action.WorkbenchName != "" && t.action.WorkbenchNamespace == "" {
		return nil, errors.New("--workbench-name requires --workbench-namespace")
	}

	if errs := validation.IsValidLabelValue(t.action.queueName()); len(errs) > 0 {
		return nil, fmt.Errorf("invalid --queue-name %q: %s", t.action.queueName(), strings.Join(errs, "; "))
	}

	discoverStep := target.Recorder.Child(
		"discover-notebooks",
		"Discover notebooks requiring Kueue label",
	)

	eligible, err := t.findEligibleNotebooks(ctx, target)
	if err != nil {
		discoverStep.Completef(result.StepFailed, "Failed to discover eligible notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(eligible) == 0 {
		discoverStep.Completef(result.StepCompleted,
			"No notebooks require the Kueue queue-name label")

		return action.BuildResult(target)
	}

	discoverStep.Completef(result.StepCompleted,
		"Found %d notebook(s) requiring the queue-name label", len(eligible))

	t.applyLabels(ctx, target, eligible)

	return action.BuildResult(target)
}

func (t *runTask) findEligibleNotebooks(
	ctx context.Context,
	target action.Target,
) ([]*unstructured.Unstructured, error) {
	kueueNamespaces, err := t.action.kueueManagedNamespaces(ctx, target)
	if err != nil {
		return nil, err
	}

	if kueueNamespaces.Len() == 0 {
		return nil, nil
	}

	notebooks, err := t.action.listNotebooks(ctx, target)
	if err != nil {
		return nil, err
	}

	var eligible []*unstructured.Unstructured

	for _, nb := range notebooks {
		if !kueueNamespaces.Has(nb.GetNamespace()) {
			continue
		}

		if hasQueueNameLabel(nb) {
			continue
		}

		eligible = append(eligible, nb)
	}

	return eligible, nil
}

func (t *runTask) applyLabels(
	ctx context.Context,
	target action.Target,
	notebooks []*unstructured.Unstructured,
) {
	step := target.Recorder.Child(
		"apply-kueue-labels",
		"Apply Kueue queue-name labels",
	)

	if target.DryRun {
		for _, nb := range notebooks {
			step.Recordf("would-label",
				fmt.Sprintf("Would add label %s=%s to %s/%s",
					constants.LabelKueueQueueName, t.action.queueName(),
					nb.GetNamespace(), nb.GetName()),
				result.StepSkipped)
		}

		step.Completef(result.StepSkipped,
			"Would label %d notebook(s) with queue-name=%s", len(notebooks), t.action.queueName())

		return
	}

	if !t.action.promptBeforeModification(target, len(notebooks)) {
		step.Completef(result.StepSkipped, "User cancelled modification")

		return
	}

	successCount := 0
	failCount := 0

	for _, nb := range notebooks {
		nbStep := step.Child(
			fmt.Sprintf("label-%s-%s", nb.GetNamespace(), nb.GetName()),
			fmt.Sprintf("Label %s/%s", nb.GetNamespace(), nb.GetName()),
		)

		if err := t.action.patchLabel(ctx, target, nb); err != nil {
			nbStep.Completef(result.StepFailed,
				"Failed to label notebook %s/%s: %v", nb.GetNamespace(), nb.GetName(), err)
			failCount++

			continue
		}

		nbStep.Completef(result.StepCompleted,
			"Added label queue-name=%s to %s/%s", t.action.queueName(), nb.GetNamespace(), nb.GetName())
		successCount++
	}

	if failCount > 0 {
		step.Completef(result.StepFailed,
			"Labeled %d/%d notebook(s), %d failure(s)", successCount, len(notebooks), failCount)
	} else {
		step.Completef(result.StepCompleted,
			"Labeled %d notebook(s) with queue-name=%s", successCount, t.action.queueName())
	}
}
