# Specification: Stratos Core

**Capability**: stratos-core
**Version**: 1.0.0
**Status**: Implemented
**Created**: 2026-01-21
**Migrated From**: specs/001-instance-pool-manager/spec.md

## Overview

Stratos is a Kubernetes node scaler that eliminates node provisioning delays. Unlike Karpenter which provisions nodes on-demand (taking 3-5 minutes), Stratos maintains a pool of pre-warmed, stopped instances that can join the cluster in seconds.

### Key Concepts

- **NodePool**: CRD defining a pool of pre-warmed nodes
- **poolSize**: Maximum total nodes (standby + running)
- **minStandby**: Minimum nodes to keep in stopped/standby state
- **NodeState**: warmup | standby | running | terminating
- **Pre-warming**: Instance initialization followed by self-stop

### Use Cases

- **CI/CD**: Runners ready instantly when jobs are queued
- **ML/LLM Inference**: GPU nodes with models pre-loaded
- **Bursty workloads**: Handle sudden traffic without cold-start delays

---

## Requirements

### NodePool CRD

#### FR-001: NodePool Custom Resource Definition

**Priority**: P1
**Status**: Implemented

Stratos MUST provide a NodePool Custom Resource Definition (API version: v1alpha1, graduating to v1beta1/v1 based on stability) for configuring node pools.

##### Scenario: Create NodePool
- **Given** a Kubernetes cluster with Stratos installed
- **When** a user applies a NodePool manifest
- **Then** Stratos accepts and reconciles the resource

#### FR-002: Configurable Pool Size

**Priority**: P1
**Status**: Implemented

NodePool MUST support configurable `poolSize` (maximum total nodes: standby + running).

##### Scenario: Pool size limit
- **Given** a NodePool with poolSize=10
- **When** the pool has 10 total nodes
- **Then** Stratos does not provision additional nodes

#### FR-003: Configurable Minimum Standby

**Priority**: P1
**Status**: Implemented

NodePool MUST support configurable `minStandby` (minimum nodes to keep in stopped/standby state).

##### Scenario: Maintain standby
- **Given** a NodePool with minStandby=5 and current standby count is 3
- **When** reconciliation runs
- **Then** Stratos provisions 2 new nodes to reach minStandby

#### FR-004: Validation - minStandby vs poolSize

**Priority**: P1
**Status**: Implemented

Stratos MUST validate that minStandby does not exceed poolSize.

##### Scenario: Invalid configuration rejected
- **Given** a NodePool where minStandby exceeds poolSize
- **When** the NodePool is applied
- **Then** it is rejected with a validation error

#### FR-005: Multiple NodePools

**Priority**: P1
**Status**: Implemented

Stratos MUST support multiple NodePool resources in a cluster.

##### Scenario: Independent pools
- **Given** two NodePool resources with different configurations
- **When** both are reconciled
- **Then** each pool maintains its own nodes independently

---

### Node Pre-warming

#### FR-006: Cloud Provider Instance Launch

**Priority**: P1
**Status**: Implemented

Stratos MUST launch instances through the configured cloud provider.

##### Scenario: AWS EC2 launch
- **Given** a NodePool with AWS configuration
- **When** Stratos needs to provision a node
- **Then** it creates an EC2 instance with specified parameters

#### FR-007: Userdata for Cluster Join

**Priority**: P1
**Status**: Implemented

Stratos MUST configure instances with userdata that joins the K8s cluster and self-stops.

##### Scenario: Node initialization
- **Given** a launched instance
- **When** userdata executes
- **Then** the node joins the cluster and the instance self-stops

#### FR-008: Monitor for Self-Stop

**Priority**: P1
**Status**: Implemented

Stratos MUST monitor launched instances waiting for them to self-stop.

##### Scenario: Detect self-stop
- **Given** a launched instance in warmup state
- **When** the instance stops
- **Then** Stratos marks the node as standby

#### FR-009: Configurable Self-Stop Timeout

**Priority**: P1
**Status**: Implemented

Stratos MUST support a configurable timeout for instances to self-stop.

##### Scenario: Timeout configuration
- **Given** a NodePool with preWarm.timeout=15m
- **When** an instance runs longer than 15 minutes during warmup
- **Then** Stratos applies the timeout action

#### FR-010: Timeout Action Configuration

**Priority**: P1
**Status**: Implemented

Stratos MUST support a configurable timeout action: "stop" or "terminate".

##### Scenario: Timeout action stop
- **Given** preWarm.timeoutAction=stop
- **When** an instance exceeds the timeout
- **Then** Stratos stops the instance

##### Scenario: Timeout action terminate
- **Given** preWarm.timeoutAction=terminate
- **When** an instance exceeds the timeout
- **Then** Stratos terminates the instance and provisions a replacement

#### FR-011: Apply Timeout Action

**Priority**: P1
**Status**: Implemented

Stratos MUST apply the configured timeout action when instances fail to self-stop.

##### Scenario: Stuck instance handling
- **Given** an instance that has not self-stopped within the timeout
- **When** the timeout expires
- **Then** Stratos applies the configured action (stop or terminate)

#### FR-012: Node Labeling

**Priority**: P1
**Status**: Implemented

Stratos MUST label pre-warmed Nodes with NodePool ownership and standby status.

##### Scenario: Node labels applied
- **Given** a pre-warmed standby node
- **When** viewed in kubectl
- **Then** it shows labels: stratos.sh/pool, stratos.sh/state, stratos.sh/instance-id

---

### Scale-Up (Event-Driven)

#### FR-013: Watch Pod Events

**Priority**: P1
**Status**: Implemented

Stratos MUST watch for Kubernetes pod events and react immediately to unschedulable pods.

##### Scenario: Pending pod detected
- **Given** a pod with PodScheduled=False due to insufficient resources
- **When** Stratos observes the event
- **Then** it evaluates the pod for scale-up

#### FR-014: Match Pods to NodePools

**Priority**: P1
**Status**: Implemented

Stratos MUST evaluate pending pods against NodePool requirements (node selectors, taints/tolerations).

##### Scenario: Pod matches NodePool
- **Given** a pending pod with nodeSelector matching a NodePool
- **When** Stratos evaluates it
- **Then** the pod is considered for that NodePool

##### Scenario: Pod does not match
- **Given** a pending pod with requirements not matching any NodePool
- **When** Stratos evaluates it
- **Then** Stratos ignores the pod

#### FR-015: Start Standby Nodes

**Priority**: P1
**Status**: Implemented

Stratos MUST start standby nodes when pending pods can be satisfied.

##### Scenario: Scale-up triggered
- **Given** pending pods that can be satisfied by a standby node
- **When** Stratos decides to scale up
- **Then** it starts the standby node

#### FR-016: Start Only Needed Nodes

**Priority**: P1
**Status**: Implemented

Stratos MUST NOT start more nodes than needed to satisfy pending pods.

##### Scenario: Minimal scale-up
- **Given** 3 pending pods requiring 2 nodes
- **When** Stratos scales up
- **Then** only 2 nodes are started

#### FR-017: Respect Pool Size Limit

**Priority**: P1
**Status**: Implemented

Stratos MUST NOT exceed poolSize total nodes (standby + running).

##### Scenario: Pool size enforced
- **Given** poolSize=10 with 10 nodes already (7 running + 3 standby)
- **When** more pods become pending
- **Then** no additional nodes are started

#### FR-018: Fast Node Ready

**Priority**: P1
**Status**: Implemented

Started nodes MUST become Ready within seconds (not minutes).

##### Scenario: Quick startup
- **Given** a standby node is started
- **When** it transitions to running
- **Then** it becomes Ready within 30 seconds

---

### Scale-Down

#### FR-019: Detect Empty Nodes

**Priority**: P1
**Status**: Implemented

Stratos MUST detect empty nodes (no pods excluding DaemonSets).

##### Scenario: Empty node identified
- **Given** a running node with only DaemonSet pods
- **When** Stratos evaluates the node
- **Then** it is considered empty

#### FR-020: Empty Node TTL

**Priority**: P1
**Status**: Implemented

Stratos MUST support configurable emptyNodeTTL (duration before stopping empty nodes).

##### Scenario: TTL elapsed
- **Given** an empty node and emptyNodeTTL=5m
- **When** the node has been empty for 5 minutes
- **Then** Stratos initiates scale-down

#### FR-021: Cordon Before Drain

**Priority**: P1
**Status**: Implemented

Stratos MUST cordon nodes before draining.

##### Scenario: Node cordoned
- **Given** a node selected for scale-down
- **When** Stratos begins the process
- **Then** the node is cordoned first

#### FR-022: Respect PDBs During Drain

**Priority**: P1
**Status**: Implemented

Stratos MUST drain nodes respecting PodDisruptionBudgets.

##### Scenario: PDB respected
- **Given** a node with pods protected by PDBs
- **When** Stratos drains the node
- **Then** it waits for PDB conditions to allow eviction

#### FR-023: Stop Not Terminate

**Priority**: P1
**Status**: Implemented

Stratos MUST stop (not terminate) nodes on scale-down to preserve pre-warming.

##### Scenario: Node stopped
- **Given** a drained node
- **When** scale-down completes
- **Then** the instance is stopped, not terminated

#### FR-024: Return to Standby

**Priority**: P1
**Status**: Implemented

Stopped nodes MUST return to standby pool and be available for future scale-up.

##### Scenario: Node returns to standby
- **Given** a node that was stopped during scale-down
- **When** it stops successfully
- **Then** it is marked as standby and can be started again

#### FR-025: Disable Scale-Down

**Priority**: P1
**Status**: Implemented

Stratos MUST support disabling automatic scale-down per NodePool.

##### Scenario: Scale-down disabled
- **Given** a NodePool with scaleDown.enabled=false
- **When** nodes become empty
- **Then** Stratos does not stop them

---

### Reconciliation (Periodic Pool Maintenance)

#### FR-026: Periodic Reconciliation Loop

**Priority**: P1
**Status**: Implemented

Stratos MUST run a periodic reconciliation loop to maintain pool health (configurable interval, default 30 seconds).

##### Scenario: Regular maintenance
- **Given** a running Stratos controller
- **When** the sync period elapses
- **Then** all NodePools are reconciled

#### FR-027: Replenish Standby

**Priority**: P1
**Status**: Implemented

Stratos MUST provision new nodes when standby count is below minStandby.

##### Scenario: Standby replenishment
- **Given** minStandby=5 and current standby=3
- **When** reconciliation runs
- **Then** 2 new nodes are provisioned

#### FR-028: Handle External Termination

**Priority**: P1
**Status**: Implemented

Stratos MUST detect and handle externally terminated instances.

##### Scenario: External termination detected
- **Given** a standby node whose instance was terminated externally
- **When** Stratos detects this
- **Then** it removes the stale Node object

#### FR-029: State Recovery on Restart

**Priority**: P1
**Status**: Implemented

Stratos MUST recover state on controller restart.

##### Scenario: Controller restart
- **Given** Stratos restarts
- **When** it initializes
- **Then** it reconciles all NodePools and recovers any inconsistent state

---

### Cloud Provider

#### FR-030: AWS EC2 Support

**Priority**: P1
**Status**: Implemented

Stratos MUST support AWS (EC2) as a cloud provider.

##### Scenario: AWS instances
- **Given** a NodePool with AWS configuration
- **When** nodes are provisioned
- **Then** EC2 instances are created with specified parameters

#### FR-031: Pluggable Provider Interface

**Priority**: P1
**Status**: Implemented

Stratos MUST support pluggable cloud provider implementations for future providers.

##### Scenario: Provider interface
- **Given** the CloudProvider interface
- **When** implementing a new provider
- **Then** only launch, start, stop, get state, and terminate need to be implemented

#### FR-032: Required Provider Operations

**Priority**: P1
**Status**: Implemented

Cloud provider MUST support: launch, start, stop, get state, and terminate operations.

##### Scenario: Operation completeness
- **Given** the CloudProvider interface
- **Then** all five operations are defined and callable

#### FR-033: Instance Tagging

**Priority**: P1
**Status**: Implemented

Stratos MUST tag all managed instances with NodePool name and cluster identifier.

##### Scenario: Tags applied
- **Given** a launched instance
- **When** created by Stratos
- **Then** it has tags: managed-by=stratos, stratos.sh/pool, stratos.sh/cluster

#### FR-034: Client-Side Rate Limiting

**Priority**: P1
**Status**: Implemented

Stratos MUST implement client-side rate limiting for cloud API calls to avoid hitting provider limits.

##### Scenario: Rate limiting
- **Given** many concurrent cloud operations
- **When** the rate limit is reached
- **Then** requests are queued rather than rejected

#### FR-035: Exponential Backoff

**Priority**: P1
**Status**: Implemented

Stratos MUST use exponential backoff when retrying failed cloud API calls (including rate limit/throttle errors).

##### Scenario: Retry with backoff
- **Given** a cloud API call fails with a transient error
- **When** Stratos retries
- **Then** it uses exponential backoff

---

### Maximum Node Runtime

#### FR-036: Configurable Max Runtime

**Priority**: P2
**Status**: Implemented

NodePool MUST support configurable maxNodeRuntime (optional, 0 = disabled).

##### Scenario: Max runtime set
- **Given** a NodePool with maxNodeRuntime=24h
- **When** a node runs longer than 24 hours
- **Then** it is marked for recycling

#### FR-037: Auto-Recycle Long-Running Nodes

**Priority**: P2
**Status**: Implemented

Stratos MUST automatically drain and stop nodes that exceed maxNodeRuntime.

##### Scenario: Node recycled
- **Given** a node running longer than maxNodeRuntime
- **When** Stratos detects this
- **Then** it drains and stops the node

#### FR-038: Warning Before Max Runtime

**Priority**: P2
**Status**: Implemented

Stratos MUST emit warning events when nodes approach maxNodeRuntime threshold.

##### Scenario: Warning emitted
- **Given** a node approaching maxNodeRuntime
- **When** it crosses the warning threshold
- **Then** a Kubernetes event is emitted

---

### Observability

#### FR-039: Pool Size Metrics

**Priority**: P1
**Status**: Implemented

Stratos MUST expose Prometheus metrics for: pool size, standby count, running count, warmup count.

##### Scenario: Metrics available
- **Given** a running Stratos controller
- **When** /metrics is scraped
- **Then** stratos_nodepool_nodes_total is available with state labels

#### FR-040: Scale-Up Metrics

**Priority**: P1
**Status**: Implemented

Stratos MUST expose metrics for scale-up operations: count, latency (time from pending to scheduled).

##### Scenario: Scale-up latency tracked
- **Given** a scale-up operation
- **When** it completes
- **Then** stratos_scaleup_duration_seconds is recorded

#### FR-041: Scale-Down Metrics

**Priority**: P1
**Status**: Implemented

Stratos MUST expose metrics for scale-down operations: count, drain duration.

##### Scenario: Scale-down metrics
- **Given** a scale-down operation
- **When** it completes
- **Then** stratos_scaledown_total and stratos_drain_duration_seconds are recorded

#### FR-042: Kubernetes Events

**Priority**: P1
**Status**: Implemented

Stratos MUST emit Kubernetes events for significant operations (node started, node stopped, errors).

##### Scenario: Events emitted
- **Given** a node is started
- **When** the operation completes
- **Then** a NodeStarted event is visible on the NodePool

---

### Security & RBAC

#### FR-043: Least-Privilege RBAC

**Priority**: P1
**Status**: Implemented

Stratos MUST operate with cluster-scoped least-privilege RBAC permissions.

##### Scenario: Minimal permissions
- **Given** the Stratos ClusterRole
- **Then** it has only the permissions required for operation

#### FR-044: Node Access

**Priority**: P1
**Status**: Implemented

Stratos MUST have full access to Node objects (get, list, watch, create, update, patch, delete).

##### Scenario: Node operations
- **Given** Stratos needs to manage nodes
- **Then** the ClusterRole grants full Node access

#### FR-045: Pod Watch Access

**Priority**: P1
**Status**: Implemented

Stratos MUST have watch access to Pod objects cluster-wide (get, list, watch).

##### Scenario: Pod monitoring
- **Given** Stratos needs to detect pending pods
- **Then** the ClusterRole grants Pod read access

#### FR-046: NodePool CRD Access

**Priority**: P1
**Status**: Implemented

Stratos MUST have full access to NodePool CRD objects (get, list, watch, create, update, patch, delete).

##### Scenario: CRD management
- **Given** Stratos manages NodePool resources
- **Then** the ClusterRole grants full NodePool access

#### FR-047: Event Access

**Priority**: P1
**Status**: Implemented

Stratos MUST have create access to Event objects for operational visibility.

##### Scenario: Event creation
- **Given** Stratos needs to emit events
- **Then** the ClusterRole grants Event create access

#### FR-048: No Cluster-Admin

**Priority**: P1
**Status**: Implemented

Stratos MUST NOT require cluster-admin or access to secrets outside its own namespace.

##### Scenario: Limited scope
- **Given** the Stratos ClusterRole
- **Then** it does not grant cluster-admin or broad secret access

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

### Image Preloading

Stratos v1 does not manage container image pre-pulling during node pre-warming. The userdata script is responsible for any image pulling.

**Future**: See `docs/future-image-preloading.md`

### Smart Consolidation

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

### Session 2026-01-17
- Stratos identifies pool instances via Kubernetes Node labels + cloud instance tags
- Watch pending pods (like Karpenter) for scale-up triggers
- Scale-down: consolidation with drain, stop instance (return to standby)
- Stop on scale-down (preserve pre-warming), terminate only on deletion/issues
- Pre-warming: userdata joins, pulls images, self-stops
- Single controller manages multiple NodePools

### Session 2026-01-18
- Event-driven scale-up + periodic reconciliation (30s default)
- Cluster-scoped least-privilege RBAC
- Renamed from "Presto" to "Stratos" (stratos.sh domain)

### Session 2026-01-19
- CRD API version: v1alpha1 initially
- Client-side rate limiting with exponential backoff
- Minimum Kubernetes version: 1.27+
