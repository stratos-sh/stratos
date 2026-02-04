---
phase: 20-controller-data-threading
plan: 01
subsystem: controller
tags: [prewarm, data-threading, userdata, image-pre-pull]
requires:
  - phase-17 (CRD types with PreWarmConfig on NodePool spec)
  - phase-18 (warmup script generator)
  - phase-19 (AMI generator integration with MIME/size validation)
provides:
  - End-to-end data flow from NodePool.Spec.PreWarm to user data generation
  - PreWarmConfig field on TemplateConfig for cloud-agnostic threading
  - checkImagePrePullSupport wired in reconcile loop
affects:
  - None (final wiring phase, all prior components are complete)
tech-stack:
  added: []
  patterns:
    - Pointer passthrough for nil-safe optional config (PreWarmConfig)
    - Non-blocking condition check early in reconcile loop
key-files:
  created:
    - internal/controller/nodepool/lifecycle/node_launch_test.go
    - internal/cloudprovider/aws/provider_test.go
  modified:
    - internal/cloudprovider/interface.go
    - internal/controller/nodepool/lifecycle/node_launch.go
    - internal/cloudprovider/aws/provider.go
    - internal/controller/nodepool/reconciler.go
key-decisions:
  - PreWarmConfig as pointer field on TemplateConfig -- nil passthrough is safe and backward compatible
  - checkImagePrePullSupport uses separate ncErr variable to avoid shadowing existing err
  - NodeClass fetch error in reconcile is silently ignored (already validated in Reconcile before reconcileNodePool)
duration: ~3min
completed: 2026-02-04
---

# Phase 20 Plan 01: Controller Data Threading Summary

Thread PreWarmConfig end-to-end from NodePool CRD through TemplateConfig to BootstrapConfig via 4 surgical wiring points plus comprehensive tests.

## Performance

- **Duration:** ~3min
- **Started:** 2026-02-04T20:44:36Z
- **Completed:** 2026-02-04T20:47:31Z
- **Tasks:** 2/2
- **Files modified:** 4
- **Files created:** 2

## Accomplishments

1. Connected PreWarmConfig field through the entire data flow path:
   - `TemplateConfig.PreWarmConfig` added to cloud-agnostic interface
   - `LaunchNode` populates it from `pool.Spec.PreWarm`
   - `generateEncodedUserData` passes it to `BootstrapConfig`
   - `reconcileNodePool` calls `checkImagePrePullSupport` on every reconcile

2. Added 6 focused tests verifying the data threading:
   - LaunchNode passes PreWarmConfig through to launcher (spy pattern)
   - LaunchNode handles nil PreWarmConfig safely
   - generateEncodedUserData produces MIME multipart with ctr commands (AL2023)
   - generateEncodedUserData produces plain YAML without PreWarmConfig
   - Nil templateConfig backward compatibility
   - AL2 PreWarmConfig produces image-pull.sh MIME part

## Task Commits

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Wire PreWarmConfig through data flow path | 26dc65e | interface.go, node_launch.go, provider.go, reconciler.go |
| 2 | Add end-to-end data threading tests | 7ea2a4e | node_launch_test.go, provider_test.go |

## Files Created

- `internal/controller/nodepool/lifecycle/node_launch_test.go` -- Tests LaunchNode PreWarmConfig passthrough
- `internal/cloudprovider/aws/provider_test.go` -- Tests generateEncodedUserData with/without PreWarmConfig

## Files Modified

- `internal/cloudprovider/interface.go` -- Added PreWarmConfig field to TemplateConfig struct
- `internal/controller/nodepool/lifecycle/node_launch.go` -- Populates PreWarmConfig from pool.Spec.PreWarm
- `internal/cloudprovider/aws/provider.go` -- Passes PreWarmConfig to BootstrapConfig
- `internal/controller/nodepool/reconciler.go` -- Wires checkImagePrePullSupport in reconcileNodePool

## Decisions Made

| Decision | Rationale |
|----------|-----------|
| PreWarmConfig as pointer on TemplateConfig | Nil passthrough is safe; backward compatible with existing callers that pass nil |
| Separate ncErr variable for NodeClass fetch | Avoids shadowing ctx err; keeps error handling clean |
| Silent ignore of NodeClass fetch error in reconcile | NodeClass readiness already validated by checkNodeClassReady before reconcileNodePool is called |

## Deviations from Plan

None -- plan executed exactly as written.

## Issues Encountered

None.

## Next Phase Readiness

Phase 20 is the final phase. With this plan complete:
- v1.2 warmup image pre-pull feature is fully wired end-to-end
- NodePool.Spec.PreWarm.Images flows to user data with ctr pull commands
- NodePools without PreWarm are unaffected (backward compatible)
- Bottlerocket gets ImagePrePullSupported=False condition
- All tests pass (existing + new)
