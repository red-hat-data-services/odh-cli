package get

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/lburgazzoli/odh-cli/pkg/components"
	utilclient "github.com/lburgazzoli/odh-cli/pkg/util/client"
)

type GetOptions struct {
	configFlags *genericclioptions.ConfigFlags
	streams     genericclioptions.IOStreams

	OutputFormat  string
	componentType string

	client *utilclient.Client
}

func NewGetOptions(
	streams genericclioptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *GetOptions {
	return &GetOptions{
		configFlags: configFlags,
		streams:     streams,
	}
}

func (o *GetOptions) Complete(cmd *cobra.Command, args []string) error {
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

func (o *GetOptions) Validate() error {
	if o.componentType == "" {
		return fmt.Errorf("component type is required")
	}

	validFormats := []string{"json", "yaml"}
	for _, format := range validFormats {
		if o.OutputFormat == format {
			return nil
		}
	}

	return fmt.Errorf("unsupported output format: %s (supported: json, yaml)", o.OutputFormat)
}

func (o *GetOptions) Run() error {
	ctx := context.Background()

	component, err := components.GetComponentByType(ctx, o.client, o.componentType)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}

	switch o.OutputFormat {
	case "json":
		encoder := json.NewEncoder(o.streams.Out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(component); err != nil {
			return fmt.Errorf("failed to encode as JSON: %w", err)
		}
	case "yaml":
		yamlData, err := yaml.Marshal(component)
		if err != nil {
			return fmt.Errorf("failed to marshal as YAML: %w", err)
		}
		fmt.Fprint(o.streams.Out, string(yamlData))
	default:
		return fmt.Errorf("unsupported output format: %s (supported: json, yaml)", o.OutputFormat)
	}

	return nil
}

