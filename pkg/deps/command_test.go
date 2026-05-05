package deps_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

func TestCommand_Validate_OutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		output  string
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
			g := NewWithT(t)

			streams := genericiooptions.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			}

			cmd := deps.NewCommand(streams, nil)
			cmd.Output = tt.output
			cmd.Refresh = true

			err := cmd.Validate()

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestCommand_Complete_DryRun(t *testing.T) {
	g := NewWithT(t)

	streams := genericiooptions.IOStreams{
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}

	cmd := deps.NewCommand(streams, nil)
	cmd.DryRun = true

	err := cmd.Complete()

	g.Expect(err).ToNot(HaveOccurred())
}

func TestCommand_Complete_OutputSchema(t *testing.T) {
	g := NewWithT(t)

	streams := genericiooptions.IOStreams{
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}

	cmd := deps.NewCommand(streams, nil)
	cmd.OutputSchema = true

	// Complete should skip client creation when OutputSchema is true
	err := cmd.Complete()
	g.Expect(err).ToNot(HaveOccurred())
}

func TestCommand_Validate_OutputSchema(t *testing.T) {
	g := NewWithT(t)

	streams := genericiooptions.IOStreams{
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}

	cmd := deps.NewCommand(streams, nil)
	cmd.OutputSchema = true
	// Invalid output format should be ignored when OutputSchema is true
	cmd.Output = "invalid"

	err := cmd.Validate()
	g.Expect(err).ToNot(HaveOccurred())
}

func TestCommand_Run_SchemaOutput(t *testing.T) {
	tests := []struct {
		name            string
		dryRun          bool
		expectedField   string
		unexpectedField string
	}{
		{
			name:            "normal mode returns dependency_status_list schema",
			dryRun:          false,
			expectedField:   "status",
			unexpectedField: "enabled",
		},
		{
			name:            "dry-run mode returns dependency_info_list schema",
			dryRun:          true,
			expectedField:   "enabled",
			unexpectedField: "status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			out := &bytes.Buffer{}
			streams := genericiooptions.IOStreams{
				Out:    out,
				ErrOut: &bytes.Buffer{},
			}

			cmd := deps.NewCommand(streams, nil)
			cmd.OutputSchema = true
			cmd.DryRun = tt.dryRun

			err := cmd.Run(context.Background())
			g.Expect(err).ToNot(HaveOccurred())

			// Parse the schema output
			var schema map[string]any
			err = json.Unmarshal(out.Bytes(), &schema)
			g.Expect(err).ToNot(HaveOccurred())

			// Navigate to dependencies items properties
			props := schema["properties"].(map[string]any)
			depsField := props["dependencies"].(map[string]any)
			items := depsField["items"].(map[string]any)
			itemProps := items["properties"].(map[string]any)

			// Verify expected field is present and unexpected field is absent
			g.Expect(itemProps).To(HaveKey(tt.expectedField),
				"schema should have %s field for dryRun=%v", tt.expectedField, tt.dryRun)
			g.Expect(itemProps).ToNot(HaveKey(tt.unexpectedField),
				"schema should not have %s field for dryRun=%v", tt.unexpectedField, tt.dryRun)
		})
	}
}
