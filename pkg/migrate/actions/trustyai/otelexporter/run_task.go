package otelexporter

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

type runTask struct {
	action *MigrateOtelExporterAction
}

func (t *runTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-otel-schema", "Check otelExporter schema")

	gorchs, err := target.Client.List(ctx, resources.GuardrailsOrchestrator)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list GuardrailsOrchestrators: %v", err)

		return action.BuildResult(target)
	}

	if len(gorchs) == 0 {
		step.Completef(result.StepSkipped, "No GuardrailsOrchestrator CRs found")

		return action.BuildResult(target)
	}

	needsMigration := 0

	for _, gorch := range gorchs {
		info := classifyCR(gorch)
		if info.status == statusNeedsMigration {
			needsMigration++
		}
	}

	if needsMigration == 0 {
		step.Completef(result.StepCompleted, "All %d GuardrailsOrchestrator CR(s) have current otelExporter schema", len(gorchs))
	} else {
		step.Completef(result.StepCompleted, "Found %d/%d GuardrailsOrchestrator CR(s) needing otelExporter migration", needsMigration, len(gorchs))
	}

	return action.BuildResult(target)
}

func (t *runTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("migrate-otel-schema", "Migrate otelExporter schema")

	gorchs, err := target.Client.List(ctx, resources.GuardrailsOrchestrator)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list GuardrailsOrchestrators: %v", err)

		return action.BuildResult(target)
	}

	if len(gorchs) == 0 {
		step.Completef(result.StepSkipped, "No GuardrailsOrchestrator CRs found")

		return action.BuildResult(target)
	}

	step.Recordf("discover", "Found %d GuardrailsOrchestrator CR(s)", result.StepCompleted, len(gorchs))

	var candidates []migrationCandidate

	for _, gorch := range gorchs {
		if cand, ok := evaluateCR(gorch, step, target); ok {
			candidates = append(candidates, cand)
		}
	}

	if len(candidates) == 0 {
		step.Completef(result.StepCompleted, "No GuardrailsOrchestrator CRs need otelExporter migration")

		return action.BuildResult(target)
	}

	if target.DryRun {
		for _, cand := range candidates {
			target.IO.Fprintln()
			target.IO.Fprintf("DRY RUN: Would apply patch to %s/%s:", cand.namespace, cand.name)
			target.IO.Fprintln()
			target.IO.Fprintf("%s", string(cand.patchData))
			target.IO.Fprintln()
		}

		step.Completef(result.StepSkipped, "Would migrate %d GuardrailsOrchestrator(s)", len(candidates))

		return action.BuildResult(target)
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to migrate otelExporter on %d GuardrailsOrchestrator(s)", len(candidates))

		if !confirmation.Prompt(target.IO, "Proceed with otelExporter migration?") {
			step.Completef(result.StepSkipped, "User cancelled otelExporter migration")

			return action.BuildResult(target)
		}
	}

	gvr := resources.GuardrailsOrchestrator.GVR()
	migratedCount := 0

	for _, cand := range candidates {
		patchStep := step.Child(
			fmt.Sprintf("patch-%s-%s", cand.namespace, cand.name),
			fmt.Sprintf("Patch %s/%s", cand.namespace, cand.name),
		)

		_, err := target.Client.Dynamic().Resource(gvr).Namespace(cand.namespace).Patch(
			ctx,
			cand.name,
			types.MergePatchType,
			cand.patchData,
			metav1.PatchOptions{},
		)
		if err != nil {
			patchStep.Completef(result.StepFailed, "Failed to patch %s/%s: %v", cand.namespace, cand.name, err)

			continue
		}

		patchStep.Completef(result.StepCompleted, "Migrated %s/%s", cand.namespace, cand.name)

		migratedCount++
	}

	step.Completef(result.StepCompleted, "Migrated %d/%d GuardrailsOrchestrator(s)", migratedCount, len(candidates))

	return action.BuildResult(target)
}

func evaluateCR(gorch *unstructured.Unstructured, step action.StepRecorder, target action.Target) (migrationCandidate, bool) {
	info := classifyCR(gorch)

	crStep := step.Child(
		fmt.Sprintf("check-%s-%s", info.namespace, info.gorchName),
		fmt.Sprintf("Check %s/%s", info.namespace, info.gorchName),
	)

	if info.status == statusInvalid {
		crStep.Completef(result.StepFailed, "%s: %s", info.gorchName, info.message)

		return migrationCandidate{}, false
	}

	if info.status != statusNeedsMigration {
		crStep.Completef(result.StepSkipped, "%s: %s", info.gorchName, info.message)

		return migrationCandidate{}, false
	}

	otel, _, _ := unstructured.NestedMap(gorch.Object, "spec", "otelExporter")
	newFields, warnings := mapOtelFields(otel)

	for _, w := range warnings {
		target.IO.Errorf("WARNING: %s/%s: %s", info.namespace, info.gorchName, w)
	}

	if len(newFields) == 0 {
		crStep.Completef(result.StepSkipped, "%s: old-schema found but could not infer new fields", info.gorchName)

		return migrationCandidate{}, false
	}

	patchData, err := buildOtelMigrationPatch(newFields)
	if err != nil {
		crStep.Completef(result.StepFailed, "Failed to build patch for %s: %v", info.gorchName, err)

		return migrationCandidate{}, false
	}

	crStep.Completef(result.StepCompleted, "%s needs migration", info.gorchName)

	return migrationCandidate{
		name:      info.gorchName,
		namespace: info.namespace,
		patchData: patchData,
	}, true
}

func classifyCR(gorch *unstructured.Unstructured) migrationInfo {
	name := gorch.GetName()
	namespace := gorch.GetNamespace()

	otel, found, err := unstructured.NestedMap(gorch.Object, "spec", "otelExporter")
	if err != nil {
		return migrationInfo{
			gorchName: name,
			namespace: namespace,
			status:    statusInvalid,
			message:   fmt.Sprintf("invalid otelExporter: %v", err),
		}
	}

	if !found || len(otel) == 0 {
		return migrationInfo{
			gorchName: name,
			namespace: namespace,
			status:    statusSkipped,
			message:   "no otelExporter configured",
		}
	}

	if hasOnlyNewFields(otel) {
		return migrationInfo{
			gorchName: name,
			namespace: namespace,
			status:    statusSkipped,
			message:   "already on new otelExporter schema",
		}
	}

	if !hasAnyOldFields(otel) {
		return migrationInfo{
			gorchName: name,
			namespace: namespace,
			status:    statusSkipped,
			message:   "unknown otelExporter schema; leaving untouched",
		}
	}

	return migrationInfo{
		gorchName: name,
		namespace: namespace,
		status:    statusNeedsMigration,
		message:   "needs migration",
	}
}
