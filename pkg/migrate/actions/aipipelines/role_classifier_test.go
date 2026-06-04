package aipipelines_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"

	. "github.com/onsi/gomega"
)

func TestClassifyRole(t *testing.T) {
	t.Run("role needing fix — has route.openshift.io but missing dspa subresource", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get", "list"},
			},
		})

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeTrue())
		g.Expect(result.RoleName).To(Equal("my-custom-role"))
		g.Expect(result.Namespace).To(Equal("user-ns"))
		g.Expect(result.RouteVerbs).To(Equal([]string{"get", "list"}))
	})

	t.Run("role needing fix — infers verbs including create and update", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get", "list", "create", "update", "delete"},
			},
		})

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeTrue())
		g.Expect(result.RouteVerbs).To(Equal([]string{"get", "list", "create", "update", "delete"}))
	})

	t.Run("role already has dspa subresource — no fix needed", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("my-custom-role", "user-ns", []map[string]any{
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

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("role without route.openshift.io — no fix needed", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("my-role", "user-ns", []map[string]any{
			{
				"apiGroups": []any{""},
				"resources": []any{"pods"},
				"verbs":     []any{"get"},
			},
		})

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("operator-managed role is excluded — ds-pipeline prefix", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("ds-pipeline-mydspa", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
		})

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("operator-managed role is excluded — pipeline-runner prefix", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("pipeline-runner-mydspa", "user-ns", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
		})

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("system namespace is excluded", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("my-role", "kube-system", []map[string]any{
			{
				"apiGroups": []any{"route.openshift.io"},
				"resources": []any{"routes"},
				"verbs":     []any{"get"},
			},
		})

		result := aipipelines.ClassifyRole(&role)
		g.Expect(result.NeedsFix).To(BeFalse())
	})

	t.Run("role with no rules", func(t *testing.T) {
		g := NewWithT(t)

		role := aipipelines.MakeRoleUnstructured("empty-role", "user-ns", nil)

		result := aipipelines.ClassifyRole(&role)
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
			g.Expect(aipipelines.IsSystemNamespace(tt.ns)).To(Equal(tt.expected))
		})
	}
}

func TestExtractStringSlice(t *testing.T) {
	t.Run("extracts string slice", func(t *testing.T) {
		g := NewWithT(t)

		obj := map[string]any{
			"verbs": []any{"get", "list", "watch"},
		}

		result, ok := aipipelines.ExtractStringSlice(obj, "verbs")
		g.Expect(ok).To(BeTrue())
		g.Expect(result).To(Equal([]string{"get", "list", "watch"}))
	})

	t.Run("key not found", func(t *testing.T) {
		g := NewWithT(t)

		obj := map[string]any{}

		_, ok := aipipelines.ExtractStringSlice(obj, "verbs")
		g.Expect(ok).To(BeFalse())
	})

	t.Run("not a slice", func(t *testing.T) {
		g := NewWithT(t)

		obj := map[string]any{
			"verbs": "get",
		}

		_, ok := aipipelines.ExtractStringSlice(obj, "verbs")
		g.Expect(ok).To(BeFalse())
	})
}
