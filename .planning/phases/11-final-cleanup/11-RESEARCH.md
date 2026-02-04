# Phase 11: Final Cleanup and Verification - Research

**Researched:** 2026-02-03
**Domain:** Go project cleanup (RBAC markers, linter config, dead code, dependency hygiene)
**Confidence:** HIGH

## Summary

Phase 11 is a cleanup and verification phase with well-scoped, mechanical changes. The codebase is already in good shape after phases 7-10 removed the strategy package hierarchy -- most residual references are limited to doc comments and one RBAC marker. The required changes are small and low-risk.

Three code changes are needed: (1) remove one RBAC marker line, (2) remove the unused `recorder` field from `drainHelper` in `drain.go`, and (3) update two stale doc.go comments. The `.golangci.yml` depguard rules already contain no `strategy/` references -- that success criterion is already met. `go.mod` and `go.sum` already contain no GitHub API client dependencies -- that criterion is also already met. The phase concludes with full test/lint verification.

**Primary recommendation:** Execute the three targeted code edits, run `go mod tidy` as a safety net, then verify with `make test`, `make test-integration`, and `make lint`.

## Standard Stack

Not applicable -- this phase uses only existing project tooling (Go, Make, golangci-lint). No new libraries or tools are introduced.

### Relevant Tooling
| Tool | Version | Purpose | Invocation |
|------|---------|---------|------------|
| Go | 1.25.5 | Build, test, `go mod tidy` | `go mod tidy` |
| golangci-lint | v2.8.0 | Linting | `make lint` |
| controller-gen | v0.16.5 | CRD/RBAC generation | `make manifests` (CRD only in this project) |
| envtest | release-0.17 | Integration test K8s API | `make test-integration` |

## Architecture Patterns

### Current File Layout (Affected Files)

```
internal/controller/nodepool/reconciler.go     # Line 84: RBAC secrets marker to remove
internal/controller/nodepool/nodestate/doc.go  # Lines 40-41: stale strategy/ reference
internal/metrics/doc.go                        # Line 34: stale strategy/kubernetes reference
internal/scaling/drain.go                      # Lines 25,33,65,71: dead recorder field+import
internal/scaling/kubernetes.go                 # Line 77: call site that passes recorder
.golangci.yml                                  # Already clean of strategy/ refs
go.mod / go.sum                                # Already clean of GitHub API deps
```

### Pattern: kubebuilder RBAC Markers

RBAC markers live as Go comments above the `Reconcile()` method in `reconciler.go`. Each `// +kubebuilder:rbac:...` line declares a ClusterRole permission. In this project, `make manifests` only generates CRD YAML (not RBAC YAML) -- the Helm chart `clusterrole.yaml` is manually authored and already omits secrets access. Removing the marker is purely a source-level hygiene fix.

**Current markers (reconciler.go lines 73-86):**
```go
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools/finalizers,verbs=update
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch          <-- REMOVE THIS
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
```

**Audit result:** All other markers are actively used:
- nodepools, awsnodeclasses: CRD CRUD by both reconcilers
- nodes, nodes/status: lifecycle management, state labels
- pods: pod-demand scanning, drain
- pods/eviction: drain eviction API
- events: event recording
- poddisruptionbudgets: drain PDB respect
- leases: leader election (controller-runtime standard)

**Recommendation:** Remove only the secrets line (line 84). No broader RBAC cleanup needed.

### Pattern: drainHelper Dead Code

The `drainHelper` struct in `drain.go` has a `recorder` field that is stored but never accessed by any of its methods. All `drainHelper` methods (`CordonNode`, `UncordonNode`, `DrainNode`, `getPodsOnNode`, `filterPodsToEvict`, `evictPod`, `waitForPodsDeletion`, `waitForPodDeletion`, `hasLocalStorage`) use only `d.client` and the config fields -- none reference `d.recorder`.

**Important distinction:** The `Strategy` struct in `kubernetes.go` also has a `recorder` field -- that one IS actively used in `readiness.go` (lines 169-170, 220-221) for event recording. Only `drainHelper.recorder` is dead.

**Change scope:**
1. Remove `recorder events.EventRecorder` field from `drainHelper` struct (drain.go line 33)
2. Remove `recorder events.EventRecorder` parameter from `newDrainHelper` function (drain.go line 65)
3. Remove `recorder: recorder,` assignment in `newDrainHelper` body (drain.go line 71)
4. Remove `"k8s.io/client-go/tools/events"` import from drain.go (line 25) -- no longer needed after field removal
5. Update call site in `kubernetes.go` line 77: `newDrainHelper(s.client, s.recorder, drainCfg)` becomes `newDrainHelper(s.client, drainCfg)`

**No test changes needed:** There are no direct tests of `newDrainHelper` -- it is only called from `Strategy.DrainAndStop`.

### Pattern: Stale Doc Comments

Two doc.go files reference the deleted strategy package hierarchy:

1. **`internal/controller/nodepool/nodestate/doc.go`** (lines 40-41):
   ```
   // This is a pure leaf package with zero internal dependencies. It is
   // imported by lifecycle, nodepool, strategy/kubernetes, and
   // strategy/githubactions.
   ```
   Should become: `imported by lifecycle, nodepool, and scaling.`

2. **`internal/metrics/doc.go`** (line 34):
   ```
   // This is a leaf package with no internal dependencies. It is imported by
   // the aws, lifecycle, nodepool, and strategy/kubernetes packages.
   ```
   Should become: `imported by the aws, lifecycle, nodepool, and scaling packages.`

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Dependency cleanup | Manual go.sum editing | `go mod tidy` | Handles transitive dep graph correctly |
| RBAC manifest sync | Manual YAML editing | Remove marker + note Helm is manual | The Helm chart is already correct; marker is just source hygiene |

## Common Pitfalls

### Pitfall 1: Removing Wrong Recorder
**What goes wrong:** Accidentally removing the `recorder` field from `Strategy` (kubernetes.go) instead of from `drainHelper` (drain.go). The `Strategy.recorder` is actively used for event recording in readiness.go.
**Why it happens:** Both structs are in the same package and both have a field named `recorder`.
**How to avoid:** Only modify `drain.go` and the `newDrainHelper` call site in `kubernetes.go` line 77. Do NOT touch the `Strategy` struct definition or `New()` constructor.
**Warning signs:** If readiness.go or startup_taints_test.go fail to compile after the change, you removed the wrong recorder.

### Pitfall 2: Stale events Import in drain.go
**What goes wrong:** After removing the `recorder` field and parameter from `drainHelper`/`newDrainHelper`, the `"k8s.io/client-go/tools/events"` import in drain.go becomes unused and causes a compile error.
**Why it happens:** The import was only needed for the `events.EventRecorder` type.
**How to avoid:** Remove the import line along with the field and parameter.
**Warning signs:** `imported and not used` compilation error.

### Pitfall 3: depguard False Positive
**What goes wrong:** Assuming the `.golangci.yml` needs strategy/ cleanup when it doesn't.
**Why it happens:** The CONTEXT.md and requirements mention depguard cleanup, but research shows the depguard rules were already cleaned in a prior phase.
**How to avoid:** The current depguard rules only deny `internal/cloudprovider/aws` and `internal/cloudprovider/fake` imports from controller sub-packages -- no strategy/ references exist. No depguard changes are needed.
**Warning signs:** Making unnecessary edits to working config.

### Pitfall 4: go mod tidy No-Op
**What goes wrong:** Expecting `go mod tidy` to remove GitHub API dependencies when they were already removed in prior phases.
**Why it happens:** The requirements list `CLN-02: Run go mod tidy to remove unused GitHub API dependencies` but research shows go.mod/go.sum already contain no go-github, ghinstallation, or similar packages.
**How to avoid:** Still run `go mod tidy` as a safety net, but expect it to be a no-op or minimal change. The success criterion (`go.sum contains no GitHub API client dependencies`) is already met.

## Code Examples

### RBAC Marker Removal (reconciler.go)
```go
// Before (line 84):
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// After: line deleted entirely
```

### drainHelper Recorder Removal (drain.go)
```go
// Before:
type drainHelper struct {
	client                   client.Client
	recorder                 events.EventRecorder  // <-- remove
	gracePeriodSeconds       int64
	// ...
}

func newDrainHelper(c client.Client, recorder events.EventRecorder, config *drainConfig) *drainHelper {
	// ...
	return &drainHelper{
		client:                   c,
		recorder:                 recorder,  // <-- remove
		// ...
	}
}

// After:
type drainHelper struct {
	client                   client.Client
	gracePeriodSeconds       int64
	// ...
}

func newDrainHelper(c client.Client, config *drainConfig) *drainHelper {
	// ...
	return &drainHelper{
		client:                   c,
		// ...
	}
}
```

### Call Site Update (kubernetes.go line 77)
```go
// Before:
drainHelper := newDrainHelper(s.client, s.recorder, drainCfg)

// After:
drainHelper := newDrainHelper(s.client, drainCfg)
```

### Doc Comment Updates
```go
// nodestate/doc.go -- update lines 39-41:
// This is a pure leaf package with zero internal dependencies. It is
// imported by lifecycle, nodepool, and scaling.

// metrics/doc.go -- update line 33-34:
// This is a leaf package with no internal dependencies. It is imported by
// the aws, lifecycle, nodepool, and scaling packages.
```

## State of the Art

| Old State | Current State | When Changed | Impact |
|-----------|---------------|--------------|--------|
| strategy/ package hierarchy | internal/scaling/ flat package | Phase 7-9 (this milestone) | doc comments still reference old paths |
| GitHub Actions scaling strategy | Removed entirely | Phase 9 (delete old packages) | RBAC secrets marker is vestigial |
| depguard guarded strategy/ boundaries | depguard rules already updated | Phase 9 or earlier | No depguard changes needed |
| GitHub API deps in go.mod | Already removed | Phase 9 | go mod tidy is a no-op for this |

## Open Questions

1. **Broader linter config cleanup**
   - CONTEXT.md says: "Scan the entire .golangci.yml for any clearly dead config"
   - Finding: The current .golangci.yml is clean. The depguard rule `controller-sub-no-impl` references existing packages (lifecycle, nodestate, nodeclass). The exclusion patterns are standard. `contextcheck` appears in both enabled linters and test exclusions, which is correct.
   - Recommendation: No additional linter config changes beyond what's already clean. Document this finding in the plan as "verified clean."

2. **Commit organization (Claude's Discretion)**
   - Recommendation: Two commits -- (1) all code cleanup together (RBAC marker + drain.go dead code + doc comments), (2) verification commit is unnecessary since verification doesn't produce artifacts. The changes are small enough to group into one logical commit ("cleanup: remove residual v1.1 migration artifacts"). Separating RBAC from dead code would be over-granular for this scale.

3. **Dependency tidying approach (Claude's Discretion)**
   - Recommendation: Run `go mod tidy` after code changes. If it produces no diff, that's expected and fine. No need for a broader indirect dep audit -- the deps are all standard K8s/AWS ecosystem libraries.

4. **Verification depth (Claude's Discretion)**
   - Recommendation: Run `make test`, `make test-integration`, `make lint` as required. Additionally, run `go build ./...` to catch any compilation issues before the full test suite. No additional verification beyond the required three commands is needed for these mechanical changes.

## Sources

### Primary (HIGH confidence)
- Direct codebase analysis via Read/Grep of all affected files
- `/home/roeeh/projects/presto/.golangci.yml` -- full depguard rules examined
- `/home/roeeh/projects/presto/internal/controller/nodepool/reconciler.go` -- all RBAC markers examined
- `/home/roeeh/projects/presto/internal/scaling/drain.go` -- dead recorder field confirmed
- `/home/roeeh/projects/presto/internal/scaling/drain_eviction.go` -- all drainHelper methods verified (no recorder usage)
- `/home/roeeh/projects/presto/internal/scaling/readiness.go` -- Strategy.recorder IS used (lines 169-170, 220-221)
- `/home/roeeh/projects/presto/internal/scaling/kubernetes.go` -- single call site for newDrainHelper
- `/home/roeeh/projects/presto/go.mod` and `go.sum` -- no GitHub API client deps present
- `/home/roeeh/projects/presto/deploy/charts/stratos/templates/clusterrole.yaml` -- Helm chart already omits secrets

### Secondary (MEDIUM confidence)
- Context7 `/websites/golangci-lint_run` -- golangci-lint v2 config format and depguard rule structure

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- RBAC cleanup: HIGH -- single line removal, all other markers verified against actual code usage
- Dead code removal: HIGH -- exhaustive grep confirms drainHelper.recorder never read; Strategy.recorder correctly preserved
- Linter config: HIGH -- .golangci.yml contains no strategy/ references; depguard rules verified against existing packages
- Dependency tidying: HIGH -- go.mod/go.sum already clean of GitHub API deps
- Doc comment fixes: HIGH -- two specific files with exact line numbers identified

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (stable -- cleanup of already-complete migration)
