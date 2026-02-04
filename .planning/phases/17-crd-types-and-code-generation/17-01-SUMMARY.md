---
phase: 17-crd-types-and-code-generation
plan: 01
subsystem: api
tags: [kubebuilder, crd, controller-gen, deepcopy, validation]

# Dependency graph
requires:
  - phase: none
    provides: existing PreWarmConfig struct in config_types.go
provides:
  - ImagePullPolicy type with Required/BestEffort constants
  - PreWarmConfig.Images []string field with per-item MinLength validation
  - PreWarmConfig.ImagePullPolicy *ImagePullPolicy field with enum and default
  - GetImages() and GetImagePullPolicy() nil-safe getter methods
  - Updated CRD YAML with images and imagePullPolicy schema
  - Updated deepcopy handling Images slice and ImagePullPolicy pointer
affects: [18-warmup-script-generator, 19-ami-generator-integration, 20-controller-data-threading]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "items:MinLength kubebuilder marker for per-item slice validation"

key-files:
  created: []
  modified:
    - api/v1alpha1/config_types.go
    - api/v1alpha1/config_types_test.go
    - api/v1alpha1/zz_generated.deepcopy.go
    - deploy/charts/stratos/crds/stratos.sh_nodepools.yaml

key-decisions:
  - "PascalCase enum values (Required/BestEffort) for ImagePullPolicy -- locked decision from v1.2 research"
  - "items:MinLength=1 marker works in controller-gen v0.16.5 -- no fallback to named ImageRef type needed"

patterns-established:
  - "items: prefix markers for per-element slice validation in kubebuilder CRDs"

# Metrics
duration: 2min
completed: 2026-02-04
---

# Phase 17 Plan 01: CRD Types and Code Generation Summary

**ImagePullPolicy enum type with Required/BestEffort, PreWarmConfig images and policy fields, nil-safe getters, and regenerated deepcopy + CRD YAML**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-04T18:18:19Z
- **Completed:** 2026-02-04T18:20:09Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- ImagePullPolicy string type with Required and BestEffort constants (PascalCase)
- PreWarmConfig has Images []string with per-item minLength:1 validation and ImagePullPolicy pointer field with enum + default
- Nil-safe GetImages() (returns empty slice) and GetImagePullPolicy() (returns Required) getters
- Deepcopy correctly handles Images slice copy and ImagePullPolicy pointer copy
- CRD YAML contains images array with items minLength:1 and imagePullPolicy with Required default and Required/BestEffort enum
- 7 new unit tests all passing

## Task Commits

Each task was committed atomically:

1. **Task 1: Add ImagePullPolicy type, PreWarmConfig fields, and nil-safe getters** - `8d3a1ae` (feat)
2. **Task 2: Run code generation and verify CRD output** - `f8838b0` (chore)

## Files Created/Modified
- `api/v1alpha1/config_types.go` - ImagePullPolicy type, constants, PreWarmConfig fields, GetImages() and GetImagePullPolicy() methods
- `api/v1alpha1/config_types_test.go` - TestPreWarmConfig_GetImages (3 cases) and TestPreWarmConfig_GetImagePullPolicy (4 cases)
- `api/v1alpha1/zz_generated.deepcopy.go` - Auto-generated deepcopy for Images slice and ImagePullPolicy pointer
- `deploy/charts/stratos/crds/stratos.sh_nodepools.yaml` - CRD schema with images array and imagePullPolicy enum under preWarm

## Decisions Made
- PascalCase enum values (Required/BestEffort) -- locked decision from v1.2 research, different from lowercase TimeoutAction pattern
- `+kubebuilder:validation:items:MinLength=1` works correctly in controller-gen v0.16.5 -- no need for ImageRef named type fallback

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- ImagePullPolicy type and constants ready for Phase 18 warmup script generator (policy-aware failure handling)
- GetImages() and GetImagePullPolicy() nil-safe getters ready for Phase 20 controller data threading
- CRD accepts warmup image configuration, Phase 19 can reference the schema

---
*Phase: 17-crd-types-and-code-generation*
*Completed: 2026-02-04*
