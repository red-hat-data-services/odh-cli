package components_test

import (
	"bytes"
	"testing"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newEnableCommand() (*components.EnableCommand, *bytes.Buffer) {
	var out bytes.Buffer

	streams := genericiooptions.IOStreams{
		In:     nil,
		Out:    &out,
		ErrOut: &out,
	}

	cmd := components.NewEnableCommand(streams, nil)
	cmd.IO = iostreams.NewIOStreams(nil, &out, &out)

	return cmd, &out
}

func newDisableCommand() (*components.DisableCommand, *bytes.Buffer) {
	var out bytes.Buffer

	streams := genericiooptions.IOStreams{
		In:     nil,
		Out:    &out,
		ErrOut: &out,
	}

	cmd := components.NewDisableCommand(streams, nil)
	cmd.IO = iostreams.NewIOStreams(nil, &out, &out)

	return cmd, &out
}

func TestEnableCommand_Validate(t *testing.T) {
	t.Run("requires component name", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _ := newEnableCommand()
		cmd.ComponentName = ""

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("component name is required"))
	})
}

func TestDisableCommand_Validate(t *testing.T) {
	t.Run("requires component name", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _ := newDisableCommand()
		cmd.ComponentName = ""

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("component name is required"))
	})
}
