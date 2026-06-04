//nolint:testpackage // Tests internal implementation (unexported functions)
package events

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
)

func TestFormatAge(t *testing.T) {
	// Use a fixed reference time for deterministic tests (no clock drift).
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"zero time", time.Time{}, "<unknown>"},
		{"25 seconds ago", now.Add(-25 * time.Second), "25s"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5m"},
		{"2 hours ago", now.Add(-2 * time.Hour), "2h"},
		{"3 days ago", now.Add(-3 * 24 * time.Hour), "3d"},
		{"future time", now.Add(1 * time.Hour), "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAgeFrom(tt.time, now)
			if result != tt.expected {
				t.Errorf("formatAgeFrom() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{"short message", "Hello", "Hello"},
		{"exact length", strings.Repeat("a", 80), strings.Repeat("a", 80)},
		{"long message", strings.Repeat("a", 100), strings.Repeat("a", 77) + "..."},
		{"empty message", "", ""},
		{"multibyte runes short", "café résumé", "café résumé"},
		{"multibyte runes long", strings.Repeat("é", 100), strings.Repeat("é", 77) + "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateMessage(tt.message)
			if result != tt.expected {
				t.Errorf("truncateMessage() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeForTerminal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello world", "hello world"},
		{"with tab", "hello\tworld", "hello\tworld"},
		{"ANSI escape", "hello\x1b[31mred\x1b[0m", "hellored"},
		{"newline", "hello\nworld", "helloworld"},
		{"carriage return", "hello\rworld", "helloworld"},
		{"null byte", "hello\x00world", "helloworld"},
		{"bell", "hello\x07world", "helloworld"},
		{"mixed control chars", "\x1b[2J\x1b[HMalicious", "Malicious"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForTerminal(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeForTerminal() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestOutputTable(t *testing.T) {
	tests := []struct {
		name          string
		events        []clusterhealth.EventInfo
		showNamespace bool
		wantContains  []string
	}{
		{
			name:          "empty events",
			events:        []clusterhealth.EventInfo{},
			showNamespace: false,
			wantContains:  []string{msgNoEventsFound},
		},
		{
			name: "single event",
			events: []clusterhealth.EventInfo{
				{
					Kind:    "Pod",
					Name:    "test-pod",
					Type:    "Warning",
					Reason:  "BackOff",
					Message: "Back-off pulling image",
					Count:   3,
				},
			},
			showNamespace: false,
			wantContains: []string{
				"LAST SEEN",
				"TYPE",
				"REASON",
				"OBJECT",
				"COUNT",
				"MESSAGE",
				"Pod/test-pod",
				"Warning",
				"BackOff",
				"3",
				"Back-off pulling image",
			},
		},
		{
			name: "multiple events",
			events: []clusterhealth.EventInfo{
				{Kind: "Pod", Name: "pod1", Type: "Warning", Reason: "BackOff"},
				{Kind: "Deployment", Name: "deploy1", Type: "Normal", Reason: "Scaled"},
			},
			showNamespace: false,
			wantContains: []string{
				"Pod/pod1",
				"Deployment/deploy1",
			},
		},
		{
			name: "shows namespace column when requested",
			events: []clusterhealth.EventInfo{
				{
					Namespace: "redhat-ods-applications",
					Kind:      "Pod",
					Name:      "test-pod",
					Type:      "Warning",
					Reason:    "BackOff",
					Count:     5,
				},
			},
			showNamespace: true,
			wantContains: []string{
				"NAMESPACE",
				"COUNT",
				"redhat-ods-applications",
				"Pod/test-pod",
				"5",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := outputTable(&buf, tt.events, tt.showNamespace)
			if err != nil {
				t.Fatalf("outputTable() error = %v", err)
			}

			result := buf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("outputTable() output missing %q\nGot: %s", want, result)
				}
			}
		})
	}
}

func TestOutputJSON(t *testing.T) {
	tests := []struct {
		name         string
		events       []clusterhealth.EventInfo
		wantContains []string
	}{
		{
			name:   "empty events",
			events: []clusterhealth.EventInfo{},
			wantContains: []string{
				`"kind": "EventList"`,
				`"items": []`,
			},
		},
		{
			name: "single event returns list",
			events: []clusterhealth.EventInfo{
				{Kind: "Pod", Name: "test-pod", Type: "Warning"},
			},
			wantContains: []string{
				`"kind": "EventList"`,
				`"items":`,
				`"name": "test-pod"`,
			},
		},
		{
			name: "multiple events returns list",
			events: []clusterhealth.EventInfo{
				{Kind: "Pod", Name: "pod1"},
				{Kind: "Pod", Name: "pod2"},
			},
			wantContains: []string{
				`"kind": "EventList"`,
				`"items":`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := outputJSON(&buf, tt.events)
			if err != nil {
				t.Fatalf("outputJSON() error = %v", err)
			}

			result := buf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("outputJSON() output missing %q\nGot: %s", want, result)
				}
			}
		})
	}
}

func TestOutputYAML(t *testing.T) {
	tests := []struct {
		name         string
		events       []clusterhealth.EventInfo
		wantContains []string
	}{
		{
			name:   "empty events",
			events: []clusterhealth.EventInfo{},
			wantContains: []string{
				"kind: EventList",
				"items: []",
			},
		},
		{
			name: "single event returns list",
			events: []clusterhealth.EventInfo{
				{Kind: "Pod", Name: "test-pod"},
			},
			wantContains: []string{
				"kind: EventList",
				"name: test-pod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := outputYAML(&buf, tt.events)
			if err != nil {
				t.Fatalf("outputYAML() error = %v", err)
			}

			result := buf.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("outputYAML() output missing %q\nGot: %s", want, result)
				}
			}
		})
	}
}

func TestToEventOutputList(t *testing.T) {
	tests := []struct {
		name   string
		events []clusterhealth.EventInfo
	}{
		{
			name:   "empty returns list",
			events: []clusterhealth.EventInfo{},
		},
		{
			name: "single event returns list",
			events: []clusterhealth.EventInfo{
				{Kind: "Pod"},
			},
		},
		{
			name: "multiple events returns list",
			events: []clusterhealth.EventInfo{
				{Kind: "Pod"},
				{Kind: "Pod"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toEventOutputList(tt.events)
			m, ok := result.(map[string]any)
			if !ok {
				t.Fatal("toEventOutputList() should always return map[string]any")
			}
			if m["kind"] != "EventList" {
				t.Errorf("toEventOutputList() kind = %v, expected EventList", m["kind"])
			}
		})
	}
}
