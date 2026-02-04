# Phase 1: Mechanical Cleanup - Research

**Researched:** 2026-02-02
**Domain:** Go codebase hygiene -- naming, context propagation, error wrapping, unexport, dead code
**Confidence:** HIGH

## Summary

This phase is a set of mechanical, non-behavioral changes to clean up the codebase before structural refactoring phases. The research investigates the exact current state of each area targeted by the phase requirements (MECH-01 through MECH-04, PKG-04).

Key findings:
- The naming collision problem is real but moderate -- 9 basenames are shared across directories, with the `reconcile.go` vs `reconciler.go` split being the most confusing. The two files have a clear logical split (reconciler.go = Reconcile entry point + deletion + type definition; reconcile.go = main reconciliation loop + scale-up), so a **merge** into a single file is the cleanest approach.
- The context.Background() problem is much smaller than originally estimated (2 production calls, not 39). The main.go calls are legitimate startup usage. Only `providers.go` has the defect.
- Error wrapping is already correct -- all `fmt.Errorf` calls that wrap errors use `%w`. The `%v` uses are for formatting non-error values. However, error message prefix conventions are inconsistent and custom error types lack `errors.Is`/`As` compatibility.
- Several exported types/functions are only used within their own package and should be unexported.
- The `_extracted/` directory and `ExponentialBackoff` are confirmed dead code.

**Primary recommendation:** Tackle in this order: (1) delete dead code, (2) rename/merge files, (3) unexport symbols, (4) fix context propagation, (5) standardize error patterns. This order minimizes merge conflicts between tasks.

## Standard Stack

Not applicable -- this phase uses only Go standard library and existing project tooling. No new libraries needed.

## Architecture Patterns

### Current Controller File Layout (internal/controller/)

```
reconciler.go          # NodePoolReconciler type, Reconcile(), handleDeletion(), cleanupNodePoolResources(), recordEvent()
reconcile.go           # reconcileNodePool() main loop, startStandbyNodes()
providers.go           # ensureCloudProvider(), newNodeManager(), getOrCreateStrategy(), getCloudProvider(), InjectCloudProvider()
setup.go               # SetupWithManager(), NodeEventHandler(), NodeClassEventHandler(), mapper types
queries.go             # getNodesForPool(), getStandbyNodes(), getRunningNodes(), getWarmupNodes(), countCloudInstances(), countNodesByState()
nodeclass.go           # getAWSNodeClass(), getNodeClass(), updateNodeClassLifecycle(), cleanupNodeClassReference(), countNodePools..., conditions
validate.go            # validateNodePool(), checkNodeClassReady()
status.go              # setReadyCondition(), setDegradedCondition()
maintenance.go         # checkMaxNodeRuntime(), replenishStandby()
cloud_sync.go          # syncNodesWithCloud(), monitorWarmupNodes(), monitorCloudWarmupInstances()
config.go              # ClusterConfig type, Validate(), DetectKubernetesVersion(), ParseKubernetesVersion()
config_test.go         # Tests for config.go
nodeclass_lifecycle_test.go  # Tests for nodeclass.go
pod_assignment_test.go # Tests for scale calculator (via strategy package)
```

### Recommended File Naming Resolution

| Current | Problem | Action | New Name |
|---------|---------|--------|----------|
| `reconciler.go` + `reconcile.go` | Ambiguous split | **Merge** into single file | `reconciler.go` |
| `api/v1alpha1/nodeclass.go` + `internal/controller/nodeclass.go` | Same basename, different packages | Rename controller's copy | `nodeclass_lifecycle.go` |
| `providers.go` | Vague name, holds provider cache + factory + strategy cache | Rename to describe its role | `provider_cache.go` |
| `queries.go` | Vague name | Rename to describe contents | `node_queries.go` |
| `validate.go` | Vague name | Rename to describe contents | `nodepool_validation.go` |
| `status.go` | Vague name | Rename to describe contents | `nodepool_status.go` |
| `maintenance.go` | Vague name | Rename to describe contents | `pool_maintenance.go` |
| `cloud_sync.go` | Fine but could be clearer | Keep or rename | `cloud_sync.go` (keep) |
| `setup.go` | Fine | Keep | `setup.go` (keep) |
| `config.go` (controller) + `config.go` (config pkg) | Same basename, different packages | Rename controller's copy | `cluster_config.go` |

**Reasoning for merge vs rename on reconcile.go/reconciler.go:**
- `reconciler.go` (247 lines) contains: type definition, `Reconcile()` entry point, `handleDeletion()`, `cleanupNodePoolResources()`, `recordEvent()`
- `reconcile.go` (318 lines) contains: `reconcileNodePool()` main loop, `startStandbyNodes()`
- Combined: ~565 lines -- within acceptable range for a single file in Go
- The split is confusing because both files contain the core reconciliation logic with no clear boundary
- A single `reconciler.go` file that contains the type, entry point, and main loop is the standard pattern in controller-runtime projects

### Pattern: Disambiguating Duplicate Basenames

Two `nodeclass.go` files exist:
- `api/v1alpha1/nodeclass.go` -- defines the `NodeClass` interface (the right name for it)
- `internal/controller/nodeclass.go` -- controller-side lifecycle management for NodeClass references

Rename the controller's file to `nodeclass_lifecycle.go` since it deals with finalizers, conditions, reference counting -- lifecycle operations. This also matches the existing test file `nodeclass_lifecycle_test.go`.

### Anti-Patterns to Avoid
- **Renaming test files that don't match source:** When renaming `nodeclass.go` to `nodeclass_lifecycle.go`, the test file `nodeclass_lifecycle_test.go` already has the right name. No test rename needed.
- **Over-splitting files:** Don't split files below ~50 lines. Files like `status.go` (81 lines) and `validate.go` (97 lines) are fine as single files.
- **Changing package structure:** This phase explicitly does NOT restructure packages. Files stay in their current packages.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Static analysis for unused exports | Custom grep scripts | `staticcheck` or `golangci-lint` with `unused` linter | Compiler-level accuracy for detecting truly unused code |
| Error wrapping validation | Manual code review | `go vet` + `errcheck` linter | Catches `%v` vs `%w` mistakes systematically |

**Key insight:** `go vet` and the project's configured `golangci-lint` should be run after changes to verify no regressions. The linter config (errcheck, gosimple, govet, staticcheck, unused) will catch most issues this phase targets.

## Common Pitfalls

### Pitfall 1: Exporting Symbols Used Only in Tests

**What goes wrong:** A function appears "used outside the package" but only in test files in another directory (e.g., integration tests import the package).
**Why it happens:** Go test files can import internal packages. A function used only in `tests/integration/` but defined in `internal/controller/` MUST remain exported because tests are in a different package.
**How to avoid:** Before unexporting any symbol, check BOTH production code AND test code outside the package. Use `grep` across the entire repo.
**Warning signs:** `InjectCloudProvider` is a prime example -- it's used extensively in integration tests and MUST stay exported even though no production code calls it.

### Pitfall 2: context.Background() in main.go is Legitimate

**What goes wrong:** Zealously replacing ALL context.Background() calls including those at startup.
**Why it happens:** The phase requirement says "zero context.Background() in non-test production code" which could be misread.
**How to avoid:** `main.go` startup code runs before the manager starts -- there IS no reconciliation context. `context.Background()` is correct there. Only fix context.Background() calls inside the reconciliation path.
**Actual scope:** Only 2 calls need fixing, both in `internal/controller/providers.go` lines 67 and 89.

### Pitfall 3: %v for Non-Error Values is Correct

**What goes wrong:** Changing `%v` to `%w` for non-error values like maps or strings, causing compilation errors.
**Why it happens:** The phase requirement says "every fmt.Errorf uses %w" which could be misread as "no %v anywhere."
**How to avoid:** `%w` is ONLY for wrapping `error` values. `%v` is correct for formatting non-error values in error messages. The 3 `%v` uses in `resolver.go` format `map[string]string` tags and are correct.
**Actual scope:** Zero changes needed for `%v` -> `%w`. The codebase already correctly uses `%w` for all error wrapping.

### Pitfall 4: Breaking Integration Tests by Unexporting

**What goes wrong:** Unexporting a function that integration tests depend on.
**Why it happens:** Integration tests live in `tests/integration/` which is a different package from `internal/controller/`.
**How to avoid:** For every unexport candidate, run: `grep -r "SymbolName" tests/ cmd/`
**Key exports that MUST stay exported:**
- `NodePoolReconciler` (used in main.go and tests)
- `ClusterConfig` (used in main.go)
- `DetectKubernetesVersion` (used in main.go)
- `InjectCloudProvider` (used in integration tests)
- `SetupWithManager` (used in main.go and tests)

### Pitfall 5: Custom Error Types and errors.As Compatibility

**What goes wrong:** Custom error types in `cloudprovider/types.go` are checked with direct type assertions (`_, ok := err.(*RateLimitError)`) instead of `errors.As()`, which breaks when errors are wrapped with `%w`.
**Why it happens:** The error types were written before establishing consistent wrapping patterns.
**How to avoid:** Audit all error type checks and convert type assertions to `errors.As()`. This ensures error checking works correctly through wrapping chains.
**Instances found:**
- `internal/cloudprovider/aws/ratelimit.go:181` uses `_, ok := err.(*cloudprovider.RateLimitError)` instead of `errors.As`

## Code Examples

### Context Propagation Fix (providers.go)

The `ensureCloudProvider` function needs a `ctx` parameter:

```go
// BEFORE (broken):
func (r *NodePoolReconciler) ensureCloudProvider(nodePool *stratosv1alpha1.NodePool) error {
    // ...
    nodeClass, fetchErr := r.getNodeClass(context.Background(), ref)  // BAD
    provider, err = aws.NewAWSProvider(context.Background(), ...)      // BAD
}

// AFTER (fixed):
func (r *NodePoolReconciler) ensureCloudProvider(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
    // ...
    nodeClass, fetchErr := r.getNodeClass(ctx, ref)                    // GOOD
    provider, err = aws.NewAWSProvider(ctx, ...)                       // GOOD
}
```

The caller in `reconciler.go` already has `ctx` and just needs to pass it:
```go
if err := r.ensureCloudProvider(ctx, nodePool); err != nil {
```

### Unexport Pattern

```go
// BEFORE (unnecessarily exported):
func NodeEventHandler(c client.Client) handler.EventHandler { ... }
type NodeToNodePoolMapper struct { ... }

// AFTER (package-private):
func nodeEventHandler(c client.Client) handler.EventHandler { ... }
type nodeToNodePoolMapper struct { ... }
```

### Error Message Prefix Convention

The codebase uses three error prefix patterns. Standardize on the most common:

```go
// Pattern 1 (RECOMMENDED - most common in codebase): "failed to <verb>: %w"
return fmt.Errorf("failed to get nodes: %w", err)
return fmt.Errorf("failed to launch instance: %w", err)

// Pattern 2 (AWS resolver - acceptable domain-specific): "<APICall> failed: %w"
return fmt.Errorf("DescribeSubnets failed: %w", err)

// Pattern 3 (github package - acceptable domain-specific): "github: <action>: %w"
return nil, fmt.Errorf("github: listing runners: %w", err)
```

**Recommendation:** Keep domain-specific prefixes in `aws/resolver.go` (API call names are useful context) and `github/client.go`. Standardize the controller and lifecycle packages on Pattern 1: `"failed to <verb>: %w"`.

### errors.As Pattern for Custom Error Types

```go
// BEFORE (breaks through wrapping chains):
if _, ok := err.(*cloudprovider.RateLimitError); !ok {
    return err
}

// AFTER (works through wrapping chains):
var rateLimitErr *cloudprovider.RateLimitError
if !errors.As(err, &rateLimitErr) {
    return err
}
```

## Detailed Audit Results

### Naming Collisions - Full Inventory

Files with duplicate basenames across directories (9 total):

| Basename | Location 1 | Location 2 | Collision Risk |
|----------|-----------|-----------|----------------|
| `nodeclass.go` | `api/v1alpha1/nodeclass.go` (NodeClass interface) | `internal/controller/nodeclass.go` (lifecycle mgmt) | HIGH - confusing |
| `reconcile.go` / `reconciler.go` | Same directory, ambiguous split | N/A | HIGH - confusing |
| `config.go` | `internal/config/config.go` (env config) | `internal/controller/config.go` (cluster config) | MEDIUM |
| `config_test.go` | `internal/config/config_test.go` | `internal/controller/config_test.go` | MEDIUM |
| `provider.go` | `internal/cloudprovider/aws/provider.go` | `internal/cloudprovider/fake/provider.go` | LOW - expected pattern |
| `resolver.go` | `internal/cloudprovider/aws/resolver.go` | `internal/cloudprovider/fake/resolver.go` | LOW - expected pattern |
| `interface.go` | `internal/cloudprovider/interface.go` | `internal/controller/strategy/interface.go` | LOW - expected pattern |
| `warmup.go` | `internal/cloudprovider/aws/warmup.go` | `internal/controller/lifecycle/warmup.go` | LOW - different domains |
| `helpers_test.go` | `tests/integration/helpers_test.go` | `tests/e2e/helpers_test.go` | LOW - expected pattern |

**Action needed:** HIGH and MEDIUM risk items. LOW risk items are normal Go patterns (same name in sibling packages).

### context.Background() - Complete Inventory

**Non-test production code:**

| File | Line | Call | Verdict |
|------|------|------|---------|
| `cmd/stratos/main.go` | 118 | `DetectKubernetesVersion(context.Background())` | LEGITIMATE - startup, no reconciliation ctx |
| `cmd/stratos/main.go` | 184 | `awsconfig.LoadDefaultConfig(context.Background())` | LEGITIMATE - startup, no reconciliation ctx |
| `internal/controller/providers.go` | 67 | `r.getNodeClass(context.Background(), ref)` | **FIX** - called from Reconcile path |
| `internal/controller/providers.go` | 89 | `aws.NewAWSProvider(context.Background(), ...)` | **FIX** - called from Reconcile path |

**Test code:** 50+ instances across test files -- all legitimate (tests create their own contexts).

**Total fixes needed: 2** (not 39 as originally estimated)

### Error Wrapping - Complete Inventory

**fmt.Errorf with %v wrapping errors:** 0 instances (clean)

**fmt.Errorf with %v formatting non-errors:** 3 instances (correct usage)
- `resolver.go:108` - formatting `selector.Tags` (map)
- `resolver.go:152` - formatting `selector.Tags` (map) + `selector.Name`
- `resolver.go:201` - formatting `selector.Tags` (map) + `selector.Name` + `selector.Owner`

**Error message prefix inconsistencies:** Minor -- three patterns coexist but are domain-appropriate. See Code Examples section above.

**Custom error type issues:**
- 5 custom error types in `cloudprovider/types.go`: `InstanceNotFoundError`, `InvalidStateError`, `RateLimitError`, `QuotaExceededError`, `InsufficientCapacityError`
- All use pointer receivers for `Error()` method -- correct for `errors.As`
- 1 instance of type assertion instead of `errors.As` in `ratelimit.go:181`

### Unexport Candidates - Complete Inventory

**controller package (internal/controller/):**

| Symbol | Type | Used Outside Package? | Action |
|--------|------|-----------------------|--------|
| `NodeEventHandler` | func | No | Unexport |
| `NodeClassEventHandler` | func | No | Unexport |
| `NodeToNodePoolMapper` | struct | No | Unexport |
| `NodeClassToNodePoolMapper` | struct | No | Unexport |
| `NodePoolReconciler` | struct | Yes (main.go, tests) | **Keep exported** |
| `ClusterConfig` | struct | Yes (main.go) | **Keep exported** |
| `DetectKubernetesVersion` | func | Yes (main.go) | **Keep exported** |
| `ParseKubernetesVersion` | func | No (only test) | Unexport |
| `InjectCloudProvider` | method | Yes (integration tests) | **Keep exported** |
| `SetupWithManager` | method | Yes (main.go, tests) | **Keep exported** |

**strategy package (internal/controller/strategy/):**

| Symbol | Type | Used Outside Package? | Action |
|--------|------|-----------------------|--------|
| `IsPodUnschedulable` | func | No (only test in same pkg) | Unexport |
| `IsNodeEmpty` | func | No | Unexport |
| `DefaultDrainConfig` | func | No | Unexport |
| `NewDrainHelper` | func | No | Unexport |
| `DrainHelper` | struct | No | Unexport |
| `DrainConfig` | struct | No | Unexport |
| `KubernetesPodEventHandler` | func | Yes (controller/setup.go) | **Keep exported** |
| `KubernetesUnschedulablePodPredicate` | func | Yes (controller/setup.go) | **Keep exported** |
| `NewKubernetesStrategy` | func | Yes (factory.go, tests) | **Keep exported** |
| `NewGitHubActionsStrategy` | func | Yes (factory.go) | **Keep exported** |
| `NewScaleCalculator` | func | Yes (tests in controller pkg) | **Keep exported** |
| `NetworkReadinessChecker` | struct | No | Unexport |
| `NewNetworkReadinessChecker` | func | No | Unexport |
| `ScaleCalculator` | struct | No references outside pkg | Check tests -- pod_assignment_test.go uses it via `strategy.NewScaleCalculator` | **Keep exported** |

**nodestate package (internal/controller/nodestate/):**

| Symbol | Type | Used Outside Package? | Action |
|--------|------|-----------------------|--------|
| `ValidTransitions` | var | No (only referenced in comment) | Unexport |
| All constants | const | Yes (widely used) | **Keep exported** |
| All functions | func | Yes (widely used) | **Keep exported** |
| `InvalidTransitionError` | struct | Yes (lifecycle pkg) | **Keep exported** |

**cloudprovider/aws package:**

| Symbol | Type | Used Outside Package? | Action |
|--------|------|-----------------------|--------|
| `ExponentialBackoff` | func | No (never called anywhere) | **Delete** (dead code) |
| `GetWarmupScript` | func | No (used only within aws pkg) | Unexport |
| `DefaultAMISelector` | func | No (used only within aws pkg) | Unexport |

### Dead Code - Complete Inventory

| Item | Location | Type | Action |
|------|----------|------|--------|
| `_extracted/` directory | `/home/roeeh/projects/presto/_extracted/` | Directory | Delete entirely |
| `ExponentialBackoff` | `internal/cloudprovider/aws/ratelimit.go:168` | Unused function | Delete |
| `tests/e2e/spot_test.go` | Already deleted in git | Orphaned test | Already handled (git status shows D) |
| `tests/integration/spot_replacement_test.go` | Already deleted in git | Orphaned test | Already handled (git status shows D) |

Note: Many files show as `D` (deleted) in git status -- those are already handled by the user's prior work. The `_extracted/` directory is untracked (`??`) and needs explicit deletion.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `%v` for error wrapping | `%w` for error wrapping | Go 1.13 (2019) | Enables `errors.Is`/`errors.As` through wrapping chains |
| Type assertions for error checking | `errors.As` for error checking | Go 1.13 (2019) | Works through wrapping chains |
| `context.TODO()` as placeholder | `context.Background()` only at entry points | Go convention | Signals intent -- TODO means "will fix later" |

## Open Questions

1. **ParseKubernetesVersion export status**
   - What we know: Only called from `DetectKubernetesVersion` (same package) and from test file `config_test.go` (same package)
   - What's unclear: Whether future phases will need this from outside the controller package
   - Recommendation: Unexport now. If needed later, re-export is trivial.

2. **conditionMatches helper (nodeclass.go)**
   - What we know: Unexported standalone function, used only in nodeclass.go
   - What's unclear: Whether it should become a method on NodePoolReconciler for consistency
   - Recommendation: Leave as standalone -- it doesn't use the reconciler and is a pure utility function. This is fine.

3. **supportedNodeClassKinds var (validate.go)**
   - What we know: Unexported package-level var, used only in validateNodePool
   - What's unclear: Whether future cloud providers will need to register kinds dynamically
   - Recommendation: Leave as-is. It's already unexported and the current static map is appropriate.

## Sources

### Primary (HIGH confidence)
- Direct codebase audit -- all findings verified by reading actual source files
- `/kubernetes-sigs/controller-runtime` Context7 docs -- reconciler context propagation patterns
- Go language specification -- `%w` verb behavior (Go 1.13+)

### Secondary (MEDIUM confidence)
- Go standard library `errors` package documentation -- `errors.Is`/`errors.As` behavior with wrapped errors

## Metadata

**Confidence breakdown:**
- Naming audit: HIGH -- directly read all files, verified all basenames
- Context propagation: HIGH -- grep found all instances, verified each manually
- Error wrapping: HIGH -- grep found all instances, verified each `%v` usage
- Unexport audit: HIGH -- grep verified cross-package usage for each symbol
- Dead code: HIGH -- verified `ExponentialBackoff` has zero callers

**Research date:** 2026-02-02
**Valid until:** Indefinite (codebase-specific findings, no external dependencies)
