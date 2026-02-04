# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-04)

**Core value:** Kubernetes pod-driven scaling with no unnecessary abstraction layers
**Current focus:** v1.2 Warmup Image Pre-Pull -- Phase 17 (CRD Types and Code Generation)

## Current Position

Phase: 17 of 20 (CRD Types and Code Generation)
Plan: 0 of TBD in current phase
Status: Ready to plan
Last activity: 2026-02-04 -- Roadmap created for v1.2 (4 phases, 22 requirements)

Progress: [==================..] 87% (milestone: [....] 0%)

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

## Accumulated Context

### Decisions

- Image pre-pull config lives on NodePool spec (not AWSNodeClass) -- images are a workload concern
- Required is the default imagePullPolicy -- stricter ensures standby nodes have expected images
- Bottlerocket deferred -- different container runtime story
- Dynamic pending-pod image pull deferred -- no clean mechanism without API server access
- Use ctr exclusively (not crictl) -- crictl missing on AL2023, ctr universal on all EKS AMIs
- Image pinning with io.cri-containerd.pinned=pinned is mandatory -- prevents kubelet GC eviction

### Pending Todos

None.

### Blockers/Concerns

None.

## Session Continuity

Last session: 2026-02-04
Stopped at: Roadmap created for v1.2 milestone. Ready to plan Phase 17.
Resume file: None
