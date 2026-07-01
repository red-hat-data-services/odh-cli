package status_test

import (
	"testing"
	"time"

	cmdpkg "github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/status"
)

func TestCommandValidate_OutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  status.OutputFormat
		wantErr bool
	}{
		{"table format", status.OutputFormatTable, false},
		{"json format", status.OutputFormatJSON, false},
		{"yaml format", status.OutputFormatYAML, false},
		{"invalid format", status.OutputFormat("xml"), true},
		{"empty format", status.OutputFormat(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: tt.format,
				Timeout:      30 * time.Second,
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with format %q error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_Sections(t *testing.T) {
	tests := []struct {
		name     string
		sections []string
		wantErr  bool
	}{
		{"nil sections", nil, false},
		{"empty sections", []string{}, false},
		{"single valid", []string{"nodes"}, false},
		{"multiple valid", []string{"nodes", "pods", "deployments"}, false},
		{"all valid", []string{"nodes", "deployments", "pods", "events", "quotas", "operator", "dsci", "dsc"}, false},
		{"single invalid", []string{"invalid"}, true},
		{"mixed valid and invalid", []string{"nodes", "invalid"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: status.OutputFormatTable,
				Timeout:      30 * time.Second,
				Sections:     tt.sections,
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with sections %v error = %v, wantErr %v", tt.sections, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_Timeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		waitFor string
		wantErr bool
	}{
		{"positive timeout", 30 * time.Second, "", false},
		{"zero timeout without wait-for", 0, "", true},
		{"negative timeout without wait-for", -1 * time.Second, "", true},
		{"zero timeout with wait-for (means no timeout)", 0, "healthy", false},
		{"positive timeout with wait-for", 300 * time.Second, "healthy", false},
		{"negative timeout with wait-for", -1 * time.Second, "healthy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: status.OutputFormatTable,
				Timeout:      tt.timeout,
				WaitOptions:  cmdpkg.WaitOptions{WaitFor: tt.waitFor, PollInterval: cmdpkg.DefaultPollInterval},
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with timeout %v, waitFor %q error = %v, wantErr %v", tt.timeout, tt.waitFor, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_WaitFor(t *testing.T) {
	tests := []struct {
		name    string
		waitFor string
		wantErr bool
	}{
		{"empty (no wait)", "", false},
		{"valid healthy", "healthy", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: status.OutputFormatTable,
				Timeout:      30 * time.Second,
				WaitOptions:  cmdpkg.WaitOptions{WaitFor: tt.waitFor, PollInterval: cmdpkg.DefaultPollInterval},
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with waitFor %q error = %v, wantErr %v", tt.waitFor, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_PollInterval(t *testing.T) {
	tests := []struct {
		name         string
		pollInterval time.Duration
		waitFor      string
		wantErr      bool
	}{
		{"positive interval with wait-for", 5 * time.Second, "healthy", false},
		{"zero interval with wait-for", 0, "healthy", true},
		{"negative interval with wait-for", -1 * time.Second, "healthy", true},
		{"zero interval without wait-for (ignored)", 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: status.OutputFormatTable,
				Timeout:      30 * time.Second,
				WaitOptions:  cmdpkg.WaitOptions{WaitFor: tt.waitFor, PollInterval: tt.pollInterval},
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with pollInterval %v, waitFor %q error = %v, wantErr %v", tt.pollInterval, tt.waitFor, err, tt.wantErr)
			}
		})
	}
}

func TestCommandValidate_Layers(t *testing.T) {
	tests := []struct {
		name    string
		layers  []string
		wantErr bool
	}{
		{"nil layers", nil, false},
		{"empty layers", []string{}, false},
		{"infrastructure layer", []string{"infrastructure"}, false},
		{"workload layer", []string{"workload"}, false},
		{"operator layer", []string{"operator"}, false},
		{"all layers", []string{"infrastructure", "workload", "operator"}, false},
		{"invalid layer", []string{"invalid"}, true},
		{"mixed valid and invalid", []string{"infrastructure", "invalid"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: status.OutputFormatTable,
				Timeout:      30 * time.Second,
				Layers:       tt.layers,
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with layers %v error = %v, wantErr %v", tt.layers, err, tt.wantErr)
			}
		})
	}
}
