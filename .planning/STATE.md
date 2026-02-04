# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-04)

**Core value:** Kubernetes pod-driven scaling with no unnecessary abstraction layers
**Current focus:** v1.2 Warmup Image Pre-Pull -- Phase 19 (AMI Generator Integration)

## Current Position

Phase: 19 of 20 (AMI Generator Integration)
Plan: 2 of 2 in current phase
Status: Phase complete
Last activity: 2026-02-04 -- Completed 19-02-PLAN.md (Bottlerocket image pre-pull warning)

Progress: [===================.] 95% (milestone: [##..] 50%)

## Performance Metrics

**v1 Velocity:**
- Total plans completed: 13
- Average duration: 6min
- Total execution time: 83min

**v1.1 Velocity:**
- Total plans completed: 5
- Average duration: 2.1min
- Total execution time: ~11min

**v1.1.1 Velocity:**
- Total plans completed: 5
- Average duration: ~1.5min
- Total execution time: ~7min

**v1.2 Velocity:**
- Total plans completed: 3
- Average duration: ~3min
- Total execution time: ~8min

## Accumulated Context

### Decisions

- Image pre-pull config lives on NodePool spec (not AWSNodeClass) -- images are a workload concern
- Required is the default imagePullPolicy -- stricter ensures standby nodes have expected images
- Bottlerocket deferred -- different container runtime story
- Dynamic pending-pod image pull deferred -- no clean mechanism without API server access
- Use ctr exclusively (not crictl) -- crictl missing on AL2023, ctr universal on all EKS AMIs
- Image pinning with io.cri-containerd.pinned=pinned is mandatory -- prevents kubelet GC eviction
- items:MinLength=1 kubebuilder marker works in controller-gen v0.16.5 -- no named type fallback needed
- Flat sequential bash script for warmup (not functions) -- simpler to generate and debug
- Use text/template with template.Must for compile-time validation -- fail-fast at init
- ECR detection at generation time via regex -- cleaner template logic than bash pattern matching
- No-op script for empty images -- caller always gets valid bash
- ImagePrePullSupported condition is informational only -- non-blocking, does not prevent launch
- Condition removed (not set True) when not applicable -- keeps conditions list clean

### Pending Todos

None.

### Blockers/Concerns

None.

## Session Continuity

Last session: 2026-02-04
Stopped at: Completed 19-02-PLAN.md (Phase 19 complete). Ready for Phase 20.
Resume file: None
