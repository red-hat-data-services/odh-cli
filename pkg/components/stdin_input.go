package components

// StdinInput defines the JSON/YAML schema for stdin input to components enable/disable commands.
// Use with --from-stdin to pass configuration via stdin.
// Precedence: CLI flags > stdin values > defaults.
type StdinInput struct {
	// Components is a list of component names to enable/disable.
	Components []string `json:"components,omitempty" yaml:"components,omitempty"`

	// DryRun shows what would change without applying (replaces --dry-run flag).
	DryRun bool `json:"dryRun,omitempty" yaml:"dryRun,omitempty"`

	// SkipConfirm skips confirmation prompts (replaces --yes flag).
	// Named "skipConfirm" instead of "yes" because "yes" is a reserved YAML 1.1 boolean.
	SkipConfirm bool `json:"skipConfirm,omitempty" yaml:"skipConfirm,omitempty"`
}
