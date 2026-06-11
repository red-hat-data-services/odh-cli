package events

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/pflag"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

var _ cmd.Command = (*Command)(nil)

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
	outputFormatYAML  = "yaml"

	namespaceDiscoveryTimeout = 15 * time.Second
	defaultSinceDuration      = 5 * time.Minute
	maxSinceDuration          = 24 * time.Hour
)

// Command contains the events command configuration.
type Command struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags
	Client      client.Client

	// Flags
	EventType          string
	Since              time.Duration
	AllNamespaces      bool
	OutputFormat       string
	OperatorNSOverride string
	Component          string

	// Resolved fields (populated during Complete)
	Namespace         string
	NamespaceExplicit bool // true if user explicitly passed -n flag
	ApplicationsNS    string
	OperatorNS        string
	MonitoringNS      string

	// Clusterhealth integration
	crClient crclient.Client
}

// NewCommand creates a new events Command with defaults.
func NewCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *Command {
	return &Command{
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags:  configFlags,
		OutputFormat: outputFormatTable,
		OperatorNS:   client.DefaultRHOAIOperatorNamespace,
	}
}

// AddFlags registers command-specific flags.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.EventType, "type", "", "Filter events by type (Warning or Normal)")
	fs.DurationVar(&c.Since, "since", defaultSinceDuration, "Only show events newer than this duration (e.g., 5m, 1h, 30s)")
	fs.BoolVarP(&c.AllNamespaces, "all-namespaces", "A", false, "List events across all ODH namespaces")
	fs.StringVarP(&c.OutputFormat, "output", "o", outputFormatTable, "Output format: table, json, or yaml")
	fs.StringVar(&c.OperatorNSOverride, "operator-namespace", "", "Override the operator namespace (auto-detected from OLM/CSV)")
	fs.StringVar(&c.Component, "component", "", "Filter events by ODH component (dashboard, kserve, ray, etc.). Only filters core Kubernetes resources (Pods, Deployments, etc.); CRD-specific events are excluded")
}

// Complete resolves derived fields after flag parsing.
func (c *Command) Complete() error {
	restConfig, err := client.NewRESTConfig(c.ConfigFlags, client.DefaultQPS, client.DefaultBurst)
	if err != nil {
		return clierrors.ErrConfigFailed(err)
	}

	k8sClient, err := client.NewClientWithConfig(restConfig)
	if err != nil {
		return clierrors.ErrClientFailed(err)
	}

	c.Client = k8sClient

	crClient, err := client.NewControllerRuntimeClient(restConfig)
	if err != nil {
		return clierrors.ErrCRClientFailed(err)
	}

	c.crClient = crClient

	// Check if user explicitly passed -n flag (vs namespace from kubeconfig)
	c.NamespaceExplicit = c.ConfigFlags.Namespace != nil && *c.ConfigFlags.Namespace != ""

	if !c.AllNamespaces {
		ns, _, err := c.ConfigFlags.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return clierrors.ErrNamespaceFailed(err)
		}

		c.Namespace = ns
	}

	return nil
}

// Validate checks that all options are valid before execution.
func (c *Command) Validate() error {
	if c.AllNamespaces && c.ConfigFlags.Namespace != nil && *c.ConfigFlags.Namespace != "" {
		return clierrors.NewValidationError(
			"INVALID_FLAGS",
			"--all-namespaces and --namespace are mutually exclusive",
			"Use either -A or -n <namespace>, not both",
		)
	}

	switch c.OutputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatYAML:
	default:
		return clierrors.NewValidationError(
			"INVALID_OUTPUT_FORMAT",
			fmt.Sprintf("invalid output format %q", c.OutputFormat),
			"Use one of: table, json, yaml",
		)
	}

	if c.EventType != "" && c.EventType != "Warning" && c.EventType != "Normal" {
		return clierrors.NewValidationError(
			"INVALID_EVENT_TYPE",
			fmt.Sprintf("invalid event type %q", c.EventType),
			"Use --type Warning or --type Normal",
		)
	}

	if c.Since < 0 {
		return clierrors.NewValidationError(
			"INVALID_DURATION",
			"--since must be a non-negative duration",
			"Use 0 or a positive value like --since 5m or --since 1h",
		)
	}

	if c.Since > maxSinceDuration {
		return clierrors.NewValidationError(
			"INVALID_DURATION",
			fmt.Sprintf("--since duration %v exceeds maximum of 24h", c.Since),
			"Use --since with values like 5m, 1h, or 24h",
		)
	}

	if c.Component != "" && resources.GetComponentCR(c.Component) == nil {
		return clierrors.NewValidationError(
			"INVALID_COMPONENT",
			"invalid component "+c.Component,
			"Use one of: "+strings.Join(ValidComponents(), ", "),
		)
	}

	return nil
}

// validComponents contains sorted list of valid component names.
//
//nolint:gochecknoglobals // Static configuration built once at init
var validComponents []string

//nolint:gochecknoinits // One-time initialization of static component list
func init() {
	validComponents = make([]string, 0, len(resources.ComponentCRResourceTypes))
	for name := range resources.ComponentCRResourceTypes {
		validComponents = append(validComponents, name)
	}

	sort.Strings(validComponents)
}

// ValidComponents returns a copy of the sorted list of valid component names.
func ValidComponents() []string {
	result := make([]string, len(validComponents))
	copy(result, validComponents)

	return result
}

// Run executes the events command.
func (c *Command) Run(ctx context.Context) error {
	if err := c.discoverNamespaces(ctx); err != nil {
		return err
	}

	return c.listEvents(ctx)
}

// discoverNamespaces resolves ODH namespaces using centralized discovery helpers.
// Called from Run() so it respects the command context for cancellation.
func (c *Command) discoverNamespaces(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, namespaceDiscoveryTimeout)
	defer cancel()

	// Use combined helper for single API call
	namespaces, err := client.GetDSCINamespaces(ctx, c.Client)
	if err != nil && !apierrors.IsNotFound(err) {
		_, _ = fmt.Fprintf(c.IO.ErrOut(), "Warning: failed to get DSCI namespaces: %v\n", err)
	}

	c.ApplicationsNS = namespaces.Applications
	c.MonitoringNS = namespaces.Monitoring

	if c.OperatorNSOverride != "" {
		c.OperatorNS = c.OperatorNSOverride

		return nil
	}

	operatorNS, err := client.DiscoverOperatorNamespace(ctx, c.Client)
	if err == nil {
		c.OperatorNS = operatorNS

		return nil
	}

	// Check if this is a "not found" error (operator not installed) vs RBAC/other errors
	var structured *clierrors.StructuredError
	if !errors.As(err, &structured) || structured.Category != clierrors.CategoryNotFound {
		// Forbidden/RBAC errors should fail - user can use --operator-namespace
		return fmt.Errorf("discovering operator namespace: %w", err)
	}

	_, _ = fmt.Fprint(c.IO.ErrOut(), "Warning: operator namespace not found, using default\n")
	c.OperatorNS = client.DefaultRHOAIOperatorNamespace

	return nil
}

// listEvents fetches and displays events.
func (c *Command) listEvents(ctx context.Context) error {
	events, err := c.fetchEvents(ctx)
	if err != nil {
		return err
	}

	if c.Component != "" {
		events, err = c.filterEventsByComponent(ctx, events)
		if err != nil {
			return err
		}
	}

	sortEventsByTime(events)

	return c.renderOutput(events)
}
