package components

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	componentspkg "github.com/opendatahub-io/odh-cli/pkg/components"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "components"
	cmdShort = "View and manage ODH component lifecycle"
)

const cmdLong = `
View and manage ODH/RHOAI component lifecycle.

Components are feature modules managed by the DataScienceCluster (DSC) resource.
Each component has a managementState: Managed, Unmanaged, or Removed.

The component list is dynamically discovered from the DSC spec and enriched
with health information from component CRs (when available).
`

const cmdExample = `
  # List all components with their state and health
  kubectl odh components

  # List components as JSON
  kubectl odh components -o json

  # List components as YAML
  kubectl odh components -o yaml

  # Describe a specific component
  kubectl odh components describe kserve

  # Enable a component
  kubectl odh components enable ray

  # Disable a component
  kubectl odh components disable trustyai
`

// command is the interface for all component subcommands.
type command interface {
	AddFlags(fs *pflag.FlagSet)
	Complete() error
	Validate() error
	Run(ctx context.Context) error
}

// mutateCommand extends command with SetComponentName for enable/disable.
type mutateCommand interface {
	command
	SetComponentName(name string)
}

// runCommand executes the Complete/Validate/Run lifecycle with error handling.
//
//nolint:wrapcheck // HandleError returns an already-handled error
func runCommand(cmd *cobra.Command, c command, outputFormat string) error {
	if err := c.Complete(); err != nil {
		return clierrors.HandleError(cmd, err, outputFormat)
	}

	if err := c.Validate(); err != nil {
		return clierrors.HandleError(cmd, err, outputFormat)
	}

	if err := c.Run(cmd.Context()); err != nil {
		return clierrors.HandleError(cmd, err, outputFormat)
	}

	return nil
}

// AddCommand adds the components command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	listCommand := componentspkg.NewListCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCommand(cmd, listCommand, listCommand.OutputFormat)
		},
	}

	listCommand.AddFlags(cmd.Flags())

	addDescribeCommand(cmd, flags, streams)
	addMutateCommand(cmd, componentspkg.NewEnableCommand(streams, flags), "enable COMPONENT", "Enable a component (set managementState to Managed)")
	addMutateCommand(cmd, componentspkg.NewDisableCommand(streams, flags), "disable COMPONENT", "Disable a component (set managementState to Removed)")

	root.AddCommand(cmd)
}

func addDescribeCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags, streams genericiooptions.IOStreams) {
	describeCommand := componentspkg.NewDescribeCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           "describe COMPONENT",
		Short:         "Show detailed information about a component",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			describeCommand.ComponentName = args[0]

			return runCommand(cmd, describeCommand, describeCommand.OutputFormat)
		},
	}

	describeCommand.AddFlags(cmd.Flags())
	parent.AddCommand(cmd)
}

func addMutateCommand(parent *cobra.Command, command mutateCommand, use, short string) {
	cmd := &cobra.Command{
		Use:           use,
		Short:         short,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command.SetComponentName(args[0])

			return runCommand(cmd, command, "")
		},
	}

	command.AddFlags(cmd.Flags())
	parent.AddCommand(cmd)
}
