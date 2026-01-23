# Tasks: Add Startup Taints Support

**Change ID**: add-startup-taints

## Overview

Ordered list of implementation tasks for adding startup taints support to Stratos.

---

## Phase 1: API Changes

### Task 1.1: Add StartupTaints and StartupTaintRemoval fields to NodeTemplate
**File**: `api/v1alpha1/nodepool_types.go`
**Validation**: `make generate && make manifests`

- [x] Add `StartupTaintRemovalMode` type with `WhenNetworkReady` and `External` constants
- [x] Add `StartupTaints []corev1.Taint` field to `NodeTemplate` struct
- [x] Add `StartupTaintRemoval StartupTaintRemovalMode` field to `NodeTemplate` struct
- [x] Add kubebuilder markers (validation enum, default value)
- [x] Add documentation comments explaining purpose and modes

### Task 1.2: Regenerate CRD manifests
**Command**: `make generate && make manifests`
**Validation**: CRD includes startupTaints field

- [x] Run code generation
- [x] Verify CRD YAML includes the new field
- [x] Verify deep copy methods are generated

---

## Phase 2: Network Readiness Detection (CNI-Agnostic)

### Task 2.1: Create network readiness checker
**File**: `internal/controller/network_readiness.go` (new)
**Validation**: Unit tests pass

- [x] Create `NetworkReadinessChecker` struct
- [x] Implement `IsReady(node *corev1.Node) bool` checking node conditions:
  - EKS: `NetworkingReady: True` (set by eks-node-monitoring-agent)
  - Cilium/Calico: `NetworkUnavailable: False` (set by CNI plugin)
- [x] Implement `GetNetworkConditionReason(node) string` for logging
- [x] Add unit tests with various node condition scenarios

### Task 2.2: Add constants for startup taint configuration
**File**: `internal/controller/state.go`
**Validation**: Build succeeds

- [x] Add `StartupTaintRemovalTimeout` constant (2 minutes)
- [x] Add `NetworkingReadyCondition = "NetworkingReady"` constant
- [x] Add annotation constants for tracking startup taint state

---

## Phase 3: Modify Node Startup Flow

### Task 3.1: Update prepareNodeForRunning to preserve startup taints
**File**: `internal/controller/manager.go`
**Validation**: Unit tests pass, existing tests still pass

- [x] Modify `prepareNodeForRunning()` signature to accept `pool *stratosv1alpha1.NodePool`
- [x] Do NOT remove taints that are in `pool.Spec.Template.StartupTaints`
- [x] Update all call sites of `prepareNodeForRunning()`
- [x] Update existing unit tests

### Task 3.2: Implement ProcessStartupTaints with mode support
**File**: `internal/controller/manager.go`
**Validation**: Unit tests pass

- [x] Add `ProcessStartupTaints()` method to NodeManager
- [x] Implement `WhenNetworkReady` mode using `NetworkReadinessChecker`
- [x] Implement `External` mode (just check if taints present, don't remove)
- [x] Add helper functions: `hasTaintWithKeyAndEffect()`, `removeTaintByKeyAndEffect()`
- [x] Add unit tests for both modes

### Task 3.3: Add timeout handling for startup taints
**File**: `internal/controller/manager.go`
**Validation**: Unit tests pass

- [x] Implement timeout check using `AnnotationLastStarted`
- [x] `WhenNetworkReady` mode: Force remove taints after timeout (with warning event)
- [x] `External` mode: Emit warning but do NOT remove taints
- [x] Add unit tests for timeout scenarios in both modes

---

## Phase 4: Reconciliation Integration

### Task 4.1: Add network readiness checker to NodeManager
**File**: `internal/controller/manager.go`
**Validation**: Build succeeds

- [x] Add `networkChecker *NetworkReadinessChecker` field to `NodeManager`
- [x] Initialize checker in `NewNodeManager()`

### Task 4.2: Call ProcessStartupTaints in reconciliation loop
**File**: `internal/controller/nodepool_controller.go`
**Validation**: Integration tests pass

- [x] In running node reconciliation, call `ProcessStartupTaints()`
- [x] Handle errors appropriately (log, don't fail reconciliation)
- [x] Check timeout and handle based on mode
- [x] Track nodes waiting for startup taint removal

---

## Phase 5: Metrics and Observability

### Task 5.1: Add startup taint metrics
**File**: `internal/metrics/metrics.go`
**Validation**: Metrics appear in /metrics endpoint

- [x] Add `stratos_startup_taint_removal_total` counter
- [x] Add `stratos_startup_taint_duration_seconds` histogram
- [x] Add recording functions

### Task 5.2: Add Kubernetes events
**File**: `internal/controller/manager.go`
**Validation**: Events visible with kubectl describe

- [x] Emit event when startup taints are removed
- [x] Emit warning event on timeout
- [x] Emit warning if CNI not ready after threshold

---

## Phase 6: Samples and Documentation

### Task 6.1: Update sample NodePool
**File**: `config/samples/nodepool_sample.yaml`
**Validation**: Sample is valid YAML, applies successfully

- [x] Add `startupTaints` configuration to full example
- [x] Add `startupTaints` to minimal example
- [x] Update header comments with setup instructions
- [x] Ensure userData includes matching `--register-with-taints`

### Task 6.2: Fix existing sample header (remove incorrect TAINT_MANAGED advice)
**File**: `config/samples/nodepool_sample.yaml`
**Validation**: Comments are accurate

- [x] Remove references to TAINT_MANAGED (VPC CNI doesn't support this)
- [x] Add correct explanation of how startup taints work

---

## Phase 7: Testing

### Task 7.1: Unit tests for network readiness checker
**File**: `internal/controller/network_readiness_test.go` (new)
**Validation**: `go test ./internal/controller/... -run NetworkReadiness`

- [x] Test EKS node with `NetworkingReady: True` → ready
- [x] Test EKS node with `NetworkingReady: False` → not ready
- [x] Test Cilium node with `NetworkUnavailable: False` → ready
- [x] Test Cilium node with `NetworkUnavailable: True` → not ready
- [x] Test node with no network conditions → not ready
- [x] Test `GetNetworkConditionReason()` returns correct reasons

### Task 7.2: Unit tests for ProcessStartupTaints
**File**: `internal/controller/startup_taints_test.go` (new)
**Validation**: `go test ./internal/controller/... -run StartupTaint`

- [x] Test WhenNetworkReady mode: removal when network ready
- [x] Test WhenNetworkReady mode: no removal when network not ready
- [x] Test WhenNetworkReady mode: timeout behavior (force remove)
- [x] Test External mode: no removal by Stratos
- [x] Test External mode: detects external removal
- [x] Test External mode: timeout behavior (warning only, no removal)
- [x] Test node with no startup taints (no-op)

### Task 7.3: Integration tests
**File**: `tests/integration/startup_taints_test.go` (new)
**Validation**: `make test-integration`

- [x] Test full scale-up flow with startup taints (WhenNetworkReady)
- [x] Test full scale-up flow with startup taints (External)
- [x] Test that taints are preserved initially
- [x] Test that taints are removed after network ready
- [x] Test backward compatibility (no startup taints configured)

---

## Completion Checklist

- [x] All unit tests pass (`make test`)
- [x] All integration tests pass (`make test-integration`)
- [x] Linting passes (`make lint`) - Note: golangci-lint has environment dependency issues but go vet passes
- [x] CRD is updated (`make manifests`)
- [x] Sample YAML is valid and accurate
- [x] No breaking changes to existing NodePools (startupTaints is optional)

---

## Dependencies

```
Task 1.1 ─┬─> Task 1.2
          │
Task 2.1 ─┴─> Task 2.2
          │
          ├─> Task 3.1 ──> Task 3.2 ──> Task 3.3
          │
          └─> Task 4.1 ──> Task 4.2
                            │
Task 5.1 ─────────────────>─┤
                            │
Task 6.1 ─────────────────>─┤
Task 6.2 ─────────────────>─┤
                            │
                            └─> Task 7.1 ──> Task 7.2 ──> Task 7.3
```

**Parallelizable**: Tasks 1.x, 2.x, 5.x, 6.x can be done in parallel after understanding the design.
