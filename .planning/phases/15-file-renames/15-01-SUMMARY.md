---
phase: 15-file-renames
plan: 01
subsystem: scaling
tags: [rename, git-mv, file-naming, cleanup]
dependency-graph:
  requires: [14-type-renames]
  provides: [subject_role.go naming in scaling package]
  affects: [16-struct-field-rename]
tech-stack:
  added: []
  patterns: [subject_role.go file naming convention]
file-tracking:
  key-files:
    renamed:
      - from: internal/scaling/kubernetes.go
        to: internal/scaling/scaler.go
      - from: internal/scaling/kubernetes_test.go
        to: internal/scaling/scaler_test.go
      - from: internal/scaling/events.go
        to: internal/scaling/pod_events.go
      - from: internal/scaling/events_test.go
        to: internal/scaling/pod_events_test.go
decisions: []
metrics:
  duration: 69s
  completed: 2026-02-04
---

# Phase 15 Plan 01: File Renames Summary

**Pure git mv renames of scaling package files to follow subject_role.go naming convention, preserving full git blame history with zero content changes.**

## What Was Done

### Task 1: kubernetes.go -> scaler.go (84ab642)

Renamed `kubernetes.go` and `kubernetes_test.go` to `scaler.go` and `scaler_test.go` using `git mv`. The file's primary type is `Scaler` (renamed from `KubernetesStrategy` in Phase 14), so the filename now matches the type it contains.

- `go build ./...` passed (Go resolves by package path, not filename)
- `go test ./internal/scaling/...` passed
- `git log --follow` confirmed history preservation back to original creation

### Task 2: events.go -> pod_events.go (a989a44)

Renamed `events.go` and `events_test.go` to `pod_events.go` and `pod_events_test.go` using `git mv`. The file contains pod event handling logic, so `pod_events.go` follows the subject_role.go convention.

- `go build ./...` passed
- `go test ./internal/scaling/...` passed
- `git log --follow` confirmed history preservation back to original creation

## Verification Results

- 4 files renamed, 0 content changes (0 insertions, 0 deletions)
- Old files confirmed absent: kubernetes.go, kubernetes_test.go, events.go, events_test.go
- New files confirmed present: scaler.go, scaler_test.go, pod_events.go, pod_events_test.go
- `git diff HEAD~2 HEAD --stat` shows pure renames only
- `git log --follow` preserves blame history for both renamed files

## Deviations from Plan

None -- plan executed exactly as written.

## Next Phase Readiness

Phase 16 (struct field rename: NodePoolSpec.StrategyConfig -> ScalerConfig) can proceed. All file paths are now stable at their final names, so gopls rename operations will work correctly.
