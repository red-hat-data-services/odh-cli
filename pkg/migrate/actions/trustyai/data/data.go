package data

import (
	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
)

const (
	actionID          = "trustyai.data"
	actionName        = "Backup and restore TrustyAI data storage"
	actionDescription = "Backup and restore TrustyAI PVC files or MariaDB database"
)

type DataAction struct {
	BackupDir   string
	BackupFile  string
	ServiceName string
}

func (a *DataAction) ID() string          { return actionID }
func (a *DataAction) Name() string        { return actionName }
func (a *DataAction) Description() string { return actionDescription }

func (a *DataAction) Group() action.ActionGroup {
	return action.GroupBackup
}

func (a *DataAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *DataAction) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&a.BackupDir, "data-dir", "/tmp/rhoai-upgrade-backup/trustyai",
		"Directory to write data backup files")
	fs.StringVar(&a.BackupFile, "data-file", "",
		"Backup directory to restore from (used with migrate run)")
	fs.StringVar(&a.ServiceName, "data-service-name", "",
		"TrustyAIService CR name (auto-detected if omitted)")
}

func (a *DataAction) CanApply(_ action.Target) bool {
	return true
}

func (a *DataAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *DataAction) Run() action.Task {
	return &runTask{action: a}
}
