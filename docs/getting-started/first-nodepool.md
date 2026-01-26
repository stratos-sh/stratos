---
sidebar_position: 3
title: First NodePool
description: Create and test your first Stratos NodePool
---

# Creating Your First NodePool

This guide walks you through creating your first NodePool and testing scale-up.

## Prerequisites

Before creating a NodePool, ensure you have:

- Stratos controller running (see [Installation](./installation.md))
- AWS credentials configured (see [AWS Setup](../guides/aws-setup.md))
- EKS-optimized AMI ID for your region
- Subnet and security group IDs
- IAM instance profile for worker nodes

## Understanding the NodeClass Pattern

Stratos separates AWS-specific configuration from pool management:

- **AWSNodeClass**: Defines EC2 instance configuration (instance type, AMI, networking, IAM, user data)
- **NodePool**: Defines pool sizing, scaling behavior, and node template (labels, taints)

You create an AWSNodeClass first, then create a NodePool that references it.

```
+------------------+       references        +------------------+
|    NodePool      | ----------------------> |  AWSNodeClass    |
+------------------+                         +------------------+
| - poolSize: 10   |                         | - instanceType   |
| - minStandby: 3  |                         | - ami            |
| - labels, taints |                         | - subnetIds      |
+------------------+                         | - userData       |
                                             +------------------+
```

## Step 1: Prepare the User Data Script

The user data script runs during the warmup phase. It must:

1. Join the Kubernetes cluster
2. Register with startup taints
3. Wait for the node to be healthy
4. Self-stop (poweroff) when ready

:::warning Important
The startup taint in `--register-with-taints` **must match** the `startupTaints` field in the NodePool spec. Mismatched taints will cause scheduling issues.
:::

Example for EKS:

```bash title="user-data.sh"
#!/bin/bash
set -e

# Join the EKS cluster with a startup taint
# The taint prevents pod scheduling until CNI is ready
/etc/eks/bootstrap.sh my-cluster \
  --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'

# Wait for kubelet to be healthy
until curl -sf http://localhost:10248/healthz >/dev/null 2>&1; do
  sleep 5
done

# Give time for node to fully register
sleep 30

# Note: Stratos automatically pre-pulls DaemonSet images during warmup.
# You can configure additional images via preWarm.imagesToPull in the NodePool spec.

# Signal warmup complete by stopping the instance
poweroff
```

:::tip Using Bottlerocket?
Bottlerocket uses TOML configuration and doesn't support shell scripts. Use **ControllerStop** completion mode instead:

```yaml
preWarm:
  completionMode: ControllerStop
```

In this mode, Stratos stops the instance when the node becomes Ready, eliminating the need for a `poweroff` script. See [Bottlerocket Setup](../guides/bottlerocket.md) for details.
:::

## Step 2: Create the AWSNodeClass

Create a file named `awsnodeclass-workers.yaml`:

```yaml title="awsnodeclass-workers.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: workers
spec:
  region: us-east-1
  instanceType: m5.large
  ami: ami-0123456789abcdef0  # Your EKS-optimized AMI
  subnetIds:
    - subnet-12345678
    - subnet-87654321
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role

  # User data script that joins cluster and self-stops
  userData: |
    #!/bin/bash
    set -e
    /etc/eks/bootstrap.sh my-cluster \
      --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
    until curl -sf http://localhost:10248/healthz >/dev/null 2>&1; do sleep 5; done
    sleep 30
    poweroff

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 50
      volumeType: gp3
      encrypted: true

  tags:
    Environment: production
    ManagedBy: stratos
```

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

    # Labels applied to managed nodes
    labels:
      stratos.sh/pool: workers
      node-role.kubernetes.io/worker: ""

    # Startup taints - MUST match --register-with-taints in userData
    # Stratos removes these when the CNI is ready
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
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

Create a deployment that requests resources:

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
      containers:
        - name: nginx
          image: nginx:latest
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
```

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

Check the user data script output:

```bash
aws ec2 get-console-output --instance-id <instance-id>
```

Common issues:
- User data script failing
- Missing `poweroff` command
- Network issues preventing cluster join

### Scale-Up Not Triggering

Verify standby nodes are available:

```bash
kubectl get nodes -l stratos.sh/pool=workers,stratos.sh/state=standby
```

Check controller logs:

```bash
kubectl -n stratos-system logs deployment/stratos-controller
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
