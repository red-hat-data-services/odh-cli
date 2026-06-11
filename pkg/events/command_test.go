package events_test

import (
	"slices"
	"testing"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/pkg/events"

	. "github.com/onsi/gomega"
)

func TestCommandValidate_OutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"table format", "table", false},
		{"json format", "json", false},
		{"yaml format", "yaml", false},
		{"invalid format", "xml", true},
		{"empty format", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &events.Command{
				OutputFormat: tt.format,
				ConfigFlags:  genericclioptions.NewConfigFlags(true),
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with format %q error = %v, wantErr %v",
					tt.format, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_EventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		wantErr   bool
	}{
		{"empty type", "", false},
		{"warning type", "Warning", false},
		{"normal type", "Normal", false},
		{"invalid type", "Error", true},
		{"lowercase warning", "warning", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &events.Command{
				OutputFormat: "table",
				EventType:    tt.eventType,
				ConfigFlags:  genericclioptions.NewConfigFlags(true),
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with eventType %q error = %v, wantErr %v",
					tt.eventType, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_Since(t *testing.T) {
	tests := []struct {
		name    string
		since   time.Duration
		wantErr bool
	}{
		{"zero duration", 0, false},
		{"positive duration", 5 * time.Minute, false},
		{"negative duration", -1 * time.Minute, true},
		{"max duration", 24 * time.Hour, false},
		{"exceeds max duration", 25 * time.Hour, true},
		{"way over max", 1000 * time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &events.Command{
				OutputFormat: "table",
				Since:        tt.since,
				ConfigFlags:  genericclioptions.NewConfigFlags(true),
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with since %v error = %v, wantErr %v",
					tt.since, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_AllNamespacesAndNamespace(t *testing.T) {
	ns := "test-namespace"

	tests := []struct {
		name          string
		allNamespaces bool
		namespace     *string
		wantErr       bool
	}{
		{"all namespaces only", true, nil, false},
		{"namespace only", false, &ns, false},
		{"both set", true, &ns, true},
		{"neither set", false, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := genericclioptions.NewConfigFlags(true)
			flags.Namespace = tt.namespace

			cmd := &events.Command{
				OutputFormat:  "table",
				AllNamespaces: tt.allNamespaces,
				ConfigFlags:   flags,
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() allNamespaces=%v, namespace=%v error = %v, wantErr %v",
					tt.allNamespaces, tt.namespace, err, tt.wantErr)
			}
		})
	}
}

func TestNewCommand(t *testing.T) {
	streams := genericclioptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	cmd := events.NewCommand(streams, flags)

	if cmd == nil {
		t.Fatal("NewCommand() returned nil")
	}

	if cmd.OutputFormat != "table" {
		t.Errorf("NewCommand() OutputFormat = %q, expected %q",
			cmd.OutputFormat, "table")
	}

	if cmd.OperatorNS != "redhat-ods-operator" {
		t.Errorf("NewCommand() OperatorNS = %q, expected %q",
			cmd.OperatorNS, "redhat-ods-operator")
	}

	if cmd.ConfigFlags != flags {
		t.Error("NewCommand() ConfigFlags not set correctly")
	}
}

func TestCommandValidate_Component(t *testing.T) {
	tests := []struct {
		name      string
		component string
		wantErr   bool
	}{
		{"empty component", "", false},
		{"valid component kserve", "kserve", false},
		{"valid component dashboard", "dashboard", false},
		{"valid component ray", "ray", false},
		{"invalid component", "invalid-component", true},
		{"typo in component", "kserver", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			cmd := &events.Command{
				OutputFormat: "table",
				Component:    tt.component,
				ConfigFlags:  genericclioptions.NewConfigFlags(true),
			}
			err := cmd.Validate()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidComponents(t *testing.T) {
	g := NewWithT(t)
	components := events.ValidComponents()

	g.Expect(components).ToNot(BeEmpty())

	// Check that known components are present
	expectedComponents := []string{"kserve", "dashboard", "ray", "workbenches"}
	for _, expected := range expectedComponents {
		g.Expect(slices.Contains(components, expected)).To(BeTrue(),
			"ValidComponents() missing expected component %q", expected)
	}

	// Check that list is sorted
	g.Expect(slices.IsSorted(components)).To(BeTrue(), "ValidComponents() should be sorted")
}
