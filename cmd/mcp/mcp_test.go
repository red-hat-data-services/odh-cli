package mcp_test

import (
	"testing"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/cmd/mcp"

	. "github.com/onsi/gomega"
)

func TestAddCommand(t *testing.T) {
	t.Run("should register mcp command with serve subcommand", func(t *testing.T) {
		g := NewWithT(t)

		root := &cobra.Command{Use: "test"}
		flags := genericclioptions.NewConfigFlags(true)
		mcp.AddCommand(root, flags)

		mcpCmd, _, err := root.Find([]string{"mcp"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(mcpCmd.Use).To(Equal("mcp"))

		serveCmd, _, err := root.Find([]string{"mcp", "serve"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(serveCmd.Use).To(Equal("serve"))
	})

	t.Run("should have correct flag defaults", func(t *testing.T) {
		g := NewWithT(t)

		root := &cobra.Command{Use: "test"}
		flags := genericclioptions.NewConfigFlags(true)
		mcp.AddCommand(root, flags)

		serveCmd, _, err := root.Find([]string{"mcp", "serve"})
		g.Expect(err).ToNot(HaveOccurred())

		transportFlag := serveCmd.Flags().Lookup("transport")
		g.Expect(transportFlag).ToNot(BeNil())
		g.Expect(transportFlag.DefValue).To(Equal("stdio"))

		portFlag := serveCmd.Flags().Lookup("port")
		g.Expect(portFlag).ToNot(BeNil())
		g.Expect(portFlag.DefValue).To(Equal("8080"))
	})
}
