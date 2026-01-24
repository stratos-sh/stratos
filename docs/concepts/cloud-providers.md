---
sidebar_position: 3
title: Cloud Providers
description: Understand the Stratos cloud provider abstraction
---

# Cloud Providers

Stratos uses a cloud provider abstraction to support multiple cloud platforms. This document explains the abstraction layer and supported providers.

## Provider Interface

All cloud operations go through the `CloudProvider` interface:

```go
type CloudProvider interface {
    // LaunchInstance creates a new instance from the launch configuration.
    LaunchInstance(ctx context.Context, cfg *LaunchConfig) (*Instance, error)

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

## Supported Providers

### AWS (Production)

The AWS provider (`--cloud-provider=aws`) manages EC2 instances.

**Features:**
- Full EC2 lifecycle management
- Built-in rate limiting
- Instance type to capacity mapping
- Subnet round-robin for AZ distribution

**Configuration:**

```yaml
cloudProvider:
  provider: aws
  aws:
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
      # Your initialization script
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

## Launch Configuration

The `LaunchConfig` struct defines instance parameters:

```go
type LaunchConfig struct {
    // InstanceType is the EC2 instance type
    InstanceType string

    // AMI is the Amazon Machine Image ID
    AMI string

    // SubnetID is the target subnet
    SubnetID string

    // SecurityGroupIDs are the security groups to attach
    SecurityGroupIDs []string

    // IAMInstanceProfile is the IAM instance profile
    IAMInstanceProfile string

    // UserData is the user data script (base64 encoded)
    UserData string

    // BlockDeviceMappings defines EBS volumes
    BlockDeviceMappings []BlockDeviceMapping

    // Tags are additional tags to apply
    Tags map[string]string
}
```

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

The cloud provider interface is designed to support additional providers. Planned support includes:

- **GCP** - Google Compute Engine instances
- **Azure** - Azure Virtual Machines

:::note
GCP and Azure support is planned but not yet implemented. Contributions are welcome.
:::

## Next Steps

- [AWS Setup](../guides/aws-setup.md) - Configure AWS prerequisites
- [NodePool API](../reference/api/nodepool.md) - Complete API reference
