<!--
  SYNC IMPACT REPORT
  ==================
  Version change: 0.0.0 → 1.0.0 (initial ratification)

  Modified principles: N/A (initial version)

  Added sections:
  - Core Principles (I. Simplicity First, II. Idiomatic Go, III. Test Coverage)
  - Technology Stack
  - Development Workflow
  - Governance

  Removed sections: N/A (initial version)

  Templates requiring updates:
  - .specify/templates/plan-template.md ✅ (compatible - no changes needed)
  - .specify/templates/spec-template.md ✅ (compatible - no changes needed)
  - .specify/templates/tasks-template.md ✅ (compatible - no changes needed)

  Follow-up TODOs: None
-->

# Stratos Constitution

## Core Principles

### I. Simplicity First

All code MUST favor clarity and maintainability over cleverness:

- Start with the simplest solution that works; add complexity only when measurably necessary
- Functions MUST do one thing well and be small enough to understand at a glance
- Avoid premature abstraction: duplicate code 2-3 times before extracting a shared component
- Dependencies MUST be justified; prefer standard library solutions when adequate
- No dead code, unused imports, or commented-out blocks in committed code

**Rationale**: Go's design philosophy prioritizes simplicity. Fighting this leads to brittle,
hard-to-maintain code.

### II. Idiomatic Go

All code MUST follow established Go conventions and patterns:

- Code MUST pass `go fmt`, `go vet`, and configured linters without errors
- Error handling MUST be explicit: check and handle every error, never use `_` to discard errors
- Package names MUST be lowercase, single-word, and descriptive of purpose
- Exported identifiers MUST have documentation comments starting with the identifier name
- Use composition over inheritance; prefer interfaces for abstraction
- Concurrency primitives (goroutines, channels) MUST only be used when parallelism provides
  clear benefit; avoid premature concurrency optimization

**Rationale**: Idiomatic code is predictable, reviewable, and maintainable by any Go developer.

### III. Test Coverage

All functionality MUST be testable and tested:

- Public APIs MUST have tests covering normal operation and edge cases
- Tests MUST be deterministic: no flaky tests, no time-dependent assertions without mocking
- Table-driven tests SHOULD be used for functions with multiple input/output scenarios
- Test files MUST live alongside the code they test (`*_test.go` in same package)
- Integration tests MUST be clearly marked and separable from unit tests via build tags

**Rationale**: Tests document behavior, prevent regressions, and enable confident refactoring.

## Technology Stack

**Language**: Go (latest stable version recommended)
**Testing**: `go test` with standard library `testing` package
**Linting**: `golangci-lint` with project-configured rules
**Formatting**: `go fmt` (non-negotiable)
**Build**: `go build` / `go install`
**Module Management**: Go modules (`go.mod`)

## Development Workflow

- All changes MUST be made on feature branches; direct commits to `main` are prohibited
- Code MUST compile without errors or warnings before commit
- All tests MUST pass before merge
- Pull requests MUST include:
  - Clear description of the change and its purpose
  - Tests for new functionality or bug fixes
  - Updated documentation if public API changes
- Commit messages MUST follow conventional format: `type: brief description`
  (types: feat, fix, docs, test, refactor, chore)

## Governance

This constitution defines the non-negotiable standards for the Stratos project:

- All code reviews MUST verify compliance with these principles
- Violations require explicit justification in the PR description and reviewer approval
- Amendments to this constitution require:
  1. Written proposal with rationale
  2. Review period of at least 24 hours
  3. Documentation of the change with version increment
- Version follows semantic versioning:
  - MAJOR: Principle removal or incompatible redefinition
  - MINOR: New principle or section added
  - PATCH: Clarifications, typo fixes, non-semantic changes

**Version**: 1.0.0 | **Ratified**: 2025-01-17 | **Last Amended**: 2025-01-17
