---
phase: 01-mechanical-cleanup
verified: 2026-02-02T19:40:31Z
status: gaps_found
score: 4/5 must-haves verified
gaps:
  - truth: "No two files in the project share an ambiguous name"
    status: partial
    reason: "interface.go exists in two packages (cloudprovider and strategy) with different purposes, warmup.go exists in aws and lifecycle packages"
    artifacts:
      - path: "internal/cloudprovider/interface.go"
        issue: "Generic name 'interface.go' doesn't indicate it defines CloudProvider interface"
      - path: "internal/controller/strategy/interface.go"
        issue: "Generic name 'interface.go' doesn't indicate it defines ScalingStrategy interface"
      - path: "internal/cloudprovider/aws/warmup.go"
        issue: "Name collision with internal/controller/lifecycle/warmup.go"
      - path: "internal/controller/lifecycle/warmup.go"
        issue: "Name collision with internal/cloudprovider/aws/warmup.go (different concerns)"
    missing:
      - "Rename internal/cloudprovider/interface.go to cloudprovider_interface.go or provider.go"
      - "Rename internal/controller/strategy/interface.go to strategy_interface.go or scaling_strategy.go"
      - "Rename internal/cloudprovider/aws/warmup.go to warmup_script.go (it generates warmup scripts)"
      - "Rename internal/controller/lifecycle/warmup.go to warmup_monitor.go (it monitors warmup state)"
---

# Phase 1: Mechanical Cleanup Verification Report

**Phase Goal:** Every file has an unambiguous name, context flows correctly from reconciliation, errors are consistently wrapped, and internal functions are not exported

**Verified:** 2026-02-02T19:40:31Z

**Status:** gaps_found

**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth                                                      | Status          | Evidence                                                                                                               |
| --- | ---------------------------------------------------------- | --------------- | ---------------------------------------------------------------------------------------------------------------------- |
| 1   | No two files share an ambiguous name                       | ⚠️ PARTIAL      | reconcile.go merged, nodeclass.go disambiguated, but interface.go exists in 2 packages, warmup.go exists in 2 packages |
| 2   | Zero context.Background() in non-test production code      | ✓ VERIFIED      | 0 occurrences in internal/controller/\*.go and internal/cloudprovider/\*.go (excluding tests)                          |
| 3   | Every fmt.Errorf uses %w for error wrapping                | ✓ VERIFIED      | 96/146 fmt.Errorf calls use %w; remaining 50 are new errors (validation), not wrapping                                 |
| 4   | Internal event handlers and query helpers are unexported   | ✓ VERIFIED      | nodeEventHandler, nodeClassEventHandler, mapper types are lowercase; query helpers are receiver methods                |
| 5   | \_extracted/ directory gone and files have descriptive names | ✓ VERIFIED      | \_extracted/ deleted, all 8 controller files renamed (queries.go → node_queries.go, etc.)                               |

**Score:** 4/5 truths verified (1 partial)

### Required Artifacts

| Artifact                                       | Expected                                 | Status     | Details                                                                    |
| ---------------------------------------------- | ---------------------------------------- | ---------- | -------------------------------------------------------------------------- |
| `internal/controller/reconciler.go`            | Merged reconciler with full loop         | ✓ VERIFIED | 531 lines, contains Reconcile() and reconcileNodePool(), no reconcile.go  |
| `internal/controller/nodeclass_lifecycle.go`   | NodeClass lifecycle (was nodeclass.go)   | ✓ VERIFIED | 8992 bytes, getNodeClass() and lifecycle helpers                           |
| `internal/controller/provider_cache.go`        | Provider caching (was providers.go)      | ✓ VERIFIED | Contains ensureCloudProvider(ctx, ...) with context propagation            |
| `internal/controller/node_queries.go`          | Node query helpers (was queries.go)      | ✓ VERIFIED | getNodesForPool, getStandbyNodes, countNodesByState - all receiver methods |
| `internal/controller/nodepool_validation.go`   | NodePool validation (was validate.go)    | ✓ VERIFIED | validateNodePool receiver method                                           |
| `internal/controller/nodepool_status.go`       | Status helpers (was status.go)           | ✓ VERIFIED | setReadyCondition and condition helpers                                    |
| `internal/controller/pool_maintenance.go`      | Pool maintenance (was maintenance.go)    | ✓ VERIFIED | checkMaxNodeRuntime, replenishStandby - receiver methods                   |
| `internal/controller/cluster_config.go`        | Cluster config (was config.go)           | ✓ VERIFIED | ClusterConfig struct and validation                                        |
| `internal/cloudprovider/interface.go`          | CloudProvider interface                  | ⚠️ AMBIGUOUS | Generic name doesn't describe what interface it defines                    |
| `internal/controller/strategy/interface.go`    | ScalingStrategy interface                | ⚠️ AMBIGUOUS | Generic name doesn't describe what interface it defines                    |
| `internal/cloudprovider/aws/warmup.go`         | AWS warmup script generation             | ⚠️ AMBIGUOUS | Name collision with lifecycle/warmup.go, different concerns                |
| `internal/controller/lifecycle/warmup.go`      | Warmup state monitoring                  | ⚠️ AMBIGUOUS | Name collision with aws/warmup.go, different concerns                      |

### Key Link Verification

| From                        | To                    | Via                              | Status     | Details                                                                   |
| --------------------------- | --------------------- | -------------------------------- | ---------- | ------------------------------------------------------------------------- |
| reconciler.go               | provider_cache.go     | reconcileNodePool calls function | ✓ WIRED    | ensureCloudProvider(ctx, nodePool) called at line 146                     |
| provider_cache.go           | nodeclass_lifecycle.go | ensureCloudProvider calls        | ✓ WIRED    | getNodeClass(ctx, ref) called at line 67, context propagated              |
| provider_cache.go           | aws.NewAWSProvider    | Cloud provider initialization    | ✓ WIRED    | NewAWSProvider(ctx, ...) called at line 89, context propagated            |
| reconciler.go               | node_queries.go       | reconcileNodePool queries nodes  | ✓ WIRED    | getNodesForPool, getStandbyNodes called throughout reconciliation         |
| pool_maintenance.go         | node_queries.go       | Maintenance needs node counts    | ✓ WIRED    | getRunningNodes called from checkMaxNodeRuntime                           |

### Requirements Coverage

Phase 1 requirements from ROADMAP.md:

| Requirement | Status     | Blocking Issue                                                                         |
| ----------- | ---------- | -------------------------------------------------------------------------------------- |
| MECH-01     | ✓ SATISFIED | File naming: reconcile.go merged, 8 controller files renamed to descriptive names      |
| MECH-02     | ✓ SATISFIED | Context propagation: ensureCloudProvider(ctx, ...) flows to getNodeClass and AWS setup |
| MECH-03     | ✓ SATISFIED | Error wrapping: All 96 error-wrapping fmt.Errorf use %w, 50 new-error cases use %v    |
| MECH-04     | ✓ SATISFIED | Unexport internals: Event handlers lowercase, query helpers are receiver methods       |
| PKG-04      | ✓ SATISFIED | Dead code removed: \_extracted/ deleted, ExponentialBackoff deleted, linters clean      |

### Anti-Patterns Found

**Scan of files modified in Phase 1:**

No blocking anti-patterns found. No TODO/FIXME comments, no placeholder implementations, no console.log patterns.

The codebase passes `go build ./...` cleanly.

### Human Verification Required

None required for Phase 1 verification. All success criteria are structurally verifiable.

### Gaps Summary

**Gap: Ambiguous file names remain**

While the success criteria explicitly required resolving `reconcile.go/reconciler.go` and disambiguating the two `nodeclass.go` files (which was achieved), there are still ambiguous file names present:

1. **interface.go duplication**: Two files named `interface.go` exist in different packages:
   - `internal/cloudprovider/interface.go` - defines CloudProvider interface
   - `internal/controller/strategy/interface.go` - defines ScalingStrategy interface
   
   These names don't communicate what interface they define. A developer searching for "the scaling strategy interface" wouldn't think to look for a generic `interface.go`.

2. **warmup.go duplication**: Two files named `warmup.go` with completely different responsibilities:
   - `internal/cloudprovider/aws/warmup.go` - generates warmup scripts for EC2 instances
   - `internal/controller/lifecycle/warmup.go` - monitors and manages warmup state transitions
   
   This is a maintainability concern: changing warmup monitoring logic requires navigating to the correct `warmup.go`.

**Impact:** While the specific files called out in the success criteria were addressed (reconcile.go and nodeclass.go), the broader goal of "every file has an unambiguous name" is not fully achieved. These remaining ambiguities will cause confusion in later phases when developers need to locate and modify these files.

**Recommendation:** Rename these files to be descriptive of their specific purpose before proceeding to Phase 2, where lifecycle extraction will require clear boundaries between warmup script generation and warmup monitoring.

---

_Verified: 2026-02-02T19:40:31Z_
_Verifier: Claude (gsd-verifier)_
