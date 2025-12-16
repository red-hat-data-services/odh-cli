package action

import (
	"context"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/action/result"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type ActionGroup string

const (
	GroupMigration  ActionGroup = "migration"
	GroupBackup     ActionGroup = "backup"
	GroupValidation ActionGroup = "validation"
)

type Action interface {
	ID() string
	Name() string
	Description() string
	Group() ActionGroup
	CanApply(
		currentVersion *semver.Version,
		targetVersion *semver.Version,
	) bool
	Validate(
		ctx context.Context,
		target *ActionTarget,
	) (*result.ActionResult, error)
	Execute(
		ctx context.Context,
		target *ActionTarget,
	) (*result.ActionResult, error)
}

type ActionTarget struct {
	Client         *client.Client
	CurrentVersion *version.ClusterVersion
	TargetVersion  *version.ClusterVersion
	DryRun         bool
	BackupPath     string
	SkipConfirm    bool
	Recorder       StepRecorder
	IO             iostreams.Interface
}
