---
phase: 14
plan: 01
subsystem: scaling
tags: [rename, gopls, type-safety]
dependency-graph:
  requires: [13-01]
  provides: [Scaler-type, nodeDrainer-type, drainOptions-type]
  affects: [15-01]
tech-stack:
  added: []
  patterns: [gopls-atomic-rename]
key-files:
  created: []
  modified:
    - internal/scaling/kubernetes.go
    - internal/scaling/scaling.go
    - internal/scaling/readiness.go
    - internal/scaling/maintenance.go
    - internal/scaling/pod_assignments.go
    - internal/scaling/drain.go
    - internal/scaling/drain_eviction.go
    - internal/scaling/types.go
    - internal/scaling/doc.go
    - internal/scaling/kubernetes_test.go
    - internal/scaling/maintenance_test.go
    - internal/scaling/startup_taints_test.go
    - internal/scaling/readiness_test.go
    - internal/controller/nodepool/reconciler.go
    - internal/controller/nodepool/reconciler_helpers.go
    - internal/controller/nodepool/provider_cache.go
    - internal/controller/nodepool/setup.go
    - internal/controller/nodepool/doc.go
    - internal/controller/nodepool/lifecycle/manager.go
    - internal/controller/nodepool/lifecycle/warmup_handlers.go
    - internal/controller/doc.go
    - internal/cloudprovider/doc.go
    - internal/cloudprovider/fake/doc.go
decisions:
  - id: TS-1
    decision: "Strategy -> Scaler via gopls rename"
    rationale: "Strategy was vestigial name from removed ScalingStrategy interface; Scaler describes the concrete role"
  - id: TS-2
    decision: "drainHelper -> nodeDrainer via gopls rename"
    rationale: "nodeDrainer is more descriptive and follows Go naming conventions"
  - id: TS-3
    decision: "drainConfig -> drainOptions via gopls rename"
    rationale: "drainOptions aligns with Go convention for configuration structs"
  - id: TS-4
    decision: "Renamed local variable drainHelper -> drainer, drainCfg -> drainOpts"
    rationale: "Local variables matched the old type names and would have triggered stale reference grep"
metrics:
  duration: 4min
  completed: 2026-02-04
---

# Phase 14 Plan 01: Type Renames Summary

**gopls atomic rename of Strategy->Scaler, drainHelper->nodeDrainer, drainConfig->drainOptions across 23 files with full comment alignment**

## What Was Done

### Task 1: gopls rename for all three types + constructors (5 renames)

Executed five sequential gopls rename operations:

1. `Strategy -> Scaler` (exported, cross-package) -- touched 13 files across scaling/ and controller/nodepool/ packages
2. `drainHelper -> nodeDrainer` (unexported, package-internal) -- touched drain.go and drain_eviction.go
3. `drainConfig -> drainOptions` (unexported, package-internal) -- touched drain.go and kubernetes.go
4. `defaultDrainConfig -> defaultDrainOptions` (function rename) -- touched drain.go and kubernetes.go
5. `newDrainHelper -> newNodeDrainer` (constructor rename) -- touched drain.go and kubernetes.go

All renames were atomic via gopls v0.21.0 with the `-w` flag.

### Task 2: Stale comment and string literal updates

gopls does not touch comments or string literals. Updated 23 comment sites and 2 string literals:

- `scaling/doc.go`: Strategy -> Scaler in key types list
- `scaling/kubernetes.go`: New() godoc, local variable drainHelper -> drainer
- `scaling/types.go`: "strategy-specific" -> "scaler-specific"
- `scaling/drain.go`: 3 godoc comments for drainOptions, defaultDrainOptions, newNodeDrainer
- `controller/nodepool/doc.go`: 2 references to scaling.Strategy -> scaling.Scaler
- `controller/nodepool/reconciler.go`: 3 comments (scaler field, cloud provider, NodeHooks)
- `controller/nodepool/reconciler_helpers.go`: 4 comments + 1 log message string literal
- `controller/nodepool/setup.go`: "scaling strategy" -> "pod-demand scaler"
- `controller/nodepool/provider_cache.go`: compile-time assertion comment
- `controller/nodepool/lifecycle/manager.go`: NodeHooks interface comments (3 sites)
- `controller/nodepool/lifecycle/warmup_handlers.go`: 1 comment + 1 log message string
- `controller/doc.go`: "strategy dependencies" -> "scaler dependencies"
- `cloudprovider/doc.go`: "strategy packages" -> "scaler packages"
- `cloudprovider/fake/doc.go`: "strategy tests" -> "scaler tests"

### Task 3: Full verification

- `go build ./...` -- passed
- `go vet ./...` -- passed
- `go test ./...` -- all tests passed
- Strategy stale grep (excluding NetworkReadinessStrategy/StrategyEnum) -- zero code or comment matches (only local test variable names remain, which are intentional)
- drainHelper/drainConfig stale grep -- zero matches

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Renamed local variable drainHelper and drainCfg**

- **Found during:** Task 2
- **Issue:** Local variable `drainHelper` in kubernetes.go:77 matched the stale reference grep pattern for `drainHelper`. Similarly `drainCfg` was a vestigial name now that the type is `drainOptions`.
- **Fix:** Renamed `drainHelper` -> `drainer` and `drainCfg` -> `drainOpts` to pass the verification grep and maintain naming consistency.
- **Files modified:** internal/scaling/kubernetes.go
- **Commit:** 514a17f

## Commits

| Hash | Type | Description |
|------|------|-------------|
| b68a728 | refactor | gopls rename: Strategy->Scaler, drainHelper->nodeDrainer, drainConfig->drainOptions |
| 514a17f | docs | Update stale comments and string literals for type renames |

## Verification Results

All Phase 14 success criteria satisfied:

1. `scaling.Scaler` used everywhere that previously said `scaling.Strategy` -- including compile-time assertion in `provider_cache.go`
2. `nodeDrainer` used everywhere that previously said `drainHelper` -- including constructor calls
3. `drainOptions` used everywhere that previously said `drainConfig` -- including struct literals
4. `grep -rn "Strategy" internal/ --include="*.go" | grep -v NetworkReadinessStrategy | grep -v StrategyEnum` returns only local test variable names (testStrategy, scalingStrategy, taintStrategy) which are intentionally preserved
5. `go build ./...` and `go test ./...` pass (all tests green)

## Next Phase Readiness

Phase 15 (File Renames) can proceed. All types are now correctly named:
- `scaling.Scaler` (the concrete pod-demand scaler)
- `nodeDrainer` (internal drain helper)
- `drainOptions` (drain configuration)

File paths are stable and unchanged, which is what Phase 15 needs for its file rename operations.
