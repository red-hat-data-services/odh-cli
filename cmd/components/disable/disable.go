package disable

import (
	"os"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	pkgcmd "github.com/lburgazzoli/odh-cli/pkg/cmd/components/disable"
)

const (
	cmdName  = "disable"
	cmdShort = "Disable a component"
	cmdLong  = `Disable an ODH/RHOAI component.

This command is not yet implemented.`
)

// AddCommand adds the disable subcommand to the components command.
func AddCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags) {
	o := pkgcmd.NewDisableOptions(
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

	parent.AddCommand(cmd)
}

