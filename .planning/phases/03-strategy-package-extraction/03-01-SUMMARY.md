---
phase: 03-strategy-package-extraction
plan: 01
subsystem: strategy
tags: [go, refactoring, package-structure, import-paths]
requires:
  - phase-02 (lifecycle package extraction complete)
provides:
  - internal/strategy/ as top-level domain package
  - internal/strategy/kubernetes/ sub-package with Strategy type
  - internal/strategy/githubactions/ sub-package with Strategy type
  - Factory logic inlined in controller to avoid import cycles
affects:
  - phase-03 plan-02 (test relocation to new package paths)
  - phase-04 (controller split will import from internal/strategy/)
tech-stack:
  added: []
  patterns:
    - "Interface at package root, implementations in sub-packages"
    - "Factory at consumer to avoid parent/child import cycles"
key-files:
  created:
    - internal/strategy/interface.go
    - internal/strategy/kubernetes/kubernetes.go
    - internal/strategy/kubernetes/drain.go
    - internal/strategy/kubernetes/events.go
    - internal/strategy/kubernetes/network.go
    - internal/strategy/kubernetes/capacity.go
    - internal/strategy/kubernetes/kubernetes_test.go
    - internal/strategy/kubernetes/events_test.go
    - internal/strategy/kubernetes/network_test.go
    - internal/strategy/githubactions/githubactions.go
  modified:
    - internal/controller/reconciler.go
    - internal/controller/provider_cache.go
    - internal/controller/setup.go
    - internal/controller/pod_assignment_test.go
  deleted:
    - internal/controller/strategy/ (entire directory)
    - internal/strategy/factory.go (moved to controller)
key-decisions:
  - id: STRAT-01
    decision: "Factory logic moved from internal/strategy/factory.go to inline in internal/controller/provider_cache.go"
    reason: "Go import cycles: strategy/ -> strategy/kubernetes -> strategy/ is not allowed. Factory must live at consumer level."
    impact: "Factory is not independently importable, but it was only ever used by the controller"
duration: 10min
completed: 2026-02-03
---

# Phase 3 Plan 1: Strategy Package Relocation Summary

Relocated strategy package from internal/controller/strategy/ to internal/strategy/ with implementations split into sub-packages following Go conventions: interface at root, kubernetes/ and githubactions/ sub-packages with renamed types (Strategy, New).

## Performance

- Start: 2026-02-02T22:28:43Z
- End: 2026-02-02T22:38:48Z
- Duration: 10min
- Tasks: 2/2

## Accomplishments

1. Created new directory structure: internal/strategy/, internal/strategy/kubernetes/, internal/strategy/githubactions/
2. Moved all 11 strategy files to new locations with correct package declarations
3. Renamed exported symbols to follow Go sub-package conventions (dropped redundant prefixes)
4. Updated all 4 consumer files with new import paths
5. Resolved Go import cycle by moving factory logic to controller package
6. All unit tests pass, go build ./... compiles cleanly

## Task Commits

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Create directory structure and move files | fbe4cc5 | 11 files moved, old dir removed |
| 2 | Update import paths and fix cross-package refs | 0646f08 | 4 consumers updated, factory relocated |

## Symbol Renames

| Old Name | New Name | Package |
|----------|----------|---------|
| strategy.KubernetesStrategy | kubernetes.Strategy | internal/strategy/kubernetes |
| strategy.NewKubernetesStrategy | kubernetes.New | internal/strategy/kubernetes |
| strategy.KubernetesPodEventHandler | kubernetes.PodEventHandler | internal/strategy/kubernetes |
| strategy.KubernetesUnschedulablePodPredicate | kubernetes.UnschedulablePodPredicate | internal/strategy/kubernetes |
| strategy.GitHubActionsStrategy | githubactions.Strategy | internal/strategy/githubactions |
| strategy.NewGitHubActionsStrategy | githubactions.New | internal/strategy/githubactions |

## Decisions Made

### STRAT-01: Factory moved to controller package

**Decision:** The factory function `NewStrategy()` was moved from `internal/strategy/factory.go` to an inline `newStrategy()` function in `internal/controller/provider_cache.go`.

**Reason:** Go does not allow import cycles. The factory in `internal/strategy/` imported sub-packages (`strategy/kubernetes`, `strategy/githubactions`), while those sub-packages imported the parent for shared types (`strategy.ScalingDemand`, `strategy.ScaleDownCandidate`). This created a cycle: `strategy -> strategy/kubernetes -> strategy`.

**Impact:** The factory is not independently importable, but it was only ever called from `getOrCreateStrategy()` in the controller. The interface and shared types remain at `internal/strategy/` for any consumer.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Import cycle between strategy/ and sub-packages**

- **Found during:** Task 2
- **Issue:** `internal/strategy/factory.go` imported `internal/strategy/kubernetes` and `internal/strategy/githubactions`, while those sub-packages imported `internal/strategy` for shared types, creating a Go import cycle.
- **Fix:** Removed `internal/strategy/factory.go` and inlined the factory logic into `internal/controller/provider_cache.go` as unexported `newStrategy()`. This is the standard Go pattern for avoiding parent/child import cycles.
- **Files modified:** internal/controller/provider_cache.go, internal/strategy/factory.go (deleted)
- **Commit:** 0646f08

## Issues Encountered

None beyond the import cycle (documented above as deviation).

## Next Phase Readiness

- All strategy files are at their final locations
- Package structure matches Go conventions: interface at root, implementations in sub-packages
- Integration tests may need import path updates (covered by plan 03-02)
- No blockers for continuing to plan 03-02
