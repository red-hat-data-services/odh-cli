package guardrails

import (
	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
)

const (
	actionID          = "trustyai.patch-guardrails"
	actionName        = "Patch GuardrailsOrchestrator readinessProbe"
	actionDescription = "Add readinessProbe to GuardrailsOrchestrator deployments for RHOAI 2.5 to 3.3 upgrades"
)

type PatchGuardrailsAction struct {
	GorchName string
}

func (a *PatchGuardrailsAction) ID() string          { return actionID }
func (a *PatchGuardrailsAction) Name() string        { return actionName }
func (a *PatchGuardrailsAction) Description() string { return actionDescription }

func (a *PatchGuardrailsAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *PatchGuardrailsAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *PatchGuardrailsAction) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&a.GorchName, "gorch-name", "", "Filter to a specific GuardrailsOrchestrator CR name (optional; default: check all)")
}

func (a *PatchGuardrailsAction) CanApply(target action.Target) bool {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false
	}

	return target.CurrentVersion.Major == 2 && target.TargetVersion.Major >= 3
}

func (a *PatchGuardrailsAction) Prepare() action.Task {
	return nil
}

func (a *PatchGuardrailsAction) Run() action.Task {
	return &runTask{action: a}
}
