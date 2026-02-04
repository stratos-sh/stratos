---
phase: 04-controller-split
plan: 01
subsystem: controller
tags: [go-packages, controller-runtime, reconciler, refactoring, import-paths]

# Dependency graph
requires:
  - phase: 03-strategy-package-extraction
    provides: "strategy/ package with kubernetes and githubactions sub-packages"
  - phase: 02-lifecycle-package-extraction
    provides: "lifecycle/ and nodestate/ sub-packages under controller/"
provides:
  - "internal/controller/nodepool/ package with Reconciler type (renamed from NodePoolReconciler)"
  - "internal/controller/nodepool/lifecycle/ and nodepool/nodestate/ sub-packages"
  - "internal/config/cluster_config.go with ClusterConfig type"
affects: [04-02-nodeclass-extraction, 04-03-aggregator-setup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Package-per-controller: each CRD gets its own package under internal/controller/"
    - "Anti-stutter naming: nodepool.Reconciler instead of nodepool.NodePoolReconciler"
    - "Config consolidation: ClusterConfig co-located with Config in internal/config/"

key-files:
  created:
    - internal/config/cluster_config.go
    - internal/config/cluster_config_test.go
    - internal/controller/nodepool/reconciler.go
    - internal/controller/nodepool/setup.go
    - internal/controller/nodepool/cloud_sync.go
    - internal/controller/nodepool/node_queries.go
    - internal/controller/nodepool/nodepool_status.go
    - internal/controller/nodepool/nodepool_validation.go
    - internal/controller/nodepool/pool_maintenance.go
    - internal/controller/nodepool/provider_cache.go
    - internal/controller/nodepool/pod_assignment_test.go
    - internal/controller/nodepool/nodeclass_lifecycle.go
  modified:
    - internal/controller/nodepool/lifecycle/manager.go
    - internal/controller/nodepool/lifecycle/node_launch.go
    - internal/controller/nodepool/lifecycle/node_startstop.go
    - internal/controller/nodepool/lifecycle/node_sync.go
    - internal/controller/nodepool/lifecycle/warmup_adoption.go
    - internal/controller/nodepool/lifecycle/warmup_handlers.go
    - internal/controller/nodepool/lifecycle/warmup_monitor.go
    - internal/strategy/kubernetes/network_readiness.go
    - internal/strategy/kubernetes/node_operations.go
    - internal/strategy/kubernetes/readiness.go
    - internal/strategy/kubernetes/readiness_test.go
    - internal/strategy/kubernetes/scale_calculator.go
    - internal/strategy/kubernetes/scale_calculator_test.go
    - internal/strategy/kubernetes/scale_down.go
    - internal/strategy/kubernetes/scale_up.go
    - internal/strategy/kubernetes/startup_taints.go
    - internal/strategy/kubernetes/startup_taints_test.go
    - internal/strategy/githubactions/strategy.go
    - tests/integration/controller_stop_test.go
    - tests/integration/error_handling_test.go
    - tests/integration/helpers_test.go
    - tests/integration/nodepool_test.go
    - tests/integration/scale_down_test.go
    - tests/integration/scale_up_test.go
    - tests/integration/startup_taints_test.go
    - tests/integration/state_transitions_test.go
    - tests/integration/suite_test.go
    - tests/e2e/e2e_test.go
    - tests/e2e/helpers_test.go

key-decisions:
  - "Created nodeclass_lifecycle.go copy in nodepool/ because Reconciler methods call getNodeClass/getAWSNodeClass; original stays in controller/ root for Plan 02"
  - "Used temporary old import paths in Task 2 (controller/nodestate, controller/lifecycle) since directories hadn't moved yet; Task 3 updated all paths"
  - "Import alias crconfig for controller-runtime config to avoid conflict with package name config"

patterns-established:
  - "Package-per-controller: nodepool.Reconciler is the canonical pattern for CRD packages"
  - "Config package consolidation: ClusterConfig + Config both in internal/config/"

# Metrics
duration: 11min
completed: 2026-02-03
---

# Phase 4 Plan 1: NodePool Package Foundation Summary

**Created internal/controller/nodepool/ package with Reconciler type, 10 source files, lifecycle/ and nodestate/ sub-packages, plus ClusterConfig in internal/config/**

## Performance

- **Duration:** 11 min
- **Tasks:** 3/3
- **Files modified:** 44

## Accomplishments
- Moved ClusterConfig type to internal/config/ alongside existing Config type, establishing the config consolidation pattern
- Created internal/controller/nodepool/ package with Reconciler type (renamed from NodePoolReconciler) and all 8 delegate files plus 1 test file
- Relocated lifecycle/ and nodestate/ sub-packages under nodepool/ and updated all 22 consumer import paths across strategy/, tests/integration/, and tests/e2e/

## Task Commits

Each task was committed atomically:

1. **Task 1: Move cluster_config.go to internal/config/** - `f8aeace` (feat)
2. **Task 2: Create nodepool/ package and move all NodePool source files** - `2a57fed` (feat)
3. **Task 3: Move lifecycle/ and nodestate/ under nodepool/** - `b720fdf` (feat)

## Files Created/Modified
- `internal/config/cluster_config.go` - ClusterConfig type, Validate(), DetectKubernetesVersion()
- `internal/config/cluster_config_test.go` - Unit tests for ClusterConfig validation and version parsing
- `internal/controller/nodepool/reconciler.go` - Reconciler type with Reconcile() and reconcileNodePool() main loop
- `internal/controller/nodepool/setup.go` - SetupWithManager() and event handlers
- `internal/controller/nodepool/cloud_sync.go` - syncNodesWithCloud(), warmup monitoring
- `internal/controller/nodepool/node_queries.go` - Node listing and counting by state
- `internal/controller/nodepool/nodepool_status.go` - Ready/Degraded condition setters
- `internal/controller/nodepool/nodepool_validation.go` - NodePool and NodeClass validation
- `internal/controller/nodepool/pool_maintenance.go` - Max runtime checks, standby replenishment
- `internal/controller/nodepool/provider_cache.go` - Cloud provider and strategy caching
- `internal/controller/nodepool/nodeclass_lifecycle.go` - NodeClass CRUD operations for Reconciler
- `internal/controller/nodepool/pod_assignment_test.go` - Pod assignment logic tests
- `internal/controller/nodepool/lifecycle/*` - 7 source + 1 test file (nodestate import paths updated)
- `internal/controller/nodepool/nodestate/*` - 1 source + 1 test file (pure leaf, no changes needed)

## Decisions Made
1. **nodeclass_lifecycle.go duplication** - Created a copy of nodeclass_lifecycle.go in nodepool/ with receiver changed to Reconciler, because the Reconciler methods (getNodeClass, getAWSNodeClass, updateNodeClassLifecycle, cleanupNodeClassReference) are called during reconciliation. The original stays in controller/ root (broken, referencing NodePoolReconciler) for Plan 02 to handle.
2. **Import alias for config package** - Used `crconfig` alias for the controller-runtime config import (`sigs.k8s.io/controller-runtime/pkg/client/config`) to avoid name collision with the `config` package name.
3. **Temporary import paths in Task 2** - Used old import paths (`controller/nodestate`, `controller/lifecycle`) in Task 2 since those directories hadn't moved yet. Task 3 updated all paths to `controller/nodepool/nodestate` and `controller/nodepool/lifecycle`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Created nodeclass_lifecycle.go in nodepool/ package**
- **Found during:** Task 2 (Create nodepool/ package)
- **Issue:** Plan listed 8 source files to move but the Reconciler methods call getNodeClass(), getAWSNodeClass(), updateNodeClassLifecycle(), cleanupNodeClassReference() which are defined in nodeclass_lifecycle.go. Without these methods, the nodepool package would not compile.
- **Fix:** Created internal/controller/nodepool/nodeclass_lifecycle.go with receiver changed from NodePoolReconciler to Reconciler. Original stays in controller/ root for Plan 02.
- **Files modified:** internal/controller/nodepool/nodeclass_lifecycle.go (created)
- **Verification:** `go build ./internal/controller/nodepool/...` compiles cleanly
- **Committed in:** 2a57fed (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Essential for compilation. The nodeclass_lifecycle.go methods are direct dependencies of the Reconciler; without them the nodepool package cannot compile. No scope creep.

## Issues Encountered
1. **Import path sequencing** - Task 2 files needed to import nodepool/nodestate and nodepool/lifecycle, but those directories did not exist yet (created in Task 3). Resolved by temporarily using old import paths in Task 2, then updating all paths in Task 3.
2. **Git staging for untracked files** - `git rm internal/controller/cloud_sync.go` failed because cloud_sync.go was an untracked new file from a previous phase. Resolved by using `git add` to stage the working tree state.
3. **Edit tool on copied files** - Files copied via `cp` command had to be explicitly Read before Edit tool would accept modifications. Required extra Read calls for 7 lifecycle files.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- nodepool/ package is fully established with all sub-packages compiling independently
- Plan 02 can now extract nodeclass_lifecycle.go into its own nodeclass/ package
- Plan 03 can create the aggregator setup.go that wires nodepool.Reconciler into the manager
- Full `go build ./...` deferred to Plan 02 (main.go and integration tests still reference old types)

---
*Phase: 04-controller-split*
*Completed: 2026-02-03*
