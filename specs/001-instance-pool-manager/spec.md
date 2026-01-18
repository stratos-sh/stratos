# Feature Specification: Stratos - Kubernetes Node Scaler

**Feature Branch**: `001-instance-pool-manager`
**Created**: 2026-01-17
**Status**: Draft
**Type**: Kubernetes Operator
**Input**: "A Kubernetes node scaler that pre-warms nodes for instant scaling, replacing Karpenter with a faster alternative"

## Overview

Stratos is a Kubernetes node scaler that eliminates node provisioning delays. Unlike Karpenter which provisions nodes on-demand (taking 3-5 minutes), Stratos maintains a pool of pre-warmed, stopped instances that can join the cluster in seconds.

**How Stratos works:**
1. **Pre-warm**: Stratos launches instances that run initialization (join cluster, pull images) then self-stop
2. **Standby**: Stopped instances wait in the pool, costing minimal resources
3. **Instant Scale**: When pods are pending, Stratos starts a pre-warmed instance (seconds, not minutes)
4. **Scale Down**: When nodes are idle, Stratos drains and stops them, returning to standby

**Stratos vs Karpenter:**

| Aspect | Karpenter | Stratos |
|--------|-----------|--------|
| Provisioning | On-demand (3-5 min) | Pre-warmed (seconds) |
| Node readiness | Cold start every time | Already configured, images pulled |
| Cost model | Pay only when running | Small cost for stopped instances |
| Best for | General workloads | Time-sensitive scaling (CI/CD, ML inference, bursty traffic) |

**Use cases:**
- **CI/CD**: Runners ready instantly when jobs are queued
- **ML/LLM Inference**: GPU nodes with models pre-loaded, ready for traffic spikes
- **Bursty workloads**: Handle sudden traffic without cold-start delays

## Clarifications

### Session 2026-01-17

- Q: How does Stratos identify which instances belong to its managed pool? → A: Kubernetes Node objects with Stratos labels, backed by cloud instance tags
- Q: How should Stratos detect when nodes are needed? → A: Watch for pending pods that match NodePool requirements (like Karpenter)
- Q: How should Stratos handle scale-down? → A: Consolidation - detect underutilized/empty nodes, drain them, stop the instance (return to standby)
- Q: Should nodes be terminated or stopped on scale-down? → A: Stopped (to preserve pre-warming). Terminate only on NodePool deletion or instance issues.
- Q: How should pre-warming work? → A: Userdata script joins cluster, pulls images, then stops the instance. Stratos detects stopped state = ready for standby.
- Q: Should a single Stratos deployment manage multiple NodePools? → A: Yes - single controller manages multiple NodePool CRDs

### Session 2026-01-18

- Q: What is the reconciliation model for Stratos? → A: Event-driven scale-up (watch unschedulable pod events for immediate response) + periodic reconciliation loop (configurable interval, default 30s) for pool maintenance (replenishing standby, detecting failures, cleanup)
- Q: What Kubernetes RBAC scope should Stratos require? → A: Cluster-scoped least-privilege (Nodes: full, Pods: watch, NodePools CRD: full, Events: create)
- Q: Project rename? → A: Renamed from "Presto" to "Stratos" to avoid PrestoDB conflict. Domain: stratos.sh

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create and Manage NodePool (Priority: P1)

As a Kubernetes operator, I want to create a NodePool resource that defines my pre-warmed node capacity so that I have standby nodes ready for instant scaling.

**Why this priority**: This is the foundational capability - without NodePool configuration, no pre-warming or scaling can occur.

**Independent Test**: Can be tested by creating a NodePool with poolSize=10 and minStandby=5, then verifying that 5 nodes are pre-warmed and in standby.

**Acceptance Scenarios**:

1. **Given** a NodePool with poolSize=10 and minStandby=5, **When** the NodePool is created, **Then** Stratos launches 5 instances that join the cluster, self-stop, and become standby nodes
2. **Given** a running NodePool with 5 standby nodes, **When** minStandby is increased to 8, **Then** Stratos provisions 3 additional pre-warmed nodes
3. **Given** a NodePool where minStandby exceeds poolSize, **When** the NodePool is applied, **Then** it is rejected with a validation error
4. **Given** a NodePool with invalid parameters, **When** the NodePool is applied, **Then** it is rejected with a clear validation error
5. **Given** a NodePool, **When** it is deleted, **Then** Stratos terminates all associated nodes (both running and standby)

---

### User Story 2 - Node Pre-warming Lifecycle (Priority: P1)

As a Kubernetes operator, I want each node to launch, join the cluster, pull required images, and then stop so that nodes are fully pre-warmed and ready for instant use.

**Why this priority**: The pre-warming lifecycle is the core mechanism that enables fast scaling - nodes must be fully configured before becoming standby.

**Independent Test**: Can be tested by creating a NodePool and verifying nodes join the cluster, execute initialization, and self-stop.

**Acceptance Scenarios**:

1. **Given** a NodePool, **When** Stratos launches a new instance, **Then** the instance runs userdata that joins the cluster and self-stops
2. **Given** a launched instance, **When** the Node appears in the cluster and the instance self-stops within the timeout, **Then** the node is marked as standby and ready
3. **Given** a launched instance, **When** the instance does NOT self-stop within the timeout AND timeout action is "stop", **Then** Stratos manually stops the instance
4. **Given** a launched instance, **When** the instance does NOT self-stop within the timeout AND timeout action is "terminate", **Then** Stratos terminates the instance and provisions a replacement
5. **Given** a pre-warmed standby node, **When** viewed in kubectl, **Then** it shows as a Node with a Stratos-managed label and appropriate status

---

### User Story 3 - Automatic Scale-Up (Priority: P1)

As a Kubernetes operator, I want Stratos to automatically start pre-warmed nodes when pods are pending so that workloads get scheduled in seconds instead of minutes.

**Why this priority**: This is the core value proposition - instant scaling when demand increases.

**Independent Test**: Can be tested by deploying pods that require more capacity than currently available, and verifying Stratos starts standby nodes to meet demand.

**Acceptance Scenarios**:

1. **Given** pending pods that can't be scheduled due to insufficient capacity, **When** standby nodes are available, **Then** Stratos starts standby nodes to satisfy the pending pods
2. **Given** pending pods, **When** a standby node is started, **Then** the node becomes Ready within seconds and pods are scheduled
3. **Given** multiple pending pods requiring multiple nodes, **When** standby nodes are started, **Then** Stratos starts enough nodes to satisfy demand (up to poolSize)
4. **Given** pending pods, **When** no standby nodes are available AND pool is at poolSize limit, **Then** pods remain pending until capacity is released
5. **Given** pending pods that don't match any NodePool (wrong requirements), **When** Stratos evaluates them, **Then** Stratos ignores them (doesn't attempt to scale)

---

### User Story 4 - Automatic Scale-Down (Priority: P1)

As a Kubernetes operator, I want Stratos to automatically stop empty nodes and return them to standby so that I maintain my pre-warmed pool without wasting resources.

**Why this priority**: Without scale-down, nodes would run forever, defeating the cost benefits of the standby pool.

**Independent Test**: Can be tested by scaling down a deployment, verifying nodes become empty, and confirming Stratos drains and stops them.

**Acceptance Scenarios**:

1. **Given** a running node with no pods (excluding DaemonSets), **When** the node has been empty for the configured emptyNodeTTL duration, **Then** Stratos cordons, drains, and stops the node (returns to standby)
2. **Given** a node being drained, **When** pods have PodDisruptionBudgets, **Then** Stratos respects the PDBs during drain
3. **Given** a node being stopped, **When** the stop completes, **Then** the node returns to standby and is available for future scale-up
4. **Given** scale-down settings, **When** scale-down is disabled, **Then** nodes remain running until manually intervened

---

### User Story 5 - NodePool Reconciliation (Priority: P1)

As a Kubernetes operator, I want Stratos to continuously reconcile actual node state with desired NodePool state so that my standby capacity is maintained automatically.

**Why this priority**: Continuous reconciliation ensures the pool self-heals without manual intervention.

**Independent Test**: Can be tested by manually terminating standby instances and verifying Stratos detects and provisions replacements.

**Acceptance Scenarios**:

1. **Given** poolSize=10, minStandby=5, and current standby count is 3, **When** reconciliation runs, **Then** Stratos provisions 2 new nodes to reach minStandby
2. **Given** poolSize=10, minStandby=5, and current standby count is 5, **When** reconciliation runs, **Then** no action is taken
3. **Given** poolSize=10, minStandby=5, with 7 running and 3 standby (10 total at poolSize limit), **When** reconciliation runs, **Then** no new nodes are provisioned (cannot exceed poolSize)
4. **Given** a standby node whose underlying instance was externally terminated, **When** Stratos detects this, **Then** it removes the stale Node and provisions a replacement
5. **Given** a running node whose underlying instance was externally terminated, **When** Stratos detects this, **Then** it removes the stale Node and replenishes standby if needed

---

### User Story 6 - Cloud Provider Support (Priority: P1)

As a Kubernetes operator, I want Stratos to work with my cloud provider (AWS, GCP, Azure) so that I can use pre-warmed nodes in my environment.

**Why this priority**: Cloud provider support is essential for Stratos to be usable.

**Independent Test**: Can be tested by creating a NodePool with cloud-specific configuration and verifying instances are created correctly.

**Acceptance Scenarios**:

1. **Given** a NodePool with AWS configuration, **When** nodes are provisioned, **Then** Stratos creates EC2 instances with the specified instance type, AMI, subnets, and security groups
2. **Given** cloud provider credentials, **When** Stratos starts, **Then** it authenticates and can perform instance operations
3. **Given** a cloud provider operation fails, **When** the error occurs, **Then** Stratos retries with backoff and reports the error via events/metrics
4. **Given** a new cloud provider, **When** implementing support, **Then** only the required operations need to be implemented (launch, start, stop, get state, terminate)

---

### User Story 7 - Graceful Controller Shutdown (Priority: P2)

As a Kubernetes operator, I want Stratos to shut down gracefully so that no orphaned resources are left behind when the controller restarts.

**Why this priority**: Graceful shutdown prevents resource leaks during upgrades/restarts, but the core functionality works without it initially.

**Independent Test**: Can be tested by restarting the Stratos controller during active operations and verifying all nodes are accounted for after restart.

**Acceptance Scenarios**:

1. **Given** a running Stratos controller, **When** shutdown is initiated (SIGTERM), **Then** Stratos stops accepting new operations
2. **Given** in-progress operations during shutdown, **When** shutdown is initiated, **Then** Stratos waits for operations to complete or times out
3. **Given** a Stratos restart, **When** the controller comes back up, **Then** it reconciles all NodePools and recovers any inconsistent state

---

### User Story 8 - Maximum Node Runtime (Priority: P2)

As a Kubernetes operator, I want to configure a maximum runtime for nodes so that long-running nodes are automatically recycled, preventing drift and resource leaks.

**Why this priority**: Prevents nodes from running indefinitely and accumulating drift. Lower priority because operators can monitor and manually recycle.

**Independent Test**: Can be tested by configuring maxNodeRuntime, running a node beyond the limit, and verifying Stratos recycles it.

**Acceptance Scenarios**:

1. **Given** a NodePool with maxNodeRuntime configured, **When** a node has been running longer than the limit, **Then** Stratos cordons, drains, and stops the node (returns to standby)
2. **Given** a NodePool with maxNodeRuntime, **When** a node approaches the limit, **Then** Stratos emits a warning event/metric
3. **Given** a NodePool without maxNodeRuntime (or set to 0), **When** nodes run indefinitely, **Then** no automatic recycling occurs

---

### Edge Cases

**Pre-warming:**
- What happens when an instance never self-stops? Stratos applies the configured timeout action (stop or terminate) after the timeout period expires.
- What happens when an instance joins the cluster but never self-stops? Stratos detects the timeout and applies the configured action.
- What happens when userdata fails and the instance doesn't join the cluster? Stratos detects the timeout and terminates the instance, provisioning a replacement.
- What happens when an instance is stuck in "stopping" state? Stratos detects this and terminates the instance after a secondary timeout.

**Scale-Up:**
- What happens when pending pods require more nodes than standby available? Stratos starts all available standby nodes; remaining pods stay pending until capacity is freed or new nodes are pre-warmed.
- What happens when no standby nodes are available? Pods remain pending. Stratos cannot provision faster than the pre-warming cycle.
- What happens when a standby node fails to start? Stratos retries or terminates and provisions a replacement, then tries another standby node.
- What happens when multiple pods race for the last standby node? Stratos starts the node; K8s scheduler handles pod placement.

**Scale-Down:**
- What happens when an empty node has DaemonSet pods with PDBs? Stratos respects PDBs and waits or skips the node.
- What happens when drain takes too long? A configurable timeout applies; Stratos may force-drain or skip the node.
- What happens when stop fails during scale-down? Stratos retries and eventually terminates if stop continues to fail.
- What happens when a node has local storage (emptyDir, local PV) from DaemonSets? Stratos warns but proceeds with drain.

**Reconciliation:**
- What happens when cloud provider operations fail? Stratos retries with backoff and reports errors via Kubernetes events and metrics.
- What happens when the Stratos controller restarts? It reconciles all NodePools and recovers any inconsistent state from the cluster.
- What happens when a Node exists but the underlying instance was terminated? Stratos deletes the stale Node object and provisions a replacement.
- What happens when standby < minStandby but pool is at poolSize limit? Stratos cannot provision more nodes; it reports this state via metrics but takes no action until running nodes are released.

**NodePool Management:**
- What happens when a NodePool is deleted while nodes are running? Stratos drains and terminates all nodes associated with the NodePool.
- What happens when NodePool spec is updated? Stratos reconciles to the new desired state (may provision or consolidate nodes).
- What happens when two NodePools have overlapping node selectors? Stratos handles them independently; K8s scheduler decides which node gets which pod.

## Requirements *(mandatory)*

### Functional Requirements

**NodePool CRD:**
- **FR-001**: Stratos MUST provide a NodePool Custom Resource Definition for configuring node pools
- **FR-002**: NodePool MUST support configurable poolSize (maximum total nodes: standby + running)
- **FR-003**: NodePool MUST support configurable minStandby (minimum nodes to keep in stopped/standby state)
- **FR-004**: Stratos MUST validate that minStandby does not exceed poolSize
- **FR-005**: Stratos MUST support multiple NodePool resources in a cluster

**Node Pre-warming:**
- **FR-006**: Stratos MUST launch instances through the configured cloud provider
- **FR-007**: Stratos MUST configure instances with userdata that joins the K8s cluster and self-stops
- **FR-008**: Stratos MUST monitor launched instances waiting for them to self-stop
- **FR-009**: Stratos MUST support a configurable timeout for instances to self-stop
- **FR-010**: Stratos MUST support a configurable timeout action: "stop" or "terminate"
- **FR-011**: Stratos MUST apply the configured timeout action when instances fail to self-stop
- **FR-012**: Stratos MUST label pre-warmed Nodes with NodePool ownership and standby status

**Scale-Up (Event-Driven):**
- **FR-013**: Stratos MUST watch for Kubernetes pod events and react immediately to unschedulable pods
- **FR-014**: Stratos MUST evaluate pending pods against NodePool requirements (node selectors, taints/tolerations)
- **FR-015**: Stratos MUST start standby nodes when pending pods can be satisfied
- **FR-016**: Stratos MUST NOT start more nodes than needed to satisfy pending pods
- **FR-017**: Stratos MUST NOT exceed poolSize total nodes (standby + running)
- **FR-018**: Started nodes MUST become Ready within seconds (not minutes)

**Scale-Down:**
- **FR-019**: Stratos MUST detect empty nodes (no pods excluding DaemonSets)
- **FR-020**: Stratos MUST support configurable emptyNodeTTL (duration before stopping empty nodes)
- **FR-021**: Stratos MUST cordon nodes before draining
- **FR-022**: Stratos MUST drain nodes respecting PodDisruptionBudgets
- **FR-023**: Stratos MUST stop (not terminate) nodes on scale-down to preserve pre-warming
- **FR-024**: Stopped nodes MUST return to standby pool and be available for future scale-up
- **FR-025**: Stratos MUST support disabling automatic scale-down per NodePool

**Reconciliation (Periodic Pool Maintenance):**
- **FR-026**: Stratos MUST run a periodic reconciliation loop to maintain pool health (configurable interval, default 30 seconds)
- **FR-027**: Stratos MUST provision new nodes when standby count is below minStandby
- **FR-028**: Stratos MUST detect and handle externally terminated instances
- **FR-029**: Stratos MUST recover state on controller restart

**Cloud Provider:**
- **FR-030**: Stratos MUST support AWS (EC2) as a cloud provider
- **FR-031**: Stratos MUST support pluggable cloud provider implementations for future providers
- **FR-032**: Cloud provider MUST support: launch, start, stop, get state, and terminate operations
- **FR-033**: Stratos MUST tag all managed instances with NodePool name and cluster identifier

**Maximum Node Runtime:**
- **FR-034**: NodePool MUST support configurable maxNodeRuntime (optional, 0 = disabled)
- **FR-035**: Stratos MUST automatically drain and stop nodes that exceed maxNodeRuntime
- **FR-036**: Stratos MUST emit warning events when nodes approach maxNodeRuntime threshold

**Observability:**
- **FR-037**: Stratos MUST expose Prometheus metrics for: pool size, standby count, running count, warmup count
- **FR-038**: Stratos MUST expose metrics for scale-up operations: count, latency (time from pending to scheduled)
- **FR-039**: Stratos MUST expose metrics for scale-down operations: count, drain duration
- **FR-040**: Stratos MUST emit Kubernetes events for significant operations (node started, node stopped, errors)

**Security & RBAC:**
- **FR-041**: Stratos MUST operate with cluster-scoped least-privilege RBAC permissions
- **FR-042**: Stratos MUST have full access to Node objects (get, list, watch, create, update, patch, delete)
- **FR-043**: Stratos MUST have watch access to Pod objects cluster-wide (get, list, watch)
- **FR-044**: Stratos MUST have full access to NodePool CRD objects (get, list, watch, create, update, patch, delete)
- **FR-045**: Stratos MUST have create access to Event objects for operational visibility
- **FR-046**: Stratos MUST NOT require cluster-admin or access to secrets outside its own namespace

### Key Entities

- **NodePool**: Custom Resource defining a pool of pre-warmed nodes with poolSize, minStandby, and node template configuration
- **Node**: A Kubernetes Node object representing a cluster node, managed by Stratos with labels indicating pool ownership and state
- **NodeState**: The state of a Stratos-managed node: warmup (initializing), running (active, serving pods), or standby (stopped, ready to start)
- **Instance**: The underlying cloud compute instance (EC2 instance, GCP VM, etc.) backing a Node
- **CloudProvider**: Abstraction for cloud operations: launch, start, stop, get state, terminate
- **TimeoutAction**: What to do when an instance fails to self-stop during pre-warming: "stop" or "terminate"
- **poolSize**: Maximum total nodes in the pool (standby + running, excluding warmup)
- **minStandby**: Minimum number of nodes to maintain in stopped/standby state
- **emptyNodeTTL**: Duration a node must be empty before Stratos stops it and returns it to standby
- **reconciliationInterval**: Configurable interval for the periodic pool maintenance loop (default: 30 seconds)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: NodePool reaches minStandby count within 2 reconciliation cycles after creation (default: ~1 minute)
- **SC-002**: NodePool never exceeds poolSize total nodes (standby + running)
- **SC-003**: When standby nodes are consumed, pool replenishes to minStandby within 2 reconciliation cycles (default: ~1 minute)
- **SC-004**: Pending pods trigger scale-up immediately via event-driven watcher and get scheduled within 30 seconds (node start time)
- **SC-005**: Pre-warmed nodes become Ready within 10 seconds of being started (vs 3-5 minutes for cold start)
- **SC-006**: Empty nodes are detected and returned to standby within configured TTL
- **SC-007**: Stratos controller restarts without losing node state or causing disruption
- **SC-008**: Prometheus metrics are exposed and accurately reflect pool state
- **SC-009**: Kubernetes events are emitted for all significant operations
- **SC-010**: Failed instances are detected and replaced within 2 reconciliation cycles (default: ~1 minute)
- **SC-011**: Nodes exceeding maxNodeRuntime are recycled automatically
- **SC-012**: PodDisruptionBudgets are respected during drain operations

## Assumptions

- Kubernetes cluster is running and accessible by the Stratos controller
- Stratos is deployed with appropriate RBAC permissions (ClusterRole with least-privilege access)
- Cloud provider credentials are configured (via IAM roles, service accounts, or secrets in Stratos's namespace)
- The cloud provider supports stop and start operations that preserve instance state
- A base AMI/image is available with Kubernetes components (kubelet, container runtime) pre-installed
- Userdata can join nodes to the cluster (cluster endpoint and authentication are configured)
- The userdata script self-stops the instance after initialization completes
- Nodes can be started from stopped state and rejoin the cluster without re-initialization
- The Kubernetes scheduler will schedule pods onto nodes that become Ready
- Stratos has permissions to create, update, and delete Node objects in the cluster

## Out of Scope

The following features are explicitly out of scope for v1:

### Image Preloading

Stratos v1 does not manage container image pre-pulling during node pre-warming. The userdata script is responsible for any image pulling during initialization. Stratos does not:

- Configure which images to pre-pull
- Monitor image pull progress
- Block standby status based on image availability

**Future direction**: See `docs/future-image-preloading.md` for potential approaches to managed image preloading.

### Smart Consolidation

Stratos v1 only scales down **empty nodes** (no pods excluding DaemonSets) after the configured `emptyNodeTTL`. It does NOT perform smart consolidation, which would involve:

- Detecting "underutilized" nodes (partial CPU/memory usage)
- Simulating pod rescheduling to determine if pods can be moved
- Bin-packing optimization across nodes
- Proactive consolidation of nodes with running workloads

**Future direction**: Smart consolidation may be implemented in a future version using LLM-based reasoning to evaluate rescheduling decisions, rather than traditional algorithmic approaches.
