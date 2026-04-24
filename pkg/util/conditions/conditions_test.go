package conditions_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/util/conditions"
)

func TestFindCondition(t *testing.T) {
	tests := []struct {
		name          string
		conditions    []metav1.Condition
		conditionType string
		wantFound     bool
		wantStatus    metav1.ConditionStatus
	}{
		{
			name: "finds existing condition",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
				{Type: "Degraded", Status: metav1.ConditionFalse},
			},
			conditionType: "Ready",
			wantFound:     true,
			wantStatus:    metav1.ConditionTrue,
		},
		{
			name: "returns nil for missing condition",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
			conditionType: "Degraded",
			wantFound:     false,
		},
		{
			name:          "handles empty conditions",
			conditions:    []metav1.Condition{},
			conditionType: "Ready",
			wantFound:     false,
		},
		{
			name:          "handles nil conditions",
			conditions:    nil,
			conditionType: "Ready",
			wantFound:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conditions.FindCondition(tt.conditions, tt.conditionType)

			if tt.wantFound {
				if got == nil {
					t.Fatalf("expected condition, got nil")
				}

				if got.Status != tt.wantStatus {
					t.Errorf("status = %v, want %v", got.Status, tt.wantStatus)
				}
			} else {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
			}
		})
	}
}

func TestFindReady(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Available", Status: metav1.ConditionTrue},
		{Type: "Ready", Status: metav1.ConditionFalse, Message: "not ready"},
	}

	got := conditions.FindReady(conds)
	if got == nil {
		t.Fatal("expected Ready condition, got nil")
	}

	if got.Status != metav1.ConditionFalse {
		t.Errorf("status = %v, want False", got.Status)
	}
}

func TestIsTrue(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Degraded", Status: metav1.ConditionFalse},
	}

	if !conditions.IsTrue(conds, "Ready") {
		t.Error("expected Ready to be true")
	}

	if conditions.IsTrue(conds, "Degraded") {
		t.Error("expected Degraded to not be true")
	}

	if conditions.IsTrue(conds, "Missing") {
		t.Error("expected Missing to not be true")
	}
}

func TestIsFalse(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Degraded", Status: metav1.ConditionFalse},
	}

	if conditions.IsFalse(conds, "Ready") {
		t.Error("expected Ready to not be false")
	}

	if !conditions.IsFalse(conds, "Degraded") {
		t.Error("expected Degraded to be false")
	}

	if conditions.IsFalse(conds, "Missing") {
		t.Error("expected Missing to not be false")
	}
}

func TestCollectMessages(t *testing.T) {
	tests := []struct {
		name           string
		conditions     []metav1.Condition
		conditionTypes []string
		want           string
	}{
		{
			name: "collects messages from matching conditions",
			conditions: []metav1.Condition{
				{Type: "Degraded", Status: metav1.ConditionTrue, Message: "error 1"},
				{Type: "Degraded", Status: metav1.ConditionTrue, Message: "error 2"},
				{Type: "Ready", Status: metav1.ConditionFalse, Message: "not ready"},
			},
			conditionTypes: []string{"Degraded"},
			want:           "error 1; error 2",
		},
		{
			name: "ignores conditions with status False",
			conditions: []metav1.Condition{
				{Type: "Degraded", Status: metav1.ConditionFalse, Message: "should ignore"},
				{Type: "Degraded", Status: metav1.ConditionTrue, Message: "real error"},
			},
			conditionTypes: []string{"Degraded"},
			want:           "real error",
		},
		{
			name: "ignores empty messages",
			conditions: []metav1.Condition{
				{Type: "Degraded", Status: metav1.ConditionTrue, Message: ""},
				{Type: "Degraded", Status: metav1.ConditionTrue, Message: "has message"},
			},
			conditionTypes: []string{"Degraded"},
			want:           "has message",
		},
		{
			name: "collects from multiple condition types",
			conditions: []metav1.Condition{
				{Type: "Error", Status: metav1.ConditionTrue, Message: "error msg"},
				{Type: "Warning", Status: metav1.ConditionTrue, Message: "warning msg"},
			},
			conditionTypes: []string{"Error", "Warning"},
			want:           "error msg; warning msg",
		},
		{
			name:           "returns empty string for no matches",
			conditions:     []metav1.Condition{},
			conditionTypes: []string{"Degraded"},
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conditions.CollectMessages(tt.conditions, tt.conditionTypes...)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectDegradedMessages(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Degraded", Status: metav1.ConditionTrue, Message: "component failing"},
		{Type: "Ready", Status: metav1.ConditionFalse, Message: "not ready"},
	}

	got := conditions.CollectDegradedMessages(conds)
	want := "component failing"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
