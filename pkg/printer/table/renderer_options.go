package table

import (
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
)

type Option func(*Renderer)

func WithWriter(w io.Writer) Option {
	return func(r *Renderer) {
		r.writer = w
	}
}

func WithHeaders(headers ...string) Option {
	return func(r *Renderer) {
		r.headers = headers
	}
}

func WithFormatter(columnName string, formatter ColumnFormatter) Option {
	return func(r *Renderer) {
		if r.formatters == nil {
			r.formatters = make(map[string]ColumnFormatter)
		}

		r.formatters[strings.ToUpper(columnName)] = formatter
	}
}

func WithTableOptions(values ...tablewriter.Option) Option {
	return func(r *Renderer) {
		r.tableOptions = append(r.tableOptions, values...)
	}
}
