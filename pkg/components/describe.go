package components

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
	printeryaml "github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

var _ cmd.Command = (*DescribeCommand)(nil)

const (
	reasonColumnWidth  = 25
	messageColumnWidth = 50
)

// ComponentDetails holds detailed information about a component.
type ComponentDetails struct {
	Name            string             `json:"name"`
	ManagementState string             `json:"managementState"`
	Ready           *bool              `json:"ready,omitempty"`
	Message         string             `json:"message,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
}

// DescribeCommand contains the describe subcommand configuration.
type DescribeCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	ComponentName string
	OutputFormat  string
	Verbose       bool
	Quiet         bool
}

// NewDescribeCommand creates a new DescribeCommand with defaults.
func NewDescribeCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *DescribeCommand {
	return &DescribeCommand{
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags:  configFlags,
		OutputFormat: outputFormatTable,
	}
}

// AddFlags registers command-specific flags.
func (c *DescribeCommand) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&c.OutputFormat, "output", "o", outputFormatTable, "Output format: table, json, or yaml")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, "Enable verbose output")
	fs.BoolVarP(&c.Quiet, "quiet", "q", false, "Suppress all non-essential output")
}

// Complete resolves derived fields after flag parsing.
func (c *DescribeCommand) Complete() error {
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
func (c *DescribeCommand) Validate() error {
	if c.ComponentName == "" {
		return errors.New("component name is required")
	}

	switch c.OutputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatYAML:
	default:
		return ErrInvalidOutputFormat(c.OutputFormat)
	}

	return nil
}

// Run executes the describe command.
func (c *DescribeCommand) Run(ctx context.Context) error {
	component, err := GetComponent(ctx, c.Client, c.ComponentName)
	if err != nil {
		return fmt.Errorf("getting component: %w", err)
	}

	details := &ComponentDetails{
		Name:            component.Name,
		ManagementState: component.ManagementState,
	}

	if component.IsActive() {
		health, err := GetComponentHealth(ctx, c.Client, c.ComponentName)
		if err != nil {
			c.IO.Errorf("warning: could not fetch health: %v", err)
		} else {
			details.Ready = health.Ready
			details.Message = health.Message
			details.Conditions = health.Conditions
		}
	}

	return c.renderOutput(details)
}

// renderOutput dispatches to the appropriate output formatter.
func (c *DescribeCommand) renderOutput(details *ComponentDetails) error {
	switch c.OutputFormat {
	case outputFormatJSON:
		return c.outputJSON(details)
	case outputFormatYAML:
		return c.outputYAML(details)
	default:
		return c.outputTable(details)
	}
}

func (c *DescribeCommand) outputTable(details *ComponentDetails) error {
	c.IO.Fprintf("Name:              %s", details.Name)
	c.IO.Fprintf("Management State:  %s", details.ManagementState)

	if details.Ready != nil {
		ready := readyNo
		if *details.Ready {
			ready = readyYes
		}

		c.IO.Fprintf("Ready:             %s", ready)
	}

	if details.Message != "" {
		c.IO.Fprintf("Message:           %s", details.Message)
	}

	if len(details.Conditions) > 0 {
		c.IO.Fprintln()
		c.IO.Fprintln("Conditions:")
		c.printConditionsTable(details.Conditions)
	}

	return nil
}

func (c *DescribeCommand) printConditionsTable(conditions []metav1.Condition) {
	c.IO.Fprintf("  %-15s %-8s %-25s %s", "TYPE", "STATUS", "REASON", "MESSAGE")

	for _, cond := range conditions {
		c.IO.Fprintf("  %-15s %-8s %-25s %s",
			cond.Type,
			cond.Status,
			truncateString(cond.Reason, reasonColumnWidth),
			truncateString(cond.Message, messageColumnWidth),
		)
	}
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	return string(runes[:maxLen-3]) + "..."
}

func (c *DescribeCommand) outputJSON(details *ComponentDetails) error {
	result := NewComponentDetailsResult(*details)
	renderer := printerjson.NewRenderer[*ComponentDetailsResult](
		printerjson.WithWriter[*ComponentDetailsResult](c.IO.Out()),
	)

	if err := renderer.Render(result); err != nil {
		return fmt.Errorf("rendering JSON: %w", err)
	}

	return nil
}

func (c *DescribeCommand) outputYAML(details *ComponentDetails) error {
	result := NewComponentDetailsResult(*details)
	renderer := printeryaml.NewRenderer[*ComponentDetailsResult](
		printeryaml.WithWriter[*ComponentDetailsResult](c.IO.Out()),
	)

	if err := renderer.Render(result); err != nil {
		return fmt.Errorf("rendering YAML: %w", err)
	}

	return nil
}
