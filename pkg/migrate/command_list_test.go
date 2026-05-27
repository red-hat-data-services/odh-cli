package migrate_test

import (
	"testing"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"

	. "github.com/onsi/gomega"
)

func TestListCommand_Validate(t *testing.T) {
	t.Run("should require target version when all is false", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = ""
		cmd.ShowAll = false

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("target-version"))
	})

	t.Run("should not require target version when all is true", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = ""
		cmd.ShowAll = true

		err := cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject both all and target-version together", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.ShowAll = true

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
	})

	t.Run("should validate successfully with target version", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestListCommand_Validate_Phase(t *testing.T) {
	t.Run("should accept valid phase", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.Phase = "pre-upgrade"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject invalid phase", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.Phase = "invalid"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid phase"))
	})

	t.Run("should accept empty phase", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.Phase = ""

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestListCommand_Complete(t *testing.T) {
	t.Run("should parse valid target version", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject invalid target version", func(t *testing.T) {
		g := NewWithT(t)

		cmd := migrate.NewListCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "invalid"

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid target version"))
	})
}
