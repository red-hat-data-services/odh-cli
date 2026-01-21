# Code Formatting

This document covers code formatting rules and the auto-registration pattern using blank imports.

For other coding conventions, see [conventions.md](conventions.md) and [patterns.md](patterns.md).

## Code Formatting

**CRITICAL: MUST use `make fmt` to format code. NEVER use `gci` or other formatters directly.**

```bash
# ✓ CORRECT - Format all code
make fmt

# ❌ WRONG - DO NOT use gci directly
gci write ./...
gci write -s standard -s default ./...

# ❌ WRONG - DO NOT use gofmt directly
gofmt -w .

# ❌ WRONG - DO NOT use goimports directly
goimports -w .
```

**Why you MUST use `make fmt`:**
- **Safety**: The Makefile applies correct flags to prevent breaking critical files
- **Consistency**: All developers use identical formatting configuration
- **Completeness**: `make fmt` runs all necessary formatters in the correct order
- **Protection**: Direct tool usage can accidentally modify blank imports in `cmd/lint/lint.go` and `cmd/migrate/migrate.go`, breaking auto-registration

**What `make fmt` does:**
1. Runs `go fmt` for basic formatting
2. Runs `gci` with project-specific import grouping rules
3. Applies special handling for files with blank imports

**Never run formatting tools directly.** Always use `make fmt`.

## Blank Imports for Auto-Registration

**CRITICAL:** Blank imports (imports prefixed with `_`) MUST NOT be removed from command entry points, as they are essential for the auto-registration pattern used by checks, migrations, and other pluggable components.

### How Auto-Registration Works

The project uses Go's `init()` function mechanism for automatic component registration:

1. Each check/migration package defines an `init()` function that registers itself with a global registry
2. Blank imports in command entry points trigger these `init()` functions at program startup
3. Without the blank imports, the `init()` functions never execute and components remain unregistered

### Files with Required Blank Imports

- `cmd/lint/lint.go` - Registers all lint checks
- `cmd/migrate/migrate.go` - Registers all migration actions
- Any future command that uses auto-registration

### Example from cmd/lint/lint.go

```go
//nolint:gci // Blank imports required for check registration - DO NOT REMOVE
import (
    "fmt"

    "github.com/spf13/cobra"

    "k8s.io/cli-runtime/pkg/genericclioptions"
    "k8s.io/cli-runtime/pkg/genericiooptions"

    lintpkg "github.com/lburgazzoli/odh-cli/pkg/lint"
    // Import check packages to trigger init() auto-registration.
    // These blank imports are REQUIRED for checks to register with the global registry.
    // DO NOT REMOVE - they appear unused but are essential for runtime check discovery.
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kserve"
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/modelmesh"
    // ... additional check packages
)
```

### Why Blank Imports Appear Unused

- IDEs and linters may flag these as unused because the packages aren't referenced directly in code
- The `//nolint:gci` directive suppresses import grouping/ordering linter warnings
- Extensive comments explain why removal would break functionality
- The `init()` side-effect is invisible to static analysis

### Verification

If blank imports are accidentally removed:
- Compilation succeeds (no syntax errors)
- Build succeeds
- Runtime behavior is broken: checks/migrations won't be registered and won't execute
- Users will see empty check lists or "no checks found" errors

### Guidelines

- ✅ **ALWAYS** preserve blank imports in command entry points
- ✅ **ALWAYS** include clear comments explaining why they cannot be removed
- ✅ **ALWAYS** use `//nolint:gci` directive to prevent import ordering changes
- ❌ **NEVER** remove blank imports even if they appear unused
- ❌ **NEVER** run automated import cleanup tools (like `goimports -w`) on these files without reviewing changes
- ❌ **NEVER** accept IDE suggestions to remove "unused" imports in these files

### Related Architecture

See [../lint/architecture.md](../lint/architecture.md#auto-registration) for details on the check registration system and [../design.md](../design.md#extensibility) for the architectural rationale behind auto-registration.
