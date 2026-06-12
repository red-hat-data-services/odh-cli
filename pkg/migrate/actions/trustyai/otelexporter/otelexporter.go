package otelexporter

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
)

const (
	actionID          = "trustyai.migrate-gorch-otel-exporter"
	actionName        = "Migrate GuardrailsOrchestrator otelExporter schema"
	actionDescription = "Migrate otelExporter from RHOAI 2.25 schema to current schema"
)

type MigrateOtelExporterAction struct{}

func (a *MigrateOtelExporterAction) ID() string          { return actionID }
func (a *MigrateOtelExporterAction) Name() string        { return actionName }
func (a *MigrateOtelExporterAction) Description() string { return actionDescription }

func (a *MigrateOtelExporterAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *MigrateOtelExporterAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *MigrateOtelExporterAction) CanApply(target action.Target) bool {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false
	}

	return target.CurrentVersion.Major == 2 && target.TargetVersion.Major >= 3
}

func (a *MigrateOtelExporterAction) Prepare() action.Task {
	return nil
}

func (a *MigrateOtelExporterAction) Run() action.Task {
	return &runTask{action: a}
}
