## Requirements

### Requirement: CloudProvider interface handles instance lifecycle only

The `CloudProvider` interface SHALL contain only operations that work with instance IDs, which are truly cloud-agnostic.

The interface SHALL include:
- `StartInstance(instanceID string)`
- `StopInstance(instanceID string, force bool)`
- `TerminateInstance(instanceID string)`
- `GetInstanceState(instanceID string)`
- `GetInstance(instanceID string)`
- `ListInstances(tags map[string]string)`
- `UpdateInstanceTags(instanceID string, tags map[string]string)`

The interface SHALL NOT include `LaunchInstance` as launch requires cloud-specific configuration.

#### Scenario: Start instance uses generic interface
- **WHEN** the controller needs to start a standby node
- **THEN** it SHALL call `CloudProvider.StartInstance(instanceID)`
- **AND** this works identically regardless of cloud provider

#### Scenario: Stop instance uses generic interface
- **WHEN** the controller needs to stop a running node for scale-down
- **THEN** it SHALL call `CloudProvider.StopInstance(instanceID, force)`
- **AND** this works identically regardless of cloud provider

### Requirement: Each provider implements launch with its own NodeClass

Each cloud provider SHALL implement a launch method that takes its own NodeClass type directly.

```go
// AWS provider
func (p *AWSProvider) LaunchInstance(ctx context.Context, nodeClass *AWSNodeClass, poolName, clusterName string) (*Instance, error)

// GCP provider (future)
func (p *GCPProvider) LaunchInstance(ctx context.Context, nodeClass *GCPNodeClass, poolName, clusterName string) (*Instance, error)
```

The AWS provider launch method SHALL read resource IDs from AWSNodeClass status (resolved fields) instead of directly from spec fields. This applies to:
- AMI: read from `status.resolvedAMI`
- Subnets: read from `status.resolvedSubnets` (round-robin by subnet ID)
- Security groups: read from `status.resolvedSecurityGroups` (use all resolved IDs)
- Instance profile: read from `status.resolvedInstanceProfile`
- Metadata options: read from `spec.metadataOptions` (passthrough, no resolution)

#### Scenario: AWS provider launches with resolved IDs from status
- **WHEN** the controller launches an instance for a NodePool referencing an AWSNodeClass
- **AND** the AWSNodeClass has `status.resolvedAMI: "ami-abc"`, `status.resolvedSubnets: [{id: "subnet-aaa"}, {id: "subnet-bbb"}]`, `status.resolvedSecurityGroups: [{id: "sg-xxx"}]`, `status.resolvedInstanceProfile: "arn:aws:iam::123:instance-profile/stratos-my-nc"`
- **THEN** the provider SHALL use these resolved values for the EC2 RunInstances call

#### Scenario: Provider handles cloud-specific distribution internally
- **WHEN** the AWS provider launches an instance
- **THEN** it SHALL handle subnet selection internally using round-robin across `status.resolvedSubnets`
- **AND** this logic is encapsulated within the AWS provider, not exposed to the controller

#### Scenario: Launch blocked when status fields not populated
- **WHEN** the controller attempts to launch an instance
- **AND** `status.resolvedAMI` is empty
- **THEN** the provider SHALL return an error indicating the AWSNodeClass has not been resolved

#### Scenario: Launch includes metadata options
- **WHEN** the AWSNodeClass has `spec.metadataOptions.httpTokens: "required"`
- **THEN** the EC2 RunInstances call SHALL include `MetadataOptions` with `HttpTokens: "required"`

### Requirement: Controller switches on NodeClass kind for launch

The controller SHALL determine which provider launch method to call based on the `NodeClassRef.Kind`.

#### Scenario: Controller launches for AWSNodeClass
- **WHEN** a NodePool has `nodeClassRef.kind: AWSNodeClass`
- **THEN** the controller SHALL fetch the AWSNodeClass
- **AND** call `awsProvider.LaunchInstance(ctx, awsNodeClass, poolName, clusterName)`

#### Scenario: Controller uses generic interface for lifecycle
- **WHEN** the controller needs to start, stop, or terminate an instance
- **THEN** it SHALL use the generic `CloudProvider` interface
- **AND** it does not need to know which cloud the instance belongs to

### Requirement: LaunchConfig becomes internal to AWS provider

The existing `LaunchConfig` struct MAY be retained as an internal implementation detail within the AWS provider, but it SHALL NOT be part of any public interface.

#### Scenario: LaunchConfig used internally
- **WHEN** the AWS provider implements `LaunchInstance`
- **THEN** it MAY use `LaunchConfig` internally to organize parameters for the EC2 API call
- **AND** this is an implementation detail not exposed to the controller

### Requirement: NodeClass changes only affect new launches

When a NodeClass is modified, the changes SHALL only affect newly launched instances. Running and standby nodes SHALL continue with their original configuration.

When a selector resolves to a different AMI than previously resolved, existing standby instances SHALL NOT be automatically rotated. Only new launches SHALL use the newly resolved AMI.

#### Scenario: NodeClass updated with different instance type
- **WHEN** an AWSNodeClass is updated to change `instanceType` from "m5.large" to "m5.xlarge"
- **AND** existing nodes were launched with "m5.large"
- **THEN** existing nodes SHALL remain as "m5.large"
- **AND** only new launches SHALL use "m5.xlarge"

#### Scenario: AMI selector resolves to newer AMI
- **WHEN** an AWSNodeClass has `amiSelector.name: "eks-node-*"`
- **AND** a new AMI "eks-node-v2" is published that is newer than the previously resolved "eks-node-v1"
- **AND** existing standby instances were launched with "eks-node-v1"
- **THEN** `status.resolvedAMI` SHALL update to the new AMI ID on next reconcile
- **AND** existing standby instances SHALL remain on "eks-node-v1"
- **AND** only new launches SHALL use "eks-node-v2"
