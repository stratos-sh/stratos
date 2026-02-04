---
phase: 18
plan: 01
subsystem: warmup-generation
tags: [warmup, bash-generation, image-pre-pull, containerd, ecr-auth]
requires:
  - phase: 17
    plan: 01
    what: ImagePullPolicy type from api/v1alpha1/config_types.go
provides:
  - GenerateScript function for warmup bash script generation
  - Bash script template with containerd readiness, ECR auth, retry, pinning
  - No-op script for empty image lists
affects:
  - phase: 19
    plan: TBD
    how: AMI generators will consume GenerateScript to inject image pulls into user data
tech-stack:
  added:
    - text/template (Go stdlib)
    - regexp (Go stdlib)
  patterns:
    - Flat sequential bash script generation (not bash functions)
    - Exponential backoff retry with ceiling
    - Policy-aware error handling (Required vs BestEffort)
    - ECR pattern matching for authentication detection
key-files:
  created:
    - internal/warmup/generator.go
    - internal/warmup/templates.go
    - internal/warmup/generator_test.go
  modified: []
key-decisions:
  - decision: Use text/template with template.Must for compile-time validation
    rationale: Template is constant, cannot fail at runtime; fail-fast at init
    impact: Generator returns string (not error), simpler API
  - decision: Flat sequential script, not bash functions
    rationale: Simpler to generate, easier to debug, matches CONTEXT.md decision
    impact: Template is longer but more straightforward
  - decision: Detect ECR images by regex pattern at generation time
    rationale: Avoids runtime regex in bash, cleaner template logic
    impact: HasECRImages and ECRRegion fields in templateData
  - decision: Return no-op script for empty image list
    rationale: Caller always gets valid bash, simplifies integration
    impact: AMI generators don't need special empty-list handling
duration: 188s
completed: 2026-02-04
---

# Phase 18 Plan 01: Warmup Script Generator Summary

**One-liner:** Bash script generator for containerd image pre-pull with ECR auth, exponential backoff retries, pinning labels, and Required/BestEffort policy handling

## Performance

**Duration:** 188 seconds (~3 minutes)
**Tasks completed:** 2/2
**Commits:** 2 (atomic per task)

## Accomplishments

Created the `internal/warmup/` package with a script generator that produces bash scripts for pulling container images during instance warmup. The generator supports:

1. **Containerd readiness detection** - Waits for socket and CRI responsiveness with 60s timeout
2. **ECR authentication** - Pattern matches `*.dkr.ecr.*.amazonaws.com` and fetches region-specific tokens via `aws ecr get-login-password`
3. **Retry with exponential backoff** - 3 attempts per image with 2s, 4s, 8s delays (max 30s ceiling)
4. **Image pinning** - Uses `io.cri-containerd.pinned=pinned` label to prevent kubelet garbage collection
5. **Policy-aware error handling** - Required policy exits 1 on failure, BestEffort always exits 0
6. **No-op script for empty lists** - Returns valid bash even when no images configured

The generator uses Go's `text/template` with `template.Must` for compile-time validation, ensuring runtime generation never fails. All script generation logic is decoupled from CRD types for clean separation.

## Task Commits

| Task | Description | Commit | Files |
|------|-------------|--------|-------|
| 1 | Create warmup script generator and template | 70c8b2c | generator.go, templates.go |
| 2 | Create unit tests for warmup script generator | 5ac8bbc | generator_test.go |

## Files Created

- `internal/warmup/generator.go` - GenerateScript function, templateData struct, ECR pattern matching
- `internal/warmup/templates.go` - Bash script template with all warmup logic
- `internal/warmup/generator_test.go` - 7 comprehensive test cases covering all scenarios

## Files Modified

None - new package created from scratch

## Decisions Made

### Template validation at init time
**Decision:** Use `template.Must` to parse template once at package init
**Rationale:** Template is a compile-time constant; parsing cannot fail at runtime
**Impact:** Generator function returns `string` (not `error`), simpler API for callers

### Flat sequential script structure
**Decision:** Generate flat bash script without functions
**Rationale:** Simpler to generate, easier to debug top-to-bottom, matches CONTEXT.md
**Impact:** Template is longer but more straightforward for AMI generator consumers

### Generation-time ECR detection
**Decision:** Scan images for ECR pattern in Go, pass `HasECRImages` to template
**Rationale:** Avoids runtime regex in bash, cleaner template conditional logic
**Impact:** Template uses `{{if .HasECRImages}}` instead of bash pattern matching

### No-op script for empty images
**Decision:** Return valid bash script (`#!/bin/bash\nexit 0`) when image list empty
**Rationale:** Caller always gets valid output, no special cases needed
**Impact:** AMI generators can unconditionally include the script

### Use aws CLI for ECR auth
**Decision:** Generate `aws ecr get-login-password` in script, not curl + IMDSv2
**Rationale:** AWS CLI is available on all EKS AMIs, simpler than signing API requests
**Impact:** Requires AWS CLI in environment (satisfied by EKS AMI base images)

## Deviations from Plan

None - plan executed exactly as written. All must-have truths validated:
- ✓ GenerateScript with Required policy produces exit 1 on failures
- ✓ GenerateScript with BestEffort policy always exits 0
- ✓ Empty images list returns no-op script (no ctr commands)
- ✓ Retry with exponential backoff (3 attempts, 2s/4s/8s)
- ✓ ECR detection via pattern and `aws ecr get-login-password`

## Issues Encountered

None

## User Setup Required

None - this is an internal package consumed by other Stratos components

## Next Phase Readiness

**Status:** READY

Phase 19 can proceed to integrate the generator into AMI user data injection. The generator API is stable:

```go
script := warmup.GenerateScript(images, policy)
// script is always valid bash, no error handling needed
```

**Integration points for Phase 19:**
1. AL2023 generator: Call `GenerateScript` and embed in systemd service (runs after nodeadm)
2. AL2 generator: Call `GenerateScript` and embed in user data shell script section
3. Pass `spec.warmup.images` and `spec.warmup.imagePullPolicy` from NodePool CRD

**No blockers.** The generator is fully tested and handles all edge cases:
- Empty image lists
- ECR-only images
- Non-ECR-only images
- Mixed ECR + non-ECR images
- Required vs BestEffort policies
