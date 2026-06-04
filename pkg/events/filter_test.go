//nolint:testpackage // Tests internal implementation (unexported functions)
package events

import (
	"testing"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
)

func TestGetTargetNamespaces(t *testing.T) {
	tests := []struct {
		name          string
		cmd           *Command
		wantNamespace []string
		wantErr       bool
	}{
		{
			name: "all namespaces with ODH namespaces",
			cmd: &Command{
				AllNamespaces:  true,
				ApplicationsNS: "redhat-ods-applications",
				OperatorNS:     "redhat-ods-operator",
				MonitoringNS:   "redhat-ods-monitoring",
			},
			wantNamespace: []string{
				"redhat-ods-applications",
				"redhat-ods-operator",
				"redhat-ods-monitoring",
			},
		},
		{
			name: "explicit -n flag is exclusive",
			cmd: &Command{
				NamespaceExplicit: true,
				Namespace:         "my-project",
				ApplicationsNS:    "redhat-ods-applications",
				OperatorNS:        "redhat-ods-operator",
				MonitoringNS:      "redhat-ods-applications",
			},
			wantNamespace: []string{
				"my-project",
			},
		},
		{
			name: "no flags uses ODH namespaces (not kubeconfig namespace)",
			cmd: &Command{
				NamespaceExplicit: false,
				Namespace:         "default", // from kubeconfig, should be ignored
				ApplicationsNS:    "redhat-ods-applications",
				OperatorNS:        "redhat-ods-operator",
				MonitoringNS:      "redhat-ods-monitoring",
			},
			wantNamespace: []string{
				"redhat-ods-applications",
				"redhat-ods-operator",
				"redhat-ods-monitoring",
			},
		},
		{
			name: "deduplicates ODH namespaces",
			cmd: &Command{
				AllNamespaces:  true,
				ApplicationsNS: "redhat-ods-applications",
				OperatorNS:     "redhat-ods-operator",
				MonitoringNS:   "redhat-ods-applications",
			},
			wantNamespace: []string{
				"redhat-ods-applications",
				"redhat-ods-operator",
			},
		},
		{
			name: "empty namespaces returns error",
			cmd: &Command{
				AllNamespaces: true,
			},
			wantErr: true,
		},
		{
			name: "skips empty namespace strings",
			cmd: &Command{
				AllNamespaces:  true,
				ApplicationsNS: "",
				OperatorNS:     "redhat-ods-operator",
				MonitoringNS:   "",
			},
			wantNamespace: []string{"redhat-ods-operator"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.cmd.getTargetNamespaces()

			if tt.wantErr {
				if err == nil {
					t.Error("getTargetNamespaces() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("getTargetNamespaces() unexpected error: %v", err)

				return
			}

			if len(result) != len(tt.wantNamespace) {
				t.Errorf("getTargetNamespaces() returned %d namespaces, expected %d\nGot: %v\nExpected: %v",
					len(result), len(tt.wantNamespace), result, tt.wantNamespace)

				return
			}

			resultSet := make(map[string]bool)
			for _, ns := range result {
				resultSet[ns] = true
			}

			for _, want := range tt.wantNamespace {
				if !resultSet[want] {
					t.Errorf("getTargetNamespaces() missing namespace %q\nGot: %v", want, result)
				}
			}
		})
	}
}

func TestSortEventsByTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		events []clusterhealth.EventInfo
		want   []string
	}{
		{
			name:   "empty events",
			events: []clusterhealth.EventInfo{},
			want:   []string{},
		},
		{
			name: "single event",
			events: []clusterhealth.EventInfo{
				{Name: "event1", LastTime: now},
			},
			want: []string{"event1"},
		},
		{
			name: "multiple events sorted by time descending",
			events: []clusterhealth.EventInfo{
				{Name: "old", LastTime: now.Add(-1 * time.Hour)},
				{Name: "newest", LastTime: now},
				{Name: "middle", LastTime: now.Add(-30 * time.Minute)},
			},
			want: []string{"newest", "middle", "old"},
		},
		{
			name: "events with zero time",
			events: []clusterhealth.EventInfo{
				{Name: "no-time", LastTime: time.Time{}},
				{Name: "has-time", LastTime: now},
			},
			want: []string{"has-time", "no-time"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortEventsByTime(tt.events)

			if len(tt.events) != len(tt.want) {
				t.Errorf("sortEventsByTime() resulted in %d events, expected %d",
					len(tt.events), len(tt.want))

				return
			}

			for i, event := range tt.events {
				if event.Name != tt.want[i] {
					t.Errorf("sortEventsByTime()[%d] = %q, expected %q",
						i, event.Name, tt.want[i])
				}
			}
		})
	}
}
