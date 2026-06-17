package migrate

// StdinInput defines the JSON/YAML schema for stdin input to migrate run command.
// Use with --from-stdin to pass configuration via stdin. Precedence: CLI flags > stdin values > defaults.
type StdinInput struct {
	// Migrations is a list of migration IDs to execute.
	Migrations []string `json:"migrations,omitempty" yaml:"migrations,omitempty"`

	// TargetVersion is the target version for migration.
	TargetVersion string `json:"targetVersion,omitempty" yaml:"targetVersion,omitempty"`

	// Phase is the lifecycle phase to execute (pre-upgrade|post-upgrade|pre-enablement).
	Phase string `json:"phase,omitempty" yaml:"phase,omitempty"`

	// DryRun shows what would change without applying (replaces --dry-run flag).
	DryRun bool `json:"dryRun,omitempty" yaml:"dryRun,omitempty"`

	// SkipConfirm skips confirmation prompts (replaces --yes flag).
	// Named "skipConfirm" instead of "yes" because "yes" is a reserved YAML 1.1 boolean.
	SkipConfirm bool `json:"skipConfirm,omitempty" yaml:"skipConfirm,omitempty"`
}
