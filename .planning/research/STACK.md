# Technology Stack: Naming Cleanup & Dead Code Removal

**Project:** Stratos Kubernetes Operator -- v1.1.1 naming and dead code cleanup
**Researched:** 2026-02-04
**Overall confidence:** HIGH

This milestone is a naming cleanup and dead code removal pass. The "stack" is the tools and techniques for safe Go refactoring, not new dependencies to add.

## Executive Assessment

Five operations need to happen: two type renames, four file renames, one dead method deletion, and one struct field type change. The Go ecosystem provides two purpose-built tools for these operations: `gopls rename` (type-safe symbol renaming) and `deadcode` (unreachable function detection). Both are verified working on this codebase today. No new module dependencies are needed. The existing linter and test infrastructure validates every change.

---

## Recommended Tools by Operation

### 1. Type Renaming: `gopls rename` (recommended)

**Tool:** `gopls rename` CLI subcommand
**Version verified:** gopls v0.21.0 (installed at `~/go/bin/gopls`)
**Confidence:** HIGH -- verified on this exact codebase

`gopls rename` is the officially recommended Go refactoring tool, replacing the deprecated `gorename`. It performs type-safe symbol renaming with the following safety guarantees:

- Detects symbol shadowing that would break compilation
- Verifies interface satisfaction is preserved after renaming methods
- Handles anonymous field embedding correctly
- Never introduces compilation errors (guaranteed by the tool's design)

**Usage pattern:**

```bash
# Rename exported type: Strategy -> Scaler
# Position format: file.go:line:column
gopls rename -w internal/scaling/kubernetes.go:39:6 Scaler

# Rename unexported type: drainHelper -> nodeDrainer
gopls rename -w internal/scaling/drain.go:30:6 nodeDrainer
```

**Flags:**
- `-w` (write): Apply changes to source files in place
- `-d` (diff): Show diffs without writing (dry-run)
- `-l` (list): Show which files would be modified

**Verified dry-run results on this codebase:**

| Rename | Files Modified | What Changes |
|--------|---------------|--------------|
| `Strategy` to `Scaler` | ~8 files (kubernetes.go, pod_assignments.go, events.go, scaling.go, readiness.go, maintenance.go, network.go, tests) | Type declaration, all method receivers `(s *Strategy)` to `(s *Scaler)`, all constructor calls, all test struct literals |
| `drainHelper` to `nodeDrainer` | ~3 files (drain.go, drain_eviction.go, scaling.go) | Type declaration, all method receivers `(d *drainHelper)` to `(d *nodeDrainer)`, constructor call site |

**Why gopls rename, not manual find-and-replace:**
1. It understands Go scope -- it renames only the correct `Strategy` in `package scaling`, not any other `Strategy` in comments, strings, or other packages.
2. It renames the type AND all its method receivers AND all call sites atomically.
3. It validates the rename would not break compilation before writing.
4. For `Strategy` (13 references across 5+ files), manual renaming is error-prone. For `drainHelper` (fewer references), gopls rename is still faster and safer than manual editing.

**Why NOT manual search-and-replace:**
- `Strategy` is a common word that appears in comments, doc strings, and planning files. A regex replace would produce false positives.
- Method receiver variables (`s *Strategy`) must be renamed in sync with the type declaration. gopls handles this atomically.

### 2. File Renaming: `git mv` (recommended)

**Tool:** `git mv`
**Confidence:** HIGH

Go does not care about filenames. Any `.go` file in a package directory is part of that package regardless of its name. File renaming is purely organizational -- it affects git history and developer navigation, not compilation.

**Usage pattern:**

```bash
# Rename files within internal/scaling/
git mv internal/scaling/kubernetes.go internal/scaling/scaler.go
git mv internal/scaling/kubernetes_test.go internal/scaling/scaler_test.go
git mv internal/scaling/types.go internal/scaling/scaling_types.go
git mv internal/scaling/events.go internal/scaling/pod_events.go

# Rename file in cloudprovider/
git mv internal/cloudprovider/types.go internal/cloudprovider/instance_types.go
```

**Why `git mv`, not `mv`:**
- `git mv` preserves file rename history in git, making `git log --follow` work correctly.
- `git mv` is equivalent to `mv` + `git add` (old path deletion + new path addition), but makes the rename intent explicit in the commit.

**Why NOT gopls for file renaming:**
- gopls does not support file renaming. It renames symbols (types, functions, variables), not files.
- File names have no semantic meaning in Go beyond the `_test.go` suffix convention.

**Important constraint:** Rename files BEFORE or AFTER type renaming, not interleaved. The `gopls rename` command references files by path, so the file must exist at the path specified when running the command.

### 3. Dead Code Removal: `deadcode` tool (recommended for detection)

**Tool:** `golang.org/x/tools/cmd/deadcode`
**Version verified:** latest (installed at `~/go/bin/deadcode`)
**Confidence:** HIGH -- verified on this exact codebase

The `deadcode` tool uses Rapid Type Analysis (RTA) to build a call graph from `main()` and reports functions that are unreachable. Unlike the `unused` linter in golangci-lint (which only detects unused unexported symbols within a single package), `deadcode` finds unreachable exported and unexported functions across the entire program.

**Verified dead code in this codebase (run on 2026-02-04):**

```
internal/scaling/drain.go:99:23: unreachable func: drainHelper.UncordonNode
internal/cloudprovider/aws/instance_types.go:151:6: unreachable func: IsKnownInstanceType
internal/cloudprovider/fake/resolver.go:43:6: unreachable func: NewFakeResolver
internal/cloudprovider/fake/resolver.go:57:24: unreachable func: FakeResolver.ResolveAMI
internal/cloudprovider/fake/resolver.go:64:24: unreachable func: FakeResolver.ResolveSubnets
internal/cloudprovider/fake/resolver.go:71:24: unreachable func: FakeResolver.ResolveSecurityGroups
internal/cloudprovider/fake/resolver.go:78:24: unreachable func: FakeResolver.ResolveInstanceProfile
internal/cloudprovider/fake/resolver.go:85:24: unreachable func: FakeResolver.DeleteInstanceProfile
internal/metrics/metrics.go:260:6: unreachable func: RecordStartingNodes
```

**Usage pattern:**

```bash
# Find all dead code (including test entry points)
~/go/bin/deadcode -test -filter=stratos ./cmd/stratos/...

# Explain why a specific function IS reachable (useful for borderline cases)
~/go/bin/deadcode -whylive="github.com/stratos-sh/stratos/internal/scaling.drainHelper.CordonNode" -test ./cmd/stratos/...
```

**Key flags:**
- `-test`: Include test executables in analysis (reduces false positives -- a function used only in tests is not "dead")
- `-filter=stratos`: Only report dead code in this module, not dependencies
- `-whylive=function`: Show shortest path from main to a function (debugging tool)

**Why NOT rely solely on golangci-lint `unused` for dead code:**
- The `unused` linter in golangci-lint detects unused unexported symbols within a single package. It does NOT detect unused exported functions, and it does NOT perform cross-package reachability analysis.
- Verified: `golangci-lint run` reports 0 issues on this codebase, yet `deadcode` correctly identifies 9 unreachable functions including `UncordonNode`.
- The `deadcode` linter (lowercase, old golangci-lint integration) is deprecated and removed.

**For this milestone:** Only `UncordonNode` is in scope for deletion. The other dead code items (FakeResolver methods, IsKnownInstanceType, RecordStartingNodes) may be intentionally retained for future use or test infrastructure. The `deadcode` tool output is a detection aid, not an automatic deletion list.

### 4. Struct Field Type Change: Manual edit + `go build` (recommended)

**Tool:** Go compiler (`go build ./...`)
**Confidence:** HIGH

Changing `ScalingDemand.Metadata` from `interface{}` to `Pods []corev1.Pod` is a type signature change. No automated refactoring tool handles this -- it requires understanding the semantic intent.

**Approach:**
1. Change the field type in `internal/scaling/types.go`
2. Run `go build ./...`
3. The compiler reports every site that passes `interface{}` where `[]corev1.Pod` is now expected, and every site that type-asserts the metadata
4. Fix each call site to pass `[]corev1.Pod` directly instead of wrapping in `interface{}`
5. Remove any type assertions (`metadata.([]corev1.Pod)`) that are now unnecessary

**Why manual editing is correct here:**
- This is a semantic change, not a mechanical rename. The compiler identifies exactly which sites need updating.
- The change simplifies code -- type assertions at consumption sites are eliminated.
- There is no tool that can infer "this `interface{}` should become `[]corev1.Pod`" -- that requires domain knowledge.

---

## Complete Toolchain (All Already Present or Trivially Installable)

| Tool | Version | Role | Status |
|------|---------|------|--------|
| `gopls` | v0.21.0 | Type-safe symbol renaming | Installed at `~/go/bin/gopls` |
| `deadcode` | latest | Dead code detection via RTA | Installed at `~/go/bin/deadcode` |
| `go build` | 1.25.5 | Primary compile-time correctness check | Bundled with Go |
| `go vet` | 1.25.5 | Shadow detection, nil analysis | Bundled with Go |
| `golangci-lint` | v2.8.0 | Lint suite (unused, staticcheck, errcheck, depguard, funlen, cyclop, contextcheck) | Installed at `bin/golangci-lint` |
| `controller-gen` | v0.16.5 | CRD + deepcopy regeneration (only if `api/v1alpha1/` types change) | Installed at `bin/controller-gen` |
| `git mv` | system | File renaming with history preservation | System git |
| `make test` | Makefile | Unit tests with coverage | Project Makefile |
| `make test-integration` | Makefile | envtest integration tests | Project Makefile |

**No new tools need to be installed. No new Go module dependencies need to be added.**

---

## What NOT to Add and Why

| Tool/Approach | Why Skip |
|---------------|----------|
| `gorename` | Deprecated and broken under Go modules. The package itself says "use gopls instead." |
| `gomvpkg` | For moving entire packages between import paths. Not needed -- we are renaming files and types within existing packages, not moving packages. |
| `gofmt -r` (rewrite rules) | For syntactic pattern rewrites (e.g., `a.Foo()` to `a.Bar()`). Tempting but dangerous for type renames because it matches syntactically, not semantically. `gopls rename` is safer because it understands Go types. |
| `sed`/`perl` regex replace | No type awareness. Would rename `Strategy` in comments, strings, and unrelated packages. Actively harmful for this use case. |
| `rf` (rsc.io/rf) | Experimental refactoring tool by Russ Cox. Not widely adopted, not as well-tested as gopls rename. Adds unnecessary risk for a straightforward rename. |
| `deadmono` | For monorepo dead code detection across multiple main packages. Stratos has a single `cmd/stratos/main.go` entry point, so standard `deadcode` is sufficient. |
| IDE-based refactoring (GoLand, VS Code) | Both use gopls under the hood. Using the CLI directly is more reproducible and scriptable for this milestone. |

---

## Operation Ordering (Recommended Sequence)

The order matters because `gopls rename` references files by path.

```
1. Dead code removal (delete UncordonNode)
   Tool: manual deletion, verified by `deadcode`
   Reason: Smallest change, independent of everything else

2. Type renames (Strategy -> Scaler, drainHelper -> nodeDrainer)
   Tool: `gopls rename -w`
   Reason: Must happen BEFORE file renames so gopls can find the files

3. File renames (kubernetes.go -> scaler.go, etc.)
   Tool: `git mv`
   Reason: Must happen AFTER type renames (gopls needs stable paths)

4. Struct field type change (Metadata interface{} -> Pods []corev1.Pod)
   Tool: manual edit + `go build`
   Reason: Independent, but logically last (naming is settled)

5. Verification
   Tool: `go build ./...` && `make lint` && `make test` && `make test-integration`
```

---

## kubebuilder/controller-gen Considerations

For this milestone, the refactoring scope is `internal/` packages, NOT `api/v1alpha1/` CRD types. The `ScalingDemand` struct is in `internal/scaling/types.go`, not in the CRD API. Therefore:

- `make generate` (deepcopy regeneration): **NOT needed** unless `api/v1alpha1/` types are modified
- `make manifests` (CRD YAML regeneration): **NOT needed** unless kubebuilder markers change
- `controller-gen` markers: **Unaffected** -- all markers are in `api/v1alpha1/`, which is outside the rename scope

The one exception: if `ScalingDemand.Metadata` is referenced in CRD types (verified: it is not -- `ScalingDemand` is an internal type used only within the scaling package and its consumers in `internal/controller/`).

---

## Verification Command Sequence

After all changes are made, run these commands in order:

```bash
# 1. Compile check (catches ALL broken imports, type mismatches, unused imports)
go build ./...

# 2. Vet check (catches shadow issues from renames, nil analysis)
go vet ./...

# 3. Lint check (catches unused code, staticcheck, depguard, funlen limits)
make lint

# 4. Dead code check (confirm UncordonNode no longer appears)
~/go/bin/deadcode -test -filter=stratos ./cmd/stratos/...

# 5. Unit tests (verifies behavior preservation)
make test

# 6. Integration tests (verifies controller reconciliation end-to-end)
make test-integration
```

If all 6 steps pass, the refactoring is complete and correct.

---

## Sources

- gopls rename documentation: [Gopls Code Transformation Features](https://go.dev/gopls/features/transformation) -- HIGH confidence (official Go documentation)
- gopls CLI documentation: [Gopls Command-line Interface](https://go.dev/gopls/command-line) -- HIGH confidence (official Go documentation)
- gorename deprecation: [golang.org/x/tools/refactor/rename](https://pkg.go.dev/golang.org/x/tools/refactor/rename) -- HIGH confidence (official package notice)
- gorename deletion issue: [Issue #69360](https://github.com/golang/go/issues/69360) -- HIGH confidence (official Go issue tracker)
- deadcode tool documentation: [Finding unreachable functions with deadcode](https://go.dev/blog/deadcode) -- HIGH confidence (official Go blog)
- deadcode package docs: [pkg.go.dev/golang.org/x/tools/cmd/deadcode](https://pkg.go.dev/golang.org/x/tools/cmd/deadcode) -- HIGH confidence (official package docs)
- golangci-lint unused vs deadcode: [Discussion #4819](https://github.com/golangci/golangci-lint/discussions/4819) -- MEDIUM confidence (community discussion, verified with tool behavior)
- gopls rename verified on codebase: `gopls rename -d internal/scaling/kubernetes.go:39:6 Scaler` -- HIGH confidence (direct verification)
- deadcode verified on codebase: `deadcode -test -filter=stratos ./cmd/stratos/...` -- HIGH confidence (direct verification)
- kubebuilder controller-gen: [controller-gen CLI Reference](https://book.kubebuilder.io/reference/controller-gen.html) -- HIGH confidence (official kubebuilder docs)
