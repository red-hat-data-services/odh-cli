package deadlock

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
)

const (
	actionID          = "trustyai.break-gpu-deadlock"
	actionName        = "Break GPU deployment deadlocks"
	actionDescription = "Detect and fix GPU deployment deadlocks caused by TrustyAI patching InferenceServices"
)

type BreakGPUDeadlockAction struct{}

func (a *BreakGPUDeadlockAction) ID() string          { return actionID }
func (a *BreakGPUDeadlockAction) Name() string        { return actionName }
func (a *BreakGPUDeadlockAction) Description() string { return actionDescription }

func (a *BreakGPUDeadlockAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *BreakGPUDeadlockAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *BreakGPUDeadlockAction) CanApply(_ action.Target) bool {
	return true
}

func (a *BreakGPUDeadlockAction) Prepare() action.Task {
	return nil
}

func (a *BreakGPUDeadlockAction) Run() action.Task {
	return &runTask{action: a}
}
