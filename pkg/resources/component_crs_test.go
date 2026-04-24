package resources_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

func TestComponentCRResourceTypes(t *testing.T) {
	expectedComponents := []string{
		"aipipelines",
		"dashboard",
		"feastoperator",
		"kserve",
		"llamastackoperator",
		"mlflowoperator",
		"modelregistry",
		"ray",
		"sparkoperator",
		"trainer",
		"trainingoperator",
		"trustyai",
		"workbenches",
	}

	for _, name := range expectedComponents {
		t.Run(name, func(t *testing.T) {
			rt, ok := resources.ComponentCRResourceTypes[name]
			if !ok {
				t.Fatalf("component %q not found in ComponentCRResourceTypes", name)
			}

			if rt.Group != "components.platform.opendatahub.io" {
				t.Errorf("group = %q, want components.platform.opendatahub.io", rt.Group)
			}

			if rt.Version != "v1alpha1" {
				t.Errorf("version = %q, want v1alpha1", rt.Version)
			}

			if rt.Kind == "" {
				t.Error("kind should not be empty")
			}

			if rt.Resource == "" {
				t.Error("resource should not be empty")
			}
		})
	}
}

func TestGetComponentCR(t *testing.T) {
	tests := []struct {
		name      string
		component string
		wantNil   bool
		wantKind  string
	}{
		{
			name:      "returns ResourceType for known component",
			component: "kserve",
			wantNil:   false,
			wantKind:  "Kserve",
		},
		{
			name:      "returns ResourceType for dashboard",
			component: "dashboard",
			wantNil:   false,
			wantKind:  "Dashboard",
		},
		{
			name:      "returns nil for unknown component",
			component: "unknown",
			wantNil:   true,
		},
		{
			name:      "returns nil for empty string",
			component: "",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resources.GetComponentCR(tt.component)

			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}

				return
			}

			if got == nil {
				t.Fatal("expected non-nil ResourceType")
			}

			if got.Kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tt.wantKind)
			}
		})
	}
}
