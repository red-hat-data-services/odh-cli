package api

// AnnotationValidValues is the pflag annotation key used to declare
// valid values for a flag. The api command reads this annotation to
// populate the validValues field in the manifest.
const AnnotationValidValues = "odh_valid_values"

// Manifest is the top-level output of the api command.
type Manifest struct {
	Version     string              `json:"version"`
	GlobalFlags []FlagDescriptor    `json:"globalFlags"`
	Commands    []CommandDescriptor `json:"commands"`
}

// CommandDescriptor describes a single CLI command or subcommand.
type CommandDescriptor struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Args        *ArgsDescriptor     `json:"args,omitempty"`
	Flags       []FlagDescriptor    `json:"flags"`
	HasSchema   bool                `json:"hasSchema"`
	Examples    []string            `json:"examples"`
	Subcommands []CommandDescriptor `json:"subcommands"`
}

// ArgsDescriptor describes a command's positional arguments.
type ArgsDescriptor struct {
	Pattern     string   `json:"pattern"`
	ValidValues []string `json:"validValues,omitempty"`
}

// FlagDescriptor describes a single command flag.
type FlagDescriptor struct {
	Name        string   `json:"name"`
	Shorthand   string   `json:"shorthand,omitempty"`
	Type        string   `json:"type"`
	Default     string   `json:"default"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	ValidValues []string `json:"validValues,omitempty"`
}
