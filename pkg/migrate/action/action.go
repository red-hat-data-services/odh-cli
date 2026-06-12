package action

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

type ActionGroup string

const (
	GroupMigration  ActionGroup = "migration"
	GroupBackup     ActionGroup = "backup"
	GroupValidation ActionGroup = "validation"
)

type ActionPhase string

const (
	PhasePreUpgrade    ActionPhase = "pre-upgrade"
	PhasePostUpgrade   ActionPhase = "post-upgrade"
	PhasePreEnablement ActionPhase = "pre-enablement"
)

func PhaseValues() []string {
	return []string{
		string(PhasePreUpgrade),
		string(PhasePostUpgrade),
		string(PhasePreEnablement),
	}
}

func (p ActionPhase) Validate() error {
	switch p {
	case PhasePreUpgrade, PhasePostUpgrade, PhasePreEnablement, "":
		return nil
	default:
		return fmt.Errorf("invalid phase %q: must be one of: pre-upgrade, post-upgrade, pre-enablement", p)
	}
}

// Task represents a single executable phase (prepare or run) with validation and execution.
type Task interface {
	Validate(ctx context.Context, target Target) (*result.ActionResult, error)
	Execute(ctx context.Context, target Target) (*result.ActionResult, error)
}

type Action interface {
	ID() string
	Name() string
	Description() string
	Group() ActionGroup
	Phase() ActionPhase

	// CanApply returns whether this action should run for the given target context.
	// Actions can use target.CurrentVersion, target.TargetVersion, or target.Client for filtering.
	CanApply(target Target) bool

	// Prepare returns the Task for the preparation phase (e.g., backups, pre-migration setup).
	// Returns nil if this action has no prepare phase.
	Prepare() Task

	// Run returns the Task for the migration execution phase.
	Run() Task
}

// ActionConfigurer is an optional interface that Actions can implement to register
// their own CLI flags. Commands call AddFlags during flag setup for any registered
// action that implements this interface.
type ActionConfigurer interface {
	AddFlags(fs *pflag.FlagSet)
}

// BuildResult extracts the final ActionResult from the target's recorder.
func BuildResult(target Target) (*result.ActionResult, error) {
	rootRecorder, ok := target.Recorder.(RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

// Target holds all context needed for executing migration actions.
type Target struct {
	Client         client.Client
	RESTConfig     *rest.Config    // Low-level K8s config for actions needing auth tokens or pod exec
	CurrentVersion *semver.Version // Version being migrated FROM
	TargetVersion  *semver.Version // Version being migrated TO
	DryRun         bool
	SkipConfirm    bool
	OutputDir      string // Output directory for backups (used in prepare phase)
	Recorder       StepRecorder
	IO             iostreams.Interface
}
