package status_test

import (
	"testing"
	"time"

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
		wantErr bool
	}{
		{"positive timeout", 30 * time.Second, false},
		{"zero timeout", 0, true},
		{"negative timeout", -1 * time.Second, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &status.Command{
				OutputFormat: status.OutputFormatTable,
				Timeout:      tt.timeout,
			}
			err := cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Validate() with timeout %v error = %v, wantErr %v", tt.timeout, err, tt.wantErr)
			}
		})
	}
}
