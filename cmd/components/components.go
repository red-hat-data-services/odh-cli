package components

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/lburgazzoli/odh-cli/cmd/components/disable"
	"github.com/lburgazzoli/odh-cli/cmd/components/enable"
	"github.com/lburgazzoli/odh-cli/cmd/components/get"
	"github.com/lburgazzoli/odh-cli/cmd/components/list"
)

const (
	cmdName  = "components"
	cmdShort = "Manage ODH/RHOAI components"
	cmdLong  = `Manage ODH/RHOAI components from the components.platform.opendatahub.io API group.

Components are cluster-scoped resources that define ODH/RHOAI platform components.`
)

// AddCommand adds the components subcommand to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	cmd := &cobra.Command{
		Use:          cmdName,
		Short:        cmdShort,
		Long:         cmdLong,
		SilenceUsage: true,
	}

	// Add subcommands
	list.AddCommand(cmd, flags)
	get.AddCommand(cmd, flags)
	enable.AddCommand(cmd, flags)
	disable.AddCommand(cmd, flags)

	root.AddCommand(cmd)
}

