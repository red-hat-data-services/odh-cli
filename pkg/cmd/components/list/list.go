package list

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/lburgazzoli/odh-cli/pkg/components"
	"github.com/lburgazzoli/odh-cli/pkg/printer/table"
	utilclient "github.com/lburgazzoli/odh-cli/pkg/util/client"
)

type ListOptions struct {
	configFlags *genericclioptions.ConfigFlags
	streams     genericclioptions.IOStreams

	OutputFormat string

	client *utilclient.Client
}

func NewListOptions(
	streams genericclioptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *ListOptions {
	return &ListOptions{
		configFlags: configFlags,
		streams:     streams,
	}
}

func (o *ListOptions) Complete(cmd *cobra.Command, args []string) error {
	var err error

	o.client, err = utilclient.NewClient(o.configFlags)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return nil
}

func (o *ListOptions) Validate() error {
	validFormats := []string{"table", "json", "yaml"}
	for _, format := range validFormats {
		if o.OutputFormat == format {
			return nil
		}
	}

	return fmt.Errorf("unsupported output format: %s (supported: table, json, yaml)", o.OutputFormat)
}

func (o *ListOptions) Run() error {
	ctx := context.Background()

	componentList, err := components.ListComponents(ctx, o.client)
	if err != nil {
		return fmt.Errorf("failed to list components: %w", err)
	}

	switch o.OutputFormat {
	case "json":
		encoder := json.NewEncoder(o.streams.Out)
		encoder.SetIndent("", "  ")

		if err := encoder.Encode(componentList); err != nil {
			return fmt.Errorf("failed to encode components as JSON: %w", err)
		}

		return nil
	case "yaml":
		yamlData, err := yaml.Marshal(componentList)
		if err != nil {
			return fmt.Errorf("failed to marshal as YAML: %w", err)
		}
		fmt.Fprint(o.streams.Out, string(yamlData))
		return nil
	case "table":
		renderer := table.NewWithColumns[unstructured.Unstructured](
			o.streams.Out,
			table.NewColumn("TYPE").
				JQ(`.kind`),
			table.NewColumn("READY").
				JQ(`.status.conditions[]? | select(.type=="Ready") | .status // "Unknown"`),
			table.NewColumn("MESSAGE").
				JQ(`.status.conditions[]? | select(.type=="Ready") | .message // ""`),
		)

		if err := renderer.AppendAll(componentList.Items); err != nil {
			return fmt.Errorf("failed to append rows: %w", err)
		}

		if err := renderer.Render(); err != nil {
			return fmt.Errorf("failed to render table: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s (supported: table, json, yaml)", o.OutputFormat)
	}
}

