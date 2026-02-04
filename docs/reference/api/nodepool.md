---
sidebar_position: 1
title: NodePool
description: NodePool Custom Resource Definition API reference
---

# NodePool API Reference

The NodePool resource is a cluster-scoped custom resource that defines a pool of pre-warmed instances.

## Resource Definition

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: <pool-name>
spec:
  poolSize: <int32>
  minStandby: <int32>
  reconciliationInterval: <duration>
  maxNodeRuntime: <duration>
  template:
    labels: <map[string]string>
    taints: <[]Taint>
    networkReadinessStrategy: <string>
    nodeClassRef:
      kind: <string>
      name: <string>
  preWarm:
    timeout: <duration>
    timeoutAction: <string>
    completionMode: <string>
  scaleUp:
    defaultPodResources: <ResourceRequirements>
  scaleDown:
    enabled: <bool>
    emptyNodeTTL: <duration>
    drainTimeout: <duration>
status:
  conditions: <[]Condition>
  observedGeneration: <int64>
  warmup: <int32>
  standby: <int32>
  running: <int32>
  total: <int32>
  lastReconcileTime: <Time>
```

## Spec Fields

### Core Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `poolSize` | int32 | Yes | - | Maximum total nodes in the pool (standby + running, excluding warmup). Range: 1-1000. |
| `minStandby` | int32 | Yes | - | Minimum number of nodes to maintain in standby (stopped) state. Must be ≤ poolSize. |
| `reconciliationInterval` | duration | No | `30s` | How often to run the maintenance reconciliation loop. |
| `maxNodeRuntime` | duration | No | disabled | Maximum time a node can run before being recycled. Set to 0 or omit to disable. |

### Template

The `template` field defines the configuration for nodes in this pool.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `nodeClassRef` | NodeClassRef | Yes | - | Reference to the cloud-specific NodeClass (e.g., AWSNodeClass) containing instance configuration. |
| `labels` | map[string]string | No | - | Labels to apply to managed nodes. The `stratos.sh/pool` label is automatically added. |
| `taints` | []Taint | No | - | Permanent taints for workload isolation. These persist throughout the node lifecycle. |
| `networkReadinessStrategy` | string | No | `Taint` | How network readiness is managed: `Taint` (auto-manage `stratos.sh/not-ready` taint) or `None` (no taint applied). |

#### NodeClassRef

The `nodeClassRef` field references a cloud-specific NodeClass resource that defines instance configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | NodeClass kind. Currently only `AWSNodeClass` is supported. |
| `name` | string | Yes | Name of the NodeClass resource to reference. |

```yaml
template:
  nodeClassRef:
    kind: AWSNodeClass
    name: standard-nodes
```

:::note
The referenced NodeClass must exist before creating the NodePool. If the NodeClass is not found, the NodePool will be marked as degraded with reason `NodeClassNotFound`.
:::

#### Network Readiness Strategy

| Strategy | Description |
|----------|-------------|
| `Taint` (default) | Stratos automatically applies and manages the `stratos.sh/not-ready=true:NoSchedule` taint. The taint is removed when the CNI reports readiness. Supports EKS VPC CNI, Cilium, and Calico. |
| `None` | No network readiness taint is applied. Use when the CNI manages its own readiness or taint gating is not needed. |

### PreWarm Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `timeout` | duration | No | `10m` | Maximum time to wait for warmup to complete. In SelfStop mode, how long to wait for instance to self-stop. In ControllerStop mode, how long to wait for node to become Ready. |
| `timeoutAction` | string | No | `stop` | Action when warmup times out: `stop` (force stop) or `terminate` (terminate instance). |
| `completionMode` | string | No | `SelfStop` | How warmup completes: `SelfStop` or `ControllerStop`. |

#### Warmup Completion Modes

| Mode | Description |
|------|-------------|
| `SelfStop` | Instance self-stops via userdata script after joining the cluster (default, existing behavior). |
| `ControllerStop` | Stratos stops the instance when the node becomes Ready. Use for OS images that cannot run shutdown scripts (e.g., Bottlerocket). |

### ScaleUp Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `defaultPodResources` | ResourceRequirements | No | - | Default resource requests for pods without explicit requests. Used in scale-up calculations. |

### ScaleDown Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `true` | Whether automatic scale-down is enabled. |
| `emptyNodeTTL` | duration | No | `5m` | How long a node must be empty before scale-down. |
| `drainTimeout` | duration | No | `5m` | Maximum time to wait for node drain before force-stopping. |

## Status Fields

The NodePool status is updated by the controller:

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Current conditions (Ready, Degraded, etc.) |
| `observedGeneration` | int64 | Last observed spec generation |
| `warmup` | int32 | Count of nodes in warmup state |
| `standby` | int32 | Count of nodes in standby state |
| `running` | int32 | Count of nodes in running state |
| `total` | int32 | Total managed node count |
| `lastReconcileTime` | Time | When the pool was last reconciled |

### Conditions

| Condition | Description |
|-----------|-------------|
| `Ready` | Pool has at least minStandby nodes in standby state |
| `Degraded` | Pool cannot meet minStandby (validation errors, cloud issues, NodeClass not found) |
| `Reconciling` | Pool is being reconciled |
| `ScaleUpInProgress` | Scale-up operation in progress |
| `ScaleDownInProgress` | Scale-down operation in progress |

## Examples

### Minimal Configuration

First, create an AWSNodeClass:

```yaml title="awsnodeclass.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: standard-nodes
spec:
  bootstrapTemplate: AL2023  # Stratos generates userData automatically
  instanceType: m5.large
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: node-role
```

Then create the NodePool referencing it:

```yaml title="nodepool-minimal.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 5
  minStandby: 2
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: standard-nodes
    labels:
      stratos.sh/pool: workers
```

Target pods to this pool:

```yaml title="deployment.yaml"
spec:
  template:
    spec:
      nodeSelector:
        stratos.sh/pool: workers
```

### Full Configuration

```yaml title="awsnodeclass-full.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: production-nodes
spec:
  bootstrapTemplate: AL2023
  region: us-east-1
  instanceType: m5.large
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: node-role
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 100
      volumeType: gp3
      encrypted: true
      iops: 3000
      throughput: 125
  tags:
    Environment: production
    Team: platform
    CostCenter: engineering
```

```yaml title="nodepool-full.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 20
  minStandby: 5
  reconciliationInterval: 30s
  maxNodeRuntime: 24h

  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: production-nodes
    labels:
      stratos.sh/pool: workers
      node-role.kubernetes.io/worker: ""
      environment: production
    taints:
      - key: dedicated
        value: workers
        effect: NoSchedule
  preWarm:
    timeout: 15m
    timeoutAction: terminate

  scaleUp:
    defaultPodResources:
      requests:
        cpu: "500m"
        memory: "1Gi"

  scaleDown:
    enabled: true
    emptyNodeTTL: 5m
    drainTimeout: 5m
```

### Cilium CNI Configuration

```yaml title="nodepool-cilium.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: cilium-workers
spec:
  poolSize: 10
  minStandby: 3
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: standard-nodes
    labels:
      stratos.sh/pool: cilium-workers
      cni: cilium
    # Stratos natively detects Cilium readiness via NetworkUnavailable condition
    # Use 'None' if you want Cilium to manage its own readiness taint instead
    # networkReadinessStrategy: None
```

Target pods to Cilium nodes:

```yaml
nodeSelector:
  stratos.sh/pool: cilium-workers
```

### Bottlerocket with ControllerStop Mode

Bottlerocket uses TOML configuration. Use `bootstrapTemplate: Bottlerocket` and `ControllerStop` mode.

```yaml title="awsnodeclass-bottlerocket.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: bottlerocket-nodes
spec:
  bootstrapTemplate: Bottlerocket  # Stratos generates TOML config
  instanceType: m5.large
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: node-role
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 8
      volumeType: gp3
      encrypted: true
    - deviceName: /dev/xvdb  # Bottlerocket data volume
      volumeSize: 20
      volumeType: gp3
      encrypted: true
```

```yaml title="nodepool-bottlerocket.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: bottlerocket-workers
spec:
  poolSize: 10
  minStandby: 3
  preWarm:
    # ControllerStop: Stratos stops instance when node is Ready
    completionMode: ControllerStop
    timeout: 10m
    timeoutAction: stop
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: bottlerocket-nodes
    labels:
      stratos.sh/pool: bottlerocket-workers
```

Target pods to Bottlerocket nodes:

```yaml
nodeSelector:
  stratos.sh/pool: bottlerocket-workers
```

### Multiple NodePools Sharing an AWSNodeClass

One AWSNodeClass can be shared by multiple NodePools with different scaling configurations:

```yaml title="shared-awsnodeclass.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: shared-ec2-config
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.large
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: node-role
---
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers-low
spec:
  poolSize: 5
  minStandby: 1
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: shared-ec2-config
    labels:
      stratos.sh/pool: workers-low
      tier: low-priority
---
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers-high
spec:
  poolSize: 20
  minStandby: 5
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: shared-ec2-config
    labels:
      stratos.sh/pool: workers-high
      tier: high-priority
```

Target pods to specific pools:

```yaml
# Low priority workloads
nodeSelector:
  stratos.sh/pool: workers-low

# High priority workloads
nodeSelector:
  stratos.sh/pool: workers-high
```

## Kubectl Commands

```bash
# List all NodePools
kubectl get nodepools

# Short name
kubectl get np

# Get detailed status
kubectl describe nodepool workers

# Get YAML output
kubectl get nodepool workers -o yaml

# Watch NodePool status
kubectl get nodepools -w

# See which AWSNodeClass each NodePool references
kubectl get nodepools -o custom-columns='NAME:.metadata.name,NODECLASS:.spec.template.nodeClassRef.name'
```

## Next Steps

- [AWSNodeClass Reference](./awsnodeclass.md) - AWS instance configuration
- [CLI Reference](../cli.md) - Controller flags reference
- [Labels and Annotations](../labels-annotations.md) - Labels and tags reference
