---
phase: 03-strategy-package-extraction
plan: 02
subsystem: strategy
tags: [go, refactoring, file-decomposition, single-responsibility]
requires:
  - phase-03 plan-01 (strategy package relocated to internal/strategy/)
provides:
  - kubernetes/ sub-package with all files under 300 lines
  - networkReadinessChecker as reusable struct field
  - Focused source files matching test file structure
affects:
  - phase-04 (controller split will import individual focused files)
  - phase-06 (doc.go files will be added per package)
tech-stack:
  added: []
  patterns:
    - "File-per-concern decomposition within package"
    - "Struct field for reusable checker (instantiate once, use everywhere)"
key-files:
  created:
    - internal/strategy/kubernetes/scaling.go
    - internal/strategy/kubernetes/pod_assignments.go
    - internal/strategy/kubernetes/maintenance.go
    - internal/strategy/kubernetes/readiness.go
    - internal/strategy/kubernetes/drain_eviction.go
    - internal/strategy/kubernetes/maintenance_test.go
    - internal/strategy/kubernetes/readiness_test.go
    - internal/strategy/kubernetes/startup_taints_test.go
  modified:
    - internal/strategy/kubernetes/kubernetes.go
    - internal/strategy/kubernetes/drain.go
    - internal/strategy/kubernetes/kubernetes_test.go
key-decisions:
  - id: STRAT-02
    decision: "networkReadinessChecker stored as struct field on Strategy, instantiated once in New()"
    reason: "Avoid re-creating checker on every IsReady/removeNetworkReadinessTaintWhenReady call"
    impact: "Tests that call processStartupTaints with network checks must set networkChecker field"
  - id: STRAT-03
    decision: "startup_taints_test.go split from readiness_test.go to keep all test files under 300 lines"
    reason: "readiness_test.go was 386 lines with all processStartupTaints tests; splitting the 3 larger integration-style tests keeps both under 300"
    impact: "Test files now mirror source structure more closely"
duration: 7min
completed: 2026-02-03
---

# Phase 3 Plan 2: Kubernetes Strategy Decomposition Summary

Decomposed 911-line kubernetes.go into 5 focused files by concern, split 358-line drain.go into drain.go + drain_eviction.go, and refactored networkReadinessChecker from per-call instantiation to a reusable struct field. All source files under 300 lines.

## Performance

- Start: 2026-02-02T22:41:38Z
- End: 2026-02-02T22:48:14Z
- Duration: ~7min
- Tasks: 2/2

## Accomplishments

### File Decomposition Results

| File | Lines | Concern |
|------|-------|---------|
| kubernetes.go | 156 | Strategy struct, constructor, DrainAndStop, node query helpers |
| scaling.go | 246 | CheckDemand, OnScaleUp, FindScaleDownCandidates, countStartingNodes |
| pod_assignments.go | 212 | filterAssignedPods, createPodAssignments, estimatePodsPerNode, cleanup |
| maintenance.go | 189 | RunMaintenance, ensureTemplateLabels, clearStaleScaleUpAnnotations |
| readiness.go | 226 | IsReady, PrepareForRunning/Standby, processStartupTaints, taint removal |
| drain.go | 174 | drainHelper struct, config, CordonNode, UncordonNode, DrainNode |
| drain_eviction.go | 211 | Pod filtering, eviction, waiting, isNodeEmpty, isDaemonSetPod |

### Test File Decomposition

| File | Lines | Tests |
|------|-------|-------|
| kubernetes_test.go | 49 | TestIsNetworkReadinessTaintEnabled |
| maintenance_test.go | 143 | TestEnsureTemplateLabels_* (2 tests) |
| readiness_test.go | 206 | TestCheckStartupTaintTimeout, TestProcessStartupTaints_Disabled/AlreadyRemoved/NetworkNotReady |
| startup_taints_test.go | 210 | TestProcessStartupTaints_NetworkReady/Timeout/DefaultEnabled |

### networkReadinessChecker Refactoring

- Added `networkChecker *networkReadinessChecker` field to Strategy struct
- Constructor `New()` now creates checker once: `newNetworkReadinessChecker(c, cniPodSelector)`
- readiness.go uses `s.networkChecker` instead of calling `newNetworkReadinessChecker()` per-method
- Tests updated to set `networkChecker` field when testing methods that use it

## Task Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | 77d5307 | Decompose kubernetes.go into 5 focused files, refactor networkReadinessChecker |
| 2 | 6755e34 | Split drain.go + drain_eviction.go, decompose test files |

## Files Created/Modified

**Created (8):**
- `internal/strategy/kubernetes/scaling.go` - scaling logic
- `internal/strategy/kubernetes/pod_assignments.go` - pod assignment logic
- `internal/strategy/kubernetes/maintenance.go` - periodic maintenance
- `internal/strategy/kubernetes/readiness.go` - node readiness and taint processing
- `internal/strategy/kubernetes/drain_eviction.go` - eviction and pod filtering
- `internal/strategy/kubernetes/maintenance_test.go` - maintenance tests
- `internal/strategy/kubernetes/readiness_test.go` - readiness tests
- `internal/strategy/kubernetes/startup_taints_test.go` - startup taint integration tests

**Modified (3):**
- `internal/strategy/kubernetes/kubernetes.go` - trimmed from 911 to 156 lines
- `internal/strategy/kubernetes/drain.go` - trimmed from 358 to 174 lines
- `internal/strategy/kubernetes/kubernetes_test.go` - trimmed from 510 to 49 lines

## Decisions Made

1. **STRAT-02**: networkReadinessChecker as struct field - avoids per-call allocation, tests must set field explicitly
2. **STRAT-03**: startup_taints_test.go split from readiness_test.go - keeps all new test files under 300 lines

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Missing strategy import in kubernetes.go**
- Found during: Task 1
- Issue: DrainAndStop uses `strategy.ScaleDownCandidate` but the `strategy` import was missing after trimming imports
- Fix: Added `"github.com/stratos-sh/stratos/internal/strategy"` to kubernetes.go imports
- Files modified: kubernetes.go

**2. [Rule 3 - Blocking] Unused cloudprovider import in scaling.go**
- Found during: Task 1
- Issue: Initial extraction included `cloudprovider` import but no functions in scaling.go reference it directly
- Fix: Removed unused import
- Files modified: scaling.go

**3. [Rule 2 - Missing Critical] Test files needed networkChecker field**
- Found during: Task 2
- Issue: Tests for processStartupTaints_NetworkNotReady/NetworkReady/Timeout/DefaultEnabled use `s.networkChecker` after refactoring, but original tests did not set this field (they relied on the old per-call pattern)
- Fix: Added `networkChecker: newNetworkReadinessChecker(fakeClient, nil)` to Strategy construction in affected tests
- Files modified: readiness_test.go, startup_taints_test.go

**4. [Rule 1 - Bug] readiness_test.go exceeded 300-line target**
- Found during: Task 2 verification
- Issue: Initial readiness_test.go was 386 lines (7 tests), exceeding the 300-line target
- Fix: Extracted 3 larger tests into startup_taints_test.go
- Files created: startup_taints_test.go

## Issues Encountered

None. All deviations were minor and auto-fixed per deviation rules.

## Next Phase Readiness

Phase 3 is now complete. All strategy package extraction work is done:
- Plan 01: Relocated strategy/ to internal/strategy/ with sub-packages
- Plan 02: Decomposed kubernetes.go and drain.go into focused files

The internal/strategy/kubernetes/ package now has 16 files, all under 300 lines (except pre-existing events_test.go at 643 lines from plan 01, which is out of scope for this plan).

Ready for Phase 4 (Controller Split) with clear, navigable strategy code.
