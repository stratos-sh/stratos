# Phase 4: Controller Split - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Split the monolithic internal/controller/ package into per-CRD packages (nodepool/, nodeclass/) following Karpenter's package-per-controller pattern, with a central setup.go for registration. No reconciliation logic remains in controller/ root except setup.go and shared utilities.

</domain>

<decisions>
## Implementation Decisions

### File-to-package mapping
- nodeclass_lifecycle.go (283 lines) becomes the core of a new nodeclass/ package with its own reconciler — not a helper consumed by NodePool
- pool_maintenance.go and cloud_sync.go stay as separate files within nodepool/ — don't merge into the reconciler
- Tests follow their source files: nodeclass_lifecycle_test.go goes to nodeclass/, pod_assignment_test.go goes to nodepool/, cluster_config_test.go goes with cluster_config.go

### Shared utilities home
- cluster_config.go moves to internal/config/ — it's not controller-specific (main.go also uses it)

### setup.go registration
- Each sub-package has its own Setup() function (per-package setup pattern)
- controller/setup.go remains as an aggregator — calls nodepool.Setup() and nodeclass.Setup() so main.go only calls one function
- Independent reconciler structs: nodepool.Reconciler and nodeclass.Reconciler are fully independent types — no shared base struct

### Reconciler decomposition
- NodeClass-related steps (resolve, validate, conditions) extracted into the nodeclass controller — NodePool reconciler reads the resolved result rather than doing resolution itself
- NodePool and NodeClass reconciler structs are fully independent — no shared base

### Claude's Discretion
- Whether lifecycle/ and nodestate/ stay at controller/ root or move under nodepool/ — evaluate based on actual import graph
- Where node_queries.go and provider_cache.go live — evaluate based on Go import rules and usage patterns
- Whether provider_cache is shared or per-controller — evaluate based on resource usage and coupling
- Dependency injection approach for Setup() functions — explicit parameters vs options struct based on parameter count
- Whether aggregator setup.go or main.go creates shared resources
- Reconciler decomposition style (thin orchestrator + delegates vs single larger file per CRD)
- How NodePool reconciler checks NodeClass readiness (status condition vs direct resolution)

</decisions>

<specifics>
## Specific Ideas

- Karpenter's package-per-controller pattern is the reference architecture
- Per-package Setup() functions match Karpenter's approach where each controller registers itself
- The aggregator setup.go pattern keeps main.go thin while providing a single inventory of all controllers

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 04-controller-split*
*Context gathered: 2026-02-03*
