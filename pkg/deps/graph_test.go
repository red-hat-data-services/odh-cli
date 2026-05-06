package deps_test

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

const (
	testGraphHeader     = "Dependency Graph"
	testGraphCertMgr    = "Cert Manager"
	testGraphRequiredBy = "Required by:"
	testGraphDependsOn  = "Depends on:"
)

func TestGraphCommand(t *testing.T) {
	t.Run("Complete", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)

		err := cmd.Complete()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("Validate", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)

		err := cmd.Validate()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("AddFlags", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)

		g.Expect(fs.Lookup("refresh")).ToNot(BeNil())
		g.Expect(fs.Lookup("version")).ToNot(BeNil())
	})

	t.Run("Defaults_VersionAndRefresh", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)

		g.Expect(cmd.Version).To(BeEmpty())
		g.Expect(cmd.Refresh).To(BeFalse())
	})

	t.Run("Run", func(t *testing.T) {
		g := NewWithT(t)

		out := &bytes.Buffer{}
		streams := genericiooptions.IOStreams{
			Out:    out,
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)

		err := cmd.Run(t.Context())

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out.String()).To(ContainSubstring(testGraphHeader))
		g.Expect(out.String()).To(ContainSubstring(testGraphCertMgr))
		g.Expect(out.String()).To(ContainSubstring(testGraphRequiredBy))
		g.Expect(out.String()).To(ContainSubstring(testGraphDependsOn))
	})

	t.Run("Run_WithRefreshFlag", func(t *testing.T) {
		g := NewWithT(t)

		out := &bytes.Buffer{}
		streams := genericiooptions.IOStreams{
			Out:    out,
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)
		cmd.Refresh = true

		// Run uses embedded manifest as fallback when refresh fails (no network in tests)
		// This verifies the refresh flag is wired up and doesn't break execution
		err := cmd.Run(t.Context())

		// Should succeed using embedded manifest fallback
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out.String()).To(ContainSubstring(testGraphHeader))
	})

	t.Run("Run_DynamicHeaderWidth", func(t *testing.T) {
		g := NewWithT(t)

		out := &bytes.Buffer{}
		streams := genericiooptions.IOStreams{
			Out:    out,
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewGraphCommand(streams, nil)

		err := cmd.Run(t.Context())

		g.Expect(err).ToNot(HaveOccurred())

		// Verify header and separator are present
		output := out.String()
		lines := bytes.Split([]byte(output), []byte("\n"))
		g.Expect(len(lines)).To(BeNumerically(">=", 2))

		// First line is header, second is separator
		headerLine := string(lines[0])
		separatorLine := string(lines[1])

		// Separator should match header length (dynamic width)
		g.Expect(len(separatorLine)).To(BeNumerically(">=", len(headerLine)))
	})
}
