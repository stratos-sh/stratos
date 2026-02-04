# Phase 2: Lifecycle Package Extraction - Context

**Gathered:** 2026-02-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Split node lifecycle operations (launch, start, stop, warmup monitoring) into focused files within the lifecycle/ leaf package. Two of four success criteria are already met: lifecycle/ has zero imports from controller/, and nodestate/ is a pure leaf. The remaining work is splitting warmup.go (455 lines) and operations.go (355 lines) into focused files under 200 lines each with single responsibilities.

</domain>

<decisions>
## Implementation Decisions

### Warmup file splitting
- Both warmup monitoring flows (K8s node monitoring via MonitorWarmup and cloud instance monitoring via MonitorCloudWarmup) stay in the same file — they are both "monitoring warmup progress" at different levels
- Combined warmup monitoring file will be ~178 lines (within the 200-line target)

### Operations file splitting
- No specific grouping locked — Claude has discretion on how to group the 9 operations methods across files
- The key constraint: each resulting file should handle one lifecycle concern and stay under 200 lines

### File naming & structure
- Flat structure — all .go files in lifecycle/ directly, no sub-packages
- manager.go (108 lines) stays as-is — types, interfaces (NodeLauncher, NodeHooks), and constructor in one file
- No doc.go in this phase — deferred to Phase 6 (Documentation)

### Claude's Discretion
- Timeout handlers (handleWarmupTimeout, handleCloudWarmupTimeout): inline with monitors or separate file — Claude picks based on call graph
- Adoption flow (adoptAndTransitionToStandby): with monitoring or separate — Claude picks based on file size and cohesion
- Warmup completion (handleControllerStopWarmup): with monitors or separate — Claude picks based on file size constraints
- Operations granularity: group-by-concern (3-4 files, ~80-120 lines each) vs one-file-per-operation (many small files). Claude picks meaningful groupings
- StartNode/StopNode: together or separate — Claude picks based on how the code reads
- SyncNodeState: own file or with transitions — Claude picks based on dependency flow
- Small helpers (FindNodeByInstanceID, setLastStartedAnnotation, deleteNode, LabelNode): helpers file or co-located with callers — Claude picks best fit
- File naming convention: prefix pattern (warmup_monitor.go) vs subject_role (node_launch.go) — Claude picks what fits lifecycle package context

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. The guiding principle is that each file should have a single responsibility and stay under 200 lines.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 02-lifecycle-package-extraction*
*Context gathered: 2026-02-02*
