package status

import (
	"context"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
	"github.com/spf13/pflag"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*Command)(nil)

// OutputFormat represents the output format for the status command.
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"

	DefaultTimeout = 30 * time.Second

	// Default operator deployment name for RHOAI.
	defaultRHOAIOperatorName = "rhods-operator"

	// Output format strings.
	fmtPlatformVersion  = "Platform version: %s\n"
	fmtOpenShiftVersion = "OpenShift version: %s\n"
)

const (
	flagDescOutput   = `Output format: "table" or "json"`
	flagDescVerbose  = "Show per-item details for each section"
	flagDescSection  = "Limit output to specific sections (repeatable): nodes, deployments, pods, events, quotas, operator, dsci, dsc"
	flagDescNoColor  = "Disable color output"
	flagDescTimeout  = "Maximum time to wait for health checks to complete"
	flagDescAppsNS   = "Override the applications namespace (auto-detected from DSCI)"
	flagDescOperNS   = "Override the operator namespace (auto-detected from OLM/CSV)"
	flagDescOperName = "Override the operator deployment name (auto-detected from CSV)"
	flagDescInfra    = "Also scan kube-system for core infrastructure health"
	flagDescQPS      = "Kubernetes API queries per second"
	flagDescBurst    = "Kubernetes API burst capacity"
)

// Command contains the status command configuration.
type Command struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags

	OutputFormat OutputFormat
	Verbose      bool
	NoColor      bool
	Timeout      time.Duration
	Sections     []string

	AppsNamespace     string
	OperatorNamespace string
	OperatorName      string
	IncludeInfra      bool

	QPS   float32
	Burst int

	// Populated during Complete
	restConfig *rest.Config
	client     client.Client
}

// NewCommand creates a new status Command with defaults.
func NewCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *Command {
	return &Command{
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags:  configFlags,
		OutputFormat: OutputFormatTable,
		Timeout:      DefaultTimeout,
		QPS:          client.DefaultQPS,
		Burst:        client.DefaultBurst,
	}
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), flagDescOutput)
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescVerbose)
	fs.StringArrayVar(&c.Sections, "section", nil, flagDescSection)
	fs.BoolVar(&c.NoColor, "no-color", false, flagDescNoColor)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescTimeout)
	fs.StringVar(&c.AppsNamespace, "apps-namespace", "", flagDescAppsNS)
	fs.StringVar(&c.OperatorNamespace, "operator-namespace", "", flagDescOperNS)
	fs.StringVar(&c.OperatorName, "operator-name", "", flagDescOperName)
	fs.BoolVar(&c.IncludeInfra, "include-infra", false, flagDescInfra)
	fs.Float32Var(&c.QPS, "qps", c.QPS, flagDescQPS)
	fs.IntVar(&c.Burst, "burst", c.Burst, flagDescBurst)
}

// Complete populates the client and performs pre-validation setup.
func (c *Command) Complete() error {
	restConfig, err := client.NewRESTConfig(c.ConfigFlags, c.QPS, c.Burst)
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	c.restConfig = restConfig

	k8sClient, err := client.NewClientWithConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	c.client = k8sClient

	if c.OutputFormat == OutputFormatJSON {
		c.NoColor = true
	}

	color.NoColor = c.NoColor

	return nil
}

// Validate checks that all required options are valid.
func (c *Command) Validate() error {
	if err := c.OutputFormat.Validate(); err != nil {
		return err
	}

	if err := validateSections(c.Sections); err != nil {
		return err
	}

	if c.Timeout <= 0 {
		return ErrInvalidTimeout()
	}

	return nil
}

// Run executes the status command.
func (c *Command) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	// Fetch DSCI once and reuse across namespace discovery and CR name extraction.
	// NotFound is non-fatal (handled by discoverNamespaces), but other errors propagate.
	dsci, err := client.GetDSCInitialization(ctx, c.client)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("getting DSCInitialization: %w", err)
	}

	nsCfg, opInfo, err := discoverNamespaces(ctx, c.client, dsci, c.AppsNamespace, c.OperatorNamespace)
	if err != nil {
		return fmt.Errorf("discovering namespaces: %w", err)
	}

	dsciName, dscName, err := discoverCRNames(ctx, c.client, dsci)
	if err != nil {
		return err
	}

	crClient, err := client.NewControllerRuntimeClient(c.restConfig)
	if err != nil {
		return fmt.Errorf("creating controller-runtime client: %w", err)
	}

	operatorName := discoverOperatorName(opInfo, c.OperatorName)

	nsCfgHealth := clusterhealth.NamespaceConfig{
		Apps:       nsCfg.Apps,
		Monitoring: nsCfg.Monitoring,
	}

	if c.IncludeInfra {
		nsCfgHealth.Extra = []string{"kube-system"}
	}

	cfg := clusterhealth.Config{
		Client: crClient,
		Operator: clusterhealth.OperatorConfig{
			Namespace: nsCfg.Operator,
			Name:      operatorName,
		},
		Namespaces:   nsCfgHealth,
		DSCI:         dsciName,
		DSC:          dscName,
		OnlySections: c.Sections,
	}

	report, err := clusterhealth.Run(ctx, cfg)
	if err != nil {
		return fmt.Errorf("running health checks: %w", err)
	}

	return c.output(ctx, report)
}

// output renders the report in the requested format.
func (c *Command) output(ctx context.Context, report *clusterhealth.Report) error {
	switch c.OutputFormat {
	case OutputFormatTable:
		return c.outputTable(ctx, report)
	case OutputFormatJSON:
		return c.outputJSON(report)
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

// outputTable renders the report as a human-readable table.
func (c *Command) outputTable(ctx context.Context, report *clusterhealth.Report) error {
	w := c.IO.Out()

	ver, err := version.Detect(ctx, c.client)
	if err == nil && ver != nil {
		if _, err := fmt.Fprintf(w, fmtPlatformVersion, ver.String()); err != nil {
			return fmt.Errorf("writing platform version: %w", err)
		}
	}

	ocpVer, err := version.DetectOpenShiftVersion(ctx, c.client)
	if err == nil && ocpVer != nil {
		if _, err := fmt.Fprintf(w, fmtOpenShiftVersion, ocpVer.String()); err != nil {
			return fmt.Errorf("writing OpenShift version: %w", err)
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("writing newline: %w", err)
	}

	if _, err := fmt.Fprint(w, report.PrettyPrint(c.Verbose)); err != nil {
		return fmt.Errorf("writing health report: %w", err)
	}

	return nil
}

// outputJSON renders the report as JSON.
func (c *Command) outputJSON(report *clusterhealth.Report) error {
	return renderJSON(c.IO.Out(), report)
}

// Validate checks if the output format is valid.
func (o OutputFormat) Validate() error {
	switch o {
	case OutputFormatTable, OutputFormatJSON:
		return nil
	default:
		return ErrInvalidOutputFormat(string(o))
	}
}

func isValidSection(section string) bool {
	switch section {
	case clusterhealth.SectionNodes,
		clusterhealth.SectionDeployments,
		clusterhealth.SectionPods,
		clusterhealth.SectionEvents,
		clusterhealth.SectionQuotas,
		clusterhealth.SectionOperator,
		clusterhealth.SectionDSCI,
		clusterhealth.SectionDSC:
		return true
	default:
		return false
	}
}

func validateSections(sections []string) error {
	for _, s := range sections {
		if !isValidSection(s) {
			return ErrInvalidSection(s)
		}
	}

	return nil
}

// discoverCRNames returns NamespacedNames for DSCI and DSC singletons.
// Uses the pre-fetched DSCI and fetches DSC separately.
// Returns zero-value names if either CR is not found (non-fatal).
// Non-NotFound errors are propagated.
//
//nolint:nonamedreturns // named returns improve readability for multiple NamespacedName values
func discoverCRNames(
	ctx context.Context,
	c client.Reader,
	dsci *unstructured.Unstructured,
) (dsciName, dscName types.NamespacedName, err error) {
	if dsci != nil {
		dsciName = types.NamespacedName{
			Namespace: dsci.GetNamespace(),
			Name:      dsci.GetName(),
		}
	}

	dsc, err := client.GetDataScienceCluster(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return dsciName, dscName, nil
		}

		return dsciName, dscName, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	if dsc != nil {
		dscName = types.NamespacedName{
			Namespace: dsc.GetNamespace(),
			Name:      dsc.GetName(),
		}
	}

	return dsciName, dscName, nil
}
