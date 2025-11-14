package list

import (
	"os"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	pkgcmd "github.com/lburgazzoli/odh-cli/pkg/cmd/components/list"
)

const (
	cmdName  = "list"
	cmdAlias = "ls"
	cmdShort = "List all components"
	cmdLong  = `List all ODH/RHOAI components from the components.platform.opendatahub.io API group.

Components are cluster-scoped resources.`
)

// AddCommand adds the list subcommand to the components command.
func AddCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags) {
	o := pkgcmd.NewListOptions(
		genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
		flags,
	)

	cmd := &cobra.Command{
		Use:          cmdName,
		Aliases:      []string{cmdAlias},
		Short:        cmdShort,
		Long:         cmdLong,
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

	cmd.Flags().StringVarP(&o.OutputFormat, "output", "o", "table", "Output format (table|json|yaml)")

	parent.AddCommand(cmd)
}
