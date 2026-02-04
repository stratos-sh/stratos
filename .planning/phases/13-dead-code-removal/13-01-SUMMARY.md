---
phase: 13-dead-code-removal
plan: 01
subsystem: scaling
tags: [dead-code, drain, cleanup]

dependency-graph:
  requires: [12-01]
  provides: ["drain.go without dead UncordonNode method"]
  affects: [14-01, 15-01]

tech-stack:
  added: []
  patterns: []

file-tracking:
  key-files:
    modified: [internal/scaling/drain.go]
    created: []

decisions: []

metrics:
  duration: "35s"
  completed: "2026-02-04"
---

# Phase 13 Plan 01: Remove Dead UncordonNode Method Summary

Removed dead UncordonNode method from drain.go -- zero callers confirmed by deadcode tool, CordonNode and DrainNode preserved intact.

## Task Results

| Task | Name | Commit | Status |
|------|------|--------|--------|
| 1 | Delete UncordonNode and verify | 196e923 | Done |

## What Changed

### internal/scaling/drain.go
- Deleted `UncordonNode` method (20 lines removed: method + doc comment)
- `CordonNode` method preserved (definition at line 78, called by `DrainNode` at line 107)
- `DrainNode` method preserved unchanged

## Verification Results

| Check | Result |
|-------|--------|
| `grep UncordonNode` in codebase | 0 matches |
| `grep CordonNode` in codebase | 3 matches (definition, signature, caller) |
| `go build ./...` | Pass |
| `go test ./internal/scaling/...` | Pass (0.069s) |

## Deviations from Plan

None -- plan executed exactly as written.

## Next Phase Readiness

No blockers. File paths stable for Phase 14 (type renames) and Phase 15 (file renames).
