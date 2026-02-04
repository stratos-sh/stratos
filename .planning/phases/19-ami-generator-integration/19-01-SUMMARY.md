---
phase: 19
plan: 01
subsystem: userdata-generation
tags: [mime, al2, al2023, image-pre-pull, userdata, warmup-integration]
requires:
  - phase: 18
    plan: 01
    what: warmup.GenerateScript function for image pull bash script generation
  - phase: 17
    plan: 01
    what: PreWarmConfig type with Images, ImagePullPolicy fields and nil-safe getters
provides:
  - Shared MIME utilities (mimePartShellScript, mimePartNodeConfig, buildMIMEMultipart)
  - AL2 generator with conditional image-pull MIME part
  - AL2023 generator with conditional MIME multipart (plain YAML vs MIME)
  - User data size validation (16 KiB hard limit, 14 KiB warning)
affects:
  - phase: 19
    plan: 02
    how: Provider launch path must populate PreWarmConfig from NodePool spec
  - phase: 20
    plan: TBD
    how: Controller integration will pass PreWarmConfig when calling GenerateUserData
tech-stack:
  added: []
  patterns:
    - Conditional MIME multipart generation based on configuration
    - Shared MIME utilities extracted from per-family generators
    - Size validation with hard limit error and soft threshold warning
key-files:
  created:
    - internal/cloudprovider/aws/mime.go
  modified:
    - internal/cloudprovider/aws/userdata.go
    - internal/cloudprovider/aws/al2.go
    - internal/cloudprovider/aws/al2023.go
    - internal/cloudprovider/aws/userdata_test.go
key-decisions:
  - decision: Extract MIME utilities into shared mime.go
    rationale: Both AL2 and AL2023 need MIME building; eliminates duplication
    impact: Single source of truth for MIME format, easier to maintain
  - decision: AL2023 conditionally switches from plain YAML to MIME multipart
    rationale: nodeadm reads plain YAML by default; MIME only needed when shell scripts must run
    impact: Backward compatible -- no images means identical output to before
  - decision: checkUserDataSize returns error on hard limit, prints warning on threshold
    rationale: EC2 rejects >16 KiB; early feedback prevents launch failures
    impact: Generators fail fast before attempting EC2 API call
  - decision: PreWarmConfig is a pointer field on BootstrapConfig
    rationale: nil means no pre-warm configured; avoids ambiguity with empty struct
    impact: Callers check nil + len(Images) > 0 to determine whether to inject scripts
duration: 196s
completed: 2026-02-04
---

# Phase 19 Plan 01: AL2/AL2023 MIME Integration Summary

**One-liner:** Integrate warmup script generator into AL2/AL2023 user data as conditional MIME parts with shared utilities and 16 KiB size validation

## Performance

**Duration:** 196 seconds (~3 minutes)
**Tasks completed:** 2/2
**Commits:** 2 (atomic per task)

## Accomplishments

Integrated the Phase 18 warmup script generator into both AL2 and AL2023 user data generators, with conditional image pull injection and shared MIME utilities.

1. **Shared MIME utilities (mime.go)** - Extracted `mimePartShellScript` and `buildMIMEMultipart` from al2023.go into shared mime.go. Added new `mimePartNodeConfig` for AL2023's `application/node.eks.aws` content type. Added `checkUserDataSize` with 16 KiB hard error and 14 KiB warning threshold.

2. **AL2 generator** - Conditionally injects image-pull.sh MIME part between bootstrap.sh and stratos-warmup.sh when `PreWarmConfig.Images` is non-empty. Preserves 2-part MIME (bootstrap + warmup) when no images configured. Supports 4-part ordering: bootstrap, image-pull, warmup, custom-userdata.

3. **AL2023 generator** - Returns plain NodeConfig YAML when no images (unchanged behavior). Switches to 3-part MIME multipart (NodeConfig + image-pull + warmup) when images configured. Uses `application/node.eks.aws` content type for NodeConfig part so nodeadm processes it correctly.

4. **BootstrapConfig** - Added `PreWarmConfig *stratosv1alpha1.PreWarmConfig` pointer field for optional image pre-pull configuration.

5. **Comprehensive tests** - 8 new test functions covering all paths: AL2 with/without/empty images, AL2023 with/without/empty images, MIME part ordering, size limit enforcement. All 28 tests pass.

## Task Commits

| Task | Description | Commit | Files |
|------|-------------|--------|-------|
| 1 | Extract MIME utilities and integrate image pre-pull | e1c76a0 | mime.go, userdata.go, al2.go, al2023.go |
| 2 | Add AL2/AL2023 image pre-pull integration tests | f3ad71a | userdata_test.go |

## Files Created

- `internal/cloudprovider/aws/mime.go` - Shared MIME utilities: mimePartShellScript, mimePartNodeConfig, buildMIMEMultipart, checkUserDataSize

## Files Modified

- `internal/cloudprovider/aws/userdata.go` - Added PreWarmConfig field to BootstrapConfig
- `internal/cloudprovider/aws/al2.go` - Conditional image-pull.sh MIME part injection with size check
- `internal/cloudprovider/aws/al2023.go` - Conditional MIME multipart when images configured; removed MIME functions (moved to mime.go)
- `internal/cloudprovider/aws/userdata_test.go` - 8 new test functions for image pre-pull integration

## Decisions Made

### Extract MIME utilities into shared mime.go
**Decision:** Move mimePartShellScript and buildMIMEMultipart from al2023.go to mime.go, add mimePartNodeConfig
**Rationale:** Both AL2 and AL2023 need MIME building; AL2023 was the only definition point but AL2 consumed them too
**Impact:** Clean separation; single source of truth for MIME format

### AL2023 conditional format switch
**Decision:** Plain YAML when no images, MIME multipart when images configured
**Rationale:** nodeadm expects plain NodeConfig YAML; MIME is only needed when shell scripts must execute
**Impact:** Fully backward compatible -- existing AL2023 user data is identical

### Size validation at generator level
**Decision:** Hard error at 16 KiB, stderr warning at 14 KiB (85% threshold)
**Rationale:** EC2 rejects user data over 16 KiB; early detection prevents cryptic launch failures
**Impact:** Generators fail fast with descriptive error including size, limit, and remediation suggestion

### PreWarmConfig as pointer field
**Decision:** `PreWarmConfig *stratosv1alpha1.PreWarmConfig` (pointer, not value)
**Rationale:** nil clearly means "no pre-warm configured"; distinguishes from empty PreWarmConfig struct
**Impact:** Callers must nil-check before accessing, but intent is unambiguous

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - internal package changes consumed by other Stratos components

## Next Phase Readiness

**Status:** READY

Plan 19-02 can proceed to wire PreWarmConfig through the provider launch path. The integration points are:
- `BootstrapConfig.PreWarmConfig` field is ready to be populated from `NodePool.Spec.PreWarm`
- AL2 and AL2023 generators automatically detect images and inject MIME parts
- Size validation happens automatically at generation time
