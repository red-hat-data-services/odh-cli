package metrics

import (
	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
)

const (
	actionID          = "trustyai.metrics"
	actionName        = "Backup and restore TrustyAI scheduled metrics"
	actionDescription = "Backup scheduled metrics via REST API before upgrade, restore after upgrade"
)

type MetricsAction struct {
	BackupDir    string
	BackupFile   string
	MetricType   string
	SkipExisting bool
	RouteLabel   string
}

func (a *MetricsAction) ID() string          { return actionID }
func (a *MetricsAction) Name() string        { return actionName }
func (a *MetricsAction) Description() string { return actionDescription }

func (a *MetricsAction) Group() action.ActionGroup {
	return action.GroupBackup
}

func (a *MetricsAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *MetricsAction) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&a.BackupDir, "metrics-dir", "/tmp/rhoai-upgrade-backup/trustyai",
		"Directory to write metrics backup files")
	fs.StringVar(&a.BackupFile, "metrics-file", "",
		"Backup file to restore from (used with migrate run)")
	fs.StringVar(&a.MetricType, "metrics-type", "all",
		"Metric type filter: all or fairness")
	fs.BoolVar(&a.SkipExisting, "metrics-skip-existing", false,
		"Skip metrics that already exist during restore")
	fs.StringVar(&a.RouteLabel, "metrics-route-label", "trustyai-service-name=trustyai-service",
		"Route label selector for TrustyAI service")
}

func (a *MetricsAction) CanApply(_ action.Target) bool {
	return true
}

func (a *MetricsAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *MetricsAction) Run() action.Task {
	return &runTask{action: a}
}
