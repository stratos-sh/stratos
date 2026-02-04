# Phase 11: Final Cleanup and Verification - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Remove all residual references to the deleted strategy/GitHub packages (RBAC markers, dependencies, linter config), clean up any clearly dead code discovered along the way, and verify the full test/lint/build suites pass clean. This is the final phase of the v1.1 Simplify Scaling milestone.

</domain>

<decisions>
## Implementation Decisions

### Linter config cleanup
- Delete depguard rules that reference `strategy/` package boundaries — do NOT replace with new guards for `internal/scaling/`
- Scan the entire `.golangci.yml` for any clearly dead config (references to deleted paths, unused skip patterns, stale excludes) and remove it, even if not directly related to v1.1
- If something is clearly dead, remove it regardless of origin; don't leave ambiguous rules

### Dead code cleanup
- Remove the unused `recorder` field from `drainHelper` in `internal/scaling/drain.go` — it's stored but never read by any method
- This includes updating `newDrainHelper` signature and all call sites that pass the recorder argument
- Pre-existing dead code, but fits naturally in this cleanup phase

### Claude's Discretion
- Commit organization (separate commits per concern vs bundled cleanup)
- RBAC cleanup scope (minimum: remove Secrets access marker; broader: audit all markers)
- Dependency tidying approach (minimum: `go mod tidy`; broader: audit indirect deps)
- Verification depth beyond the required `make test`, `make test-integration`, `make lint`

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches for the RBAC, dependency, and verification work.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 11-final-cleanup*
*Context gathered: 2026-02-03*
