package logs

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	logspkg "github.com/opendatahub-io/odh-cli/pkg/logs"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "logs"
	cmdShort = "Show logs from ODH operator or components"
)

const cmdLong = `
Display logs from ODH/RHOAI operator or component pods.

The command auto-discovers pods based on the target name, eliminating the need
to manually find pod names with kubectl get pods.

Supported targets:
  operator    ODH/RHOAI operator pod

Examples:
  # Show operator logs
  kubectl odh logs operator

  # Follow operator logs (stream new logs as they appear)
  kubectl odh logs operator -f

  # Show last 100 lines
  kubectl odh logs operator --tail 100

  # Show logs from the last hour
  kubectl odh logs operator --since 1h

  # Show previous container logs (useful for crash investigation)
  kubectl odh logs operator --previous
`

const cmdExample = `
  # Show operator logs
  kubectl odh logs operator

  # Follow operator logs in real-time
  kubectl odh logs operator -f

  # Show last 100 lines of operator logs
  kubectl odh logs operator --tail 100

  # Show logs from the last 30 minutes
  kubectl odh logs operator --since 30m

  # Show previous container logs (after a crash)
  kubectl odh logs operator --previous
`

// AddCommand adds the logs command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := logspkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           "logs TARGET",
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return []string{"operator"}, cobra.ShellCompDirectiveNoFileComp
			}

			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			command.Target = args[0]

			if err := command.Complete(); err != nil {
				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			if err := command.Validate(); err != nil {
				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			if err := command.Run(cmd.Context()); err != nil {
				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			return nil
		},
	}

	command.AddFlags(cmd.Flags())

	root.AddCommand(cmd)
}
