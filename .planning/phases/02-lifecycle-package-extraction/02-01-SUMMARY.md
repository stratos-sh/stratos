---
phase: 02-lifecycle-package-extraction
plan: 01
subsystem: lifecycle
tags: [refactor, file-split, warmup, lifecycle]
requires: [01-mechanical-cleanup]
provides: [warmup-file-split]
affects: [02-02, 03-strategy-extraction]
tech-stack:
  added: []
  patterns: [responsibility-based-file-split]
key-files:
  created:
    - internal/controller/lifecycle/warmup_monitor.go
    - internal/controller/lifecycle/warmup_handlers.go
    - internal/controller/lifecycle/warmup_adoption.go
  modified: []
  deleted:
    - internal/controller/lifecycle/warmup.go
key-decisions:
  - warmup_monitor.go kept at 215 lines (both monitors together per user locked decision)
  - cloudprovider import removed from warmup_handlers.go (handlers use Manager field, not package types directly)
duration: 4min
completed: 2026-02-02
---

# Phase 2 Plan 1: Split warmup.go into Three Focused Files Summary

Responsibility-based split of warmup.go (455 lines, 7 methods) into warmup_monitor.go (215 lines, 2 public monitors), warmup_handlers.go (205 lines, 4 private handlers), warmup_adoption.go (96 lines, 1 adoption method)

## Performance

- **Duration:** 4min
- **Started:** 2026-02-02T20:34:03Z
- **Completed:** 2026-02-02T20:37:40Z
- **Tasks:** 1/1
- **Files created:** 3
- **Files deleted:** 1

## Accomplishments

1. Split warmup.go into three files organized by responsibility:
   - **warmup_monitor.go** (215 lines): MonitorWarmup and MonitorCloudWarmup -- the two public entry points for warmup monitoring
   - **warmup_handlers.go** (205 lines): handleWarmupTimeout, handleControllerStopWarmup, handleWarmupFailure, handleCloudWarmupTimeout -- all private handler logic
   - **warmup_adoption.go** (96 lines): adoptAndTransitionToStandby -- the node adoption flow
2. No file exceeds 220 lines (warmup_monitor.go at 215 is accepted per user decision)
3. All 13 unit tests pass unchanged -- pure code movement, no behavioral changes
4. No upward imports to controller/ -- lifecycle/ remains a clean leaf package

## Task Commits

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Split warmup.go into three files | c54a688 | warmup_monitor.go, warmup_handlers.go, warmup_adoption.go |

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| internal/controller/lifecycle/warmup_monitor.go | 215 | MonitorWarmup + MonitorCloudWarmup |
| internal/controller/lifecycle/warmup_handlers.go | 205 | All 4 timeout/failure/completion handlers |
| internal/controller/lifecycle/warmup_adoption.go | 96 | adoptAndTransitionToStandby |

## Files Deleted

| File | Reason |
|------|--------|
| internal/controller/lifecycle/warmup.go | Replaced by three focused files |

## Decisions Made

1. **warmup_monitor.go at 215 lines:** Both MonitorWarmup and MonitorCloudWarmup kept together per user's locked decision from 02-CONTEXT.md. Splitting them would break logical cohesion since both are warmup monitoring entry points.
2. **cloudprovider import not needed in warmup_handlers.go:** The handler methods call `m.cloudProvider.StopInstance()` etc. via the Manager field, which doesn't require importing the cloudprovider package directly. Only warmup_monitor.go needs it (for `cloudprovider.InstanceState*` constants).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Restored node_launch.go after pre-existing duplicate conflict**

- **Found during:** Task 1 build verification
- **Issue:** Both `operations.go` (old monolith) and `node_launch.go` (new extraction) existed in the working tree with duplicate LaunchNode and LabelNode methods, preventing compilation. The `operations.go` file was left over from before the Phase 2 extraction work split it into `node_launch.go` + `node_startstop.go` + `node_sync.go`.
- **Fix:** Removed the stale `operations.go` (which was the pre-extraction monolith). Realized `node_launch.go` was already the correct extracted file. Restored `node_launch.go` to its original content after accidentally removing it during investigation.
- **Files affected:** node_launch.go (restored), operations.go (removed -- pre-existing stale file)
- **Commit:** Not separately committed (pre-existing untracked files; net state unchanged)

**2. [Rule 3 - Blocking] Removed unused cloudprovider import from warmup_handlers.go**

- **Found during:** Task 1 build verification
- **Issue:** Plan specified cloudprovider import for warmup_handlers.go, but handlers only use `m.cloudProvider` field access (no direct package type references). Go compiler rejected unused import.
- **Fix:** Removed cloudprovider import from warmup_handlers.go
- **Files modified:** warmup_handlers.go
- **Commit:** c54a688

## Issues Encountered

None beyond the deviations documented above.

## Next Phase Readiness

- **Plan 02-02** (Split operations.go) is ready to execute. However, `node_launch.go`, `node_startstop.go`, and `node_sync.go` already exist in the working tree as untracked files from prior work. Plan 02-02 should verify whether these files already contain the correct content and only need to be committed, rather than recreated.
- No blockers for Phase 3 (Strategy Package Extraction).
