---
sidebar_position: 2
title: Bottlerocket Setup
description: Configure Stratos with Bottlerocket OS using ControllerStop mode
---

# Bottlerocket Setup

This guide explains how to use Stratos with [Bottlerocket](https://aws.amazon.com/bottlerocket/), Amazon's purpose-built Linux-based operating system for container workloads.

## Why Bottlerocket Needs ControllerStop Mode

Bottlerocket is designed for security and minimal attack surface. Unlike Amazon Linux or Ubuntu, Bottlerocket:

- Uses **TOML configuration** instead of shell scripts
- Has a **read-only root filesystem**
- Does not support arbitrary user data scripts
- Cannot execute `poweroff` from user data

Standard Stratos warmup relies on user data scripts that call `poweroff` after the node joins the cluster (SelfStop mode). This approach doesn't work with Bottlerocket.

### The Solution: ControllerStop Mode

Stratos 1.0 introduces **ControllerStop** warmup completion mode, which allows the controller to manage the warmup-to-standby transition:

1. Instance launches and boots Bottlerocket
2. Bottlerocket joins the cluster using TOML configuration
3. Stratos monitors the node until it becomes Ready
4. Stratos stops the instance when the node is fully ready
5. Node transitions to standby state

This eliminates the need for shutdown scripts entirely.

## Configuration

### Step 1: Create the AWSNodeClass

Create an AWSNodeClass with Bottlerocket configuration:

```yaml title="awsnodeclass-bottlerocket.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: bottlerocket-nodes
spec:
  region: us-east-1
  instanceType: m5.large

  # Bottlerocket EKS-optimized AMI
  # aws ssm get-parameter --name /aws/service/bottlerocket/aws-k8s-1.34/x86_64/latest/image_id
  ami: ami-0123456789abcdef0

  subnetIds:
    - subnet-12345678
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/node-role

  # Bottlerocket has two volumes: root (small) and data
  blockDeviceMappings:
    - deviceName: /dev/xvda  # Root volume (OS)
      volumeSize: 8
      volumeType: gp3
      encrypted: true
    - deviceName: /dev/xvdb  # Data volume (containers, images)
      volumeSize: 20
      volumeType: gp3
      encrypted: true

  tags:
    Environment: production
    OS: bottlerocket

  # Bottlerocket TOML configuration
  # No shutdown script - use ControllerStop mode in NodePool
  userData: |
    [settings.kubernetes]
    cluster-name = "my-cluster"
    api-server = "https://ABCDEF1234567890.gr7.us-east-1.eks.amazonaws.com"
    cluster-certificate = "LS0tLS1CRUdJTi..."

    [settings.kubernetes.node-taints]
    "node.eks.amazonaws.com/not-ready" = "true:NoSchedule"

    [settings.kubernetes.node-labels]
    "stratos.sh/pool" = "bottlerocket-workers"
```

### Step 2: Create the NodePool

Create a NodePool with ControllerStop mode:

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
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
    # Wait for CNI to be ready before considering warmup complete
    startupTaintRemoval: WhenNetworkReady
```

### Apply the Resources

```bash
# Create the AWSNodeClass first
kubectl apply -f awsnodeclass-bottlerocket.yaml

# Then create the NodePool
kubectl apply -f nodepool-bottlerocket.yaml
```

:::warning Order Matters
The AWSNodeClass must exist before creating the NodePool. If the NodePool references a non-existent AWSNodeClass, it will be marked as Degraded with reason `NodeClassNotFound`.
:::

### User Data (TOML Format)

Bottlerocket user data must be TOML. Here's a complete example:

```toml title="bottlerocket-userdata.toml"
[settings.kubernetes]
# Required: Cluster configuration
cluster-name = "my-cluster"
api-server = "https://ABCDEF1234567890.gr7.us-east-1.eks.amazonaws.com"
cluster-certificate = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURCVENDQWUyZ0F..."

# Optional: Custom kubelet settings for faster status updates
# [settings.kubernetes.kubelet]
# node-status-update-frequency = "4s"

# Register with startup taint to prevent scheduling until CNI is ready
[settings.kubernetes.node-taints]
"node.eks.amazonaws.com/not-ready" = "true:NoSchedule"

# Pool label for Stratos management
[settings.kubernetes.node-labels]
"stratos.sh/pool" = "bottlerocket-workers"

# Optional: Additional labels
# "node-role.kubernetes.io/worker" = ""
# "environment" = "production"
```

:::tip Getting Cluster Configuration
Retrieve your cluster's API server and certificate:

```bash
# Get API server endpoint
aws eks describe-cluster --name my-cluster \
  --query "cluster.endpoint" --output text

# Get cluster certificate (already base64 encoded)
aws eks describe-cluster --name my-cluster \
  --query "cluster.certificateAuthority.data" --output text
```
:::

## How ControllerStop Mode Works

```
  Instance Launch
        |
        v
  +------------+
  |  Warmup    |  Bottlerocket boots, joins cluster
  +-----+------+
        |
        | Node becomes Ready + CNI ready
        v
  +------------+
  | Controller |  Stratos detects Ready state
  | Stops      |  Calls StopInstance API
  +-----+------+
        |
        v
  +------------+
  |  Standby   |  Instance stopped, ready for scale-up
  +------------+
```

### Readiness Checks

Before stopping the instance, Stratos verifies:

1. **Node Ready condition**: The Kubernetes node has `Ready=True`
2. **Network Ready** (when `startupTaintRemoval: WhenNetworkReady`):
   - For EKS VPC CNI: `NetworkingReady=True` condition AND `aws-node` pod Ready
   - For Cilium: `NetworkUnavailable=False` with reason `CiliumIsUp`
   - For Calico: `NetworkUnavailable=False` with reason `CalicoIsUp`

This ensures the node is fully functional before being stopped for standby.

## EBS Volume Considerations

Bottlerocket has two volumes:

| Volume | Device | Purpose | Recommended Size |
|--------|--------|---------|-----------------|
| Root | `/dev/xvda` | OS (read-only) | 2-8 GiB |
| Data | `/dev/xvdb` | Container images, logs | 20+ GiB |

The root volume is minimal (read-only OS), while the data volume stores container images and runtime data.

```yaml
blockDeviceMappings:
  - deviceName: /dev/xvda
    volumeSize: 8      # Minimal root volume
    volumeType: gp3
    encrypted: true
  - deviceName: /dev/xvdb
    volumeSize: 20     # Data volume for images
    volumeType: gp3
    encrypted: true
```

:::note
Unlike traditional Linux AMIs, Bottlerocket doesn't require EBS pre-warming (`dd if=/dev/zero`). The minimal root filesystem means first-read latency has minimal impact.
:::

## Getting the Bottlerocket AMI

Query for the latest Bottlerocket EKS-optimized AMI:

```bash
# x86_64 architecture
aws ssm get-parameter \
  --name /aws/service/bottlerocket/aws-k8s-1.34/x86_64/latest/image_id \
  --query "Parameter.Value" --output text

# ARM64 (Graviton) architecture
aws ssm get-parameter \
  --name /aws/service/bottlerocket/aws-k8s-1.34/arm64/latest/image_id \
  --query "Parameter.Value" --output text
```

Replace `1.34` with your EKS cluster version.

## Metrics

ControllerStop mode is tracked in the `stratos_nodepool_warmup_duration_seconds` metric with a `mode` label:

```promql
# Warmup duration for ControllerStop mode
histogram_quantile(0.95,
  sum(rate(stratos_nodepool_warmup_duration_seconds_bucket{mode="controller_stop"}[5m])) by (le, pool)
)

# Compare warmup modes
sum by (mode) (
  increase(stratos_nodepool_warmup_duration_seconds_count[1h])
)
```

| Mode Label | Description |
|------------|-------------|
| `self_stop` | Traditional mode: instance self-stopped via user data script |
| `controller_stop` | ControllerStop mode: Stratos stopped the instance |
| `timeout` | Warmup timed out and was force-stopped |

## Troubleshooting

### Node Stuck in Warmup

If nodes remain in warmup state:

1. **Check node status**:
   ```bash
   kubectl get nodes -l stratos.sh/pool=bottlerocket-workers \
     -o custom-columns='NAME:.metadata.name,STATE:.metadata.labels.stratos\.sh/state,READY:.status.conditions[?(@.type=="Ready")].status'
   ```

2. **Verify CNI is ready** (for EKS):
   ```bash
   kubectl get pods -n kube-system -l k8s-app=aws-node \
     --field-selector spec.nodeName=<node-name>
   ```

3. **Check controller logs**:
   ```bash
   kubectl logs -n stratos-system deployment/stratos | grep "ControllerStop"
   ```

4. **Check if AWSNodeClass exists**:
   ```bash
   kubectl get awsnodeclasses
   kubectl describe awsnodeclass bottlerocket-nodes
   ```

### NodeClassNotFound Error

If the NodePool shows `NodeClassNotFound`:

```bash
# Verify the AWSNodeClass exists
kubectl get awsnodeclasses

# Check the NodePool's nodeClassRef
kubectl get nodepool bottlerocket-workers -o jsonpath='{.spec.template.nodeClassRef}'
```

Ensure the `name` in `nodeClassRef` exactly matches the AWSNodeClass name.

### User Data Not Applied

Bottlerocket user data must be valid TOML. Common issues:

- Missing quotes around values
- Invalid TOML syntax
- Wrong cluster certificate (must be base64-encoded)

Validate TOML syntax:
```bash
# Using a TOML validator
cat userdata.toml | tomlq .
```

Check Bottlerocket console output:
```bash
aws ec2 get-console-output --instance-id i-0123456789abcdef0
```

### Timeout Before Ready

If warmup times out before the node is ready:

1. Increase the timeout:
   ```yaml
   preWarm:
     completionMode: ControllerStop
     timeout: 15m  # Increase from default 10m
   ```

2. Check if the node is joining the cluster:
   ```bash
   kubectl get nodes --watch
   ```

3. Verify network connectivity from the subnet to the API server

## Comparison: SelfStop vs ControllerStop

| Aspect | SelfStop (Default) | ControllerStop |
|--------|-------------------|----------------|
| User data | Bash script with `poweroff` | TOML configuration only |
| OS support | Amazon Linux, Ubuntu | All OS including Bottlerocket |
| Warmup completion | Instance self-stops | Controller stops instance |
| Complexity | Script must be correct | No script needed |
| Visibility | Less observable | Controller logs stop action |

## Next Steps

- [AWSNodeClass Reference](../reference/api/awsnodeclass.md) - Complete AWSNodeClass API
- [Node Lifecycle](../concepts/node-lifecycle.md) - Understand warmup completion modes
- [Monitoring](./monitoring.md) - Monitor warmup metrics
- [NodePool API Reference](../reference/api/nodepool.md) - Full NodePool specification
