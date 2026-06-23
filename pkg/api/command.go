package api

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/internal/version"
)

func Run(root *cobra.Command, out io.Writer) error {
	manifest := Manifest{
		Version:     version.GetVersion(),
		GlobalFlags: collectFlags(root.PersistentFlags()),
		Commands:    walkCommands(root),
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("encoding API manifest: %w", err)
	}

	return nil
}

func collectFlags(fs *pflag.FlagSet) []FlagDescriptor {
	flags := []FlagDescriptor{}

	fs.VisitAll(func(f *pflag.Flag) {
		if f.Deprecated != "" {
			return
		}

		flags = append(flags, buildFlagDescriptor(f))
	})

	return flags
}

func walkCommands(parent *cobra.Command) []CommandDescriptor {
	descriptors := []CommandDescriptor{}

	for _, child := range parent.Commands() {
		if child.Hidden {
			continue
		}

		if child.Deprecated != "" {
			continue
		}

		if child.Name() == "help" {
			continue
		}

		descriptors = append(descriptors, CommandDescriptor{
			Name:        child.Name(),
			Description: child.Short,
			Args:        buildArgsDescriptor(child),
			Flags:       collectFlags(child.NonInheritedFlags()),
			HasSchema:   child.Flags().Lookup("schema") != nil,
			Examples:    parseExamples(child.Example),
			Subcommands: walkCommands(child),
		})
	}

	return descriptors
}

func buildArgsDescriptor(cmd *cobra.Command) *ArgsDescriptor {
	pattern := ""
	if i := strings.IndexByte(cmd.Use, ' '); i != -1 {
		pattern = cmd.Use[i+1:]
	}

	if pattern == "" && len(cmd.ValidArgs) == 0 {
		return nil
	}

	return &ArgsDescriptor{
		Pattern:     pattern,
		ValidValues: cmd.ValidArgs,
	}
}

func buildFlagDescriptor(f *pflag.Flag) FlagDescriptor {
	return FlagDescriptor{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Default:     f.DefValue,
		Required:    isRequiredFlag(f),
		Description: f.Usage,
		ValidValues: validValuesFromAnnotation(f),
	}
}

func validValuesFromAnnotation(f *pflag.Flag) []string {
	if f.Annotations == nil {
		return nil
	}

	return f.Annotations[AnnotationValidValues]
}

func isRequiredFlag(f *pflag.Flag) bool {
	if f.Annotations == nil {
		return false
	}

	_, ok := f.Annotations[cobra.BashCompOneRequiredFlag]

	return ok
}

func parseExamples(examples string) []string {
	if examples == "" {
		return []string{}
	}

	result := []string{}

	for line := range strings.SplitSeq(examples, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		result = append(result, line)
	}

	return result
}
