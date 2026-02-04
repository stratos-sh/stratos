# Phase 1: Mechanical Cleanup - Context

**Gathered:** 2026-02-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Fix naming collisions, context propagation, error wrapping, unexport internals, and delete dead code. No package restructuring — files stay in their current packages. This is preparation for the structural phases that follow.

</domain>

<decisions>
## Implementation Decisions

### Naming Resolution
- reconcile.go vs reconciler.go: Claude's discretion on merge vs rename — pick the cleanest approach
- Duplicate nodeclass.go (api/v1alpha1/ and internal/controller/): Claude's discretion on disambiguation
- providers.go: Claude's discretion on rename (e.g., provider_cache.go)
- Fix ALL ambiguous file names in one pass — queries.go, validate.go, status.go, etc. — not just the worst offenders. Clean slate for later phases.

### Unexport Strategy
- Audit ALL packages (not just controller/) for unnecessary exports
- Unexport everything that's only used within its own package
- Consolidate related helpers into methods on their receiver type where it makes sense (e.g., countNodesByState → method on NodePoolReconciler)
- Claude audits usage and decides case-by-case what should be methods vs standalone functions, and what should be unexported

### Error Wrapping Style
- Claude's discretion on %w vs %v pattern (Go best practices)
- Claude's discretion on error message prefix format
- Audit custom error types in cloudprovider/types.go — ensure they're used consistently and errors.Is/As works correctly with them
- Standardize across the entire codebase

### Dead Code Scope
- Delete _extracted/ directory
- Full dead code audit using staticcheck/unused linter — find all unreachable code, unused functions, dead variables across entire codebase
- Delete orphaned test files (spot_test.go, spot_replacement_test.go) where feature code no longer exists
- Ignore git status / deleted files — user will handle git separately

### Context Propagation
- Replace all 39 context.Background() instances in non-test production code with proper context from reconciliation loop
- No user preferences to capture — straightforward mechanical fix

### Claude's Discretion
- Exact file naming choices (reconcile.go resolution, nodeclass.go disambiguation, providers.go rename)
- Which functions become methods vs stay as functions
- Error wrapping pattern (%w vs %v rules)
- Error message prefix format
- How aggressively to unexport based on usage analysis

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard Go approaches. User wants clean, readable code and trusts Claude's judgment on Go conventions.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 01-mechanical-cleanup*
*Context gathered: 2026-02-02*
