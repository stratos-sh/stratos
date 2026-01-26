---
sidebar_position: 3
title: Cloud Providers
description: Understand the Stratos cloud provider abstraction and NodeClass pattern
---

# Cloud Providers

Stratos uses a cloud provider abstraction to support multiple cloud platforms. This document explains the architecture, the NodeClass pattern for cloud-specific configuration, and supported providers.

## Architecture Overview

Stratos separates concerns between pool management and cloud-specific configuration:

```
+------------------+       references        +------------------+
|    NodePool      | ----------------------> |  AWSNodeClass    |
+------------------+                         +------------------+
| - poolSize       |                         | - instanceType   |
| - minStandby     |                         | - ami            |
| - labels, taints |                         | - subnetIds      |
| - preWarm config |                         | - securityGroups |
| - scaleDown      |                         | - userData       |
+------------------+                         +------------------+
         |
         | uses
         v
+------------------+
| CloudProvider    |
|    Interface     |
+------------------+
| - StartInstance  |
| - StopInstance   |
| - Terminate...   |
| - GetInstance    |
| - ListInstances  |
+------------------+
```

### Why Separate NodeClass?

1. **Reusability**: Multiple NodePools can share the same EC2 configuration
2. **Separation of concerns**: Pool sizing vs. instance configuration
3. **Multi-cloud support**: Each cloud gets its own NodeClass type (AWSNodeClass, GCPNodeClass, etc.)
4. **Independent evolution**: Cloud-specific schemas can evolve without affecting NodePool

This pattern follows [Karpenter's NodeClass design](https://karpenter.sh/docs/concepts/nodeclasses/).

## Provider Interface

The CloudProvider interface defines instance lifecycle operations that work with instance IDs:

```go
type CloudProvider interface {
    // StartInstance starts a stopped instance.
    StartInstance(ctx context.Context, instanceID string) error

    // StopInstance stops a running instance.
    StopInstance(ctx context.Context, instanceID string, force bool) error

    // TerminateInstance terminates an instance permanently.
    TerminateInstance(ctx context.Context, instanceID string) error

    // GetInstanceState returns the current state of an instance.
    GetInstanceState(ctx context.Context, instanceID string) (InstanceState, error)

    // GetInstance returns full details of an instance.
    GetInstance(ctx context.Context, instanceID string) (*Instance, error)

    // ListInstances returns instances matching the given tags.
    ListInstances(ctx context.Context, tags map[string]string) ([]*Instance, error)

    // UpdateInstanceTags updates tags on an instance.
    UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error
}
```

:::note
`LaunchInstance` is **not** part of the generic interface because launch requires cloud-specific configuration (e.g., AWSNodeClass). Each provider implements its own `LaunchInstance` method that takes its specific NodeClass type directly.
:::

## NodeClass Resources

NodeClass resources define cloud-specific instance configuration. Currently, Stratos supports:

| NodeClass | Cloud | Description |
|-----------|-------|-------------|
| `AWSNodeClass` | AWS | EC2 instance configuration (instance type, AMI, networking, IAM) |
| `GCPNodeClass` | GCP | Planned - Compute Engine configuration |
| `AzureNodeClass` | Azure | Planned - Virtual Machine configuration |

### AWSNodeClass

AWSNodeClass defines AWS EC2 configuration:

```yaml
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: production-nodes
spec:
  region: us-east-1
  instanceType: m5.large
  ami: ami-0123456789abcdef0
  subnetIds:
    - subnet-12345678
    - subnet-87654321
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/node-role
  userData: |
    #!/bin/bash
    /etc/eks/bootstrap.sh my-cluster
    # ... warmup script
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 50
      volumeType: gp3
      encrypted: true
  tags:
    Environment: production
```

See [AWSNodeClass API Reference](../reference/api/awsnodeclass.md) for complete documentation.

### NodePool Reference

NodePools reference NodeClass resources via `spec.template.nodeClassRef`:

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
      kind: AWSNodeClass      # NodeClass type
      name: production-nodes  # NodeClass name
    labels:
      stratos.sh/pool: workers
```

## Supported Providers

### AWS (Production)

The AWS provider (`--cloud-provider=aws`) manages EC2 instances.

**Features:**
- Full EC2 lifecycle management (launch, start, stop, terminate)
- Built-in rate limiting for API throttling protection
- Instance type to capacity mapping for scale-up calculations
- Subnet round-robin for AZ distribution
- AWSNodeClass for configuration

**NodeClass:** `AWSNodeClass`

**Configuration Example:**

```yaml
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: standard-nodes
spec:
  region: us-east-1
  instanceType: m5.large
  ami: ami-0123456789abcdef0
  subnetIds:
    - subnet-12345678
    - subnet-87654321
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/node-role
  userData: |
    #!/bin/bash
    /etc/eks/bootstrap.sh my-cluster \
      --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
    until curl -sf http://localhost:10248/healthz; do sleep 5; done
    sleep 30
    poweroff
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 50
      volumeType: gp3
      encrypted: true
  tags:
    Environment: production
```

### Fake (Testing)

The fake provider (`--cloud-provider=fake`) is a mock implementation for testing and local development.

**Features:**
- In-memory instance tracking
- Configurable hooks for testing
- No cloud costs

**Usage:**

```bash
go run ./cmd/stratos/main.go --cluster-name=test --cloud-provider=fake
```

:::tip
Use the fake provider for local development and testing. It allows rapid iteration without cloud costs or API rate limits.
:::

## Instance States

The provider interface uses a common set of instance states:

| State | Description |
|-------|-------------|
| `pending` | Instance is launching |
| `running` | Instance is running |
| `stopping` | Instance is stopping |
| `stopped` | Instance is stopped |
| `terminated` | Instance is terminated |
| `unknown` | State cannot be determined |

## Instance Tags

Stratos uses tags to track instance ownership and state:

| Tag | Description | Example |
|-----|-------------|---------|
| `managed-by` | Identifies Stratos-managed instances | `stratos` |
| `stratos.sh/pool` | NodePool name | `workers` |
| `stratos.sh/cluster` | Kubernetes cluster name | `production` |
| `stratos.sh/state` | Current Stratos state | `standby` |

These tags are used for:
- Discovering managed instances on startup
- Filtering instances by pool
- Auditing and cost allocation

## Rate Limiting

The AWS provider includes built-in rate limiting to avoid EC2 API throttling:

| Operation | Rate Limit |
|-----------|------------|
| DescribeInstances | 20 req/s |
| RunInstances | 5 req/s |
| StartInstances | 5 req/s |
| StopInstances | 5 req/s |
| TerminateInstances | 5 req/s |
| CreateTags | 10 req/s |

:::note
Rate limits are applied per controller instance. If you see `RateLimitError` in logs, consider reducing the reconciliation frequency.
:::

## Error Handling

The provider interface defines common error types:

| Error | Description |
|-------|-------------|
| `InstanceNotFoundError` | Instance does not exist |
| `RateLimitError` | API rate limit exceeded |
| `InsufficientCapacityError` | Insufficient capacity in region/AZ |
| `InvalidConfigError` | Invalid launch configuration |

The controller handles these errors appropriately:
- `InstanceNotFoundError`: Cleans up orphaned Kubernetes node
- `RateLimitError`: Retries with exponential backoff
- `InsufficientCapacityError`: Tries different subnets/AZs

## Future Providers

The cloud provider architecture is designed to support additional providers:

- **GCP** - Google Compute Engine instances (GCPNodeClass)
- **Azure** - Azure Virtual Machines (AzureNodeClass)

Adding a new provider requires:
1. Implement the CloudProvider interface
2. Create a new NodeClass CRD (e.g., GCPNodeClass)
3. Implement the provider-specific LaunchInstance method
4. Register the provider in the factory

:::note
GCP and Azure support is planned but not yet implemented. Contributions are welcome.
:::

## Next Steps

- [AWSNodeClass Reference](../reference/api/awsnodeclass.md) - Complete AWSNodeClass API
- [AWS Setup](../guides/aws-setup.md) - Configure AWS prerequisites
- [NodePool API](../reference/api/nodepool.md) - Complete NodePool API reference
