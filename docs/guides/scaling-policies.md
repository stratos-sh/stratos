---
sidebar_position: 2
title: Scaling Policies
description: Configure Stratos scaling behavior
---

# Scaling Policies

This guide covers how to configure Stratos scaling behavior, including scale-up, scale-down, and pool maintenance.

## Pool Sizing

### Core Parameters

| Parameter | Description | Impact |
|-----------|-------------|--------|
| `poolSize` | Maximum total nodes (standby + running) | Limits maximum capacity |
| `minStandby` | Minimum standby nodes to maintain | Controls scale-up speed |

```yaml
spec:
  poolSize: 20      # Max 20 nodes total
  minStandby: 5     # Always keep 5 ready to start
```

### Sizing Guidelines

**poolSize:**
- Set to peak running nodes + minStandby + buffer
- Account for temporary warmup nodes
- Consider cost implications (EBS storage for standby)

**minStandby:**
- Set based on expected burst size
- Higher = faster scale-up, higher storage cost
- Lower = slower scale-up for large bursts, lower cost

:::tip
Start with `minStandby` equal to your typical burst size. Monitor `stratos_nodepool_nodes_total{state="standby"}` and adjust based on how often you hit 0 standby.
:::

## Scale-Up Configuration

### Resource-Based Calculation

Stratos calculates how many nodes to start based on pending pod resource requests:

```yaml
spec:
  scaleUp:
    defaultPodResources:
      requests:
        cpu: "500m"
        memory: "1Gi"
```

When pods don't have explicit resource requests, Stratos uses these defaults for scale-up calculations.

### How Scale-Up Works

1. Controller detects unschedulable pods
2. Calculates total resource requests (CPU, memory)
3. Divides by node capacity to determine nodes needed
4. Subtracts in-flight scale-ups (nodes already starting)
5. Caps at available standby nodes
6. Starts required number of standby nodes

### In-Flight Tracking

To prevent duplicate scale-ups, Stratos tracks "starting" nodes:

- Nodes marked with `stratos.sh/scale-up-started` annotation
- TTL: 60 seconds
- Nodes are considered "starting" until they become Ready or TTL expires

## Scale-Down Configuration

### Parameters

```yaml
spec:
  scaleDown:
    enabled: true        # Enable automatic scale-down
    emptyNodeTTL: 5m     # Wait time before scaling down empty node
    drainTimeout: 5m     # Max time to drain pods
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `enabled` | `true` | Enable/disable automatic scale-down |
| `emptyNodeTTL` | `5m` | How long a node must be empty before scale-down |
| `drainTimeout` | `5m` | Maximum time to wait for node drain |

### How Scale-Down Works

1. Controller identifies nodes with no scheduled pods (excluding DaemonSets)
2. Marks empty nodes with `scale-down-candidate-since` annotation
3. After `emptyNodeTTL` elapses, node becomes scale-down candidate
4. Node is cordoned and drained (respecting PDBs)
5. After drain completes (or timeout), instance is stopped
6. Node transitions to standby state

### Tuning Scale-Down Timing

The `emptyNodeTTL` controls how quickly empty nodes return to standby. Since Stratos only scales down nodes with no scheduled pods, this is purely a cost/churn trade-off:

- **Shorter TTL** (e.g., `2m`): Faster return to standby, saves compute cost, but more start/stop cycles if demand fluctuates
- **Longer TTL** (e.g., `15m`): Nodes stay running longer after becoming empty, reduces churn for bursty workloads

```yaml
spec:
  scaleDown:
    emptyNodeTTL: 10m    # Wait 10 minutes before returning empty node to standby
```

### Disabling Scale-Down

To disable automatic scale-down entirely:

```yaml
spec:
  scaleDown:
    enabled: false
```

:::warning
With scale-down disabled, nodes will run until `maxNodeRuntime` is reached or the pool is deleted.
:::

## Node Recycling

### Max Node Runtime

Automatically recycle nodes after a specified duration:

```yaml
spec:
  maxNodeRuntime: 24h
```

Use cases:
- Apply AMI updates
- Clear memory leaks
- Refresh credentials
- Ensure security patches

:::note
Set to `0` or omit to disable node recycling.
:::

## Pre-Warm Configuration

### Parameters

```yaml
spec:
  preWarm:
    timeout: 15m           # Max time for warmup
    timeoutAction: terminate  # Action on timeout
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `timeout` | `10m` | Maximum time for instance to self-stop |
| `timeoutAction` | `stop` | Action on timeout: `stop` or `terminate` |

### Timeout Actions

| Action | Behavior |
|--------|----------|
| `stop` | Force stop instance, transition to standby (recoverable) |
| `terminate` | Terminate instance (non-recoverable, good for stuck instances) |

### Optimizing Warmup Time

To minimize warmup time and achieve the fastest scale-up:

1. **Use minimal user data scripts:**
   ```bash
   #!/bin/bash
   set -e
   /etc/eks/bootstrap.sh my-cluster \
     --kubelet-extra-args '--register-with-taints=...'
   until curl -sf http://localhost:10248/healthz; do sleep 5; done
   sleep 30
   poweroff
   ```

2. **Set appropriate timeout:**
   - Measure typical warmup time
   - Add buffer for variability
   - Use `terminate` action for faster recovery from stuck instances

### Image Pre-Pulling

Image pre-pulling is a key factor in Stratos's speed advantage over Karpenter. By pre-pulling images during warmup, Stratos eliminates one of the remaining bottlenecks at scale-up time.

**Automatic DaemonSet Image Pre-Pulling:**

Stratos automatically pre-pulls images for all DaemonSets that will run on nodes in the pool. This happens during the warmup phase, so when the node starts, DaemonSet pods can start immediately without waiting for image pulls.

**Manual Image Pre-Pulling:**

For application images that aren't DaemonSets, you can configure additional images to pre-pull:

```yaml
spec:
  preWarm:
    timeout: 15m
    imagesToPull:
      - docker.io/library/nginx:latest
      - your-registry/app:v1.2.3
      - gcr.io/your-project/worker:stable
```

This is particularly useful for:
- Large application images that take time to pull
- Images used by frequently-scheduled workloads
- Images from registries with rate limits

:::tip
Pre-pulling application images can reduce pod startup time from minutes to seconds, especially for large images or when using registries with rate limits.
:::

## Startup Taint Management

### WhenNetworkReady Mode

Stratos monitors network conditions and removes taints:

```yaml
spec:
  template:
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
    startupTaintRemoval: WhenNetworkReady
```

Supported CNIs:
- **EKS VPC CNI:** `NetworkingReady=True` condition
- **Cilium:** `NetworkUnavailable=False` with reason `CiliumIsUp`
- **Calico:** `NetworkUnavailable=False` with reason `CalicoIsUp`

Timeout: 2 minutes (then forcibly removed)

### External Mode

For CNIs that manage their own taints:

```yaml
spec:
  template:
    startupTaints:
      - key: node.cilium.io/agent-not-ready
        value: "true"
        effect: NoSchedule
    startupTaintRemoval: External
```

:::warning Important
The `startupTaints` field **must match** the `--register-with-taints` kubelet argument in your user data script.
:::

## Example Configurations

### High-Throughput Burst Workloads

For workloads with large, sudden bursts:

```yaml
spec:
  poolSize: 50
  minStandby: 10    # Large standby pool for instant bursts
  scaleDown:
    emptyNodeTTL: 2m   # Quick return to standby
    drainTimeout: 3m
```

### Cost-Sensitive Workloads

For cost optimization with acceptable scale-up latency:

```yaml
spec:
  poolSize: 20
  minStandby: 2     # Smaller standby pool
  scaleDown:
    emptyNodeTTL: 10m  # Slower scale-down to avoid churn
```

### CI/CD Runners

For CI/CD workloads with variable demand:

```yaml
spec:
  poolSize: 30
  minStandby: 5
  maxNodeRuntime: 12h  # Recycle frequently for fresh state
  scaleDown:
    emptyNodeTTL: 5m
```

### Long-Running Services

For stable services with occasional scaling:

```yaml
spec:
  poolSize: 15
  minStandby: 3
  maxNodeRuntime: 24h
  scaleDown:
    emptyNodeTTL: 15m   # Conservative scale-down
    drainTimeout: 10m   # Longer drain for graceful termination
```

## Monitoring Scaling

### Key Metrics

```promql
# Standby availability
stratos_nodepool_nodes_total{state="standby"}

# In-flight scale-ups
stratos_nodepool_starting_nodes

# Scale-up latency
histogram_quantile(0.95, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))

# Scale operations rate
rate(stratos_nodepool_scaleup_total[5m])
rate(stratos_nodepool_scaledown_total[5m])
```

### Alerts

```yaml
# Low standby warning
- alert: StratosLowStandby
  expr: stratos_nodepool_nodes_total{state="standby"} < 2
  for: 5m

# High scale-up latency
- alert: StratosSlowScaleUp
  expr: histogram_quantile(0.95, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m])) > 60
  for: 5m
```

## Next Steps

- [Monitoring](./monitoring.md) - Set up comprehensive monitoring
- [NodePool API](../reference/api/nodepool.md) - Complete API reference
