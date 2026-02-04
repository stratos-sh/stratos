# Phase 19: AMI Generator Integration - Context

**Gathered:** 2026-02-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Inject the image pull script (from Phase 18's warmup script generator) into EC2 user data for AL2 and AL2023 instances. Produce a validation warning for Bottlerocket. This phase does NOT add new AMI family support or change the warmup script generator itself.

</domain>

<decisions>
## Implementation Decisions

### AL2023 MIME format
- Use MIME multipart only when images are configured; plain NodeConfig YAML when no images
- Shell script MIME part uses `text/x-shellscript` content type
- NodeConfig part uses `application/node.eks.aws` content type
- Shared MIME builder utility used by both AL2 and AL2023 generators
- Fixed MIME boundary string (deterministic output, easier to test)

### Script placement in boot sequence
- Image pull script is a separate MIME part (not appended to bootstrap script)
- 3 MIME parts when images present: bootstrap/NodeConfig, image pull, warmup completion
- 2 MIME parts when no images: bootstrap/NodeConfig, warmup completion (image pull omitted)
- Same 3-part pattern for both AL2 and AL2023 when images are configured
- Phase 18's built-in containerd readiness wait is sufficient — no additional ordering guards needed

### Bottlerocket warning
- Surface warning as a status condition on the NodePool (e.g., ImagePrePullNotSupported)
- Controller proceeds with launching Bottlerocket instances without image pre-pull (degraded but functional)
- AMI family detection mechanism: Claude's discretion (investigate existing codebase)
- Bottlerocket image pre-pull support is a separate future phase; status condition stays until that ships

### Claude's Discretion
- AMI family detection mechanism (investigate existing codebase patterns)
- Exact status condition type name and message wording
- MIME builder API design (shared utility structure)
- Size limit warning threshold and implementation

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 19-ami-generator-integration*
*Context gathered: 2026-02-04*
