---
sidebar_position: 3
title: Labels and Annotations
description: Reference for Stratos labels, annotations, and cloud tags
---

# Labels and Annotations

Stratos uses labels, annotations, and cloud tags to track node state and ownership. This document provides a complete reference.

## Kubernetes Node Labels

Labels set by Stratos on managed Kubernetes nodes:

| Label | Description | Example Values |
|-------|-------------|----------------|
| `stratos.sh/pool` | NodePool name managing this node | `workers`, `ci-runners` |
| `stratos.sh/state` | Current Stratos state | `warmup`, `standby`, `running`, `terminating` |
| `stratos.sh/instance-id` | Cloud instance ID | `i-0123456789abcdef0` |
| `stratos.sh/state-since` | Timestamp when state changed | `2024-01-15T10:30:00Z` |
| `stratos.sh/capacity-type` | Instance capacity type | `on-demand`, `spot` |

### Querying by Labels

```bash
# Get all nodes in a pool
kubectl get nodes -l stratos.sh/pool=workers

# Get nodes by state
kubectl get nodes -l stratos.sh/pool=workers,stratos.sh/state=standby

# Get running nodes
kubectl get nodes -l stratos.sh/pool=workers,stratos.sh/state=running

# Custom columns output
kubectl get nodes -l stratos.sh/pool=workers \
  -o custom-columns='NAME:.metadata.name,STATE:.metadata.labels.stratos\.sh/state,INSTANCE:.metadata.labels.stratos\.sh/instance-id'

# Get Spot nodes in a pool
kubectl get nodes -l stratos.sh/pool=workers,stratos.sh/capacity-type=spot

# Get On-Demand nodes in a pool
kubectl get nodes -l stratos.sh/pool=workers,stratos.sh/capacity-type=on-demand

# Show capacity type for all pool nodes
kubectl get nodes -l stratos.sh/pool=workers \
  -o custom-columns='NAME:.metadata.name,STATE:.metadata.labels.stratos\.sh/state,CAPACITY:.metadata.labels.stratos\.sh/capacity-type'
```

## Kubernetes Node Annotations

Annotations set by Stratos on managed Kubernetes nodes:

| Annotation | Description | Example Values |
|------------|-------------|----------------|
| `stratos.sh/warmup-completed` | When warmup finished | `2024-01-15T10:25:00Z` |
| `stratos.sh/last-started` | When node was last started | `2024-01-15T10:30:00Z` |
| `stratos.sh/scale-down-candidate-since` | When node became empty | `2024-01-15T11:00:00Z` |
| `stratos.sh/scale-up-started` | When scale-up was triggered | `2024-01-15T10:30:00Z` |
| `stratos.sh/spot-replacement-started` | When spot replacement was initiated for this On-Demand node | `2024-01-15T11:00:00Z` |
| `stratos.sh/spot-replacing-node` | Name of the On-Demand node being replaced by this Spot node | `ip-10-0-1-50.ec2.internal` |

### Scale-Up Tracking

The `stratos.sh/scale-up-started` annotation is used for in-flight tracking to prevent duplicate scale-ups:

- **TTL:** 60 seconds
- **Purpose:** Track nodes that have been triggered for scale-up but are not yet Ready
- **Cleared:** When node becomes Ready or TTL expires

### Scale-Down Tracking

The `stratos.sh/scale-down-candidate-since` annotation marks when a node became empty:

- **Set:** When node has no scheduled pods (excluding DaemonSets)
- **Cleared:** When pod is scheduled on the node
- **Used for:** Determining when `emptyNodeTTL` has elapsed

## Cloud Instance Tags

Tags set by Stratos on cloud instances (e.g., EC2):

| Tag | Description | Example Values |
|-----|-------------|----------------|
| `managed-by` | Identifies Stratos-managed instances | `stratos` |
| `stratos.sh/pool` | NodePool name | `workers` |
| `stratos.sh/cluster` | Kubernetes cluster name | `production` |
| `stratos.sh/state` | Current Stratos state | `warmup`, `standby`, `running`, `terminating` |
| `stratos.sh/capacity-type` | Instance capacity type | `on-demand`, `spot` |
| `stratos.sh/replacing-node` | Name of the On-Demand node this Spot instance is replacing (Spot instances only) | `ip-10-0-1-50.ec2.internal` |

### Tag Usage

These tags are used for:

1. **Discovery:** Finding managed instances on controller startup
2. **Filtering:** Listing instances by pool
3. **Auditing:** Cost allocation and resource tracking
4. **Security:** Scoping IAM policies to Stratos-managed resources

### AWS CLI Queries

```bash
# List all Stratos-managed instances
aws ec2 describe-instances \
  --filters "Name=tag:managed-by,Values=stratos" \
  --query 'Reservations[].Instances[].{ID:InstanceId,State:State.Name,Pool:Tags[?Key==`stratos.sh/pool`].Value|[0]}'

# List instances in a specific pool
aws ec2 describe-instances \
  --filters "Name=tag:stratos.sh/pool,Values=workers" \
  --query 'Reservations[].Instances[].{ID:InstanceId,State:State.Name}'

# List standby instances
aws ec2 describe-instances \
  --filters "Name=tag:stratos.sh/state,Values=standby" \
  --query 'Reservations[].Instances[].InstanceId'

# List Spot instances in a pool
aws ec2 describe-instances \
  --filters "Name=tag:stratos.sh/pool,Values=workers" "Name=tag:stratos.sh/capacity-type,Values=spot" \
  --query 'Reservations[].Instances[].{ID:InstanceId,State:State.Name,Replacing:Tags[?Key==`stratos.sh/replacing-node`].Value|[0]}'

# List On-Demand instances in a pool
aws ec2 describe-instances \
  --filters "Name=tag:stratos.sh/pool,Values=workers" "Name=tag:stratos.sh/capacity-type,Values=on-demand" \
  --query 'Reservations[].Instances[].{ID:InstanceId,State:State.Name}'
```

## User-Defined Labels

Labels specified in `spec.template.labels` are applied to managed nodes:

```yaml
spec:
  template:
    labels:
      stratos.sh/pool: workers       # Automatically added
      node-role.kubernetes.io/worker: ""
      environment: production
      team: platform
```

:::note
The `stratos.sh/pool` label is automatically added and matches the NodePool name. You don't need to specify it explicitly, but if you do, it must match the NodePool name.
:::

## User-Defined Tags

Tags specified in `spec.template.cloudProvider.aws.tags` are applied to instances:

```yaml
spec:
  template:
    cloudProvider:
      aws:
        tags:
          Environment: production
          Team: platform
          CostCenter: engineering
```

These are merged with Stratos management tags. User tags cannot override management tags.

## Taints

### Permanent Taints

Taints specified in `spec.template.taints` persist throughout the node lifecycle:

```yaml
spec:
  template:
    taints:
      - key: dedicated
        value: workers
        effect: NoSchedule
```

### Network Readiness Taint

When `networkReadinessStrategy` is `Taint` (the default), Stratos automatically manages the `stratos.sh/not-ready=true:NoSchedule` taint. This taint blocks scheduling until the CNI is ready.

| Taint Key | Value | Effect | Managed By |
|-----------|-------|--------|------------|
| `stratos.sh/not-ready` | `true` | NoSchedule | Stratos (automatic) |

:::note Automatic Taint Registration
Stratos handles taint registration internally. You do not need to configure `--register-with-taints` or specify taint keys in your NodePool spec.
:::

### Standby Taint

Stratos applies a standby taint to cordoned nodes:

| Taint Key | Value | Effect |
|-----------|-------|--------|
| `stratos.sh/standby` | - | NoExecute |

This taint ensures pods are evicted from standby nodes.

## Label Selectors

### Pod Matching

Stratos uses labels to determine which pools can satisfy pending pods:

1. Pods must tolerate all permanent taints on the pool
2. Pod node selectors must match pool labels
3. Pod affinity/anti-affinity rules are evaluated

### Node Selection

When scaling up, Stratos selects standby nodes matching:

- `stratos.sh/pool=<pool-name>`
- `stratos.sh/state=standby`

## Prometheus Label Cardinality

Metrics use these labels:

| Metric Label | Source | Values |
|--------------|--------|--------|
| `pool` | NodePool name | One per NodePool |
| `state` | Node state | `warmup`, `standby`, `running`, `terminating` |
| `provider` | Cloud provider | `aws`, `fake` |
| `operation` | Cloud API operation | `launch`, `start`, `stop`, `terminate`, `describe`, `LaunchSpotInstance` |
| `status` | Operation result | `success`, `error` |
| `trigger` | Taint removal trigger | `network_ready`, `timeout` |
| `result` | Taint removal result | `success`, `error` |
| `reason` | Warmup failure reason | `timeout`, `error` |
| `type` | Error type | Varies by error |

### Spot Replacement Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `stratos_nodepool_spot_replacements_total` | Counter | Total successful On-Demand to Spot replacements |
| `stratos_nodepool_spot_interruptions_total` | Counter | Total Spot instance interruption events |
| `stratos_nodepool_spot_replacement_duration_seconds` | Histogram | Time from spot replacement start to migration completion |
| `stratos_nodepool_spot_fallbacks_total` | Counter | Total Spot to On-Demand fallback events (triggered by interruptions) |

## Next Steps

- [NodePool API](./api/nodepool.md) - Complete API reference
- [Architecture](../concepts/architecture.md) - System architecture
