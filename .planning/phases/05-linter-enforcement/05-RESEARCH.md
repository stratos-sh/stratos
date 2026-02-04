# Phase 5: Linter Enforcement - Research

**Researched:** 2026-02-03
**Domain:** golangci-lint v2 structural linter configuration (depguard, funlen, cyclop, contextcheck)
**Confidence:** HIGH

## Summary

Phase 5 adds four new linters to the existing golangci-lint v2.8.0 configuration to enforce package boundaries, function length limits, package complexity limits, and context propagation correctness. The project already uses golangci-lint v2.8.0 (`version: "2"` format) with 8 linters enabled.

All four target linters (depguard, funlen, cyclop, contextcheck) are available in the installed version and have been tested against the codebase. The current codebase has 23 existing lint violations from the 8 already-enabled linters. Adding the 4 new linters with appropriate thresholds introduces only 2 additional production-code violations (both funlen). Depguard boundary rules already pass with zero violations -- the Phase 4 restructure correctly separated the packages. Contextcheck passes clean with zero violations.

**Primary recommendation:** Add all four linters in a single `.golangci.yml` update, fix the 2 new funlen violations (main.go and scaling.go), and fix the 23 pre-existing violations to achieve zero-violation `make lint`.

## Standard Stack

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| golangci-lint | v2.8.0 | Linter aggregator | Already installed and configured in project; v2 format with `version: "2"` |
| depguard | (bundled) | Import boundary enforcement | Only golangci-lint linter that enforces per-directory import rules via `files` glob + `deny` |
| funlen | (bundled) | Function length limits | Standard function-length linter; configurable lines + statements thresholds |
| cyclop | (bundled) | Function + package complexity | Unlike gocyclo, cyclop adds `package-average` for aggregate complexity checks |
| contextcheck | (bundled) | Context propagation | Catches non-inherited context.Background() in production code; zero-config |

### Supporting
| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| gocyclo | (bundled) | Function cyclomatic complexity | Already enabled at threshold 15; cyclop duplicates this + adds package-average |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| depguard | gomodguard | gomodguard operates at module level, not package-import level -- cannot enforce internal package boundaries |
| cyclop | gocognit | gocognit measures cognitive complexity (different metric); cyclop uses same cyclomatic metric as gocyclo but adds package-average |
| Keep both gocyclo + cyclop | Replace gocyclo with cyclop | Both measure cyclomatic complexity at function level; cyclop is a strict superset (adds package-average). Replacing gocyclo with cyclop reduces redundant reports. However, keeping both is also viable since they report on the same functions. |

## Architecture Patterns

### Config Structure (golangci-lint v2 format)

The `.golangci.yml` uses the v2 format (required by v2.8.0). All new linters go under `linters.enable`, settings under `linters.settings`, and test exclusions under `linters.exclusions.rules`.

```yaml
version: "2"

linters:
  default: none
  enable:
    # ... existing linters ...
    - funlen
    - cyclop
    - depguard
    - contextcheck

  settings:
    # ... existing settings ...
    funlen:
      lines: 80
      statements: 50
      ignore-comments: true
    cyclop:
      max-complexity: 15
      package-average: 7.0
    depguard:
      rules:
        strategy-no-aws:
          files:
            - "**/internal/strategy/**"
            - "!$test"
          deny:
            - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
              desc: "strategy/ must not import aws/ directly; use the cloudprovider interface"
        controller-sub-no-impl:
          files:
            - "**/internal/controller/nodepool/lifecycle/**"
            - "**/internal/controller/nodepool/nodestate/**"
            - "**/internal/controller/nodeclass/**"
            - "!$test"
          deny:
            - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
              desc: "controller sub-packages must not import provider implementations directly"
            - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/fake"
              desc: "controller sub-packages must not import provider implementations directly"

  exclusions:
    rules:
      - path: _test\.go
        linters:
          # ... existing exclusions ...
          - funlen
          - cyclop
          - contextcheck
```

### Depguard Rules Architecture

Depguard rules use named rule sets. Each rule set has:
- `files`: Glob patterns specifying which source files the rule applies to. Defaults to `$all`. The `$test` variable matches `_test.go` files; prefix with `!` to negate.
- `deny`: List of `pkg` + `desc` pairs. The `pkg` field matches import path prefixes (or exact matches if ending with `$`).
- `list-mode`: Use `lax` (default) -- denies only what's in the deny list; everything else is allowed.

**Boundary rules needed (verified against codebase):**

1. **strategy-no-aws**: `internal/strategy/**` must NOT import `cloudprovider/aws`. Currently passes clean -- strategy only imports `cloudprovider` (the interface package).

2. **controller-sub-no-impl**: `internal/controller/nodepool/lifecycle/**`, `nodestate/**`, and `nodeclass/**` must NOT import `cloudprovider/aws` or `cloudprovider/fake`. Currently passes clean in production code. Test files use `!$test` exclusion since `warmup_test.go` legitimately imports the fake provider.

**Not restricted (by design):**
- `internal/controller/nodepool/setup.go` and `provider_cache.go` legitimately import `cloudprovider/aws` for factory/wiring
- `internal/controller/setup.go` legitimately imports `cloudprovider/aws` for the aggregator setup

### Anti-Patterns to Avoid
- **Over-restricting the top-level controller package:** The `nodepool/` package itself (setup.go, provider_cache.go, reconciler.go) needs aws imports for wiring. Only sub-packages (lifecycle/, nodestate/) should be restricted.
- **Applying complexity linters to test files:** Table-driven tests can be legitimately long. Always exclude `_test.go` from funlen, cyclop, gocyclo, errcheck, gosec, contextcheck.
- **Setting funlen too aggressively:** Default of 60 lines flags 27+ violations. The 80-line threshold is practical -- only 4 production functions exceed it, and 2 of those are the only truly new violations.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Import boundary checks | Custom go/ast scripts | depguard linter | Depguard integrates into golangci-lint, runs in CI, supports glob-scoped rules |
| Function size checks | Manual code review | funlen linter | Automated, configurable lines + statements thresholds |
| Package complexity | Manual complexity audits | cyclop package-average | Automated aggregate check prevents creeping complexity |
| Context propagation | grep for context.Background | contextcheck linter | Static analysis traces context flow through call graphs |

**Key insight:** All four linters are already bundled in golangci-lint v2.8.0. No additional Go modules, installs, or tooling needed.

## Common Pitfalls

### Pitfall 1: Depguard files globs not matching expected paths
**What goes wrong:** Depguard `files` patterns use different glob semantics than shell globs. Patterns like `internal/strategy/**` may not match as expected.
**Why it happens:** Depguard uses filepath.Match with `**` for recursive matching. The leading `**/` is important for matching regardless of the working directory.
**How to avoid:** Always use `**/internal/...` prefix in file patterns. Verified working pattern: `"**/internal/strategy/**"`.
**Warning signs:** No depguard violations reported even when you know imports exist.

### Pitfall 2: Cyclop and gocyclo double-reporting
**What goes wrong:** Both linters report the same function-level cyclomatic complexity violations, cluttering output.
**Why it happens:** Cyclop measures function cyclomatic complexity identically to gocyclo, plus adds package-average.
**How to avoid:** Either (a) keep both and accept the duplicate reports, or (b) replace gocyclo with cyclop (preferred since cyclop is a superset). If keeping both, ensure they have the same threshold (15) so violations are consistent.
**Warning signs:** Seeing identical functions flagged by both `(gocyclo)` and `(cyclop)` in output.

### Pitfall 3: contextcheck false positives in test files
**What goes wrong:** contextcheck flags `context.Background()` usage in test files, which is perfectly valid.
**Why it happens:** Tests legitimately create root contexts. contextcheck doesn't distinguish test vs production code.
**How to avoid:** Add `contextcheck` to the `_test\.go` exclusion rule.
**Warning signs:** Many violations in `*_test.go` files after enabling contextcheck.

### Pitfall 4: Fixing existing violations before adding new linters
**What goes wrong:** Trying to add linters and fix pre-existing violations in the same PR creates huge diffs.
**Why it happens:** The codebase currently has 23 violations from existing linters (errcheck:9, gocyclo:4, gosec:3, govet:3, misspell:3, staticcheck:1).
**How to avoid:** The success criteria requires `make lint` to pass with zero violations. Plan to fix ALL 25 violations (23 existing + 2 new funlen). Consider splitting into: (1) fix existing violations, (2) add new linters, (3) final verification.
**Warning signs:** `make lint` still failing after adding new linters because of pre-existing issues.

### Pitfall 5: reconcileNodePool complexity of 46
**What goes wrong:** The main reconciliation function has cyclomatic complexity 46 (threshold: 15). This is the single largest violation and will require significant refactoring to fix.
**Why it happens:** The reconcileNodePool function handles scale-up, monitoring, scale-down, max-runtime recycling, standby replenishment, and status updates in a single 230-line function.
**How to avoid:** Extract logical phases into helper methods (e.g., `handleScaleDown`, `handleMaxRuntimeRecycling`, `handleStandbyReplenishment`, `updateStatus`).
**Warning signs:** Function is 230+ lines with complexity 46; this requires careful extraction, not just reformatting.

## Code Examples

### Complete .golangci.yml with all new linters (verified working)

```yaml
version: "2"

linters:
  default: none
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gosec
    - gocyclo
    - misspell
    - funlen
    - cyclop
    - depguard
    - contextcheck

  settings:
    errcheck:
      check-type-assertions: true
      check-blank: true
    gocyclo:
      min-complexity: 15
    govet:
      enable:
        - shadow
        - nilness
    funlen:
      lines: 80
      statements: 50
      ignore-comments: true
    cyclop:
      max-complexity: 15
      package-average: 7.0
    depguard:
      rules:
        strategy-no-aws:
          files:
            - "**/internal/strategy/**"
            - "!$test"
          deny:
            - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
              desc: "strategy/ must not import aws/ directly; use the cloudprovider interface"
        controller-sub-no-impl:
          files:
            - "**/internal/controller/nodepool/lifecycle/**"
            - "**/internal/controller/nodepool/nodestate/**"
            - "**/internal/controller/nodeclass/**"
            - "!$test"
          deny:
            - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
              desc: "controller sub-packages must not import provider implementations directly"
            - pkg: "github.com/stratos-sh/stratos/internal/cloudprovider/fake"
              desc: "controller sub-packages must not import provider implementations directly"

  exclusions:
    paths:
      - vendor
      - testdata
    rules:
      - path: _test\.go
        linters:
          - gocyclo
          - errcheck
          - gosec
          - funlen
          - cyclop
          - contextcheck

run:
  timeout: 5m
```

### Violation Inventory (verified via actual lint runs)

**Pre-existing violations (23):**
- errcheck: 9 (unchecked errors in cloud_sync.go, warmup_monitor.go, github/client.go, scaling.go)
- gocyclo: 4 (LaunchInstance:19, MonitorWarmup:17, MonitorCloudWarmup:18, reconcileNodePool:46)
- gosec: 3 (integer overflow conversions in reconciler.go, nodeclass/reconciler.go)
- govet: 3 (shadow declarations in reconciler.go)
- misspell: 3 (`strat` flagged as misspelling of `start` in cloud_sync.go)
- staticcheck: 1 (unnecessary ObjectMeta selector in reconciler.go)

**New violations from added linters (2):**
- funlen: `main()` in cmd/stratos/main.go (75 statements > 50)
- funlen: `CheckDemand` in internal/strategy/kubernetes/scaling.go (81 lines > 80)

**Already passing (0 violations):**
- depguard: All boundary rules pass clean
- contextcheck: No context.Background() misuse in production code
- cyclop: Same 4 functions as gocyclo (overlap, not additive)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| golangci-lint v1 config | golangci-lint v2 format (`version: "2"`) | v2.0 (March 2025) | Different YAML structure; `enable-all`/`disable-all` replaced with `linters.default` |
| depguard v1 config (type/packages) | depguard v2 config (rules/deny/files) | golangci-lint v1.50+ | Completely different config structure; old `type: denylist` replaced with named rules |
| Separate gocyclo only | cyclop as superset | Available since golangci-lint v1.37 | cyclop adds package-average metric on top of per-function cyclomatic checks |

## Threshold Rationale

### funlen: lines=80, statements=50
- **Default:** 60 lines, 40 statements
- **Rationale:** Default 60 flags 27+ functions including many that are only slightly over. At 80/50, only 4 production functions are flagged (main.go, LaunchInstance, reconcileNodePool, CheckDemand). Of these, reconcileNodePool and LaunchInstance are already flagged by gocyclo, leaving only 2 net-new violations to fix.
- **Test files excluded:** Table-driven tests legitimately have long function bodies.

### cyclop: max-complexity=15, package-average=7.0
- **max-complexity matches gocyclo:** Both at 15 for consistency. The 4 functions flagged are the same.
- **package-average=7.0:** At 5.0, cmd/main (average 7.0) is flagged. At 7.0, only the already-flagged high-complexity functions matter. This provides a guard against future creep without requiring immediate refactoring of main.go's package.
- **Test files excluded.**

### depguard: lax mode, glob-scoped deny rules
- **list-mode=lax (default):** Only blocks explicitly denied packages; everything else is allowed. This is the right mode for targeted boundary enforcement.
- **Two rule sets:** One for strategy/, one for controller sub-packages.
- **Test files excluded via `!$test`:** Tests need to import fake providers for mocking.

### contextcheck: no configuration
- **Zero-config linter.** Simply enable it.
- **Test files excluded:** Tests legitimately use `context.Background()`.

## Open Questions

1. **Replace gocyclo with cyclop or keep both?**
   - What we know: cyclop is a strict superset of gocyclo (same cyclomatic metric + package-average). Both at threshold 15 report identical function violations.
   - What's unclear: Whether the project has any reason to keep gocyclo specifically (e.g., CI pipeline references).
   - Recommendation: Replace gocyclo with cyclop to eliminate duplicate reports. The CLAUDE.md mentions gocyclo but this is a documentation update, not a functional change.

2. **Fixing pre-existing violations: same phase or separate?**
   - What we know: 23 existing violations must be fixed to achieve `make lint` passing. Some (reconcileNodePool complexity:46) require significant refactoring.
   - What's unclear: Whether the phase scope should include fixing all violations or just adding the linter config.
   - Recommendation: Phase 5 should fix ALL violations since the success criterion is "make lint passes with zero violations." The reconcileNodePool refactoring (complexity 46) is the largest task.

3. **misspell: `strat` flagged as misspelling of `start`**
   - What we know: `strat` is used as an abbreviation for `strategy` in variable names. The misspell linter flags it.
   - What's unclear: Whether to rename the variable or configure misspell to ignore it.
   - Recommendation: Rename to `scalingStrategy` or `strategyImpl` -- it's clearer and eliminates the false positive without needing linter configuration workarounds.

## Sources

### Primary (HIGH confidence)
- Context7 `/websites/golangci-lint_run` - depguard configuration, funlen settings, cyclop settings, exclusion rules
- golangci-lint v2.8.0 binary (installed at `/home/roeeh/projects/presto/bin/golangci-lint`) - all linter availability verified via `golangci-lint linters`
- Actual lint runs against codebase - all violation counts and patterns verified empirically

### Secondary (MEDIUM confidence)
- https://golangci-lint.run/docs/linters/configuration/ - depguard, funlen, cyclop configuration reference
- https://golangci-lint.run/docs/configuration/file/ - v2 config file format, exclusions structure, path modes

### Tertiary (LOW confidence)
- None. All findings verified via actual lint runs against the codebase.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All linters tested against codebase with exact violation counts
- Architecture: HIGH - Complete .golangci.yml verified working via actual lint runs
- Pitfalls: HIGH - Each pitfall discovered through empirical testing (e.g., depguard glob patterns, cyclop/gocyclo overlap)

**Research date:** 2026-02-03
**Valid until:** 2026-04-03 (stable -- golangci-lint v2 config format is settled)
