package disable

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	utilclient "github.com/lburgazzoli/odh-cli/pkg/util/client"
)

type DisableOptions struct {
	configFlags *genericclioptions.ConfigFlags
	streams     genericclioptions.IOStreams

	componentType string

	client *utilclient.Client
}

func NewDisableOptions(
	streams genericclioptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *DisableOptions {
	return &DisableOptions{
		configFlags: configFlags,
		streams:     streams,
	}
}

func (o *DisableOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.componentType = args[0]
	}

	var err error

	o.client, err = utilclient.NewClient(o.configFlags)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return nil
}

func (o *DisableOptions) Validate() error {
	if o.componentType == "" {
		return fmt.Errorf("component type is required")
	}

	return nil
}

func (o *DisableOptions) Run() error {
	return fmt.Errorf("disable command not yet implemented")
}

