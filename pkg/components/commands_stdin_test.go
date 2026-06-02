package components_test

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/components"

	. "github.com/onsi/gomega"
)

// Test fixtures for stdin input parsing.
const (
	fixtureStdinJSON = `{"components": ["dashboard", "kserve"], "dryRun": true, "skipConfirm": true}`

	fixtureStdinYAML = `
components:
  - dashboard
  - kserve
  - ray
dryRun: true
`
	fixtureStdinInvalid       = `{"components": invalid}`
	fixtureStdinUnknownFields = `{"kompnents": ["dashboard"]}`
	fixtureStdinMinimal       = `{"components": ["dashboard"]}`
	fixtureStdinEmpty         = `{}`
	fixtureStdinOnlyFlags     = `{"dryRun": true, "skipConfirm": true}`
)

// testConfigFlags creates ConfigFlags for testing.
func testConfigFlags() *genericclioptions.ConfigFlags {
	return genericclioptions.NewConfigFlags(true)
}

func TestEnableCommand_FromStdinFlag(t *testing.T) {
	t.Run("AddFlags should register --from-stdin flag", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)

		flag := fs.Lookup("from-stdin")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.DefValue).To(Equal("false"))
	})
}

func TestEnableCommand_StdinInput(t *testing.T) {
	t.Run("Complete should parse stdin JSON and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinJSON)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard", "kserve"}))
		g.Expect(command.DryRun).To(BeTrue())
		g.Expect(command.Yes).To(BeTrue())
	})

	t.Run("Complete should parse stdin YAML and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinYAML)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard", "kserve", "ray"}))
		g.Expect(command.DryRun).To(BeTrue())
	})

	t.Run("Complete should fail on invalid stdin JSON", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInvalid)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing stdin"))
	})

	t.Run("Complete should reject unknown fields in stdin", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinUnknownFields)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing stdin"))
	})

	t.Run("Complete should keep defaults when stdin fields are omitted", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinMinimal)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard"}))
		g.Expect(command.DryRun).To(BeFalse())
		g.Expect(command.Yes).To(BeFalse())
	})

	t.Run("Explicit CLI flags should take precedence over stdin values", func(t *testing.T) {
		g := NewWithT(t)

		// Stdin sets dryRun=true, but CLI flag sets dry-run=false
		stdin := bytes.NewBufferString(fixtureStdinJSON) // has dryRun: true

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)
		err := fs.Parse([]string{"--dry-run=false", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// CLI flag should win over stdin
		g.Expect(command.DryRun).To(BeFalse())

		// Stdin values should apply for non-explicitly-set flags
		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard", "kserve"}))
		g.Expect(command.Yes).To(BeTrue())
	})

	t.Run("ComponentName from positional arg should be used when stdin has no components", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinOnlyFlags)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true
		command.ComponentName = "dashboard" // Set via positional arg

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard"}))
		g.Expect(command.DryRun).To(BeTrue())
		g.Expect(command.Yes).To(BeTrue())
	})

	t.Run("Validate should fail when no components provided via stdin or positional arg", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinEmpty)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewEnableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("at least one component name is required"))
	})
}

func TestDisableCommand_FromStdinFlag(t *testing.T) {
	t.Run("AddFlags should register --from-stdin flag", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)

		flag := fs.Lookup("from-stdin")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.DefValue).To(Equal("false"))
	})
}

func TestDisableCommand_StdinInput(t *testing.T) {
	t.Run("Complete should parse stdin JSON and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinJSON)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard", "kserve"}))
		g.Expect(command.DryRun).To(BeTrue())
		g.Expect(command.Yes).To(BeTrue())
	})

	t.Run("Complete should parse stdin YAML and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinYAML)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard", "kserve", "ray"}))
		g.Expect(command.DryRun).To(BeTrue())
	})

	t.Run("Complete should fail on invalid stdin JSON", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInvalid)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing stdin"))
	})

	t.Run("Complete should reject unknown fields in stdin", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinUnknownFields)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing stdin"))
	})

	t.Run("Explicit CLI flags should take precedence over stdin values", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinJSON)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)
		err := fs.Parse([]string{"--yes=false", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// CLI flag should win over stdin
		g.Expect(command.Yes).To(BeFalse())

		// Stdin values should apply for non-explicitly-set flags
		g.Expect(command.ComponentNames).To(Equal([]string{"dashboard", "kserve"}))
		g.Expect(command.DryRun).To(BeTrue())
	})

	t.Run("Validate should fail when no components provided", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinEmpty)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := components.NewDisableCommand(streams, testConfigFlags())
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("at least one component name is required"))
	})
}
