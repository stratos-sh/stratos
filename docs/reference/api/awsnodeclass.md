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
  # Bootstrap template for userData generation (required)
  bootstrapTemplate: <string>  # AL2023, AL2, or Bottlerocket
  region: <string>
  instanceType: <string>
  architecture: <string>
  # AMI: use one of ami or amiSelector
  ami: <string>
  amiSelector: <AMISelector>
  # Subnets: use one of subnetIds or subnetSelector
  subnetIds: <[]string>
  subnetSelector: <SubnetSelector>
  # Security groups: use one of securityGroupIds or securityGroupSelector
  securityGroupIds: <[]string>
  securityGroupSelector: <SecurityGroupSelector>
  # IAM: use one of iamInstanceProfile or role
  iamInstanceProfile: <string>
  role: <string>
  # IMDS configuration
  metadataOptions: <MetadataOptions>
  # Custom scripts to merge with generated bootstrap (optional)
  customUserData: <string>
  blockDeviceMappings: <[]BlockDeviceMapping>
  tags: <map[string]string>
  spotConfig:
    instanceTypes: <[]string>
    allocationStrategy: <string>
    maxPrice: <string>
status:
  nodePoolCount: <int32>
  resolvedAMI: <string>
  resolvedSubnets: <[]ResolvedSubnet>
  resolvedSecurityGroups: <[]ResolvedSecurityGroup>
  resolvedInstanceProfile: <string>
  resolvedLaunchTemplateID: <string>
  conditions: <[]Condition>
```

## Spec Fields

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `bootstrapTemplate` | string | Bootstrap template for userData generation: `AL2023`, `AL2`, or `Bottlerocket`. Stratos generates the complete bootstrap script using cluster configuration from Helm values. |
| `instanceType` | string | EC2 instance type (e.g., `m5.large`, `c5.xlarge`, `m8g.large`). |

### Bootstrap Templates

| Template | AMI Family | Bootstrap Format | Warmup Mode |
|----------|-----------|------------------|-------------|
| `AL2023` | Amazon Linux 2023 | nodeadm MIME multipart | SelfStop |
| `AL2` | Amazon Linux 2 | bootstrap.sh MIME multipart | SelfStop |
| `Bottlerocket` | Bottlerocket | TOML configuration | ControllerStop |

### AMI Configuration (one required)

Exactly one of `ami` or `amiSelector` must be specified. Setting both is rejected by CEL validation.

| Field | Type | Description |
|-------|------|-------------|
| `ami` | string | Static AMI ID. Must match pattern `^ami-[a-z0-9]+$`. |
| `amiSelector` | [AMISelector](#amiselector) | Dynamic AMI selection by tags, name, and/or owner. The newest matching AMI is selected. |

### Subnet Configuration (one required)

Exactly one of `subnetIds` or `subnetSelector` must be specified.

| Field | Type | Description |
|-------|------|-------------|
| `subnetIds` | []string | Static list of subnet IDs. Minimum 1. Instances are distributed round-robin. |
| `subnetSelector` | [SubnetSelector](#subnetselector) | Dynamic subnet selection by tags. All matching subnets are used for round-robin placement. |

### Security Group Configuration (one required)

Exactly one of `securityGroupIds` or `securityGroupSelector` must be specified.

| Field | Type | Description |
|-------|------|-------------|
| `securityGroupIds` | []string | Static list of security group IDs. Minimum 1. |
| `securityGroupSelector` | [SecurityGroupSelector](#securitygroupselector) | Dynamic security group selection by tags and/or name. |

### IAM Configuration (one required)

Exactly one of `iamInstanceProfile` or `role` must be specified.

| Field | Type | Description |
|-------|------|-------------|
| `iamInstanceProfile` | string | Static IAM instance profile ARN or name. |
| `role` | string | IAM role name. The controller creates and manages an instance profile named `stratos-<cluster>-<name>` automatically. |

### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `region` | string | Controller region | AWS region for launching instances (e.g., `us-east-1`). |
| `architecture` | string | `x86_64` | Instance architecture: `x86_64` or `arm64`. Used for AMI selection when using `amiSelector`. |
| `metadataOptions` | [MetadataOptions](#metadataoptions) | EC2 defaults | IMDS configuration for launched instances. |
| `customUserData` | string | - | Additional scripts/config to merge with generated bootstrap. For AL2/AL2023, shell scripts. For Bottlerocket, additional TOML settings. |
| `blockDeviceMappings` | [][BlockDeviceMapping](#block-device-mapping) | - | EBS volume configuration. |
| `tags` | map[string]string | - | Additional tags to apply to instances. Stratos management tags are added automatically. |

### Spot Configuration

Configure Spot instance fleet parameters for use with NodePool `spotReplacement`. When `spotConfig` is set, Stratos creates an EC2 Launch Template and uses the EC2 `CreateFleet` API (type `instant`, not the legacy SpotFleet API) for diversified Spot instance launches. The fleet builds a cross-product of `instanceTypes` and resolved subnets as overrides, maximizing availability across types and availability zones.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `spotConfig.instanceTypes` | []string | Yes (if spotConfig set) | - | Instance types for Spot fleet diversification. Multiple types increase the chance of getting capacity. Minimum 1. |
| `spotConfig.allocationStrategy` | string | No | `price-capacity-optimized` | Spot fleet allocation strategy. |
| `spotConfig.maxPrice` | string | No | On-Demand price | Maximum Spot price. Empty string uses On-Demand price as cap. |

```yaml title="Example"
spotConfig:
  instanceTypes:
    - m5.large
    - m5a.large
    - m5d.large
  allocationStrategy: price-capacity-optimized
  maxPrice: "0.05"
```

### AMISelector

Selects an AMI dynamically. All specified fields use AND semantics. When multiple AMIs match, the most recently created one is selected.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tags` | map[string]string | No | Tag key-value pairs to match (AND semantics). |
| `name` | string | No | AMI name pattern. Supports wildcards (e.g., `my-eks-ami-*`). |
| `owner` | string | No | Owner account ID or alias (`self`, `amazon`). |

```yaml title="Example"
amiSelector:
  name: "my-eks-ami-*"
  owner: self
  tags:
    kubernetes.io/os: linux
```

### SubnetSelector

Selects subnets dynamically by tags.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tags` | map[string]string | No | Tag key-value pairs to match (AND semantics). |

```yaml title="Example"
subnetSelector:
  tags:
    stratos.sh/discovery: my-cluster
```

### SecurityGroupSelector

Selects security groups dynamically by tags and/or name.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tags` | map[string]string | No | Tag key-value pairs to match (AND semantics). |
| `name` | string | No | Security group name pattern. Supports wildcards (e.g., `stratos-nodes-*`). |

```yaml title="Example"
securityGroupSelector:
  tags:
    stratos.sh/discovery: my-cluster
  name: "stratos-*"
```

### MetadataOptions

Controls the Instance Metadata Service (IMDS) configuration for launched instances. Maps directly to EC2 `InstanceMetadataOptionsRequest`.

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `httpTokens` | string | No | `required` or `optional` | `required` enforces IMDSv2; `optional` allows both v1 and v2. |
| `httpPutResponseHopLimit` | *int32 | No | 1--64 | HTTP PUT response hop limit for IMDS token requests. Set to 2 for containerized workloads. |
| `httpEndpoint` | string | No | `enabled` or `disabled` | Controls whether the IMDS endpoint is available. |

```yaml title="Example: Enforce IMDSv2"
metadataOptions:
  httpTokens: required
  httpPutResponseHopLimit: 2
  httpEndpoint: enabled
```

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

The AWSNodeClass status is updated by the controller. When selectors are used, the resolved values are populated after successful resolution.

| Field | Type | Description |
|-------|------|-------------|
| `nodePoolCount` | int32 | Number of NodePools currently referencing this AWSNodeClass. |
| `resolvedAMI` | string | The resolved AMI ID (from selector or static field). |
| `resolvedSubnets` | [][ResolvedSubnet](#resolvedsubnet) | Resolved subnets with IDs and availability zones. |
| `resolvedSecurityGroups` | [][ResolvedSecurityGroup](#resolvedsecuritygroup) | Resolved security groups with IDs and names. |
| `resolvedInstanceProfile` | string | The resolved instance profile ARN. |
| `resolvedLaunchTemplateID` | string | The EC2 Launch Template ID created for Spot fleet. Populated when `spotConfig` is set. |
| `conditions` | []Condition | Current conditions (see below). |

### ResolvedSubnet

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Subnet ID (e.g., `subnet-12345678`). |
| `zone` | string | Availability zone (e.g., `us-east-1a`). Empty for static subnet IDs. |

### ResolvedSecurityGroup

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Security group ID (e.g., `sg-12345678`). |
| `name` | string | Security group name. Empty for static security group IDs. |

### Conditions

| Condition | Description |
|-----------|-------------|
| `Valid` | The AWSNodeClass spec is valid. |
| `InUse` | The AWSNodeClass is referenced by one or more NodePools. |
| `AMIReady` | The AMI has been resolved successfully. |
| `SubnetsReady` | Subnets have been resolved successfully. |
| `SecurityGroupsReady` | Security groups have been resolved successfully. |
| `InstanceProfileReady` | The instance profile is ready (static or controller-managed). |

#### Condition Reasons

| Reason | Applies To | Description |
|--------|-----------|-------------|
| `Resolved` | AMI, Subnets, SGs, InstanceProfile | Resource successfully resolved. |
| `ResolutionFailed` | AMI, Subnets, SGs, InstanceProfile | AWS API call failed. |
| `NoMatchingResources` | AMI, Subnets, SGs | No resources matched the selector. |
| `RoleNotFound` | InstanceProfile | The IAM role does not exist. |
| `RoleUpdateFailed` | InstanceProfile | Failed to update the role on the instance profile. |

:::note
The NodePool controller checks all four readiness conditions before launching instances. If any condition is `False`, the NodePool is set to `Ready=False` with reason `NodeClassNotReady`.
:::

## Validation Rules

The CRD uses CEL (Common Expression Language) validation to enforce mutual exclusivity:

| Rule | Error Message |
|------|---------------|
| `ami` or `amiSelector` must be set | "one of ami or amiSelector is required" |
| `ami` and `amiSelector` cannot both be set | "ami and amiSelector are mutually exclusive" |
| `subnetIds` or `subnetSelector` must be set | "one of subnetIds or subnetSelector is required" |
| `subnetIds` and `subnetSelector` cannot both be set | "subnetIds and subnetSelector are mutually exclusive" |
| `securityGroupIds` or `securityGroupSelector` must be set | "one of securityGroupIds or securityGroupSelector is required" |
| `securityGroupIds` and `securityGroupSelector` cannot both be set | "securityGroupIds and securityGroupSelector are mutually exclusive" |
| `iamInstanceProfile` or `role` must be set | "one of iamInstanceProfile or role is required" |
| `iamInstanceProfile` and `role` cannot both be set | "role and iamInstanceProfile are mutually exclusive" |

These validations run at admission time, providing immediate feedback when applying manifests.

## Lifecycle Management

### Deletion Protection

AWSNodeClass uses a finalizer (`stratos.sh/in-use`) to prevent deletion while referenced by NodePools. You must delete all referencing NodePools before deleting an AWSNodeClass.

```bash
# This will block if NodePools reference it
kubectl delete awsnodeclass my-nodeclass

# Check which NodePools reference it
kubectl get nodepools -o json | jq '.items[] | select(.spec.template.nodeClassRef.name == "my-nodeclass") | .metadata.name'

# Delete the NodePools first, then the AWSNodeClass
kubectl delete nodepool my-pool
kubectl delete awsnodeclass my-nodeclass
```

### Instance Profile Cleanup

When using `spec.role`, the controller adds a second finalizer (`stratos.sh/instance-profile`). On deletion, the controller:

1. Waits for the `stratos.sh/in-use` finalizer to be removed (all NodePools deleted)
2. Removes the role from the instance profile
3. Deletes the instance profile
4. Removes the `stratos.sh/instance-profile` finalizer

This ordering ensures no NodePools are launching with the profile while it is being deleted.

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
      name: standard-nodes
    labels:
      stratos.sh/pool: workers
```

## Examples

### Static IDs (Original Pattern)

```yaml title="awsnodeclass-static.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: static-nodes
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.large
  ami: ami-0123456789abcdef0
  subnetIds:
    - subnet-12345678
    - subnet-87654321
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role
```

### Dynamic Selectors (Recommended)

```yaml title="awsnodeclass-selectors.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: dynamic-nodes
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.large

  amiSelector:
    name: "my-eks-ami-*"
    owner: self
    tags:
      kubernetes.io/os: linux

  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  role: my-eks-node-role

  metadataOptions:
    httpTokens: required
    httpPutResponseHopLimit: 2

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 20
      volumeType: gp3
      encrypted: true
```

### Mixed (Static AMI, Dynamic Networking)

```yaml title="awsnodeclass-mixed.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: mixed-nodes
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.large
  ami: ami-0123456789abcdef0

  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/eks-node-role
```

### Bottlerocket with Selectors

```yaml title="awsnodeclass-bottlerocket-selectors.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: bottlerocket-dynamic
spec:
  # Stratos generates Bottlerocket TOML configuration automatically
  bootstrapTemplate: Bottlerocket
  instanceType: m5.large
  architecture: x86_64

  amiSelector:
    name: "bottlerocket-aws-k8s-*-x86_64-*"
    owner: amazon

  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  role: my-eks-node-role

  metadataOptions:
    httpTokens: required
    httpPutResponseHopLimit: 2

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 8
      volumeType: gp3
      encrypted: true
    - deviceName: /dev/xvdb
      volumeSize: 20
      volumeType: gp3
      encrypted: true

  # Optional: Additional Bottlerocket settings
  customUserData: |
    [settings.host-containers.admin]
    enabled = true
```

### Spot-Enabled Configuration

```yaml title="awsnodeclass-spot.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: spot-enabled
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.large

  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  role: my-eks-node-role

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 50
      volumeType: gp3
      encrypted: true

  spotConfig:
    instanceTypes:
      - m5.large
      - m5a.large
      - m5d.large
      - m5ad.large
    allocationStrategy: price-capacity-optimized
```

The `instanceType` field defines the On-Demand instance type. The `spotConfig.instanceTypes` list defines the Spot fleet diversification pool. Using multiple instance types increases the chance of getting Spot capacity.

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

# Get detailed status including resolved resources
kubectl describe awsnodeclass dynamic-nodes

# Get YAML output to see resolved status
kubectl get awsnodeclass dynamic-nodes -o yaml

# Check which NodePools reference an AWSNodeClass
kubectl get nodepools -o custom-columns='NAME:.metadata.name,NODECLASS:.spec.template.nodeClassRef.name'

# Check resolved AMI
kubectl get awsnodeclass dynamic-nodes -o jsonpath='{.status.resolvedAMI}'

# Check all conditions
kubectl get awsnodeclass dynamic-nodes -o jsonpath='{.status.conditions[*].type}={.status.conditions[*].status}'
```

## Next Steps

- [Dynamic Resource Selectors Guide](../../guides/dynamic-resource-selectors.md) -- Step-by-step guide with examples and tagging patterns
- [NodePool API Reference](./nodepool.md) -- NodePool configuration
- [AWS Setup](../../guides/aws-setup.md) -- AWS prerequisites and IAM configuration
- [Bottlerocket Setup](../../guides/bottlerocket.md) -- Using AWSNodeClass with Bottlerocket
