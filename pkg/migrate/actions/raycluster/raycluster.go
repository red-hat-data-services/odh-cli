package raycluster

import (
	"time"

	"github.com/spf13/pflag"

	rcpkg "github.com/opendatahub-io/odh-cli/pkg/migrate/raycluster"
)

type sharedOptions struct {
	ClusterName  string
	Namespace    string
	OutputDir    string
	FromBackup   string
	RouteTimeout time.Duration
}

// NewActions creates the paired raycluster backup and migrate actions.
// Both share an options struct so flag values set on one are visible to the other.
func NewActions() (*BackupAction, *MigrateAction) {
	opts := &sharedOptions{
		OutputDir:    rcpkg.DefaultBackupDir,
		RouteTimeout: rcpkg.DefaultRouteTimeout,
	}

	return &BackupAction{opts: opts}, &MigrateAction{opts: opts}
}

func addScopeFlags(opts *sharedOptions, fs *pflag.FlagSet) {
	if fs.Lookup("raycluster-cluster") == nil {
		fs.StringVar(&opts.ClusterName, "raycluster-cluster", "", "Backup/migrate a specific RayCluster (requires --raycluster-namespace)")
	}

	if fs.Lookup("raycluster-namespace") == nil {
		fs.StringVar(&opts.Namespace, "raycluster-namespace", "", "Limit to RayClusters in this namespace")
	}
}

func addBackupFlags(opts *sharedOptions, fs *pflag.FlagSet) {
	addScopeFlags(opts, fs)
	fs.StringVar(&opts.OutputDir, "raycluster-output-dir", rcpkg.DefaultBackupDir, "Directory for RayCluster backup YAML files")
}

func addMigrateFlags(opts *sharedOptions, fs *pflag.FlagSet) {
	addScopeFlags(opts, fs)
	fs.StringVar(&opts.FromBackup, "raycluster-from-backup", "", "Restore RayClusters from backup file or directory (deletes existing cluster first)")
	fs.DurationVar(&opts.RouteTimeout, "raycluster-timeout", rcpkg.DefaultRouteTimeout, "Timeout for waiting for cluster route to become available")
}
