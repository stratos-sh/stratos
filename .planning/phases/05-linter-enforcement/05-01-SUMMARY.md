---
phase: 05-linter-enforcement
plan: 01
subsystem: tooling
tags: [golangci-lint, depguard, funlen, cyclop, contextcheck, errcheck, gosec, govet, misspell, staticcheck]
dependency-graph:
  requires: [04-02]
  provides: [linter-config-with-boundary-rules, non-complexity-violations-fixed]
  affects: [05-02, 06-01]
tech-stack:
  added: []
  patterns: [depguard-boundary-enforcement, safeInt32-overflow-guard]
key-files:
  created: []
  modified:
    - .golangci.yml
    - cmd/stratos/main.go
    - internal/strategy/kubernetes/scaling.go
    - internal/controller/nodepool/cloud_sync.go
    - internal/controller/nodepool/lifecycle/warmup_monitor.go
    - internal/controller/nodepool/reconciler.go
    - internal/controller/nodeclass/reconciler.go
    - internal/github/client.go
    - internal/controller/nodepool/provider_cache.go
    - internal/strategy/kubernetes/maintenance_test.go
    - internal/strategy/kubernetes/readiness_test.go
    - internal/strategy/kubernetes/startup_taints_test.go
decisions:
  - id: LINT-01
    decision: "safeInt32() overflow guard pattern used instead of nolint comments for gosec G115"
    context: "gosec flags int->int32 conversions as potential overflow. Options: nolint, bounds check, or helper function"
    alternatives: ["inline bounds check", "nolint:gosec comment"]
  - id: LINT-02
    decision: "Renamed strat -> scaler/scalingStrategy/testStrategy to avoid misspell false positive"
    context: "misspell linter flags 'strat' as misspelling of 'start'. Could also configure misspell ignore list."
    alternatives: ["add strat to misspell ignore list"]
  - id: LINT-03
    decision: "resp.Body.Close() uses nolint:errcheck comment since error is genuinely unactionable"
    context: "errcheck with check-blank:true flags even _ = expr. HTTP response body close errors have no useful recovery."
    alternatives: ["disable check-blank", "log the error"]
  - id: LINT-04
    decision: "FindScaleDownCandidates complexity increased to 16 (from 14) due to time.Parse error handling fix"
    context: "errcheck fix for time.Parse added a branch, pushing cyclomatic complexity from 14 to 16, exceeding threshold 15"
    alternatives: ["extract time parsing to helper to reduce complexity"]
metrics:
  duration: 7min
  completed: 2026-02-03
---

# Phase 5 Plan 01: Linter Config and Non-Complexity Fixes Summary

Added 4 structural linters (depguard, funlen, cyclop, contextcheck) with boundary rules and fixed all 23 pre-existing non-complexity violations plus 2 new funlen violations, leaving only 5 gocyclo complexity violations for Plan 02.

## What Was Done

### Task 1: Updated .golangci.yml (a4dde44)
- Added 4 new linters to `linters.enable`: funlen, cyclop, depguard, contextcheck (total: 12)
- Configured funlen: 80 lines, 50 statements, ignore-comments
- Configured cyclop: max-complexity 15, package-average 7.0
- Added depguard boundary rules:
  - `strategy-no-aws`: blocks strategy/ from importing cloudprovider/aws
  - `controller-sub-no-impl`: blocks lifecycle/, nodestate/, nodeclass/ from importing aws/ or fake/
- Added test exclusions for funlen, cyclop, contextcheck

### Task 2: Fixed All Non-Complexity Violations (440faeb)

**errcheck (9 violations fixed):**
- cloud_sync.go: 3x `getOrCreateStrategy()` error now checked with proper error return
- warmup_monitor.go: `time.Parse` error handled via `if parseErr == nil` pattern
- scaling.go: `time.Parse` error handled via `if parseErr == nil` pattern
- github/client.go: `resp.Body.Close()` -- nolint:errcheck (unactionable)
- github/client.go: `strconv.Atoi/ParseInt` in ParseRateLimitHeaders now checked

**gosec (3 violations fixed):**
- reconciler.go, nodeclass/reconciler.go: Added `safeInt32()` helper with `math.MaxInt32/MinInt32` bounds checking for int->int32 conversions

**govet (3+3 violations fixed):**
- reconciler.go: Renamed shadowing `err` variables to `transErr`, `drainErr`, `prepErr`, `getErr`, `repErr` in scale-down, max-runtime, replenish, and status update sections

**misspell (3+3 violations fixed):**
- cloud_sync.go: `strat` -> `scalingStrategy` (3 occurrences)
- provider_cache.go: `strat` -> `scalingStrategy` in newNodeManager parameter (3 occurrences)
- reconciler.go: `strat` -> `scaler` throughout reconcileNodePool and startStandbyNodes
- maintenance_test.go, readiness_test.go, startup_taints_test.go: `strat` -> `testStrategy`/`scalingStrategy`

**staticcheck (1 violation fixed):**
- reconciler.go: Removed redundant `ObjectMeta` from `nodePool.ObjectMeta.DeletionTimestamp`

**funlen (2 violations fixed):**
- cmd/stratos/main.go: Extracted `parseFlags()`, `buildClusterConfig()`, `registerAWSNodeClassReconciler()` from main() (75 -> ~25 statements)
- scaling.go: Extracted `adjustForCapacity()` from CheckDemand (81 -> ~65 lines)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] FindScaleDownCandidates complexity increased to 16**
- **Found during:** Task 2 (errcheck fix for time.Parse)
- **Issue:** The errcheck fix for `candidateSince, _ = time.Parse(...)` added a branch (`if parseErr == nil`), pushing cyclomatic complexity from 14 to 16 (threshold: 15)
- **Fix:** This is a necessary errcheck fix. The function will be addressed in Plan 02 along with the other complexity violations.
- **Files modified:** internal/strategy/kubernetes/scaling.go

**2. [Rule 2 - Missing Critical] Additional misspell violations in provider_cache.go and test files**
- **Found during:** Task 2 (after initial lint run)
- **Issue:** The `strat` variable name appeared in provider_cache.go and 3 test files beyond what was listed in the plan
- **Fix:** Renamed all occurrences consistently: `scalingStrategy` in production, `testStrategy` in tests
- **Files modified:** internal/controller/nodepool/provider_cache.go, internal/strategy/kubernetes/maintenance_test.go, readiness_test.go, startup_taints_test.go

## Decisions Made

| ID | Decision | Rationale |
|----|----------|-----------|
| LINT-01 | safeInt32() helper over nolint comments | Reusable, explicit bounds check is clearer than suppressing the warning |
| LINT-02 | Rename strat to avoid misspell false positive | Clearer variable names (scaler, scalingStrategy) are better than ignore-list hacks |
| LINT-03 | nolint:errcheck for resp.Body.Close() | HTTP response body close errors have no useful recovery path |
| LINT-04 | Accept FindScaleDownCandidates at complexity 16 | errcheck fix is mandatory; Plan 02 will refactor complexity |

## Final State

**Lint violations remaining: 5 (all gocyclo complexity, for Plan 02)**
1. `reconcileNodePool` - complexity 46
2. `LaunchInstance` - complexity 19
3. `MonitorCloudWarmup` - complexity 19
4. `MonitorWarmup` - complexity 17
5. `FindScaleDownCandidates` - complexity 16 (new, from errcheck fix)

**go build ./...**: passes
**make test**: passes

## Next Phase Readiness

Plan 02 has 5 complexity violations to refactor (one more than originally expected due to the errcheck-induced complexity increase in FindScaleDownCandidates). All other lint categories pass clean. The depguard boundary rules are verified working with zero violations.
