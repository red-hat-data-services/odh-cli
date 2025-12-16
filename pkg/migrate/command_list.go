package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/cmd"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action"
	"github.com/lburgazzoli/odh-cli/pkg/printer/table"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*ListCommand)(nil)

type migrationRow struct {
	ID          string
	Name        string
	Description string
	Applicable  string
}

type ListCommand struct {
	*SharedOptions

	TargetVersion string
	ShowAll       bool

	parsedTargetVersion *semver.Version
}

func NewListCommand(streams genericiooptions.IOStreams) *ListCommand {
	shared := NewSharedOptions(streams)

	return &ListCommand{
		SharedOptions: shared,
	}
}

func (c *ListCommand) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), flagDescListOutput)
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescListVerbose)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescListTargetVersion)
	fs.BoolVar(&c.ShowAll, "all", false, flagDescListAll)
}

func (c *ListCommand) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	if !c.Verbose {
		c.IO = iostreams.NewQuietWrapper(c.IO)
	}

	if c.TargetVersion != "" {
		targetVer, err := semver.Parse(c.TargetVersion)
		if err != nil {
			return fmt.Errorf("invalid target version %q: %w", c.TargetVersion, err)
		}
		c.parsedTargetVersion = &targetVer
	}

	return nil
}

func (c *ListCommand) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	if c.ShowAll && c.TargetVersion != "" {
		return errors.New("--all and --target-version are mutually exclusive")
	}

	if !c.ShowAll && c.TargetVersion == "" {
		return errors.New("--target-version flag is required")
	}

	return nil
}

func (c *ListCommand) Run(ctx context.Context) error {
	var currentVersion *version.ClusterVersion
	var err error

	if !c.ShowAll {
		currentVersion, err = version.Detect(ctx, c.Client)
		if err != nil {
			return fmt.Errorf("detecting cluster version: %w", err)
		}
	}

	registry := action.GetGlobalRegistry()
	allActions := registry.ListAll()

	if len(allActions) == 0 {
		c.IO.Errorf("No migrations registered")

		return nil
	}

	rows := make([]migrationRow, 0)

	for _, act := range allActions {
		var applicableStr string

		if c.ShowAll && c.parsedTargetVersion == nil {
			applicableStr = "N/A"
		} else {
			applicable := act.CanApply(
				parseVersion(currentVersion.Version),
				c.parsedTargetVersion,
			)

			if !c.ShowAll && !applicable {
				continue
			}

			if applicable {
				applicableStr = "Yes"
			} else {
				applicableStr = "No"
			}
		}

		rows = append(rows, migrationRow{
			ID:          act.ID(),
			Name:        act.Name(),
			Description: act.Description(),
			Applicable:  applicableStr,
		})
	}

	if len(rows) == 0 {
		c.IO.Errorf("no migrations applicable for version %s", c.TargetVersion)

		return nil
	}

	switch c.OutputFormat {
	case OutputFormatTable:
		return c.printTable(rows)
	case OutputFormatJSON:
		return c.printJSON(rows)
	case OutputFormatYAML:
		return c.printYAML(rows)
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

func (c *ListCommand) printTable(rows []migrationRow) error {
	renderer := table.NewRenderer(
		table.WithWriter[migrationRow](c.IO.Out()),
		table.WithHeaders[migrationRow]("ID", "NAME", "APPLICABLE", "DESCRIPTION"),
		table.WithTableOptions[migrationRow](table.DefaultTableOptions...),
	)

	for _, row := range rows {
		if err := renderer.Append(row); err != nil {
			return fmt.Errorf("failed to append row: %w", err)
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("failed to render table: %w", err)
	}

	return nil
}

func (c *ListCommand) printJSON(rows []migrationRow) error {
	//nolint:musttag // Table rows don't need JSON tags
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	c.IO.Fprintf("%s\n", string(data))

	return nil
}

func (c *ListCommand) printYAML(rows []migrationRow) error {
	data, err := yaml.Marshal(rows)
	if err != nil {
		return fmt.Errorf("marshaling YAML: %w", err)
	}

	c.IO.Fprintf("%s", string(data))

	return nil
}

func parseVersion(versionStr string) *semver.Version {
	ver, err := semver.Parse(versionStr)
	if err != nil {
		return nil
	}

	return &ver
}
