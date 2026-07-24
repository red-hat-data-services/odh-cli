package diagnose_test

import (
	"testing"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/cmd/diagnose"

	. "github.com/onsi/gomega"
)

func TestAddCommand(t *testing.T) {
	t.Run("should register diagnose command", func(t *testing.T) {
		g := NewWithT(t)

		root := &cobra.Command{Use: "test"}
		flags := genericclioptions.NewConfigFlags(true)
		diagnose.AddCommand(root, flags)

		cmd, _, err := root.Find([]string{"diagnose"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.Use).To(Equal("diagnose"))
	})

	t.Run("should have correct flag defaults", func(t *testing.T) {
		g := NewWithT(t)

		root := &cobra.Command{Use: "test"}
		flags := genericclioptions.NewConfigFlags(true)
		diagnose.AddCommand(root, flags)

		cmd, _, err := root.Find([]string{"diagnose"})
		g.Expect(err).ToNot(HaveOccurred())

		jsonFlag := cmd.Flags().Lookup("json")
		g.Expect(jsonFlag).ToNot(BeNil())
		g.Expect(jsonFlag.DefValue).To(Equal("false"))

		componentFlag := cmd.Flags().Lookup("component")
		g.Expect(componentFlag).ToNot(BeNil())
		g.Expect(componentFlag.DefValue).To(Equal(""))

		eventsSinceFlag := cmd.Flags().Lookup("events-since")
		g.Expect(eventsSinceFlag).ToNot(BeNil())
		g.Expect(eventsSinceFlag.DefValue).To(Equal("5m0s"))
	})
}
