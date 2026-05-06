package status

import (
	"io"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	statuspkg "github.com/opendatahub-io/odh-cli/pkg/status"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

// handleErr writes the error in structured or text format and returns an already-handled error.
//
//nolint:wrapcheck // NewAlreadyHandledError is a sentinel, not meant to be wrapped
func handleErr(w io.Writer, err error, outputFormat string) error {
	if clierrors.WriteStructuredError(w, err, outputFormat) {
		return clierrors.NewAlreadyHandledError(err)
	}

	clierrors.WriteTextError(w, err)

	return clierrors.NewAlreadyHandledError(err)
}

const (
	cmdName  = "status"
	cmdShort = "Show platform health and version information"
)

const cmdLong = `
Shows the health status of an OpenShift AI / Open Data Hub installation.

Runs health checks across eight sections and displays a summary table:
  Operator, DSCI, DSC, Nodes, Deployments, Pods, Quotas, Events

The applications and operator namespaces are auto-detected from the
DSCInitialization resource and OLM ClusterServiceVersions. Use
--apps-namespace and --operator-namespace to override.

Examples:
  # Show platform health summary
  kubectl odh status

  # Show detailed per-item output
  kubectl odh status --verbose

  # Check only nodes and deployments
  kubectl odh status --section nodes --section deployments

  # Output full report as JSON
  kubectl odh status -o json
`

const cmdExample = `
  # Show platform health summary
  kubectl odh status

  # Verbose output with per-item details
  kubectl odh status --verbose

  # Filter to specific sections
  kubectl odh status --section operator --section dsci --section dsc

  # JSON output for scripting
  kubectl odh status -o json

  # Override namespace detection
  kubectl odh status --apps-namespace my-apps --operator-namespace my-operator
`

// AddCommand adds the status command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := statuspkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			errOut := cmd.ErrOrStderr()
			outputFormat := string(command.OutputFormat)

			if err := command.Complete(); err != nil {
				return handleErr(errOut, err, outputFormat)
			}

			if err := command.Validate(); err != nil {
				return handleErr(errOut, err, outputFormat)
			}

			if err := command.Run(cmd.Context()); err != nil {
				return handleErr(errOut, err, outputFormat)
			}

			return nil
		},
	}

	command.AddFlags(cmd.Flags())

	root.AddCommand(cmd)
}
