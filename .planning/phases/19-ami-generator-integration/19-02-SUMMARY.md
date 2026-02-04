---
phase: 19
plan: 02
subsystem: controller-validation
tags: [bottlerocket, image-pre-pull, status-conditions, validation]
dependency-graph:
  requires: [18-01]
  provides: [ImagePrePullSupported-condition, bottlerocket-validation]
  affects: []
tech-stack:
  added: []
  patterns: [non-blocking-status-condition, type-assertion-nodeclass]
key-files:
  created:
    - internal/controller/nodepool/nodepool_validation_test.go
  modified:
    - api/v1alpha1/nodepool_types.go
    - internal/controller/nodepool/nodepool_validation.go
decisions:
  - condition-is-informational: "ImagePrePullSupported condition is non-blocking -- Bottlerocket instances still launch without cached images"
  - condition-removal-pattern: "Condition is removed (not set to True) when images are not configured or AMI family supports pre-pull"
metrics:
  duration: 2min
  completed: 2026-02-04
---

# Phase 19 Plan 02: Bottlerocket Image Pre-Pull Warning Summary

Non-blocking ImagePrePullSupported=False condition for Bottlerocket NodePools with configured images, using type-assertion on AWSNodeClass.BootstrapTemplate.

## What Was Done

### Task 1: Add ImagePrePullSupported condition type and validation function
**Commit:** d51f3f6

Added `ConditionTypeImagePrePullSupported` constant and two reason constants (`ReasonBottlerocketNotSupported`, `ReasonImagePrePullSupported`) to `nodepool_types.go`. Implemented `checkImagePrePullSupport` method on Reconciler in `nodepool_validation.go` that:

- Returns early (removing condition) when no images are configured
- Type-asserts NodeClass to `*AWSNodeClass` to check `BootstrapTemplate`
- Sets `ImagePrePullSupported=False` with reason `BottlerocketNotSupported` for Bottlerocket AMIs
- Removes condition for AL2/AL2023 AMIs (handles user switching AMI family)
- Only mutates in-memory conditions -- caller persists status

### Task 2: Add tests for Bottlerocket image pre-pull validation
**Commit:** d2a6f55

Created 6 test cases covering all AMI family and image configuration combinations:

| Test | AMI | Images | Expected |
|------|-----|--------|----------|
| BottlerocketWithImages | Bottlerocket | yes | condition False |
| AL2023WithImages | AL2023 | yes | no condition |
| AL2WithImages | AL2 | yes | no condition |
| BottlerocketNoImages | Bottlerocket | no | no condition |
| NilPreWarm | Bottlerocket | nil | no condition |
| RemovesPreviousCondition | AL2023 | yes | condition removed |

## Deviations from Plan

None -- plan executed exactly as written.

## Decisions Made

1. **Condition is informational only** -- `checkImagePrePullSupport` returns void, does not return error, does not prevent instance launch. The function signature makes this explicit.
2. **Remove rather than set True** -- When images are not configured or AMI supports pre-pull, the condition is removed entirely rather than set to True. This keeps the conditions list clean for the common case.
3. **Zero-value Reconciler for tests** -- Since `checkImagePrePullSupport` only mutates in-memory structs and makes no API calls, tests use `&Reconciler{}` with nil client. This is intentionally simple and matches the function's scope.

## Verification Results

- `go build ./api/v1alpha1/... ./internal/controller/nodepool/...` -- compiles cleanly
- `go test -v ./internal/controller/nodepool/...` -- all 21 tests pass (6 new + 15 existing)
- `go vet ./api/v1alpha1/... ./internal/controller/nodepool/...` -- no issues
