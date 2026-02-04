---
phase: 07-type-relocation
plan: 01
subsystem: infra
tags: [go, refactoring, type-aliases, package-relocation]

# Dependency graph
requires:
  - phase: none
    provides: existing internal/strategy/kubernetes/ package
provides:
  - internal/scaling/ package with all Kubernetes scaling logic
  - ScalingDemand and ScaleDownCandidate types in internal/scaling/types.go
  - Type aliases in internal/strategy/interface.go for backward compatibility
affects: [08-consumer-rewire, 09-old-package-deletion]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Type alias migration: define types in target package, alias in source"

key-files:
  created:
    - internal/scaling/types.go
    - internal/scaling/doc.go
    - internal/scaling/kubernetes.go
    - internal/scaling/scaling.go
    - internal/scaling/capacity.go
    - internal/scaling/drain.go
    - internal/scaling/drain_eviction.go
    - internal/scaling/events.go
    - internal/scaling/maintenance.go
    - internal/scaling/network.go
    - internal/scaling/pod_assignments.go
    - internal/scaling/readiness.go
    - internal/scaling/events_test.go
    - internal/scaling/kubernetes_test.go
    - internal/scaling/maintenance_test.go
    - internal/scaling/network_test.go
    - internal/scaling/readiness_test.go
    - internal/scaling/startup_taints_test.go
  modified:
    - internal/strategy/interface.go

key-decisions:
  - "Type aliases (=) used instead of wrapper types for zero-overhead backward compatibility"
  - "corev1 import retained in interface.go because ScalingStrategy interface still references corev1.Node"

patterns-established:
  - "Additive migration: new package first, aliases for compat, consumers later"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 7 Plan 1: Type Relocation Summary

**Created internal/scaling/ package with 18 Go files and type aliases in strategy/interface.go for zero-breakage migration**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-03T19:14:39Z
- **Completed:** 2026-02-03T19:17:58Z
- **Tasks:** 2
- **Files modified:** 19 (18 created, 1 modified)

## Accomplishments
- Created internal/scaling/ package with all 18 Go files (11 source + 6 test + 1 types.go)
- Relocated ScalingDemand and ScaleDownCandidate type definitions to internal/scaling/types.go
- Added type aliases in internal/strategy/interface.go pointing to internal/scaling
- go build ./... passes, go vet passes, all tests pass -- zero consumer changes needed

## Task Commits

Each task was committed atomically:

1. **Task 1: Create internal/scaling/ package with all files and types** - `02d225a` (feat)
2. **Task 2: Add type aliases in strategy/interface.go and verify build** - `42ed63d` (refactor)

## Files Created/Modified
- `internal/scaling/types.go` - ScalingDemand and ScaleDownCandidate type definitions
- `internal/scaling/doc.go` - Package documentation for scaling package
- `internal/scaling/kubernetes.go` - Strategy struct, New constructor, DrainAndStop, node helpers
- `internal/scaling/scaling.go` - CheckDemand, OnScaleUp, FindScaleDownCandidates methods
- `internal/scaling/capacity.go` - ScaleCalculator for pod-to-node resource mapping
- `internal/scaling/drain.go` - Node drain orchestration (cordon, evict, wait)
- `internal/scaling/drain_eviction.go` - Pod eviction and empty-node detection
- `internal/scaling/events.go` - Pod event handler and unschedulable pod predicates
- `internal/scaling/maintenance.go` - Template label sync, stale annotation cleanup
- `internal/scaling/network.go` - CNI readiness checker (EKS, Cilium, Calico)
- `internal/scaling/pod_assignments.go` - Pod-to-node assignment tracking
- `internal/scaling/readiness.go` - Node readiness, startup taint management
- `internal/scaling/events_test.go` - Tests for well-known labels, pod satisfaction, tolerations
- `internal/scaling/kubernetes_test.go` - Tests for NetworkReadinessStrategy enum
- `internal/scaling/maintenance_test.go` - Tests for template label patching
- `internal/scaling/network_test.go` - Tests for network readiness conditions
- `internal/scaling/readiness_test.go` - Tests for startup taint timeout logic
- `internal/scaling/startup_taints_test.go` - Tests for startup taint processing
- `internal/strategy/interface.go` - Replaced struct defs with type aliases to scaling package

## Decisions Made
- Used Go type aliases (`type X = Y`) rather than new type definitions to ensure zero consumer changes -- existing code referencing `strategy.ScalingDemand` resolves to the same underlying type
- Kept `corev1` import in `interface.go` because the ScalingStrategy interface method signatures still reference `corev1.Node` directly

## Deviations from Plan

None - plan executed exactly as written.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- internal/scaling/ package ready for Phase 8 consumer rewire
- All consumers currently import strategy.ScalingDemand/ScaleDownCandidate via type aliases
- Phase 8 can incrementally change consumer imports from internal/strategy to internal/scaling
- internal/strategy/kubernetes/ still exists unchanged -- deletion is Phase 9

---
*Phase: 07-type-relocation*
*Completed: 2026-02-03*
