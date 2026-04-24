package deps_test

import (
	"bytes"
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
