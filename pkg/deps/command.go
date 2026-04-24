package deps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	utilserrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// Verify Command implements cmd.Command interface at compile time.
var _ cmd.Command = (*Command)(nil)

const (
	outputTable = "table"
	outputJSON  = "json"
	outputYAML  = "yaml"

	yamlIndent         = 2
	tableWidthNormal   = 130
	tableWidthExpanded = 150
	semverParts        = 3

	msgCreateClient       = "create kubernetes client: %w"
	msgInvalidOutput      = "invalid output format %q: must be table, json, or yaml"
	msgNoManifestVersion  = "failed to determine manifest version from embedded Chart.yaml"
	msgVersionUnavailable = "dependency graph for version %q is not available; only version %s is supported"
	msgGetManifest        = "get manifest: %w"
	msgVersionMismatch    = "cluster version %s does not match dependency graph version %s; use --dry-run to view the manifest without cluster validation"
	msgCheckDeps          = "check dependencies: %w"
	msgEncodeJSON         = "encode json: %w"
	msgEncodeYAML         = "encode yaml: %w"

	suggestionVersionUnavailable        = "Use --refresh to fetch the latest manifest, or omit --version to use the embedded version"
	suggestionVersionUnavailableRefresh = "Omit --version to use the fetched manifest version"
	suggestionVersionMismatch           = "Use --dry-run to view the manifest without cluster validation"
)

// Command holds the deps command configuration.
type Command struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags

	Refresh bool
	DryRun  bool
	Output  string
	Version string

	client          client.Client
	clusterVersion  string
	manifestVersion string
	useColor        bool
}

// NewCommand creates a new Command with defaults.
func NewCommand(streams genericiooptions.IOStreams, configFlags *genericclioptions.ConfigFlags) *Command {
	return &Command{
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags: configFlags,
		Output:      outputTable,
	}
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.Refresh, "refresh", false, "Fetch latest manifest from odh-gitops")
	fs.BoolVar(&c.DryRun, "dry-run", false, "Show manifest data without querying cluster")
	fs.StringVarP(&c.Output, "output", "o", outputTable, "Output format: table, json, yaml")
	fs.StringVar(&c.Version, "version", "", "ODH/RHOAI version to show dependencies for")
}

// Complete prepares the command for execution.
func (c *Command) Complete() error {
	c.useColor = shouldUseColor(c.IO.Out())

	if c.DryRun {
		return nil
	}

	cl, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf(msgCreateClient, err)
	}

	c.client = cl

	return nil
}

// shouldUseColor checks if colored output should be used.
// Returns false if NO_COLOR env is set, the writer is not an *os.File, or not a terminal.
func shouldUseColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	return term.IsTerminal(int(f.Fd()))
}

// Validate checks the command options.
func (c *Command) Validate() error {
	switch c.Output {
	case outputTable, outputJSON, outputYAML:
	default:
		return fmt.Errorf(msgInvalidOutput, c.Output)
	}

	if c.Refresh {
		return nil
	}

	manifestVer, err := ManifestVersion()
	if err != nil {
		return fmt.Errorf("%s: %w", msgNoManifestVersion, err)
	}

	c.manifestVersion = manifestVer

	if c.Version != "" && !majorMinorMatch(c.Version, c.manifestVersion) {
		return utilserrors.NewValidationError(
			"VERSION_UNAVAILABLE",
			fmt.Sprintf(msgVersionUnavailable, c.Version, c.manifestVersion),
			suggestionVersionUnavailable,
		)
	}

	return nil
}

// Run executes the command.
func (c *Command) Run(ctx context.Context) error {
	result, err := GetManifest(ctx, c.Refresh)
	if err != nil {
		return fmt.Errorf(msgGetManifest, err)
	}

	if result.Version != "" {
		c.manifestVersion = result.Version
	}

	if c.Version != "" && !majorMinorMatch(c.Version, c.manifestVersion) {
		suggestion := suggestionVersionUnavailable
		if c.Refresh {
			suggestion = suggestionVersionUnavailableRefresh
		}

		return utilserrors.NewValidationError(
			"VERSION_UNAVAILABLE",
			fmt.Sprintf(msgVersionUnavailable, c.Version, c.manifestVersion),
			suggestion,
		)
	}

	if c.DryRun {
		return c.printDryRun(result.Manifest)
	}

	ver, err := version.Detect(ctx, c.client)
	if err != nil {
		_, _ = fmt.Fprintf(c.IO.ErrOut(), "Warning: failed to detect cluster version: %v\n", err)
	} else if ver != nil {
		c.clusterVersion = ver.String()

		if !majorMinorMatch(c.clusterVersion, c.manifestVersion) {
			return utilserrors.NewValidationError(
				"VERSION_MISMATCH",
				fmt.Sprintf(msgVersionMismatch, c.clusterVersion, c.manifestVersion),
				suggestionVersionMismatch,
			)
		}
	}

	statuses, err := CheckDependencies(ctx, c.client.OLM(), result.Manifest)
	if err != nil {
		return fmt.Errorf(msgCheckDeps, err)
	}

	return c.printResults(statuses)
}

func (c *Command) printDryRun(manifest *Manifest) error {
	deps := manifest.GetDependencies()

	switch c.Output {
	case outputJSON:
		return c.printJSON(deps)
	case outputYAML:
		return c.printYAML(deps)
	default:
		return c.printDryRunTable(deps)
	}
}

func (c *Command) printResults(statuses []DependencyStatus) error {
	switch c.Output {
	case outputJSON:
		return c.printJSON(statuses)
	case outputYAML:
		return c.printYAML(statuses)
	default:
		return c.printTable(statuses)
	}
}

func (c *Command) printTable(statuses []DependencyStatus) error {
	w := c.IO.Out()

	_, _ = fmt.Fprintf(w, "Dependency graph: ODH/RHOAI %s\n", c.manifestVersion)
	if c.clusterVersion != "" {
		_, _ = fmt.Fprintf(w, "Installed version: %s\n", c.clusterVersion)
	}

	_, _ = fmt.Fprintln(w)

	_, _ = fmt.Fprintf(w, "%-26s %-10s %-8s %-42s %s\n",
		"OPERATOR", "STATUS", "VERSION", "NAMESPACE", "REQUIRED BY")
	_, _ = fmt.Fprint(w, strings.Repeat("-", tableWidthNormal)+"\n")

	for _, s := range statuses {
		statusIcon := c.statusToIcon(s.Status)

		ver := s.Version
		if ver == "" {
			ver = "-"
		}

		requiredBy := strings.Join(s.RequiredBy, ", ")
		if requiredBy == "" {
			requiredBy = "-"
		}

		_, _ = fmt.Fprintf(w, "%-26s %-10s %-8s %-42s %s\n",
			s.DisplayName,
			statusIcon,
			ver,
			s.Namespace,
			requiredBy,
		)
	}

	return nil
}

func (c *Command) printDryRunTable(deps []DependencyInfo) error {
	w := c.IO.Out()

	_, _ = fmt.Fprintf(w, "[DRY RUN] Dependency graph for ODH/RHOAI %s (no cluster query)\n\n", c.manifestVersion)

	_, _ = fmt.Fprintf(w, "%-25s %-10s %-35s %-35s %s\n",
		"OPERATOR", "ENABLED", "SUBSCRIPTION", "NAMESPACE", "REQUIRED BY")
	_, _ = fmt.Fprint(w, strings.Repeat("-", tableWidthExpanded)+"\n")

	for _, d := range deps {
		requiredBy := strings.Join(d.RequiredBy, ", ")
		if requiredBy == "" {
			requiredBy = "-"
		}

		_, _ = fmt.Fprintf(w, "%-25s %-10s %-35s %-35s %s\n",
			d.DisplayName,
			d.Enabled,
			d.Subscription,
			d.Namespace,
			requiredBy,
		)
	}

	return nil
}

func (c *Command) printJSON(data any) error {
	encoder := json.NewEncoder(c.IO.Out())
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf(msgEncodeJSON, err)
	}

	return nil
}

func (c *Command) printYAML(data any) error {
	encoder := yaml.NewEncoder(c.IO.Out())
	encoder.SetIndent(yamlIndent)

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf(msgEncodeYAML, err)
	}

	return nil
}

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

func (c *Command) statusToIcon(status Status) string {
	switch status {
	case StatusInstalled:
		if c.useColor {
			return colorGreen + "✓ installed" + colorReset
		}

		return "✓ installed"
	case StatusMissing:
		if c.useColor {
			return colorRed + "✗ MISSING" + colorReset
		}

		return "✗ MISSING"
	case StatusOptional:
		if c.useColor {
			return colorYellow + "○ optional" + colorReset
		}

		return "○ optional"
	case StatusUnknown:
		if c.useColor {
			return colorCyan + "? unknown" + colorReset
		}

		return "? unknown"
	default:
		return string(status)
	}
}

// majorMinorMatch checks if two versions have the same major.minor.
// Handles versions with or without "v" prefix (e.g., "v2.17.0" and "2.17.0").
func majorMinorMatch(v1, v2 string) bool {
	if v1 == "" || v2 == "" {
		return false
	}

	// Strip leading "v" prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	ver1, err := semver.Parse(v1)
	if err != nil {
		return false
	}

	ver2, err := semver.Parse(v2)
	if err != nil {
		return false
	}

	return ver1.Major == ver2.Major && ver1.Minor == ver2.Minor
}
