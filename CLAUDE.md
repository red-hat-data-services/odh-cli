# odh-cli Development Guidelines

## CRITICAL: Required Reading for All Agents

**Before starting ANY work on this project, agents MUST read the following documents in their entirety:**

- **[docs/development.md](docs/development.md)** - REQUIRED: Development practices, coding conventions, testing guidelines
- **[docs/design.md](docs/design.md)** - REQUIRED: Overall CLI design and architecture
- **[docs/lint/architecture.md](docs/lint/architecture.md)** - Lint command architecture (if working on lint)
- **[docs/lint/writing-checks.md](docs/lint/writing-checks.md)** - Writing lint checks (if adding checks)

These documents contain critical requirements that MUST be followed. Failure to read and follow these guidelines will result in code that does not meet project standards.

## Self-Review (Mandatory)

Before ANY response containing code, analysis, or recommendations:

1. **Pause and re-read your work**
2. **Ask yourself:**
   - "What would a senior engineer critique?"
   - "What edge case am I missing?"
   - "Is this actually correct?"
   - "Does it follow development and architecture rules?"
3. **Fix issues before responding**
4. **Note significant fixes**: "Self-review: [what you caught]"
5. **If there is significant work, recommend the steps to fix it**

This self-review step is NOT optional. Taking an extra 30 seconds to review prevents wasted time from incorrect implementations.
