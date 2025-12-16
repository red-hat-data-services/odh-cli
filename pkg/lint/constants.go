package lint

// Flag descriptions for the lint command.
const (
	flagDescTargetVersion = "target version for upgrade readiness checks (e.g., 2.25.0, 3.0.0)"
	flagDescOutput        = "output format (table|json|yaml)"
	flagDescSeverity      = "minimum severity to display (critical|warning|info)"
	flagDescFailCritical  = "exit with error if critical findings are detected"
	flagDescFailWarning   = "exit with error if warning or critical findings are detected"
	flagDescVerbose       = "show detailed output"
	flagDescTimeout       = "operation timeout (e.g., 10m, 30m)"
)

const flagDescChecks = `check selector patterns (glob patterns or categories):
  - '*'             : all checks
  - 'components.*'  : all component checks
  - 'services.*'    : all service checks
  - 'workloads.*'   : all workload checks
  - 'dependencies.*': all dependency checks
  - '*dashboard*'   : all checks with 'dashboard' in ID
  - 'exact.id'      : exact check ID
Can be specified multiple times`
