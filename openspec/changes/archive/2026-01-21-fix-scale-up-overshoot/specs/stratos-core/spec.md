# Spec Delta: stratos-core

**Change**: fix-scale-up-overshoot
**Base**: openspec/specs/stratos-core/spec.md

## MODIFIED Requirements

### Requirement: FR-016: Start Only Needed Nodes (Enhanced)

**Priority**: P1
**Status**: Draft

Stratos MUST NOT start more nodes than needed to satisfy pending pods. The calculation MUST:
1. Sum the resource requests (CPU, memory) of all pending pods
2. Determine node capacity from instance type (using 80% to account for overhead)
3. Calculate minimum nodes needed: `max(ceil(totalCPU/nodeCPU), ceil(totalMem/nodeMem))`
4. Account for nodes already starting (in-flight tracking)

#### Scenario: Minimal scale-up
- **Given** 3 pending pods requiring 2 nodes based on resource calculation
- **When** Stratos scales up
- **Then** only 2 nodes are started

#### Scenario: Resource-based calculation
- **Given** 10 pending pods each requesting 500m CPU and 2Gi memory
- **And** node capacity is 4 vCPU, 16Gi (m5.xlarge at 80% = 3.2 vCPU, 12.8Gi)
- **When** Stratos calculates scale-up need
- **Then** nodesNeeded = max(ceil(5/3.2), ceil(20/12.8)) = max(2, 2) = 2 nodes

#### Scenario: Account for starting nodes
- **Given** 10 pending pods requiring 2 nodes and 1 node already starting
- **When** Stratos evaluates scale-up need
- **Then** only 1 additional node is started (2 - 1 = 1)

#### Scenario: No duplicate scale-up
- **Given** 10 pending pods requiring 2 nodes and 2 nodes already starting
- **When** Stratos evaluates scale-up need
- **Then** no additional nodes are started (2 - 2 = 0)

#### Scenario: Starting node tracking is time-bounded
- **Given** pending pods and nodes marked as starting more than 60 seconds ago
- **When** Stratos evaluates scale-up need
- **Then** stale starting annotations are ignored

---

## ADDED Requirements

### Requirement: FR-049: Track In-Flight Scale-Up Operations

**Priority**: P1
**Status**: Draft

Stratos MUST track nodes that are in the process of starting for scale-up to prevent over-provisioning. This tracking MUST:
- Use a node annotation (`stratos.sh/scale-up-started`) with a timestamp
- Be set when a standby node is started for scale-up
- Be cleared when the node becomes Ready or after a TTL of 60 seconds
- Survive controller restarts (persisted in K8s)

#### Scenario: Set scale-up started annotation
- **Given** a standby node being started for scale-up
- **When** `StartNode()` is called
- **Then** the node has annotation `stratos.sh/scale-up-started` with current timestamp

#### Scenario: Clear annotation on node ready
- **Given** a node with `stratos.sh/scale-up-started` annotation that becomes Ready
- **When** the reconciliation loop runs
- **Then** the annotation is removed

#### Scenario: Clear annotation after TTL
- **Given** a node with `stratos.sh/scale-up-started` annotation older than 60 seconds
- **When** the reconciliation loop runs
- **Then** the annotation is removed (even if node is not Ready)

---

### Requirement: FR-050: Resource-Based Scale-Up Calculation

**Priority**: P1
**Status**: Draft

Stratos MUST calculate scale-up need based on pod resource requests and node capacity.

#### Scenario: Sum pod resource requests
- **Given** pending pods with CPU and memory requests
- **When** calculating scale-up need
- **Then** total CPU and memory requests are summed across all pending pods

#### Scenario: Use instance type capacity
- **Given** a NodePool with AWS instanceType configured
- **When** calculating node capacity
- **Then** capacity is derived from a static mapping of instance type to resources

#### Scenario: Apply 80% capacity factor
- **Given** node capacity of 4 vCPU, 16Gi
- **When** calculating usable capacity
- **Then** usable capacity is 3.2 vCPU, 12.8Gi (80% of total)

---

### Requirement: FR-051: Default Pod Resource Configuration

**Priority**: P1
**Status**: Draft

NodePool MUST support configurable default resource requests for pods that don't specify their own requests. This is used in scale-up calculations.

```yaml
spec:
  scaleUp:
    defaultPodResources:
      requests:
        cpu: "500m"
        memory: "1Gi"
```

#### Scenario: Pod with resource requests
- **Given** a pending pod with explicit CPU and memory requests
- **When** calculating scale-up need
- **Then** the pod's explicit requests are used

#### Scenario: Pod without resource requests
- **Given** a pending pod without resource requests
- **And** NodePool has `scaleUp.defaultPodResources` configured
- **When** calculating scale-up need
- **Then** the default resources from NodePool config are used

#### Scenario: Pod without requests, no defaults configured
- **Given** a pending pod without resource requests
- **And** NodePool does NOT have `scaleUp.defaultPodResources` configured
- **When** calculating scale-up need
- **Then** the pod is treated as requiring unknown resources (falls back to 1:1 mapping)

---

### Requirement: FR-052: AWS Instance Capacity Lookup (Hybrid)

**Priority**: P1
**Status**: Draft

Stratos MUST support looking up AWS instance capacity for scale-up calculations using a hybrid approach:
1. **Primary**: Read from existing node's `.status.allocatable` (real data)
2. **Fallback**: Static mapping for known AWS instance types
3. **Default**: Fall back to 1:1 pod-to-node if unknown

#### Scenario: Capacity from existing node
- **Given** a NodePool with existing standby nodes
- **When** looking up capacity
- **Then** uses `.status.allocatable` from an existing node

#### Scenario: Capacity from static mapping (empty pool)
- **Given** a NodePool with no existing nodes and instanceType "m5.xlarge"
- **When** looking up capacity
- **Then** returns 4 vCPU, 16Gi memory from static AWS mapping

#### Scenario: Unknown instance type
- **Given** a NodePool with an unknown instanceType and no existing nodes
- **When** looking up capacity
- **Then** returns zero values and scale-up falls back to 1:1 pod-to-node mapping

---

## MODIFIED Success Criteria

| ID | Criteria | Target | Notes |
|----|----------|--------|-------|
| SC-004 | Pending pods trigger scale-up | Scheduled within 30 seconds | No change |
| SC-013 (NEW) | Single pending pod starts exactly 1 node | 100% | New |
| SC-014 (NEW) | N pods requiring M nodes starts exactly M nodes | 100% | New |
| SC-015 (NEW) | Resource calculation matches expected | Within 1 node of optimal | New |
