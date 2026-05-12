package status_test

import (
	"testing"

	"github.com/fatih/color"

	"github.com/opendatahub-io/odh-cli/pkg/status"

	. "github.com/onsi/gomega"
)

func TestIsRBACError(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"empty string", "", false},
		{"generic error", "connection refused", false},
		{"forbidden lowercase", "forbidden: cannot list nodes", true},
		{"Forbidden capitalized", "Forbidden: access denied", true},
		{"FORBIDDEN uppercase", "FORBIDDEN", true},
		{"unauthorized", "unauthorized: invalid token", true},
		{"cannot list", "cannot list resource pods", true},
		{"cannot get", "cannot get resource deployments", true},
		{"mixed case forbidden", "User is Forbidden from accessing", true},
		{"unhealthy error", "unhealthy nodes: node-1 (MemoryPressure)", false},
		{"deployment error", "deployments not ready: 2/3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(status.IsRBACError(tt.errStr)).To(Equal(tt.expected))
		})
	}
}

func TestStatusSymbol(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	tests := []struct {
		name     string
		errStr   string
		expected string
	}{
		{"no error shows pass", "", "✓"},
		{"rbac error shows unknown", "forbidden: cannot list", "?"},
		{"health error shows fail", "unhealthy nodes", "✗"},
		{"generic error shows fail", "connection timeout", "✗"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(status.StatusSymbol(tt.errStr)).To(Equal(tt.expected))
		})
	}
}

func TestVisibleLen(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"plain text", "hello", 5},
		{"empty", "", 0},
		{"with ANSI codes", "\x1b[32m✓\x1b[0m", 1},
		{"unicode", "✓✗?", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(status.VisibleLen(tt.input)).To(Equal(tt.expected))
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{"shorter than width", "foo", 6, "foo   "},
		{"equal to width", "foobar", 6, "foobar"},
		{"longer than width", "foobar", 3, "foobar"},
		{"empty string", "", 3, "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(status.PadRight(tt.input, tt.width)).To(Equal(tt.expected))
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"equal to max", "hello", 5, "hello"},
		{"longer than max", "hello world", 8, "hello..."},
		{"with whitespace", "  hello  ", 10, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(status.Truncate(tt.input, tt.maxLen)).To(Equal(tt.expected))
		})
	}
}
