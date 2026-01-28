## Requirements

### Requirement: Resolver translates selectors into concrete resource IDs

The AWS provider SHALL include a `Resolver` interface that translates selector-based lookups into concrete AWS resource IDs. The resolver SHALL support AMI, subnet, and security group resolution.

The `Resolver` interface SHALL define:
- `ResolveAMI(ctx, selector *AMISelector) (string, error)`
- `ResolveSubnets(ctx, selector *SubnetSelector) ([]ResolvedSubnet, error)`
- `ResolveSecurityGroups(ctx, selector *SecurityGroupSelector) ([]ResolvedSecurityGroup, error)`

The resolver SHALL be injectable to support testing with fakes and LocalStack.

#### Scenario: Resolver interface used by controller
- **WHEN** the controller reconciles an AWSNodeClass with selector fields
- **THEN** it SHALL call the injected `Resolver` implementation to resolve selectors
- **AND** the resolver implementation MAY be a real AWS resolver, a fake, or a LocalStack-backed resolver

### Requirement: Subnet selector resolves subnets by tags

The resolver SHALL resolve subnets by matching EC2 tags using the `DescribeSubnets` API with tag filters. All tags in the selector SHALL be combined with AND semantics.

The resolved result SHALL include for each subnet:
- Subnet ID
- Availability zone

#### Scenario: Subnet selector matches by tags
- **WHEN** an AWSNodeClass has `subnetSelector.tags: {"kubernetes.io/cluster/my-cluster": "owned", "stratos.sh/discovery": "true"}`
- **AND** three subnets in the account have both tags
- **THEN** the resolver SHALL return all three subnets with their IDs and availability zones

#### Scenario: Subnet selector matches no subnets
- **WHEN** an AWSNodeClass has `subnetSelector.tags: {"nonexistent": "tag"}`
- **AND** no subnets match
- **THEN** the resolver SHALL return an error indicating no matching subnets were found

#### Scenario: Subnet selector with multiple tags uses AND semantics
- **WHEN** an AWSNodeClass has `subnetSelector.tags: {"env": "prod", "tier": "private"}`
- **AND** subnet-a has both tags, subnet-b has only `env: prod`
- **THEN** the resolver SHALL return only subnet-a

### Requirement: Security group selector resolves by tags and name

The resolver SHALL resolve security groups using `DescribeSecurityGroups` API with tag filters and/or name filters. Tags use AND semantics. The `name` field SHALL support wildcard matching using the `*` character.

The resolved result SHALL include for each security group:
- Security group ID
- Security group name

#### Scenario: Security group selector matches by tags
- **WHEN** an AWSNodeClass has `securityGroupSelector.tags: {"stratos.sh/discovery": "my-cluster"}`
- **AND** two security groups have that tag
- **THEN** the resolver SHALL return both security groups

#### Scenario: Security group selector matches by name wildcard
- **WHEN** an AWSNodeClass has `securityGroupSelector.name: "my-cluster-node-*"`
- **AND** security groups "my-cluster-node-sg" and "my-cluster-node-extra" exist
- **THEN** the resolver SHALL return both security groups

#### Scenario: Security group selector with both tags and name
- **WHEN** an AWSNodeClass has `securityGroupSelector.tags: {"env": "prod"}` and `securityGroupSelector.name: "cluster-*"`
- **THEN** the resolver SHALL return security groups matching both the tag AND the name pattern

#### Scenario: Security group selector matches nothing
- **WHEN** an AWSNodeClass has a security group selector that matches no security groups
- **THEN** the resolver SHALL return an error indicating no matching security groups were found

### Requirement: AMI selector resolves by tags, name, and owner

The resolver SHALL resolve AMIs using `DescribeImages` API with tag filters, name filters, and owner filters. The `name` field SHALL support wildcard matching. The `owner` field SHALL accept account IDs or aliases ("self", "amazon").

When multiple AMIs match, the resolver SHALL select the one with the most recent `CreationDate`.

The resolved result SHALL be a single AMI ID string.

#### Scenario: AMI selector matches by tags
- **WHEN** an AWSNodeClass has `amiSelector.tags: {"stratos.sh/ami": "eks-node"}`
- **AND** two AMIs have that tag with creation dates 2025-01-01 and 2025-06-01
- **THEN** the resolver SHALL return the AMI ID of the one created on 2025-06-01

#### Scenario: AMI selector matches by name wildcard
- **WHEN** an AWSNodeClass has `amiSelector.name: "amazon-eks-node-1.30-*"`
- **THEN** the resolver SHALL return the most recently created AMI matching that name pattern

#### Scenario: AMI selector with owner filter
- **WHEN** an AWSNodeClass has `amiSelector.owner: "amazon"` and `amiSelector.name: "amazon-eks-*"`
- **THEN** the resolver SHALL only consider AMIs owned by the "amazon" account

#### Scenario: AMI selector with tags and name combined
- **WHEN** an AWSNodeClass has `amiSelector.tags: {"approved": "true"}` and `amiSelector.name: "my-ami-*"`
- **THEN** the resolver SHALL return the newest AMI matching both the tag AND the name pattern

#### Scenario: AMI selector matches nothing
- **WHEN** an AWSNodeClass has an AMI selector that matches no AMIs
- **THEN** the resolver SHALL return an error indicating no matching AMIs were found

### Requirement: Resolved resources are cached in AWSNodeClass status

The controller SHALL write resolved resource IDs to AWSNodeClass status fields after successful resolution. When static IDs are used (no selector), the controller SHALL populate the same status fields with the static values.

Status fields:
- `status.resolvedAMI`: The resolved AMI ID (string)
- `status.resolvedSubnets`: List of resolved subnets with ID and availability zone
- `status.resolvedSecurityGroups`: List of resolved security groups with ID and name
- `status.resolvedInstanceProfile`: The resolved instance profile ARN

#### Scenario: Selector resolution populates status
- **WHEN** an AWSNodeClass has `subnetSelector.tags: {"stratos.sh/discovery": "my-cluster"}`
- **AND** resolution finds subnets subnet-aaa (us-east-1a) and subnet-bbb (us-east-1b)
- **THEN** `status.resolvedSubnets` SHALL contain `[{id: "subnet-aaa", zone: "us-east-1a"}, {id: "subnet-bbb", zone: "us-east-1b"}]`

#### Scenario: Static IDs also populate status
- **WHEN** an AWSNodeClass has `subnetIds: ["subnet-xxx", "subnet-yyy"]` (no selector)
- **THEN** `status.resolvedSubnets` SHALL be populated with the static IDs

#### Scenario: Resolution re-runs on each reconcile
- **WHEN** an AWSNodeClass with selectors is reconciled
- **AND** a new subnet matching the tags has been added since last reconcile
- **THEN** `status.resolvedSubnets` SHALL include the newly discovered subnet

### Requirement: Transient resolution failures preserve last known good values

When resolution fails due to a transient error (AWS API throttle, network error, timeout), the controller SHALL keep the previously resolved values in status and emit a warning event. The readiness condition SHALL remain True if previous values exist.

Only when resolution has NEVER succeeded (no cached values in status) SHALL a transient failure set the condition to False.

#### Scenario: Transient failure with cached values
- **WHEN** an AWSNodeClass has previously resolved `status.resolvedSubnets: [{id: "subnet-aaa"}]`
- **AND** the next resolution attempt fails with an AWS throttle error
- **THEN** `status.resolvedSubnets` SHALL retain `[{id: "subnet-aaa"}]`
- **AND** `SubnetsReady` SHALL remain True
- **AND** the controller SHALL emit a Warning event "Subnet resolution failed, using cached values"

#### Scenario: Transient failure with no cached values
- **WHEN** an AWSNodeClass has never successfully resolved subnets
- **AND** resolution fails with an AWS API error
- **THEN** `SubnetsReady` SHALL be set to False with reason "ResolutionFailed"
- **AND** launches SHALL be blocked

#### Scenario: Resolution succeeds after transient failure
- **WHEN** resolution previously failed but cached values were preserved
- **AND** the next resolution attempt succeeds with updated results
- **THEN** `status.resolvedSubnets` SHALL be updated with the new results

### Requirement: Dedicated AWSNodeClass reconciler handles resolution

A dedicated AWSNodeClass reconciler in the AWS provider package (`internal/cloudprovider/aws/`) SHALL watch AWSNodeClass resources and run resource resolution independently of the NodePool controller. This ensures resolution runs immediately when an AWSNodeClass is created or updated, regardless of whether a NodePool references it. The reconciler is registered with the manager in `cmd/stratos/main.go`.

The AWSNodeClass reconciler SHALL:
- Run selector resolution (AMI, subnets, security groups)
- Manage instance profile lifecycle when `spec.role` is set
- Set readiness conditions (`AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`)
- Handle the instance profile cleanup finalizer on deletion

The NodePool controller SHALL read from AWSNodeClass status (resolved fields) and check conditions before launching. It SHALL NOT perform resolution.

#### Scenario: AWSNodeClass resolved without NodePool reference
- **WHEN** an AWSNodeClass is created with `subnetSelector.tags: {"stratos.sh/discovery": "my-cluster"}`
- **AND** no NodePool references it yet
- **THEN** the AWSNodeClass reconciler SHALL resolve the subnets
- **AND** `status.resolvedSubnets` SHALL be populated
- **AND** `SubnetsReady` SHALL be True

#### Scenario: NodePool reads resolved values from status
- **WHEN** a NodePool references an AWSNodeClass
- **AND** the AWSNodeClass has `SubnetsReady=True` and `status.resolvedSubnets` populated
- **THEN** the NodePool controller SHALL use the resolved subnet IDs for launching

### Requirement: Resolution status conditions indicate readiness

The controller SHALL set status conditions on AWSNodeClass indicating whether each resource type has been successfully resolved.

Conditions:
- `AMIReady`: True when `status.resolvedAMI` is populated
- `SubnetsReady`: True when `status.resolvedSubnets` has at least one entry
- `SecurityGroupsReady`: True when `status.resolvedSecurityGroups` has at least one entry
- `InstanceProfileReady`: True when `status.resolvedInstanceProfile` is populated

The controller SHALL NOT launch instances for a NodePool until all conditions on its referenced AWSNodeClass are True.

#### Scenario: All conditions True enables launch
- **WHEN** an AWSNodeClass has `AMIReady=True`, `SubnetsReady=True`, `SecurityGroupsReady=True`, `InstanceProfileReady=True`
- **THEN** the controller SHALL proceed with launching instances for referencing NodePools

#### Scenario: Failed resolution blocks launch
- **WHEN** an AWSNodeClass has `SubnetsReady=False` with reason "NoMatchingResources"
- **THEN** the controller SHALL NOT launch instances for referencing NodePools
- **AND** the controller SHALL set the NodePool condition `Ready=False` with reason "NodeClassNotReady"

#### Scenario: Resolution failure sets condition with reason
- **WHEN** subnet resolution fails because no subnets match the selector
- **THEN** the controller SHALL set `SubnetsReady=False` with reason "NoMatchingResources" and a message describing the selector that failed

### Requirement: Resolver is integration tested with LocalStack

The resolver implementation SHALL be validated with integration tests using testcontainers-go and LocalStack. Tests SHALL seed AWS resources and verify the resolver returns correct results against real AWS SDK calls.

Tests SHALL live in `internal/cloudprovider/aws/resolver_integration_test.go` with the `integration` build tag.

#### Scenario: LocalStack test validates subnet resolution
- **WHEN** the integration test seeds two subnets with tag `stratos.sh/discovery: test-cluster` in LocalStack
- **AND** the resolver runs `ResolveSubnets` with that tag selector
- **THEN** the resolver SHALL return both seeded subnet IDs

#### Scenario: LocalStack test validates AMI newest-wins
- **WHEN** the integration test seeds two AMIs with the same tag but different creation dates
- **AND** the resolver runs `ResolveAMI` with that tag selector
- **THEN** the resolver SHALL return the AMI with the more recent creation date

#### Scenario: LocalStack test validates security group name wildcard
- **WHEN** the integration test seeds security groups "cluster-node-sg" and "cluster-node-extra"
- **AND** the resolver runs `ResolveSecurityGroups` with name `cluster-node-*`
- **THEN** the resolver SHALL return both security groups
