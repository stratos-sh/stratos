# Phase 18: Warmup Script Generator - Context

**Gathered:** 2026-02-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Generate bash scripts that pull configured container images using `ctr`, with ECR auth, retry logic, image pinning, and policy-aware failure handling. The generator is a Go package that produces script strings consumed by AMI generators (Phase 19). Instance user data injection and controller wiring are separate phases.

</domain>

<decisions>
## Implementation Decisions

### Script structure
- Flat sequential script, not bash functions — simpler to generate, easier to debug top-to-bottom
- When no images are configured, generator returns a no-op script (valid bash that exits 0 with a comment) — caller always gets a valid script
- 3 retries per image with exponential backoff, max 30s ceiling — if it fails 3 times in ~45s total, it's not transient
- With imagePullPolicy=Required, script exits non-zero on any failure after retries; with BestEffort, script completes regardless

### ECR auth handling
- Pattern match on `*.dkr.ecr.*.amazonaws.com` to detect ECR images — if matched, fetch ECR token; otherwise pull unauthenticated
- Use direct API calls (curl + instance metadata) for ECR token — no AWS CLI dependency
- If ECR auth fails, attempt the pull anyway (image might be public in ECR)
- One ECR token fetched for all ECR images — token won't expire during a single pull session

### Generator API design
- Generator lives in `internal/warmup/` — new package, clean separation from controller
- Uses Go `text/template` for script construction — separates template from logic, bash is readable in template form
- Function signature: `GenerateScript(images []string, policy ImagePullPolicy) string` — decoupled from CRD types
- Tests use string assertions — assert output contains expected commands (ctr pull, labels, etc.)

### Claude's Discretion
- Containerd readiness detection approach (socket check, ctr ping, or both)
- Exact exponential backoff timing within the 30s ceiling
- Template file organization within internal/warmup/
- Logging format and verbosity within the generated script
- Exact curl/metadata approach for ECR token fetching

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

- Support for non-ECR private registries (generic auth mechanism, imagePullSecrets) — future phase or extension

</deferred>

---

*Phase: 18-warmup-script-generator*
*Context gathered: 2026-02-04*
