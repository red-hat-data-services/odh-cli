package aipipelines

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

// UpdateDSPRoleAction patches custom RBAC roles to add the datasciencepipelinesapplications/api subresource.
type UpdateDSPRoleAction struct{}

func (a *UpdateDSPRoleAction) ID() string          { return updateDSPRoleID }
func (a *UpdateDSPRoleAction) Name() string        { return updateDSPRoleName }
func (a *UpdateDSPRoleAction) Description() string { return updateDSPRoleDescription }

func (a *UpdateDSPRoleAction) Group() action.ActionGroup { return action.GroupMigration }
func (a *UpdateDSPRoleAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *UpdateDSPRoleAction) CanApply(target action.Target) bool {
	return target.CurrentVersion != nil && target.CurrentVersion.Major == 2
}

func (a *UpdateDSPRoleAction) Prepare() action.Task { return &updateDSPRolePrepareTask{} }
func (a *UpdateDSPRoleAction) Run() action.Task     { return &updateDSPRoleRunTask{} }

// --- Prepare task: scan and report roles needing the fix ---

type updateDSPRolePrepareTask struct{}

func (t *updateDSPRolePrepareTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	return t.Execute(ctx, target)
}

func (t *updateDSPRolePrepareTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	_, _ = findRolesNeedingFix(ctx, target.Client, recorder)

	return recorder.Build(), nil
}

// --- Run task: patch roles ---

type updateDSPRoleRunTask struct{}

func (t *updateDSPRoleRunTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	_, _ = findRolesNeedingFix(ctx, target.Client, recorder)

	return recorder.Build(), nil
}

func (t *updateDSPRoleRunTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)

	roles, err := findRolesNeedingFix(ctx, target.Client, recorder)
	if err != nil {
		return recorder.Build(), fmt.Errorf("scanning roles: %w", err)
	}

	if len(roles) == 0 {
		return recorder.Build(), nil
	}

	patchStep := recorder.Child("patch-roles", "Patch custom roles")

	for _, role := range roles {
		t.patchRole(ctx, target, patchStep, role)
	}

	patchStep.Complete(result.StepCompleted, "Processed %d role(s)", len(roles))

	return recorder.Build(), nil
}

func (t *updateDSPRoleRunTask) patchRole(
	ctx context.Context,
	target action.Target,
	recorder action.StepRecorder,
	role roleClassification,
) {
	step := recorder.Child(
		"patch-"+role.Namespace+"-"+role.RoleName,
		fmt.Sprintf("Patch role %s/%s", role.Namespace, role.RoleName),
	)

	namespaceDSPAs, err := listDSPAsInNamespace(ctx, target.Client, resources.DataSciencePipelinesApplicationV1, role.Namespace)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to list DSPAs in %s: %v", role.Namespace, err)

		return
	}

	if len(namespaceDSPAs) == 0 {
		step.Complete(result.StepCompleted, "No DSPAs found in namespace %s — skipping", role.Namespace)

		return
	}

	for _, dspa := range namespaceDSPAs {
		t.patchRoleForDSPA(ctx, target, step, role, dspa)
	}

	step.Complete(result.StepCompleted, "Role %s/%s processed", role.Namespace, role.RoleName)
}

func (t *updateDSPRoleRunTask) patchRoleForDSPA(
	ctx context.Context,
	target action.Target,
	recorder action.StepRecorder,
	role roleClassification,
	dspa dspaInfo,
) {
	stepID := "add-rule-" + dspa.Name
	step := recorder.Child(stepID,
		"Add datasciencepipelinesapplications/api rule for DSPA "+dspa.Name)

	// Infer verbs from the existing route.openshift.io rule
	verbs := role.RouteVerbs
	if len(verbs) == 0 {
		verbs = []string{"get", "list", "watch"}
	}

	// Build the JSON patch
	patchOps := []map[string]any{
		{
			"op":   "add",
			"path": "/rules/-",
			"value": map[string]any{
				"apiGroups":     []string{dspaAPIGroup},
				"resources":     []string{"datasciencepipelinesapplications/api"},
				"resourceNames": []string{dspa.Name},
				"verbs":         verbs,
			},
		},
	}

	patchData, err := json.Marshal(patchOps)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to marshal patch: %v", err)

		return
	}

	if !target.DryRun && !target.SkipConfirm {
		target.IO.Errorf("\nAbout to patch role %s/%s to add datasciencepipelinesapplications/api for DSPA %s (verbs: %v)\n",
			role.Namespace, role.RoleName, dspa.Name, verbs)

		if !confirmation.Prompt(target.IO, "Proceed with role patch?") {
			step.Complete(result.StepSkipped, "User cancelled patch")

			return
		}
	}

	patchOpts := metav1.PatchOptions{}
	if target.DryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	_, err = target.Client.Dynamic().Resource(resources.Role.GVR()).
		Namespace(role.Namespace).
		Patch(ctx, role.RoleName, types.JSONPatchType, patchData, patchOpts)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to patch role: %v", err)

		return
	}

	if target.DryRun {
		step.Complete(result.StepSkipped,
			"Would add datasciencepipelinesapplications/api rule for DSPA %s (verbs: %v)", dspa.Name, verbs)

		return
	}

	// Validate: re-read the role and confirm the rule was added
	updatedRole, err := target.Client.Dynamic().Resource(resources.Role.GVR()).
		Namespace(role.Namespace).
		Get(ctx, role.RoleName, metav1.GetOptions{})
	if err != nil {
		step.Complete(result.StepFailed, "Patch succeeded but validation failed — could not re-read role: %v", err)

		return
	}

	validated := classifyRole(updatedRole)
	if validated.NeedsFix {
		step.Complete(result.StepFailed,
			"Patch succeeded but validation failed — datasciencepipelinesapplications/api rule not found in re-read")

		return
	}

	step.Complete(result.StepCompleted,
		"Added datasciencepipelinesapplications/api rule for DSPA %s (verbs: %v)", dspa.Name, verbs)
}
