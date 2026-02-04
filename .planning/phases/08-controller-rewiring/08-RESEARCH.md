# Phase 8: Controller Rewiring - Research

**Researched:** 2026-02-03
**Domain:** Go refactoring -- remove strategy interface dispatch from controller, replace with direct concrete type calls
**Confidence:** HIGH

## Summary

Phase 8 rewires the NodePool controller to use a single `*scaling.Strategy` field instead of the current `strategies map[string]strategy.ScalingStrategy` with mutex, factory, and cache machinery. After Phase 7 (type relocation), the `internal/scaling/` package already contains all the code -- this phase changes only the controller's wiring to bypass the `strategy.ScalingStrategy` interface and call scaling methods directly on the concrete type.

The codebase was exhaustively analyzed. The changes touch exactly 5 files in `internal/controller/nodepool/` plus 1 file in `internal/controller/nodepool/cloud_sync.go`. The integration tests do NOT reference the strategy package and will continue to work after rewiring because they create `nodepool.Reconciler` structs directly and the scaler will be initialized transparently. The `scaling.Strategy` struct already exists (Phase 7 created it) and already satisfies `lifecycle.NodeHooks`, so the bridge pattern is preserved.

**Primary recommendation:** Replace the `strategies map` + `strategiesMu` + `getOrCreateStrategy()` + `newStrategy()` machinery with a single `scaler *scaling.Strategy` field on the Reconciler, created once in `Setup()`. All helper methods that currently accept `strategy.ScalingStrategy` as a parameter switch to using `r.scaler` directly. The `newNodeManager()` variadic pattern simplifies to always pass `r.scaler` as NodeHooks (except in `replenishStandby` which intentionally skips hooks).

## Standard Stack

Not applicable -- this phase uses only Go's built-in package system and the `go build` toolchain. No new libraries are needed. The only imports that change are from `internal/strategy` and `internal/strategy/kubernetes` to `internal/scaling`.

## Architecture Patterns

### Current State: 7 Files Consuming Strategy

Based on exhaustive grep of `strategy.ScalingStrategy`, `getOrCreateStrategy`, and `strategy.ScaleDownCandidate` references:

| File | What It Uses | Lines of Interest |
|------|-------------|-------------------|
| `reconciler.go` | `strategies map[string]strategy.ScalingStrategy` field, `strategiesMu sync.RWMutex` field, `import "internal/strategy"`, `scaler strategy.ScalingStrategy` param on `startStandbyNodes` | 41, 69-72, 164, 209 |
| `reconciler_helpers.go` | `strategy.ScalingStrategy` param on 5 methods, `strategy.ScaleDownCandidate` struct literal, `import "internal/strategy"` | 35, 40, 76, 100, 130-132, 171, 195 |
| `provider_cache.go` | `getOrCreateStrategy()`, `newStrategy()` factory, compile-time assertions, `newNodeManager()` variadic, imports of `strategy`, `strategy/kubernetes`, `strategy/githubactions` | 107-182 |
| `setup.go` | `kubernetes.PodEventHandler`, `kubernetes.UnschedulablePodPredicate`, `import "internal/strategy/kubernetes"` | 34, 71-72 |
| `cloud_sync.go` | Three calls to `getOrCreateStrategy()`, three calls to `newNodeManager(provider, scalingStrategy)` | 40-44, 70-74, 106-110 |
| `pool_maintenance.go` | One call to `newNodeManager(provider)` WITHOUT strategy (intentionally no hooks for launch-only path) | 126 |
| `nodepool_validation.go` | References `stratosv1alpha1.ScalingStrategyGitHubActions` in validation block (lines 50-64) -- NOT a Phase 8 concern but good to note for Phase 10 |

### Target State: What Each File Becomes

#### `reconciler.go` -- Field replacement

```go
// BEFORE (lines 69-72)
strategiesMu sync.RWMutex
strategies   map[string]strategy.ScalingStrategy

// AFTER
scaler *scaling.Strategy
```

The `sync` import can be removed from `reconciler.go` because `strategiesMu` is the only sync usage in this file. The `import "internal/strategy"` changes to `import "internal/scaling"`.

The call in `reconcileNodePool()` changes from:
```go
scaler, err := r.getOrCreateStrategy(nodePool)
if err != nil { ... }
```
To simply using `r.scaler` directly (no map lookup, no error).

The `startStandbyNodes()` parameter changes from `scaler strategy.ScalingStrategy` to `scaler *scaling.Strategy` (or drops the parameter entirely and uses `r.scaler`).

#### `reconciler_helpers.go` -- Parameter type changes + type reference changes

Every helper method signature changes:

| Method | Current Param | New Approach |
|--------|--------------|--------------|
| `handleScaleUp` | `scaler strategy.ScalingStrategy` | `scaler *scaling.Strategy` |
| `handleMonitoring` | `scaler strategy.ScalingStrategy` | `scaler *scaling.Strategy` |
| `handleScaleDown` | `scaler strategy.ScalingStrategy` | `scaler *scaling.Strategy` |
| `processScaleDownCandidate` | `scaler strategy.ScalingStrategy` | `scaler *scaling.Strategy` |
| `handleMaxRuntimeRecycling` | `scaler strategy.ScalingStrategy` | `scaler *scaling.Strategy` |

Type reference changes:
- `strategy.ScaleDownCandidate{Node: ...}` (line 195) becomes `scaling.ScaleDownCandidate{Node: ...}`
- `candidate strategy.ScaleDownCandidate` (line 130) becomes `candidate scaling.ScaleDownCandidate`
- `import "internal/strategy"` becomes `import "internal/scaling"`

**Design decision: Keep scaler as a parameter or use r.scaler field?**

Option A: Replace all `scaler strategy.ScalingStrategy` params with `scaler *scaling.Strategy` params (minimal diff, same threading pattern).

Option B: Remove scaler params from all helpers, use `r.scaler` field directly (cleaner code but larger diff, more call sites change).

**Recommendation: Option A (keep parameters, just change the type).** Rationale:
1. Smaller diff means lower risk of merge issues
2. The parameter threading pattern is already established and understood
3. `reconcileNodePool()` already binds `scaler` as a local variable; helpers use it naturally
4. Phase 8 goal is "rewire", not "rearchitect"

#### `provider_cache.go` -- Major simplification

**Delete entirely:**
- `getOrCreateStrategy()` function (lines 126-152) -- replaced by `r.scaler` field
- `newStrategy()` factory function (lines 154-182) -- no longer needed (single concrete type)
- Compile-time assertion `_ lifecycle.NodeHooks = (*githubactions.Strategy)(nil)` (line 111) -- githubactions goes away in Phase 9

**Keep but modify:**
- `newNodeManager()` (lines 115-123) -- simplify variadic signature

**Remove imports:**
- `"github.com/stratos-sh/stratos/internal/strategy"` -- no longer referenced
- `"github.com/stratos-sh/stratos/internal/strategy/githubactions"` -- deleted in Phase 9 but unreferenced after this phase
- `"github.com/stratos-sh/stratos/internal/strategy/kubernetes"` -- replaced by `internal/scaling`

**New compile-time assertion (replaces the two deleted ones):**
```go
var _ lifecycle.NodeHooks = (*scaling.Strategy)(nil)
```

#### `setup.go` -- Import path change

```go
// BEFORE
import "github.com/stratos-sh/stratos/internal/strategy/kubernetes"
// ...
kubernetes.PodEventHandler(mgr.GetClient())
kubernetes.UnschedulablePodPredicate()

// AFTER
import "github.com/stratos-sh/stratos/internal/scaling"
// ...
scaling.PodEventHandler(mgr.GetClient())
scaling.UnschedulablePodPredicate()
```

**Scaler initialization in Setup():**
```go
func Setup(mgr ctrl.Manager, opts SetupOptions) error {
    scaler := scaling.New(mgr.GetClient(), mgr.GetEventRecorder("nodepool-controller"), opts.CapacityProvider, opts.CNIPodSelector)
    r := &Reconciler{
        // ...existing fields...
        scaler: scaler,
    }
    return r.SetupWithManager(mgr)
}
```

Note: `SetupWithManager()` currently creates the recorder (`r.Recorder = mgr.GetEventRecorder(...)`). The scaler needs a recorder, so either:
(a) Create recorder in `Setup()` before the scaler, or
(b) Create scaler in `SetupWithManager()` after the recorder is available.

**Recommendation:** Create the scaler in `SetupWithManager()`, after `r.Recorder` is set. This matches the existing pattern where recorder is set first.

#### `cloud_sync.go` -- Remove getOrCreateStrategy calls

Three call sites (lines 40, 70, 106) all follow this pattern:
```go
scalingStrategy, err := r.getOrCreateStrategy(nodePool)
if err != nil {
    return fmt.Errorf("failed to get strategy: %w", err)
}
nodeMgr := r.newNodeManager(provider, scalingStrategy)
```

After rewiring, these simplify to:
```go
nodeMgr := r.newNodeManager(provider)
```

Where `newNodeManager` always uses `r.scaler` internally (see below).

#### `newNodeManager()` -- Signature simplification

**Current (provider_cache.go, lines 115-123):**
```go
func (r *Reconciler) newNodeManager(provider cloudprovider.CloudProvider, scalingStrategy ...strategy.ScalingStrategy) *lifecycle.Manager {
    mgr := lifecycle.NewManager(r.Client, r.Recorder, provider, r.ClusterName)
    if len(scalingStrategy) > 0 && scalingStrategy[0] != nil {
        if hooks, ok := scalingStrategy[0].(lifecycle.NodeHooks); ok {
            mgr = mgr.WithNodeHooks(hooks)
        }
    }
    return mgr
}
```

**Two options for the new signature:**

Option A -- Always attach hooks (since `r.scaler` always implements NodeHooks):
```go
func (r *Reconciler) newNodeManager(provider cloudprovider.CloudProvider) *lifecycle.Manager {
    return lifecycle.NewManager(r.Client, r.Recorder, provider, r.ClusterName).
        WithNodeHooks(r.scaler)
}
```

But this breaks `pool_maintenance.go` line 126 (`r.newNodeManager(provider)`) which intentionally skips hooks for the launch-only path. The `replenishStandby` function launches new nodes -- it should NOT call `PrepareForRunning` or `PrepareForStandby` hooks because new instances go through warmup, not the running/standby preparation.

Option B -- Two methods:
```go
func (r *Reconciler) newNodeManager(provider cloudprovider.CloudProvider) *lifecycle.Manager {
    return lifecycle.NewManager(r.Client, r.Recorder, provider, r.ClusterName)
}

func (r *Reconciler) newNodeManagerWithHooks(provider cloudprovider.CloudProvider) *lifecycle.Manager {
    return r.newNodeManager(provider).WithNodeHooks(r.scaler)
}
```

**Recommendation: Option B (two methods).** This makes the hook attachment explicit. Callers that need hooks (scale-up, scale-down, cloud sync, warmup monitoring) use `newNodeManagerWithHooks`. The launch-only path (`replenishStandby`) uses `newNodeManager`. The intent is clear from the method name.

### Pattern: Integration Test Impact

Integration tests create the Reconciler directly (in `suite_test.go`):
```go
reconciler = &nodepool.Reconciler{
    Client:        mgr.GetClient(),
    Scheme:        mgr.GetScheme(),
    ClusterName:   testClusterName,
    CloudProvider: "fake",
}
```

After Phase 8, `reconciler.scaler` is a private field -- it cannot be set directly by tests. The scaler must be created inside `SetupWithManager()` (the method tests already call). This works because:
1. `SetupWithManager()` already initializes `r.Recorder`
2. `scaling.New()` accepts nil `capacityProvider` and nil `cniPodSelector` (verified -- both are nil-safe)
3. The scaler will be created from the reconciler's `Client` and `Recorder` fields

**No test changes needed.** The scaler is created internally by `SetupWithManager()`.

### Anti-Patterns to Avoid

- **Do NOT rename `Strategy` to `Scaler` in this phase.** Phase 7 created the struct as `Strategy` in `internal/scaling/`. The earlier research (Phase 7) recommended deferring the rename. Renaming now would create churn in Phase 7's files. If renaming is desired, it can be a separate phase or can be deferred.

- **Do NOT delete the `internal/strategy/` package in this phase.** That is Phase 9's job. Phase 8 only removes the controller's DEPENDENCY on it. After Phase 8, `internal/strategy/` still exists but nothing in `internal/controller/nodepool/` imports it.

- **Do NOT remove the `ScalingStrategy` CRD field or `GitHubActions` validation in this phase.** That is Phase 10's job. Phase 8 is purely controller wiring.

- **Do NOT remove the `import "sync"` from `reconciler.go` without checking if any other sync usage exists.** The `cloudProvidersMu` also uses `sync.RWMutex` in the same struct. Only `strategiesMu` is removed.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Strategy parameter threading cleanup | Custom dependency injection | Direct struct field access (`r.scaler`) | The reconciler already owns the lifecycle of the scaler; field access is the Go-idiomatic approach |
| NodeHooks bridge | Custom adapter pattern | Compile-time interface satisfaction (`var _ lifecycle.NodeHooks = (*scaling.Strategy)(nil)`) | Go's structural typing means the `Strategy` struct already satisfies `NodeHooks` -- just verify with a compile-time check |
| Scaler lifecycle management | Custom init/shutdown | Create in `SetupWithManager()` | The scaler has no cleanup needs (no goroutines, no connections) -- just create and store |

**Key insight:** The entire Phase 8 is a deletion/simplification phase. There is nothing to build -- only things to remove and simplify. If you find yourself writing new logic, you are likely over-engineering.

## Common Pitfalls

### Pitfall 1: Missing `cloud_sync.go` Call Sites

**What goes wrong:** The planner maps changes to `reconciler.go`, `reconciler_helpers.go`, `provider_cache.go`, and `setup.go` but misses `cloud_sync.go`, which has THREE separate calls to `getOrCreateStrategy()`.
**Why it happens:** The earlier research documents focused on the "helper method signatures" and didn't catalog `cloud_sync.go`.
**How to avoid:** The complete list of `getOrCreateStrategy()` callers is:
1. `reconciler.go` line 164 (in `reconcileNodePool`)
2. `cloud_sync.go` line 40 (in `syncNodesWithCloud`)
3. `cloud_sync.go` line 70 (in `monitorWarmupNodes`)
4. `cloud_sync.go` line 106 (in `monitorCloudWarmupInstances`)
**Warning signs:** Build error: `r.getOrCreateStrategy undefined`.

### Pitfall 2: Breaking the No-Hooks Launch Path

**What goes wrong:** After simplifying `newNodeManager()`, the `replenishStandby` function (pool_maintenance.go line 126) accidentally gets hooks attached, causing `PrepareForRunning` to be called on freshly launched nodes in warmup state.
**Why it happens:** If `newNodeManager()` always attaches hooks, all callers get them.
**How to avoid:** Use two separate methods (`newNodeManager` and `newNodeManagerWithHooks`) so the no-hooks intent is explicit.
**Warning signs:** Newly launched nodes get uncordoned or have taints removed before they complete warmup.

### Pitfall 3: Forgetting `sync` Import Still Needed

**What goes wrong:** Removing `strategiesMu sync.RWMutex` and then removing the `sync` import, but `cloudProvidersMu sync.RWMutex` still exists in the same struct.
**Why it happens:** Assuming `strategiesMu` is the only sync usage.
**How to avoid:** Check: `cloudProvidersMu sync.RWMutex` (line 65) and `cloudProviders map[string]cloudprovider.CloudProvider` (line 67) with their mutex are KEPT. The `sync` import stays.
**Warning signs:** Build error: `undefined: sync.RWMutex`.

### Pitfall 4: Integration Tests Not Setting Scaler

**What goes wrong:** If the scaler field is exported and tests need to set it, or if `SetupWithManager()` doesn't initialize it, tests get nil pointer panics.
**Why it happens:** The test setup in `suite_test.go` creates `&nodepool.Reconciler{...}` directly and then calls `reconciler.SetupWithManager(mgr)`.
**How to avoid:** The scaler must be initialized inside `SetupWithManager()` using `r.Client` and `r.Recorder`. Since `SetupWithManager()` sets `r.Recorder` first, the scaler can be created right after. The field is private (`scaler` not `Scaler`), so tests cannot and need not set it.
**Warning signs:** `nil pointer dereference` in any scaling method during integration tests.

### Pitfall 5: Stale Compile-Time Assertions

**What goes wrong:** The compile-time assertion `_ lifecycle.NodeHooks = (*githubactions.Strategy)(nil)` remains in `provider_cache.go`, but the `githubactions` import is removed.
**Why it happens:** Removing the import but not the assertion.
**How to avoid:** Remove both the `githubactions` import AND the assertion. Replace with `_ lifecycle.NodeHooks = (*scaling.Strategy)(nil)`.
**Warning signs:** Build error: `undefined: githubactions`.

### Pitfall 6: ScalingStrategy Enum Still Referenced in Validation

**What goes wrong:** `nodepool_validation.go` line 50 references `stratosv1alpha1.ScalingStrategyGitHubActions`. Removing the strategy factory without updating validation causes the validation to reference a CRD type that doesn't functionally matter anymore.
**Why it happens:** Assuming Phase 8 only touches strategy wiring, not validation.
**How to avoid:** The validation block (lines 50-64) can be left as-is for Phase 8. It references CRD types in `api/v1alpha1/strategy_types.go` which still exist. Phase 10 (CRD changes) deletes it. Phase 8 success criteria do not require changing validation.
**Warning signs:** None -- this is safe to leave for now.

## Code Examples

### Example 1: Reconciler Struct After Rewiring

```go
// reconciler.go
type Reconciler struct {
    client.Client
    Scheme           *runtime.Scheme
    Recorder         events.EventRecorder
    ClusterName      string
    CloudProvider    string
    ClusterConfig    *config.ClusterConfig
    CapacityProvider cloudprovider.InstanceCapacityProvider
    CNIPodSelector   map[string]string
    RateLimitConfig  *aws.RateLimitConfig

    // cloudProvidersMu protects cloudProviders map from concurrent access
    cloudProvidersMu sync.RWMutex
    // cloudProviders caches cloud provider instances per pool
    cloudProviders map[string]cloudprovider.CloudProvider

    // scaler handles pod-demand scaling decisions and node preparation
    scaler *scaling.Strategy
}
```

### Example 2: Scaler Initialization in SetupWithManager

```go
// setup.go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    r.Recorder = mgr.GetEventRecorder("nodepool-controller")

    // Initialize scaler for pod-demand scaling
    r.scaler = scaling.New(r.Client, r.Recorder, r.CapacityProvider, r.CNIPodSelector)

    return ctrl.NewControllerManagedBy(mgr).
        For(&stratosv1alpha1.NodePool{}).
        Watches(
            &corev1.Pod{},
            scaling.PodEventHandler(mgr.GetClient()),
            builder.WithPredicates(scaling.UnschedulablePodPredicate()),
        ).
        Watches(&corev1.Node{}, nodeEventHandler(mgr.GetClient())).
        Watches(&stratosv1alpha1.AWSNodeClass{}, nodeClassEventHandler(mgr.GetClient(), "AWSNodeClass")).
        Named("nodepool").
        Complete(r)
}
```

### Example 3: Two Node Manager Methods

```go
// provider_cache.go
var _ lifecycle.NodeHooks = (*scaling.Strategy)(nil)

// newNodeManager creates a lifecycle.Manager without scaling hooks.
// Use for launch-only operations (replenishStandby) where hooks are not needed.
func (r *Reconciler) newNodeManager(provider cloudprovider.CloudProvider) *lifecycle.Manager {
    return lifecycle.NewManager(r.Client, r.Recorder, provider, r.ClusterName)
}

// newNodeManagerWithHooks creates a lifecycle.Manager with the scaler's NodeHooks.
// Use for operations that need node preparation (scale-up, scale-down, sync, warmup).
func (r *Reconciler) newNodeManagerWithHooks(provider cloudprovider.CloudProvider) *lifecycle.Manager {
    return r.newNodeManager(provider).WithNodeHooks(r.scaler)
}
```

### Example 4: reconcileNodePool After Rewiring

```go
// reconciler.go
func (r *Reconciler) reconcileNodePool(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (ctrl.Result, error) {
    provider := r.getCloudProvider(nodePool.Name)

    // PRIORITY: Check for scale-up need FIRST
    if result, scaled, scaleErr := r.handleScaleUp(ctx, nodePool, r.scaler); scaleErr != nil {
        return result, scaleErr
    } else if scaled {
        return result, nil
    }

    r.handleMonitoring(ctx, nodePool, provider, r.scaler)

    // ... count nodes, metrics ...

    r.handleScaleDown(ctx, nodePool, provider, r.scaler)
    r.handleMaxRuntimeRecycling(ctx, nodePool, provider, r.scaler)

    // ... replenishment, status update ...
}
```

### Example 5: cloud_sync.go After Rewiring

```go
// cloud_sync.go
func (r *Reconciler) syncNodesWithCloud(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
    logger := log.FromContext(ctx)

    nodes, err := r.getNodesForPool(ctx, nodePool.Name)
    if err != nil {
        return fmt.Errorf("failed to get nodes: %w", err)
    }

    nodeMgr := r.newNodeManagerWithHooks(provider)

    for i := range nodes {
        node := &nodes[i]
        if err := nodeMgr.SyncNodeState(ctx, nodePool, node); err != nil {
            logger.Error(err, "Failed to sync node state", "node", node.Name)
        }
    }
    return nil
}
```

## State of the Art

Not applicable -- standard Go refactoring. No library versions or evolving patterns involved.

## Complete Change Inventory

### Files Modified (6)

| File | Changes | Complexity |
|------|---------|------------|
| `reconciler.go` | (1) Replace `strategies`/`strategiesMu` fields with `scaler *scaling.Strategy`. (2) Change import from `internal/strategy` to `internal/scaling`. (3) Remove `getOrCreateStrategy` call in `reconcileNodePool`. (4) Change `startStandbyNodes` param type. | Medium |
| `reconciler_helpers.go` | (1) Change import from `internal/strategy` to `internal/scaling`. (2) Replace 5 function param types from `strategy.ScalingStrategy` to `*scaling.Strategy`. (3) Replace `strategy.ScaleDownCandidate` with `scaling.ScaleDownCandidate`. | Medium |
| `provider_cache.go` | (1) Delete `getOrCreateStrategy()`. (2) Delete `newStrategy()`. (3) Delete githubactions compile-time assertion. (4) Add `scaling.Strategy` compile-time assertion. (5) Split `newNodeManager` into two methods. (6) Remove strategy/githubactions/kubernetes imports. (7) Add scaling import. | High |
| `setup.go` | (1) Change import from `internal/strategy/kubernetes` to `internal/scaling`. (2) Change `kubernetes.PodEventHandler` to `scaling.PodEventHandler`. (3) Change `kubernetes.UnschedulablePodPredicate` to `scaling.UnschedulablePodPredicate`. (4) Add scaler initialization in `SetupWithManager`. | Medium |
| `cloud_sync.go` | (1) Remove 3 `getOrCreateStrategy` calls. (2) Change 3 `newNodeManager(provider, scalingStrategy)` calls to `newNodeManagerWithHooks(provider)`. | Low |
| `doc.go` | Update text: replace "strategy.ScalingStrategy" reference with "scaling.Strategy" | Cosmetic |

### Files NOT Modified (confirmed safe)

| File | Why No Changes |
|------|---------------|
| `pool_maintenance.go` | Already calls `r.newNodeManager(provider)` (no strategy param). Signature doesn't change. |
| `nodepool_validation.go` | GitHub Actions validation stays until Phase 10. No strategy import used. |
| `lifecycle/manager.go` | NodeHooks interface unchanged. `*scaling.Strategy` continues to satisfy it. |
| `internal/scaling/*.go` | Already created in Phase 7. No changes needed. |
| `internal/strategy/interface.go` | Still exists with type aliases. Phase 9 deletes it. |
| `tests/integration/*.go` | No strategy imports. `SetupWithManager()` handles scaler creation. |

### Lines Deleted vs Added (Estimated)

| Category | Lines Deleted | Lines Added | Net |
|----------|--------------|-------------|-----|
| `getOrCreateStrategy()` | ~27 | 0 | -27 |
| `newStrategy()` factory | ~26 | 0 | -26 |
| Compile-time assertions (old) | ~4 | ~1 | -3 |
| `strategies`/`strategiesMu` fields | ~4 | ~2 | -2 |
| `getOrCreateStrategy` calls (4 sites) | ~12 | 0 | -12 |
| `newNodeManagerWithHooks` method | 0 | ~4 | +4 |
| Scaler init in SetupWithManager | 0 | ~3 | +3 |
| Import changes (6 files) | ~6 | ~6 | 0 |
| Type reference changes | ~8 | ~8 | 0 |
| **Total** | **~87** | **~24** | **-63** |

Net result: approximately 63 lines deleted. This is a pure simplification.

## Open Questions

### 1. Should helper methods drop the scaler parameter entirely?

- **What we know:** The scaler is now a field on the Reconciler. Passing it as a parameter is redundant but follows the existing pattern.
- **What's unclear:** Whether the cleaner code (no param) outweighs the smaller diff (keep param, change type).
- **Recommendation:** Keep parameters, just change types. This is the minimal-diff approach that satisfies the Phase 8 success criteria ("invoke `r.scaler.Method()` directly instead of going through interface dispatch"). The success criteria say "r.scaler.Method()" which suggests field access, but the critical change is replacing interface dispatch with concrete type. Either approach satisfies the goal.

### 2. Should `Strategy` be renamed to `Scaler` in this phase?

- **What we know:** The Phase 8 success criteria say "single `scaler *scaling.Scaler` field" -- suggesting the type is named `Scaler`. But the current type in `internal/scaling/` is named `Strategy` (from Phase 7).
- **What's unclear:** Whether renaming is part of Phase 8 or a separate concern.
- **Recommendation:** Rename `Strategy` to `Scaler` in `internal/scaling/kubernetes.go` as part of this phase since the success criteria explicitly reference `*scaling.Scaler`. This is a single find-and-replace within the scaling package (struct name + constructor) plus updating all method receivers.

## Sources

### Primary (HIGH confidence)

- Direct codebase investigation: read all 7 consuming files, verified every call site, traced every import
- `go build ./...`: confirmed current build compiles clean
- Phase 7 verification report: confirmed `internal/scaling/` package exists with all types and methods
- Phase 7 research: confirmed type aliases are in place for backward compatibility

### Secondary (MEDIUM confidence)

- Earlier milestone research (`/.planning/research/ARCHITECTURE.md`, `STACK.md`): comprehensive analysis done at milestone planning time, verified against current codebase state

## Metadata

**Confidence breakdown:**
- Architecture: HIGH - complete file inventory, every call site traced, no ambiguity
- Pitfalls: HIGH - each pitfall derived from direct codebase analysis
- Change inventory: HIGH - exhaustive grep + read of every affected file

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (stable -- codebase is the source of truth)

## Appendix: All `getOrCreateStrategy` and `strategy.ScalingStrategy` References

### `getOrCreateStrategy` callers (4 sites to eliminate)

1. `reconciler.go:164` -- `scaler, err := r.getOrCreateStrategy(nodePool)` in `reconcileNodePool`
2. `cloud_sync.go:40` -- `scalingStrategy, err := r.getOrCreateStrategy(nodePool)` in `syncNodesWithCloud`
3. `cloud_sync.go:70` -- `scalingStrategy, err := r.getOrCreateStrategy(nodePool)` in `monitorWarmupNodes`
4. `cloud_sync.go:106` -- `scalingStrategy, err := r.getOrCreateStrategy(nodePool)` in `monitorCloudWarmupInstances`

### `strategy.ScalingStrategy` type references (8 sites to change)

1. `reconciler.go:72` -- `strategies map[string]strategy.ScalingStrategy` field
2. `reconciler.go:209` -- `scaler strategy.ScalingStrategy` param on `startStandbyNodes`
3. `reconciler_helpers.go:40` -- `scaler strategy.ScalingStrategy` param on `handleScaleUp`
4. `reconciler_helpers.go:76` -- `scaler strategy.ScalingStrategy` param on `handleMonitoring`
5. `reconciler_helpers.go:100` -- `scaler strategy.ScalingStrategy` param on `handleScaleDown`
6. `reconciler_helpers.go:132` -- `scaler strategy.ScalingStrategy` param on `processScaleDownCandidate`
7. `reconciler_helpers.go:171` -- `scaler strategy.ScalingStrategy` param on `handleMaxRuntimeRecycling`
8. `provider_cache.go:115` -- `scalingStrategy ...strategy.ScalingStrategy` variadic param on `newNodeManager`

### `strategy.ScaleDownCandidate` type references (2 sites to change)

1. `reconciler_helpers.go:130` -- `candidate strategy.ScaleDownCandidate` param on `processScaleDownCandidate`
2. `reconciler_helpers.go:195` -- `candidate := strategy.ScaleDownCandidate{Node: exceededNodes[i]}` struct literal

### `newNodeManager` callers (8 sites, 7 need `WithHooks`, 1 does not)

**With hooks (change to `newNodeManagerWithHooks`):**
1. `reconciler.go:233` -- `r.newNodeManager(provider, scaler)` in `startStandbyNodes`
2. `reconciler_helpers.go:113` -- `r.newNodeManager(provider, scaler)` in `handleScaleDown`
3. `reconciler_helpers.go:185` -- `r.newNodeManager(provider, scaler)` in `handleMaxRuntimeRecycling`
4. `cloud_sync.go:44` -- `r.newNodeManager(provider, scalingStrategy)` in `syncNodesWithCloud`
5. `cloud_sync.go:74` -- `r.newNodeManager(provider, scalingStrategy)` in `monitorWarmupNodes`
6. `cloud_sync.go:110` -- `r.newNodeManager(provider, scalingStrategy)` in `monitorCloudWarmupInstances`

**Without hooks (keep as `newNodeManager`):**
7. `pool_maintenance.go:126` -- `r.newNodeManager(provider)` in `replenishStandby`
