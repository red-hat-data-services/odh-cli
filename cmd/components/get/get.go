package get

import (
	"os"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	pkgcmd "github.com/lburgazzoli/odh-cli/pkg/cmd/components/get"
)

const (
	cmdName  = "get"
	cmdShort = "Get a specific component by type"
	cmdLong  = `Get a specific ODH/RHOAI component by type name.

The command intelligently matches the component type (case-insensitive) and
returns the singleton instance of that component type.

Components are cluster-scoped resources and follow a singleton pattern - each
type typically has one instance (e.g., "default-kserve", "default-dashboard").

Examples:
  kubectl odh components get kserve
  kubectl odh components get dashboard
  kubectl odh components get DataSciencePipelines`
)

// AddCommand adds the get subcommand to the components command.
func AddCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags) {
	o := pkgcmd.NewGetOptions(
		genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
		flags,
	)

	cmd := &cobra.Command{
		Use:          cmdName + " <component-type>",
		Short:        cmdShort,
		Long:         cmdLong,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run()
		},
	}

	cmd.Flags().StringVarP(&o.OutputFormat, "output", "o", "json", "Output format (json|yaml)")

	parent.AddCommand(cmd)
}
