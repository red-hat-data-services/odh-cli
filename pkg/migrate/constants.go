package migrate

// Flag descriptions for the migrate list command.
const (
	flagDescListOutput        = "Output format (table|json|yaml)"
	flagDescListVerbose       = "Show detailed information"
	flagDescListTargetVersion = "Target version for migration filtering (required unless --all is specified)"
	flagDescListAll           = "Show all migrations, not just applicable ones"
	flagDescListPhase         = "Filter migrations by lifecycle phase (pre-upgrade|post-upgrade|pre-enablement)"
)

// Flag descriptions for the migrate run command.
const (
	flagDescRunVerbose       = "Show detailed progress"
	flagDescRunTimeout       = "Operation timeout (e.g., 10m, 30m)"
	flagDescRunDryRun        = "Show what would be done without making changes"
	flagDescRunYes           = "Skip confirmation prompts"
	flagDescRunMigration     = "Migration ID to execute (can be specified multiple times)"
	flagDescRunTargetVersion = "Target version for migration (required)"
	flagDescRunPhase         = "Lifecycle phase to execute (pre-upgrade|post-upgrade|pre-enablement). Auto-detected from version comparison if not specified"
)

// Flag descriptions for the migrate prepare command.
const (
	flagDescPrepareVerbose       = "Show detailed progress"
	flagDescPrepareTimeout       = "Operation timeout (e.g., 10m, 30m)"
	flagDescPrepareDryRun        = "Show what would be backed up without making changes"
	flagDescPrepareYes           = "Skip confirmation prompts"
	flagDescPrepareOutputDir     = "Output directory for backups (default: ./backup-<timestamp>/)"
	flagDescPrepareMigration     = "Migration ID to prepare (can be specified multiple times)"
	flagDescPrepareTargetVersion = "Target version for migration (required)"
	flagDescPreparePhase         = "Filter preparations by lifecycle phase (pre-upgrade|post-upgrade|pre-enablement)"
)
