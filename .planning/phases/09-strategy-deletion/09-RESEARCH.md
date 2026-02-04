# Phase 9: Strategy Deletion - Research

**Researched:** 2026-02-03
**Domain:** Go package deletion (dead code removal)
**Confidence:** HIGH

## Summary

Phase 9 deletes three package trees that are now completely unreferenced by any live code: `internal/strategy/` (including `internal/strategy/kubernetes/` and `internal/strategy/githubactions/`), and `internal/github/`. After Phase 7 relocated the kubernetes strategy code to `internal/scaling/` and Phase 8 rewired the controller to use `internal/scaling/` directly, these packages are orphaned -- they compile independently but nothing in the module imports them.

The codebase was exhaustively investigated. Zero Go source files outside the strategy/ and github/ packages import them. `go build ./...` passes today. The deletion is purely subtractive: `rm -rf` the three directory trees, verify `go build ./...` and `go list ./...` still work. The only secondary concern is a depguard linter rule in `.golangci.yml` that references the deleted `internal/strategy/` path and will need to be removed.

**Primary recommendation:** Delete the three directories with `rm -rf`, remove the depguard linter rule referencing `internal/strategy/`, verify `go build ./...` passes, and run `go mod tidy` as a hygiene step (no dependency changes expected since `internal/github/` uses only stdlib).

## Standard Stack

Not applicable -- this phase uses only filesystem deletion and Go's build toolchain. No new libraries or tools are needed.

## Architecture Patterns

### What Gets Deleted

Three directories containing 7 source files and 17 old-copy files:

**`internal/strategy/` (parent package):**

| File | Lines | Contents |
|------|-------|----------|
| `interface.go` | 63 | `ScalingStrategy` interface + type aliases to `scaling.ScalingDemand` / `scaling.ScaleDownCandidate` |
| `doc.go` | 37 | Package documentation |

**`internal/strategy/githubactions/`:**

| File | Lines | Contents |
|------|-------|----------|
| `githubactions.go` | 393 | GitHub Actions scaling strategy implementation |
| `doc.go` | 32 | Package documentation |

**`internal/strategy/kubernetes/` (old copy -- live code is in `internal/scaling/`):**

| File | Lines | Contents |
|------|-------|----------|
| 12 source files + 6 test files | 3,532 | Old copy of kubernetes strategy before relocation to `internal/scaling/` |

**`internal/github/`:**

| File | Lines | Contents |
|------|-------|----------|
| `client.go` | 425 | GitHub REST API client (stdlib-only imports) |
| `doc.go` | 32 | Package documentation |

**Total deleted:** ~4,482 lines across 24 files in 4 directories.

### Import Reference Audit

Exhaustive grep results for all Go source files in the project:

| Import path | Files referencing it OUTSIDE the deleted packages | Action needed |
|-------------|--------------------------------------------------|---------------|
| `internal/strategy` | 0 files | None -- safe to delete |
| `internal/strategy/kubernetes` | 0 files | None -- safe to delete |
| `internal/strategy/githubactions` | 0 files | None -- safe to delete |
| `internal/github` | 0 files (only `githubactions.go` inside the deleted tree) | None -- safe to delete |

Self-references (within the deleted tree, deleted along with the files):
- `internal/strategy/kubernetes/kubernetes.go` imports `internal/strategy`
- `internal/strategy/kubernetes/scaling.go` imports `internal/strategy`
- `internal/strategy/githubactions/githubactions.go` imports `internal/strategy` and `internal/github`

### Test Reference Audit

| Location | References to deleted packages | Impact |
|----------|-------------------------------|--------|
| `tests/integration/` | 0 references | None |
| `internal/controller/` test files | 0 references | None |
| `internal/scaling/` test files | 0 references (uses `scaling.` types directly) | None |

The test files inside `internal/strategy/kubernetes/` are the OLD copies from before Phase 7 relocation. The LIVE test files are in `internal/scaling/` and run correctly today. Deleting the old copies has no impact on test coverage.

### Linter Configuration Change

The `.golangci.yml` file contains a depguard rule that references the deleted path:

```yaml
depguard:
  rules:
    strategy-no-aws:
      files:
        - "**/internal/strategy/**"
        - "!$test"
      deny:
        - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
          desc: "strategy/ must not import aws/ directly; use the cloudprovider interface"
```

This rule targets files matching `**/internal/strategy/**`. After deletion, no files match this glob, so the rule is dead. It will not cause lint failures (it simply matches zero files), but it should be removed as dead configuration to avoid confusion.

### Dependency Impact

The `internal/github/` package imports **only Go standard library** (`net/http`, `encoding/json`, `fmt`, `io`, `strconv`, `strings`, `time`). Deleting it will NOT orphan any third-party dependencies in `go.mod`. Running `go mod tidy` is a hygiene step but is expected to produce no changes.

## Don't Hand-Roll

Not applicable -- this phase is pure deletion. There is nothing to build or implement.

## Common Pitfalls

### Pitfall 1: Forgetting `internal/strategy/kubernetes/` Directory

**What goes wrong:** The requirements (DEL-01, DEL-02, DEL-03) mention deleting `internal/strategy/interface.go`, `internal/strategy/doc.go`, `internal/strategy/githubactions/`, and `internal/github/`. They do NOT explicitly mention `internal/strategy/kubernetes/` because REL-02 said to "rename" it to `internal/scaling/`. However, Phase 7 was implemented as a copy (the old directory still exists with 18 files totaling 3,532 lines). If the planner only deletes the explicitly named files/directories, `internal/strategy/kubernetes/` survives, and the success criterion "internal/strategy/ directory does not exist" fails.

**How to avoid:** Delete the entire `internal/strategy/` directory tree (`rm -rf internal/strategy/`), not individual files. This handles all subdirectories including `kubernetes/`.

**Warning signs:** `ls internal/strategy/` returns results after Phase 9 execution; `go list ./...` still shows `internal/strategy/kubernetes`.

### Pitfall 2: Leaving the Depguard Linter Rule

**What goes wrong:** The `strategy-no-aws` depguard rule in `.golangci.yml` references `**/internal/strategy/**`. After deletion, this rule matches zero files and does nothing. It is not harmful but creates confusion for future maintainers who will wonder what `internal/strategy/` was. More importantly, if `make lint` or CI runs depguard validation on the rule definition itself (checking that the glob matches at least one file), it could fail.

**How to avoid:** Delete the `strategy-no-aws` rule block from `.golangci.yml` as part of this phase.

**Warning signs:** `make lint` produces warnings about unmatched depguard rules; `.golangci.yml` references paths that do not exist.

### Pitfall 3: Not Running `go build ./...` After Deletion

**What goes wrong:** The Go module still lists the deleted packages in its build cache. A developer might assume deletion is safe without running the build verification. While our audit shows zero external references, there could be generated files or edge cases the grep missed.

**How to avoid:** Always run `go build ./...` after deletion. Also run `go list ./...` to confirm the deleted packages no longer appear in the module's package list.

**Warning signs:** `go build ./...` errors; `go list ./...` still shows deleted packages.

### Pitfall 4: Planning Doc References Cause Grep False Positives

**What goes wrong:** The success criterion says `grep -r 'internal/strategy' .` returns zero matches in Go source files. The `.planning/` directory contains many markdown files that reference `internal/strategy`. If the grep is run without a file type filter, it will show matches and the criterion appears to fail.

**How to avoid:** Use a Go-specific grep: `grep -r 'internal/strategy' --include='*.go' .` (excluding the deleted files themselves). Or rely on `go build ./...` as the authoritative check.

**Warning signs:** Running unfiltered grep shows matches in `.planning/` markdown files.

## Code Examples

### Deletion Commands

```bash
# Delete the three directory trees
rm -rf internal/strategy/
rm -rf internal/github/

# Verify deletion
ls internal/strategy/ 2>&1  # Should fail: "No such file or directory"
ls internal/github/ 2>&1    # Should fail: "No such file or directory"
```

### Linter Config Update

```yaml
# BEFORE (.golangci.yml depguard section)
depguard:
  rules:
    strategy-no-aws:          # <-- DELETE this entire block
      files:
        - "**/internal/strategy/**"
        - "!$test"
      deny:
        - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
          desc: "strategy/ must not import aws/ directly; use the cloudprovider interface"
    controller-sub-no-impl:   # <-- KEEP this block
      ...

# AFTER
depguard:
  rules:
    controller-sub-no-impl:
      ...
```

### Build Verification

```bash
# Verify compilation
go build ./...

# Verify package list no longer includes deleted packages
go list ./... | grep -E 'strategy|internal/github'
# Should return nothing (exit code 1)

# Verify no Go file references deleted paths
grep -r 'internal/strategy' --include='*.go' .
# Should return nothing (exit code 1)

grep -r 'internal/github' --include='*.go' .
# Should return nothing (exit code 1)

# Hygiene: tidy modules (expected: no changes)
go mod tidy
```

## State of the Art

Not applicable -- this phase is pure deletion with no technology decisions.

## Open Questions

No open questions. This phase is fully deterministic:
1. The files to delete are identified.
2. The import audit confirms zero external references.
3. The linter config change is identified.
4. The verification commands are known.

## Sources

### Primary (HIGH confidence)

All findings are based on direct codebase analysis:

- `grep -r` across Go source files confirmed zero external imports of deleted packages
- `go build ./...` succeeds today, confirming packages are independently compilable but unreferenced
- `go list ./...` confirms the 4 packages to be removed from the module
- `.golangci.yml` directly inspected for references to deleted paths
- `go.mod` confirms `internal/github/` has no third-party dependency impact
- `diff` confirms `internal/strategy/kubernetes/` is the old copy of `internal/scaling/` (identical files minus `types.go` and package declaration changes)

### Secondary (MEDIUM confidence)

- Phase 7 and Phase 8 research documents confirmed the migration path (copy + alias in Phase 7, rewire in Phase 8, delete in Phase 9)

## Metadata

**Confidence breakdown:**
- Files to delete: HIGH -- directly verified by filesystem listing and line counts
- Import safety: HIGH -- exhaustive grep of all Go source files, zero external references found
- Linter config: HIGH -- `.golangci.yml` directly inspected
- Build safety: HIGH -- `go build ./...` passes today with these packages unreferenced
- Dependency impact: HIGH -- `internal/github/` imports only stdlib (verified in source)

**Research date:** 2026-02-03
**Valid until:** No expiration -- findings are based on current codebase state, not external dependencies
