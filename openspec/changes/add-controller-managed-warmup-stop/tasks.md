# Tasks: Controller-Managed Warmup Stop

**Change ID**: add-controller-managed-warmup-stop

## Implementation Tasks

### 1. Add CompletionMode to PreWarmConfig API
**Files**: `api/v1alpha1/config_types.go`
**Effort**: Small

- Add `CompletionMode` field to `PreWarmConfig` struct
- Define `WarmupCompletionMode` type with `SelfStop` and `ControllerStop` values
- Add `GetCompletionMode()` helper that defaults to `SelfStop`
- Add kubebuilder validation enum

**Validation**:
- `make generate` succeeds
- `make manifests` succeeds
- Unit test for `GetCompletionMode()` default behavior

---

### 2. Update MonitorWarmup to support ControllerStop mode
**Files**: `internal/controller/manager.go`, `internal/controller/scale_up.go`
**Effort**: Medium

Currently `MonitorWarmup()` only polls EC2 instance state waiting for `Stopped`. It does NOT check the K8s node Ready condition.

**Changes needed:**
- Move `isNodeReady()` from `scale_up.go` to `manager.go` (or a shared location) so it can be reused
- In `MonitorWarmup()`, when instance is `Running`:
  - Check `pool.Spec.PreWarm.GetCompletionMode()`
  - If `ControllerStop`:
    - Check if node is Ready using `isNodeReady(node)`
    - If `startupTaintRemoval: WhenNetworkReady`, also check `isNetworkReady(node)`
    - When both conditions met → call `m.cloudProvider.StopInstance(ctx, instanceID)`
    - Then transition to standby (reuse existing standby transition logic)
  - If `SelfStop`: Keep existing behavior (wait for instance to self-stop)

**Validation**:
- Unit test: ControllerStop mode stops instance when node Ready
- Unit test: ControllerStop mode waits for NetworkReady when configured
- Unit test: SelfStop mode unchanged (backward compatibility)

---

### 3. Update MonitorCloudWarmup for ControllerStop mode
**Files**: `internal/controller/manager.go`
**Effort**: Small

Currently `MonitorCloudWarmup()` handles instances that exist in EC2 but may not have a K8s node yet. When the instance is `Running` and a node exists, it labels the node as warmup.

**Changes needed:**
- When instance is `Running` and node exists with warmup label:
  - If `ControllerStop` mode, check if node is Ready
  - If Ready (and NetworkReady if configured), call `StopInstance()` directly
  - Or delegate to `MonitorWarmup()` which will handle the stop
- Ensure timeout handling still works in both modes

**Validation**:
- Unit test: Cloud warmup monitoring works with ControllerStop
- Unit test: Timeout still triggers in ControllerStop mode

---

### 4. Add integration test for ControllerStop mode
**Files**: `tests/integration/controller_stop_test.go`
**Effort**: Medium

- Test: Instance launched with ControllerStop mode
- Test: Node joins cluster and becomes Ready
- Test: Stratos stops instance automatically
- Test: Node transitions to standby
- Test: Timeout handling in ControllerStop mode

**Validation**:
- `make test-integration` passes

---

### 5. Create Bottlerocket sample NodePool
**Files**: `config/samples/test_pool_bottlerocket.yaml`
**Effort**: Small

- Create sample with Bottlerocket AMI
- Use `completionMode: ControllerStop`
- TOML-only userdata (no bootstrap containers)
- Document in comments

**Validation**:
- Manual test: Bottlerocket nodes warm up successfully

---

### 6. Update AL2023 sample with optional ControllerStop mode
**Files**: `config/samples/test_pool_al2023.yaml`
**Effort**: Small

- Add commented example showing ControllerStop mode
- Keep existing SelfStop mode as default
- Document when to use each mode

**Validation**:
- Sample applies successfully

---

### 7. Update documentation
**Files**: `README.md`, `CLAUDE.md`
**Effort**: Small

- Document `preWarm.completionMode` option
- Explain when to use ControllerStop vs SelfStop
- Add Bottlerocket support notes

**Validation**:
- Documentation is clear and accurate

---

## Task Dependencies

```
[1] API changes
 └── [2] MonitorWarmup changes
      └── [3] MonitorCloudWarmup changes
           └── [4] Integration tests
                └── [5] Bottlerocket sample
                └── [6] AL2023 sample update
                └── [7] Documentation
```

Tasks 5, 6, 7 can be done in parallel after task 4.

## Estimated Scope

- **API changes**: ~20 lines
- **Controller changes**: ~50-80 lines
- **Tests**: ~100-150 lines
- **Samples/docs**: ~50 lines

Total: ~250-300 lines of changes
