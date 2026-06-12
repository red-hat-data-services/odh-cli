package training

import (
	"context"
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// VerifyWorkloadsAction enumerates Kubeflow v1 training workloads as a pre-upgrade check,
// categorizing them by migration readiness for Trainer v2 TrainJob.
type VerifyWorkloadsAction struct{}

func (a *VerifyWorkloadsAction) ID() string          { return verifyWorkloadsID }
func (a *VerifyWorkloadsAction) Name() string        { return verifyWorkloadsName }
func (a *VerifyWorkloadsAction) Description() string { return verifyWorkloadsDescription }

func (a *VerifyWorkloadsAction) Group() action.ActionGroup { return action.GroupValidation }
func (a *VerifyWorkloadsAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *VerifyWorkloadsAction) CanApply(target action.Target) bool {
	return target.CurrentVersion != nil && target.TargetVersion != nil &&
		target.CurrentVersion.Major == 2 && target.TargetVersion.Major == 3
}

func (a *VerifyWorkloadsAction) Prepare() action.Task { return nil }
func (a *VerifyWorkloadsAction) Run() action.Task     { return &verifyWorkloadsTask{} }

type verifyWorkloadsTask struct{}

func (t *verifyWorkloadsTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	return t.Execute(ctx, target)
}

func (t *verifyWorkloadsTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	trainjobReady, err := t.checkTrainJobCRD(ctx, target, recorder)
	if err != nil {
		return recorder.Build(), err
	}

	allEntries, err := t.enumerateV1Workloads(ctx, target, recorder)
	if err != nil {
		return recorder.Build(), err
	}

	t.assessMigrationReadiness(allEntries, recorder)
	t.buildSummary(allEntries, trainjobReady, recorder)

	return recorder.Build(), nil
}

func (t *verifyWorkloadsTask) checkTrainJobCRD(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) (bool, error) {
	step := recorder.Child("trainjob-crd", "Check TrainJob v2 CRD readiness")

	_, err := target.Client.List(ctx, resources.TrainJob)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			step.AddDetail("trainjobCRDInstalled", false)
			step.Completef(result.StepCompleted, "TrainJob CRD not installed — v2 API not yet available on this cluster")

			return false, nil
		}

		step.AddDetail("trainjobCRDInstalled", false)
		step.Completef(result.StepFailed, "Failed to check TrainJob CRD: %v", err)

		return false, fmt.Errorf("checking TrainJob CRD: %w", err)
	}

	step.AddDetail("trainjobCRDInstalled", true)
	step.Completef(result.StepCompleted, "TrainJob CRD installed — v2 API available")

	return true, nil
}

func (t *verifyWorkloadsTask) enumerateV1Workloads(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) ([]WorkloadEntry, error) {
	entries, failures, err := EnumerateWorkloads(ctx, target.Client)

	failuresByKind := make(map[string]ListFailure, len(failures))
	for _, f := range failures {
		failuresByKind[f.Kind] = f
	}

	entriesByKind := make(map[string][]WorkloadEntry)
	for _, e := range entries {
		entriesByKind[e.Kind] = append(entriesByKind[e.Kind], e)
	}

	for _, rt := range TrainingJobTypes {
		step := recorder.Child(rt.Resource, fmt.Sprintf("List %s workloads", rt.Kind))

		if f, ok := failuresByKind[rt.Kind]; ok {
			step.Completef(result.StepFailed, "Failed to list: %v", f.Err)

			continue
		}

		kindEntries := entriesByKind[rt.Kind]
		if len(kindEntries) == 0 {
			step.Completef(result.StepCompleted, "No %s found", rt.Kind)

			continue
		}

		for _, entry := range kindEntries {
			step.Recordf(
				entry.Name,
				"%s/%s — %s (age: %s)",
				result.StepCompleted,
				entry.Namespace, entry.Name, entry.Status, entry.Age,
			)
		}

		step.Completef(result.StepCompleted, "Found %d %s(s)", len(kindEntries), rt.Kind)
	}

	if err != nil {
		return entries, fmt.Errorf("enumerating v1 workloads: %w", err)
	}

	return entries, nil
}

func (t *verifyWorkloadsTask) assessMigrationReadiness(
	entries []WorkloadEntry,
	recorder action.RootRecorder,
) {
	step := recorder.Child("migration-readiness", "Assess migration readiness")

	var blockers []WorkloadEntry

	for _, e := range entries {
		if isActiveStatus(e.Status) {
			blockers = append(blockers, e)
		}
	}

	step.AddDetail("blockers", len(blockers))
	step.AddDetail("completable", len(entries)-len(blockers))

	if len(entries) == 0 {
		step.Completef(result.StepCompleted, "No v1 training workloads found — nothing to migrate")

		return
	}

	if len(blockers) == 0 {
		step.Completef(result.StepCompleted, "All %d v1 training job(s) are completed — safe to proceed with migration", len(entries))

		return
	}

	for _, b := range blockers {
		step.Recordf(
			b.Name,
			"%s/%s is %s — must complete or stop before migration",
			result.StepFailed,
			b.Namespace, b.Name, b.Status,
		)
	}

	step.Completef(result.StepFailed,
		"[BLOCKER] %d active v1 job(s) must complete or be stopped before migration", len(blockers))
}

func (t *verifyWorkloadsTask) buildSummary(
	entries []WorkloadEntry,
	trainjobReady bool,
	recorder action.RootRecorder,
) {
	step := recorder.Child("summary", "Migration summary")

	report := BuildReport(entries)

	step.AddDetail("total", report.Summary.Total)
	step.AddDetail("byKind", report.Summary.ByKind)
	step.AddDetail("byStatus", report.Summary.ByStatus)
	step.AddDetail("trainjobCRDInstalled", trainjobReady)

	migrationMap := buildMigrationMap(report.Summary.ByKind)
	step.AddDetail("migrationMap", migrationMap)

	var blockerCount int

	for _, e := range entries {
		if isActiveStatus(e.Status) {
			blockerCount++
		}
	}

	step.AddDetail("blockers", blockerCount)
	step.AddDetail("completable", report.Summary.Total-blockerCount)

	step.Completef(result.StepCompleted,
		"Found %d v1 training workload(s): %d blocking migration, %d completable",
		report.Summary.Total, blockerCount, report.Summary.Total-blockerCount)
}

func isActiveStatus(status string) bool {
	lower := strings.ToLower(status)

	return lower == "running" || lower == "created"
}

func buildMigrationMap(byKind map[string]int) map[string]string {
	migrationMap := make(map[string]string)

	for kind := range byKind {
		if runtime, ok := v1KindToV2Runtime[kind]; ok {
			migrationMap[kind] = fmt.Sprintf("TrainJob with runtime %q", runtime)
		}
	}

	return migrationMap
}
