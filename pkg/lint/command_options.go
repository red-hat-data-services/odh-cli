package lint

import (
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/fatih/color"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	printerjson "github.com/lburgazzoli/odh-cli/pkg/printer/json"
	"github.com/lburgazzoli/odh-cli/pkg/printer/table"
	printeryaml "github.com/lburgazzoli/odh-cli/pkg/printer/yaml"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

// OutputFormat represents the output format for doctor commands.
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"

	// DefaultTimeout is the default timeout for doctor commands.
	DefaultTimeout = 5 * time.Minute
)

//nolint:gochecknoglobals
var (
	// Table output symbols
	statusPass = color.New(color.FgGreen).Sprint("✓")
	statusWarn = color.New(color.FgYellow).Sprint("⚠")
	statusFail = color.New(color.FgRed).Sprint("✗")

	// Severity level formatting
	severityCrit = color.New(color.FgRed).Sprint("critical")
	severityWarn = color.New(color.FgYellow).Add(color.Bold).Sprint("warning") // Bold yellow (orange-ish)
	severityInfo = color.New(color.FgCyan).Sprint("info")

	// Table headers
	tableHeaders = []string{"STATUS", "GROUP", "KIND", "CHECK", "SEVERITY", "MESSAGE"}
)

// Validate checks if the output format is valid.
func (o OutputFormat) Validate() error {
	switch o {
	case OutputFormatTable, OutputFormatJSON, OutputFormatYAML:
		return nil
	default:
		return fmt.Errorf("invalid output format: %s (must be one of: table, json, yaml)", o)
	}
}

// MinimumSeverity represents the minimum severity level to display in results.
type MinimumSeverity string

const (
	MinimumSeverityCritical MinimumSeverity = "critical"
	MinimumSeverityWarning  MinimumSeverity = "warning"
	MinimumSeverityInfo     MinimumSeverity = "info"
	MinimumSeverityAll      MinimumSeverity = "" // Empty string means show all
)

// Validate checks if the minimum severity is valid.
func (m MinimumSeverity) Validate() error {
	switch m {
	case MinimumSeverityCritical, MinimumSeverityWarning, MinimumSeverityInfo, MinimumSeverityAll:
		return nil
	default:
		return fmt.Errorf("invalid minimum severity: %s (must be one of: critical, warning, info)", m)
	}
}

// ShouldInclude returns true if a check result with the given severity should be included.
func (m MinimumSeverity) ShouldInclude(severity *string) bool {
	// Always include pass/error results
	if severity == nil {
		return true
	}

	// If showing all or info (which includes all), return true
	if m == MinimumSeverityAll || m == MinimumSeverityInfo {
		return true
	}

	// For critical filter, only show critical
	if m == MinimumSeverityCritical {
		return *severity == string(check.SeverityCritical)
	}

	// For warning filter, show critical and warning
	if m == MinimumSeverityWarning {
		return *severity == string(check.SeverityCritical) || *severity == string(check.SeverityWarning)
	}

	// Default: show all
	return true
}

// SharedOptions contains options common to all doctor subcommands.
type SharedOptions struct {
	// IO provides structured access to stdin, stdout, stderr with convenience methods
	IO iostreams.Interface

	// ConfigFlags provides access to kubeconfig and context
	ConfigFlags *genericclioptions.ConfigFlags

	// OutputFormat specifies the output format (table, json, yaml)
	OutputFormat OutputFormat

	// CheckSelector filters which checks to run (glob pattern)
	CheckSelector string

	// MinSeverity filters results by minimum severity level
	MinSeverity MinimumSeverity

	// FailOnCritical exits with non-zero code if critical findings detected
	FailOnCritical bool

	// FailOnWarning exits with non-zero code if warning findings detected
	FailOnWarning bool

	// Verbose enables progress messages (default: false, quiet by default)
	Verbose bool

	// Timeout is the maximum duration for command execution
	Timeout time.Duration

	// Client is the Kubernetes client (populated during Complete)
	Client *client.Client
}

// NewSharedOptions creates a new SharedOptions with defaults.
func NewSharedOptions(streams genericiooptions.IOStreams) *SharedOptions {
	return &SharedOptions{
		ConfigFlags:    genericclioptions.NewConfigFlags(true),
		OutputFormat:   OutputFormatTable,
		CheckSelector:  "*",                // Run all checks by default
		MinSeverity:    MinimumSeverityAll, // Show all severity levels by default
		FailOnCritical: true,               // Exit with error on critical findings (default)
		FailOnWarning:  false,              // Don't exit on warnings by default
		Timeout:        DefaultTimeout,     // Default timeout to prevent hanging on slow clusters
		IO:             iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
	}
}

// Complete populates the client and performs pre-validation setup.
func (o *SharedOptions) Complete() error {
	// Create the unified client
	c, err := client.NewClient(o.ConfigFlags)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	o.Client = c

	return nil
}

// Validate checks that all required options are valid.
func (o *SharedOptions) Validate() error {
	// Validate output format
	if err := o.OutputFormat.Validate(); err != nil {
		return err
	}

	// Validate check selector
	if err := ValidateCheckSelector(o.CheckSelector); err != nil {
		return err
	}

	// Validate minimum severity
	if err := o.MinSeverity.Validate(); err != nil {
		return err
	}

	// Validate timeout
	if o.Timeout <= 0 {
		return errors.New("timeout must be greater than 0")
	}

	return nil
}

// ValidateCheckSelector validates the check selector pattern.
func ValidateCheckSelector(selector string) error {
	if selector == "" {
		return errors.New("check selector cannot be empty")
	}

	// Allow category shortcuts
	if selector == "components" || selector == "services" || selector == "workloads" || selector == "dependencies" {
		return nil
	}

	// Allow wildcard (default)
	if selector == "*" {
		return nil
	}

	// Validate glob pattern
	_, err := path.Match(selector, "test.check")
	if err != nil {
		return fmt.Errorf("invalid check selector pattern %q: %w", selector, err)
	}

	return nil
}

// CommandOptions contains configuration for creating a Command using struct-based initialization.
// This is the preferred pattern for simple command construction.
//
// Example:
//
//	cmd := lint.NewCommandWithOptions(lint.CommandOptions{
//	    Streams:       streams,
//	    TargetVersion: "3.0",
//	})
type CommandOptions struct {
	// Streams provides access to stdin, stdout, stderr
	Streams genericiooptions.IOStreams

	// TargetVersion is the optional target version for upgrade assessment
	TargetVersion string

	// Shared allows passing a pre-configured SharedOptions (advanced use case)
	Shared *SharedOptions
}

// CommandOption is a functional option for configuring a Command.
// This pattern is useful for complex initialization scenarios.
//
// Example:
//
//	cmd := lint.NewCommandWithFunctionalOptions(
//	    lint.WithStreams(streams),
//	    lint.WithTargetVersion("3.0"),
//	)
type CommandOption func(*Command)

// WithStreams returns a CommandOption that sets the IO streams.
func WithStreams(streams genericiooptions.IOStreams) CommandOption {
	return func(c *Command) {
		if c.SharedOptions == nil {
			c.SharedOptions = NewSharedOptions(streams)
		} else {
			// Update existing SharedOptions streams
			c.IO = iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut)
		}
	}
}

// WithTargetVersion returns a CommandOption that sets the target version.
func WithTargetVersion(version string) CommandOption {
	return func(c *Command) {
		c.TargetVersion = version
	}
}

// WithShared returns a CommandOption that sets the SharedOptions.
// This is an advanced option for cases where SharedOptions needs custom configuration.
func WithShared(shared *SharedOptions) CommandOption {
	return func(c *Command) {
		c.SharedOptions = shared
	}
}

// NewCommandWithOptions creates a new Command using struct-based initialization.
// This is the preferred pattern for simple command construction.
func NewCommandWithOptions(opts CommandOptions) *Command {
	var shared *SharedOptions
	if opts.Shared != nil {
		shared = opts.Shared
	} else {
		shared = NewSharedOptions(opts.Streams)
	}

	return &Command{
		SharedOptions: shared,
		TargetVersion: opts.TargetVersion,
	}
}

// NewCommandWithFunctionalOptions creates a new Command using functional options.
// This pattern is useful for complex initialization scenarios.
func NewCommandWithFunctionalOptions(options ...CommandOption) *Command {
	// Initialize with default empty streams
	cmd := &Command{
		SharedOptions: NewSharedOptions(genericiooptions.IOStreams{}),
	}

	// Apply functional options
	for _, opt := range options {
		opt(cmd)
	}

	return cmd
}

// CheckResultOutput represents a check result for JSON/YAML output.
type CheckResultOutput struct {
	CheckID     string         `json:"checkId"               yaml:"checkId"`
	CheckName   string         `json:"checkName"             yaml:"checkName"`
	Group       string         `json:"group"                 yaml:"group"`
	Status      string         `json:"status"                yaml:"status"`
	Severity    *string        `json:"severity,omitempty"    yaml:"severity,omitempty"`
	Message     string         `json:"message"               yaml:"message"`
	Remediation string         `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	Details     map[string]any `json:"details,omitempty"     yaml:"details,omitempty"`
}

// CheckResultTableRow represents a single condition row for table output.
// Each row represents one condition from a diagnostic result.
type CheckResultTableRow struct {
	Status      string
	Group       string
	Kind        string
	Check       string
	Severity    string
	Message     string
	Description string
}

// LintOutput represents the full lint output for JSON/YAML.
type LintOutput struct {
	ClusterVersion *string             `json:"clusterVersion,omitempty" yaml:"clusterVersion,omitempty"`
	TargetVersion  *string             `json:"targetVersion,omitempty"  yaml:"targetVersion,omitempty"`
	Components     []CheckResultOutput `json:"components"               yaml:"components"`
	Services       []CheckResultOutput `json:"services"                 yaml:"services"`
	Dependencies   []CheckResultOutput `json:"dependencies"             yaml:"dependencies"`
	Workloads      []CheckResultOutput `json:"workloads"                yaml:"workloads"`
	Summary        struct {
		Total  int `json:"total"  yaml:"total"`
		Passed int `json:"passed" yaml:"passed"`
		Failed int `json:"failed" yaml:"failed"`
	} `json:"summary" yaml:"summary"`
}

// FilterResultsBySeverity filters check results based on minimum severity level.
func FilterResultsBySeverity(
	resultsByGroup map[check.CheckGroup][]check.CheckExecution,
	minSeverity MinimumSeverity,
) map[check.CheckGroup][]check.CheckExecution {
	// If no filtering requested, return original results
	if minSeverity == MinimumSeverityAll {
		return resultsByGroup
	}

	filtered := make(map[check.CheckGroup][]check.CheckExecution)
	for group, results := range resultsByGroup {
		var groupResults []check.CheckExecution
		for _, res := range results {
			// Always include pass/error results (no severity)
			// Include results that match the minimum severity filter
			if minSeverity.ShouldInclude(res.Result.GetSeverity()) {
				groupResults = append(groupResults, res)
			}
		}
		filtered[group] = groupResults
	}

	return filtered
}

// FlattenResults converts a map of results by group to a flat sorted array.
// Results are sorted by group in the order: Component, Service, Dependency, Workload.
func FlattenResults(resultsByGroup map[check.CheckGroup][]check.CheckExecution) []check.CheckExecution {
	groups := []check.CheckGroup{
		check.GroupComponent,
		check.GroupService,
		check.GroupDependency,
		check.GroupWorkload,
	}

	flattened := make([]check.CheckExecution, 0)
	for _, group := range groups {
		flattened = append(flattened, resultsByGroup[group]...)
	}

	return flattened
}

// getSeverityString determines the severity string from a condition.
// For Status=False, uses explicit severity if set (allows overriding critical to warning).
// For other statuses, always derives from status.
func getSeverityString(
	condition *result.Condition,
	critStr string,
	warnStr string,
	infoStr string,
) string {
	switch condition.Status {
	case metav1.ConditionFalse:
		// For failures, check if explicit severity is set to override default critical
		if condition.Severity != "" {
			switch condition.Severity {
			case result.SeverityCritical:
				return critStr
			case result.SeverityWarning:
				return warnStr
			case result.SeverityInfo:
				return infoStr
			}
		}

		return critStr
	case metav1.ConditionTrue:
		return infoStr
	case metav1.ConditionUnknown:
		return warnStr
	}

	return infoStr
}

// OutputTable is a shared function for outputting check results in table format.
func OutputTable(out io.Writer, results []check.CheckExecution) error {
	totalChecks := 0
	totalPassed := 0
	totalWarnings := 0
	totalFailed := 0

	// Create single table renderer for all results
	renderer := table.NewRenderer[CheckResultTableRow](
		table.WithWriter[CheckResultTableRow](out),
		table.WithHeaders[CheckResultTableRow](tableHeaders...),
		table.WithTableOptions[CheckResultTableRow](table.DefaultTableOptions...),
	)

	// Append all results to single table - one row per condition
	for _, exec := range results {
		// Each diagnostic result can have multiple conditions
		// Create one table row per condition
		for _, condition := range exec.Result.Status.Conditions {
			totalChecks++

			// Determine severity first (uses explicit severity if set for Status=False)
			severity := getSeverityString(&condition, severityCrit, severityWarn, severityInfo)

			// Determine status symbol and count based on severity
			var status string

			switch severity {
			case severityCrit:
				// Critical severity = failed check
				status = statusFail
				totalFailed++
			case severityWarn:
				// Warning severity = warning (not counted as failure)
				status = statusWarn
				totalWarnings++
			default:
				// Info/success
				status = statusPass
				totalPassed++
			}

			msg := condition.Message

			row := CheckResultTableRow{
				Status:      status,
				Group:       exec.Result.Group,
				Kind:        exec.Result.Kind,
				Check:       exec.Result.Name,
				Severity:    severity,
				Message:     msg,
				Description: exec.Result.Spec.Description,
			}

			if err := renderer.Append(row); err != nil {
				return fmt.Errorf("appending table row: %w", err)
			}
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("rendering table: %w", err)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Summary:")
	_, _ = fmt.Fprintf(out, "  Total: %d | Passed: %d | Warnings: %d | Failed: %d\n", totalChecks, totalPassed, totalWarnings, totalFailed)

	return nil
}

// OutputJSON outputs diagnostic results in List format.
func OutputJSON(out io.Writer, results []check.CheckExecution, clusterVersion *string, targetVersion *string) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion)

	// Add all results in execution order
	for _, exec := range results {
		list.Results = append(list.Results, exec.Result)
	}

	renderer := printerjson.NewRenderer[*result.DiagnosticResultList](
		printerjson.WithWriter[*result.DiagnosticResultList](out),
	)

	if err := renderer.Render(list); err != nil {
		return fmt.Errorf("rendering JSON output: %w", err)
	}

	return nil
}

// OutputYAML outputs diagnostic results in List format.
func OutputYAML(out io.Writer, results []check.CheckExecution, clusterVersion *string, targetVersion *string) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion)

	// Add all results in execution order
	for _, exec := range results {
		list.Results = append(list.Results, exec.Result)
	}

	renderer := printeryaml.NewRenderer[*result.DiagnosticResultList](
		printeryaml.WithWriter[*result.DiagnosticResultList](out),
	)

	if err := renderer.Render(list); err != nil {
		return fmt.Errorf("rendering YAML output: %w", err)
	}

	return nil
}
