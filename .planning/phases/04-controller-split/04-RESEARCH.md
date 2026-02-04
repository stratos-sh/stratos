# Phase 4: Controller Split - Research

**Researched:** 2026-02-03
**Domain:** Go package restructuring, controller-runtime multi-controller patterns
**Confidence:** HIGH

## Summary

This phase splits the monolithic `internal/controller/` package into per-CRD sub-packages (`nodepool/`, `nodeclass/`) with a central `setup.go` aggregator. The research focused on three areas: (1) understanding the exact file-to-package mapping with dependency analysis to prevent circular imports, (2) Karpenter's reference architecture for per-controller packages, and (3) the specific mechanics of splitting a Go receiver type across packages.

The current controller package has 14 source files (non-test) and 4 test files, all bound to a single `NodePoolReconciler` struct. The split requires creating two new reconciler types, moving methods off `NodePoolReconciler` to new homes, and re-wiring integration tests. The existing `lifecycle/` and `nodestate/` sub-packages are already leaves with clean import graphs -- they import nothing from `controller/` -- which simplifies the split considerably.

**Primary recommendation:** Execute in three steps: (1) move `cluster_config.go` to `internal/config/`, (2) create `nodepool/` package with the NodePool reconciler and its files, (3) create `nodeclass/` package with the NodeClass reconciler. Use the aggregator `setup.go` pattern at `controller/` root. Move `lifecycle/` and `nodestate/` under `nodepool/` since their sole consumers are NodePool reconciliation operations.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| controller-runtime | v0.23.1 | Controller framework | Already in use, provides ctrl.Manager, Builder, Reconciler |
| k8s.io/client-go | (transitive) | Kubernetes client | Required for events.EventRecorder, retry, discovery |
| k8s.io/apimachinery | (transitive) | API types | metav1.Condition, runtime.Scheme |

### Supporting

No new libraries needed. This is a pure restructuring phase using existing dependencies.

## Architecture Patterns

### Recommended Project Structure

```
internal/controller/
    setup.go                    # Aggregator: Setup(mgr) calls nodepool.Setup() + nodeclass.Setup()
    nodepool/
        reconciler.go           # NodePool reconciler type + Reconcile() + reconcileNodePool()
        setup.go                # Setup(mgr, opts) registers controller with manager
        cloud_sync.go           # syncNodesWithCloud, monitorWarmupNodes, monitorCloudWarmupInstances
        node_queries.go         # getNodesForPool, getStandbyNodes, etc.
        nodepool_status.go      # setReadyCondition, setDegradedCondition
        nodepool_validation.go  # validateNodePool, checkNodeClassReady, supportedNodeClassKinds
        pool_maintenance.go     # checkMaxNodeRuntime, replenishStandby
        provider_cache.go       # ensureCloudProvider, getOrCreateStrategy, newNodeManager, etc.
        pod_assignment_test.go  # Tests for pod assignment logic
        lifecycle/              # Moved from controller/lifecycle/ (sole consumer is nodepool)
            manager.go
            node_launch.go
            node_startstop.go
            node_sync.go
            warmup_adoption.go
            warmup_handlers.go
            warmup_monitor.go
            warmup_test.go
        nodestate/              # Moved from controller/nodestate/ (shared constants, used by lifecycle too)
            nodestate.go
            nodestate_test.go
    nodeclass/
        reconciler.go           # NodeClass reconciler type + Reconcile() method
        setup.go                # Setup(mgr) registers with manager
        nodeclass_lifecycle.go  # updateNodeClassLifecycle, cleanupNodeClassReference, conditions
        nodeclass_lifecycle_test.go
```

### Pattern 1: Per-Package Setup Function

**What:** Each controller sub-package exports a `Setup()` function that registers its reconciler with the manager.

**When to use:** When splitting controllers into separate packages with independent reconciler structs.

**Example:**
```go
// internal/controller/nodepool/setup.go
package nodepool

import (
    ctrl "sigs.k8s.io/controller-runtime"
)

// SetupOptions holds the configuration needed to set up the NodePool controller.
type SetupOptions struct {
    ClusterName      string
    CloudProvider    string
    ClusterConfig    *config.ClusterConfig
    CapacityProvider cloudprovider.InstanceCapacityProvider
    CNIPodSelector   map[string]string
    RateLimitConfig  *aws.RateLimitConfig
}

// Setup registers the NodePool controller with the manager.
func Setup(mgr ctrl.Manager, opts SetupOptions) error {
    return (&Reconciler{
        Client:           mgr.GetClient(),
        Scheme:           mgr.GetScheme(),
        ClusterName:      opts.ClusterName,
        CloudProvider:    opts.CloudProvider,
        ClusterConfig:    opts.ClusterConfig,
        CapacityProvider: opts.CapacityProvider,
        CNIPodSelector:   opts.CNIPodSelector,
        RateLimitConfig:  opts.RateLimitConfig,
    }).SetupWithManager(mgr)
}
```

```go
// internal/controller/setup.go (aggregator)
package controller

import (
    ctrl "sigs.k8s.io/controller-runtime"
    "github.com/stratos-sh/stratos/internal/controller/nodepool"
    "github.com/stratos-sh/stratos/internal/controller/nodeclass"
)

// Setup registers all controllers with the manager.
func Setup(mgr ctrl.Manager, opts SetupOptions) error {
    if err := nodepool.Setup(mgr, nodepool.SetupOptions{...}); err != nil {
        return err
    }
    if err := nodeclass.Setup(mgr, nodeclass.SetupOptions{...}); err != nil {
        return err
    }
    return nil
}
```

**Source:** Karpenter AWS provider uses a similar `NewControllers()` factory function in `pkg/controllers/controllers.go` that instantiates controllers from separate packages.

### Pattern 2: Independent Reconciler Structs

**What:** Each package defines its own Reconciler struct with only the fields it needs. No shared base struct.

**When to use:** When controllers have different dependencies and don't share reconciliation state.

**Key insight from codebase analysis:**

The current `NodePoolReconciler` has fields used only by NodePool logic:
- `cloudProviders` map + mutex (provider cache)
- `strategies` map + mutex (strategy cache)
- `CapacityProvider`, `CNIPodSelector`, `RateLimitConfig`

The NodeClass lifecycle logic (nodeclass_lifecycle.go) only needs `client.Client` and `Scheme` -- it uses the embedded `r.Client` for Get/List/Update. The NodeClass reconciler struct will be much simpler.

### Pattern 3: Aggregator setup.go as Single Import Point

**What:** `controller/setup.go` is the only file at the package root. It imports `nodepool` and `nodeclass` sub-packages and provides a single `Setup(mgr)` function.

**Why:** Keeps `main.go` thin (one import, one function call) while providing a clear inventory of all controllers. Matches the decision in CONTEXT.md.

### Anti-Patterns to Avoid

- **Shared base reconciler struct:** Using struct embedding to share fields between NodePool and NodeClass reconcilers creates tight coupling and makes the dependency graph unclear. Use independent structs.
- **Cross-package method calls:** `nodeclass.Reconciler` must not call `nodepool.Reconciler` methods or vice versa. If both need the same helper, move it to a shared utility (e.g., `nodestate/`, or a new `internal/controller/shared/` package if needed).
- **Putting leaf packages at controller root:** `lifecycle/` and `nodestate/` should move under `nodepool/` since the NodePool reconciler is their sole consumer. Leaving them at root creates a misleading flat structure.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Event handler mapping | Custom node-to-pool mappers per package | Reuse existing handler patterns from setup.go | The mapper logic is simple and can be defined inline in each package's Setup() |
| Shared K8s helpers | Duplicating getNodesForPool in both packages | Keep in nodepool/ (only nodepool uses node queries) | Analysis shows all node_queries.go methods are NodePool-specific |
| Condition helpers | New condition utility package | conditionMatches is small enough to inline in nodeclass/ | Only 6 lines, used only by nodeclass lifecycle |

**Key insight:** The existing codebase has very clean separation. The nodeclass lifecycle operations only need `client.Client` -- they don't need cloud provider, strategy, or node query helpers. This makes the split natural.

## Common Pitfalls

### Pitfall 1: Circular Import Between nodepool/ and nodeclass/

**What goes wrong:** NodePool reconciler currently calls `updateNodeClassLifecycle()` and `checkNodeClassReady()`, which are methods on `NodePoolReconciler`. If these stay as methods on the NodePool reconciler but need types from `nodeclass/`, a circular import can occur.

**Why it happens:** Both controllers operate on the same NodeClass CRD objects, creating apparent coupling.

**How to avoid:** Two approaches (recommended: approach A):
- **A. NodePool reads NodeClass status conditions directly** -- the `checkNodeClassReady()` function (13 lines) uses the `NodeClass` interface from `api/v1alpha1`. It does NOT need anything from the `nodeclass/` controller package. Keep it in `nodepool/`.
- **B. NodeClass lifecycle methods become standalone functions** -- `updateNodeClassLifecycle()` and `cleanupNodeClassReference()` currently live in nodeclass_lifecycle.go as methods on `NodePoolReconciler`. These get extracted to the `nodeclass/` package as the NodeClass reconciler's own responsibilities. The NodePool reconciler just calls `updateNodeClassLifecycle` from within its reconcile loop -- but since it's a method on `NodePoolReconciler`, it stays in nodepool/ where it's called from. The underlying helpers (counting NodePools, getting conditions) only need `client.Client`.

**Resolution (from CONTEXT.md):** NodeClass-related steps are extracted into the nodeclass controller. The NodePool reconciler reads the resolved result rather than doing resolution itself. This means `checkNodeClassReady` stays in nodepool/ (it only reads status conditions), while `updateNodeClassLifecycle` and `cleanupNodeClassReference` move to nodeclass/ where they become the NodeClass reconciler's responsibility.

**Warning signs:** `import cycle not allowed` compiler errors after moving files.

### Pitfall 2: Breaking Integration Tests

**What goes wrong:** Integration tests in `tests/integration/suite_test.go` directly reference `controller.NodePoolReconciler` and `controller.ClusterConfig`. After the split, these types move to `nodepool/` and `config/` packages.

**Why it happens:** The test suite constructs the reconciler directly rather than using a Setup() function.

**How to avoid:** Update `suite_test.go` to import `nodepool` package instead of `controller`. The reconciler type changes from `controller.NodePoolReconciler` to `nodepool.Reconciler` (or use the aggregator Setup function). The `ClusterConfig` import changes from `controller.ClusterConfig` to `config.ClusterConfig`.

**Warning signs:** Compilation errors in `tests/integration/` after the split.

### Pitfall 3: Lost kubebuilder RBAC Markers

**What goes wrong:** kubebuilder RBAC markers (`// +kubebuilder:rbac:...`) are currently on `NodePoolReconciler.Reconcile()` in `reconciler.go`. Moving to a sub-package may require re-running `make manifests` and verifying the generated RBAC is correct.

**Why it happens:** kubebuilder scans for markers in specific packages. Moving the reconciler to a sub-package changes where it looks.

**How to avoid:** After moving, run `make manifests` and diff the generated RBAC ClusterRole. The markers should stay with the Reconcile method in its new package. Verify no permissions are lost.

**Warning signs:** Missing RBAC permissions causing 403 errors at runtime.

### Pitfall 4: nodeclass_lifecycle.go Tests Reference NodePoolReconciler

**What goes wrong:** `nodeclass_lifecycle_test.go` (704 lines) constructs `NodePoolReconciler{Client: fakeClient}` to test `getAWSNodeClass`, `countNodePoolsReferencingNodeClass`, `updateNodeClassLifecycle`, etc. After the split, these functions move to a `nodeclass.Reconciler` with a different struct.

**Why it happens:** Tests are tightly coupled to the current reconciler type.

**How to avoid:** When moving to `nodeclass/`, update the test to construct `nodeclass.Reconciler` instead. The test setup is simple (just needs a fake client), so the change is mechanical.

### Pitfall 5: provider_cache.go Has the Heaviest Import Set

**What goes wrong:** `provider_cache.go` imports 7 internal packages (cloudprovider, aws, fake, lifecycle, strategy, githubactions, kubernetes). Moving it to `nodepool/` is safe, but it must be moved as a whole unit.

**Why it happens:** This file is the factory/wiring layer for the NodePool controller -- it creates cloud providers and strategies.

**How to avoid:** Move `provider_cache.go` to `nodepool/` as-is. All its imports are downstream (leaf packages), so no circular import risk. The `InjectCloudProvider` test helper must also move -- integration tests need to update their import.

## Code Examples

### Moving cluster_config.go to internal/config/

```go
// internal/config/cluster_config.go
package config

// ClusterConfig holds the cluster configuration needed for userData generation.
type ClusterConfig struct {
    Name                 string
    APIServerEndpoint    string
    CertificateAuthority string
    CIDR                 string
    KubernetesVersion    string
}

// Validate, DetectKubernetesVersion, etc. move here unchanged.
// Only the package declaration changes.
```

Update imports in:
- `cmd/stratos/main.go`: `controller.ClusterConfig` -> `config.ClusterConfig`
- `internal/controller/nodepool/reconciler.go`: same import change
- `tests/integration/suite_test.go`: same import change

Note: `internal/config/config.go` already exists with the `Config` struct. `ClusterConfig` naturally belongs there.

### NodeClass Reconciler Structure

```go
// internal/controller/nodeclass/reconciler.go
package nodeclass

import (
    "context"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "k8s.io/apimachinery/pkg/runtime"
)

// Reconciler reconciles NodeClass-related lifecycle (finalizers, status, conditions).
// It manages the in-use tracking and readiness conditions for AWSNodeClass objects
// based on which NodePools reference them.
type Reconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Handles: updateNodeClassLifecycle, cleanupNodeClassReference
    // for the NodeClass objects referenced by NodePools.
    // ...
}
```

### Aggregator Setup Pattern

```go
// internal/controller/setup.go
package controller

import (
    ctrl "sigs.k8s.io/controller-runtime"
    "github.com/stratos-sh/stratos/internal/controller/nodeclass"
    "github.com/stratos-sh/stratos/internal/controller/nodepool"
)

// SetupOptions holds all options needed to register controllers.
type SetupOptions struct {
    // NodePool options
    ClusterName      string
    CloudProvider    string
    // ... other fields
}

// Setup registers all Stratos controllers with the manager.
func Setup(mgr ctrl.Manager, opts SetupOptions) error {
    if err := nodepool.Setup(mgr, nodepool.SetupOptions{
        ClusterName:   opts.ClusterName,
        CloudProvider: opts.CloudProvider,
        // ...
    }); err != nil {
        return err
    }
    // nodeclass setup only needs Client and Scheme (from mgr)
    if err := nodeclass.Setup(mgr); err != nil {
        return err
    }
    return nil
}
```

## Detailed Import Graph Analysis

### Current State: Who Imports What

```
controller/ imports:
  api/v1alpha1, cloudprovider, cloudprovider/aws, cloudprovider/fake,
  controller/lifecycle, controller/nodestate, strategy, strategy/githubactions,
  strategy/kubernetes, metrics

controller/lifecycle/ imports:
  api/v1alpha1, cloudprovider, controller/nodestate, metrics

controller/nodestate/ imports:
  (no internal imports - pure leaf)
```

### After Split: Import Graph

```
controller/setup.go imports:
  controller/nodepool, controller/nodeclass

controller/nodepool/ imports:
  api/v1alpha1, cloudprovider, cloudprovider/aws, cloudprovider/fake,
  config, controller/nodepool/lifecycle, controller/nodepool/nodestate,
  strategy, strategy/githubactions, strategy/kubernetes, metrics

controller/nodeclass/ imports:
  api/v1alpha1
  (only needs client.Client from controller-runtime, Scheme)

controller/nodepool/lifecycle/ imports:
  api/v1alpha1, cloudprovider, controller/nodepool/nodestate, metrics

controller/nodepool/nodestate/ imports:
  (no internal imports - pure leaf)
```

**Critical observation:** No circular imports are possible in this layout. `nodepool/` and `nodeclass/` never import each other. The aggregator `setup.go` imports both but neither imports it. `lifecycle/` and `nodestate/` are leaves under `nodepool/`.

### File Movement Plan

| Current Location | New Location | Reason |
|------------------|-------------|--------|
| `controller/reconciler.go` | `controller/nodepool/reconciler.go` | Core NodePool reconciler |
| `controller/setup.go` | `controller/nodepool/setup.go` + `controller/setup.go` (aggregator) | Split: per-package setup + aggregator |
| `controller/cloud_sync.go` | `controller/nodepool/cloud_sync.go` | Used only by NodePool reconciler |
| `controller/node_queries.go` | `controller/nodepool/node_queries.go` | Used only by NodePool reconciler |
| `controller/nodepool_status.go` | `controller/nodepool/nodepool_status.go` | Used only by NodePool reconciler |
| `controller/nodepool_validation.go` | `controller/nodepool/nodepool_validation.go` | Used only by NodePool reconciler |
| `controller/pool_maintenance.go` | `controller/nodepool/pool_maintenance.go` | Used only by NodePool reconciler |
| `controller/provider_cache.go` | `controller/nodepool/provider_cache.go` | Used only by NodePool reconciler |
| `controller/pod_assignment_test.go` | `controller/nodepool/pod_assignment_test.go` | Tests for NodePool-specific logic |
| `controller/nodeclass_lifecycle.go` | `controller/nodeclass/reconciler.go` (merged) | Becomes NodeClass reconciler |
| `controller/nodeclass_lifecycle_test.go` | `controller/nodeclass/reconciler_test.go` | Tests follow source |
| `controller/cluster_config.go` | `internal/config/cluster_config.go` | Not controller-specific |
| `controller/cluster_config_test.go` | `internal/config/cluster_config_test.go` | Tests follow source |
| `controller/lifecycle/` | `controller/nodepool/lifecycle/` | Sole consumer is nodepool |
| `controller/nodestate/` | `controller/nodepool/nodestate/` | Primary consumer is nodepool, also used by lifecycle |

## Discretion Area Analysis

### lifecycle/ and nodestate/ Placement

**Recommendation: Move under nodepool/**

Evidence:
- `lifecycle/` is imported by: `pool_maintenance.go` and `provider_cache.go` (both NodePool-specific)
- `nodestate/` is imported by: `cloud_sync.go`, `node_queries.go`, `pool_maintenance.go`, `reconciler.go`, `setup.go` (all NodePool), and `lifecycle/*.go`
- Neither `lifecycle/` nor `nodestate/` is imported by `nodeclass_lifecycle.go`
- The nodeclass package only needs `api/v1alpha1` types, not `nodestate` constants

Moving them under `nodepool/` accurately reflects the import graph. The only alternative consumer would be `nodeclass/` for labels/tags, but the nodeclass reconciler (updating conditions, finalizers) does not use node labels or state constants at all.

**Risk:** If a future phase needs `nodestate` from outside `nodepool/`, it would need to be extracted again. However, the CONTEXT.md says to evaluate based on actual import graph, and today the import graph is clear.

### node_queries.go and provider_cache.go Placement

**Recommendation: Both stay in nodepool/**

Evidence:
- `node_queries.go` methods (`getNodesForPool`, `getStandbyNodes`, etc.) are all `(r *NodePoolReconciler)` methods used exclusively in the NodePool reconciliation loop
- `provider_cache.go` contains the cloud provider factory, strategy factory, and lifecycle manager factory -- all NodePool-specific
- Neither has any consumers outside the NodePool controller

### provider_cache: Shared or Per-Controller

**Recommendation: Per-controller (stays in nodepool/)**

Evidence:
- The `cloudProviders` map caches per-pool providers -- only the NodePool reconciler needs this
- The `strategies` map caches per-pool strategies -- only the NodePool reconciler needs this
- The `newNodeManager()` helper creates lifecycle managers -- only the NodePool reconciler uses these
- The existing `AWSNodeClassReconciler` in `cloudprovider/aws/nodeclass_controller.go` has its own direct `Resolver` dependency, not shared with the NodePool's cloud provider cache

### Dependency Injection for Setup()

**Recommendation: Options struct pattern**

Evidence:
- NodePool Setup needs 6+ parameters (ClusterName, CloudProvider, ClusterConfig, CapacityProvider, CNIPodSelector, RateLimitConfig)
- More than 3 parameters warrants a struct
- NodeClass Setup needs 0 extra parameters (just `ctrl.Manager` which provides Client and Scheme)

```go
// nodepool.SetupOptions for 6+ params
type SetupOptions struct { ... }
func Setup(mgr ctrl.Manager, opts SetupOptions) error { ... }

// nodeclass.Setup takes only mgr (no options needed)
func Setup(mgr ctrl.Manager) error { ... }
```

### Whether aggregator or main.go Creates Shared Resources

**Recommendation: main.go creates shared resources, passes to aggregator**

Evidence:
- `main.go` currently creates `ClusterConfig`, AWS SDK clients, rate limiter
- These are infrastructure-level resources, not controller-level
- The aggregator just wires controllers -- it should not create AWS SDK clients
- Keep the existing pattern where `main.go` owns the factory/infrastructure

### Reconciler Decomposition Style

**Recommendation: Thin orchestrator + delegate files**

Evidence:
- The current `reconciler.go` (531 lines) is already structured as an orchestrator calling delegate methods
- Files like `cloud_sync.go`, `node_queries.go`, `pool_maintenance.go` are already focused delegates
- Moving them to `nodepool/` preserves this pattern naturally
- No need to merge delegate files into the reconciler

### How NodePool Checks NodeClass Readiness

**Recommendation: Read status conditions directly (current approach works)**

Evidence:
- `checkNodeClassReady()` (13 lines in `nodepool_validation.go`) fetches the NodeClass via the `NodeClass` interface from `api/v1alpha1` and checks conditions
- It does NOT import anything from the nodeclass controller package
- It only needs `client.Client` (which the NodePool reconciler already has)
- No cross-package dependency is created

## Relationship to Existing AWSNodeClassReconciler

**Critical finding:** An `AWSNodeClassReconciler` already exists at `internal/cloudprovider/aws/nodeclass_controller.go`. It handles AWS-specific resolution (AMI, subnets, security groups, instance profiles). This is a **separate** reconciler from the NodeClass lifecycle management being extracted.

The new `nodeclass/` package should contain the lifecycle management (finalizers, in-use tracking, conditions) that was previously embedded in the NodePool reconciler. The AWS-specific resolution stays in `cloudprovider/aws/`. These are two separate controllers watching the same CRD:
1. `aws.AWSNodeClassReconciler` -- resolves cloud selectors to concrete IDs
2. `nodeclass.Reconciler` -- manages lifecycle (in-use tracking, finalizer management)

Both are already registered separately in `main.go`. The new `nodeclass/` package consolidates the lifecycle management that was incorrectly housed in the NodePool reconciler.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Single flat controller package | Per-CRD sub-packages | Karpenter v1.0+ | Better separation, independent reconcilers |
| Shared reconciler struct | Independent reconciler types | Karpenter convention | No accidental coupling between CRD controllers |
| Direct main.go registration | Aggregator setup.go | kubebuilder convention | Single inventory of all controllers |

## Open Questions

1. **NodeClass reconciler trigger mechanism**
   - What we know: Currently `updateNodeClassLifecycle` is called from NodePool's Reconcile(). After the split, NodeClass has its own reconciler. It needs its own trigger.
   - What's unclear: Should the NodeClass reconciler watch NodePool events (to detect reference changes), or should the NodePool reconciler still call into nodeclass logic?
   - Recommendation: The NodeClass reconciler should watch NodePool events via a mapper (same pattern as current `nodeClassEventHandler` but reversed -- watch NodePools, map to NodeClasses). This decouples the two controllers. Alternatively, keep `updateNodeClassLifecycle` in the NodePool reconciler since it already has the context of which NodePool references which NodeClass. The CONTEXT.md says "NodeClass-related steps extracted into the nodeclass controller", so a dedicated watch is more aligned.

2. **Integration test InjectCloudProvider**
   - What we know: Tests call `reconciler.InjectCloudProvider()` to inject the fake provider
   - What's unclear: After the split, the reconciler type changes to `nodepool.Reconciler`
   - Recommendation: Export `InjectCloudProvider` on the new `nodepool.Reconciler` type. Integration tests update their import from `controller` to `nodepool`.

## Sources

### Primary (HIGH confidence)

- **Codebase analysis** -- Direct reading of all 14 source files and 4 test files in `internal/controller/`, plus lifecycle/, nodestate/, integration tests, and main.go
- **Import graph analysis** -- Complete dependency tree via `go list -deps` and manual grep of all import statements
- **Build verification** -- `go build ./...` compiles cleanly on current branch
- `/kubernetes-sigs/controller-runtime` Context7 docs -- Controller registration patterns, Builder API, SetupWithManager convention
- `/aws/karpenter-provider-aws` Context7 docs -- Per-controller package pattern, factory function, conditional registration

### Secondary (MEDIUM confidence)

- **Karpenter AWS `pkg/controllers/controllers.go`** (via WebFetch of GitHub raw content) -- Factory function pattern, dependency injection through parameters, conditional controller registration
- **Karpenter core `pkg/controllers/controllers.go`** (via WebFetch) -- NewControllers() centralizing controller instantiation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, pure restructuring
- Architecture: HIGH -- direct codebase analysis, verified import graph, confirmed no circular dependency risk
- Pitfalls: HIGH -- identified from actual code analysis (not hypothetical), verified by reading every file that would be affected
- Discretion recommendations: HIGH -- based on measured import graph, line counts, and consumer analysis

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (stable -- this is internal restructuring, not dependent on external library changes)
