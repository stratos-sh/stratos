# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-02-04)

**Core value:** Kubernetes pod-driven scaling with no unnecessary abstraction layers
**Current focus:** v1.2 Warmup Image Pre-Pull -- Phase 19 (AMI Generator Integration)

## Current Position

Phase: 19 of 20 (AMI Generator Integration)
Plan: 1 of 2 in current phase
Status: In progress
Last activity: 2026-02-04 -- Completed 19-01-PLAN.md (AL2/AL2023 MIME integration)

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
- Extract MIME utilities into shared mime.go -- both AL2 and AL2023 need MIME building
- AL2023 conditionally switches from plain YAML to MIME multipart -- backward compatible
- Size validation at generator level -- hard error at 16 KiB, warning at 14 KiB
- PreWarmConfig as pointer field on BootstrapConfig -- nil means no pre-warm configured

### Pending Todos

None.

### Blockers/Concerns

None.

## Session Continuity

Last session: 2026-02-04
Stopped at: Completed 19-01-PLAN.md. Ready for 19-02.
Resume file: None
