package api_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	"github.com/opendatahub-io/odh-cli/pkg/api"

	. "github.com/onsi/gomega"
)

func runManifest(t *testing.T, root *cobra.Command) api.Manifest {
	t.Helper()

	g := NewWithT(t)

	var buf bytes.Buffer
	err := api.Run(root, &buf)
	g.Expect(err).ToNot(HaveOccurred())

	var manifest api.Manifest
	err = json.Unmarshal(buf.Bytes(), &manifest)
	g.Expect(err).ToNot(HaveOccurred())

	return manifest
}

func TestRun_BasicCommandTree(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh", Short: "test root"}
	root.PersistentFlags().String("kubeconfig", "", "path to kubeconfig")

	child := &cobra.Command{
		Use:     "lint",
		Short:   "Validate installation",
		Example: "  # Validate\n  kubectl odh lint\n\n  # JSON output\n  kubectl odh lint -o json",
	}
	child.Flags().StringP("output", "o", "table", "output format (table|json|yaml)")
	child.Flags().Bool("schema", false, "output JSON Schema")
	root.AddCommand(child)

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Name).To(Equal("lint"))
	g.Expect(manifest.Commands[0].Description).To(Equal("Validate installation"))
	g.Expect(manifest.Commands[0].HasSchema).To(BeTrue())
	g.Expect(manifest.Commands[0].Examples).To(Equal([]string{
		"kubectl odh lint",
		"kubectl odh lint -o json",
	}))

	g.Expect(manifest.GlobalFlags).To(HaveLen(1))
	g.Expect(manifest.GlobalFlags[0].Name).To(Equal("kubeconfig"))
}

func TestRun_FlagDetails(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	child := &cobra.Command{Use: "test", Short: "test cmd"}
	child.Flags().StringP("output", "o", "table", "output format")
	_ = child.Flags().SetAnnotation("output", api.AnnotationValidValues, []string{"table", "json", "yaml"})
	child.Flags().String("target-version", "", "target version for upgrade readiness checks")
	root.AddCommand(child)

	manifest := runManifest(t, root)

	var outputFlag, targetFlag api.FlagDescriptor
	for _, f := range manifest.Commands[0].Flags {
		switch f.Name {
		case "output":
			outputFlag = f
		case "target-version":
			targetFlag = f
		}
	}

	g.Expect(outputFlag.Shorthand).To(Equal("o"))
	g.Expect(outputFlag.Type).To(Equal("string"))
	g.Expect(outputFlag.Default).To(Equal("table"))
	g.Expect(outputFlag.ValidValues).To(Equal([]string{"table", "json", "yaml"}))

	g.Expect(targetFlag.Shorthand).To(BeEmpty())
	g.Expect(targetFlag.Default).To(BeEmpty())
	g.Expect(targetFlag.ValidValues).To(BeNil())
}

func TestRun_NestedSubcommands(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	parent := &cobra.Command{Use: "migrate", Short: "Manage migrations"}
	listCmd := &cobra.Command{Use: "list", Short: "List migrations"}
	listCmd.Flags().String("phase", "", "lifecycle phase")
	_ = listCmd.Flags().SetAnnotation("phase", api.AnnotationValidValues, []string{"pre-upgrade", "post-upgrade"})
	parent.AddCommand(listCmd)
	root.AddCommand(parent)

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Name).To(Equal("migrate"))
	g.Expect(manifest.Commands[0].Subcommands).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Subcommands[0].Name).To(Equal("list"))
	g.Expect(manifest.Commands[0].Subcommands[0].Flags).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Subcommands[0].Flags[0].ValidValues).To(
		Equal([]string{"pre-upgrade", "post-upgrade"}))
}

func TestRun_HiddenCommandsExcluded(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "lint", Short: "Validate"})
	root.AddCommand(&cobra.Command{Use: "secret", Short: "Hidden cmd", Hidden: true})

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Name).To(Equal("lint"))
}

func TestRun_HiddenCommandsExcludedIncludingAPI(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "lint", Short: "Validate"})
	root.AddCommand(&cobra.Command{Use: "api", Short: "API manifest", Hidden: true})

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Name).To(Equal("lint"))
}

func TestRun_DeprecatedFlagsExcluded(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	child := &cobra.Command{Use: "test", Short: "test cmd"}
	child.Flags().String("good-flag", "", "a valid flag")
	child.Flags().String("old-flag", "", "deprecated")
	_ = child.Flags().MarkDeprecated("old-flag", "use --good-flag instead")
	root.AddCommand(child)

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands[0].Flags).To(HaveLen(1))
	g.Expect(manifest.Commands[0].Flags[0].Name).To(Equal("good-flag"))
}

func TestRun_GlobalFlagsNotDuplicatedPerCommand(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.PersistentFlags().String("kubeconfig", "", "path to kubeconfig")
	child := &cobra.Command{Use: "lint", Short: "Validate"}
	child.Flags().String("output", "table", "output format (table|json|yaml)")
	root.AddCommand(child)

	manifest := runManifest(t, root)

	g.Expect(manifest.GlobalFlags).To(HaveLen(1))
	g.Expect(manifest.GlobalFlags[0].Name).To(Equal("kubeconfig"))

	for _, f := range manifest.Commands[0].Flags {
		g.Expect(f.Name).ToNot(Equal("kubeconfig"))
	}
}

func TestRun_CommandWithNoFlagsOrExamples(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "simple", Short: "no flags"})

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands[0].Flags).To(BeEmpty())
	g.Expect(manifest.Commands[0].Examples).To(BeEmpty())
	g.Expect(manifest.Commands[0].Subcommands).To(BeEmpty())
	g.Expect(manifest.Commands[0].HasSchema).To(BeFalse())
}

func TestRun_EmptySlicesSerializeAsArrays(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "bare", Short: "bare"})

	var buf bytes.Buffer
	err := api.Run(root, &buf)
	g.Expect(err).ToNot(HaveOccurred())

	raw := buf.String()
	g.Expect(raw).To(ContainSubstring(`"flags": []`))
	g.Expect(raw).To(ContainSubstring(`"examples": []`))
	g.Expect(raw).To(ContainSubstring(`"subcommands": []`))
	g.Expect(raw).ToNot(ContainSubstring(`"validValues"`))
}

func TestRun_HelpCommandExcluded(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "lint", Short: "Validate"})

	manifest := runManifest(t, root)

	for _, cmd := range manifest.Commands {
		g.Expect(cmd.Name).ToNot(Equal("help"))
	}
}

func TestRun_ValidValuesOmittedWhenEmpty(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	child := &cobra.Command{Use: "test", Short: "test"}
	child.Flags().String("plain", "", "a plain flag")
	child.Flags().String("enum", "", "output format")
	_ = child.Flags().SetAnnotation("enum", api.AnnotationValidValues, []string{"table", "json"})
	root.AddCommand(child)

	var buf bytes.Buffer
	err := api.Run(root, &buf)
	g.Expect(err).ToNot(HaveOccurred())

	var raw map[string]any
	err = json.Unmarshal(buf.Bytes(), &raw)
	g.Expect(err).ToNot(HaveOccurred())

	commands := raw["commands"].([]any)
	flags := commands[0].(map[string]any)["flags"].([]any)

	for _, f := range flags {
		flag := f.(map[string]any)
		if flag["name"] == "plain" {
			_, hasValidValues := flag["validValues"]
			g.Expect(hasValidValues).To(BeFalse())
		}
		if flag["name"] == "enum" {
			g.Expect(flag["validValues"]).To(Equal([]any{"table", "json"}))
		}
	}
}

func TestRun_ArgsPattern(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "get RESOURCE [NAME]", Short: "Get resources"})
	root.AddCommand(&cobra.Command{Use: "lint", Short: "Validate"})

	manifest := runManifest(t, root)

	var getCmd, lintCmd api.CommandDescriptor
	for _, c := range manifest.Commands {
		switch c.Name {
		case "get":
			getCmd = c
		case "lint":
			lintCmd = c
		}
	}

	g.Expect(getCmd.Args).ToNot(BeNil())
	g.Expect(getCmd.Args.Pattern).To(Equal("RESOURCE [NAME]"))
	g.Expect(lintCmd.Args).To(BeNil())
}

func TestRun_ArgsValidValues(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{
		Use:       "logs TARGET",
		Short:     "Show logs",
		ValidArgs: []string{"operator", "dashboard", "kserve"},
	})

	manifest := runManifest(t, root)

	g.Expect(manifest.Commands[0].Args).ToNot(BeNil())
	g.Expect(manifest.Commands[0].Args.Pattern).To(Equal("TARGET"))
	g.Expect(manifest.Commands[0].Args.ValidValues).To(Equal([]string{"operator", "dashboard", "kserve"}))
}

func TestRun_ArgsOmittedWhenNoPattern(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.AddCommand(&cobra.Command{Use: "status", Short: "Show status"})

	var buf bytes.Buffer
	err := api.Run(root, &buf)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(buf.String()).ToNot(ContainSubstring(`"args"`))
}

func TestRun_DeprecatedGlobalFlagsExcluded(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "kubectl-odh"}
	root.PersistentFlags().String("password", "", "password for basic auth")
	_ = root.PersistentFlags().MarkDeprecated("password", "use --token instead")
	root.AddCommand(&cobra.Command{Use: "test", Short: "test"})

	manifest := runManifest(t, root)

	for _, f := range manifest.GlobalFlags {
		g.Expect(f.Name).ToNot(Equal("password"))
	}
}
