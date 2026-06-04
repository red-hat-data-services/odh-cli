package aipipelines

import (
	"context"
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

var (
	systemNamespaceRe   = regexp.MustCompile(systemNamespacePattern)
	operatorManagedRole = regexp.MustCompile(`^(ds-pipeline-|pipeline-runner-)`)
)

type roleClassification struct {
	NeedsFix   bool
	RoleName   string
	Namespace  string
	RouteVerbs []string // verbs from the route.openshift.io rule, used to infer DSPA verbs
}

func classifyRole(role *unstructured.Unstructured) roleClassification {
	classification := roleClassification{
		RoleName:  role.GetName(),
		Namespace: role.GetNamespace(),
	}

	if isSystemNamespace(classification.Namespace) {
		return classification
	}

	if operatorManagedRole.MatchString(classification.RoleName) {
		return classification
	}

	rules, found, _ := unstructured.NestedSlice(role.Object, "rules")
	if !found {
		return classification
	}

	hasRouteAPIGroup := false
	hasDSPASubresource := false

	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			continue
		}

		apiGroups, _ := extractStringSlice(rule, "apiGroups")
		ruleResources, _ := extractStringSlice(rule, "resources")
		verbs, _ := extractStringSlice(rule, "verbs")

		for _, g := range apiGroups {
			if g == "route.openshift.io" {
				hasRouteAPIGroup = true
				classification.RouteVerbs = mergeStringSlices(classification.RouteVerbs, verbs)
			}
		}

		for _, res := range ruleResources {
			if res == "datasciencepipelinesapplications/api" {
				hasDSPASubresource = true
			}
		}
	}

	classification.NeedsFix = hasRouteAPIGroup && !hasDSPASubresource

	return classification
}

func isSystemNamespace(ns string) bool {
	return systemNamespaceRe.MatchString(ns)
}

func findRolesNeedingFix(
	ctx context.Context,
	c client.Client,
	recorder action.StepRecorder,
) ([]roleClassification, error) {
	step := recorder.Child("scan-roles", "Scan custom roles for RBAC gaps")

	roleList, err := c.Dynamic().Resource(resources.Role.GVR()).
		Namespace("").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		step.Complete(result.StepFailed, "Failed to list roles: %v", err)

		return nil, fmt.Errorf("listing roles: %w", err)
	}

	var needsFix []roleClassification

	for i := range roleList.Items {
		classification := classifyRole(&roleList.Items[i])
		if classification.NeedsFix {
			needsFix = append(needsFix, classification)
		}
	}

	if len(needsFix) == 0 {
		step.Complete(result.StepCompleted, "No custom roles need updating")
	} else {
		for _, r := range needsFix {
			step.Record(
				r.RoleName,
				"Role %s in namespace %s needs datasciencepipelinesapplications/api subresource",
				result.StepCompleted,
				r.RoleName,
				r.Namespace,
			)
		}

		step.Complete(result.StepCompleted, "Found %d role(s) needing update", len(needsFix))
	}

	return needsFix, nil
}

func extractStringSlice(obj map[string]any, key string) ([]string, bool) {
	raw, ok := obj[key]
	if !ok {
		return nil, false
	}

	slice, ok := raw.([]any)
	if !ok {
		return nil, false
	}

	out := make([]string, 0, len(slice))

	for _, v := range slice {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}

	return out, len(out) > 0
}

func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]bool, len(a))

	for _, v := range a {
		seen[v] = true
	}

	merged := append([]string{}, a...)

	for _, v := range b {
		if !seen[v] {
			merged = append(merged, v)
			seen[v] = true
		}
	}

	return merged
}
