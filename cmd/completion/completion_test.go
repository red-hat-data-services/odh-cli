package completion_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/opendatahub-io/odh-cli/cmd/completion"
	"github.com/opendatahub-io/odh-cli/cmd/get"
	"github.com/opendatahub-io/odh-cli/cmd/logs"

	. "github.com/onsi/gomega"
)

func TestCompletionCommandExists(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "test"}
	completion.AddCommand(root, nil)

	cmd, _, err := root.Find([]string{"completion"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.Use).To(Equal("completion"))
}

func TestCompletionSubcommandsExist(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	completion.AddCommand(root, nil)

	shells := []string{"bash", "zsh", "fish"}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			g := NewWithT(t)

			cmd, _, err := root.Find([]string{"completion", shell})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(cmd.Use).To(Equal(shell))
		})
	}
}

func TestCompletionOutputsScript(t *testing.T) {
	tests := []struct {
		shell    string
		contains string
	}{
		{"bash", "bash completion"},
		{"zsh", "#compdef"},
		{"fish", "fish completion"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			g := NewWithT(t)

			root := &cobra.Command{Use: "test"}
			completion.AddCommand(root, nil)

			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetArgs([]string{"completion", tt.shell})

			err := root.Execute()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(buf.String()).ToNot(BeEmpty())
			g.Expect(buf.String()).To(ContainSubstring(tt.contains))
		})
	}
}

func TestCompletionReturnsSubcommands(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	completion.AddCommand(root, nil)
	get.AddCommand(root, nil)
	logs.AddCommand(root, nil)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"__complete", ""})

	err := root.Execute()
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("completion"))
	g.Expect(output).To(ContainSubstring("get"))
	g.Expect(output).To(ContainSubstring("logs"))
}

func TestGetCommandCompletion(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	get.AddCommand(root, nil)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"__complete", "get", ""})

	err := root.Execute()
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("notebooks"))
	g.Expect(output).To(ContainSubstring("inferenceservices"))
}

func TestLogsCommandCompletion(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	logs.AddCommand(root, nil)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"__complete", "logs", ""})

	err := root.Execute()
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("operator"))
	g.Expect(output).To(ContainSubstring("dashboard"))
	g.Expect(output).To(ContainSubstring("kserve"))
}
