---
phase: 08-controller-rewiring
plan: 01
subsystem: controller
tags: [scaling, refactoring, strategy-removal, controller-runtime]

# Dependency graph
requires:
  - phase: 07-type-relocation
    provides: scaling package with Strategy struct and type aliases
provides:
  - Reconciler with single scaler *scaling.Strategy field
  - Simplified newNodeManager/newNodeManagerWithHooks methods
  - setup.go using scaling.PodEventHandler and scaling.UnschedulablePodPredicate
affects: [09-old-code-deletion, 10-crd-pruning]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Direct struct field instead of interface dispatch for single implementation"
    - "Two explicit newNodeManager variants (with/without hooks) replacing variadic pattern"

key-files:
  created: []
  modified:
    - internal/controller/nodepool/reconciler.go
    - internal/controller/nodepool/provider_cache.go
    - internal/controller/nodepool/reconciler_helpers.go
    - internal/controller/nodepool/cloud_sync.go
    - internal/controller/nodepool/setup.go
    - internal/controller/nodepool/doc.go
    - internal/controller/nodepool/pod_assignment_test.go

key-decisions:
  - "Single scaler field replaces per-pool strategy cache -- all pools share one stateless Strategy"
  - "newNodeManager split into two explicit methods instead of variadic optional pattern"

patterns-established:
  - "Direct field access: r.scaler replaces getOrCreateStrategy() cache lookup"
  - "Explicit method variants: newNodeManager (no hooks) vs newNodeManagerWithHooks"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 8 Plan 1: Controller Rewiring Summary

**Replaced strategy cache/factory/interface in NodePool controller with single scaler *scaling.Strategy field, removing 81 lines of abstraction machinery**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-03T19:56:11Z
- **Completed:** 2026-02-03T19:59:17Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Removed strategies map, strategiesMu mutex, getOrCreateStrategy() cache, and newStrategy() factory (81 lines net reduction)
- Replaced all strategy.ScalingStrategy interface parameters with concrete *scaling.Strategy across 5 helper methods
- Rewired setup.go to initialize scaler via scaling.New() and use scaling.PodEventHandler/UnschedulablePodPredicate
- Split variadic newNodeManager into two explicit methods: newNodeManager (no hooks, for replenish) and newNodeManagerWithHooks (for all scaling paths)

## Task Commits

Each task was committed atomically:

1. **Task 1: Rewire controller to use *scaling.Strategy directly** - `352b0c1` (feat)
2. **Task 2: Run tests to verify behavior preservation** - verification only, no commit

## Files Created/Modified
- `internal/controller/nodepool/reconciler.go` - Reconciler struct with scaler field, simplified reconcileNodePool
- `internal/controller/nodepool/provider_cache.go` - newNodeManager/newNodeManagerWithHooks, removed factory/cache
- `internal/controller/nodepool/reconciler_helpers.go` - All helper method signatures updated to *scaling.Strategy
- `internal/controller/nodepool/cloud_sync.go` - Removed getOrCreateStrategy calls, uses newNodeManagerWithHooks
- `internal/controller/nodepool/setup.go` - Scaler initialization, scaling.PodEventHandler import
- `internal/controller/nodepool/doc.go` - Updated package comment to reference scaling.Strategy
- `internal/controller/nodepool/pod_assignment_test.go` - Updated to use scaling.NewScaleCalculator

## Decisions Made
- Single scaler field replaces per-pool strategy cache -- all pools share one stateless Strategy instance (the Strategy holds no pool-specific state, so caching per pool was unnecessary overhead)
- Split newNodeManager into two explicit named methods instead of keeping variadic optional pattern -- clearer intent at each call site

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Controller fully rewired to use scaling package directly
- Ready for phase 09 (old code deletion) -- strategy/, githubactions/, kubernetes/ packages can now be deleted
- All type aliases in scaling package still forward to strategy/kubernetes types for backward compatibility during migration

---
*Phase: 08-controller-rewiring*
*Completed: 2026-02-03*
