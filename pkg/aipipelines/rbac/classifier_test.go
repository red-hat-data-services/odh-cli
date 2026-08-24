package rbac_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/aipipelines/rbac"

	. "github.com/onsi/gomega"
)

func makeRoleUnstructured(name, namespace string, rules []map[string]any) unstructured.Unstructured {
	rulesAny := make([]any, len(rules))
	for i, r := range rules {
		rulesAny[i] = r
	}

	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"rules": rulesAny,
		},
	}
}

func TestClassifyRole(t *testing.T) {
	t.Run("role needing fix — has route.openshift.io but missing dspa subresource", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get", "list"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeTrue())
		g.Expect(result.RoleName).To(Equal("my-custom-role"))
		g.Expect(result.Namespace).To(Equal("user-ns"))
		g.Expect(result.RouteVerbs).To(Equal([]string{"get", "list"}))
	})

	t.Run("role needing fix — infers verbs including create and update", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get", "list", "create", "update", "delete"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeTrue())
		g.Expect(result.RouteVerbs).To(Equal([]string{"get", "list", "create", "update", "delete"}))
	})

	t.Run("role with dspa resource under unrelated api group in same rule — still needs fix", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
			{
				"apiGroups": []any{"example.invalid"},
				"resources": []any{"datasciencepipelinesapplications/api"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeTrue())
	})

	t.Run("role with wildcard dspa grant — no fix needed", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
			{
				"apiGroups": []any{"*"},
				"resources": []any{"datasciencepipelinesapplications/api"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("role with wildcard dspa resource — no fix needed", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
			{
				"apiGroups": []any{"datasciencepipelinesapplications.opendatahub.io"},
				"resources": []any{"*"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("role already has dspa subresource — no fix needed", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
			{
				"apiGroups": []any{"datasciencepipelinesapplications.opendatahub.io"},
				"resources": []any{"datasciencepipelinesapplications/api"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("role without route.openshift.io — no fix needed", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{""},
				"resources": []any{"pods"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("operator-managed role is excluded — ds-pipeline prefix", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("ds-pipeline-mydspa", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("operator-managed role is excluded — pipeline-runner prefix", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("pipeline-runner-mydspa", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("system namespace is excluded", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("my-role", "kube-system", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
		})

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("role with no rules", func(t *testing.T) {
		g := NewWithT(t)

		role := makeRoleUnstructured("empty-role", "user-ns", nil)

		result := rbac.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})
}

func TestIsSystemNamespace(t *testing.T) {
	tests := []struct {
		ns       string
		expected bool
	}{
		{"kube-system", true},
		{"default", true},
		{"openshift", true},
		{"openshift-operators", true},
		{"openshift-monitoring", true},
		{"redhat-ods-applications", true},
		{"redhat-ods-monitoring", true},
		{"user-namespace", false},
		{"my-project", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ns, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(rbac.IsSystemNamespace(tt.ns)).To(Equal(tt.expected))
		})
	}
}

func TestExtractStringSlice(t *testing.T) {
	t.Run("extracts string slice", func(t *testing.T) {
		g := NewWithT(t)

		obj := map[string]any{
			"verbs": []any{"get", "list", "watch"},
		}

		result, ok := rbac.ExtractStringSlice(obj, "verbs")
		g.Expect(ok).To(BeTrue())
		g.Expect(result).To(Equal([]string{"get", "list", "watch"}))
	})

	t.Run("key not found", func(t *testing.T) {
		g := NewWithT(t)

		obj := map[string]any{}

		_, ok := rbac.ExtractStringSlice(obj, "verbs")
		g.Expect(ok).To(BeFalse())
	})

	t.Run("not a slice", func(t *testing.T) {
		g := NewWithT(t)

		obj := map[string]any{
			"verbs": "get",
		}

		_, ok := rbac.ExtractStringSlice(obj, "verbs")
		g.Expect(ok).To(BeFalse())
	})
}
