package migrate

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/data"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/deadlock"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/guardrails"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/metrics"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/otelexporter"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/upgrade"
)

func newDefaultRegistry() *action.ActionRegistry {
	registry := action.NewActionRegistry()

	registry.MustRegister(&rhbok.RHBOKMigrationAction{})
	registry.MustRegister(&aipipelines.PreUpgradeCheckAction{})
	registry.MustRegister(&aipipelines.UpdateDSPRoleAction{})
	registry.MustRegister(&aipipelines.PostUpgradeCheckAction{})
	registry.MustRegister(&modelserving.ServerlessToRawAction{})
	registry.MustRegister(&modelserving.ModelMeshToRawAction{})
	registry.MustRegister(&modelserving.HardwareProfilesIgnorelistAction{})
	registry.MustRegister(&modelserving.AddOwnerReferencesAction{})
	registry.MustRegister(&modelserving.ManagedISVCConfigAction{})
	registry.MustRegister(&upgrade.WorkbenchUpgradeAction{})
	registry.MustRegister(&deadlock.BreakGPUDeadlockAction{})
	registry.MustRegister(&guardrails.PatchGuardrailsAction{})
	registry.MustRegister(&otelexporter.MigrateOtelExporterAction{})
	registry.MustRegister(&metrics.MetricsAction{})
	registry.MustRegister(&data.DataAction{})

	return registry
}
