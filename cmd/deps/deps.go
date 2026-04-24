package deps

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	depspkg "github.com/opendatahub-io/odh-cli/pkg/deps"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "deps"
	cmdShort = "Show operator dependencies for ODH/RHOAI"
)

const cmdLong = `
Shows operator dependencies required by ODH/RHOAI components.

Displays dependency status by querying OLM subscriptions on the cluster.
Each dependency shows:
  - Installation status (installed, missing, optional)
  - Installed version (if available)
  - Namespace where the operator runs
  - Components that require this dependency

Examples:
  # Show all dependencies
  kubectl odh deps

  # Show dependencies for a specific version
  kubectl odh deps --version 3.4.0

  # Refresh manifest from odh-gitops (fetch latest)
  kubectl odh deps --refresh

  # Output as JSON
  kubectl odh deps -o json

  # Dry run (show manifest data without cluster query)
  kubectl odh deps --dry-run
`

// AddCommand adds the deps command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := depspkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat := command.Output

			if err := command.Complete(); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			if err := command.Validate(); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			if err := command.Run(cmd.Context()); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			return nil
		},
	}

	command.AddFlags(cmd.Flags())

	root.AddCommand(cmd)
}
