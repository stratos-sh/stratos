# Phase 3: Strategy Package Extraction - Context

**Gathered:** 2026-02-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Move `internal/controller/strategy/` to `internal/strategy/` as a first-class top-level package. Decompose the 910-line `kubernetes.go` into a `kubernetes/` sub-package with focused files (no file over 300 lines). Ensure `strategy/` imports `lifecycle/` and `cloudprovider/` but never imports `controller/`. No new capabilities — this is pure code organization.

</domain>

<decisions>
## Implementation Decisions

### Package naming
- Keep the name `strategy/` — fits the Strategy Pattern (pluggable scaling implementations). Consumers already think in terms of ScalingStrategy interface
- Final location: `internal/strategy/`

### Package structure
- Each strategy implementation becomes its own sub-package: `strategy/kubernetes/`, `strategy/githubactions/`
- Interface and shared types stay at the `strategy/` root level
- Factory refactored to import sub-packages and dispatch based on config

### kubernetes.go file decomposition
- `maintenance.go` — `RunMaintenance()` and all sub-tasks it coordinates (startup taints, template labels, annotation cleanup, pod assignment cleanup) get their own dedicated file
- Scaling logic (CheckDemand, FindScaleDownCandidates), pod assignment management, node readiness/taint management, and helpers — Claude decides file boundaries based on line counts and logical grouping

### Drain logic placement
- Drain stays in `strategy/kubernetes/drain.go` — it's a scaling strategy concern, not a lifecycle operation
- `DrainAndStop()` stays as a thin wrapper in the strategy: calls drainHelper for drain, then stops the cloud instance. Drain knows about draining, strategy knows about stopping
- Cordon/uncordon duplication between drain.go and kubernetes.go — Claude decides how to resolve (consolidate vs keep separate with clear naming)

### Network readiness ownership
- Network readiness stays within `strategy/kubernetes/` — it's strategy-only, not needed by lifecycle
- `networkReadinessChecker` should be instantiated once and reused (per KubernetesStrategy construction or per-reconcile) rather than created ephemerally each time
- Whether to consolidate the checker and taint removal logic into one file or keep them separate — Claude decides based on how other decomposition choices land

### Interface boundary
- Whether ScalingStrategy stays monolithic (8 methods) or splits into role interfaces — Claude decides based on reconciler consumption patterns and testability
- Interface and ScalingDemand type location — Claude decides based on Go conventions

### Claude's Discretion
- Scale-up and scale-down: same file (scaling.go) or separate files (scale_up.go / scale_down.go)
- Pod assignment management: inline with scaling or own file (pod_assignments.go)
- Node readiness + taint management: single file or split (readiness.go + taints.go)
- Helper methods placement (getNodesForPool, getRunningNodes, etc.)
- Network readiness file consolidation strategy
- Cordon deduplication approach
- ScalingStrategy interface: monolithic vs composed role interfaces
- interface.go and ScalingDemand placement within the package

</decisions>

<specifics>
## Specific Ideas

- kubernetes/ sub-package should have a parallel structure to githubactions/ — consistent pattern across all strategy implementations
- The 300-line-per-file cap from success criteria is the hard constraint for decomposition decisions
- Existing satellite files (kubernetes_drain.go, kubernetes_events.go, kubernetes_network.go, kubernetes_capacity.go) are already well-isolated and should move cleanly into the kubernetes/ sub-package

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 03-strategy-package-extraction*
*Context gathered: 2026-02-02*
