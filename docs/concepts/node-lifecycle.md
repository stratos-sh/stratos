---
sidebar_position: 2
title: Node Lifecycle
description: Understand the Stratos node state machine and lifecycle
---

# Node Lifecycle

Stratos manages nodes through a well-defined state machine. Understanding these states is essential for operating and troubleshooting Stratos.

## State Machine

```
                 On-Demand Path              Spot Path
                 ==============              =========

                    +---------+
                    | warmup  |
                    +----+----+
                         |
           self-stop,    |  timeout (terminate)
           controller-   |       |
           stop, or      |       |
           timeout (stop)|       |
                         v       v
                    +---------+  X (terminated)
                    | standby |
                    +----+----+
                         |
         scale-up        |
         (start instance)|
                         v
                    +---------+     spot replacement     +---------+
                    | running |  ----------------------> | warmup  | (Spot)
                    | (OD)    |                          +----+----+
                    +----+----+                               |
                         |                          stays running (no stop)
         scale-down or   |  external stop                     |
         max-runtime     |  (e.g., spot                       v
                         v   interruption)              +---------+
                    +----------+                        | running | (Spot)
                    |terminating|                       +----+----+
                    +----+-----+                             |
                         |                   spot interruption |  scale-down
         drain complete  |                   (AWS reclaim)     |  (empty node)
         (stop instance) |                          |          |
                         v                          v          v
                    +---------+                X (terminated + K8s node deleted)
                    | standby |  <----- OD standby starts instantly
                    +---------+         via normal scale-up
```

## States

### Warmup

**Cloud State:** Running
**K8s Node:** May not exist yet

A node enters the warmup state when a new instance is launched to replenish the pool. During warmup:

1. The instance boots and runs user data (script or TOML configuration)
2. Joins the Kubernetes cluster
3. Registers with network readiness taint
4. Waits for kubelet to be healthy
5. Transitions to standby (method depends on completion mode)

**Transitions:**
- `warmup -> standby`: Instance stopped (via self-stop or controller-stop) or timeout with stop action
- `warmup -> terminating`: Timeout with terminate action

#### Warmup Completion Modes

Stratos supports two modes for completing warmup:

| Mode | Configuration | How It Works |
|------|--------------|--------------|
| **SelfStop** (default) | `preWarm.completionMode: SelfStop` | User data script calls `poweroff` when ready |
| **ControllerStop** | `preWarm.completionMode: ControllerStop` | Stratos stops the instance when node is Ready |

**SelfStop Mode** (Default for AL2/AL2023)

When using `bootstrapTemplate: AL2023` or `bootstrapTemplate: AL2`, Stratos automatically generates a bootstrap script that:
1. Joins the Kubernetes cluster
2. Registers with network readiness taint
3. Waits for kubelet health
4. Calls `poweroff` when ready

You don't need to write custom userData scripts. Stratos handles the entire bootstrap process.

Use SelfStop mode with:
- Amazon Linux 2023 (`bootstrapTemplate: AL2023`)
- Amazon Linux 2 (`bootstrapTemplate: AL2`)

**ControllerStop Mode**

Stratos monitors the node and stops it when ready:

```yaml
preWarm:
  completionMode: ControllerStop
  timeout: 10m
```

Stratos stops the instance when:
1. The Kubernetes node has `Ready=True`
2. Network is ready (if `networkReadinessStrategy: Taint`)

Use ControllerStop mode with:
- Bottlerocket (TOML-only configuration)
- Any OS where shutdown scripts are impractical
- Environments where you prefer controller-managed warmup

See [Bottlerocket Setup](../guides/bottlerocket.md) for a complete example.

:::tip Image Pre-Pulling
During the warmup phase, Stratos automatically pre-pulls images for all DaemonSets that will run on the node. This eliminates image pull time at scale-up, contributing to Stratos's ~20-25 second scale-up time (compared to Karpenter's ~40-50 seconds).

You can also configure additional images to pre-pull via the `preWarm.imagesToPull` field in the NodePool spec.
:::

### Standby

**Cloud State:** Stopped
**K8s Node:** Exists, cordoned

A standby node is a stopped instance that is ready for instant start. Key characteristics:

- Cloud instance is in stopped state
- Kubernetes node object exists
- Node is cordoned (unschedulable)
- Only incurs EBS storage costs

**Transitions:**
- `standby -> running`: Scale-up triggered
- `standby -> terminating`: Pool deleted or node recycling

### Running

**Cloud State:** Running
**K8s Node:** Ready, schedulable

A running node is actively serving workloads:

- Cloud instance is running
- Kubernetes node is Ready
- Node accepts pod scheduling
- Network readiness taint has been removed

Running nodes have a `stratos.sh/capacity-type` label indicating whether they are `on-demand` or `spot`. On-Demand running nodes may be replaced by Spot nodes when `spotReplacement` is enabled (see [Spot Node Lifecycle](#spot-node-lifecycle) below).

**Transitions:**
- `running -> terminating`: Scale-down, max runtime exceeded, or pool deletion
- `running -> standby`: Instance stopped externally (rare)

### Terminating

**Cloud State:** Running (during drain)
**K8s Node:** Cordoned, draining

A terminating node is being prepared for return to standby:

1. Node is cordoned (unschedulable)
2. Pods are drained (respecting PodDisruptionBudgets)
3. After drain completes, instance is stopped
4. Node transitions to standby

**Transitions:**
- `terminating -> standby`: Drain complete, instance stopped

### Spot Node Lifecycle

When `spotReplacement` is enabled, Spot nodes follow a modified lifecycle compared to On-Demand nodes:

**Launch:** Spot nodes are launched as replacements for On-Demand running nodes. Stratos uses the EC2 `CreateFleet` API (not `RunInstances` with Spot options or the legacy SpotFleet API) with diversified instance types from `spotConfig.instanceTypes` for optimal availability. The fleet is configured as type `instant` with the allocation strategy from `spotConfig.allocationStrategy`.

**Warmup:** Like On-Demand nodes, Spot nodes go through warmup (join cluster, run user data). However, Spot nodes stay running after warmup completes -- they are not stopped to standby. The Spot node is annotated with `stratos.sh/spot-replacing-node` indicating which On-Demand node it replaces.

**Migration:** Once the Spot node completes warmup:
1. The On-Demand node is drained (respecting PodDisruptionBudgets)
2. Workloads migrate to the Spot node
3. The On-Demand node returns to standby (stopped), ready for instant reuse
4. The `stratos_nodepool_spot_replacements_total` metric is incremented

**Interruption:** When AWS reclaims a Spot instance:
1. The Spot node is terminated
2. Stratos cleans up the Kubernetes node object
3. Pods become pending and trigger normal scale-up
4. An On-Demand standby node starts in seconds
5. The `stratos_nodepool_spot_interruptions_total` metric is incremented

:::warning
Spot nodes follow a different scale-down path than On-Demand nodes. When an empty Spot node is selected for scale-down, it is **terminated** (not stopped), because Spot instances cannot be stopped. The Kubernetes node object is also deleted. On-Demand nodes are stopped and returned to standby for reuse.
:::

## Valid Transitions

The state machine enforces these valid transitions:

```go
var ValidTransitions = map[NodeState][]NodeState{
    NodeStateWarmup: {
        NodeStateStandby,     // Instance self-stopped or timed out (stop action)
        NodeStateTerminating, // Timeout with terminate action
    },
    NodeStateStandby: {
        NodeStateRunning,     // Scale-up triggered
        NodeStateTerminating, // Pool deleted or node needs recycling
    },
    NodeStateRunning: {
        NodeStateTerminating, // Scale-down or max runtime exceeded
        NodeStateStandby,     // Instance stopped externally
    },
    NodeStateTerminating: {
        NodeStateStandby,     // Drain complete, instance stopped
    },
}
```

:::warning
Invalid state transitions are rejected by the controller. If you see `invalid state transition` errors, check for external modifications to node labels or instance states.
:::

## Network Readiness Management

The network readiness taint (`stratos.sh/not-ready=true:NoSchedule`) prevents pod scheduling until the CNI is ready. This avoids "connection refused on port 50051" errors during pod sandbox creation.

### Taint Strategy (Default)

When `networkReadinessStrategy` is `Taint` (the default), Stratos automatically applies and manages the `stratos.sh/not-ready=true:NoSchedule` taint. Stratos monitors network conditions and removes the taint when the CNI is ready:

| CNI | Condition | Reason |
|-----|-----------|--------|
| EKS VPC CNI | `NetworkingReady=True` | Set by eks-node-monitoring-agent |
| Cilium | `NetworkUnavailable=False` | `CiliumIsUp` |
| Calico | `NetworkUnavailable=False` | `CalicoIsUp` |

**Timeout:** 2 minutes (after which the taint is forcibly removed)

### None Strategy

When `networkReadinessStrategy` is `None`, Stratos does not apply a network readiness taint. Use this for:
- CNI plugins that manage their own readiness taints
- Environments where network readiness gating is not needed

## Timeouts

### Warmup Timeout

Configured via `preWarm.timeout` (default: 10 minutes).

The timeout behavior depends on the completion mode:

| Mode | Timeout Condition |
|------|------------------|
| SelfStop | Instance doesn't self-stop within the timeout |
| ControllerStop | Node doesn't become Ready within the timeout |

When timeout occurs:

| Action | Behavior |
|--------|----------|
| `stop` (default) | Force stop, transition to standby |
| `terminate` | Terminate instance |

### Drain Timeout

Configured via `scaleDown.drainTimeout` (default: 5 minutes).

If draining doesn't complete within the timeout, pods are forcibly evicted.

### Network Readiness Timeout

Fixed at 2 minutes.

If network conditions don't indicate CNI readiness within 2 minutes (when `networkReadinessStrategy: Taint`), the taint is forcibly removed.

## Max Node Runtime

Configured via `maxNodeRuntime` (optional).

When set, nodes are automatically recycled after running for the specified duration. This is useful for:

- Applying AMI updates
- Clearing potential memory leaks
- Refreshing credentials

```yaml
spec:
  maxNodeRuntime: 24h
```

## Observing State

### Node Labels

Check the current state via labels:

```bash
kubectl get nodes -l stratos.sh/pool=workers \
  -o custom-columns='NAME:.metadata.name,STATE:.metadata.labels.stratos\.sh/state'
```

### NodePool Status

Check aggregate counts:

```bash
kubectl get nodepool workers -o yaml
```

```yaml
status:
  warmup: 0
  standby: 3
  running: 2
  total: 5
```

### Metrics

Monitor state distributions via Prometheus:

```promql
stratos_nodepool_nodes_total{pool="workers"}
```

## Next Steps

- [Cloud Providers](./cloud-providers.md) - Cloud provider abstraction
- [Labels and Annotations](../reference/labels-annotations.md) - Complete label reference
