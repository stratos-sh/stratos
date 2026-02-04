---
phase: 02-lifecycle-package-extraction
plan: 02
subsystem: lifecycle
tags: [go, refactor, file-split, lifecycle, operations]
requires: [01-mechanical-cleanup]
provides: [focused-lifecycle-files, node-launch, node-startstop, node-sync]
affects: [03-strategy-package-extraction]
tech-stack:
  added: []
  patterns: [single-concern-files, lifecycle-domain-grouping]
key-files:
  created:
    - internal/controller/lifecycle/node_launch.go
    - internal/controller/lifecycle/node_startstop.go
    - internal/controller/lifecycle/node_sync.go
  modified: []
  deleted:
    - internal/controller/lifecycle/operations.go
key-decisions: []
duration: 4min
completed: 2026-02-02
---

# Phase 02 Plan 02: Split operations.go into focused lifecycle files

Split operations.go (355 lines, 9 methods) into three single-concern files: node_launch.go (provisioning), node_startstop.go (start/stop transitions), node_sync.go (state sync and discovery)

## Performance

- **Duration:** 4min
- **Started:** 2026-02-02T20:34:33Z
- **Completed:** 2026-02-02T20:38:24Z
- **Tasks:** 1/1
- **Files created:** 3
- **Files deleted:** 1

## Accomplishments

1. Split operations.go into three focused files organized by lifecycle concern:
   - **node_launch.go** (113 lines): Instance provisioning (LaunchNode) and node labeling (LabelNode)
   - **node_startstop.go** (166 lines): State transitions (TransitionState), start (StartNode), stop (StopNode)
   - **node_sync.go** (137 lines): Cloud state sync (SyncNodeState), node discovery (FindNodeByInstanceID), cleanup (deleteNode), annotation helpers (setLastStartedAnnotation)
2. All files are well under the 200-line target
3. Deleted operations.go after confirming all methods were placed in new files
4. Verified no upward imports from lifecycle/ to controller/
5. Verified nodestate/ remains a clean leaf package

## Task Commits

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Split operations.go into three files | 9c84e52 | node_launch.go, node_startstop.go, node_sync.go |

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| internal/controller/lifecycle/node_launch.go | 113 | LaunchNode + LabelNode (instance provisioning) |
| internal/controller/lifecycle/node_startstop.go | 166 | TransitionState + StartNode + StopNode (lifecycle transitions) |
| internal/controller/lifecycle/node_sync.go | 137 | SyncNodeState + FindNodeByInstanceID + deleteNode + setLastStartedAnnotation (state sync) |

## Files Deleted

| File | Reason |
|------|--------|
| internal/controller/lifecycle/operations.go | Replaced by three focused files |

## Decisions Made

None -- plan executed exactly as written. File groupings followed the plan's lifecycle concern domains.

## Deviations from Plan

None -- plan executed exactly as written.

## Issues Encountered

None.

## Next Phase Readiness

- lifecycle/ package compiles cleanly with all three new files
- All 13 unit tests pass unchanged
- No upward imports from lifecycle/ to controller/
- nodestate/ remains a clean leaf package
- Ready for Phase 3 (Strategy Package Extraction) once all Phase 2 plans complete
