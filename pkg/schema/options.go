package schema

import "github.com/spf13/pflag"

// OutputOptions holds schema-related output flags.
type OutputOptions struct {
	// OutputSchema outputs JSON Schema instead of running the command.
	OutputSchema bool
}

// AddFlags registers schema-related flags.
func (o *OutputOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.OutputSchema, "schema", false,
		"output JSON Schema for the command's structured output format")
}
