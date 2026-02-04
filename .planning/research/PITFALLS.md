# Domain Pitfalls: Naming Cleanup & Dead Code Removal

**Domain:** Go type/file rename and dead code removal in a kubebuilder project
**Project:** Stratos v1.1.1
**Researched:** 2026-02-04
**Confidence:** HIGH (based on direct codebase analysis of all affected files, verified Go patterns)

---

## Critical Pitfalls

Mistakes that cause build failures, test failures, or silent runtime breakage.

---

### Pitfall 1: Compile-Time Interface Assertion Breaks on Type Rename

**What goes wrong:** The project has a compile-time assertion in `internal/controller/nodepool/provider_cache.go:103`:

```go
var _ lifecycle.NodeHooks = (*scaling.Strategy)(nil)
```

Renaming `Strategy` to `Scaler` without updating this line causes a build failure. This pattern is easy to miss because it is not a function call, a struct field, or a method -- it is a bare `var _` declaration that only exists for compile-time verification.

**Why it happens:** Grep for the old type name typically catches function signatures and struct fields. Compile-time interface assertions (`var _ InterfaceName = (*TypeName)(nil)`) use the type in a non-obvious way that can be skipped during manual search.

**Consequences:** Build failure with an error like `cannot use (*Strategy)(nil) as lifecycle.NodeHooks` (referencing a type that no longer exists), or if the assertion is updated but the interface method set no longer matches, silent removal of the compile-time safety check.

**Prevention:**
1. After renaming `Strategy` to `Scaler`, run `go build ./...` immediately before touching anything else.
2. Explicitly search for `*scaling.Strategy` and `scaling.Strategy` as separate patterns -- the pointer-to-nil assertion uses the pointer form.
3. Confirmed location: `provider_cache.go:103`.

**Detection:** `go build ./...` fails. Caught early if builds are run after each rename.

**Phase:** Must be handled in the same commit as the `Strategy` to `Scaler` rename.

---

### Pitfall 2: Metadata Field Change from interface{} to []corev1.Pod Breaks Type Assertions at Runtime

**What goes wrong:** `ScalingDemand.Metadata` is currently `interface{}`. The plan changes it to a typed `Pods []corev1.Pod` field. There is an existing type assertion in `scaling.go:138`:

```go
pods, ok := demand.Metadata.([]corev1.Pod)
```

This is a **structural change, not just a rename**. Changing from `interface{}` to a concrete typed field means:
- All write sites (`Metadata: unassignedPods` at `scaling.go:95`) must change to `Pods: unassignedPods`.
- All read sites (the type assertion at `scaling.go:138`) must change to direct field access (`demand.Pods`).
- The type assertion `demand.Metadata.([]corev1.Pod)` must be completely removed, not just renamed.

**Why it happens:** The developer treats this as a simple rename (`Metadata` to `Pods`) and misses that the entire access pattern changes from type-assertion to direct field access.

**Consequences:** The compiler catches the structural mismatch (good). But if the change is done partially -- e.g., the field is renamed but the old `interface{}` type is kept somewhere -- callers may compile but break at runtime. The Go `unused` linter will NOT catch leftover type assertion code because it involves used variables -- only `go build` catches the compile error.

**Prevention:**
1. Audit all sites before changing. `Metadata` is written at exactly 1 location (`scaling.go:95`) and read with a type assertion at exactly 1 location (`scaling.go:138`). Total: 2 lines to change.
2. After the change, verify zero remaining `interface{}` references for this field.
3. Run tests: `go test ./internal/scaling/... ./internal/controller/nodepool/...`

**Detection:** `go build ./...` catches mismatched field access. `go vet ./...` catches some interface assertion issues.

**Phase:** Handle as a standalone commit, separate from the `Strategy` to `Scaler` rename, because this changes behavior not just naming.

---

### Pitfall 3: doc.go and Inline Comments Mention Old Type Names After Rename

**What goes wrong:** After renaming `Strategy` to `Scaler`, the codebase still contains the old name in documentation comments that the compiler does not check. These stale references confuse future developers.

Confirmed stale reference locations (11 sites across 6 files):
- `internal/scaling/doc.go:23` -- `Strategy: core scaling logic`
- `internal/scaling/kubernetes.go:36` -- `Strategy scales NodePool nodes`
- `internal/scaling/kubernetes.go:47` -- `New creates a new Strategy.`
- `internal/controller/nodepool/doc.go:27` -- `demand checking (via scaling.Strategy)`
- `internal/controller/nodepool/doc.go:35` -- `to scaling.Strategy for demand calculation`
- `internal/controller/nodepool/reconciler.go:69` -- `scaler is the single scaling strategy for all pools`
- `internal/controller/nodepool/reconciler_helpers.go:59` -- `start standby nodes and notify strategy`
- `internal/controller/nodepool/reconciler_helpers.go:75` -- `runs cloud sync, warmup monitoring, and strategy maintenance`
- `internal/controller/nodepool/reconciler_helpers.go:93` -- `Strategy maintenance (startup taints...)`
- `internal/controller/nodepool/provider_cache.go:102` -- `Compile-time assertion that scaling.Strategy implements`
- `internal/controller/nodepool/lifecycle/manager.go:41` -- `The scaling.Strategy implements this interface.`

**Why it happens:** The Go compiler does not validate comment content. Tools like `gopls rename` update code references but leave comments untouched. Developers run `go build` and `go test`, see green, and assume the rename is complete.

**Consequences:** Incorrect documentation misleads future contributors. In a kubebuilder project where doc comments describe the reconciliation flow, stale type names create confusion about architecture.

**Prevention:**
1. After every type rename, grep for the old name across ALL file types (case-sensitive): `grep -rn "Strategy" internal/ --include="*.go" | grep -v NetworkReadinessStrategy`.
2. Check `doc.go` files explicitly -- there are 11 doc.go files in this project.
3. Update comments in the same commit as the code rename.
4. The `misspell` linter will NOT catch this; it only checks spelling, not semantic accuracy.

**Detection:** Manual grep. No automated tool catches stale type names in comments.

**Phase:** Must be part of each rename commit -- not deferred to a separate "fix comments" phase.

---

## Moderate Pitfalls

Mistakes that cause test failures, degraded git history, or technical debt.

---

### Pitfall 4: Git Blame Destroyed by Combining File Rename with Content Changes

**What goes wrong:** When renaming `kubernetes.go` to `scaler.go` and simultaneously renaming the `Strategy` type inside it, `git blame` loses the line-level history. Git's rename detection relies on content similarity (default threshold ~50%). If both the filename and significant content change in the same commit, Git treats it as a file deletion + file creation.

This project has 4 planned file renames:
- `kubernetes.go` to `scaler.go` (HIGH RISK -- file also contains the `Strategy` struct being renamed to `Scaler`)
- `types.go` to `scaling_types.go` (LOW RISK -- no content changes planned)
- `events.go` to `pod_events.go` (LOW RISK -- no content changes planned)
- `cloudprovider/types.go` to `instance_types.go` (LOW RISK -- no content changes planned)

The first rename is highest risk because the file also contains the type being renamed.

**Why it happens:** The developer does all changes in one commit for "atomic" correctness. Git's content-similarity heuristic then fails to detect the rename.

**Prevention:**
1. Use `git mv` for file renames (ensures clean staging).
2. For `kubernetes.go` to `scaler.go`, split into two commits:
   - Commit 1: `git mv internal/scaling/kubernetes.go internal/scaling/scaler.go` (pure rename, no content change)
   - Commit 2: Rename `Strategy` to `Scaler` inside `scaler.go` and all referencing files
3. For the 3 files that are rename-only (types.go, events.go, cloudprovider/types.go), a single commit per rename is fine because content does not change.
4. Verify with `git log --follow --diff-filter=R -- internal/scaling/scaler.go` after each rename commit.

**Detection:** `git log --follow <newfile>` should show history before the rename. If it does not, blame was lost.

**Phase:** File renames should be their own phase/commits, executed before type renames that change content inside those files.

---

### Pitfall 5: Test Files Reference Old Type Name in Struct Literals

**What goes wrong:** Test files create `Strategy` struct literals directly. After renaming to `Scaler`, all test struct literals break. The project has 8 struct literal references across 3 test files:

- `internal/scaling/readiness_test.go` -- 3 sites (lines 98, 140, 186): `&Strategy{client: fakeClient}`
- `internal/scaling/maintenance_test.go` -- 2 sites (lines 52, 109): `&Strategy{...}`
- `internal/scaling/startup_taints_test.go` -- 3 sites (lines 64, 124, 186): `&Strategy{...}`

Total: 8 struct literal references in test code.

**Why it happens:** The developer renames the type in production code and the `New()` constructor, runs `go build`, sees success, and forgets that test files in the same package directly instantiate the struct. `go build` does NOT compile test files.

**Consequences:** `go test ./internal/scaling/...` fails with `undefined: Strategy`. Build succeeds because test files are only compiled during test runs.

**Prevention:**
1. Always run `go test ./internal/scaling/...` immediately after renaming the type, not just `go build`.
2. Better: run `go vet ./...` which processes test files too.
3. Best: use the full test suite (`make test`) which catches all test compilation failures.

**Detection:** `go test` fails at compilation. `go build` alone does NOT catch this.

**Phase:** Test file updates must be in the same commit as the type rename.

---

### Pitfall 6: Removing UncordonNode() -- Confusing It with CordonNode()

**What goes wrong:** `drainHelper` has three public methods: `CordonNode`, `UncordonNode`, and `DrainNode`. Only `UncordonNode` is dead code (zero callers -- confirmed). `CordonNode` IS called by `DrainNode` at `drain.go:127`. Removing `CordonNode` instead of `UncordonNode` by mistake breaks the drain flow.

**Why it happens:** The method names are very similar (`CordonNode` vs `UncordonNode`). During cleanup, the developer may confuse which one is dead code.

**Consequences:** If `CordonNode` is accidentally removed, `DrainNode` fails at compile time (good -- caught immediately). But if the developer then "fixes" it by removing the `CordonNode` call from `DrainNode`, the drain operation silently stops cordoning nodes before eviction -- a correctness bug that only manifests in production.

**Prevention:**
1. Before removing any method, verify zero callers: `grep -rn "UncordonNode" --include="*.go"` -- must show ONLY the definition (2 lines: comment at drain.go:98 and func at drain.go:99).
2. Compare with `grep -rn "CordonNode" --include="*.go"` -- must show the definition AND the call in `DrainNode` at drain.go:127.
3. Run `make test` and `make test-integration` after removal.

**Detection:** `go build ./...` catches accidental removal of called methods. Integration tests catch semantic regressions.

**Phase:** Dead code removal should be its own commit, after all renames are complete.

---

### Pitfall 7: Uncommitted Branch Changes Conflict with Cleanup Work

**What goes wrong:** The current git status shows modified files that overlap with files the cleanup will touch:
- `api/v1alpha1/aws_nodeclass_types.go` (modified)
- `api/v1alpha1/config_types.go` (modified)
- `internal/scaling/readiness_test.go` (modified)
- `internal/scaling/startup_taints_test.go` (modified)
- `internal/controller/nodepool/lifecycle/warmup_test.go` (modified)

These are uncommitted changes from the `feat/network-readiness-strategy-enum` branch. If the cleanup milestone starts without committing these changes first, there is a risk of merge conflicts or accidentally reverting the enum changes while doing the rename.

**Why it happens:** The cleanup milestone starts on a branch that already has uncommitted modifications to the same files that the cleanup will touch.

**Consequences:** Merge conflicts when rebasing or merging. Worse: accidentally reverting the enum changes while doing the rename.

**Prevention:**
1. Start the cleanup milestone from a clean commit -- either merge or stash the current branch changes first.
2. If working on the same branch, commit all pending changes before starting renames.
3. Use `git stash` or create a fresh branch from the latest committed state.

**Detection:** `git status` shows modified files before cleanup begins. If there are uncommitted changes in files that will be touched, stop and resolve first.

**Phase:** Pre-work: ensure clean working tree before starting any renames.

---

## Minor Pitfalls

Mistakes that cause annoyance or minor tech debt but are easily fixable.

---

### Pitfall 8: Constructor Comment Drift After Type Rename

**What goes wrong:** The constructor `New()` in `kubernetes.go:48` returns `*Strategy`. After renaming the type to `Scaler`, the function signature changes to return `*Scaler`. The function name `New()` itself is fine (Go convention: `scaling.New()` is idiomatic for a package-level constructor). However, the comment `// New creates a new Strategy.` must be updated to `// New creates a new Scaler.`

**Why it happens:** The function name `New` is generic and does not encode the type name, so developers focus on updating the return type but skip the comment.

**Prevention:** Include comment updates in the type rename grep pass.

**Detection:** `grep -rn "Strategy" --include="*.go"` in the rename verification step catches this.

**Phase:** Same commit as the `Strategy` to `Scaler` rename.

---

### Pitfall 9: File Rename cloudprovider/types.go to instance_types.go Creates Name Collision

**What goes wrong:** The plan renames `cloudprovider/types.go` to `cloudprovider/instance_types.go`. But `internal/cloudprovider/aws/instance_types.go` already exists. While they are in different packages (no Go compilation issue), having two files named `instance_types.go` in a parent and child directory creates confusion:

- `internal/cloudprovider/instance_types.go` (the renamed file)
- `internal/cloudprovider/aws/instance_types.go` (already exists)

**Why it happens:** The developer picks a descriptive name without checking sibling directories.

**Consequences:** No build issue, but developer confusion when navigating: "Which `instance_types.go` am I editing?" Tab completion and fuzzy finders show two identical filenames.

**Prevention:** Before renaming, check if the target filename exists in related directories. Consider alternative names:
- `provider_types.go` -- describes the provider-level abstractions (Instance, InstanceState, error types, InstanceCapacity)
- `cloud_types.go` -- distinguishes from aws-specific types

**Detection:** Search for the target filename before renaming: glob for `**/instance_types.go` in the cloudprovider tree.

**Phase:** Decide on final names during planning, before executing renames.

---

### Pitfall 10: Unexported Type Renames Split Across Two Files

**What goes wrong:** `drainHelper` is defined in `drain.go` but has 5 additional method definitions in `drain_eviction.go`:
- `drain_eviction.go:33` -- `func (d *drainHelper) getPodsOnNode`
- `drain_eviction.go:55` -- `func (d *drainHelper) filterPodsToEvict`
- `drain_eviction.go:86` -- `func (d *drainHelper) hasLocalStorage`
- `drain_eviction.go:96` -- `func (d *drainHelper) evictPod`
- `drain_eviction.go:130` -- `func (d *drainHelper) waitForPodsDeletion`
- `drain_eviction.go:143` -- `func (d *drainHelper) waitForPodDeletion`

Renaming `drainHelper` to `nodeDrainer` requires updating both files. Similarly, `drainConfig` is defined in `drain.go` and referenced only in `drain.go` and `kubernetes.go`.

**Why it happens:** The developer renames in `drain.go` but forgets `drain_eviction.go` has method definitions on the same type.

**Consequences:** Build failure -- caught immediately by `go build`.

**Prevention:**
1. Grep for the full type name before renaming: `grep -rn "drainHelper" --include="*.go"` shows all 15 references across both files.
2. Similarly, `grep -rn "drainConfig" --include="*.go"` shows 5 references in `drain.go` and 1 in `kubernetes.go`.
3. The receiver variable `d` does NOT need to change -- Go convention allows keeping short receiver names.

**Detection:** `go build ./...` fails immediately.

**Phase:** Same commit as other renames in the drain file family.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Pre-work: clean working tree | Pitfall 7: uncommitted changes in files being renamed | `git status` check, commit or stash all changes first |
| File renames (git mv) | Pitfall 4: blame loss from combined rename+edit | Separate file rename commits from content change commits; use `git mv` |
| File renames (git mv) | Pitfall 9: name collision cloudprovider/instance_types.go | Choose alternative name like `provider_types.go` |
| Strategy to Scaler rename | Pitfall 1: compile-time assertion in provider_cache.go:103 | Grep for `*scaling.Strategy` specifically |
| Strategy to Scaler rename | Pitfall 3: 11 stale doc.go/comment references across 6 files | Grep for old name across all .go files including comments |
| Strategy to Scaler rename | Pitfall 5: 8 test struct literals across 3 test files | Run `go test` not just `go build` |
| Strategy to Scaler rename | Pitfall 8: constructor comment says "new Strategy" | Update in same grep pass |
| Metadata to Pods field change | Pitfall 2: type assertion pattern must become direct field access | Audit exactly 2 reference sites; this is structural, not just a rename |
| UncordonNode removal | Pitfall 6: confusion with CordonNode | Verify zero callers before deletion; confirm CordonNode has callers |
| drainHelper/drainConfig renames | Pitfall 10: references split across drain.go and drain_eviction.go | Grep shows all 15+ references; update together |

---

## Verification Checklist

Run after ALL renames and removals are complete:

```bash
# 1. Build (catches type errors, missing references)
go build ./...

# 2. Vet (catches issues in test files too)
go vet ./...

# 3. Unit tests (catches test compilation + behavior)
make test

# 4. Integration tests (catches end-to-end regressions)
make test-integration

# 5. Lint (catches unused code, style issues)
make lint

# 6. Grep for old names (catches stale comments)
# Strategy -- exclude NetworkReadinessStrategy which is a different type
grep -rn "Strategy" internal/ --include="*.go" | grep -v NetworkReadinessStrategy | grep -v _test.go
# Also check test files separately
grep -rn "\bStrategy\b" internal/scaling/ --include="*_test.go" | grep -v NetworkReadinessStrategy

# Unexported renames
grep -rn "drainHelper" internal/ --include="*.go"
grep -rn "drainConfig" internal/ --include="*.go"

# Dead code removal
grep -rn "UncordonNode" internal/ --include="*.go"

# Metadata field
grep -rn "\.Metadata" internal/scaling/ --include="*.go"

# 7. Git rename detection (verify blame preserved)
git log --follow --diff-filter=R -- internal/scaling/scaler.go
git log --follow --diff-filter=R -- internal/scaling/scaling_types.go
git log --follow --diff-filter=R -- internal/scaling/pod_events.go
git log --follow --diff-filter=R -- internal/cloudprovider/instance_types.go
```

---

## Sources

- Direct codebase analysis of all affected files in `/home/roeeh/projects/presto/` (HIGH confidence -- primary source for all pitfall locations and reference counts)
- [Git Move Files: Renames and History Preservation](https://thelinuxcode.com/git-move-files-practical-renames-refactors-and-history-preservation-in-2026/)
- [Renaming files in Git: the right way](https://jtemporal.com/renaming-files-in-git-the-right-way/)
- [golangci-lint unused linter and exported functions](https://github.com/golangci/golangci-lint/discussions/4819)
- [Kubebuilder: Having issues while renaming API type](https://github.com/kubernetes-sigs/kubebuilder/issues/443)
- [Go Type Assertions - YourBasic](https://yourbasic.org/golang/type-assertion-switch/)
- [gopls rename (replacement for gorename)](https://pkg.go.dev/golang.org/x/tools/refactor/rename)

---
*Pitfalls research for: Stratos v1.1.1 -- naming cleanup and dead code removal*
*Researched: 2026-02-04*
