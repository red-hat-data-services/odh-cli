# odh-cli Development Guidelines

## Project Overview

kubectl plugin for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI). The CLI validates cluster configuration, assesses upgrade readiness, and backs up workloads.

## Architecture

The project follows a standard Go CLI structure with Cobra commands:

```text
cmd/           - Command definitions (Cobra wrappers)
pkg/           - Business logic, lint checks, utilities
internal/      - Internal version information
docs/          - Development documentation
```

Commands use a four-phase lifecycle: AddFlags -> Complete -> Validate -> Run.
All commands use `Command` struct with `NewCommand()` constructor.

## Build and Run

```bash
make build       # Build the binary
make run         # Run the CLI
make fmt         # Format code (NEVER use gci/gofmt directly)
make lint        # Run linter (NEVER use golangci-lint directly)
make lint/fix    # Auto-fix lint issues (try this FIRST)
make test        # Run tests
make check       # Run all quality gates (lint)
make tidy        # Tidy dependencies
make clean       # Clean build artifacts
```

CRITICAL: Always use `make` commands for formatting and linting. Never invoke `gci`, `gofmt`, or `golangci-lint` directly. Direct `go test` is acceptable for targeted test runs.

## Test Guidelines

- Use vanilla Gomega (not Ginkgo) with dot imports
- Use `t.Run()` subtests and `t.Context()` (Go 1.24+)
- All test data must be package-level constants, never inline
- Use `HaveField`/`MatchFields` for struct assertions
- Mocks must use testify/mock in `pkg/util/test/mocks/`
- Use fake clients from `sigs.k8s.io/controller-runtime/pkg/client/fake`

## Debug and Troubleshooting

- Run `make lint/fix` first for any lint issues before manual debug
- Use `make check` after every implementation change
- For test failures: `go test -v ./pkg/<package> -run TestName`
- Check `.golangci.yml` for linter configuration

## CRITICAL: Required Reading for All Agents

**Before starting ANY work on this project, agents MUST read the following documents:**

### Core Guidelines
- @docs/agent.md
- @docs/development.md (navigation hub - read this first)
- @docs/design.md

### Setup and Workflow
- @docs/setup.md
- @docs/quality.md
- @docs/code-review.md

### Coding Standards
- @docs/coding/conventions.md
- @docs/coding/patterns.md
- @docs/coding/formatting.md

### Testing and Extensibility
- @docs/testing.md
- @docs/extensibility.md

### Lint Command (when working on lint features)
- @docs/lint/architecture.md
- @docs/lint/writing-checks.md

These documents contain critical requirements that MUST be followed.
Failure to read and follow these guidelines will result in code that does not meet project standards.
