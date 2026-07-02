package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	actionID          = "workbenches.cleanup-oauth"
	actionName        = "Clean up legacy OAuth resources from workbenches"
	actionDescription = "Removes stale OAuth-proxy resources (Route, Service, Secrets, OAuthClient) " +
		"left behind after migrating workbenches from 2.x to 3.x"

	annotationInjectAuth  = "notebooks.opendatahub.io/inject-auth"
	annotationInjectOAuth = "notebooks.opendatahub.io/inject-oauth"

	containerKubeRBACProxy = "kube-rbac-proxy"
	containerOAuthProxy    = "oauth-proxy"

	envNotebookArgs       = "NOTEBOOK_ARGS"
	tornadoSettingsPrefix = "--ServerApp.tornado_settings="

	minTargetMajorVersion = 3
)

// CleanupOAuthAction implements the workbenches.cleanup-oauth migration action.
// It removes legacy OAuth-proxy resources (Route, Service, Secrets, OAuthClient)
// for notebooks that have been migrated to kube-rbac-proxy.
type CleanupOAuthAction struct {
	Scope *workbenches.SharedScopeOptions
}

func (a *CleanupOAuthAction) ID() string          { return actionID }
func (a *CleanupOAuthAction) Name() string        { return actionName }
func (a *CleanupOAuthAction) Description() string { return actionDescription }

func (a *CleanupOAuthAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *CleanupOAuthAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *CleanupOAuthAction) AddFlags(fs *pflag.FlagSet) {
	workbenches.AddScopeFlags(a.Scope, fs)
}

func (a *CleanupOAuthAction) CanApply(target action.Target) bool {
	if target.TargetVersion == nil {
		return false
	}

	return target.TargetVersion.Major >= minTargetMajorVersion
}

func (a *CleanupOAuthAction) Prepare() action.Task {
	return nil
}

func (a *CleanupOAuthAction) Run() action.Task {
	return &runTask{action: a}
}

// checkMigrationState verifies that a notebook has been successfully migrated
// from OAuth-proxy to kube-rbac-proxy. Returns true if all checks pass, along
// with a list of failure messages for any checks that did not pass.
func checkMigrationState(nb *unstructured.Unstructured) (bool, []string) {
	var failures []string

	annotations := nb.GetAnnotations()

	if annotations[annotationInjectAuth] != "true" {
		failures = append(failures,
			fmt.Sprintf("inject-auth annotation missing or not 'true' (found: %q)",
				annotations[annotationInjectAuth]))
	}

	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		failures = append(failures, fmt.Sprintf("could not read containers: %v", err))

		return false, failures
	}

	hasKubeRBACProxy := false
	hasOAuthProxy := false

	for _, raw := range containers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := containerMap["name"].(string)

		switch name {
		case containerKubeRBACProxy:
			hasKubeRBACProxy = true
		case containerOAuthProxy:
			hasOAuthProxy = true
		}
	}

	if !hasKubeRBACProxy {
		failures = append(failures, "kube-rbac-proxy sidecar container missing")
	}

	if hasOAuthProxy {
		failures = append(failures, "legacy oauth-proxy sidecar still present")
	}

	if injectOAuth, exists := annotations[annotationInjectOAuth]; exists {
		if !hasKubeRBACProxy || hasOAuthProxy {
			failures = append(failures,
				fmt.Sprintf("legacy inject-oauth annotation still exists: %q", injectOAuth))
		}
	}

	if hasTornadoSettings(containers) {
		failures = append(failures,
			"--ServerApp.tornado_settings still present in NOTEBOOK_ARGS")
	}

	return len(failures) == 0, failures
}

func hasTornadoSettings(containers []any) bool {
	for _, raw := range containers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		envVars, ok := containerMap["env"].([]any)
		if !ok {
			continue
		}

		for _, envRaw := range envVars {
			envMap, ok := envRaw.(map[string]any)
			if !ok {
				continue
			}

			name, _ := envMap["name"].(string)
			value, _ := envMap["value"].(string)

			if name == envNotebookArgs && strings.Contains(value, tornadoSettingsPrefix) {
				return true
			}
		}
	}

	return false
}

// deleteResourceIfPresent performs an idempotent deletion: if the resource
// exists it is deleted; if it is already absent, no error is raised.
// For cluster-scoped resources, pass an empty namespace.
// Returns true if the resource was deleted or already absent; false on error.
func deleteResourceIfPresent(
	ctx context.Context,
	target action.Target,
	gvr schema.GroupVersionResource,
	name string,
	namespace string,
	step action.StepRecorder,
) bool {
	label := resourceLabel(gvr, name, namespace)
	stepID := fmt.Sprintf("delete-%s-%s", gvr.Resource, name)

	if target.DryRun {
		step.Recordf(stepID, "Would delete %s", result.StepSkipped, label)

		return true
	}

	ri := target.Client.Dynamic().Resource(gvr)

	var err error
	if namespace != "" {
		err = ri.Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	} else {
		err = ri.Delete(ctx, name, metav1.DeleteOptions{})
	}

	if apierrors.IsNotFound(err) {
		step.Recordf(stepID, "Already absent: %s", result.StepCompleted, label)

		return true
	}

	if err != nil {
		step.Recordf(stepID, "Failed to delete %s: %v", result.StepFailed, label, err)

		return false
	}

	step.Recordf(stepID, "Deleted %s", result.StepCompleted, label)

	return true
}

func resourceLabel(gvr schema.GroupVersionResource, name, namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s/%s in namespace %q", gvr.Resource, name, namespace)
	}

	return fmt.Sprintf("%s/%s (cluster-scoped)", gvr.Resource, name)
}

func (a *CleanupOAuthAction) promptCleanupContinueOrSkip(
	target action.Target,
	name string,
	namespace string,
	failures []string,
) bool {
	if target.SkipConfirm {
		return true
	}

	target.IO.Fprintln()
	target.IO.Errorf("Pre-checks failed for %s/%s:", namespace, name)

	for _, f := range failures {
		target.IO.Errorf("  - %s", f)
	}

	return confirmation.Prompt(target.IO,
		fmt.Sprintf("Continue cleanup for %s/%s despite failed pre-checks?", namespace, name))
}

func (a *CleanupOAuthAction) promptBeforeCleanup(
	target action.Target,
	count int,
) bool {
	if target.SkipConfirm {
		return true
	}

	target.IO.Fprintln()
	target.IO.Errorf("About to delete legacy OAuth resources for %d Notebook(s)", count)

	return confirmation.Prompt(target.IO, "Proceed with OAuth resource cleanup?")
}
