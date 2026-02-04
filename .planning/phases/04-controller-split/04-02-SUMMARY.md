---
phase: 04-controller-split
plan: 02
subsystem: infra
tags: [controller-runtime, reconciler, nodeclass, nodepool, aggregator]

# Dependency graph
requires:
  - phase: 04-controller-split/01
    provides: nodepool/ package with Reconciler type and all NodePool files
  - phase: 03-strategy-package-extraction
    provides: strategy package used by nodepool reconciler
provides:
  - nodeclass/ package with independent NodeClass lifecycle reconciler
  - Aggregator setup.go registering all controllers
  - Decoupled nodepool and nodeclass packages (zero cross-imports)
  - Updated main.go using controller.Setup() aggregator
affects: [05-docs-alignment, 06-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns: [per-CRD controller package, aggregator setup.go, secondary watcher pattern]

key-files:
  created:
    - internal/controller/nodeclass/reconciler.go
    - internal/controller/nodeclass/setup.go
    - internal/controller/nodeclass/reconciler_test.go
    - internal/controller/setup.go
    - internal/controller/nodepool/nodeclass_fetch.go
  modified:
    - internal/controller/nodepool/reconciler.go
    - internal/controller/nodepool/setup.go
    - cmd/stratos/main.go
    - tests/integration/suite_test.go
    - internal/cloudprovider/aws/provider.go

key-decisions:
  - "NodeClass reconciler gets its own Reconcile() entry point with handleDeletion and reconcileLifecycle"
  - "NodePool watcher in nodeclass/setup.go maps NodePool events to referenced AWSNodeClass via nodePoolToNodeClassMapper"
  - "getInUseCondition and getValidCondition converted from receiver methods to package-level functions in nodeclass/"
  - "getNodeClass/getAWSNodeClass kept in nodepool/ as nodeclass_fetch.go (read-only fetchers needed by validation, provider cache, maintenance)"

patterns-established:
  - "Aggregator setup.go: controller/ root has only setup.go calling sub-package Setup() functions"
  - "Secondary watcher: nodeclass watches NodePool events to stay autonomously in sync"
  - "Per-CRD package independence: nodepool/ has zero imports from nodeclass/"

# Metrics
duration: 13min
completed: 2026-02-03
---

# Phase 4 Plan 2: NodeClass Package Extraction and Aggregator Summary

**NodeClass lifecycle controller extracted to nodeclass/ package with autonomous NodePool watcher, aggregator setup.go, and fully decoupled per-CRD controller packages**

## Performance

- **Duration:** 13 min
- **Started:** 2026-02-02T23:56:46Z
- **Completed:** 2026-02-03T00:10:21Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments
- Created nodeclass/ package with Reconciler, Reconcile(), handleDeletion(), reconcileLifecycle(), and 3 test files
- Built aggregator setup.go at controller/ root that registers nodepool.Setup() and nodeclass.Setup()
- Achieved full structural independence: nodepool/ has zero imports from nodeclass/
- NodeClass controller autonomously watches NodePool events via secondary watcher pattern
- Updated main.go to use controller.Setup() aggregator (no more direct NodePoolReconciler construction)
- Integration tests updated to use nodepool.Reconciler and register nodeclass.Setup()

## Task Commits

Each task was committed atomically:

1. **Task 1: Create nodeclass/ package from nodeclass_lifecycle.go** - `cf03fe3` (feat)
2. **Task 2: Create aggregator setup.go, update main.go, remove cross-package calls** - `a69ecee` (feat)
3. **Task 3: Update integration tests for new package structure** - `c0fcd06` (feat)

## Files Created/Modified
- `internal/controller/nodeclass/reconciler.go` - NodeClass Reconciler with lifecycle management
- `internal/controller/nodeclass/setup.go` - Setup() with NodePool secondary watcher
- `internal/controller/nodeclass/reconciler_test.go` - Unit tests for lifecycle reconciler
- `internal/controller/setup.go` - Aggregator registering nodepool.Setup() and nodeclass.Setup()
- `internal/controller/nodepool/nodeclass_fetch.go` - Read-only getNodeClass/getAWSNodeClass helpers
- `internal/controller/nodepool/reconciler.go` - Removed updateNodeClassLifecycle and cleanupNodeClassReference calls
- `internal/controller/nodepool/setup.go` - Added SetupOptions struct and Setup() entry point
- `cmd/stratos/main.go` - Uses controller.Setup(), config.ClusterConfig, config.DetectKubernetesVersion
- `tests/integration/suite_test.go` - Uses nodepool.Reconciler, registers nodeclass.Setup()
- `internal/cloudprovider/aws/provider.go` - Fixed stale comment reference

## Decisions Made
- NodeClass reconciler gets its own Reconcile() entry point with handleDeletion() and reconcileLifecycle() methods (not method forwarding from NodePool)
- getInUseCondition and getValidCondition converted from receiver methods to package-level functions since they don't need Reconciler state
- getNodeClass/getAWSNodeClass kept as read-only fetchers in nodepool/nodeclass_fetch.go since nodepool needs them for validation, provider cache creation, and pool maintenance (these are read queries, not lifecycle management)
- NodePool watcher in nodeclass/setup.go uses handler.EnqueueRequestsFromMapFunc with a simple mapper function (not a struct-based mapper) for cleaner code

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Created nodeclass_fetch.go in nodepool/ for read-only NodeClass fetchers**
- **Found during:** Task 2 (removing nodeclass_lifecycle.go from nodepool/)
- **Issue:** nodepool/ still needs getNodeClass() and getAWSNodeClass() for validation (checkNodeClassReady), provider cache creation, and pool maintenance -- these are read-only queries, not lifecycle management
- **Fix:** Created nodeclass_fetch.go with just the two fetch methods
- **Files modified:** internal/controller/nodepool/nodeclass_fetch.go
- **Verification:** go build ./internal/controller/nodepool/... passes
- **Committed in:** a69ecee (Task 2 commit)

**2. [Rule 1 - Bug] Fixed stale comment in aws/provider.go**
- **Found during:** Task 3 (verification scan for stale references)
- **Issue:** Comment said "controller.ClusterConfig" but type was moved to config package
- **Fix:** Updated comment to say "config.ClusterConfig"
- **Files modified:** internal/cloudprovider/aws/provider.go
- **Committed in:** c0fcd06 (Task 3 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both auto-fixes necessary for correctness. nodeclass_fetch.go is the minimal addition needed for nodepool/ to function independently.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 4 (Controller Split) is fully complete
- controller/ root has only setup.go (the aggregator)
- nodepool/ has 9 source files + 1 test file + lifecycle/ and nodestate/ sub-packages
- nodeclass/ has 3 files (reconciler.go, setup.go, reconciler_test.go)
- go build ./... and go vet ./... pass cleanly
- All unit tests pass across nodepool/, nodeclass/, config/, nodestate/
- Ready for Phase 5 (docs alignment) and Phase 6 (final cleanup)

---
*Phase: 04-controller-split*
*Completed: 2026-02-03*
