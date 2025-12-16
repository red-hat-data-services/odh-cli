package migrate

// Flag descriptions for the migrate list command.
const (
	flagDescListOutput        = "Output format (table|json|yaml)"
	flagDescListVerbose       = "Show detailed information"
	flagDescListTargetVersion = "Target version for migration filtering (required unless --all is specified)"
	flagDescListAll           = "Show all migrations, not just applicable ones"
)

// Flag descriptions for the migrate run command.
const (
	flagDescRunVerbose       = "Show detailed progress"
	flagDescRunTimeout       = "Operation timeout (e.g., 10m, 30m)"
	flagDescRunDryRun        = "Show what would be done without making changes"
	flagDescRunPrepare       = "Run pre-flight checks and backup resources (does not execute migration)"
	flagDescRunYes           = "Skip confirmation prompts"
	flagDescRunBackupPath    = "Path to store backup files (used with --prepare)"
	flagDescRunMigration     = "Migration ID to execute (can be specified multiple times)"
	flagDescRunTargetVersion = "Target version for migration (required)"
)
