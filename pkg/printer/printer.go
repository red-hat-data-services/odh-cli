package printer

import (
	"encoding/json"
	"io"

	"github.com/lburgazzoli/odh-cli/pkg/doctor"
	"github.com/lburgazzoli/odh-cli/pkg/printer/table"
)

type Printer interface {
	PrintResults(results *doctor.CheckResults) error
}

func NewPrinter(opts Options) Printer {
	switch opts.OutputFormat {
	case "json":
		return &JSONPrinter{out: opts.IOStreams.Out}
	case "table":
		return &TablePrinter{out: opts.IOStreams.Out}
	default:
		return &TablePrinter{out: opts.IOStreams.Out}
	}
}

type TablePrinter struct {
	out io.Writer
}

func (p *TablePrinter) PrintResults(results *doctor.CheckResults) error {
	renderer := table.NewRenderer(
		table.WithWriter(p.out),
		table.WithHeaders("CHECK", "STATUS", "MESSAGE"),
	)

	for _, result := range results.Checks {
		if err := renderer.Append([]any{result.Name, result.Status, result.Message}); err != nil {
			return err
		}
	}

	return renderer.Render()
}

type JSONPrinter struct {
	out io.Writer
}

func (p *JSONPrinter) PrintResults(results *doctor.CheckResults) error {
	encoder := json.NewEncoder(p.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}
