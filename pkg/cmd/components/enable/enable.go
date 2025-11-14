package enable

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	utilclient "github.com/lburgazzoli/odh-cli/pkg/util/client"
)

type EnableOptions struct {
	configFlags *genericclioptions.ConfigFlags
	streams     genericclioptions.IOStreams

	componentType string

	client *utilclient.Client
}

func NewEnableOptions(
	streams genericclioptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *EnableOptions {
	return &EnableOptions{
		configFlags: configFlags,
		streams:     streams,
	}
}

func (o *EnableOptions) Complete(cmd *cobra.Command, args []string) error {
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

func (o *EnableOptions) Validate() error {
	if o.componentType == "" {
		return fmt.Errorf("component type is required")
	}

	return nil
}

func (o *EnableOptions) Run() error {
	return fmt.Errorf("enable command not yet implemented")
}

