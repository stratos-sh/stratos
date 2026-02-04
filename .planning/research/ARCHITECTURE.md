# Architecture Research: Naming & Dead Code Cleanup -- Safe Rename Ordering

**Domain:** Kubernetes operator refactoring -- renaming types and files in internal/scaling/ and internal/cloudprovider/
**Researched:** 2026-02-04
**Confidence:** HIGH (every cross-package reference verified by reading source code)

## Executive Summary

The naming cleanup involves renaming exported types, unexported types, file names, and
doc comments across two packages (`internal/scaling/` and `internal/cloudprovider/`).
The critical constraint is that Go requires all packages to compile at every commit. A
type rename that breaks a cross-package reference creates a compile error that blocks
all other work in that package.

This document maps every cross-package reference, categorizes renames by blast radius
(cross-package vs. package-internal), and prescribes a safe ordering that keeps the
build green at every step.

---

## 1. Complete Type and Symbol Inventory

### 1.1 internal/scaling/ -- Exported Symbols Used Cross-Package

| Symbol | Defined In | Cross-Package Consumers | Reference Count |
|--------|-----------|------------------------|-----------------|
| `Strategy` (struct) | kubernetes.go:39 | reconciler.go:70, reconciler_helpers.go:40/76/100/135/174, setup.go:68, provider_cache.go:103, reconciler.go:203, doc.go:27/35 | 11 sites in controller/nodepool |
| `New()` (constructor) | kubernetes.go:48 | setup.go:68 | 1 site |
| `ScalingDemand` (struct) | types.go:22 | (none -- used only within scaling package and passed by value through scaling.Strategy methods) | 0 cross-package direct refs |
| `ScaleDownCandidate` (struct) | types.go:31 | reconciler_helpers.go:130, reconciler_helpers.go:198 | 2 sites in controller/nodepool |
| `ScaleCalculator` (struct) | capacity.go:36 | pod_assignment_test.go:112/139 | 2 sites (test only) |
| `NewScaleCalculator()` | capacity.go:45 | pod_assignment_test.go:112/139 | 2 sites (test only) |
| `PodEventHandler()` | events.go:209 | setup.go:74 | 1 site |
| `UnschedulablePodPredicate()` | events.go:198 | setup.go:75 | 1 site |

### 1.2 internal/scaling/ -- Unexported Symbols (Package-Internal Only)

| Symbol | Defined In | Used In (within scaling/) |
|--------|-----------|--------------------------|
| `drainHelper` (struct) | drain.go:30 | drain.go, drain_eviction.go, kubernetes.go |
| `drainConfig` (struct) | drain.go:41 | drain.go, kubernetes.go |
| `newDrainHelper()` | drain.go:63 | kubernetes.go:77 |
| `defaultDrainConfig()` | drain.go:51 | drain.go:64 |
| `networkReadinessChecker` (struct) | network.go:30 | network.go, kubernetes.go:44, readiness.go |
| `newNetworkReadinessChecker()` | network.go:39 | kubernetes.go:59 |
| `isNodeEmpty()` | drain_eviction.go:170 | scaling.go:184 |
| `isDaemonSetPod()` | drain_eviction.go:204 | drain_eviction.go:65 |
| `isPodUnschedulable()` | events.go:78 | events.go:44, kubernetes.go:150, pod_assignments.go:172 |
| `couldSatisfyPod()` | events.go:117 | events.go:60, kubernetes.go:150 |
| `parseScaleDownTimestamp()` | scaling.go:243 | scaling.go:195 |
| `getNodeStartTime()` | readiness.go:150 | readiness.go:139 |

### 1.3 internal/cloudprovider/ -- Exported Symbols

| Symbol | Defined In | Cross-Package Consumers |
|--------|-----------|------------------------|
| `CloudProvider` (interface) | interface.go:41 | nodepool/reconciler.go, nodepool/provider_cache.go, nodepool/reconciler_helpers.go, lifecycle/manager.go, fake/provider.go |
| `Instance` (struct) | types.go:27 | aws/provider.go, fake/provider.go, lifecycle/manager.go, lifecycle/node_launch.go |
| `InstanceState` (type) | types.go:57 | aws/provider.go, fake/provider.go, lifecycle/* |
| `InstanceCapacity` (struct) | types.go:154 | aws/instance_types.go, scaling/capacity.go |
| `InstanceCapacityProvider` (func type) | types.go:166 | scaling/kubernetes.go, nodepool/reconciler.go, nodepool/setup.go |
| `TemplateConfig` (struct) | interface.go:27 | lifecycle/node_launch.go, fake/provider.go, aws/provider.go |
| Error types (5) | types.go | aws/provider.go, fake/provider.go |
| `NodeLauncher` (interface) | lifecycle/manager.go:36 | lifecycle/node_launch.go |
| `NodeHooks` (interface) | lifecycle/manager.go:42 | provider_cache.go:103 (compile-time assert) |

### 1.4 internal/cloudprovider/fake/ -- Symbols

| Symbol | Defined In | Cross-Package Consumers |
|--------|-----------|------------------------|
| `FakeProvider` (struct) | provider.go:31 | nodepool/provider_cache.go:64, tests |
| `NewFakeProvider()` | provider.go:48 | nodepool/provider_cache.go:64, tests |
| `FakeResolver` (struct) | resolver.go:27 | controller/nodeclass/reconciler_test.go |
| `NewFakeResolver()` | resolver.go:43 | controller/nodeclass/reconciler_test.go |

---

## 2. Dependency Graph

```
cmd/stratos/main.go
  |
  v
internal/controller/setup.go  (aggregator)
  |
  +---> internal/controller/nodepool/  (imports scaling, cloudprovider, lifecycle, nodestate)
  |       |
  |       +---> internal/scaling/          (imports cloudprovider, nodestate)
  |       |       |
  |       |       +---> internal/cloudprovider/types.go  (InstanceCapacity, InstanceCapacityProvider)
  |       |       +---> internal/controller/nodepool/nodestate/
  |       |
  |       +---> internal/controller/nodepool/lifecycle/  (imports cloudprovider, nodestate)
  |       |       |
  |       |       +---> internal/cloudprovider/  (CloudProvider, Instance, TemplateConfig)
  |       |
  |       +---> internal/cloudprovider/     (CloudProvider interface, types)
  |       +---> internal/cloudprovider/aws/  (AWSProvider, RateLimitConfig)
  |       +---> internal/cloudprovider/fake/ (FakeProvider -- test paths)
  |
  +---> internal/controller/nodeclass/  (imports cloudprovider/aws)
```

Key observations:
- **No circular dependencies.** The graph is a DAG.
- `internal/scaling/` is a **leaf package** -- nothing imports it except `controller/nodepool/`.
- `internal/cloudprovider/` is at the **bottom of the DAG** -- many packages depend on it.
- `internal/controller/nodepool/nodestate/` is shared by both `scaling/` and `lifecycle/`.

---

## 3. Rename Categories by Blast Radius

### Category A: Cross-Package Type Renames (HIGH blast radius)

These change an exported type name referenced from outside the defining package. Every
consumer must be updated in the same commit (or the same file, atomically).

| Rename | Package | Consumer Sites |
|--------|---------|---------------|
| `Strategy` -> ? | scaling | 11 sites in controller/nodepool (4 files + doc.go) |
| `ScaleDownCandidate` -> ? | scaling | 2 sites in reconciler_helpers.go, 1 in lifecycle compile-time assert concept |
| `ScaleCalculator` / `NewScaleCalculator` -> ? | scaling | 2 sites in pod_assignment_test.go (test file) |

### Category B: Cross-Package File Renames (LOW blast radius)

Go does not care about file names -- only package names matter. Renaming a `.go` file
has zero compile-time impact. The only concern is git history and PR readability.

| File Rename | Package | Compile Impact |
|-------------|---------|---------------|
| `kubernetes.go` -> ? | scaling | None |
| `types.go` -> ? (if splitting) | scaling | None |
| Any cloudprovider file renames | cloudprovider | None |

### Category C: Package-Internal Renames (ZERO cross-package blast radius)

These are unexported symbols. Renaming them cannot break any other package.

| Rename | Package | Impact |
|--------|---------|--------|
| `drainHelper` -> ? | scaling | drain.go, drain_eviction.go, kubernetes.go only |
| `drainConfig` -> ? | scaling | drain.go, kubernetes.go only |
| `networkReadinessChecker` -> ? | scaling | network.go, kubernetes.go, readiness.go only |
| Any unexported helper functions | scaling | Package-internal only |

### Category D: Doc/Comment Updates (ZERO compile impact)

| Update | Files |
|--------|-------|
| doc.go godoc for scaling package | scaling/doc.go |
| doc.go godoc for nodepool package | nodepool/doc.go |
| Comment references to old names | scattered |

---

## 4. Safe Rename Ordering

The core principle: **rename leaf packages first, then work up the dependency tree.**
Within a package, **rename unexported symbols first, then exported symbols.** File
renames can happen at any point since Go does not encode file names in the type system.

### Phase 1: Package-Internal Renames (scaling/)

**Why first:** Zero cross-package blast radius. Each rename compiles independently.
No coordination with other packages needed.

**Order within phase:**

1. **`drainHelper` -> new name** (e.g., `nodeDrainer`)
   - Files: drain.go, drain_eviction.go, kubernetes.go
   - All within `package scaling`
   - Test: `go build ./internal/scaling/...`

2. **`drainConfig` -> new name** (e.g., `drainOptions`)
   - Files: drain.go, kubernetes.go
   - Test: `go build ./internal/scaling/...`

3. **`networkReadinessChecker` -> new name** (if renaming)
   - Files: network.go, kubernetes.go, readiness.go
   - Test: `go build ./internal/scaling/...`

4. **Any other unexported symbol renames**
   - Purely internal, one at a time

**Build verification after Phase 1:**
```bash
go build ./internal/scaling/...
go test ./internal/scaling/... -count=1 -short
```

### Phase 2: File Renames (scaling/ and cloudprovider/)

**Why second:** File renames have zero compile impact but are best done before
exported type renames to avoid confusion in diffs. Git tracks renames better when
the file content has not also changed substantially.

**Order within phase (no ordering constraint -- all independent):**

1. `scaling/kubernetes.go` -> new name (e.g., `strategy.go` or `scaler.go`)
2. `scaling/types.go` -> new name if needed
3. Any cloudprovider file renames

**Build verification after Phase 2:**
```bash
go build ./...
```

### Phase 3: Exported Type Renames in scaling/ (cross-package)

**Why third:** These are the highest-blast-radius changes. Each rename must update
both the definition (in scaling/) and all consumers (in controller/nodepool/) atomically.

**Order within phase:**

1. **`Strategy` -> new name** (e.g., `Scaler` or `PodDemandScaler`)
   - **Definition:** scaling/kubernetes.go (struct + constructor + all methods)
   - **Consumers (ALL must update in same commit):**
     - `controller/nodepool/reconciler.go:70` -- `scaler *scaling.Strategy`
     - `controller/nodepool/reconciler.go:203` -- `scaler *scaling.Strategy` param
     - `controller/nodepool/reconciler_helpers.go:40` -- `scaler *scaling.Strategy` param
     - `controller/nodepool/reconciler_helpers.go:76` -- `scaler *scaling.Strategy` param
     - `controller/nodepool/reconciler_helpers.go:100` -- `scaler *scaling.Strategy` param
     - `controller/nodepool/reconciler_helpers.go:135` -- `scaler *scaling.Strategy` param
     - `controller/nodepool/reconciler_helpers.go:174` -- `scaler *scaling.Strategy` param
     - `controller/nodepool/setup.go:68` -- `scaling.New(...)` (if constructor also renames)
     - `controller/nodepool/provider_cache.go:103` -- compile-time assert `(*scaling.Strategy)(nil)`
     - `controller/nodepool/doc.go:27,35` -- comment references
     - `lifecycle/manager.go:41` -- comment reference only
     - `scaling/doc.go:23` -- comment reference
   - **Test:** `go build ./...`

2. **`ScaleDownCandidate` -> new name** (if renaming)
   - **Definition:** scaling/types.go
   - **Consumers:**
     - `controller/nodepool/reconciler_helpers.go:130` -- param type
     - `controller/nodepool/reconciler_helpers.go:198` -- struct literal
   - **Test:** `go build ./...`

3. **`ScalingDemand` -> new name** (if renaming)
   - **Definition:** scaling/types.go
   - **Consumers:** None cross-package (used only as return/param of Strategy methods;
     consumers reference it via `demand.NodesNeeded` after calling `scaler.CheckDemand()`
     which returns it by value)
   - Actually check: reconciler_helpers.go does not import the type name directly --
     it uses `demand, err := scaler.CheckDemand(...)` and `demand.NodesNeeded`, which
     uses Go's type inference. The only explicit reference would be if the variable is
     typed, but it uses `:=`.
   - **Verdict:** Lower risk, but still update doc.go + scaling/doc.go comments.
   - **Test:** `go build ./...`

4. **`ScaleCalculator` / `NewScaleCalculator` -> new name** (if renaming)
   - **Definition:** scaling/capacity.go
   - **Consumers:**
     - `controller/nodepool/pod_assignment_test.go:112,139` -- test file only
   - **Test:** `go test ./internal/controller/nodepool/... -count=1`

**Build verification after Phase 3:**
```bash
go build ./...
go test ./... -count=1 -short
```

### Phase 4: cloudprovider/ Renames (if any)

**Why last:** cloudprovider/ is at the bottom of the dependency tree. Renaming
anything there has the widest blast radius -- scaling/, lifecycle/, nodepool/,
nodeclass/, fake/, aws/, and tests all import it.

**If renaming types in cloudprovider/types.go:**

Each rename requires updating ALL importers simultaneously:

| Type | Importer Count |
|------|---------------|
| `InstanceCapacity` | scaling/capacity.go, aws/instance_types.go, aws/instance_types_test.go |
| `InstanceCapacityProvider` | scaling/kubernetes.go, nodepool/reconciler.go, nodepool/setup.go |
| `InstanceState` | Many files in aws/, fake/, lifecycle/ |
| `CloudProvider` | nodepool/, lifecycle/, fake/ |

**Recommendation:** Do NOT rename cloudprovider/ types in this milestone unless
there is a compelling reason. The blast radius is very large and the current names
(`CloudProvider`, `Instance`, `InstanceState`) are already clear and idiomatic.

---

## 5. The Strategy Rename in Detail

This is the highest-value and highest-risk rename. Here is the exact edit list.

### Definition Side (internal/scaling/)

| File | Line | Current | New (example: Scaler) |
|------|------|---------|----|
| kubernetes.go | 39 | `type Strategy struct {` | `type Scaler struct {` |
| kubernetes.go | 48 | `func New(...) *Strategy {` | `func New(...) *Scaler {` |
| kubernetes.go | 54 | `return &Strategy{` | `return &Scaler{` |
| kubernetes.go | 64 | `func (s *Strategy) DrainAndStop(...)` | `func (s *Scaler) DrainAndStop(...)` |
| kubernetes.go | 105 | `func (s *Strategy) getNodesForPool(...)` | `func (s *Scaler) getNodesForPool(...)` |
| kubernetes.go | 116 | `func (s *Strategy) getRunningNodes(...)` | `func (s *Scaler) getRunningNodes(...)` |
| kubernetes.go | 128 | `func (s *Strategy) getNodeClass(...)` | `func (s *Scaler) getNodeClass(...)` |
| kubernetes.go | 142 | `func (s *Strategy) getUnschedulablePods(...)` | `func (s *Scaler) getUnschedulablePods(...)` |
| scaling.go | 33 | `func (s *Strategy) CheckDemand(...)` | `func (s *Scaler) CheckDemand(...)` |
| scaling.go | 100 | `func (s *Strategy) adjustForCapacity(...)` | `func (s *Scaler) adjustForCapacity(...)` |
| scaling.go | 132 | `func (s *Strategy) OnScaleUp(...)` | `func (s *Scaler) OnScaleUp(...)` |
| scaling.go | 149 | `func (s *Strategy) FindScaleDownCandidates(...)` | `func (s *Scaler) FindScaleDownCandidates(...)` |
| scaling.go | 181 | `func (s *Strategy) evaluateScaleDownNode(...)` | `func (s *Scaler) evaluateScaleDownNode(...)` |
| scaling.go | 214 | `func (s *Strategy) clearScaleDownAnnotation(...)` | `func (s *Scaler) clearScaleDownAnnotation(...)` |
| scaling.go | 230 | `func (s *Strategy) markScaleDownCandidate(...)` | `func (s *Scaler) markScaleDownCandidate(...)` |
| scaling.go | 259 | `func (s *Strategy) countStartingNodes(...)` | `func (s *Scaler) countStartingNodes(...)` |
| readiness.go | 34 | `func (s *Strategy) IsReady(...)` | `func (s *Scaler) IsReady(...)` |
| readiness.go | 57 | `func (s *Strategy) PrepareForRunning(...)` | `func (s *Scaler) PrepareForRunning(...)` |
| readiness.go | 85 | `func (s *Strategy) PrepareForStandby(...)` | `func (s *Scaler) PrepareForStandby(...)` |
| readiness.go | 129 | `func (s *Strategy) processStartupTaints(...)` | `func (s *Scaler) processStartupTaints(...)` |
| readiness.go | 163 | `func (s *Strategy) forceRemoveNetworkReadinessTaint(...)` | `func (s *Scaler) forceRemoveNetworkReadinessTaint(...)` |
| readiness.go | 190 | `func (s *Strategy) removeNetworkReadinessTaintWhenReady(...)` | `func (s *Scaler) removeNetworkReadinessTaintWhenReady(...)` |
| maintenance.go | 39 | `func (s *Strategy) RunMaintenance(...)` | `func (s *Scaler) RunMaintenance(...)` |
| maintenance.go | 82 | `func (s *Strategy) ensureTemplateLabels(...)` | `func (s *Scaler) ensureTemplateLabels(...)` |
| maintenance.go | 133 | `func (s *Strategy) clearStaleScaleUpAnnotations(...)` | `func (s *Scaler) clearStaleScaleUpAnnotations(...)` |
| maintenance.go | 178 | `func (s *Strategy) removeScaleUpAnnotation(...)` | `func (s *Scaler) removeScaleUpAnnotation(...)` |
| pod_assignments.go | 35 | `func (s *Strategy) filterAssignedPods(...)` | `func (s *Scaler) filterAssignedPods(...)` |
| pod_assignments.go | 69 | `func (s *Strategy) createPodAssignments(...)` | `func (s *Scaler) createPodAssignments(...)` |
| pod_assignments.go | 116 | `func (s *Strategy) estimatePodsPerNode(...)` | `func (s *Scaler) estimatePodsPerNode(...)` |
| pod_assignments.go | 140 | `func (s *Strategy) cleanupPodAssignments(...)` | `func (s *Scaler) cleanupPodAssignments(...)` |
| doc.go | 23 | `Strategy: core scaling logic` | update to new name |

**Total definition-side edits:** ~30 method receivers + struct + constructor + doc.

### Consumer Side (internal/controller/nodepool/)

| File | Line | Current | New |
|------|------|---------|----|
| reconciler.go | 70 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| reconciler.go | 203 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| reconciler_helpers.go | 40 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| reconciler_helpers.go | 76 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| reconciler_helpers.go | 100 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| reconciler_helpers.go | 135 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| reconciler_helpers.go | 174 | `scaler *scaling.Strategy` | `scaler *scaling.Scaler` |
| provider_cache.go | 103 | `(*scaling.Strategy)(nil)` | `(*scaling.Scaler)(nil)` |
| doc.go | 27 | `scaling.Strategy` | `scaling.Scaler` |
| doc.go | 35 | `scaling.Strategy` | `scaling.Scaler` |

**Total consumer-side edits:** 10 sites across 4 files.

### Comment-Only References

| File | Line | Note |
|------|------|------|
| lifecycle/manager.go | 41 | `// The scaling.Strategy implements this interface.` -- comment only |
| scaling/doc.go | 23 | `Strategy: core scaling logic` -- comment only |

---

## 6. Build Order Verification Strategy

After each rename step, the entire module must compile. Here is the verification sequence:

```bash
# After each change:
go build ./...                          # Compile check -- must pass
go vet ./...                            # Static analysis
go test ./internal/scaling/... -short   # scaling package tests
go test ./internal/controller/... -short # controller tests (cross-package)
```

### Atomic Commit Boundaries

Each commit must leave the tree in a compilable state. The following are valid commit
boundaries:

1. **One commit per unexported rename** -- safe because no cross-package impact
2. **One commit per exported rename + all consumers** -- MUST be atomic
3. **One commit for all file renames** -- safe because no compile impact
4. **One commit for all doc/comment updates** -- safe because no compile impact

Anti-pattern to avoid: renaming the type in the definition but forgetting a consumer.
This breaks the build. Always use `go build ./...` before committing.

### Recommended Commit Sequence

```
Commit 1: Rename unexported drainHelper -> nodeDrainer (+ drainConfig if desired)
Commit 2: Rename file kubernetes.go -> strategy.go (or scaler.go)
Commit 3: Rename Strategy -> Scaler (definition + ALL consumers, atomic)
Commit 4: Rename ScaleDownCandidate -> ? (definition + 2 consumer sites, atomic)
Commit 5: Update doc.go files and comments
Commit 6: Dead code removal (separate concern, keep renames clean)
```

---

## 7. Special Considerations

### 7.1 The lifecycle.NodeHooks Compile-Time Assert

```go
// provider_cache.go:103
var _ lifecycle.NodeHooks = (*scaling.Strategy)(nil)
```

This is a compile-time assertion that `*scaling.Strategy` implements `lifecycle.NodeHooks`.
When `Strategy` is renamed, this line MUST be updated in the same commit. If missed,
the build fails with:

```
cannot use (*scaling.Strategy)(nil) (untyped nil value) as lifecycle.NodeHooks value
```

This is actually helpful -- it acts as a forced-update sentinel. But it is easy to miss
in a search for just `scaling.Strategy` because it uses pointer-to-type syntax.

### 7.2 ScalingDemand Is Safe to Rename Independently

`ScalingDemand` is returned by `CheckDemand()` and passed to `OnScaleUp()`. Consumers
in reconciler_helpers.go use `:=` (type inference):

```go
demand, err := scaler.CheckDemand(ctx, nodePool, standby, running, int(nodePool.Spec.PoolSize))
// ...
if onErr := scaler.OnScaleUp(ctx, nodePool, startedNodes, demand); onErr != nil {
```

The type name `ScalingDemand` never appears in controller/nodepool/*.go source code.
Only `demand.NodesNeeded` (field access) and `demand` (passed by value) appear. This
means renaming `ScalingDemand` has ZERO cross-package compile impact. Only scaling/
internal files and scaling/doc.go need updating.

### 7.3 Git Rename Detection

Git detects renames when file content similarity is above 50%. To maximize rename
detection:

- Rename files in separate commits from content changes
- If renaming a file AND changing its contents, do the file rename first (empty diff),
  then change contents in the next commit

### 7.4 Test Files Are in the Same Package

All test files in `internal/scaling/` use `package scaling` (not `package scaling_test`).
They reference `Strategy` as an unqualified name. When renaming, these test files need
updating too, but they are package-internal so they compile with the definition.

The exception is `controller/nodepool/pod_assignment_test.go` which is in `package
nodepool` and uses `scaling.NewScaleCalculator`. This is a cross-package test reference
that must be updated if `NewScaleCalculator` is renamed.

### 7.5 No Interface Satisfaction Changes

`Strategy` implements `lifecycle.NodeHooks` (3 methods: PrepareForRunning,
PrepareForStandby, IsReady). Renaming the struct does NOT change the method set.
The compile-time assert just needs the new type name. No interface changes needed.

---

## 8. Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Missing a consumer site during Strategy rename | HIGH (build breaks) | Use `go build ./...` after every rename; grep for old name |
| Forgetting the compile-time assert in provider_cache.go | MEDIUM (build breaks, but obvious) | Include in checklist |
| Git history confusion from simultaneous file + content rename | LOW (cosmetic) | Separate file rename from content rename into distinct commits |
| Renaming cloudprovider types (if attempted) | HIGH (very wide blast radius) | Do NOT rename cloudprovider types in this milestone |
| Missing comment/doc updates | LOW (no compile impact) | Do as final pass; imperfect is acceptable |

---

## 9. Implications for Roadmap

### Recommended Phase Structure

**Phase 1: Internal Renames (scaling/ unexported types)**
- `drainHelper`, `drainConfig`, `networkReadinessChecker`
- Zero cross-package risk, can be done file-by-file
- Good warmup for the team/tooling

**Phase 2: File Renames**
- `kubernetes.go` -> better name
- Any other file renames
- Zero compile risk, one commit each

**Phase 3: Exported Type Renames (cross-package, atomic)**
- `Strategy` rename is the big one: definition + 10 consumer sites in one commit
- `ScaleDownCandidate` rename: definition + 2 consumer sites
- `ScalingDemand` rename: definition only (no cross-package refs)
- `ScaleCalculator` / `NewScaleCalculator`: definition + 2 test sites

**Phase 4: Documentation and Comment Cleanup**
- Update all doc.go files
- Update comment references to old type names
- No compile impact

**Phase 5: Dead Code Removal (if in scope)**
- Separate from renames to keep diffs clean

### Phase Ordering Rationale

The ordering is driven by two principles:
1. **Increasing blast radius:** Start with zero-risk internal changes, end with
   cross-package atomics
2. **File renames before content renames:** Better git history, easier PR review

### Research Flags

- Phase 3 (exported type renames) benefits from having an exact checklist of every
  reference site (provided in Sections 5 and above)
- No phase needs deeper research -- all integration points are documented in this file

---

## Sources

All findings verified by reading source code directly:

- `internal/scaling/*.go` -- all 12 source files + 6 test files
- `internal/controller/nodepool/*.go` -- reconciler.go, reconciler_helpers.go, setup.go, provider_cache.go, doc.go
- `internal/controller/nodepool/lifecycle/manager.go` -- NodeHooks interface
- `internal/cloudprovider/interface.go` -- CloudProvider interface
- `internal/cloudprovider/types.go` -- InstanceCapacity, InstanceCapacityProvider
- `internal/cloudprovider/fake/provider.go` -- FakeProvider
- `internal/cloudprovider/aws/instance_types.go` -- InstanceCapacity alias

No external sources needed -- this is a codebase-internal architectural analysis.
