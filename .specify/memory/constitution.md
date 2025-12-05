<!--
============================================================================
SYNC IMPACT REPORT
============================================================================
Version Change: [NONE] → 1.0.0
Modified Principles: Initial constitution creation
Added Sections: All core principles and governance sections
Removed Sections: None
Templates Requiring Updates:
  ✅ .specify/templates/plan-template.md - Constitution Check section aligned
  ✅ .specify/templates/spec-template.md - Requirements structure aligned
  ✅ .specify/templates/tasks-template.md - Test-optional, task organization aligned
  ⚠ .specify/templates/agent-file-template.md - Generic placeholder, no updates needed
  ⚠ .specify/templates/checklist-template.md - Generic template, no updates needed
Follow-up TODOs: None
============================================================================
-->

# odh-cli Constitution

## Core Principles

### I. kubectl Plugin Integration

The CLI MUST function as a native kubectl plugin following kubectl UX patterns. The binary MUST be named `kubectl-odh` and automatically discovered when placed in PATH. The CLI MUST leverage the user's active kubeconfig for cluster authentication without requiring separate configuration.

**Rationale**: Users interacting with Kubernetes expect kubectl-like tools. Following kubectl conventions reduces cognitive load and provides a familiar, consistent experience.

### II. Extensible Command Structure

All commands MUST follow the modular Cobra-based pattern separating command definition (cmd/) from business logic (pkg/cmd/). New commands MUST be independently testable without Cobra dependencies. Each command MUST implement the Complete/Validate/Run pattern for consistent lifecycle management.

**Rationale**: Separation of concerns enables independent testing, code reuse, and maintains a consistent structure as the CLI grows. This pattern is standard in kubectl plugins and kubectl itself.

### III. Consistent Output Formats

All commands MUST support table (default), JSON, and YAML output formats via the `-o/--output` flag. Table output MUST be human-readable with consistent formatting. JSON and YAML output MUST be machine-parsable and suitable for scripting.

**Rationale**: Different consumers need different formats. Humans need readable tables, scripts need structured JSON/YAML. Consistency across commands reduces learning curve and enables composition.

### IV. Functional Options Pattern

All struct initialization MUST use the functional options pattern with the generic `Option[T]` interface. Configuration MUST be applied via `ApplyTo(target *T)` method. Options MUST be defined in `*_options.go` or `*_option.go` files.

**Rationale**: Provides type-safe, extensible, and composable configuration. This pattern is used in k8s-controller-lib and enables backward-compatible API evolution.

### V. Strict Error Handling

Errors MUST be wrapped using `fmt.Errorf` with `%w` for proper error chain propagation. Context MUST be passed through all operations for cancellation support. First error encountered MUST stop processing and be returned immediately. All constructors MUST validate inputs and return errors when appropriate.

**Rationale**: Proper error handling enables debugging, supports graceful degradation, and provides meaningful error messages to users. Context propagation is essential for timeout and cancellation support.

### VI. Test-First Development

Tests MUST use vanilla Gomega (no Ginkgo). All test data MUST be defined as package-level constants, never inline. Tests MUST use subtests via `t.Run()`. Tests MUST use `t.Context()` for context creation. Both unit tests (isolated components) and integration tests (full command flow) are REQUIRED.

**Rationale**: Test-first ensures correctness, enables refactoring, and serves as living documentation. Package-level constants improve readability and enable test data reuse.

## Development Standards

### Code Organization

Projects MUST follow the standard Go CLI structure:
- `cmd/` - Command definitions and entry points
- `pkg/` - Public packages (command logic, shared utilities)
- `internal/` - Internal packages not for external use

Commands MUST be organized as:
- `cmd/<command>/<command>.go` - Minimal Cobra wrapper
- `pkg/cmd/<command>/<command>.go` - Options struct with Complete/Validate/Run
- `pkg/<command>/` - Domain-specific logic (optional)

### Function Signatures

Each parameter MUST have its own type declaration. Parameters MUST NOT be grouped even if they share the same type. Functions with many parameters MUST use multiline formatting.

**Rationale**: Explicit type declarations improve code clarity and prevent subtle bugs from parameter reordering.

### Naming Conventions

Use camelCase for unexported functions and variables. Use PascalCase for exported functions and types. Prefer descriptive names over abbreviations.

## Quality Gates

### Linting

All code MUST pass `make lint` using golangci-lint v2 with the project's `.golangci.yml` configuration. All linters are enabled by default except: wsl, varnamelen, exhaustruct, ireturn, depguard, err113, paralleltest, funcorder, noinlineerr.

### Testing

All code MUST pass `make test`. New features MUST include both unit and integration tests. Test coverage SHOULD increase or remain stable with new code.

### Formatting

All code MUST be formatted with `make fmt`. Imports MUST be organized using `gci` in sections: standard, default, k8s.io, project, dot.

### Dependencies

Dependencies MUST be kept tidy via `make tidy`. New dependencies MUST pass `make vulncheck` security scanning.

## Governance

This constitution supersedes all other development practices. All pull requests MUST be reviewed for constitutional compliance. Amendments require documentation of rationale, approval from maintainers, and a migration plan if breaking changes are introduced.

Constitutional violations MUST be justified in the implementation plan's Complexity Tracking table, documenting why the simpler alternative was insufficient.

**Constitution Check Gates**:
- Phase 0 (Research): Verify approach aligns with kubectl plugin integration and output format consistency
- Phase 1 (Design): Verify command structure follows Complete/Validate/Run pattern and functional options
- Phase 2 (Implementation): Verify error handling, test coverage, and linting compliance

**Version**: 1.0.0 | **Ratified**: 2025-12-05 | **Last Amended**: 2025-12-05