---
sidebar_position: 2
title: AWSNodeClass
description: AWSNodeClass Custom Resource Definition API reference
---

# AWSNodeClass API Reference

The AWSNodeClass resource is a cluster-scoped custom resource that defines AWS EC2 configuration for nodes managed by Stratos NodePools. This resource follows the Karpenter pattern of separating cloud-specific configuration from pool management, enabling reusability and independent schema evolution per cloud provider.

## Overview

AWSNodeClass decouples AWS-specific instance configuration from NodePool resources:

- **NodePool**: Defines pool sizing, scaling behavior, and node template (labels, taints)
- **AWSNodeClass**: Defines AWS EC2 configuration (instance type, AMI, networking, IAM)

This separation allows:
- Multiple NodePools to share the same EC2 configuration
- Independent updates to pool sizing vs. instance configuration
- Clean path to multi-cloud support (GCPNodeClass, AzureNodeClass)

## Resource Definition

```yaml
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: <nodeclass-name>
spec:
  region: <string>
  instanceType: <string>
  ami: <string>
  subnetIds: <[]string>
  securityGroupIds: <[]string>
  iamInstanceProfile: <string>
  userData: <string>
  blockDeviceMappings: <[]BlockDeviceMapping>
  tags: <map[string]string>
status:
  nodePoolCount: <int32>
  conditions: <[]Condition>
```

## Spec Fields

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `instanceType` | string | EC2 instance type (e.g., `m5.large`, `c5.xlarge`, `m8g.large`). |
| `ami` | string | Amazon Machine Image ID. Must match pattern `^ami-[a-z0-9]+$`. Use EKS-optimized AMIs for EKS clusters. |
| `subnetIds` | []string | List of subnet IDs for instance placement. Instances are distributed round-robin across subnets for AZ distribution. Minimum 1 required. |
| `securityGroupIds` | []string | List of security group IDs to attach to instances. Minimum 1 required. |
| `iamInstanceProfile` | string | IAM instance profile ARN or name. The profile must have permissions to join the Kubernetes cluster. |

### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `region` | string | Controller region | AWS region for launching instances (e.g., `us-east-1`). Defaults to the region where the controller is running. |
| `userData` | string | - | User data script. For AL2/AL2023, this is a shell script. For Bottlerocket, this is TOML configuration. Not base64 encoded - Stratos handles encoding. |
| `blockDeviceMappings` | []BlockDeviceMapping | - | EBS volume configuration. See Block Device Mapping below. |
| `tags` | map[string]string | - | Additional tags to apply to instances. Stratos management tags (`managed-by`, `stratos.sh/pool`, etc.) are added automatically. |

### Block Device Mapping

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `deviceName` | string | Yes | - | Device name (e.g., `/dev/xvda`, `/dev/xvdb`). |
| `volumeSize` | int32 | Yes | - | Volume size in GiB. Minimum: 1. |
| `volumeType` | string | Yes | - | EBS volume type: `gp3`, `gp2`, `io1`, `io2`. |
| `encrypted` | bool | No | `false` | Enable EBS encryption. |
| `iops` | int32 | No | - | IOPS for io1/io2 volumes, or provisioned IOPS for gp3. |
| `throughput` | int32 | No | - | Throughput in MB/s for gp3 volumes. |

## Status Fields

The AWSNodeClass status is updated by the controller:

| Field | Type | Description |
|-------|------|-------------|
| `nodePoolCount` | int32 | Number of NodePools currently referencing this AWSNodeClass. |
| `conditions` | []Condition | Current conditions (Valid, InUse). |

### Conditions

| Condition | Description |
|-----------|-------------|
| `Valid` | The AWSNodeClass spec is valid (AMI exists, networking configured correctly). |
| `InUse` | The AWSNodeClass is referenced by one or more NodePools. |

## Lifecycle Management

### Deletion Protection

AWSNodeClass uses a finalizer (`stratos.sh/in-use`) to prevent deletion while referenced by NodePools. You must delete all referencing NodePools before deleting an AWSNodeClass.

```bash
# This will fail if NodePools reference it
kubectl delete awsnodeclass my-nodeclass

# Check which NodePools reference it
kubectl get nodepools -o json | jq '.items[] | select(.spec.template.nodeClassRef.name == "my-nodeclass") | .metadata.name'

# Delete the NodePools first, then the AWSNodeClass
kubectl delete nodepool my-pool
kubectl delete awsnodeclass my-nodeclass
```

### Reference from NodePool

NodePools reference AWSNodeClass via `spec.template.nodeClassRef`:

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 3
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: standard-nodes  # References AWSNodeClass by name
    labels:
      stratos.sh/pool: workers
```

## Examples

### Amazon Linux 2023 (AL2023)

```yaml title="awsnodeclass-al2023.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: al2023-standard
spec:
  region: us-east-1
  instanceType: m8g.large  # ARM64 for AL2023 ARM64 AMI

  # AL2023 ARM64 AMI (EKS 1.34)
  # aws ssm get-parameter --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2023/arm64/standard/recommended/image_id
  ami: ami-00171c2155dff951e

  subnetIds:
    - subnet-12345678
    - subnet-87654321
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 20
      volumeType: gp3
      encrypted: true

  tags:
    Environment: production
    OS: al2023
    Architecture: arm64

  # AL2023 uses nodeadm with MIME multipart user data
  userData: |
    MIME-Version: 1.0
    Content-Type: multipart/mixed; boundary="BOUNDARY"

    --BOUNDARY
    Content-Type: application/node.eks.aws

    ---
    apiVersion: node.eks.aws/v1alpha1
    kind: NodeConfig
    spec:
      cluster:
        name: my-cluster
        apiServerEndpoint: https://ABCDEF.gr7.us-east-1.eks.amazonaws.com
        certificateAuthority: LS0tLS1CRUdJTi...
        cidr: 10.100.0.0/16
      kubelet:
        flags:
          - --register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule

    --BOUNDARY
    Content-Type: text/x-shellscript; charset="us-ascii"

    #!/bin/bash
    # Wait for kubelet, then poweroff for Stratos standby
    (
      until curl -sf http://localhost:10248/healthz; do sleep 5; done
      sleep 30
      poweroff
    ) &> /var/log/stratos-warmup.log &

    --BOUNDARY--
```

### Bottlerocket

```yaml title="awsnodeclass-bottlerocket.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: bottlerocket-standard
spec:
  region: us-east-1
  instanceType: m5.large

  # Bottlerocket x86_64 AMI (EKS 1.34)
  # aws ssm get-parameter --name /aws/service/bottlerocket/aws-k8s-1.34/x86_64/latest/image_id
  ami: ami-02471ee0da5a34d1e

  subnetIds:
    - subnet-12345678
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role

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
    api-server = "https://ABCDEF.gr7.us-east-1.eks.amazonaws.com"
    cluster-certificate = "LS0tLS1CRUdJTi..."

    [settings.kubernetes.node-taints]
    "node.eks.amazonaws.com/not-ready" = "true:NoSchedule"

    [settings.kubernetes.node-labels]
    "stratos.sh/pool" = "bottlerocket-workers"
```

### Minimal Configuration

```yaml title="awsnodeclass-minimal.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: minimal
spec:
  instanceType: m5.large
  ami: ami-0123456789abcdef0
  subnetIds:
    - subnet-12345678
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role
```

### High-Performance Configuration

```yaml title="awsnodeclass-performance.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: high-performance
spec:
  region: us-east-1
  instanceType: c5.4xlarge
  ami: ami-0123456789abcdef0

  subnetIds:
    - subnet-12345678
    - subnet-87654321
    - subnet-abcdefgh
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 100
      volumeType: gp3
      encrypted: true
      iops: 6000
      throughput: 500

  tags:
    Environment: production
    Tier: compute
    CostCenter: engineering
```

## Finding AMI IDs

### EKS-Optimized AMIs

```bash
# Amazon Linux 2 (x86_64)
aws ssm get-parameter \
  --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2/recommended/image_id \
  --query "Parameter.Value" --output text

# Amazon Linux 2023 (x86_64)
aws ssm get-parameter \
  --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2023/x86_64/standard/recommended/image_id \
  --query "Parameter.Value" --output text

# Amazon Linux 2023 (ARM64)
aws ssm get-parameter \
  --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2023/arm64/standard/recommended/image_id \
  --query "Parameter.Value" --output text

# Bottlerocket (x86_64)
aws ssm get-parameter \
  --name /aws/service/bottlerocket/aws-k8s-1.34/x86_64/latest/image_id \
  --query "Parameter.Value" --output text

# Bottlerocket (ARM64)
aws ssm get-parameter \
  --name /aws/service/bottlerocket/aws-k8s-1.34/arm64/latest/image_id \
  --query "Parameter.Value" --output text
```

Replace `1.34` with your EKS cluster version.

## Kubectl Commands

```bash
# List all AWSNodeClasses
kubectl get awsnodeclasses

# Short name
kubectl get awsnc

# Get detailed status
kubectl describe awsnodeclass al2023-standard

# Get YAML output
kubectl get awsnodeclass al2023-standard -o yaml

# Check which NodePools reference an AWSNodeClass
kubectl get nodepools -o custom-columns='NAME:.metadata.name,NODECLASS:.spec.template.nodeClassRef.name'
```

## Migration from Inline CloudProvider

If you have existing NodePools using the inline `cloudProvider` configuration, migrate to AWSNodeClass:

### Before (Inline Configuration)

```yaml
# OLD FORMAT - No longer supported
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 3
  template:
    labels:
      stratos.sh/pool: workers
    cloudProvider:           # Inline configuration
      provider: aws
      aws:
        instanceType: m5.large
        ami: ami-0123456789abcdef0
        subnetIds:
          - subnet-12345678
        securityGroupIds:
          - sg-12345678
        iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role
```

### After (AWSNodeClass Reference)

```yaml
# Step 1: Create the AWSNodeClass
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: standard-nodes
spec:
  instanceType: m5.large
  ami: ami-0123456789abcdef0
  subnetIds:
    - subnet-12345678
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role
---
# Step 2: Update NodePool to reference AWSNodeClass
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 3
  template:
    nodeClassRef:            # Reference to AWSNodeClass
      kind: AWSNodeClass
      name: standard-nodes
    labels:
      stratos.sh/pool: workers
```

:::warning Breaking Change
The inline `cloudProvider` configuration is no longer supported. You must migrate to the AWSNodeClass pattern. Since Stratos is in v1alpha1, breaking changes are expected.
:::

## Next Steps

- [NodePool API Reference](./nodepool.md) - NodePool configuration
- [AWS Setup](../../guides/aws-setup.md) - AWS prerequisites and IAM configuration
- [Bottlerocket Setup](../../guides/bottlerocket.md) - Using AWSNodeClass with Bottlerocket
