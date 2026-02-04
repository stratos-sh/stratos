# Feature Landscape: Naming Cleanup & Dead Code Removal (v1.1.1)

**Domain:** Go naming conventions in Kubernetes operators
**Researched:** 2026-02-04
**Confidence:** HIGH (type renames validated against Karpenter, cluster-autoscaler, controller-runtime, and kubectl/drain patterns)

This research validates the proposed renames in the v1.1.1 milestone against naming
conventions used by mature Kubernetes Go projects: Karpenter (sigs.k8s.io/karpenter),
cluster-autoscaler (k8s.io/autoscaler), controller-runtime (sigs.k8s.io/controller-runtime),
and kubectl's drain package (k8s.io/kubectl/pkg/drain).

---

## Reference Project Naming Patterns

Before evaluating each rename, here are the patterns established by the reference
projects. These form the basis for all recommendations below.

### File Naming Conventions (all projects)

| Pattern | Examples | Used By |
|---------|----------|---------|
| `subject.go` | `provisioner.go`, `terminator.go`, `actuator.go` | Karpenter, cluster-autoscaler |
| `subject_role.go` | `async_initializer.go`, `node_launch.go`, `warmup_monitor.go` | Karpenter, Stratos |
| `types.go` | `types.go` in disruption/, cloudprovider/ | Karpenter, cluster-autoscaler |
| `controller.go` | Main reconciler file per package | Karpenter (every controller pkg) |
| `helpers.go` | Internal utilities | Karpenter disruption/ |
| `metrics.go` | Prometheus instrumentation | Karpenter, Stratos |

**Key finding:** Go files use `snake_case`. Both `subject.go` (single concept) and
`subject_role.go` (compound concept) are standard. There is NO convention for renaming
`types.go` to a domain-prefixed variant. `types.go` is universally understood.

### Type Naming Conventions

| Pattern | Examples | Used By |
|---------|----------|---------|
| Role-based noun | `Provisioner`, `Terminator`, `Actuator`, `Controller` | Karpenter, cluster-autoscaler |
| `New*` constructor | `NewProvisioner()`, `NewTerminator()`, `NewActuator()` | All projects |
| Unexported helper | `consolidation`, `drainHelper` (lowercase = internal) | Karpenter, kubectl |
| `*Options` for config | `LaunchOptions`, `ControllerOptions`, `AutoscalerOptions` | All projects |
| `*Error` for errors | `NodeDrainError`, `NodeClaimNotFoundError`, `InsufficientCapacityError` | Karpenter |
| Interface as noun | `Strategy`, `Planner`, `Actuator`, `CloudProvider` | cluster-autoscaler |

### How Reference Projects Name Scaling Coordinators

| Project | Main Scaling Type | What It Does |
|---------|------------------|--------------|
| cluster-autoscaler | `StaticAutoscaler` | Top-level coordinator. Delegates to `Planner` + `Actuator` for scale-down, `Orchestrator` for scale-up |
| Karpenter | `Provisioner` | Coordinates node provisioning. Delegates to `Terminator` for drain |
| Karpenter disruption/ | `Controller` (with `Method` interface) | Coordinates disruption. Methods: `Emptiness`, `Drift`, `Consolidation` |
| cluster-autoscaler expander | `Strategy` (interface) | Selects which node group to expand. Implementations: `random`, `price`, `priority` |
| kubectl drain | `Helper` (struct) | Coordinates node draining with `RunNodeDrain()`, `RunCordonOrUncordon()` |

### How Reference Projects Name Drain Types

| Project | Drain Type | File | Visibility |
|---------|-----------|------|------------|
| kubectl | `Helper` struct | `drain.go` in `pkg/drain/` | Exported (public API) |
| Karpenter | `Terminator` struct | `terminator.go` in `terminator/` | Exported |
| Karpenter | `NodeDrainError` | `eviction.go` in `terminator/` | Exported |
| cluster-autoscaler | `Actuator` struct | `actuator.go` in `actuation/` | Exported |

**Key finding for drain naming:** kubectl upstream names its drain coordinator `Helper`
(generic, inside a `drain` package). Karpenter names it `Terminator` (action-oriented).
Neither uses `drainHelper` or `nodeDrainer`. The dominant pattern is: **the package
name provides the domain context, the type name provides the role**.

---

## Table Stakes

Naming fixes that correct clear violations of Go/K8s naming conventions, or remove
genuinely dead code. Missing any of these leaves the codebase inconsistent with
community norms.

### TS-1: Rename `Strategy` to `Scaler`

**Current:** `type Strategy struct` in `internal/scaling/kubernetes.go`
**Proposed:** `type Scaler struct` in `internal/scaling/scaler.go`

**Verdict: VALIDATED -- rename to `Scaler`**

**Evidence:**

In the cluster-autoscaler, `Strategy` is used specifically for the expander interface
(selects which node group to expand). It is an interface with multiple implementations.
Karpenter uses `Provisioner` for its main node coordinator and `Controller` for its
disruption coordinator. Neither project uses `Strategy` for a concrete, single-
implementation scaling coordinator.

In Stratos, `Strategy` is the sole implementation that coordinates all scaling
operations (demand evaluation, drain, readiness, maintenance). It is not a strategy
selected from alternatives -- it IS the scaler. The name `Strategy` is a vestige of
the removed `ScalingStrategy` interface.

`Scaler` is appropriate because:
1. It describes what the type does (scales nodes up and down)
2. The package is `scaling`, making the fully-qualified name `scaling.Scaler` -- clear
   and non-redundant (cf. Kubernetes guideline: avoid `storage.StorageInterface`)
3. The `New` constructor becomes `scaling.New()` returning `*scaling.Scaler`, matching
   Go idiom where the constructor is just `New()` in the package

**Anti-pattern to avoid:** Do NOT name it `ScalingStrategy`, `ScaleManager`, or
`ScaleController`. The `scaling` package already provides domain context.

**File rename:** `kubernetes.go` -> `scaler.go` is correct. The file's primary type is
`Scaler`, and `kubernetes.go` is a holdover from the old `strategy/kubernetes/` path.
The `subject.go` naming pattern (Karpenter: `provisioner.go`, `terminator.go`,
`actuator.go`) supports naming the file after its primary type.

**Complexity:** MEDIUM -- touches every file that references `Strategy` or `New()` in
the scaling package, plus the reconciler and lifecycle wiring.

**Dependencies:** None. This is the first rename that should happen.

### TS-2: Rename `drainHelper` to `nodeDrainer`

**Current:** `type drainHelper struct` in `internal/scaling/drain.go`
**Proposed:** `type nodeDrainer struct` in `internal/scaling/drain.go`

**Verdict: VALIDATED WITH CAVEAT -- `nodeDrainer` is acceptable; consider just `drainer`**

**Evidence:**

The kubectl upstream drain package names its coordinator `Helper` -- a notoriously
generic name that works only because the package is `drain`. Karpenter avoids the
drain name entirely, using `Terminator` (because Karpenter terminates instances rather
than stopping them). The cluster-autoscaler uses `Actuator` for its drain+delete
coordinator.

In Stratos, `drainHelper` is unexported (lowercase) and internal to the `scaling`
package. Go convention for unexported types is less strict, but `helper` is widely
considered a code smell name -- it says nothing about what the type actually does.

Two valid options:
- **`nodeDrainer`** -- explicitly states it drains nodes. Clear, unambiguous. Slightly
  redundant given the file is `drain.go`, but self-documenting.
- **`drainer`** -- shorter, follows the Go `-er` suffix convention for agent nouns
  (like `Reader`, `Writer`, `Terminator`). The `drain.go` file provides context.

**Recommendation:** `nodeDrainer` is the safer choice because it is unambiguous even
when read outside the context of its file. Both are legitimate.

**The file name `drain.go` should NOT change.** It describes the file's concern (drain
operations), not its primary type. This follows the pattern seen in Karpenter where
`eviction.go` contains eviction logic without being named after its types.

**Complexity:** LOW -- the type is unexported and used only within the scaling package.
All references are in `drain.go`, `drain_eviction.go`, and `kubernetes.go`/`scaler.go`.

### TS-3: Rename `drainConfig` to `drainOptions`

**Current:** `type drainConfig struct` in `internal/scaling/drain.go`
**Proposed:** `type drainOptions struct` in `internal/scaling/drain.go`

**Verdict: VALIDATED**

**Evidence:**

The `*Options` suffix is the dominant convention across all reference projects for
configuration structs:
- Karpenter: `LaunchOptions`, `ControllerOptions`
- cluster-autoscaler: `AutoscalerOptions`, `AutoscalingOptions`
- controller-runtime: `Options` struct in manager, controller packages

The kubectl drain `Helper` uses flat fields rather than a separate config struct, but
when Kubernetes projects DO extract configuration, they name it `*Options`.

`drainConfig` uses `Config`, which in K8s context typically means cluster/runtime
configuration (like `AutoscalingConfig`, `KubeletConfiguration`), not function call
options. `drainOptions` correctly signals "options passed to configure a drain
operation."

**Complexity:** LOW -- unexported type, used only within `drain.go`.

### TS-4: Replace `ScalingDemand.Metadata interface{}` with `Pods []corev1.Pod`

**Current:**
```go
type ScalingDemand struct {
    NodesNeeded int
    Metadata    interface{}
}
```

**Proposed:**
```go
type ScalingDemand struct {
    NodesNeeded int
    Pods        []corev1.Pod
}
```

**Verdict: VALIDATED**

**Evidence:**

The `Metadata interface{}` field existed to support polymorphic strategies (the GitHub
Actions strategy stored different metadata). With only the Kubernetes strategy
remaining, the metadata is always `[]corev1.Pod`. There is exactly one consumer:

```go
// scaling.go:138
pods, ok := demand.Metadata.([]corev1.Pod)
```

Karpenter and cluster-autoscaler never use `interface{}` for known-type fields.
Karpenter's equivalent (`Command` struct) contains `candidates []*state.StateNode` --
a concrete, typed slice. The cluster-autoscaler's `UnneededNode` struct contains
`Node *apiv1.Node` -- again, concrete.

Using `interface{}` when the type is known violates Go type safety idiom. The rename
to `Pods []corev1.Pod` eliminates the type assertion and makes the data flow
self-documenting.

**Complexity:** LOW -- change the struct field, update 2 callsites (construction in
`CheckDemand` and consumption in `OnScaleUp`).

### TS-5: Remove dead `UncordonNode` method

**Current:** `func (d *drainHelper) UncordonNode(...)` in `internal/scaling/drain.go`

**Verdict: VALIDATED -- remove it**

**Evidence:**

Grep confirms zero callers of `UncordonNode` anywhere in the codebase. The only
references are in planning documentation and the method definition itself. The
`PrepareForRunning` method in `readiness.go` handles uncordoning directly via a
`client.Patch` call, not via the drain helper.

In mature K8s projects, dead code is removed aggressively:
- Karpenter's drain path (`Terminator.Drain()`) only includes methods actually called
- The kubectl `drain.Helper` exports `RunCordonOrUncordon()` as a package-level
  function because it IS called by kubectl CLI

Dead methods on unexported types are especially harmful -- they inflate the type's
apparent API surface and mislead code readers about the type's responsibilities.

**Complexity:** LOW -- delete one method (18 lines). No callers to update.

---

## Differentiators

Improvements beyond the obvious that would bring Stratos naming closer to community
norms. Not strictly required, but recommended while the files are being touched.

### D-1: Rename `types.go` to `scaling_types.go` (in `internal/scaling/`)

**Current:** `internal/scaling/types.go`
**Proposed:** `internal/scaling/scaling_types.go`

**Verdict: NOT RECOMMENDED -- keep `types.go`**

**Evidence:**

`types.go` is the universal convention:
- Karpenter `pkg/cloudprovider/types.go` -- defines `CloudProvider`, `InstanceType`,
  `Offering`, all error types
- Karpenter `pkg/controllers/disruption/types.go` -- defines `Candidate`, `Command`,
  `Method`, `Decision`
- kubebuilder-generated CRD packages always use `types.go`

NO mature K8s project uses `scaling_types.go` within a `scaling/` package. The
pattern `<domain>_types.go` appears only at the API level (e.g.,
`api/v1alpha1/nodepool_types.go`) where multiple CRD kinds share a package. Within
an internal package with a single domain, `types.go` is standard.

Renaming to `scaling_types.go` would be redundant (`scaling/scaling_types.go`) and
deviate from community convention.

### D-2: Rename `events.go` to `pod_events.go` (in `internal/scaling/`)

**Current:** `internal/scaling/events.go`
**Proposed:** `internal/scaling/pod_events.go`

**Verdict: ACCEPTABLE but NOT REQUIRED**

**Evidence:**

Karpenter uses `events/` subdirectories (e.g., `disruption/events/`, `terminator/events/`)
to separate event constants from logic. The cluster-autoscaler does not have a
specific pattern for event files.

In Stratos, `events.go` contains pod-to-nodepool mapping functions
(`kubernetesPodToNodePoolMapper`, `isPodUnschedulable`, `PodEventHandler`) and
predicates. The file is misnamed -- its content is about pod event handling
and filtering, not about emitting Kubernetes events.

`pod_events.go` would be more descriptive. However, examining the file contents
more carefully, it contains:
1. `kubernetesPodToNodePoolMapper` -- maps unschedulable pods to NodePools
2. `isPodUnschedulable` -- checks pod unschedulable condition
3. `couldSatisfyPod` -- checks if pool can satisfy a pod
4. `PodEventHandler` -- returns the event handler
5. `UnschedulablePodPredicate` -- returns the predicate

These are all about **pod-to-nodepool matching and event handling**, not just "events."
A better name might be `pod_matching.go` or keeping it as `events.go` (which at least
signals "event handling" to a reader).

**Recommendation:** If renaming, `pod_events.go` is acceptable. But this is cosmetic.
The file name does not impede understanding.

### D-3: Rename `cloudprovider/types.go` to `instance_types.go`

**Current:** `internal/cloudprovider/types.go`
**Proposed:** `internal/cloudprovider/instance_types.go`

**Verdict: NOT RECOMMENDED -- keep `types.go`**

**Evidence:**

Karpenter's `pkg/cloudprovider/types.go` contains:
- `CloudProvider` interface
- `InstanceType` struct
- `InstanceTypeOverhead` struct
- `Offering` struct
- `RepairPolicy` struct
- All error types (`NodeClaimNotFoundError`, `InsufficientCapacityError`, etc.)

Stratos's `internal/cloudprovider/types.go` contains:
- `Instance` struct
- `InstanceState` type and constants
- Error types (`InstanceNotFoundError`, `InvalidStateError`, `RateLimitError`,
  `QuotaExceededError`, `InsufficientCapacityError`)
- `InstanceCapacity` struct
- `InstanceCapacityProvider` type

The Stratos file is a near-exact structural parallel to Karpenter's -- both are the
"bag of types" file for the cloudprovider package. Karpenter calls it `types.go`.
The cluster-autoscaler's cloudprovider package also uses `types.go`.

Renaming to `instance_types.go` would be misleading because the file contains error
types and capacity types that are not specifically about instances. It would also
conflict with the existing `internal/cloudprovider/aws/instance_types.go` file
(which IS specifically about instance type -> capacity mapping).

### D-4: Consider renaming `kubernetes.go` test file

**Current:** `internal/scaling/kubernetes_test.go`
**After TS-1:** Should become `internal/scaling/scaler_test.go`

**Verdict: REQUIRED (follows from TS-1)**

When renaming `kubernetes.go` to `scaler.go`, the test file MUST be renamed to match.
Go convention requires test files to mirror their source file name with `_test.go`
suffix. This is not optional -- it is part of TS-1.

### D-5: Rename `doc.go` package documentation references

**Current:** `internal/scaling/doc.go` references `Strategy` as a key type
**After TS-1:** Must reference `Scaler`

**Verdict: REQUIRED (follows from TS-1)**

Package documentation that references renamed types is actively misleading. This is
mechanical and should be done as part of each rename.

---

## Anti-Features

Things to deliberately NOT do during this naming cleanup.

### AF-1: Do NOT rename the `scaling` package itself

**Trap:** "The main type is `Scaler`, so maybe the package should be `scaler`."

**Why avoid:** Go convention says packages are named for what they PROVIDE, not for
their primary type. The `scaling` package provides scaling operations (demand evaluation,
drain, readiness, maintenance). The name `scaler` as a package would be agent-noun-
oriented, which is less idiomatic for multi-concern packages.

Reference: `controller-runtime/pkg/reconcile/` provides reconciliation types but is
named `reconcile` (verb form), not `reconciler`. Similarly, `kubectl/pkg/drain/`
provides drain operations but is named `drain` (verb), not `drainer`.

**Keep `scaling` as the package name.**

### AF-2: Do NOT rename `ScalingDemand` to `Demand`

**Trap:** "Since it is in the `scaling` package, `scaling.Demand` is sufficient."

**Why avoid:** While Go convention discourages redundant package-qualified names (e.g.,
`storage.StorageInterface` is bad), `ScalingDemand` is not redundant -- it is a
domain-specific compound noun. `Demand` alone is too generic and could be confused
with resource demand, scheduling demand, etc.

Reference: cluster-autoscaler's `AutoscalerOptions` lives in a `core` package, not
an `autoscaler` package. The full qualifier `core.AutoscalerOptions` is clear.
Similarly, `scaling.ScalingDemand` is clear -- the `Scaling` prefix specifies what
kind of demand.

**However,** if the team prefers brevity, `scaling.Demand` is not wrong -- it is just
less self-documenting. The recommendation is to keep `ScalingDemand` for clarity.

### AF-3: Do NOT rename `ScaleCalculator` to `Calculator`

**Trap:** "It is in the `scaling` package, so `scaling.Calculator` is enough."

**Why avoid:** `Calculator` is overly generic. `ScaleCalculator` clearly states it
calculates how many nodes to scale. This mirrors Karpenter's `LaunchOptions` (not
just `Options`) and cluster-autoscaler's `ClusterStateRegistry` (not just `Registry`).

Types that perform specific domain operations benefit from descriptive prefixes even
inside domain packages. Save abbreviation for types that ARE the package's primary
abstraction (like `Scaler` being the main type in `scaling`).

### AF-4: Do NOT rename `ScaleDownCandidate`

**Trap:** "Let's rename it to `DownscaleCandidate` or `RemovalCandidate` for consistency."

**Why avoid:** `ScaleDownCandidate` is already idiomatic. The cluster-autoscaler uses
`UnneededNode` with a `RemovalThreshold` field. Karpenter uses `Candidate` (inside
a `disruption` package that provides context). Stratos's `ScaleDownCandidate` is
perfectly clear and self-documenting. Renaming it would be churn without benefit.

### AF-5: Do NOT rename exported functions that match Go convention

**Trap:** "While we are renaming things, let's also rename `CheckDemand` to
`EvaluateDemand` or `RunMaintenance` to `Maintain`."

**Why avoid:** The existing method names on `Strategy` (soon `Scaler`) already follow
Go convention:
- `CheckDemand` -- verb phrase, describes action
- `OnScaleUp` -- event handler convention
- `FindScaleDownCandidates` -- verb phrase, returns candidates
- `DrainAndStop` -- compound verb, describes the two-step operation
- `RunMaintenance` -- verb phrase, matches Karpenter's `Reconcile` pattern
- `IsReady` -- predicate convention
- `PrepareForRunning` / `PrepareForStandby` -- state preparation convention

These are well-named. Renaming them would be churn with no convention improvement.

### AF-6: Do NOT merge drain.go and drain_eviction.go into a single file

**Trap:** "These are both about draining, so combine them."

**Why avoid:** `drain.go` (172 lines) defines the `drainHelper` type and its high-level
drain workflow. `drain_eviction.go` contains the low-level eviction mechanics (pod
filtering, eviction API calls, deletion waiting). This separation mirrors Karpenter's
split between `terminator.go` (high-level drain ordering) and `eviction.go` (eviction
queue mechanics). The split is intentional and aids readability.

---

## Feature Dependencies

```
TS-1: Rename Strategy -> Scaler (+ file rename kubernetes.go -> scaler.go)
  |
  +--> D-4: Rename kubernetes_test.go -> scaler_test.go (REQUIRED, part of TS-1)
  |
  +--> D-5: Update doc.go references (REQUIRED, part of TS-1)

TS-2: Rename drainHelper -> nodeDrainer
  |
  (independent of TS-1)

TS-3: Rename drainConfig -> drainOptions
  |
  (independent, typically done with TS-2 since same file)

TS-4: Replace ScalingDemand.Metadata interface{} -> Pods []corev1.Pod
  |
  (independent of TS-1 through TS-3)

TS-5: Remove dead UncordonNode method
  |
  (independent, typically done with TS-2 since same file)
```

All five table-stakes items are independent of each other. They can be done in any
order or in parallel. The recommended grouping is:

1. **TS-1 + D-4 + D-5** -- The Strategy -> Scaler rename with file and doc updates
2. **TS-2 + TS-3 + TS-5** -- All drain.go changes together (rename types + remove dead code)
3. **TS-4** -- The ScalingDemand type change (touches types.go and scaling.go)

---

## Recommended Execution Order

1. **TS-2 + TS-3 + TS-5** -- Drain cleanup first (lowest risk, fewest cross-file refs)
2. **TS-4** -- ScalingDemand.Metadata -> Pods (contained within scaling package)
3. **TS-1 + D-4 + D-5** -- Strategy -> Scaler rename last (most files touched)

**Rationale:** Start with the most contained changes (drain types are unexported and
file-local). Build confidence. Then do the Scaler rename which touches more files.

---

## Impact Summary

| Change | Files Modified | Lines Changed (est.) | Risk |
|--------|---------------|---------------------|------|
| TS-1: Strategy -> Scaler | ~8 files | ~60 | MEDIUM (cross-package) |
| TS-2: drainHelper -> nodeDrainer | 3 files | ~15 | LOW (unexported) |
| TS-3: drainConfig -> drainOptions | 1 file | ~8 | LOW (unexported) |
| TS-4: Metadata -> Pods | 2 files | ~6 | LOW (2 callsites) |
| TS-5: Remove UncordonNode | 1 file | -18 | LOW (dead code) |

**Total estimated: ~10 files modified, ~107 lines changed (including -18 deleted)**

---

## Sources

### Primary Sources (HIGH confidence)

- Karpenter source: `sigs.k8s.io/karpenter` -- file structure, type naming,
  [controllers](https://github.com/kubernetes-sigs/karpenter/tree/main/pkg/controllers),
  [cloudprovider/types.go](https://github.com/kubernetes-sigs/karpenter/blob/main/pkg/cloudprovider/types.go),
  [disruption/types.go](https://github.com/kubernetes-sigs/karpenter/blob/main/pkg/controllers/disruption/types.go),
  [terminator/](https://github.com/kubernetes-sigs/karpenter/tree/main/pkg/controllers/node/termination/terminator)
- cluster-autoscaler source: `k8s.io/autoscaler` -- [core/](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/core),
  [scaledown/](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/core/scaledown),
  [expander/](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/expander)
- kubectl drain: [pkg/drain/drain.go](https://github.com/kubernetes/kubectl/blob/master/pkg/drain/drain.go) --
  `Helper` struct naming
- [Kubernetes coding conventions](https://www.kubernetes.dev/docs/guide/coding-convention/)
- [Effective Go](https://go.dev/doc/effective_go) -- canonical Go naming guidance
- [Google Go Style Decisions](https://google.github.io/styleguide/go/decisions.html)

### Codebase Analysis (HIGH confidence)

- Direct grep and read of all files in `internal/scaling/`, `internal/cloudprovider/`,
  `internal/controller/` to verify current naming and cross-references
- Confirmed `UncordonNode` has zero callers via codebase-wide grep
- Confirmed `Metadata` field has exactly one consumer (type assertion in `OnScaleUp`)

---
*Feature research for: Stratos operator v1.1.1 naming cleanup*
*Researched: 2026-02-04*
