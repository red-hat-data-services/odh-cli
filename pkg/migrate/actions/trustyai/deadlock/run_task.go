package deadlock

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

const (
	predictorLabel = "component=predictor"
	isvcLabel      = "serving.kserve.io/inferenceservice"
)

type deadlockInfo struct {
	namespace        string
	inferenceService string
	runningPod       string
	pendingPod       string
}

type runTask struct {
	action *BreakGPUDeadlockAction
}

func (t *runTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("detect-deadlocks", "Detect GPU deployment deadlocks")

	deadlocks, err := detectDeadlocks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to detect deadlocks: %v", err)

		return action.BuildResult(target)
	}

	if len(deadlocks) == 0 {
		step.Completef(result.StepCompleted, "No GPU deadlocks detected")
	} else {
		step.Completef(result.StepCompleted, "Found %d GPU deadlock(s) across cluster", len(deadlocks))
	}

	return action.BuildResult(target)
}

func (t *runTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("break-gpu-deadlocks", "Break GPU deployment deadlocks")

	deadlocks, err := detectDeadlocks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to detect deadlocks: %v", err)

		return action.BuildResult(target)
	}

	if len(deadlocks) == 0 {
		step.Completef(result.StepSkipped, "No GPU deadlocks detected")

		return action.BuildResult(target)
	}

	step.Recordf("detect", "Found %d GPU deadlock(s)", result.StepCompleted, len(deadlocks))

	for _, dl := range deadlocks {
		step.AddDetail(fmt.Sprintf("%s/%s", dl.namespace, dl.inferenceService), map[string]string{
			"runningPod": dl.runningPod,
			"pendingPod": dl.pendingPod,
		})
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would delete %d Running pod(s) to resolve deadlocks", len(deadlocks))

		return action.BuildResult(target)
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to delete %d Running pod(s) to resolve GPU deadlocks", len(deadlocks))

		if !confirmation.Prompt(target.IO, "Proceed with pod deletion?") {
			step.Completef(result.StepSkipped, "User cancelled deadlock resolution")

			return action.BuildResult(target)
		}
	}

	fixedCount := 0

	for _, dl := range deadlocks {
		fixStep := step.Child(
			fmt.Sprintf("fix-%s-%s", dl.namespace, dl.inferenceService),
			fmt.Sprintf("Delete Running pod for %s/%s", dl.namespace, dl.inferenceService),
		)

		err := target.Client.CoreV1().Pods(dl.namespace).Delete(ctx, dl.runningPod, metav1.DeleteOptions{})
		if err != nil {
			fixStep.Completef(result.StepFailed, "Failed to delete pod %s: %v", dl.runningPod, err)

			continue
		}

		fixStep.Completef(result.StepCompleted, "Deleted pod %s for InferenceService %s", dl.runningPod, dl.inferenceService)

		fixedCount++
	}

	step.Completef(result.StepCompleted, "Resolved %d/%d GPU deadlock(s)", fixedCount, len(deadlocks))

	return action.BuildResult(target)
}

func detectDeadlocks(ctx context.Context, target action.Target) ([]deadlockInfo, error) {
	pods, err := target.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: predictorLabel,
	})
	if err != nil {
		return nil, fmt.Errorf("listing predictor pods: %w", err)
	}

	type isvcPods struct {
		namespace string
		running   string
		pending   string
	}

	type isvcKey struct {
		namespace string
		name      string
	}

	byISVC := make(map[isvcKey]*isvcPods)

	for i := range pods.Items {
		pod := &pods.Items[i]

		isvcName := pod.Labels[isvcLabel]
		if isvcName == "" {
			continue
		}

		key := isvcKey{namespace: pod.Namespace, name: isvcName}

		entry, ok := byISVC[key]
		if !ok {
			entry = &isvcPods{namespace: pod.Namespace}
			byISVC[key] = entry
		}

		switch pod.Status.Phase { //nolint:exhaustive // only Running and Pending are relevant
		case corev1.PodRunning:
			if entry.running == "" {
				entry.running = pod.Name
			}
		case corev1.PodPending:
			if entry.pending == "" {
				entry.pending = pod.Name
			}
		}
	}

	var deadlocks []deadlockInfo

	for key, pods := range byISVC {
		if pods.running != "" && pods.pending != "" {
			deadlocks = append(deadlocks, deadlockInfo{
				namespace:        key.namespace,
				inferenceService: key.name,
				runningPod:       pods.running,
				pendingPod:       pods.pending,
			})
		}
	}

	sort.Slice(deadlocks, func(i, j int) bool {
		if deadlocks[i].namespace != deadlocks[j].namespace {
			return deadlocks[i].namespace < deadlocks[j].namespace
		}

		return deadlocks[i].inferenceService < deadlocks[j].inferenceService
	})

	return deadlocks, nil
}
