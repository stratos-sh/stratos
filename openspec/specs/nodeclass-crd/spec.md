## Requirements

### Requirement: NodePool references NodeClass via nodeClassRef

NodePool SHALL reference cloud-specific configuration through a `nodeClassRef` field in `spec.template` instead of embedding cloud configuration directly.

The `nodeClassRef` field SHALL contain:
- `kind`: The NodeClass kind (e.g., "AWSNodeClass", "GCPNodeClass")
- `name`: The name of the NodeClass resource

#### Scenario: NodePool references AWSNodeClass
- **WHEN** a NodePool is created with `nodeClassRef.kind: AWSNodeClass` and `nodeClassRef.name: gpu-optimized`
- **THEN** the controller SHALL fetch the AWSNodeClass named "gpu-optimized" to obtain cloud configuration

#### Scenario: NodePool with missing nodeClassRef
- **WHEN** a NodePool is created without a `nodeClassRef` field
- **THEN** the NodePool SHALL fail validation with error "nodeClassRef is required"

#### Scenario: NodePool references non-existent NodeClass
- **WHEN** a NodePool references a NodeClass that does not exist
- **THEN** the controller SHALL set condition `Ready=False` with reason "NodeClassNotFound"
- **AND** the controller SHALL emit a Warning event "Referenced NodeClass not found"

### Requirement: AWSNodeClass is a cluster-scoped CRD

AWSNodeClass SHALL be a cluster-scoped custom resource containing AWS EC2 instance configuration.

The AWSNodeClass spec SHALL include:
- `bootstrapTemplate`: Bootstrap format - AL2023, AL2, or Bottlerocket (required)
- `instanceType`: EC2 instance type (required)
- `architecture`: CPU architecture - x86_64 or arm64 (optional, defaults to x86_64)
- `amiSelector`: AMI selection criteria (optional - auto-discovers if omitted)
- `subnetSelector`: Subnet selection criteria (required, or `subnetIds`)
- `subnetIds`: List of subnet IDs (required, or `subnetSelector`)
- `securityGroupSelector`: Security group selection criteria (required, or `securityGroupIds`)
- `securityGroupIds`: List of security group IDs (required, or `securityGroupSelector`)
- `role`: IAM role name for instance profile (required, or `iamInstanceProfile`)
- `iamInstanceProfile`: IAM instance profile ARN or name (required, or `role`)
- `kubelet`: Kubelet configuration - maxPods, nodeLabels, nodeTaints, extraArgs (optional)
- `customUserData`: User scripts merged with generated bootstrap (optional)
- `blockDeviceMappings`: EBS volume configuration (optional)
- `tags`: Additional instance tags (optional)
- `region`: AWS region, defaults to controller's region (optional)
- `metadataOptions`: IMDS configuration (optional)

For each mutually exclusive pair, exactly one SHALL be specified. CRD validation SHALL enforce this at admission time using CEL rules.

The AWSNodeClass status SHALL include:
- `nodePoolCount`: Number of referencing NodePools
- `resolvedAMI`: Resolved AMI ID
- `resolvedSubnets`: List of resolved subnets (ID + availability zone)
- `resolvedSecurityGroups`: List of resolved security groups (ID + name)
- `resolvedInstanceProfile`: Resolved instance profile ARN
- `conditions`: Including `Valid`, `InUse`, `AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`

#### Scenario: Create valid AWSNodeClass with bootstrapTemplate
- **WHEN** an AWSNodeClass is created with `bootstrapTemplate: AL2023` and all other required fields
- **THEN** the resource SHALL be accepted and stored

#### Scenario: AWSNodeClass missing bootstrapTemplate
- **WHEN** an AWSNodeClass is created without `bootstrapTemplate`
- **THEN** the resource SHALL be rejected with validation error "bootstrapTemplate is required"

#### Scenario: AWSNodeClass with invalid bootstrapTemplate
- **WHEN** an AWSNodeClass is created with `bootstrapTemplate: Windows`
- **THEN** the resource SHALL be rejected with validation error "bootstrapTemplate must be one of: AL2023, AL2, Bottlerocket"

#### Scenario: AWSNodeClass without amiSelector uses auto-discovery
- **WHEN** an AWSNodeClass is created with `bootstrapTemplate: AL2023` and no `amiSelector`
- **THEN** the controller SHALL auto-discover the AMI based on bootstrapTemplate and cluster version
- **AND** set `status.resolvedAMI` to the discovered AMI

#### Scenario: AWSNodeClass with explicit amiSelector
- **WHEN** an AWSNodeClass is created with `bootstrapTemplate: AL2023` and `amiSelector: {name: my-custom-*}`
- **THEN** the controller SHALL use the explicit amiSelector for AMI resolution

#### Scenario: AWSNodeClass with kubelet configuration
- **WHEN** an AWSNodeClass is created with `kubelet: {maxPods: 110, nodeLabels: {team: platform}}`
- **THEN** the controller SHALL include kubelet settings in generated userData

#### Scenario: AWSNodeClass with customUserData
- **WHEN** an AWSNodeClass is created with `customUserData: "#!/bin/bash\necho hello"`
- **THEN** the controller SHALL merge the custom script with generated userData

#### Scenario: Create valid AWSNodeClass with static IDs
- **WHEN** an AWSNodeClass is created with `amiSelector`, `subnetIds`, `securityGroupIds`, and `iamInstanceProfile`
- **THEN** the resource SHALL be accepted and stored

#### Scenario: Create valid AWSNodeClass with selectors
- **WHEN** an AWSNodeClass is created with `amiSelector`, `subnetSelector`, `securityGroupSelector`, and `role`
- **THEN** the resource SHALL be accepted and stored

#### Scenario: Create valid AWSNodeClass with mixed static and selector
- **WHEN** an AWSNodeClass is created with `amiSelector` (dynamic), `subnetSelector` (dynamic), `securityGroupIds` (static), and `role` (dynamic)
- **THEN** the resource SHALL be accepted and stored

#### Scenario: AWSNodeClass with both role and iamInstanceProfile
- **WHEN** an AWSNodeClass is created with both `role` and `iamInstanceProfile`
- **THEN** the resource SHALL be rejected with validation error "role and iamInstanceProfile are mutually exclusive"

#### Scenario: AWSNodeClass missing required field
- **WHEN** an AWSNodeClass is created without `instanceType`
- **THEN** the resource SHALL be rejected with validation error "instanceType is required"

### Requirement: NodePool cloudProvider field is removed

The `spec.template.cloudProvider` field SHALL be removed from the NodePool CRD.

This is a **BREAKING CHANGE** from the previous API.

#### Scenario: NodePool with legacy cloudProvider field
- **WHEN** a NodePool manifest contains the `cloudProvider` field
- **THEN** the API server SHALL reject it as an unknown field (with strict validation)

### Requirement: Multiple NodePools can share a NodeClass

A single NodeClass resource SHALL be referenceable by multiple NodePool resources.

#### Scenario: Two NodePools reference same AWSNodeClass
- **WHEN** NodePool "pool-a" and NodePool "pool-b" both reference AWSNodeClass "shared-config"
- **THEN** both NodePools SHALL successfully use the same cloud configuration
- **AND** instances launched for each pool SHALL use the shared configuration
