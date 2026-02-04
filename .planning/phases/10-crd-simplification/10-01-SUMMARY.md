---
phase: 10-crd-simplification
plan: 01
subsystem: api
tags: [crd, kubebuilder, controller-gen, deepcopy, nodepool]

# Dependency graph
requires:
  - phase: 09-strategy-deletion
    provides: "Strategy packages deleted, no runtime references to old strategy types"
provides:
  - "Simplified NodePoolSpec without ScalingStrategy or GitHubActions fields"
  - "Clean deepcopy without strategy type methods"
  - "CRD manifest reflecting simplified spec"
  - "TemplateConfig without SkipKubernetesBootstrap field"
affects: [11-final-cleanup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "+kubebuilder:object:generate=false marker for interface types"

key-files:
  created: []
  modified:
    - "api/v1alpha1/nodepool_types.go"
    - "api/v1alpha1/nodeclass.go"
    - "api/v1alpha1/zz_generated.deepcopy.go"
    - "internal/cloudprovider/interface.go"
    - "internal/cloudprovider/aws/provider.go"
    - "internal/controller/nodepool/nodepool_validation.go"
    - "internal/controller/nodepool/lifecycle/node_launch.go"
    - "internal/controller/nodepool/lifecycle/manager.go"
    - "internal/controller/nodepool/doc.go"
    - "deploy/charts/stratos/crds/stratos.sh_nodepools.yaml"

key-decisions:
  - "Added +kubebuilder:object:generate=false to NodeClass interface to fix controller-gen deepcopy generation error"

patterns-established:
  - "Interface types in api/ package need +kubebuilder:object:generate=false marker"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 10 Plan 01: CRD Simplification Summary

**Removed ScalingStrategy/GitHubActions fields from NodePool CRD, deleted strategy_types.go, cleaned SkipKubernetesBootstrap from cloud provider, regenerated deepcopy and CRD manifests**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-03T21:13:10Z
- **Completed:** 2026-02-03T21:16:17Z
- **Tasks:** 2
- **Files modified:** 10 (+ 1 deleted)

## Accomplishments
- Deleted api/v1alpha1/strategy_types.go (ScalingStrategyType, GitHubActionsConfig, SecretReference types)
- Removed ScalingStrategy and GitHubActions fields from NodePoolSpec struct
- Removed SkipKubernetesBootstrap from TemplateConfig and AWS provider setUserData
- Removed GHA validation block from nodepool_validation.go
- Regenerated deepcopy and CRD manifests cleanly
- All unit tests pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Delete strategy types and remove all references** - `3c60070` (feat)
2. **Task 2: Regenerate code and verify full build** - `b624b4a` (chore)

## Files Created/Modified
- `api/v1alpha1/strategy_types.go` - DELETED (contained ScalingStrategyType, GitHubActionsConfig, SecretReference)
- `api/v1alpha1/nodepool_types.go` - Removed ScalingStrategy and GitHubActions fields from NodePoolSpec
- `api/v1alpha1/nodeclass.go` - Added +kubebuilder:object:generate=false marker
- `api/v1alpha1/zz_generated.deepcopy.go` - Regenerated without strategy types
- `internal/cloudprovider/interface.go` - Removed SkipKubernetesBootstrap from TemplateConfig
- `internal/cloudprovider/aws/provider.go` - Removed SkipKubernetesBootstrap branch from setUserData
- `internal/controller/nodepool/nodepool_validation.go` - Removed GHA validation block
- `internal/controller/nodepool/lifecycle/node_launch.go` - Removed SkipKubernetesBootstrap line from TemplateConfig init
- `internal/controller/nodepool/lifecycle/manager.go` - Updated NodeHooks comment
- `internal/controller/nodepool/doc.go` - Updated demand checking comment
- `deploy/charts/stratos/crds/stratos.sh_nodepools.yaml` - Regenerated CRD manifest

## Decisions Made
- Added `+kubebuilder:object:generate=false` marker to the `NodeClass` interface in `nodeclass.go`. controller-gen was attempting to generate deepcopy methods for the NodeClass interface type (which embeds `client.Object` with `DeepCopyObject()`). Since interfaces cannot have method receivers in Go, this caused a generation failure. The marker tells controller-gen to skip this type.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed controller-gen failure on NodeClass interface**
- **Found during:** Task 2 (make generate)
- **Issue:** controller-gen tried to generate deepcopy for the NodeClass interface type, which is invalid in Go (interfaces cannot have pointer receivers). The stale zz_generated.deepcopy.go had `*NodeClass` methods that compiled before but prevented regeneration.
- **Fix:** Added `+kubebuilder:object:generate=false` marker to the NodeClass interface declaration in `api/v1alpha1/nodeclass.go`
- **Files modified:** api/v1alpha1/nodeclass.go
- **Verification:** `make generate` exits 0, `go build ./...` passes
- **Committed in:** b624b4a (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Fix was necessary for `make generate` to succeed. No scope creep.

## Issues Encountered
None beyond the deviation documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CRD simplification complete -- NodePoolSpec is clean of all strategy abstractions
- Ready for Phase 11 (final cleanup) if any remaining tasks exist
- All success criteria from the roadmap are satisfied

---
*Phase: 10-crd-simplification*
*Completed: 2026-02-03*
