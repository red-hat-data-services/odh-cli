package table

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter/tw"

	"github.com/olekukonko/tablewriter"
)

// ColumnFormatter is a function that transforms a value for display in a specific column
type ColumnFormatter func(value interface{}) any

// Renderer provides a flexible interface for creating and rendering tables
type Renderer struct {
	writer       io.Writer
	headers      []string
	formatters   map[string]ColumnFormatter
	table        *tablewriter.Table
	tableOptions []tablewriter.Option
}

// NewRenderer creates a new table renderer with the given tableOptions
func NewRenderer(opts ...Option) *Renderer {
	r := &Renderer{
		writer:     os.Stdout,
		formatters: make(map[string]ColumnFormatter),
	}

	// Apply tableOptions first to set basic configuration
	for _, opt := range opts {
		opt(r)
	}

	r.table = tablewriter.NewTable(r.writer)

	if len(r.tableOptions) == 0 {
		r.table = r.table.Options(tablewriter.WithRendition(
			tw.Rendition{
				Settings: tw.Settings{
					Separators: tw.Separators{
						BetweenColumns: tw.Off,
					},
				},
			}),
		)
	} else {
		r.table = r.table.Options(r.tableOptions...)
	}

	if len(r.headers) > 0 {
		r.table.Header(r.headers)
	}

	return r
}

func (r *Renderer) Append(values []any) error {
	if len(values) != len(r.headers) {
		return fmt.Errorf("TODO")
	}

	row := values[:0]

	for i := range r.headers {
		v := values[i]
		h := strings.ToUpper(r.headers[i])

		// Apply formatter if one exists for this column
		if formatter, exists := r.formatters[h]; exists {
			v = formatter(v)
		}

		row = append(row, v)
	}

	return r.table.Append(row)
}

// AppendAll adds multiple rows to the table in a single operation
func (r *Renderer) AppendAll(rows [][]any) error {
	for _, values := range rows {
		if err := r.Append(values); err != nil {
			return err
		}
	}
	return nil
}

// Render outputs the table to the configured writer
func (r *Renderer) Render() error {
	return r.table.Render()
}

// SetHeaders updates the table headers (useful for dynamic header configuration)
func (r *Renderer) SetHeaders(headers ...string) {
	r.headers = headers
	r.table.Header(headers)
}

// GetHeaders returns the current headers
func (r *Renderer) GetHeaders() []string {
	return r.headers
}
