package deps_test

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

// Test fixtures for stdin input parsing.
const (
	fixtureStdinInstallJSON = `{"deps": ["cert-manager", "kueue"], "version": "2.19.0", "dryRun": true, "includeOptional": true}`

	fixtureStdinInstallYAML = `
deps:
  - cert-manager
  - kueue
  - jobset
version: "2.19.0"
dryRun: true
`
	fixtureStdinInstallSingleDep = `{"deps": ["cert-manager"], "dryRun": true}`

	fixtureStdinInstallFlagsOnly = `{"version": "2.19.0", "dryRun": true, "includeOptional": false}`

	fixtureStdinInstallEmptyDeps     = `{"deps": [], "dryRun": true}`
	fixtureStdinInstallInvalid       = `{"deps": invalid}`
	fixtureStdinInstallUnknownFields = `{"deeps": ["cert-manager"]}`
)

func TestInstallCommand_FromStdinFlag(t *testing.T) {
	t.Run("AddFlags should register --from-stdin flag", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)

		flag := fs.Lookup("from-stdin")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.DefValue).To(Equal("false"))
	})
}

func TestInstallCommand_StdinInput(t *testing.T) {
	// Tests below set FromStdin directly without calling AddFlags.
	// This is intentional — they test stdin parsing and error handling only.
	// CLI-over-stdin precedence is validated by the tests that call AddFlags
	// and fs.Parse() further below.

	t.Run("Complete should parse stdin JSON and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallJSON)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(cmd.TargetDeps).To(Equal([]string{"cert-manager", "kueue"}))
		g.Expect(cmd.Version).To(Equal("2.19.0"))
		g.Expect(cmd.DryRun).To(BeTrue())
		g.Expect(cmd.IncludeOptional).To(BeTrue())
	})

	t.Run("Complete should parse stdin YAML and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallYAML)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(cmd.TargetDeps).To(Equal([]string{"cert-manager", "kueue", "jobset"}))
		g.Expect(cmd.Version).To(Equal("2.19.0"))
		g.Expect(cmd.DryRun).To(BeTrue())
	})

	t.Run("Complete should apply flags-only stdin (no deps)", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallFlagsOnly)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(cmd.TargetDeps).To(BeEmpty())
		g.Expect(cmd.Version).To(Equal("2.19.0"))
		g.Expect(cmd.DryRun).To(BeTrue())
	})

	t.Run("Complete should keep defaults when stdin fields are omitted", func(t *testing.T) {
		g := NewWithT(t)

		// Single dep with dryRun:true to avoid cluster client creation; other fields test defaults
		stdin := bytes.NewBufferString(fixtureStdinInstallSingleDep)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(cmd.TargetDeps).To(Equal([]string{"cert-manager"}))
		g.Expect(cmd.Version).To(BeEmpty())
		g.Expect(cmd.IncludeOptional).To(BeFalse())
		g.Expect(cmd.Refresh).To(BeFalse())
	})

	t.Run("Complete should treat explicit empty deps list as install-nothing", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallEmptyDeps)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// Non-nil empty slice: explicit empty list means "install nothing", not bulk mode
		g.Expect(cmd.TargetDeps).ToNot(BeNil())
		g.Expect(cmd.TargetDeps).To(BeEmpty())
	})

	t.Run("Complete should fail on invalid stdin JSON", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallInvalid)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing stdin"))
	})

	t.Run("Complete should reject unknown fields in stdin", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallUnknownFields)
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing stdin"))
	})

	t.Run("Complete should fail on empty stdin", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString("")
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("empty input"))
	})

	t.Run("Explicit CLI flags should take precedence over stdin values", func(t *testing.T) {
		g := NewWithT(t)

		// Stdin sets includeOptional=true; CLI flag overrides with false.
		// DryRun stays true (from stdin, not overridden) so Complete() exits before k8s client creation.
		stdin := bytes.NewBufferString(fixtureStdinInstallJSON) // has dryRun: true, includeOptional: true
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)
		err := fs.Parse([]string{"--include-optional=false", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// CLI flag wins over stdin
		g.Expect(cmd.IncludeOptional).To(BeFalse())
		// Stdin values apply for non-explicitly-set flags
		g.Expect(cmd.TargetDeps).To(Equal([]string{"cert-manager", "kueue"}))
		g.Expect(cmd.Version).To(Equal("2.19.0"))
		g.Expect(cmd.DryRun).To(BeTrue())
	})

	t.Run("CLI --version should take precedence over stdin version", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallJSON) // has version: "2.19.0"
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)
		err := fs.Parse([]string{"--version=3.0.0", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(cmd.Version).To(Equal("3.0.0"))
		g.Expect(cmd.TargetDeps).To(Equal([]string{"cert-manager", "kueue"}))
	})

	t.Run("Positional arg and stdin deps should conflict", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallSingleDep) // has deps: ["cert-manager"]
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true
		cmd.TargetDep = "kueue" // simulates positional arg set by cobra

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("cannot specify both"))
	})

	t.Run("Single positional arg should be normalized into TargetDeps", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.TargetDep = "cert-manager"
		cmd.DryRun = true // avoid cluster client creation

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(cmd.TargetDeps).To(Equal([]string{"cert-manager"}))
		g.Expect(cmd.TargetDep).To(BeEmpty(), "TargetDep must be zeroed after normalization to prevent stale reads")
	})

	t.Run("Pre-populated TargetDeps and stdin deps should conflict", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinInstallSingleDep) // has deps: ["cert-manager"]
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.FromStdin = true
		cmd.TargetDeps = []string{"kueue"} // pre-populated, no positional arg

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("cannot specify both"))
	})
}
