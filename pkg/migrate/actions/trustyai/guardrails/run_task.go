package guardrails

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

const (
	expectedProbePath        = "/health"
	expectedProbePort        = int64(8034)
	probeInitialDelaySeconds = 10
	probeTimeoutSeconds      = 10
	probePeriodSeconds       = 20
	probeSuccessThreshold    = 1
	probeFailureThreshold    = 3
)

type probeStatus string

const (
	probeOK         probeStatus = "OK"
	probeNeedsPatch probeStatus = "NEEDS_PATCH"
)

type runTask struct {
	action *PatchGuardrailsAction
}

func (t *runTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-guardrails-probe", "Check GuardrailsOrchestrator readinessProbe")

	gorchs, err := t.discoverGorchs(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list GuardrailsOrchestrators: %v", err)

		return action.BuildResult(target)
	}

	if len(gorchs) == 0 {
		step.Completef(result.StepSkipped, "No GuardrailsOrchestrator CRs found")

		return action.BuildResult(target)
	}

	needsPatch := 0

	for _, gorch := range gorchs {
		name := gorch.GetName()
		ns := gorch.GetNamespace()

		deployment, err := target.Client.GetResource(ctx, resources.Deployment, name, client.InNamespace(ns))
		if err != nil {
			continue
		}

		status := checkProbe(deployment, name)
		if status == probeNeedsPatch {
			needsPatch++
		}
	}

	if needsPatch == 0 {
		step.Completef(result.StepCompleted, "All %d GuardrailsOrchestrator deployment(s) have correct readinessProbe", len(gorchs))
	} else {
		step.Completef(result.StepCompleted, "Found %d/%d deployment(s) needing readinessProbe patch", needsPatch, len(gorchs))
	}

	return action.BuildResult(target)
}

func (t *runTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("patch-guardrails-probe", "Patch GuardrailsOrchestrator readinessProbe")

	gorchs, err := t.discoverGorchs(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list GuardrailsOrchestrators: %v", err)

		return action.BuildResult(target)
	}

	if len(gorchs) == 0 {
		step.Completef(result.StepSkipped, "No GuardrailsOrchestrator CRs found")

		return action.BuildResult(target)
	}

	step.Recordf("discover", "Found %d GuardrailsOrchestrator CR(s)", result.StepCompleted, len(gorchs))

	type patchCandidate struct {
		name      string
		namespace string
		patch     []byte
	}

	var candidates []patchCandidate

	for _, gorch := range gorchs {
		name := gorch.GetName()
		ns := gorch.GetNamespace()

		deployStep := step.Child(
			fmt.Sprintf("check-%s-%s", ns, name),
			fmt.Sprintf("Check %s/%s", ns, name),
		)

		deployment, err := target.Client.GetResource(ctx, resources.Deployment, name, client.InNamespace(ns))
		if err != nil {
			if apierrors.IsNotFound(err) {
				deployStep.Completef(result.StepSkipped, "Deployment %s/%s not found", ns, name)

				continue
			}

			deployStep.Completef(result.StepFailed, "Failed to get deployment %s/%s: %v", ns, name, err)

			return action.BuildResult(target)
		}

		status := checkProbe(deployment, name)
		if status == probeOK {
			deployStep.Completef(result.StepSkipped, "%s/%s: readinessProbe already configured", ns, name)

			continue
		}

		patchBytes, err := buildReadinessProbePatch(name)
		if err != nil {
			deployStep.Completef(result.StepFailed, "Failed to build patch for %s/%s: %v", ns, name, err)

			continue
		}

		deployStep.Completef(result.StepCompleted, "%s/%s needs readinessProbe patch", ns, name)

		candidates = append(candidates, patchCandidate{name: name, namespace: ns, patch: patchBytes})
	}

	if len(candidates) == 0 {
		step.Completef(result.StepCompleted, "No deployments need readinessProbe patching")

		return action.BuildResult(target)
	}

	if target.DryRun {
		for _, cand := range candidates {
			target.IO.Fprintln()
			target.IO.Fprintf("DRY RUN: Would apply readinessProbe patch to %s/%s:", cand.namespace, cand.name)
			target.IO.Fprintln()
			target.IO.Fprintf("%s", string(cand.patch))
			target.IO.Fprintln()
		}

		step.Completef(result.StepSkipped, "Would patch %d deployment(s)", len(candidates))

		return action.BuildResult(target)
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to patch readinessProbe on %d deployment(s)", len(candidates))

		if !confirmation.Prompt(target.IO, "Proceed with patching?") {
			step.Completef(result.StepSkipped, "User cancelled readinessProbe patching")

			return action.BuildResult(target)
		}
	}

	gvr := resources.Deployment.GVR()
	patchedCount := 0

	for _, cand := range candidates {
		patchStep := step.Child(
			fmt.Sprintf("patch-%s-%s", cand.namespace, cand.name),
			fmt.Sprintf("Patch %s/%s", cand.namespace, cand.name),
		)

		_, err := target.Client.Dynamic().Resource(gvr).Namespace(cand.namespace).Patch(
			ctx,
			cand.name,
			types.StrategicMergePatchType,
			cand.patch,
			metav1.PatchOptions{},
		)
		if err != nil {
			patchStep.Completef(result.StepFailed, "Failed to patch %s/%s: %v", cand.namespace, cand.name, err)

			continue
		}

		patchStep.Completef(result.StepCompleted, "Patched %s/%s", cand.namespace, cand.name)

		patchedCount++
	}

	step.Completef(result.StepCompleted, "Patched %d/%d deployment(s)", patchedCount, len(candidates))

	return action.BuildResult(target)
}

func (t *runTask) discoverGorchs(ctx context.Context, target action.Target) ([]*unstructured.Unstructured, error) {
	gorchs, err := target.Client.List(ctx, resources.GuardrailsOrchestrator)
	if err != nil {
		return nil, fmt.Errorf("listing GuardrailsOrchestrators: %w", err)
	}

	if t.action.GorchName == "" {
		return gorchs, nil
	}

	var filtered []*unstructured.Unstructured

	for _, gorch := range gorchs {
		if gorch.GetName() == t.action.GorchName {
			filtered = append(filtered, gorch)
		}
	}

	return filtered, nil
}

func checkProbe(deployment *unstructured.Unstructured, containerName string) probeStatus {
	containers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return probeNeedsPatch
	}

	for _, c := range containers {
		container, ok := c.(map[string]any)
		if !ok {
			continue
		}

		name, _, _ := unstructured.NestedString(container, "name")
		if name != containerName {
			continue
		}

		path, _, _ := unstructured.NestedString(container, "readinessProbe", "httpGet", "path")
		port, _, _ := unstructured.NestedFieldNoCopy(container, "readinessProbe", "httpGet", "port")

		portInt := toInt64(port)

		if path == expectedProbePath && portInt == expectedProbePort {
			return probeOK
		}

		return probeNeedsPatch
	}

	return probeNeedsPatch
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func buildReadinessProbePatch(containerName string) ([]byte, error) {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []map[string]any{
						{
							"name": containerName,
							"readinessProbe": map[string]any{
								"httpGet": map[string]any{
									"path":   expectedProbePath,
									"port":   expectedProbePort,
									"scheme": "HTTP",
								},
								"initialDelaySeconds": probeInitialDelaySeconds,
								"timeoutSeconds":      probeTimeoutSeconds,
								"periodSeconds":       probePeriodSeconds,
								"successThreshold":    probeSuccessThreshold,
								"failureThreshold":    probeFailureThreshold,
							},
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(patch, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling patch: %w", err)
	}

	return data, nil
}
