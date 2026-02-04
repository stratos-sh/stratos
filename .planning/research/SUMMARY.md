# Project Research Summary

**Project:** Stratos Kubernetes Operator -- v1.1.1 Naming & Dead Code Cleanup
**Domain:** Go refactoring (type renames, file renames, dead code removal) in a kubebuilder operator
**Researched:** 2026-02-04
**Confidence:** HIGH

## Executive Summary

This milestone is a pure refactoring pass: rename two types, rename two files, remove dead code, and replace one `interface{}` field with a concrete type. No new features, no new dependencies, no API surface changes. The Go ecosystem provides two purpose-built tools that handle the hard parts: `gopls rename -w` for type-safe symbol renaming (verified working on this codebase) and the `deadcode` tool for unreachable function detection (identified 9 dead functions, of which `UncordonNode` is in scope). The remaining operations -- file renames via `git mv` and the `Metadata` field type change -- are mechanical and verified by the compiler.

The recommended approach is a strict ordering: dead code removal first (smallest, independent), then type renames via `gopls rename` (while file paths are stable), then file renames via `git mv` (after gopls no longer needs the old paths), and finally the `ScalingDemand.Metadata` structural change. This ordering was validated by cross-referencing the architecture research (which mapped every cross-package reference) against the pitfalls research (which identified exactly where each rename can break). Two scope items from the original plan are explicitly rejected by research: renaming `types.go` to `scaling_types.go` and renaming `cloudprovider/types.go` to `instance_types.go` -- both violate established Go/K8s community conventions where `types.go` is the universal name for type-definition files within a package.

The primary risk is the `Strategy` to `Scaler` rename, which touches 11 consumer sites across 4 files in `controller/nodepool/`, plus 8 test struct literals in 3 test files, plus a compile-time interface assertion in `provider_cache.go:103`. All references have been enumerated. Using `gopls rename -w` eliminates the risk of missing code references; the remaining risk is stale comments (11 identified sites across 6 files) which must be updated manually. A pre-condition for starting work is a clean git working tree -- the current branch has uncommitted changes in files that overlap with the rename scope.

## Key Findings

### Recommended Stack

No new tools or dependencies are needed. The entire milestone uses tools already installed and verified on this codebase.

**Core tools:**
- `gopls rename -w` (v0.21.0): Type-safe symbol renaming -- handles `Strategy` to `Scaler` (8+ files) and `drainHelper` to `nodeDrainer` (3 files) atomically, updating all method receivers, constructor calls, and consumer sites
- `deadcode` (golang.org/x/tools): Dead code detection via Rapid Type Analysis -- found `UncordonNode` plus 8 other unreachable functions; only `UncordonNode` is in scope for deletion
- `git mv`: File renaming with history preservation -- Go does not encode file names in the type system, so file renames have zero compile impact
- `go build ./...` + `go vet ./...` + `make test`: Verification chain -- catches all type mismatches, shadow issues, and behavioral regressions

**What NOT to use:** `gorename` (deprecated), `gofmt -r` (syntactic not semantic), `sed`/regex (no type awareness), IDE refactoring (use CLI for reproducibility).

### Expected Features

**Must have (table stakes):**
- TS-1: Rename `Strategy` to `Scaler` -- the type is a concrete scaling coordinator, not a strategy pattern; `scaling.Scaler` is clear and non-redundant
- TS-2: Rename `drainHelper` to `nodeDrainer` -- `helper` is a code smell name; `nodeDrainer` is unambiguous
- TS-3: Rename `drainConfig` to `drainOptions` -- `*Options` is the K8s convention for configuration structs
- TS-4: Replace `ScalingDemand.Metadata interface{}` with `Pods []corev1.Pod` -- eliminates unsafe type assertion; exactly 2 call sites to update
- TS-5: Remove dead `UncordonNode` method -- zero callers, confirmed by grep and `deadcode` tool

**Should have (part of rename):**
- Rename `kubernetes.go` to `scaler.go` + `kubernetes_test.go` to `scaler_test.go` -- file name follows primary type
- Update `doc.go` references in both `scaling/` and `controller/nodepool/` -- 11 stale comment sites identified
- Rename `events.go` to `pod_events.go` (cosmetic, optional) -- file contains pod event handling, not event emission

**Defer / Do NOT do:**
- Do NOT rename `types.go` to `scaling_types.go` -- violates community convention; `types.go` is universal
- Do NOT rename `cloudprovider/types.go` to `instance_types.go` -- same convention violation, plus name collision with `aws/instance_types.go`
- Do NOT rename the `scaling` package itself, `ScalingDemand`, `ScaleCalculator`, or `ScaleDownCandidate` -- all follow correct conventions already
- Do NOT merge `drain.go` and `drain_eviction.go` -- the split mirrors Karpenter's pattern and aids readability

### Architecture Approach

The rename scope is confined to `internal/scaling/` (a leaf package) and its single consumer `internal/controller/nodepool/`. The dependency graph is a clean DAG with no circular dependencies. `internal/scaling/` is only imported by `controller/nodepool/`, making the blast radius of all renames contained to these two packages. The `cloudprovider/` package sits at the bottom of the DAG and is imported by many packages -- renaming anything there would have a very wide blast radius, which is why cloudprovider renames are explicitly out of scope.

**Blast radius categories:**
1. **Cross-package (HIGH):** `Strategy` to `Scaler` -- 11 consumer sites in `controller/nodepool/`, must be atomic
2. **Package-internal (ZERO cross-package):** `drainHelper`, `drainConfig` -- unexported, confined to `scaling/`
3. **File renames (ZERO compile impact):** `kubernetes.go` to `scaler.go` -- Go ignores file names
4. **Structural change (LOW):** `Metadata interface{}` to `Pods []corev1.Pod` -- 2 call sites, compiler-verified

### Critical Pitfalls

1. **Compile-time interface assertion in `provider_cache.go:103`** -- `var _ lifecycle.NodeHooks = (*scaling.Strategy)(nil)` must be updated in the same commit as the `Strategy` to `Scaler` rename. This non-obvious `var _` pattern is easy to miss in grep-based searches.

2. **Stale comments after type rename** -- 11 comment sites across 6 files reference `Strategy` by name. `gopls rename` does not touch comments. These must be updated manually in the same commit, found via `grep -rn "Strategy" internal/ --include="*.go" | grep -v NetworkReadinessStrategy`.

3. **Test struct literals break silently** -- 8 `&Strategy{...}` struct literals in 3 test files are not caught by `go build` (only by `go test`). Always run `go test` after each type rename, not just `go build`.

4. **Git blame destruction** -- renaming `kubernetes.go` to `scaler.go` AND changing its content (Strategy to Scaler) in the same commit destroys git blame. Split into two commits: file rename first, then content changes.

5. **Uncommitted branch changes overlap** -- `readiness_test.go` and `startup_taints_test.go` are modified on the current branch and will also be touched by the `Strategy` rename. Commit or stash current changes before starting.

## Implications for Roadmap

Based on research, the milestone should be structured as 5 phases executed in strict dependency order. All changes are independent enough for separate commits but must follow this sequence because `gopls rename` references files by path.

### Phase 1: Pre-work -- Clean Working Tree
**Rationale:** Pitfall 7 identified uncommitted changes in files that overlap with the rename scope. Starting with a dirty tree risks merge conflicts or accidental reverts.
**Delivers:** Clean git state, ready for refactoring.
**Addresses:** Pitfall 7 (uncommitted branch changes).
**Avoids:** Merge conflicts, accidental reverts of enum changes.

### Phase 2: Dead Code Removal
**Rationale:** Smallest, most independent change. Removes `UncordonNode` (18 lines) which has zero callers. Does this first so subsequent renames operate on a cleaner codebase.
**Delivers:** Removal of `UncordonNode` method from `drain.go`.
**Addresses:** TS-5 (dead code removal).
**Avoids:** Pitfall 6 (confusing `UncordonNode` with `CordonNode` -- verify `CordonNode` HAS callers before deleting).
**Tools:** Manual deletion, verified by `grep` and `deadcode -test`.

### Phase 3: Type Renames (via `gopls rename -w`)
**Rationale:** Must happen BEFORE file renames because `gopls rename` references files by their current path. This is the highest-value and highest-risk phase -- the `Strategy` to `Scaler` rename touches 8+ files and has the compile-time assertion trap.
**Delivers:** `Strategy` renamed to `Scaler`, `drainHelper` to `nodeDrainer`, `drainConfig` to `drainOptions`. All method receivers, constructors, consumer sites, test struct literals, and doc comments updated.
**Addresses:** TS-1, TS-2, TS-3, D-4 (test file rename), D-5 (doc.go updates).
**Avoids:** Pitfall 1 (compile-time assertion), Pitfall 3 (stale comments), Pitfall 5 (test struct literals), Pitfall 10 (split across drain files).
**Tools:** `gopls rename -w` for code, manual grep for comments, `go test` for verification.

### Phase 4: File Renames (via `git mv`)
**Rationale:** Must happen AFTER type renames so `gopls` can find files at their original paths. File renames have zero compile impact, so this is low risk. Separated from Phase 3 to preserve git blame (Pitfall 4).
**Delivers:** `kubernetes.go` renamed to `scaler.go`, `kubernetes_test.go` to `scaler_test.go`. Optionally `events.go` to `pod_events.go`.
**Addresses:** File rename items from TS-1.
**Avoids:** Pitfall 4 (blame destruction -- file rename in its own commit, content changes already done in Phase 3).
**Tools:** `git mv`.

### Phase 5: Struct Field Type Change
**Rationale:** Independent of all renames but logically last -- naming is settled, now improve type safety. Changes `ScalingDemand.Metadata interface{}` to `Pods []corev1.Pod`, eliminating a type assertion.
**Delivers:** Type-safe pod field on `ScalingDemand`. Removes the `demand.Metadata.([]corev1.Pod)` assertion at `scaling.go:138`.
**Addresses:** TS-4.
**Avoids:** Pitfall 2 (this is a structural change, not just a rename -- the type assertion must be replaced with direct field access, not just renamed).
**Tools:** Manual edit + `go build ./...` to find all affected sites (exactly 2).

### Phase Ordering Rationale

- **Dead code before renames:** Removing `UncordonNode` first means the `drainHelper` to `nodeDrainer` rename in Phase 3 operates on a smaller type (one fewer method). No dependency, but cleaner.
- **Type renames before file renames:** `gopls rename -w` requires stable file paths. If `kubernetes.go` is renamed to `scaler.go` first, the `gopls rename` command would need to reference the new path. Doing type renames first avoids this coordination.
- **File renames after content changes:** Git detects renames via content similarity. If the file is renamed in a separate commit from content changes, `git log --follow` works correctly. Combining them risks losing blame history.
- **Struct field change last:** This is the only semantic change (all others are mechanical renames). Doing it last keeps the rename phases pure and makes the structural change easy to review in isolation.

### Research Flags

Phases with standard patterns (skip deeper research):
- **All phases:** Every reference site has been enumerated in ARCHITECTURE.md. The exact line numbers for every consumer of `Strategy`, `drainHelper`, `drainConfig`, and `ScalingDemand.Metadata` are documented. No phase needs further research.

No phases need `/gsd:research-phase` -- the research is exhaustive for this scope.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Tools verified on this exact codebase; `gopls rename -d` dry-run confirmed; `deadcode` output confirmed |
| Features | HIGH | All renames validated against Karpenter, cluster-autoscaler, controller-runtime, kubectl/drain conventions |
| Architecture | HIGH | Every cross-package reference enumerated by reading source code; dependency graph is a clean DAG |
| Pitfalls | HIGH | All pitfall locations verified by direct codebase analysis; line numbers confirmed |

**Overall confidence:** HIGH

### Gaps to Address

- **Uncommitted branch changes:** The current branch has modified files that overlap with the rename scope. This must be resolved (commit or stash) before starting Phase 1. Not a research gap, but a prerequisite.
- **Optional renames not decided:** `events.go` to `pod_events.go` is marked optional/cosmetic. The roadmapper should include it or exclude it -- do not leave it ambiguous during execution.
- **Additional dead code beyond UncordonNode:** The `deadcode` tool found 8 other unreachable functions (FakeResolver methods, `IsKnownInstanceType`, `RecordStartingNodes`). These are out of scope for v1.1.1 but should be tracked for a future cleanup pass. Some may be intentionally retained for future use or test infrastructure.

## Sources

### Primary (HIGH confidence)
- gopls rename documentation: [Gopls Code Transformation Features](https://go.dev/gopls/features/transformation)
- deadcode tool: [Finding unreachable functions with deadcode](https://go.dev/blog/deadcode) (official Go blog)
- Karpenter source: [sigs.k8s.io/karpenter](https://github.com/kubernetes-sigs/karpenter) -- naming conventions, file structure
- cluster-autoscaler source: [k8s.io/autoscaler](https://github.com/kubernetes/autoscaler) -- type naming patterns
- kubectl drain: [pkg/drain/drain.go](https://github.com/kubernetes/kubectl/blob/master/pkg/drain/drain.go) -- `Helper` struct naming
- Direct codebase analysis: All files in `internal/scaling/`, `internal/controller/nodepool/`, `internal/cloudprovider/` read and cross-referenced

### Secondary (MEDIUM confidence)
- golangci-lint unused vs deadcode discussion: [Discussion #4819](https://github.com/golangci/golangci-lint/discussions/4819)
- Git rename detection behavior: verified heuristic threshold and `--follow` behavior

### Tertiary (LOW confidence)
- None -- all findings verified against primary sources or direct codebase analysis

---
*Research completed: 2026-02-04*
*Ready for roadmap: yes*
