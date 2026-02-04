# Phase 7: Type Relocation - Research

**Researched:** 2026-02-03
**Domain:** Go package restructuring (rename + type relocation)
**Confidence:** HIGH

## Summary

Phase 7 moves all files from `internal/strategy/kubernetes/` to a new `internal/scaling/` package and relocates the `ScalingDemand` and `ScaleDownCandidate` types from `internal/strategy/interface.go` into that new package. This is an additive change: the old `internal/strategy/` package remains until Phase 9. The build must pass after this phase.

The codebase was thoroughly investigated. The `internal/strategy/kubernetes/` package contains 12 source files and 6 test files (~5,050 lines total). The two types being relocated (`ScalingDemand`, `ScaleDownCandidate`) are defined in `internal/strategy/interface.go` and consumed in 6 files across 3 packages. After relocation, the old `internal/strategy/interface.go` will need type aliases or the consumer imports must be updated. Since Phase 8 rewires the controller and Phase 9 deletes strategy/, the cleanest approach for Phase 7 is to use **type aliases** in the old `internal/strategy/interface.go` pointing to the new `internal/scaling/` definitions. This keeps Phase 7 purely additive.

**Primary recommendation:** Create `internal/scaling/` by copying `internal/strategy/kubernetes/` files, update package declarations to `package scaling`, move `ScalingDemand` and `ScaleDownCandidate` into `internal/scaling/types.go`, and add type aliases in `internal/strategy/interface.go` so existing consumers compile without changes.

## Standard Stack

Not applicable -- this phase uses only Go's built-in package system and the `go build` toolchain. No new libraries are needed.

## Architecture Patterns

### Source Package: `internal/strategy/kubernetes/`

12 source files, 6 test files:

| File | Purpose | Lines | Imports strategy pkg? |
|------|---------|-------|-----------------------|
| `doc.go` | Package documentation | 34 | No |
| `kubernetes.go` | Strategy struct + New() constructor + DrainAndStop + node query helpers | 156 | Yes (ScaleDownCandidate) |
| `scaling.go` | CheckDemand, OnScaleUp, FindScaleDownCandidates, evaluateScaleDownNode | 287 | Yes (ScalingDemand, ScaleDownCandidate) |
| `capacity.go` | ScaleCalculator for pod-to-node resource calculation | 183 | No |
| `drain.go` | drainHelper (cordon, uncordon, drain node) | 174 | No |
| `drain_eviction.go` | Pod eviction, pod filtering, isNodeEmpty, isDaemonSetPod | 211 | No |
| `events.go` | PodEventHandler, UnschedulablePodPredicate, couldSatisfyPod, isPodUnschedulable | 211 | No |
| `maintenance.go` | RunMaintenance, ensureTemplateLabels, clearStaleScaleUpAnnotations | 189 | No |
| `network.go` | networkReadinessChecker (CNI/EKS network detection) | 131 | No |
| `pod_assignments.go` | filterAssignedPods, createPodAssignments, cleanupPodAssignments | 212 | No |
| `readiness.go` | IsReady, PrepareForRunning, PrepareForStandby, processStartupTaints | 226 | No |
| `events_test.go` | Tests for pod event handling | 643 | No |
| `kubernetes_test.go` | Tests for NetworkReadinessTaint config | 49 | No |
| `maintenance_test.go` | Tests for maintenance operations | 143 | No |
| `network_test.go` | Tests for network readiness checker | 267 | No |
| `readiness_test.go` | Tests for readiness checks | 206 | No |
| `startup_taints_test.go` | Tests for startup taint removal | 210 | No |

### Types to Relocate

Defined in `internal/strategy/interface.go` (lines 30-42):

```go
// ScalingDemand describes how many nodes a strategy wants started.
type ScalingDemand struct {
    NodesNeeded int
    Metadata    interface{}
}

// ScaleDownCandidate wraps a node that the strategy considers eligible for scale-down.
type ScaleDownCandidate struct {
    Node corev1.Node
}
```

Both are simple value types with no methods. Their only dependency is `corev1.Node`.

### Consumers of `ScalingDemand`

| File | Usage |
|------|-------|
| `internal/strategy/interface.go` | Type definition + interface method signatures |
| `internal/strategy/kubernetes/scaling.go` | `strategy.ScalingDemand{}` construction, return type |
| `internal/strategy/githubactions/githubactions.go` | `strategy.ScalingDemand{}` construction, return type |
| `internal/controller/nodepool/reconciler_helpers.go` | (indirect via ScalingStrategy interface) |

### Consumers of `ScaleDownCandidate`

| File | Usage |
|------|-------|
| `internal/strategy/interface.go` | Type definition + interface method signatures |
| `internal/strategy/kubernetes/kubernetes.go` | `strategy.ScaleDownCandidate` parameter in DrainAndStop |
| `internal/strategy/kubernetes/scaling.go` | `strategy.ScaleDownCandidate{}` construction, return type |
| `internal/strategy/githubactions/githubactions.go` | `strategy.ScaleDownCandidate{}` construction, parameters |
| `internal/controller/nodepool/reconciler_helpers.go` | `strategy.ScaleDownCandidate` parameter type, `strategy.ScaleDownCandidate{Node: ...}` construction |

### Consumers of `ScalingStrategy` Interface

| File | Import | Usage |
|------|--------|-------|
| `internal/controller/nodepool/reconciler.go` | `internal/strategy` | Field type: `map[string]strategy.ScalingStrategy` |
| `internal/controller/nodepool/reconciler_helpers.go` | `internal/strategy` | Parameter types on helper methods |
| `internal/controller/nodepool/provider_cache.go` | `internal/strategy` + `strategy/kubernetes` + `strategy/githubactions` | Factory, cache, compile-time assertions |
| `internal/controller/nodepool/setup.go` | `internal/strategy/kubernetes` | `kubernetes.PodEventHandler`, `kubernetes.UnschedulablePodPredicate` |

### Import Graph (Dependency Chain)

```
internal/strategy/
  imports: api/v1alpha1, internal/cloudprovider

internal/strategy/kubernetes/
  imports: api/v1alpha1, internal/cloudprovider, internal/controller/nodepool/nodestate,
           internal/metrics, internal/strategy

internal/strategy/githubactions/
  imports: api/v1alpha1, internal/cloudprovider, internal/controller/nodepool/nodestate,
           internal/github, internal/strategy

internal/controller/nodepool/
  imports: ... internal/strategy, internal/strategy/kubernetes, internal/strategy/githubactions

internal/controller/nodepool/nodestate/
  imports: fmt (ONLY - no stratos imports, no circular dependency risk)

internal/controller/nodepool/lifecycle/
  imports: api/v1alpha1, internal/cloudprovider, internal/controller/nodepool/nodestate, internal/metrics
```

**Critical finding:** No circular dependency risk. The new `internal/scaling/` package will import `nodestate`, `cloudprovider`, `api/v1alpha1`, and `metrics` -- none of which import anything from `strategy/` or will import from `scaling/`.

### Recommended Relocation Strategy

**Approach: Type aliases in the old package (keeps Phase 7 purely additive)**

1. Create `internal/scaling/` directory
2. Copy all 18 files from `internal/strategy/kubernetes/` to `internal/scaling/`
3. Update `package kubernetes` -> `package scaling` in all copied files
4. Create `internal/scaling/types.go` with `ScalingDemand` and `ScaleDownCandidate` type definitions
5. Remove `"github.com/stratos-sh/stratos/internal/strategy"` import from copied files, replacing `strategy.ScalingDemand` -> `ScalingDemand` and `strategy.ScaleDownCandidate` -> `ScaleDownCandidate` (now local to the package)
6. Add type aliases in `internal/strategy/interface.go`:
   ```go
   import "github.com/stratos-sh/stratos/internal/scaling"

   type ScalingDemand = scaling.ScalingDemand
   type ScaleDownCandidate = scaling.ScaleDownCandidate
   ```
7. Verify `go build ./...` passes

**Why type aliases, not import updates?** Type aliases keep this phase strictly additive -- the old `internal/strategy/` package still works identically, so `githubactions/` and `controller/nodepool/` don't need changes. Phase 8 rewires the controller and Phase 9 deletes strategy/, so temporary aliases are the right tool here.

### Anti-Patterns to Avoid

- **Do NOT update consumer imports in this phase.** That's Phase 8's job (controller rewiring) and Phase 9 (deletion). Phase 7 should compile without touching anything outside `internal/scaling/` and `internal/strategy/interface.go`.
- **Do NOT delete `internal/strategy/kubernetes/` in this phase.** It remains as-is until Phase 9. Phase 7 is additive.
- **Do NOT rename the struct from `Strategy` to `Scaler` yet.** That's a Phase 8 concern. Phase 7 preserves existing names.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Type aliasing for gradual migration | Custom wrapper types | Go type aliases (`type X = pkg.X`) | Type aliases are identical types at compile time; wrapper types break interface satisfaction |
| Package copying | Manual file-by-file copy | `cp -r` + `sed` for package declaration | Consistent, no missed files |

**Key insight:** Go type aliases (`type X = Y`) create true type identity -- a `scaling.ScalingDemand` is exactly the same type as `strategy.ScalingDemand` when aliased. This means existing code that passes/returns `strategy.ScalingDemand` works seamlessly with functions expecting `scaling.ScalingDemand`. A defined type (`type X struct { ... }`) would NOT work because it creates a distinct type.

## Common Pitfalls

### Pitfall 1: Forgetting to Update Package Declarations

**What goes wrong:** Files are copied but `package kubernetes` is not changed to `package scaling`, causing compile errors.
**Why it happens:** Bulk copy without find-and-replace.
**How to avoid:** After copying, verify every `.go` file in `internal/scaling/` has `package scaling` as its package declaration.
**Warning signs:** `go build` errors about "found packages kubernetes and scaling in directory".

### Pitfall 2: Self-Referencing Imports After Move

**What goes wrong:** Files in `internal/scaling/` still import `internal/strategy/kubernetes` or `internal/strategy`.
**Why it happens:** The copied files originally imported `internal/strategy` for `ScalingDemand`/`ScaleDownCandidate`.
**How to avoid:** Remove the `internal/strategy` import from files that now define those types locally. The `strategy.ScalingDemand` references become just `ScalingDemand`.
**Warning signs:** Import cycle errors or redundant imports.

### Pitfall 3: Using Defined Types Instead of Type Aliases

**What goes wrong:** Using `type ScalingDemand struct { ... }` in the alias file instead of `type ScalingDemand = scaling.ScalingDemand` in `internal/strategy/interface.go`.
**Why it happens:** Confusion between Go type definitions and type aliases.
**How to avoid:** Always use `=` for aliases. `type X = Y` is an alias (same type). `type X Y` is a new type (different type).
**Warning signs:** Compile errors about type mismatches between `strategy.ScalingDemand` and `scaling.ScalingDemand`.

### Pitfall 4: Breaking the `ScalingStrategy` Interface

**What goes wrong:** The `ScalingStrategy` interface in `internal/strategy/interface.go` references `ScalingDemand` and `ScaleDownCandidate` in its method signatures. If these become aliases, the interface methods use the aliased types. Implementors in `githubactions/` still use `strategy.ScalingDemand`, which resolves to the alias. This should work, but must be verified.
**Why it happens:** Subtlety of Go's type system.
**How to avoid:** After adding aliases, verify `go build ./...` passes -- the compiler will catch any mismatches.
**Warning signs:** `go build` errors in `githubactions/` or `controller/nodepool/`.

### Pitfall 5: Import Cycle from strategy -> scaling -> strategy

**What goes wrong:** If `internal/scaling/` imports `internal/strategy/` for the `ScalingStrategy` interface, and `internal/strategy/` imports `internal/scaling/` for the type aliases, a cycle is created.
**Why it happens:** Trying to keep the interface in the old package while types move.
**How to avoid:** The `internal/scaling/` files must NOT import `internal/strategy/`. The type aliases go only in `internal/strategy/` -> `internal/scaling/` direction. The `ScalingStrategy` interface stays in `internal/strategy/` and its method signatures use the now-aliased types.
**Warning signs:** `import cycle` compiler errors.

### Pitfall 6: Test File Package Declarations

**What goes wrong:** Test files are copied but still have `package kubernetes` or `package kubernetes_test`.
**Why it happens:** Test files may use `_test` package suffix which also needs updating.
**How to avoid:** Check all `_test.go` files use `package scaling` (all current tests use the internal package name `package kubernetes`, not `package kubernetes_test`, so they should become `package scaling`).
**Warning signs:** Package name mismatch errors during `go test`.

## Code Examples

### Type Alias Pattern (in `internal/strategy/interface.go`)

```go
import "github.com/stratos-sh/stratos/internal/scaling"

// ScalingDemand is an alias for scaling.ScalingDemand during migration.
type ScalingDemand = scaling.ScalingDemand

// ScaleDownCandidate is an alias for scaling.ScaleDownCandidate during migration.
type ScaleDownCandidate = scaling.ScaleDownCandidate
```

### New Types File (in `internal/scaling/types.go`)

```go
package scaling

import corev1 "k8s.io/api/core/v1"

// ScalingDemand describes how many nodes should be started for scaling.
type ScalingDemand struct {
    NodesNeeded int
    Metadata    interface{}
}

// ScaleDownCandidate wraps a node that is eligible for scale-down.
type ScaleDownCandidate struct {
    Node corev1.Node
}
```

### Package Declaration Update (all copied files)

Before: `package kubernetes`
After: `package scaling`

### Import Update in Copied Files (files that used `strategy.ScalingDemand`)

Before (in `scaling.go`):
```go
import "github.com/stratos-sh/stratos/internal/strategy"
// ...
return strategy.ScalingDemand{NodesNeeded: nodesNeeded, ...}
```

After (in `scaling.go`):
```go
// No strategy import needed -- types are now local
// ...
return ScalingDemand{NodesNeeded: nodesNeeded, ...}
```

## State of the Art

Not applicable -- standard Go package restructuring. No library versions or evolving patterns involved.

## Open Questions

### 1. Should the Strategy struct be renamed to Scaler in this phase?

- **What we know:** Phase 8 rewires the controller to use a direct `*scaling.Scaler` field. The roadmap says Phase 7 is just type relocation.
- **What's unclear:** Whether renaming in Phase 7 causes unnecessary churn.
- **Recommendation:** Do NOT rename in Phase 7. Keep `Strategy` as the struct name. Phase 8 can rename to `Scaler` when it rewires the controller. Phase 7 success criteria say nothing about renaming -- only that files move and types are importable.

### 2. Do test files need any changes beyond package declaration?

- **What we know:** All 6 test files use `package kubernetes` (internal tests). They import `api/v1alpha1`, `cloudprovider`, `nodestate`, and `metrics` but none import `internal/strategy`. They should work with just a package declaration change.
- **What's unclear:** Whether any test helper setup references package-specific paths.
- **Recommendation:** Change package declarations and run `go test ./internal/scaling/...` to verify. If any test imports `internal/strategy/kubernetes` (self-import), that path needs updating.

## Sources

### Primary (HIGH confidence)

- Direct codebase investigation: read all 18 files in `internal/strategy/kubernetes/`, all 3 files in `internal/strategy/`, and all consumer files in `internal/controller/nodepool/`
- `go list` import graph analysis: verified no circular dependency risk
- `go build ./...`: confirmed current build compiles clean

### Secondary (MEDIUM confidence)

- Go language spec: type aliases (`type X = Y`) provide true type identity -- verified from Go specification knowledge

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no external libraries involved, pure Go package operations
- Architecture: HIGH - complete file inventory, full import graph traced, no ambiguity
- Pitfalls: HIGH - each pitfall is derived from direct codebase analysis and Go semantics

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (stable -- codebase is the source of truth)

## Appendix: Complete File Inventory

### Files to Copy (internal/strategy/kubernetes/ -> internal/scaling/)

Source files (12):
1. `doc.go` -- package documentation
2. `kubernetes.go` -- Strategy struct, New(), DrainAndStop, node query helpers
3. `scaling.go` -- CheckDemand, OnScaleUp, FindScaleDownCandidates
4. `capacity.go` -- ScaleCalculator
5. `drain.go` -- drainHelper
6. `drain_eviction.go` -- eviction logic
7. `events.go` -- PodEventHandler, UnschedulablePodPredicate
8. `maintenance.go` -- RunMaintenance
9. `network.go` -- networkReadinessChecker
10. `pod_assignments.go` -- pod assignment management
11. `readiness.go` -- IsReady, PrepareForRunning/Standby, startup taints

Test files (6):
1. `events_test.go`
2. `kubernetes_test.go`
3. `maintenance_test.go`
4. `network_test.go`
5. `readiness_test.go`
6. `startup_taints_test.go`

### Files to Modify (in old package)

1. `internal/strategy/interface.go` -- Replace ScalingDemand/ScaleDownCandidate definitions with type aliases pointing to `internal/scaling/`

### Files NOT Touched (consumers compile unchanged)

1. `internal/strategy/doc.go` -- no changes needed
2. `internal/strategy/githubactions/` -- all files compile unchanged (type aliases)
3. `internal/controller/nodepool/reconciler.go` -- compiles unchanged
4. `internal/controller/nodepool/reconciler_helpers.go` -- compiles unchanged
5. `internal/controller/nodepool/provider_cache.go` -- compiles unchanged
6. `internal/controller/nodepool/setup.go` -- compiles unchanged
7. `tests/integration/` -- no direct strategy imports
