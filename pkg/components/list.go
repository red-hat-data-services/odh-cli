package components

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
	printeryaml "github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

var _ cmd.Command = (*ListCommand)(nil)

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
	outputFormatYAML  = "yaml"
)

const (
	colName  = "NAME"
	colState = "STATE"
	colReady = "READY"
	colMsg   = "MESSAGE"
)

// ListCommand contains the components list command configuration.
type ListCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	OutputFormat string
	Verbose      bool
	Quiet        bool
}

// NewListCommand creates a new ListCommand with defaults.
func NewListCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *ListCommand {
	return &ListCommand{
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags:  configFlags,
		OutputFormat: outputFormatTable,
	}
}

// AddFlags registers command-specific flags.
func (c *ListCommand) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&c.OutputFormat, "output", "o", outputFormatTable, "Output format: table, json, or yaml")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, "Enable verbose output")
	fs.BoolVarP(&c.Quiet, "quiet", "q", false, "Suppress all non-essential output")
}

// Complete resolves derived fields after flag parsing.
func (c *ListCommand) Complete() error {
	if c.Verbose && c.Quiet {
		return errors.New("--verbose and --quiet are mutually exclusive")
	}

	k8sClient, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	c.Client = k8sClient

	// Wrap IO only when --quiet is explicitly passed
	if c.Quiet {
		c.IO = iostreams.NewFullQuietWrapper(c.IO)
	}

	return nil
}

// Validate checks that all options are valid before execution.
func (c *ListCommand) Validate() error {
	switch c.OutputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatYAML:
	default:
		return ErrInvalidOutputFormat(c.OutputFormat)
	}

	return nil
}

// Run executes the components list command.
func (c *ListCommand) Run(ctx context.Context) error {
	components, err := DiscoverComponents(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("discovering components: %w", err)
	}

	components = EnrichWithHealth(ctx, c.Client, components)

	return c.renderOutput(components)
}

// renderOutput dispatches to the appropriate output formatter.
func (c *ListCommand) renderOutput(components []ComponentInfo) error {
	switch c.OutputFormat {
	case outputFormatJSON:
		return OutputJSON(c.IO.Out(), components)
	case outputFormatYAML:
		return OutputYAML(c.IO.Out(), components)
	default:
		return OutputTable(c.IO.Out(), components)
	}
}

// readyFormatter converts Ready bool pointer to display string.
func readyFormatter(value any) any {
	switch v := value.(type) {
	case bool:
		if v {
			return readyYes
		}

		return readyNo
	case *bool:
		if v == nil {
			return "-"
		}

		if *v {
			return readyYes
		}

		return readyNo
	case nil:
		return "-"
	default:
		return readyUnknown
	}
}

// componentColumns returns the base table columns for component list.
func componentColumns() []table.Column {
	return []table.Column{
		table.NewColumn(colName).JQ(".name"),
		table.NewColumn(colState).JQ(".managementState"),
		table.NewColumn(colReady).JQ(".ready").Fn(readyFormatter),
	}
}

// messageColumn returns the optional message column.
func messageColumn() table.Column {
	return table.NewColumn(colMsg).JQ(`.message // ""`)
}

// OutputTable renders components as a formatted table.
func OutputTable(w io.Writer, components []ComponentInfo) error {
	columns := componentColumns()

	for _, c := range components {
		if c.Message != "" {
			columns = append(columns, messageColumn())

			break
		}
	}

	renderer := table.NewWithColumns[ComponentInfo](w, columns...)

	for _, c := range components {
		if err := renderer.Append(c); err != nil {
			return fmt.Errorf("rendering row: %w", err)
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("rendering table: %w", err)
	}

	return nil
}

// OutputJSON renders components as JSON.
func OutputJSON(w io.Writer, components []ComponentInfo) error {
	list := NewComponentList(components)

	renderer := printerjson.NewRenderer[*ComponentList](
		printerjson.WithWriter[*ComponentList](w),
	)

	if err := renderer.Render(list); err != nil {
		return fmt.Errorf("rendering JSON: %w", err)
	}

	return nil
}

// OutputYAML renders components as YAML.
func OutputYAML(w io.Writer, components []ComponentInfo) error {
	list := NewComponentList(components)

	renderer := printeryaml.NewRenderer[*ComponentList](
		printeryaml.WithWriter[*ComponentList](w),
	)

	if err := renderer.Render(list); err != nil {
		return fmt.Errorf("rendering YAML: %w", err)
	}

	return nil
}
