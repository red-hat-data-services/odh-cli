package migrate_test

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"

	. "github.com/onsi/gomega"
)

// Test fixtures for stdin input parsing.
const (
	fixtureStdinJSON = `{"migrations": ["kueue.rhbok.migrate", "modelserving.serverless-to-raw"], "targetVersion": "3.0.0", "dryRun": true, "skipConfirm": true}`

	fixtureStdinYAML = `
migrations:
  - kueue.rhbok.migrate
  - modelserving.serverless-to-raw
  - modelserving.modelmesh-to-raw
targetVersion: "3.0.0"
dryRun: true
`
	fixtureStdinWithPhase = `{
		"phase": "pre-upgrade",
		"targetVersion": "3.0.0",
		"skipConfirm": true
	}`
	fixtureStdinInvalid       = `{"migrations": invalid}`
	fixtureStdinUnknownFields = `{"migrashuns": ["kueue.rhbok.migrate"]}`
	fixtureStdinMinimal       = `{"migrations": ["kueue.rhbok.migrate"], "targetVersion": "3.0.0"}`
)

func TestRunCommand_FromStdinFlag(t *testing.T) {
	t.Run("AddFlags should register --from-stdin flag", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)

		// Verify --from-stdin flag is registered
		flag := fs.Lookup("from-stdin")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.DefValue).To(Equal("false"))
	})
}

func TestRunCommand_StdinInput(t *testing.T) {
	t.Run("Complete should parse stdin JSON and apply to command", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinJSON)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.MigrationIDs).To(Equal([]string{"kueue.rhbok.migrate", "modelserving.serverless-to-raw"}))
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
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

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.MigrationIDs).To(Equal([]string{"kueue.rhbok.migrate", "modelserving.serverless-to-raw", "modelserving.modelmesh-to-raw"}))
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
		g.Expect(command.DryRun).To(BeTrue())
	})

	t.Run("Complete should parse stdin with phase", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinWithPhase)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.Phase).To(Equal("pre-upgrade"))
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
		g.Expect(command.Yes).To(BeTrue())
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

		command := migrate.NewRunCommand(streams)
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

		command := migrate.NewRunCommand(streams)
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

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(command.MigrationIDs).To(Equal([]string{"kueue.rhbok.migrate"}))
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
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

		command := migrate.NewRunCommand(streams)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)
		err := fs.Parse([]string{"--dry-run=false", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// CLI flag should win over stdin
		g.Expect(command.DryRun).To(BeFalse())

		// Stdin values should apply for non-explicitly-set flags
		g.Expect(command.MigrationIDs).To(Equal([]string{"kueue.rhbok.migrate", "modelserving.serverless-to-raw"}))
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
		g.Expect(command.Yes).To(BeTrue())
	})

	t.Run("CLI migration flags should take precedence over stdin migrations", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinJSON)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)
		err := fs.Parse([]string{"--migration=different.migration", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// CLI flag should win over stdin
		g.Expect(command.MigrationIDs).To(Equal([]string{"different.migration"}))

		// Stdin values should apply for non-explicitly-set flags
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
	})

	t.Run("CLI target-version should take precedence over stdin", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString(fixtureStdinJSON) // has targetVersion: "3.0.0"

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)
		err := fs.Parse([]string{"--target-version=4.0.0", "--from-stdin"})
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// CLI flag should win over stdin
		g.Expect(command.TargetVersion).To(Equal("4.0.0"))
	})

	t.Run("Validate should fail when no migrations or phase provided via stdin", func(t *testing.T) {
		g := NewWithT(t)

		// Only flags, no migrations or phase
		stdin := bytes.NewBufferString(`{"targetVersion": "3.0.0", "dryRun": true}`)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("--migration flag is required"))
	})

	t.Run("Validate should fail when no target-version provided", func(t *testing.T) {
		g := NewWithT(t)

		// Missing targetVersion
		stdin := bytes.NewBufferString(`{"migrations": ["kueue.rhbok.migrate"]}`)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = command.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("--target-version flag is required"))
	})

	t.Run("Complete should fail on empty stdin", func(t *testing.T) {
		g := NewWithT(t)

		stdin := bytes.NewBufferString("")

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     stdin,
			Out:    &out,
			ErrOut: &errOut,
		}

		command := migrate.NewRunCommand(streams)
		command.FromStdin = true

		err := command.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("empty input"))
	})
}
