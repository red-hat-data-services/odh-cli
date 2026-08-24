package rbac

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	systemNamespacePattern = `^(kube-system|default|openshift.*|redhat-ods-.*)$`

	dspaAPIGroup       = "datasciencepipelinesapplications.opendatahub.io"
	dspaAPISubresource = "datasciencepipelinesapplications/api"
	routeAPIGroup      = "route.openshift.io"
)

var (
	systemNamespaceRe   = regexp.MustCompile(systemNamespacePattern)
	operatorManagedRole = regexp.MustCompile(`^(ds-pipeline-|pipeline-runner-)`)
)

// Classification describes whether a Role needs the datasciencepipelinesapplications/api subresource.
type Classification struct {
	NeedsFix   bool
	RoleName   string
	Namespace  string
	RouteVerbs []string // verbs from the route.openshift.io rule, used to infer DSPA verbs
}

// ClassifyRole determines whether a Role grants route.openshift.io permissions without the
// datasciencepipelinesapplications/api subresource required after the kube-rbac-proxy migration.
func ClassifyRole(role *unstructured.Unstructured) Classification {
	classification := Classification{
		RoleName:  role.GetName(),
		Namespace: role.GetNamespace(),
	}

	if IsSystemNamespace(classification.Namespace) {
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

		apiGroups, _ := ExtractStringSlice(rule, "apiGroups")
		ruleResources, _ := ExtractStringSlice(rule, "resources")
		verbs, _ := ExtractStringSlice(rule, "verbs")

		if containsStringOrWildcard(apiGroups, routeAPIGroup) {
			hasRouteAPIGroup = true
			classification.RouteVerbs = mergeStringSlices(classification.RouteVerbs, verbs)
		}

		if containsStringOrWildcard(apiGroups, dspaAPIGroup) &&
			containsStringOrWildcard(ruleResources, dspaAPISubresource) {
			hasDSPASubresource = true
		}
	}

	classification.NeedsFix = hasRouteAPIGroup && !hasDSPASubresource

	return classification
}

// IsSystemNamespace reports whether the namespace is managed by the platform or ODH.
func IsSystemNamespace(ns string) bool {
	return systemNamespaceRe.MatchString(ns)
}

// ExtractStringSlice reads a string slice from an unstructured rule field.
func ExtractStringSlice(obj map[string]any, key string) ([]string, bool) {
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

func containsStringOrWildcard(values []string, target string) bool {
	for _, v := range values {
		if v == target || v == "*" {
			return true
		}
	}

	return false
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
