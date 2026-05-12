package logs_test

import (
	"errors"
	"testing"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/logs"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

// testIOStreams returns a minimal IOStreams for testing.
func testIOStreams() genericiooptions.IOStreams {
	return genericiooptions.IOStreams{}
}

func TestValidate(t *testing.T) {
	t.Run("accepts operator target", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &logs.Command{Target: "operator"}
		err := cmd.Validate()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("accepts component targets", func(t *testing.T) {
		g := NewWithT(t)

		componentTargets := []string{"dashboard", "kserve", "ray", "workbenches"}
		for _, target := range componentTargets {
			cmd := &logs.Command{Target: target}
			err := cmd.Validate()

			g.Expect(err).ToNot(HaveOccurred(), "expected %q to be valid", target)
		}
	})

	t.Run("rejects unknown target", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &logs.Command{Target: "unknown"}
		err := cmd.Validate()

		g.Expect(err).To(HaveOccurred())

		var structErr *clierrors.StructuredError
		g.Expect(errors.As(err, &structErr)).To(BeTrue(), "expected StructuredError")
		g.Expect(structErr.Code).To(Equal("INVALID_TARGET"))
		g.Expect(structErr.Message).To(ContainSubstring("unsupported target"))
		g.Expect(structErr.Message).To(ContainSubstring("unknown"))
	})

	t.Run("error message lists valid targets", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &logs.Command{Target: "invalid"}
		err := cmd.Validate()

		g.Expect(err).To(HaveOccurred())

		var structErr *clierrors.StructuredError
		g.Expect(errors.As(err, &structErr)).To(BeTrue(), "expected StructuredError")
		g.Expect(structErr.Message).To(ContainSubstring("operator"))
		g.Expect(structErr.Message).To(ContainSubstring("dashboard"))
	})

	t.Run("rejects empty target", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &logs.Command{Target: ""}
		err := cmd.Validate()

		g.Expect(err).To(HaveOccurred())

		var structErr *clierrors.StructuredError
		g.Expect(errors.As(err, &structErr)).To(BeTrue(), "expected StructuredError")
		g.Expect(structErr.Code).To(Equal("INVALID_TARGET"))
	})
}

func TestAddFlags(t *testing.T) {
	t.Run("registers all expected flags", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &logs.Command{}
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)

		g.Expect(fs.Lookup("follow")).ToNot(BeNil())
		g.Expect(fs.Lookup("tail")).ToNot(BeNil())
		g.Expect(fs.Lookup("since")).ToNot(BeNil())
		g.Expect(fs.Lookup("previous")).ToNot(BeNil())
		g.Expect(fs.Lookup("container")).ToNot(BeNil())
	})

	t.Run("sets correct default values", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &logs.Command{}
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)

		tailFlag := fs.Lookup("tail")
		g.Expect(tailFlag.DefValue).To(Equal("-1"))

		previousFlag := fs.Lookup("previous")
		g.Expect(previousFlag.DefValue).To(Equal("false"))
	})
}

func TestNewCommand(t *testing.T) {
	t.Run("initializes with default tail value", func(t *testing.T) {
		g := NewWithT(t)

		cmd := logs.NewCommand(testIOStreams(), nil)

		g.Expect(cmd.Tail).To(Equal(int64(-1)))
	})
}
