# Coding Conventions

This document covers core coding conventions for odh-cli development.

For architectural patterns, see [patterns.md](patterns.md). For code formatting rules, see [formatting.md](formatting.md).

## Error Handling Conventions

* Errors are wrapped using `fmt.Errorf` with `%w` for proper error chain propagation
* Context is passed through operations for cancellation support
* First error encountered stops processing and is returned immediately
* All constructors validate inputs and return errors when appropriate
* Use `errors.As()` to extract typed errors from error chains
* Use `errors.Is()` to check for specific underlying errors

## Function Signatures

* Each parameter must have its own type declaration
* Never group parameters with the same type
* Use multiline formatting for functions with many parameters:

```go
func ProcessRequest(
    ctx context.Context,
    userID string,
    requestType int,
    payload []byte,
    timeout time.Duration,
) (*Response, error) {
    // implementation
}
```

## Package Organization

```
odh-cli/
├── cmd/
│   ├── main.go          # Entry point
│   └── version/         # Version command
├── pkg/
│   ├── <command>/       # Command-specific logic
│   │   ├── types.go     # Command-specific types
│   │   └── ...          # Additional command logic
│   ├── printer/         # Output formatting
│   │   ├── types.go     # Printer types
│   │   └── table/       # Table rendering
│   └── util/            # Shared utilities
├── internal/
│   └── version/         # Version information
├── go.mod
├── go.sum
└── Makefile
```

## Naming Conventions

* Use camelCase for unexported functions and variables
* Use PascalCase for exported functions and types
* Prefer descriptive names over short abbreviations
* For status constants, use clear, unambiguous names (e.g., `StatusOK`, `StatusError`, `StatusWarning`)
* Avoid package name repetition in type names (e.g., use `client.Client`, NOT `client.ClientClient`)

## Code Comments

Comments MUST explain **WHY**, not **WHAT**. Code should be self-documenting through clear naming and structure.

**When comments are REQUIRED:**
- Exported functions, types, and constants (godoc comments)
- Complex algorithms or non-obvious logic
- Business rule explanations
- Workarounds for known issues or limitations
- Security-sensitive code

**When comments are PROHIBITED:**
- Restating what the code obviously does
- Describing language features
- Redundant information already clear from code

**Examples:**

```go
// ❌ BAD: States the obvious
// Set the name field to the provided name
user.Name = name

// ❌ BAD: Describes what, not why
// Loop through all items
for _, item := range items {
    process(item)
}

// ✓ GOOD: Explains why
// Process items sequentially to maintain order dependency between transformations
for _, item := range items {
    process(item)
}

// ✓ GOOD: Explains business rule
// Version detection prioritizes DataScienceCluster over DSCInitialization
// because DSC represents the user's desired state while DSCI is system-level.
version, err := detectVersion(ctx, client)

// ✓ GOOD: Explains workaround
// Use string comparison instead of semantic versioning to avoid
// dependency on semver library. See issue #123.
if versionString > "3.0" {
    // ...
}
```

## Message Constants

User-facing messages MUST be defined as package-level constants, NOT inline strings.

**Rationale:** Constants enable message reuse, consistent wording, easier localization, and prevent typos in error messages.

**Required:**
```go
const (
    msgComponentNotFound     = "component %q not found in cluster"
    msgInvalidVersion        = "invalid version format: %s"
    msgClusterNotReachable   = "unable to connect to cluster: %w"
)

func ValidateComponent(name string) error {
    if !exists(name) {
        return fmt.Errorf(msgComponentNotFound, name)
    }
    return nil
}
```

**Prohibited:**
```go
// ❌ WRONG: Inline string literal
func ValidateComponent(name string) error {
    if !exists(name) {
        return fmt.Errorf("component %q not found in cluster", name)
    }
    return nil
}
```

**Multi-line messages:**
```go
const msgUpgradeBlocked = `Upgrade to version %s is blocked due to:
- Incompatible component configurations
- Deprecated API usage
Run 'kubectl odh lint --target-version %s' for details.`
```

## Command Interface Pattern

All commands must implement the Command interface with four lifecycle methods:

```go
type Command interface {
    AddFlags(fs *pflag.FlagSet)  // Register command-specific flags
    Complete() error              // Initialize runtime state
    Validate() error              // Verify configuration
    Run(ctx context.Context) error // Execute business logic
}
```

**Strict Requirements:**
- Struct MUST be named `Command` (NOT `Options`, `CommandOptions`, or any variant)
- Constructor MUST be named `NewCommand()` (NOT `NewOptions()`, `New()`, or any variant)
- Implementation file MUST be named `<command>.go` (NOT `options.go`, `<command>_options.go`, etc.)
- Commands initialize their own `SharedOptions` internally

**Examples:**

```go
// ✓ CORRECT naming
type Command struct {
    shared        *SharedOptions
    targetVersion string
}

func NewCommand(opts CommandOptions) *Command {
    return &Command{
        shared:        opts.Shared,
        targetVersion: opts.TargetVersion,
    }
}

// ❌ WRONG naming
type LintOptions struct {  // Wrong: should be Command
    shared *SharedOptions
}

func NewLintOptions() *LintOptions {  // Wrong: should be NewCommand
    return &LintOptions{}
}
```

See [../lint/architecture.md](../lint/architecture.md#command-lifecycle) for detailed lifecycle documentation.
