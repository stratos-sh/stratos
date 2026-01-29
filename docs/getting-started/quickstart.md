---
sidebar_position: 3
title: Quickstart
description: Create and test your first Stratos NodePool
---

# Quickstart

This guide walks you through creating your first NodePool and testing scale-up.

## Prerequisites

Before creating a NodePool, ensure you have:

- Stratos installed via Helm (see [Installation](./installation.md))
- AWS credentials configured (see [AWS Setup](../guides/aws-setup.md))
- EKS-optimized AMI ID for your region
- Subnet and security group IDs
- IAM instance profile for worker nodes

## Understanding the NodeClass Pattern

Stratos separates AWS-specific configuration from pool management:

- **AWSNodeClass**: Defines EC2 instance configuration (instance type, AMI family, networking, IAM)
- **NodePool**: Defines pool sizing, scaling behavior, and node template (labels, taints)

You create an AWSNodeClass first, then create a NodePool that references it.

```
+------------------+       references        +------------------+
|    NodePool      | ----------------------> |  AWSNodeClass    |
+------------------+                         +------------------+
| - poolSize: 10   |                         | - instanceType   |
| - minStandby: 3  |                         | - bootstrapTemplate |
| - labels, taints |                         | - subnetSelector |
+------------------+                         | - role           |
                                             +------------------+
```

### Simplified Bootstrap

Stratos automatically generates the node bootstrap script (userData) based on the `bootstrapTemplate` field:

| Template | AMI Family | Bootstrap Format |
|----------|-----------|------------------|
| `AL2023` | Amazon Linux 2023 | nodeadm MIME multipart |
| `AL2` | Amazon Linux 2 | bootstrap.sh MIME multipart |
| `Bottlerocket` | Bottlerocket | TOML configuration |

The controller uses cluster configuration (endpoint, CA, CIDR) from Helm values to generate the complete bootstrap script. You no longer need to write custom userData scripts.

## Step 1: Configure Helm Values for Bootstrap

Stratos automatically generates the node bootstrap script using cluster configuration from Helm values. Make sure your Helm installation includes the required cluster settings:

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set cluster.name=my-cluster \
  --set cluster.apiServerEndpoint=https://ABCDEF.gr7.us-east-1.eks.amazonaws.com \
  --set cluster.certificateAuthority=LS0tLS1CRUdJTi... \
  --set cluster.cidr=172.20.0.0/16
```

:::tip Getting Cluster Configuration
Retrieve your EKS cluster's configuration:

```bash
# Get cluster name
CLUSTER_NAME=my-cluster

# Get API server endpoint
aws eks describe-cluster --name $CLUSTER_NAME \
  --query "cluster.endpoint" --output text

# Get CA certificate (already base64 encoded)
aws eks describe-cluster --name $CLUSTER_NAME \
  --query "cluster.certificateAuthority.data" --output text

# Get service CIDR
aws eks describe-cluster --name $CLUSTER_NAME \
  --query "cluster.kubernetesNetworkConfig.serviceIpv4Cidr" --output text
```
:::

:::note Automatic Bootstrap Generation
With the cluster configuration provided, Stratos generates the complete userData script for each AMI family:
- **AL2023**: Uses nodeadm MIME multipart format
- **AL2**: Uses bootstrap.sh MIME multipart format
- **Bottlerocket**: Uses TOML configuration

You no longer need to write custom bootstrap scripts with `poweroff` commands.
:::

## Step 2: Create the AWSNodeClass

Create a file named `awsnodeclass-workers.yaml`:

```yaml title="awsnodeclass-workers.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: workers
spec:
  # Bootstrap template determines the userData format
  # Stratos generates the bootstrap script automatically
  bootstrapTemplate: AL2023

  region: us-east-1
  instanceType: m5.large

  # Use selectors for dynamic resource discovery (recommended)
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  # Stratos manages the instance profile automatically
  role: eks-node-role

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 50
      volumeType: gp3
      encrypted: true

  tags:
    Environment: production
    ManagedBy: stratos
```

:::note Static IDs Alternative
If you prefer static IDs instead of selectors:

```yaml
subnetIds:
  - subnet-12345678
  - subnet-87654321
securityGroupIds:
  - sg-12345678
iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role
```
:::

Apply the AWSNodeClass:

```bash
kubectl apply -f awsnodeclass-workers.yaml
```

Verify it was created:

```bash
kubectl get awsnodeclasses
```

## Step 3: Create the NodePool

Create a file named `nodepool-workers.yaml`:

```yaml title="nodepool-workers.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  # Maximum nodes in the pool (standby + running)
  poolSize: 10

  # Minimum standby nodes to maintain
  minStandby: 3

  # Reconciliation interval
  reconciliationInterval: 30s

  template:
    # Reference to AWSNodeClass for EC2 configuration
    nodeClassRef:
      kind: AWSNodeClass
      name: workers

    # Labels applied to managed nodes - use these with nodeSelector to target the pool
    labels:
      stratos.sh/pool: workers
      node-role.kubernetes.io/worker: ""
      workload-type: general       # Custom label for pod targeting

    # Startup taints - Stratos removes these when the CNI is ready
    startupTaints:
      - key: stratos.sh/not-ready
        value: "true"
        effect: NoSchedule

    # How startup taints are removed
    startupTaintRemoval: WhenNetworkReady

  # Pre-warm configuration
  preWarm:
    timeout: 15m
    timeoutAction: terminate

  # Scale-down configuration
  scaleDown:
    enabled: true
    emptyNodeTTL: 5m
    drainTimeout: 5m
```

Apply the NodePool:

```bash
kubectl apply -f nodepool-workers.yaml
```

:::warning Order Matters
The AWSNodeClass must exist before creating the NodePool. If the NodePool references a non-existent AWSNodeClass, it will be marked as Degraded with reason `NodeClassNotFound`.
:::

## Step 4: Verify the NodePool

Check the NodePool status:

```bash
kubectl get nodepools
```

Expected output:

```
NAME      POOLSIZE   MINSTANDBY   STANDBY   RUNNING   READY   AGE
workers   10         3            0         0         False   10s
```

Watch nodes being created:

```bash
kubectl get nodes -l stratos.sh/pool=workers -w
```

View detailed status:

```bash
kubectl describe nodepool workers
```

Check the AWSNodeClass status:

```bash
kubectl describe awsnodeclass workers
```

:::note
Initial warmup takes several minutes. Nodes will transition through `warmup` to `standby` state.
:::

After warmup completes (3-5 minutes), you should see:

```
NAME      POOLSIZE   MINSTANDBY   STANDBY   RUNNING   READY   AGE
workers   10         3            3         0         True    5m
```

## Step 5: Test Scale-Up

Create a deployment that targets the NodePool using `nodeSelector`:

```yaml title="test-workload.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-workload
spec:
  replicas: 5
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      # Target the Stratos NodePool by label
      nodeSelector:
        stratos.sh/pool: workers
      containers:
        - name: nginx
          image: nginx:latest
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
```

:::tip Targeting Strategies
Use one of these approaches to schedule pods on Stratos-managed nodes:

**nodeSelector** (simplest):
```yaml
nodeSelector:
  stratos.sh/pool: workers
```

**nodeAffinity** (more flexible):
```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: stratos.sh/pool
              operator: In
              values: [workers, ci-runners]
```
:::

```bash
kubectl apply -f test-workload.yaml
```

When pods become pending due to insufficient capacity, Stratos will:

1. Detect the unschedulable pods
2. Start standby nodes (pre-warmed instances resume in ~15-20 seconds)
3. Nodes become Ready in ~20-25 seconds total
4. Pods are scheduled on the newly available nodes

This is roughly half the time of Karpenter (~40-50 seconds) because Stratos nodes have already completed boot, cluster join, CNI initialization, and DaemonSet image pulls during the warmup phase.

Monitor the process:

```bash
# Watch pods
kubectl get pods -w

# Watch nodes
kubectl get nodes -l stratos.sh/pool=workers -w

# Check NodePool events
kubectl describe nodepool workers
```

## Step 6: Test Scale-Down

Delete the test workload:

```bash
kubectl delete deployment test-workload
```

After `emptyNodeTTL` (5 minutes by default), Stratos will:

1. Detect empty nodes
2. Drain and cordon the nodes
3. Stop the instances
4. Return them to standby state

## Troubleshooting

### NodePool Shows Degraded with NodeClassNotFound

The AWSNodeClass doesn't exist or has a different name:

```bash
# Check if AWSNodeClass exists
kubectl get awsnodeclasses

# Check the NodePool's nodeClassRef
kubectl get nodepool workers -o jsonpath='{.spec.template.nodeClassRef}'
```

### Nodes Stuck in Warmup

Check the instance console output:

```bash
aws ec2 get-console-output --instance-id <instance-id>
```

Common issues:
- Network issues preventing cluster join
- Incorrect cluster configuration in Helm values
- IAM role missing required permissions

### Scale-Up Not Triggering

Verify standby nodes are available:

```bash
kubectl get nodes -l stratos.sh/pool=workers,stratos.sh/state=standby
```

Check controller logs:

```bash
kubectl -n stratos-system logs deployment/stratos
```

### AWSNodeClass Status Shows Issues

Check the AWSNodeClass conditions:

```bash
kubectl describe awsnodeclass workers
```

Look for:
- `Valid` condition: Is the spec valid?
- `InUse` condition: Is it referenced by NodePools?

## Next Steps

- [Node Lifecycle](../concepts/node-lifecycle.md) - Understand node states
- [AWSNodeClass Reference](../reference/api/awsnodeclass.md) - Complete AWSNodeClass API
- [Scaling Policies](../guides/scaling-policies.md) - Configure scaling behavior
- [Monitoring](../guides/monitoring.md) - Set up monitoring and alerts
