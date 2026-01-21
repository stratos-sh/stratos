# Proposal: Fix Scale-Up Overshoot

**Change ID**: fix-scale-up-overshoot
**Created**: 2026-01-21
**Updated**: 2026-01-21
**Status**: Draft
**Author**: AI Assistant

## Why

The scale-up logic has two critical bugs causing over-provisioning:
1. **Duplicate scale-up**: Rapid reconciliation loops start multiple nodes for a single pending pod
2. **1:1 pod-to-node assumption**: Starts N nodes for N pods even when pods fit on fewer nodes

## What Changes

1. **Resource-based calculation**: Calculate nodes needed based on pod resource requests and node capacity
2. **In-flight tracking**: Track nodes being started via annotations to prevent duplicate scale-ups
3. **Default resources config**: Configure default resource requests for pods without explicit requests

## Summary

Fix a bug where the scale-up logic starts more nodes than needed. The fix introduces:
1. **Resource-based calculation**: Calculate nodes needed based on pod resource requests and node capacity
2. **In-flight tracking**: Track nodes being started to prevent duplicate scale-ups during rapid reconciliation
3. **Default resources config**: Configure default resource requests for pods without explicit requests

## Problem Statement

**Bug 1: Duplicate scale-up (critical)**
When a single pod becomes unschedulable:
1. Reconciliation detects the pending pod and starts 1 standby node
2. The reconciler requeues after 5 seconds
3. The pod is still pending (instance takes ~10-20s to become ready)
4. Reconciliation sees the same pending pod and starts another node
5. This repeats, potentially starting ALL standby nodes for 1 pod

**Bug 2: 1:1 pod-to-node assumption (inefficient)**
The current logic assumes 1 pod = 1 node. If 10 pods can fit on 2 nodes, the current logic would still try to start 10 nodes.

## Design Decisions

Based on discussion:

1. **Node capacity**: Derive from instance type using static mapping (e.g., m5.xlarge → 4 vCPU, 16Gi)
2. **Pods without requests**: Configurable default in NodePool spec (`scaleUp.defaultPodResources`)
3. **Capacity buffer**: Use 80% of node capacity to account for DaemonSets/system overhead

## Proposed Solution

### Resource-Based Calculation

```
nodesNeeded = max(
    ceil(totalPodCPU / (nodeCapacityCPU * 0.80)),
    ceil(totalPodMemory / (nodeCapacityMemory * 0.80))
)
```

**Example:**
- 10 pods requesting 500m CPU, 2Gi memory each = 5 vCPU, 20Gi total
- Node (m5.xlarge): 4 vCPU, 16Gi → usable (80%): 3.2 vCPU, 12.8Gi
- Nodes needed: max(ceil(5/3.2), ceil(20/12.8)) = max(2, 2) = **2 nodes**

### In-Flight Tracking

Add annotation `stratos.sh/scale-up-started` when starting nodes. Subtract starting nodes from calculation:

```
actualNeed = nodesNeeded - startingNodes
```

Annotation is cleared when node becomes Ready or after 60s TTL.

### New NodePool Configuration

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: runners
spec:
  poolSize: 10
  minStandby: 3
  scaleUp:
    defaultPodResources:
      requests:
        cpu: "500m"
        memory: "1Gi"
  template:
    cloudProvider:
      provider: aws
      aws:
        instanceType: m5.xlarge
```

## Performance Requirements

- **MUST** maintain current performance: pods scheduled within ~20 seconds
- **MUST NOT** add blocking API calls to the reconciliation loop
- **MUST** be safe under high concurrency

## Scope

### In Scope
- Resource-based scale-up calculation
- In-flight tracking via annotations
- NodePool `scaleUp.defaultPodResources` configuration
- AWS instance type → capacity mapping
- Unit and integration tests

### Out of Scope
- Changes to scale-down logic
- Bin-packing optimization (we use simple sum, not optimal packing)
- GPU resource tracking
- Dynamic instance type discovery (using static mapping)

## Success Criteria

| Criteria | Target |
|----------|--------|
| Single pending pod starts exactly 1 node | 100% |
| N pods requiring M nodes starts M nodes | Within 1 of optimal |
| Pod scheduling latency | < 30 seconds |
| No performance regression | Maintained |

## Risks

| Risk | Mitigation |
|------|------------|
| Unknown instance type | Fall back to 1:1 pod-to-node |
| Stale tracking blocks scale-up | 60s TTL auto-clears |
| 80% factor too aggressive | Can adjust, configurable in future |

## Files Changed

| File | Change |
|------|--------|
| `api/v1alpha1/nodepool_types.go` | Add `ScaleUpConfig` |
| `internal/cloudprovider/aws/instance_types.go` | New: instance type mapping |
| `internal/controller/scale_calculator.go` | New: resource calculator |
| `internal/controller/nodepool_controller.go` | Update `calculateScaleUpNeeded()` |
| `internal/nodemanager/manager.go` | Set annotation in `StartNode()` |
| `internal/nodemanager/labels.go` | Add annotation constant |

## Related Requirements

- FR-015: Start Standby Nodes
- FR-016: Start Only Needed Nodes (enhanced)
- FR-017: Respect Pool Size Limit
- SC-004: Pending pods scheduled within 30 seconds
