package components

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/stdin"
)

var (
	_ cmd.Command = (*EnableCommand)(nil)
	_ cmd.Command = (*DisableCommand)(nil)
)

// EnableCommand contains the enable subcommand configuration.
type EnableCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	// ComponentName is the single component name from positional arg (legacy).
	ComponentName string
	// ComponentNames is the list of components to enable (from stdin or single name).
	ComponentNames []string

	DryRun    bool
	Yes       bool
	FromStdin bool

	// flags stores the FlagSet for checking explicit flag usage.
	flags *pflag.FlagSet
}

// NewEnableCommand creates a new EnableCommand with defaults.
func NewEnableCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *EnableCommand {
	return &EnableCommand{
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags: configFlags,
	}
}

// SetComponentName sets the component name from command args.
func (c *EnableCommand) SetComponentName(name string) {
	c.ComponentName = name
}

// IsFromStdin returns true if the command is reading from stdin.
func (c *EnableCommand) IsFromStdin() bool {
	return c.FromStdin
}

// AddFlags registers command-specific flags.
func (c *EnableCommand) AddFlags(fs *pflag.FlagSet) {
	c.flags = fs
	fs.BoolVar(&c.DryRun, "dry-run", false, "Show what would change without applying")
	fs.BoolVarP(&c.Yes, "yes", "y", false, "Skip confirmation prompt")
	fs.BoolVar(&c.FromStdin, "from-stdin", false, stdin.FlagDesc)
}

// Complete resolves derived fields after flag parsing.
func (c *EnableCommand) Complete() error {
	// Parse stdin configuration if --from-stdin is specified
	if c.FromStdin {
		if err := c.parseStdinConfig(); err != nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation, err)
		}
	}

	// Build ComponentNames from stdin input or single positional arg
	if len(c.ComponentNames) == 0 && c.ComponentName != "" {
		c.ComponentNames = []string{c.ComponentName}
	}

	k8sClient, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	c.Client = k8sClient

	return nil
}

// parseStdinConfig reads and applies configuration from stdin.
func (c *EnableCommand) parseStdinConfig() error {
	if err := stdin.CheckPiped(c.IO.In()); err != nil {
		return err //nolint:wrapcheck // CheckPiped returns a self-descriptive user-facing error
	}

	var input StdinInput
	if err := stdin.Parse(c.IO.In(), &input); err != nil {
		return fmt.Errorf("parsing stdin: %w", err)
	}

	return c.applyStdinInput(&input)
}

// applyStdinInput merges stdin configuration into command options.
// Explicit CLI flags take precedence over stdin values.
func (c *EnableCommand) applyStdinInput(input *StdinInput) error {
	for i, name := range input.Components {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("empty component name at index %d", i)
		}

		input.Components[i] = trimmed
	}

	if len(input.Components) > 0 && c.ComponentName != "" {
		return errors.New("cannot specify both positional component and components in stdin")
	}

	if len(input.Components) > 0 {
		c.ComponentNames = input.Components
	}

	if input.DryRun && !stdin.FlagChanged(c.flags, "dry-run") {
		c.DryRun = true
	}

	if input.SkipConfirm && !stdin.FlagChanged(c.flags, "yes") {
		c.Yes = true
	}

	return nil
}

// Validate checks that all options are valid before execution.
func (c *EnableCommand) Validate() error {
	if len(c.ComponentNames) == 0 {
		return errors.New("at least one component name is required")
	}

	return nil
}

// Run executes the enable command.
func (c *EnableCommand) Run(ctx context.Context) error {
	var failed []string

	for _, componentName := range c.ComponentNames {
		if err := MutateComponentState(ctx, MutateConfig{
			IO:            c.IO,
			Client:        c.Client,
			ComponentName: componentName,
			TargetState:   constants.ManagementStateManaged,
			ActionVerb:    "enable",
			DryRun:        c.DryRun,
			SkipConfirm:   c.Yes,
		}); err != nil {
			c.IO.Errorf("Error enabling '%s': %v", componentName, err)
			failed = append(failed, componentName)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to enable %d component(s): %s", len(failed), strings.Join(failed, ", "))
	}

	return nil
}

// DisableCommand contains the disable subcommand configuration.
type DisableCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	// ComponentName is the single component name from positional arg (legacy).
	ComponentName string
	// ComponentNames is the list of components to disable (from stdin or single name).
	ComponentNames []string

	DryRun    bool
	Yes       bool
	FromStdin bool

	// flags stores the FlagSet for checking explicit flag usage.
	flags *pflag.FlagSet
}

// NewDisableCommand creates a new DisableCommand with defaults.
func NewDisableCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *DisableCommand {
	return &DisableCommand{
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags: configFlags,
	}
}

// SetComponentName sets the component name from command args.
func (c *DisableCommand) SetComponentName(name string) {
	c.ComponentName = name
}

// IsFromStdin returns true if the command is reading from stdin.
func (c *DisableCommand) IsFromStdin() bool {
	return c.FromStdin
}

// AddFlags registers command-specific flags.
func (c *DisableCommand) AddFlags(fs *pflag.FlagSet) {
	c.flags = fs
	fs.BoolVar(&c.DryRun, "dry-run", false, "Show what would change without applying")
	fs.BoolVarP(&c.Yes, "yes", "y", false, "Skip confirmation prompt")
	fs.BoolVar(&c.FromStdin, "from-stdin", false, stdin.FlagDesc)
}

// Complete resolves derived fields after flag parsing.
func (c *DisableCommand) Complete() error {
	// Parse stdin configuration if --from-stdin is specified
	if c.FromStdin {
		if err := c.parseStdinConfig(); err != nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation, err)
		}
	}

	// Build ComponentNames from stdin input or single positional arg
	if len(c.ComponentNames) == 0 && c.ComponentName != "" {
		c.ComponentNames = []string{c.ComponentName}
	}

	k8sClient, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	c.Client = k8sClient

	return nil
}

// parseStdinConfig reads and applies configuration from stdin.
func (c *DisableCommand) parseStdinConfig() error {
	if err := stdin.CheckPiped(c.IO.In()); err != nil {
		return err //nolint:wrapcheck // CheckPiped returns a self-descriptive user-facing error
	}

	var input StdinInput
	if err := stdin.Parse(c.IO.In(), &input); err != nil {
		return fmt.Errorf("parsing stdin: %w", err)
	}

	return c.applyStdinInput(&input)
}

// applyStdinInput merges stdin configuration into command options.
// Explicit CLI flags take precedence over stdin values.
func (c *DisableCommand) applyStdinInput(input *StdinInput) error {
	for i, name := range input.Components {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("empty component name at index %d", i)
		}

		input.Components[i] = trimmed
	}

	if len(input.Components) > 0 && c.ComponentName != "" {
		return errors.New("cannot specify both positional component and components in stdin")
	}

	if len(input.Components) > 0 {
		c.ComponentNames = input.Components
	}

	if input.DryRun && !stdin.FlagChanged(c.flags, "dry-run") {
		c.DryRun = true
	}

	if input.SkipConfirm && !stdin.FlagChanged(c.flags, "yes") {
		c.Yes = true
	}

	return nil
}

// Validate checks that all options are valid before execution.
func (c *DisableCommand) Validate() error {
	if len(c.ComponentNames) == 0 {
		return errors.New("at least one component name is required")
	}

	return nil
}

// Run executes the disable command.
func (c *DisableCommand) Run(ctx context.Context) error {
	var failed []string

	for _, componentName := range c.ComponentNames {
		if err := MutateComponentState(ctx, MutateConfig{
			IO:            c.IO,
			Client:        c.Client,
			ComponentName: componentName,
			TargetState:   constants.ManagementStateRemoved,
			ActionVerb:    "disable",
			DryRun:        c.DryRun,
			SkipConfirm:   c.Yes,
		}); err != nil {
			c.IO.Errorf("Error disabling '%s': %v", componentName, err)
			failed = append(failed, componentName)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to disable %d component(s): %s", len(failed), strings.Join(failed, ", "))
	}

	return nil
}
