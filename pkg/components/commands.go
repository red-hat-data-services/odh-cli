package components

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
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

	ComponentName string
	DryRun        bool
	Yes           bool
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

// AddFlags registers command-specific flags.
func (c *EnableCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.DryRun, "dry-run", false, "Show what would change without applying")
	fs.BoolVarP(&c.Yes, "yes", "y", false, "Skip confirmation prompt")
}

// Complete resolves derived fields after flag parsing.
func (c *EnableCommand) Complete() error {
	k8sClient, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	c.Client = k8sClient

	return nil
}

// Validate checks that all options are valid before execution.
func (c *EnableCommand) Validate() error {
	if c.ComponentName == "" {
		return errors.New("component name is required")
	}

	return nil
}

// Run executes the enable command.
func (c *EnableCommand) Run(ctx context.Context) error {
	return MutateComponentState(ctx, MutateConfig{
		IO:            c.IO,
		Client:        c.Client,
		ComponentName: c.ComponentName,
		TargetState:   constants.ManagementStateManaged,
		ActionVerb:    "enable",
		DryRun:        c.DryRun,
		SkipConfirm:   c.Yes,
	})
}

// DisableCommand contains the disable subcommand configuration.
type DisableCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	ComponentName string
	DryRun        bool
	Yes           bool
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

// AddFlags registers command-specific flags.
func (c *DisableCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.DryRun, "dry-run", false, "Show what would change without applying")
	fs.BoolVarP(&c.Yes, "yes", "y", false, "Skip confirmation prompt")
}

// Complete resolves derived fields after flag parsing.
func (c *DisableCommand) Complete() error {
	k8sClient, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	c.Client = k8sClient

	return nil
}

// Validate checks that all options are valid before execution.
func (c *DisableCommand) Validate() error {
	if c.ComponentName == "" {
		return errors.New("component name is required")
	}

	return nil
}

// Run executes the disable command.
func (c *DisableCommand) Run(ctx context.Context) error {
	return MutateComponentState(ctx, MutateConfig{
		IO:            c.IO,
		Client:        c.Client,
		ComponentName: c.ComponentName,
		TargetState:   constants.ManagementStateRemoved,
		ActionVerb:    "disable",
		DryRun:        c.DryRun,
		SkipConfirm:   c.Yes,
	})
}
