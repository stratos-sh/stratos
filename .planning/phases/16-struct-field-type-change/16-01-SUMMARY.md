---
phase: 16-struct-field-type-change
plan: 01
subsystem: scaling
tags: [type-safety, refactor, struct-field]
dependency-graph:
  requires: [15-file-renames]
  provides: [typed-pods-field]
  affects: []
tech-stack:
  added: []
  patterns: [typed-struct-fields-over-interface]
key-files:
  created: []
  modified:
    - internal/scaling/types.go
    - internal/scaling/scaling.go
decisions:
  - id: "16-01-01"
    decision: "Replace Metadata interface{} with Pods []corev1.Pod"
    rationale: "Only value ever stored was []corev1.Pod; concrete field is compiler-verified"
metrics:
  duration: "52s"
  completed: "2026-02-04"
---

# Phase 16 Plan 01: Struct Field Type Change Summary

**One-liner:** Replace unsafe `ScalingDemand.Metadata interface{}` with typed `Pods []corev1.Pod` field, eliminating runtime type assertion

## What Was Done

### Task 1: Replace Metadata field with Pods field and update all references

**Commit:** `be48bc0`

Changes in `internal/scaling/types.go`:
- Replaced `Metadata interface{}` field with `Pods []corev1.Pod`
- Updated field comment to describe the concrete purpose

Changes in `internal/scaling/scaling.go`:
- `CheckDemand` (line 95): `Metadata: unassignedPods` changed to `Pods: unassignedPods`
- `OnScaleUp` (lines 138-141): Replaced type assertion block `demand.Metadata.([]corev1.Pod)` with direct `len(demand.Pods) == 0` check
- `OnScaleUp` (line 143): `pods` variable replaced with `demand.Pods` direct access

## Decisions Made

| ID | Decision | Rationale |
|----|----------|-----------|
| 16-01-01 | Typed `Pods []corev1.Pod` instead of `interface{}` | Only value ever stored was `[]corev1.Pod`; compiler-verified type safety eliminates runtime assertion |

## Deviations from Plan

None -- plan executed exactly as written.

## Verification Results

| Check | Result |
|-------|--------|
| `go build ./...` | Pass |
| `go test ./internal/scaling/...` | Pass (0.069s) |
| `golangci-lint run ./...` | 0 issues |
| No `Metadata` in `internal/scaling/*.go` | Confirmed (zero matches) |
| No `interface{}` in `types.go` | Confirmed (zero matches) |
| `Pods []corev1.Pod` in `types.go` | Present on line 27 |

## Next Phase Readiness

This is the final plan in the v1.1.1 naming cleanup milestone. All 5 plans across phases 12-16 are now complete:
- Phase 12: Dead code removal
- Phase 13: Dead code removal (continued)
- Phase 14: Type renames (Strategy -> Scaler)
- Phase 15: File renames (kubernetes.go -> scaler.go, events.go -> pod_events.go)
- Phase 16: Struct field type change (Metadata interface{} -> Pods []corev1.Pod)
