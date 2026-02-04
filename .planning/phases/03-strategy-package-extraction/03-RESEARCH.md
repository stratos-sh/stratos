# Phase 3: Strategy Package Extraction - Research

**Researched:** 2026-02-03
**Domain:** Go package restructuring -- decomposing a 910-line monolithic file into a sub-package hierarchy and relocating a package
**Confidence:** HIGH

## Summary

This phase moves `internal/controller/strategy/` to `internal/strategy/` and decomposes the 910-line `kubernetes.go` into a `kubernetes/` sub-package with focused files. The research analyzed every source file in the current strategy package, mapped all dependency imports, catalogued function groupings by concern with line counts, identified the cordon duplication between drain.go and kubernetes.go, and traced every consumer site in the reconciler.

The current strategy package is already well-factored at the file level -- `kubernetes_drain.go` (358 lines), `kubernetes_events.go` (211 lines), `kubernetes_network.go` (131 lines), `kubernetes_capacity.go` (184 lines), and `githubactions.go` (392 lines) are all clean satellite files. The only file exceeding the 300-line cap is `kubernetes.go` (910 lines), which mixes 8 distinct concerns: scaling logic (185 lines), pod assignment management (180 lines), maintenance tasks (157 lines), network readiness/taint processing (102 lines), node readiness/preparation (100 lines), drain invocation (39 lines), and node query helpers (36 lines).

**Primary recommendation:** Execute in two plans: (1) move the package tree and update all import paths, (2) decompose kubernetes.go into the kubernetes/ sub-package files.

## Standard Stack

This phase uses no new libraries. It is pure Go package reorganization using existing dependencies.

### Core
| Library | Purpose | Notes |
|---------|---------|-------|
| Standard Go toolchain | Package moves, import rewrites | `go build ./...` verifies compilation |
| controller-runtime | Existing dependency of strategy (client, handler, predicate, reconcile) | No version change |
| client-go | Existing dependency (events, retry) | No version change |

### Supporting
| Tool | Purpose | When to Use |
|------|---------|-------------|
| `go vet ./...` | Catches import/compilation errors | After every file move |
| `goimports` | Rewrites import paths automatically | After package relocation |

## Architecture Patterns

### Current Package Layout (before)
```
internal/controller/strategy/
  interface.go                  (74 lines)   -- ScalingStrategy interface + types
  factory.go                    (55 lines)   -- NewStrategy factory dispatch
  kubernetes.go                 (910 lines)  -- KubernetesStrategy (ALL logic)
  kubernetes_drain.go           (358 lines)  -- drainHelper + isNodeEmpty
  kubernetes_events.go          (211 lines)  -- pod mapper, predicates, matching
  kubernetes_network.go         (131 lines)  -- networkReadinessChecker
  kubernetes_capacity.go        (184 lines)  -- ScaleCalculator
  githubactions.go              (392 lines)  -- GitHubActionsStrategy
  kubernetes_test.go            (510 lines)
  kubernetes_events_test.go     (643 lines)
  kubernetes_network_test.go    (267 lines)
```

### Target Package Layout (after)
```
internal/strategy/
  interface.go                  -- ScalingStrategy, ScalingDemand, ScaleDownCandidate
  factory.go                    -- NewStrategy (imports kubernetes/ and githubactions/)

  kubernetes/
    kubernetes.go               -- KubernetesStrategy struct + constructor + node helpers
    scaling.go                  -- CheckDemand, OnScaleUp, FindScaleDownCandidates, countStartingNodes
    pod_assignments.go          -- filterAssignedPods, createPodAssignments, estimatePodsPerNode, cleanupPodAssignments
    maintenance.go              -- RunMaintenance, ensureTemplateLabels, clearStaleScaleUpAnnotations, removeScaleUpAnnotation
    readiness.go                -- IsReady, PrepareForRunning, PrepareForStandby, processStartupTaints,
                                   getNodeStartTime, forceRemoveNetworkReadinessTaint, removeNetworkReadinessTaintWhenReady
    drain.go                    -- DrainAndStop (thin wrapper), drainHelper (moved from kubernetes_drain.go)
    network.go                  -- networkReadinessChecker (moved from kubernetes_network.go)
    capacity.go                 -- ScaleCalculator (moved from kubernetes_capacity.go)
    events.go                   -- pod mapper, predicates (moved from kubernetes_events.go)
    scaling_test.go             -- (tests moved from kubernetes_test.go relevant to scaling)
    maintenance_test.go         -- (tests for ensureTemplateLabels, startup taints)
    events_test.go              -- (moved from kubernetes_events_test.go)
    network_test.go             -- (moved from kubernetes_network_test.go)

  githubactions/
    githubactions.go            -- GitHubActionsStrategy (moved from githubactions.go)
```

### Pattern 1: Sub-Package with Re-exported Constructor

The factory at `strategy/factory.go` imports both `strategy/kubernetes` and `strategy/githubactions` sub-packages and dispatches based on config. This is the standard Go pattern for a factory that selects implementations.

```go
package strategy

import (
    "github.com/stratos-sh/stratos/internal/strategy/kubernetes"
    "github.com/stratos-sh/stratos/internal/strategy/githubactions"
)

func NewStrategy(nodePool ...) (ScalingStrategy, error) {
    switch strategyType {
    case ScalingStrategyKubernetes:
        return kubernetes.New(c, recorder, capacityProvider, cniPodSelector), nil
    case ScalingStrategyGitHubActions:
        return githubactions.New(c, config), nil
    }
}
```

### Pattern 2: Interface at Package Root, Implementations in Sub-Packages

`ScalingStrategy` interface and shared types (`ScalingDemand`, `ScaleDownCandidate`) stay at `internal/strategy/interface.go`. Implementations import the parent to satisfy the interface. Consumers import only the parent.

### Pattern 3: Shared Node Query Helpers via Methods on Struct

Both `KubernetesStrategy` and `GitHubActionsStrategy` have duplicate `getNodesForPool` and `getRunningNodes` methods. These are 11-12 line methods that are simple enough to remain duplicated in each implementation (the Go proverb "a little copying is far better than a little dependency"). Extracting them into a shared helper would require a shared struct or package-level functions, adding unnecessary coupling.

### Anti-Patterns to Avoid
- **Circular imports between strategy/ and its sub-packages:** The interface.go at strategy/ root defines types that sub-packages must import. Sub-packages must NOT import back up to strategy/ for other symbols. Keep the root minimal.
- **Moving tests to a separate _test package:** The existing tests use `package strategy` (same package) to test unexported functions. When moving to kubernetes/, tests must use `package kubernetes` to maintain access to unexported helpers. Do NOT change to `package kubernetes_test`.
- **Over-abstracting shared helpers:** Creating a `strategy/internal/` package for 12-line helper functions adds package overhead with zero value.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Import path rewriting | Manual find-and-replace | `goimports` + targeted edits | Consistent, correct import blocks |
| Verifying no circular imports | Manual inspection | `go build ./...` | Go compiler catches cycles |
| Checking file lengths | Manual counting | `wc -l` on each file | Objective 300-line cap verification |

**Key insight:** This is purely mechanical restructuring. Every function exists today and works. The risk is in the moves, not in new logic. Use the compiler as the primary verification tool.

## Common Pitfalls

### Pitfall 1: The nodestate Import Path Constraint

**What goes wrong:** Success criterion 3 says "strategy/ imports lifecycle/ and cloudprovider/ but never imports controller/". Currently, strategy imports `internal/controller/nodestate`. If strategy moves to `internal/strategy/`, it would still import `internal/controller/nodestate` -- which is technically under `controller/`.

**Why it happens:** nodestate was extracted as a leaf package within the controller tree during Phase 1. It has zero upward dependencies (only imports `corev1` from k8s.io). It is a pure data/constants package.

**How to avoid:** The success criterion "never imports controller/" refers to the controller reconciliation package (`internal/controller/*.go`), not to sub-packages like `nodestate/` which are leaf constants/types packages. `nodestate` does not import anything from `controller/` itself. The intent is to prevent circular dependencies, and importing `controller/nodestate` creates no cycle risk.

Interpret the constraint as: **strategy/ must not import the `internal/controller` package itself** (i.e., no import of `"github.com/stratos-sh/stratos/internal/controller"`). Importing `internal/controller/nodestate` is acceptable because nodestate is a leaf package with no upward dependencies. This is the same pattern as lifecycle/ already uses (lifecycle imports nodestate too).

If the user requires strict path interpretation (no `controller/` substring in any import), nodestate would need to move first (e.g., to `internal/nodestate/`). However, this is NOT part of Phase 3 scope per the phase boundary. nodestate is addressed in Phase 4 (Controller Split) which restructures everything under controller/.

**Warning signs:** `go build ./...` will pass regardless. This is a convention question, not a compilation question.

### Pitfall 2: Package Name Collision in Sub-Packages

**What goes wrong:** When `kubernetes.go` moves into `internal/strategy/kubernetes/`, the package name becomes `kubernetes`. Go code calling `kubernetes.New(...)` reads naturally, but imports from the parent factory must use the full path. No collision with `k8s.io/api/core/v1` which uses alias `corev1`.

**Why it happens:** Go uses the last directory segment as the default package name.

**How to avoid:** Name the package `kubernetes` (matching the directory). The factory imports it as `"github.com/stratos-sh/stratos/internal/strategy/kubernetes"`. If needed, alias in factory.go: `kubestrat "github.com/stratos-sh/stratos/internal/strategy/kubernetes"`. Similarly for githubactions: `ghstrat "github.com/stratos-sh/stratos/internal/strategy/githubactions"`.

### Pitfall 3: Test File Access to Unexported Symbols

**What goes wrong:** Tests in `kubernetes_test.go` access unexported functions like `processStartupTaints`, `ensureTemplateLabels`, `getNodeStartTime`. After moving to the kubernetes/ sub-package, these tests must remain in `package kubernetes` (not `package kubernetes_test`) to maintain access.

**Why it happens:** Go restricts unexported symbol access to the same package.

**How to avoid:** Keep test files in `package kubernetes`. Split the 510-line `kubernetes_test.go` into focused test files that match their source files (e.g., `maintenance_test.go` for template label and startup taint tests, `scaling_test.go` for capacity-related tests).

### Pitfall 4: Exported vs Unexported After Sub-Package Split

**What goes wrong:** Functions like `isPodUnschedulable`, `couldSatisfyPod`, `isDaemonSetPod`, `isNodeEmpty` are currently unexported in `package strategy`. When they move to `package kubernetes`, they are still only needed within that package. But `isPodUnschedulable` is also used by `kubernetes_events.go` -- if events.go and the pod scheduling logic end up in different packages, one would need to export it.

**Why it happens:** All files currently share one flat package namespace.

**How to avoid:** Keep ALL kubernetes-related files in the same `package kubernetes`. The events.go (pod mapper, predicates) moves into `kubernetes/` alongside scaling.go because `isPodUnschedulable` and `couldSatisfyPod` are shared between demand checking and event filtering. This keeps everything unexported.

### Pitfall 5: Cordon/Uncordon Duplication

**What goes wrong:** Cordon logic exists in two places: `drainHelper.CordonNode` (kubernetes_drain.go:85) and `PrepareForStandby` / `PrepareForRunning` (kubernetes.go:314, 342). The drainHelper cordon is used during drain. The PrepareFor* methods set cordon + taints for state transitions.

**Why it happens:** Drain is a specific operation (cordon + evict + wait). State preparation is a different operation (cordon/uncordon + taint management).

**How to resolve:** Keep them separate. The drainHelper cordon is internal to the drain workflow. PrepareForStandby/PrepareForRunning are strategy interface methods with different responsibilities (they manage taints in addition to cordon). No deduplication needed -- the overlap is 4 lines (`node.Spec.Unschedulable = true/false` + patch) which is trivial duplication. Extracting a shared helper would couple drain's internal flow to the readiness concern.

### Pitfall 6: networkReadinessChecker Instantiation

**What goes wrong:** `newNetworkReadinessChecker` is called multiple times per reconciliation: once in `IsReady()` (line 299), once in `removeNetworkReadinessTaintWhenReady()` (line 784). The checker is stateless (just holds client + selector), so creating it multiple times is not a correctness issue but is wasteful.

**Why it happens:** The checker was added incrementally to specific methods rather than as a field on KubernetesStrategy.

**How to resolve:** Per CONTEXT.md decision: "networkReadinessChecker should be instantiated once and reused." Add it as a field on KubernetesStrategy, initialized in the constructor. The `IsReady` and `processStartupTaints` code paths then use `s.networkChecker` instead of calling `newNetworkReadinessChecker` each time.

## Code Examples

### File Decomposition: kubernetes.go -> kubernetes/ Sub-Package

Based on the line-count analysis of the 910-line file:

| Target File | Functions | Estimated Lines | Under 300? |
|-------------|-----------|-----------------|------------|
| `kubernetes.go` | struct, constructor, DrainAndStop, getNodesForPool, getRunningNodes, getNodeClass, getUnschedulablePods | ~140 | Yes |
| `scaling.go` | CheckDemand, OnScaleUp, FindScaleDownCandidates, countStartingNodes | ~215 | Yes |
| `pod_assignments.go` | filterAssignedPods, createPodAssignments, estimatePodsPerNode, cleanupPodAssignments | ~180 | Yes |
| `maintenance.go` | RunMaintenance, ensureTemplateLabels, clearStaleScaleUpAnnotations, removeScaleUpAnnotation | ~157 | Yes |
| `readiness.go` | IsReady, PrepareForRunning, PrepareForStandby, processStartupTaints, getNodeStartTime, forceRemoveNetworkReadinessTaint, removeNetworkReadinessTaintWhenReady | ~202 | Yes |
| `drain.go` | DrainAndStop wrapper + drainHelper (from kubernetes_drain.go) | ~398 (drain.go consolidation) | See note |
| `network.go` | networkReadinessChecker (from kubernetes_network.go) | ~131 | Yes |
| `capacity.go` | ScaleCalculator (from kubernetes_capacity.go) | ~184 | Yes |
| `events.go` | pod mapper, predicates, matching (from kubernetes_events.go) | ~211 | Yes |

**Note on drain.go:** The existing `kubernetes_drain.go` is 358 lines. With DrainAndStop (39 lines) moved in, it would be ~397 lines -- exceeding the 300-line cap. Options:
1. Keep `DrainAndStop` in `kubernetes.go` as a thin 39-line wrapper that calls into drain.go -- this is what the CONTEXT.md decision says: "DrainAndStop stays as a thin wrapper in the strategy: calls drainHelper for drain, then stops the cloud instance."
2. Split drain.go itself: `drain.go` (DrainAndStop + drainHelper core ~180 lines) and `drain_helpers.go` (pod filtering, eviction, waiting ~180 lines).

**Recommendation:** Option 1. Keep DrainAndStop in `kubernetes.go` (the main file). The drainHelper type and all its methods stay in `drain.go` at 358 lines. While this exceeds 300 lines, the CONTEXT.md scope says "kubernetes.go is replaced by a kubernetes/ sub-package with separate files -- no single file exceeds 300 lines." The drain.go file already exists as a separate file and is not being created from the kubernetes.go decomposition. The 300-line cap applies to the new files created from splitting kubernetes.go. However, if strict interpretation is required, drain.go can be split into `drain.go` + `drain_eviction.go`.

### Interface Decision: Monolithic vs Composed

Current ScalingStrategy interface has 5 methods:
```go
type ScalingStrategy interface {
    CheckDemand(...)     (ScalingDemand, error)
    OnScaleUp(...)       error
    FindScaleDownCandidates(...) ([]ScaleDownCandidate, error)
    DrainAndStop(...)    error
    RunMaintenance(...)  error
}
```

Plus 3 methods from lifecycle.NodeHooks (implemented by strategies but not in the ScalingStrategy interface):
```go
// In lifecycle/manager.go
type NodeHooks interface {
    PrepareForRunning(...)  error
    PrepareForStandby(...)  error
    IsReady(...)            (bool, error)
}
```

**Reconciler consumption pattern analysis:**
- `reconcileNodePool()` calls: `CheckDemand`, `OnScaleUp`, `FindScaleDownCandidates`, `DrainAndStop`, `RunMaintenance` -- uses ALL 5 ScalingStrategy methods
- `startStandbyNodes()` casts strategy to `lifecycle.NodeHooks` for the lifecycle manager
- There is no case where only a subset of ScalingStrategy methods is needed

**Recommendation: Keep ScalingStrategy monolithic (5 methods).** The reconciler always needs the full interface. Splitting into role interfaces (e.g., `DemandChecker`, `ScaleDownProvider`) would add type assertions or multiple parameters with zero benefit. The NodeHooks interface already exists as a separate concern in lifecycle/ -- this natural split is sufficient.

### Import Path Updates

Files that import `internal/controller/strategy` and need updating:

| File | Import Used For |
|------|----------------|
| `internal/controller/reconciler.go` | `strategy.ScalingStrategy`, `strategy.ScaleDownCandidate` |
| `internal/controller/provider_cache.go` | `strategy.ScalingStrategy`, `strategy.KubernetesStrategy`, `strategy.GitHubActionsStrategy`, `strategy.NewStrategy` |
| `internal/controller/setup.go` | `strategy.KubernetesPodEventHandler`, `strategy.KubernetesUnschedulablePodPredicate` |
| `internal/controller/pod_assignment_test.go` | `strategy.NewScaleCalculator` |

After the move:
- `strategy.ScalingStrategy` -> `strategy.ScalingStrategy` (same, just new import path)
- `strategy.KubernetesStrategy` -> `kubernetes.Strategy` (or keep as `kubernetes.KubernetesStrategy`)
- `strategy.NewKubernetesStrategy` -> `kubernetes.New`
- `strategy.KubernetesPodEventHandler` -> `kubernetes.PodEventHandler`
- `strategy.KubernetesUnschedulablePodPredicate` -> `kubernetes.UnschedulablePodPredicate`
- `strategy.NewScaleCalculator` -> `kubernetes.NewScaleCalculator`

The compile-time assertions in `provider_cache.go` need updating:
```go
var (
    _ lifecycle.NodeHooks = (*kubernetes.Strategy)(nil)
    _ lifecycle.NodeHooks = (*githubactions.Strategy)(nil)
)
```

### Execution Order

**Plan 1: Package relocation + import rewrites**
1. Create `internal/strategy/` directory
2. Move `interface.go` and `factory.go` to `internal/strategy/`
3. Create `internal/strategy/kubernetes/` directory
4. Move all `kubernetes*.go` files into `internal/strategy/kubernetes/`
5. Create `internal/strategy/githubactions/` directory
6. Move `githubactions.go` into `internal/strategy/githubactions/`
7. Update package declarations in all moved files
8. Update factory.go to import sub-packages
9. Update all 4 consumer files with new import paths
10. Rename exported symbols (drop "Kubernetes" prefix in kubernetes/)
11. `go build ./...` to verify

**Plan 2: kubernetes.go decomposition**
1. Split kubernetes.go into: kubernetes.go, scaling.go, pod_assignments.go, maintenance.go, readiness.go
2. Instantiate networkReadinessChecker once on struct
3. Split kubernetes_test.go into focused test files matching new source files
4. Verify all files under 300 lines with `wc -l`
5. `go build ./...` and `go test ./internal/strategy/...` to verify

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Flat package with kubernetes_ prefixed files | Sub-package per implementation | Go community standard | Cleaner imports, natural namespace |
| 910-line monolith | Files organized by concern | This phase | Maintainability, readability |

## Open Questions

### 1. Strict Interpretation of "never imports controller/"

**What we know:** The success criterion says strategy/ "never imports controller/". Currently, strategy imports `internal/controller/nodestate` which IS under the `controller/` path. nodestate is a pure leaf package (imports only k8s.io types).

**What is unclear:** Whether the constraint means "never imports the `controller` package" (the reconciler code) or "never has `controller/` in any import path."

**Recommendation:** Interpret as "never imports the controller reconciler package." This matches the intent (prevent circular dependencies) and is consistent with how lifecycle/ works (lifecycle also imports `controller/nodestate`). If strict path interpretation is required, nodestate would need to move first, which is Phase 4 scope. Flag this to the user if the planner encounters ambiguity.

### 2. drain.go Line Count

**What we know:** The existing kubernetes_drain.go is 358 lines. The 300-line cap from success criteria applies to "no single file exceeds 300 lines."

**What is unclear:** Whether the cap applies only to files created from decomposing kubernetes.go, or to ALL files in the kubernetes/ sub-package.

**Recommendation:** The success criterion states "kubernetes.go is replaced by a kubernetes/ sub-package with separate files -- no single file exceeds 300 lines." If applied to all files, drain.go needs splitting. Split at the natural boundary: `drain.go` (DrainAndStop wrapper + drainHelper struct + CordonNode + UncordonNode + DrainNode -- ~177 lines) and `drain_eviction.go` (getPodsOnNode + filterPodsToEvict + hasLocalStorage + evictPod + waitForPodsDeletion + waitForPodDeletion + isNodeEmpty + isDaemonSetPod -- ~181 lines). This is a clean split.

## Sources

### Primary (HIGH confidence)
- Direct codebase analysis of all 11 source files in `internal/controller/strategy/`
- Line-by-line function mapping with line count analysis
- Import dependency graph traced via `go list -json`
- Consumer analysis of 4 files importing the strategy package
- Full read of lifecycle/manager.go NodeHooks interface

### Secondary (HIGH confidence)
- Go language specification for package naming, import resolution, and circular dependency detection
- Prior phase research (Phase 1, Phase 2) establishing patterns for this codebase

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - No new dependencies, pure Go restructuring
- Architecture: HIGH - Based on direct analysis of every file, every function, every import
- Pitfalls: HIGH - Based on concrete code analysis, not hypotheticals
- File decomposition: HIGH - Line counts verified via analysis of actual function boundaries

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (stable - this is codebase analysis, not library version research)
