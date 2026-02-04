---
phase: 01-mechanical-cleanup
plan: 02
subsystem: controller, strategy, cloudprovider
tags: [go, unexport, api-surface, encapsulation, refactoring]

# Dependency graph
requires:
  - phase: 01-mechanical-cleanup plan 01
    provides: renamed files with clear subject_role.go naming convention
provides:
  - Reduced exported API surface across controller, strategy, nodestate, and aws packages
  - Package-internal symbols locked down before structural refactoring
affects: [01-mechanical-cleanup plan 03, 02-state-machine-extraction]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Only export symbols used outside their package"
    - "Test functions for unexported symbols use Test_ prefix (e.g., Test_parseKubernetesVersion)"

key-files:
  modified:
    - internal/controller/setup.go
    - internal/controller/cluster_config.go
    - internal/controller/cluster_config_test.go
    - internal/controller/strategy/kubernetes.go
    - internal/controller/strategy/kubernetes_drain.go
    - internal/controller/strategy/kubernetes_events.go
    - internal/controller/strategy/kubernetes_network.go
    - internal/controller/strategy/kubernetes_network_test.go
    - internal/controller/nodestate/nodestate.go
    - internal/cloudprovider/aws/warmup.go
    - internal/cloudprovider/aws/ami.go
    - internal/cloudprovider/aws/ami_test.go
    - internal/cloudprovider/aws/al2.go
    - internal/cloudprovider/aws/userdata_test.go
    - internal/cloudprovider/aws/nodeclass_controller.go

key-decisions:
  - "IsPodUnschedulable exported wrapper removed entirely (already had iPodUnschedulable lowercase)"
  - "Test functions for unexported symbols use Test_ prefix to satisfy Go naming convention"

patterns-established:
  - "Unexport symbols that are only used within their own package"
  - "Go test naming: Test_lowerCase for tests of unexported functions"

# Metrics
duration: 5min
completed: 2026-02-02
---

# Phase 1 Plan 2: Unexport Internal Symbols Summary

**17 symbols unexported across 4 packages (controller, strategy, nodestate, aws) to lock down public API surface before structural refactoring**

## Performance

- **Duration:** 5 min
- **Started:** 2026-02-02T19:31:06Z
- **Completed:** 2026-02-02T19:36:29Z
- **Tasks:** 2
- **Files modified:** 15

## Accomplishments
- 6 controller package symbols unexported (NodeEventHandler, NodeClassEventHandler, NodeToNodePoolMapper, NodeClassToNodePoolMapper, ParseKubernetesVersion)
- 8 strategy package symbols unexported (IsPodUnschedulable removed, IsNodeEmpty, DrainHelper, DrainConfig, DefaultDrainConfig, NewDrainHelper, NetworkReadinessChecker, NewNetworkReadinessChecker)
- 1 nodestate symbol unexported (ValidTransitions)
- 2 aws symbols unexported (GetWarmupScript, DefaultAMISelector)
- `go build ./...` passes cleanly, all test files compile

## Task Commits

Each task was committed atomically:

1. **Task 1: Unexport controller package symbols** - `eeb3960` (refactor)
2. **Task 2: Unexport strategy, nodestate, and aws package symbols** - `4fe676e` (refactor)

## Files Created/Modified
- `internal/controller/setup.go` - Unexported event handlers and mapper types
- `internal/controller/cluster_config.go` - Unexported parseKubernetesVersion
- `internal/controller/cluster_config_test.go` - Updated test references to use lowercase names
- `internal/controller/strategy/kubernetes.go` - Updated call sites for unexported drain/network helpers
- `internal/controller/strategy/kubernetes_drain.go` - Unexported DrainHelper, DrainConfig, DefaultDrainConfig, NewDrainHelper, IsNodeEmpty
- `internal/controller/strategy/kubernetes_events.go` - Removed exported IsPodUnschedulable wrapper
- `internal/controller/strategy/kubernetes_network.go` - Unexported NetworkReadinessChecker and NewNetworkReadinessChecker
- `internal/controller/strategy/kubernetes_network_test.go` - Updated test references
- `internal/controller/nodestate/nodestate.go` - Unexported ValidTransitions
- `internal/cloudprovider/aws/warmup.go` - Unexported GetWarmupScript
- `internal/cloudprovider/aws/ami.go` - Unexported DefaultAMISelector
- `internal/cloudprovider/aws/ami_test.go` - Updated test references
- `internal/cloudprovider/aws/al2.go` - Updated call site for getWarmupScript
- `internal/cloudprovider/aws/userdata_test.go` - Updated test reference for getWarmupScript
- `internal/cloudprovider/aws/nodeclass_controller.go` - Updated call site for defaultAMISelector

## Decisions Made
- Removed `IsPodUnschedulable` exported wrapper entirely since `isPodUnschedulable` (lowercase) already existed and was used by all callers
- Used `Test_` prefix (e.g., `Test_parseKubernetesVersion`) for test functions testing unexported symbols, satisfying Go's naming convention that requires uppercase after `Test`
- Verified every symbol via grep before unexporting to prevent breaking external references

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed ensureCloudProvider missing ctx argument**
- **Found during:** Task 1 (build verification)
- **Issue:** `reconciler.go` line 146 called `r.ensureCloudProvider(nodePool)` but the function signature requires `(ctx, nodePool)` -- pre-existing bug from Plan 01 file merge
- **Fix:** Added `ctx` argument to the call site
- **Files modified:** `internal/controller/reconciler.go`
- **Verification:** `go build ./...` passes
- **Committed in:** `eeb3960` (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Auto-fix was necessary for compilation. No scope creep.

## Issues Encountered
- Go test naming convention requires first letter after `Test` to be uppercase -- `TestparseKubernetesVersion` and `TestdefaultAMISelector` were invalid. Fixed with `Test_` prefix pattern.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All internal symbols locked down across controller, strategy, nodestate, and aws packages
- Public API surface is now minimal: only symbols used by main.go, integration tests, and cross-package imports remain exported
- Ready for Plan 03 (final mechanical cleanup tasks)

---
*Phase: 01-mechanical-cleanup*
*Completed: 2026-02-02*
