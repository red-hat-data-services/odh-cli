package get

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

var _ cmd.Command = (*Command)(nil)

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
	outputFormatYAML  = "yaml"

	msgCRDNotInstalled = "resource type %q is not installed on this cluster\n"
)

// Command contains the get command configuration.
type Command struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	// ResourceName is the positional arg identifying the resource type (e.g. "nb").
	ResourceName string
	// ItemName is the optional positional arg for a specific resource name.
	ItemName string

	// Namespace is the resolved target namespace (populated during Complete).
	Namespace string
	// AllNamespaces lists resources across all namespaces when true.
	AllNamespaces bool
	// LabelSelector filters resources by label (e.g. "app=my-model").
	LabelSelector string
	// OutputFormat controls output rendering: table, json, or yaml.
	OutputFormat string
	// Verbose enables verbose output.
	Verbose bool
	// Quiet suppresses all non-essential output.
	Quiet bool

	// ResolvedType is the ResourceType resolved from ResourceName during Complete.
	ResolvedType resources.ResourceType
}

// NewCommand creates a new get Command with defaults.
func NewCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *Command {
	return &Command{
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags:  configFlags,
		OutputFormat: outputFormatTable,
	}
}

// AddFlags registers command-specific flags.
// Note: -n/--namespace is already registered by ConfigFlags on the root command.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&c.AllNamespaces, "all-namespaces", "A", false, "List resources across all namespaces")
	fs.StringVarP(&c.LabelSelector, "selector", "l", "", "Label selector to filter resources (e.g. app=my-model)")
	fs.StringVarP(&c.OutputFormat, "output", "o", outputFormatTable, "Output format: table, json, or yaml")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, "Enable verbose output")
	fs.BoolVarP(&c.Quiet, "quiet", "q", false, "Suppress all non-essential output")
}

// Complete resolves derived fields after flag parsing.
func (c *Command) Complete() error {
	if c.Verbose && c.Quiet {
		return errors.New("--verbose and --quiet are mutually exclusive")
	}

	rt, err := Resolve(c.ResourceName)
	if err != nil {
		return fmt.Errorf("resolving resource type: %w", err)
	}

	c.ResolvedType = rt

	if !c.AllNamespaces {
		ns, _, err := c.ConfigFlags.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return fmt.Errorf("determining namespace: %w", err)
		}

		c.Namespace = ns
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
func (c *Command) Validate() error {
	if c.AllNamespaces && c.ConfigFlags.Namespace != nil && *c.ConfigFlags.Namespace != "" {
		return errors.New("--all-namespaces and --namespace are mutually exclusive")
	}

	switch c.OutputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatYAML:
	default:
		return fmt.Errorf("invalid output format %q (must be one of: table, json, yaml)", c.OutputFormat)
	}

	return nil
}

// Run executes the get command.
func (c *Command) Run(ctx context.Context) error {
	if c.ItemName != "" {
		return c.getResource(ctx)
	}

	return c.listResources(ctx)
}

// listResources lists resources of the resolved type.
func (c *Command) listResources(ctx context.Context) error {
	opts := []client.ListResourcesOption{}
	if c.Namespace != "" {
		opts = append(opts, client.WithNamespace(c.Namespace))
	}

	if c.LabelSelector != "" {
		opts = append(opts, client.WithLabelSelector(c.LabelSelector))
	}

	items, err := c.Client.List(ctx, c.ResolvedType, opts...)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			c.IO.Fprintf(msgCRDNotInstalled, c.ResolvedType.Kind)

			return nil
		}

		if !client.IsPermissionError(err) {
			return fmt.Errorf("listing %s: %w", c.ResolvedType.Kind, err)
		}

		c.IO.Errorf("Warning: insufficient permissions to list %s", c.ResolvedType.Kind)
		items = []*unstructured.Unstructured{}
	}

	return c.renderOutput(items)
}

// getResource retrieves a single named resource.
func (c *Command) getResource(ctx context.Context) error {
	opts := []client.GetOption{}
	if c.Namespace != "" {
		opts = append(opts, client.InNamespace(c.Namespace))
	}

	// Verify resource type exists by attempting a lightweight List.
	// This reliably detects CRD-missing vs resource-missing since List returns
	// empty results (no error) when CRD exists but no resources match.
	listOpts := []client.ListResourcesOption{client.WithLimit(1)}
	if c.Namespace != "" {
		listOpts = append(listOpts, client.WithNamespace(c.Namespace))
	}

	_, listErr := c.Client.List(ctx, c.ResolvedType, listOpts...)
	if listErr != nil {
		if client.IsResourceTypeNotFound(listErr) {
			c.IO.Fprintf(msgCRDNotInstalled, c.ResolvedType.Kind)

			return nil
		}

		return fmt.Errorf("verifying resource type %s: %w", c.ResolvedType.Kind, listErr)
	}

	item, err := c.Client.GetResource(ctx, c.ResolvedType, c.ItemName, opts...)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if c.Namespace == "" {
				return fmt.Errorf("%s %q not found: %w", c.ResolvedType.Kind, c.ItemName, err)
			}

			return fmt.Errorf("%s %q not found in namespace %q: %w", c.ResolvedType.Kind, c.ItemName, c.Namespace, err)
		}

		return fmt.Errorf("getting %s %q: %w", c.ResolvedType.Kind, c.ItemName, err)
	}

	if item == nil {
		c.IO.Errorf("Warning: insufficient permissions to get %s %q", c.ResolvedType.Kind, c.ItemName)

		return nil
	}

	return c.renderOutput([]*unstructured.Unstructured{item})
}

// renderOutput dispatches to the appropriate output formatter.
func (c *Command) renderOutput(items []*unstructured.Unstructured) error {
	switch c.OutputFormat {
	case outputFormatJSON:
		return outputJSON(c.IO.Out(), items)
	case outputFormatYAML:
		return outputYAML(c.IO.Out(), items)
	default:
		return outputTable(c.IO.Out(), items, c.ResolvedType, c.AllNamespaces)
	}
}
