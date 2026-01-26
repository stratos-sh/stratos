# Specification: Stratos Core

**Capability**: stratos-core
**Version**: 1.0.0
**Status**: Implemented
**Created**: 2026-01-21
**Migrated From**: specs/001-instance-pool-manager/spec.md

## Purpose

Stratos is a Kubernetes node scaler that eliminates node provisioning delays. Unlike Karpenter which provisions nodes on-demand (taking 3-5 minutes), Stratos maintains a pool of pre-warmed, stopped instances that can join the cluster in seconds.

**Key Concepts**

- **NodePool**: CRD defining a pool of pre-warmed nodes
- **poolSize**: Maximum total nodes (standby + running)
- **minStandby**: Minimum nodes to keep in stopped/standby state
- **NodeState**: warmup | standby | running | terminating
- **Pre-warming**: Instance initialization followed by self-stop

**Use Cases**

- **CI/CD**: Runners ready instantly when jobs are queued
- **ML/LLM Inference**: GPU nodes with models pre-loaded
- **Bursty workloads**: Handle sudden traffic without cold-start delays

---
## Requirements
### Requirement: FR-001 NodePool Custom Resource Definition

Stratos MUST provide a NodePool Custom Resource Definition (API version: v1alpha1, graduating to v1beta1/v1 based on stability) for configuring node pools.

**Priority**: P1
**Status**: Implemented

#### Scenario: Create NodePool
- **Given** a Kubernetes cluster with Stratos installed
- **When** a user applies a NodePool manifest
- **Then** Stratos accepts and reconciles the resource

### Requirement: FR-002 Configurable poolSize

NodePool MUST support configurable `poolSize` (maximum total nodes: standby + running).

**Priority**: P1
**Status**: Implemented

#### Scenario: Pool size limit
- **Given** a NodePool with poolSize=10
- **When** the pool has 10 total nodes
- **Then** Stratos does not provision additional nodes

### Requirement: FR-003 Configurable minStandby

NodePool MUST support configurable `minStandby` (minimum nodes to keep in stopped/standby state).

**Priority**: P1
**Status**: Implemented

#### Scenario: Maintain standby
- **Given** a NodePool with minStandby=5 and current standby count is 3
- **When** reconciliation runs
- **Then** Stratos provisions 2 new nodes to reach minStandby

### Requirement: FR-004 minStandby Validation

Stratos MUST validate that minStandby does not exceed poolSize.

**Priority**: P1
**Status**: Implemented

#### Scenario: Invalid configuration rejected
- **Given** a NodePool where minStandby exceeds poolSize
- **When** the NodePool is applied
- **Then** it is rejected with a validation error

### Requirement: FR-005 Multiple NodePools

Stratos MUST support multiple NodePool resources in a cluster.

**Priority**: P1
**Status**: Implemented

#### Scenario: Independent pools
- **Given** two NodePool resources with different configurations
- **When** both are reconciled
- **Then** each pool maintains its own nodes independently

---

### Requirement: FR-006 Cloud Provider Instance Launch

Stratos MUST launch instances through the configured cloud provider.

**Priority**: P1
**Status**: Implemented

#### Scenario: AWS EC2 launch
- **Given** a NodePool with AWS configuration
- **When** Stratos needs to provision a node
- **Then** it creates an EC2 instance with specified parameters

### Requirement: FR-007 Userdata Configuration for Cluster Join

Stratos MUST configure instances with userdata that joins the K8s cluster. In `SelfStop` mode (default), userdata MUST also include logic to stop the instance. In `ControllerStop` mode, no shutdown logic is required.

**Priority**: P1
**Status**: Implemented

#### Scenario: Node initialization (SelfStop mode)
- **Given** a launched instance with `completionMode: SelfStop`
- **When** userdata executes
- **Then** the node joins the cluster and the instance self-stops

#### Scenario: Node initialization (ControllerStop mode)
- **Given** a launched instance with `completionMode: ControllerStop`
- **When** userdata executes
- **Then** the node joins the cluster (no self-stop required)

---

### Requirement: FR-008 Warmup Instance Monitoring

Stratos MUST monitor launched instances in warmup state:
- In `SelfStop` mode: Wait for instance to self-stop (detect stopped state)
- In `ControllerStop` mode: Wait for node Ready, then call StopInstance

**Priority**: P1
**Status**: Implemented

#### Scenario: Detect self-stop (SelfStop mode)
- **Given** a launched instance in warmup state with `completionMode: SelfStop`
- **When** the instance stops
- **Then** Stratos marks the node as standby

#### Scenario: Stop on ready (ControllerStop mode)
- **Given** a launched instance in warmup state with `completionMode: ControllerStop`
- **When** the node becomes Ready
- **Then** Stratos stops the instance and marks it as standby

### Requirement: FR-009 Configurable Warmup Timeout

Stratos MUST support a configurable timeout for instances to self-stop.

**Priority**: P1
**Status**: Implemented

#### Scenario: Timeout configuration
- **Given** a NodePool with preWarm.timeout=15m
- **When** an instance runs longer than 15 minutes during warmup
- **Then** Stratos applies the timeout action

### Requirement: FR-010 Configurable Timeout Action

Stratos MUST support a configurable timeout action: "stop" or "terminate".

**Priority**: P1
**Status**: Implemented

#### Scenario: Timeout action stop
- **Given** preWarm.timeoutAction=stop
- **When** an instance exceeds the timeout
- **Then** Stratos stops the instance

#### Scenario: Timeout action terminate
- **Given** preWarm.timeoutAction=terminate
- **When** an instance exceeds the timeout
- **Then** Stratos terminates the instance and provisions a replacement

### Requirement: FR-011 Timeout Action Application

Stratos MUST apply the configured timeout action when instances fail to self-stop.

**Priority**: P1
**Status**: Implemented

#### Scenario: Stuck instance handling
- **Given** an instance that has not self-stopped within the timeout
- **When** the timeout expires
- **Then** Stratos applies the configured action (stop or terminate)

### Requirement: FR-012 Node Labels for Pool Ownership

Stratos MUST label pre-warmed Nodes with NodePool ownership and standby status.

**Priority**: P1
**Status**: Implemented

#### Scenario: Node labels applied
- **Given** a pre-warmed standby node
- **When** viewed in kubectl
- **Then** it shows labels: stratos.sh/pool, stratos.sh/state, stratos.sh/instance-id

---

### Requirement: FR-013 Pod Event Watching

Stratos MUST watch for Kubernetes pod events and react immediately to unschedulable pods.

**Priority**: P1
**Status**: Implemented

#### Scenario: Pending pod detected
- **Given** a pod with PodScheduled=False due to insufficient resources
- **When** Stratos observes the event
- **Then** it evaluates the pod for scale-up

### Requirement: FR-014 Pod Matching Against NodePool

Stratos MUST evaluate pending pods against NodePool requirements (node selectors, taints/tolerations).

**Priority**: P1
**Status**: Implemented

#### Scenario: Pod matches NodePool
- **Given** a pending pod with nodeSelector matching a NodePool
- **When** Stratos evaluates it
- **Then** the pod is considered for that NodePool

#### Scenario: Pod does not match
- **Given** a pending pod with requirements not matching any NodePool
- **When** Stratos evaluates it
- **Then** Stratos ignores the pod

### Requirement: FR-015 Scale-Up Trigger

Stratos MUST start standby nodes when pending pods can be satisfied.

**Priority**: P1
**Status**: Implemented

#### Scenario: Scale-up triggered
- **Given** pending pods that can be satisfied by a standby node
- **When** Stratos decides to scale up
- **Then** it starts the standby node

### Requirement: FR-016 Minimal Node Start Calculation

Stratos MUST NOT start more nodes than needed to satisfy pending pods. The calculation MUST:
1. Sum the resource requests (CPU, memory) of all pending pods
2. Determine node capacity from instance type (using 80% to account for overhead)
3. Calculate minimum nodes needed: `max(ceil(totalCPU/nodeCPU), ceil(totalMem/nodeMem))`
4. Account for nodes already starting (in-flight tracking)

**Priority**: P1
**Status**: Implemented

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

### Requirement: FR-017 Pool Size Enforcement

Stratos MUST NOT exceed poolSize total nodes (standby + running).

**Priority**: P1
**Status**: Implemented

#### Scenario: Pool size enforced
- **Given** poolSize=10 with 10 nodes already (7 running + 3 standby)
- **When** more pods become pending
- **Then** no additional nodes are started

### Requirement: FR-018 Fast Node Startup

Started nodes MUST become Ready within seconds (not minutes).

**Priority**: P1
**Status**: Implemented

#### Scenario: Quick startup
- **Given** a standby node is started
- **When** it transitions to running
- **Then** it becomes Ready within 30 seconds

### Requirement: FR-049 Scale-Up Started Tracking

Stratos MUST track nodes that are in the process of starting for scale-up to prevent over-provisioning. This tracking MUST:
- Use a node annotation (`stratos.sh/scale-up-started`) with a timestamp
- Be set when a standby node is started for scale-up
- Be cleared when the node becomes Ready or after a TTL of 60 seconds
- Survive controller restarts (persisted in K8s)

**Priority**: P1
**Status**: Implemented

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

### Requirement: FR-050 Resource-Based Scale-Up Calculation

Stratos MUST calculate scale-up need based on pod resource requests and node capacity.

**Priority**: P1
**Status**: Implemented

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

### Requirement: FR-051 Default Pod Resource Requests

NodePool MUST support configurable default resource requests for pods that don't specify their own requests. This is used in scale-up calculations.

**Priority**: P1
**Status**: Implemented

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

### Requirement: FR-052 AWS Instance Capacity Lookup

Stratos MUST support looking up AWS instance capacity for scale-up calculations using a hybrid approach:
1. **Primary**: Read from existing node's `.status.allocatable` (real data)
2. **Fallback**: Static mapping for known AWS instance types
3. **Default**: Fall back to 1:1 pod-to-node if unknown

**Priority**: P1
**Status**: Implemented

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

### Requirement: FR-019 Empty Node Detection

Stratos MUST detect empty nodes (no pods excluding DaemonSets).

**Priority**: P1
**Status**: Implemented

#### Scenario: Empty node identified
- **Given** a running node with only DaemonSet pods
- **When** Stratos evaluates the node
- **Then** it is considered empty

### Requirement: FR-020 Configurable emptyNodeTTL

Stratos MUST support configurable emptyNodeTTL (duration before stopping empty nodes).

**Priority**: P1
**Status**: Implemented

#### Scenario: TTL elapsed
- **Given** an empty node and emptyNodeTTL=5m
- **When** the node has been empty for 5 minutes
- **Then** Stratos initiates scale-down

### Requirement: FR-021 Cordon Before Drain

Stratos MUST cordon nodes before draining.

**Priority**: P1
**Status**: Implemented

#### Scenario: Node cordoned
- **Given** a node selected for scale-down
- **When** Stratos begins the process
- **Then** the node is cordoned first

### Requirement: FR-022 PDB-Respecting Drain

Stratos MUST drain nodes respecting PodDisruptionBudgets.

**Priority**: P1
**Status**: Implemented

#### Scenario: PDB respected
- **Given** a node with pods protected by PDBs
- **When** Stratos drains the node
- **Then** it waits for PDB conditions to allow eviction

### Requirement: FR-023 Stop on Scale-Down

Stratos MUST stop (not terminate) nodes on scale-down to preserve pre-warming.

**Priority**: P1
**Status**: Implemented

#### Scenario: Node stopped
- **Given** a drained node
- **When** scale-down completes
- **Then** the instance is stopped, not terminated

### Requirement: FR-024 Standby Pool Return

Stopped nodes MUST return to standby pool and be available for future scale-up.

**Priority**: P1
**Status**: Implemented

#### Scenario: Node returns to standby
- **Given** a node that was stopped during scale-down
- **When** it stops successfully
- **Then** it is marked as standby and can be started again

### Requirement: FR-025 Disable Scale-Down Option

Stratos MUST support disabling automatic scale-down per NodePool.

**Priority**: P1
**Status**: Implemented

#### Scenario: Scale-down disabled
- **Given** a NodePool with scaleDown.enabled=false
- **When** nodes become empty
- **Then** Stratos does not stop them

---

### Requirement: FR-026 Periodic Reconciliation Loop

Stratos MUST run a periodic reconciliation loop to maintain pool health (configurable interval, default 30 seconds).

**Priority**: P1
**Status**: Implemented

#### Scenario: Regular maintenance
- **Given** a running Stratos controller
- **When** the sync period elapses
- **Then** all NodePools are reconciled

### Requirement: FR-027 Standby Replenishment

Stratos MUST provision new nodes when standby count is below minStandby.

**Priority**: P1
**Status**: Implemented

#### Scenario: Standby replenishment
- **Given** minStandby=5 and current standby=3
- **When** reconciliation runs
- **Then** 2 new nodes are provisioned

### Requirement: FR-028 External Termination Handling

Stratos MUST detect and handle externally terminated instances.

**Priority**: P1
**Status**: Implemented

#### Scenario: External termination detected
- **Given** a standby node whose instance was terminated externally
- **When** Stratos detects this
- **Then** it removes the stale Node object

### Requirement: FR-029 State Recovery on Restart

Stratos MUST recover state on controller restart.

**Priority**: P1
**Status**: Implemented

#### Scenario: Controller restart
- **Given** Stratos restarts
- **When** it initializes
- **Then** it reconciles all NodePools and recovers any inconsistent state

---

### Requirement: FR-030 AWS EC2 Support

Stratos MUST support AWS (EC2) as a cloud provider.

**Priority**: P1
**Status**: Implemented

#### Scenario: AWS instances
- **Given** a NodePool with AWS configuration
- **When** nodes are provisioned
- **Then** EC2 instances are created with specified parameters

### Requirement: FR-031 Pluggable Cloud Providers

Stratos MUST support pluggable cloud provider implementations for future providers.

**Priority**: P1
**Status**: Implemented

#### Scenario: Provider interface
- **Given** the CloudProvider interface
- **When** implementing a new provider
- **Then** only launch, start, stop, get state, and terminate need to be implemented

### Requirement: FR-032 Cloud Provider Operations

Cloud provider MUST support: launch, start, stop, get state, and terminate operations.

**Priority**: P1
**Status**: Implemented

#### Scenario: Operation completeness
- **Given** the CloudProvider interface
- **Then** all five operations are defined and callable

### Requirement: FR-033 Instance Tagging

Stratos MUST tag all managed instances with NodePool name and cluster identifier.

**Priority**: P1
**Status**: Implemented

#### Scenario: Tags applied
- **Given** a launched instance
- **When** created by Stratos
- **Then** it has tags: managed-by=stratos, stratos.sh/pool, stratos.sh/cluster

### Requirement: FR-034 Client-Side Rate Limiting

Stratos MUST implement client-side rate limiting for cloud API calls to avoid hitting provider limits.

**Priority**: P1
**Status**: Implemented

#### Scenario: Rate limiting
- **Given** many concurrent cloud operations
- **When** the rate limit is reached
- **Then** requests are queued rather than rejected

### Requirement: FR-035 Exponential Backoff on Retry

Stratos MUST use exponential backoff when retrying failed cloud API calls (including rate limit/throttle errors).

**Priority**: P1
**Status**: Implemented

#### Scenario: Retry with backoff
- **Given** a cloud API call fails with a transient error
- **When** Stratos retries
- **Then** it uses exponential backoff

---

### Requirement: FR-036 Configurable maxNodeRuntime

NodePool MUST support configurable maxNodeRuntime (optional, 0 = disabled).

**Priority**: P2
**Status**: Implemented

#### Scenario: Max runtime set
- **Given** a NodePool with maxNodeRuntime=24h
- **When** a node runs longer than 24 hours
- **Then** it is marked for recycling

### Requirement: FR-037 Automatic Node Recycling

Stratos MUST automatically drain and stop nodes that exceed maxNodeRuntime.

**Priority**: P2
**Status**: Implemented

#### Scenario: Node recycled
- **Given** a node running longer than maxNodeRuntime
- **When** Stratos detects this
- **Then** it drains and stops the node

### Requirement: FR-038 Max Runtime Warning Events

Stratos MUST emit warning events when nodes approach maxNodeRuntime threshold.

**Priority**: P2
**Status**: Implemented

#### Scenario: Warning emitted
- **Given** a node approaching maxNodeRuntime
- **When** it crosses the warning threshold
- **Then** a Kubernetes event is emitted

---

### Requirement: FR-039 Prometheus Metrics for Pool State

Stratos MUST expose Prometheus metrics for: pool size, standby count, running count, warmup count.

**Priority**: P1
**Status**: Implemented

#### Scenario: Metrics available
- **Given** a running Stratos controller
- **When** /metrics is scraped
- **Then** stratos_nodepool_nodes_total is available with state labels

### Requirement: FR-040 Scale-Up Metrics

Stratos MUST expose metrics for scale-up operations: count, latency (time from pending to scheduled).

**Priority**: P1
**Status**: Implemented

#### Scenario: Scale-up latency tracked
- **Given** a scale-up operation
- **When** it completes
- **Then** stratos_scaleup_duration_seconds is recorded

### Requirement: FR-041 Scale-Down Metrics

Stratos MUST expose metrics for scale-down operations: count, drain duration.

**Priority**: P1
**Status**: Implemented

#### Scenario: Scale-down metrics
- **Given** a scale-down operation
- **When** it completes
- **Then** stratos_scaledown_total and stratos_drain_duration_seconds are recorded

### Requirement: FR-042 Kubernetes Events

Stratos MUST emit Kubernetes events for significant operations (node started, node stopped, errors).

**Priority**: P1
**Status**: Implemented

#### Scenario: Events emitted
- **Given** a node is started
- **When** the operation completes
- **Then** a NodeStarted event is visible on the NodePool

---

### Requirement: FR-043 Least-Privilege RBAC

Stratos MUST operate with cluster-scoped least-privilege RBAC permissions.

**Priority**: P1
**Status**: Implemented

#### Scenario: Minimal permissions
- **Given** the Stratos ClusterRole
- **Then** it has only the permissions required for operation

### Requirement: FR-044 Node Object Access

Stratos MUST have full access to Node objects (get, list, watch, create, update, patch, delete).

**Priority**: P1
**Status**: Implemented

#### Scenario: Node operations
- **Given** Stratos needs to manage nodes
- **Then** the ClusterRole grants full Node access

### Requirement: FR-045 Pod Watch Access

Stratos MUST have watch access to Pod objects cluster-wide (get, list, watch).

**Priority**: P1
**Status**: Implemented

#### Scenario: Pod monitoring
- **Given** Stratos needs to detect pending pods
- **Then** the ClusterRole grants Pod read access

### Requirement: FR-046 NodePool CRD Access

Stratos MUST have full access to NodePool CRD objects (get, list, watch, create, update, patch, delete).

**Priority**: P1
**Status**: Implemented

#### Scenario: CRD management
- **Given** Stratos manages NodePool resources
- **Then** the ClusterRole grants full NodePool access

### Requirement: FR-047 Event Creation Access

Stratos MUST have create access to Event objects for operational visibility.

**Priority**: P1
**Status**: Implemented

#### Scenario: Event creation
- **Given** Stratos needs to emit events
- **Then** the ClusterRole grants Event create access

### Requirement: FR-048 No Cluster-Admin Required

Stratos MUST NOT require cluster-admin or access to secrets outside its own namespace.

**Priority**: P1
**Status**: Implemented

#### Scenario: Limited scope
- **Given** the Stratos ClusterRole
- **Then** it does not grant cluster-admin or broad secret access

---

### Requirement: FR-053 Configurable Warmup Completion Mode

NodePool MUST support configurable `preWarm.completionMode` to control how warmup completes:
- `SelfStop` (default): Instance self-stops via userdata script (current behavior)
- `ControllerStop`: Stratos stops instance when node is Ready

**Priority**: P1
**Status**: Implemented

```yaml
spec:
  preWarm:
    completionMode: ControllerStop  # or SelfStop (default)
```

#### Scenario: Default to SelfStop for backward compatibility
- **Given** a NodePool without `preWarm.completionMode` specified
- **When** an instance completes warmup
- **Then** Stratos waits for the instance to self-stop (existing behavior)

#### Scenario: ControllerStop mode configured
- **Given** a NodePool with `preWarm.completionMode: ControllerStop`
- **When** an instance is launched for warmup
- **Then** Stratos monitors the node and stops it when ready

---

### Requirement: FR-054 Controller Stop on Node Ready

In ControllerStop mode, Stratos MUST call `StopInstance` when:
1. The Kubernetes node exists (instance joined cluster)
2. The node has Ready condition = True
3. Network readiness conditions are met (if `startupTaintRemoval: WhenNetworkReady`)

**Priority**: P1
**Status**: Implemented

#### Scenario: Stop on node Ready
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** an instance in warmup state
- **When** the node becomes Ready
- **Then** Stratos calls StopInstance and transitions to standby

#### Scenario: Wait for network ready when configured
- **Given** a NodePool with `completionMode: ControllerStop` and `startupTaintRemoval: WhenNetworkReady`
- **And** a node that is Ready but NetworkingReady is False
- **When** Stratos evaluates the node
- **Then** it waits until NetworkingReady becomes True before stopping

#### Scenario: Node not yet Ready
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** an instance in warmup state
- **When** the node exists but Ready condition is not True
- **Then** Stratos continues monitoring (does not stop)

---

### Requirement: FR-055 Timeout Handling in ControllerStop Mode

In ControllerStop mode, if the node does not become Ready within `preWarm.timeout`, Stratos MUST apply the configured `timeoutAction` (stop or terminate).

**Priority**: P1
**Status**: Implemented

#### Scenario: Timeout in ControllerStop mode
- **Given** a NodePool with `completionMode: ControllerStop` and `timeout: 10m`
- **And** an instance that has been warming up for more than 10 minutes
- **When** the node is still not Ready
- **Then** Stratos applies the timeout action (stop or terminate)

---

### Requirement: FR-056 ControllerStop Without Userdata Shutdown Scripts

In ControllerStop mode, instances MUST be able to complete warmup without any shutdown commands in userdata. The userdata only needs to configure the node to join the cluster.

**Priority**: P1
**Status**: Implemented

#### Scenario: Bottlerocket with config-only userdata
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** Bottlerocket AMI with TOML-only userdata (no bootstrap containers)
- **When** the instance boots and joins the cluster
- **Then** Stratos detects the Ready node and stops it

#### Scenario: AL2023 without shutdown script
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** AL2023 AMI with nodeadm config but no poweroff script
- **When** the instance boots and joins the cluster
- **Then** Stratos detects the Ready node and stops it

---

## Success Criteria

| ID | Criteria | Target |
|----|----------|--------|
| SC-001 | NodePool reaches minStandby count | Within 2 reconciliation cycles (~1 minute) |
| SC-002 | Pool never exceeds poolSize | Always enforced |
| SC-003 | Standby replenishment | Within 2 reconciliation cycles |
| SC-004 | Pending pods trigger scale-up | Scheduled within 30 seconds |
| SC-005 | Pre-warmed nodes become Ready | Within 10 seconds of start |
| SC-006 | Empty nodes return to standby | Within configured TTL |
| SC-007 | Controller restart recovery | No node state lost |
| SC-008 | Prometheus metrics exposed | All metrics accurate |
| SC-009 | Kubernetes events emitted | All significant operations |
| SC-010 | Failed instances replaced | Within 2 reconciliation cycles |
| SC-011 | Max runtime recycling | Automatic when exceeded |
| SC-012 | PDB respect | Always during drain |
| SC-013 | Single pending pod starts exactly 1 node | 100% |
| SC-014 | N pods requiring M nodes starts exactly M nodes | 100% |
| SC-015 | Resource calculation matches expected | Within 1 node of optimal |

---

## Assumptions

- Kubernetes cluster version 1.27+ is running
- Stratos is deployed with appropriate RBAC permissions
- Cloud provider credentials are configured
- Cloud provider supports stop/start operations
- Base AMI/image has Kubernetes components pre-installed
- Userdata can join nodes to the cluster
- Userdata script self-stops the instance after initialization
- Nodes can be started from stopped state and rejoin without re-initialization

---

## Out of Scope (v1)

**Image Preloading**

Stratos v1 does not manage container image pre-pulling during node pre-warming. The userdata script is responsible for any image pulling.

**Future**: See `docs/future-image-preloading.md`

**Smart Consolidation**

Stratos v1 only scales down **empty nodes**. It does NOT perform:
- Detection of underutilized nodes
- Pod rescheduling simulation
- Bin-packing optimization

**Future**: Smart consolidation may use LLM-based reasoning (see `docs/future-llm-consolidation.md`)

---

## Key Entities

| Entity | Description |
|--------|-------------|
| NodePool | CRD defining a pool with poolSize, minStandby, and node template |
| Node | Kubernetes Node with Stratos labels for pool/state |
| NodeState | Enum: warmup, standby, running, terminating |
| Instance | Cloud compute instance (EC2) backing a Node |
| CloudProvider | Interface for cloud operations |
| TimeoutAction | What to do on pre-warm timeout: stop or terminate |

---

## Clarifications Log

**Session 2026-01-17**
- Stratos identifies pool instances via Kubernetes Node labels + cloud instance tags
- Watch pending pods (like Karpenter) for scale-up triggers
- Scale-down: consolidation with drain, stop instance (return to standby)
- Stop on scale-down (preserve pre-warming), terminate only on deletion/issues
- Pre-warming: userdata joins, pulls images, self-stops
- Single controller manages multiple NodePools

**Session 2026-01-18**
- Event-driven scale-up + periodic reconciliation (30s default)
- Cluster-scoped least-privilege RBAC
- Renamed from "Presto" to "Stratos" (stratos.sh domain)

**Session 2026-01-19**
- CRD API version: v1alpha1 initially
- Client-side rate limiting with exponential backoff
- Minimum Kubernetes version: 1.27+
