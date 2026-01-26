## ADDED Requirements

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
- `instanceType`: EC2 instance type (required)
- `ami`: AMI ID (required)
- `subnetIds`: List of subnet IDs (required, min 1)
- `securityGroupIds`: List of security group IDs (required, min 1)
- `iamInstanceProfile`: IAM instance profile ARN or name (required)
- `userData`: Base64-encoded user data script (optional)
- `blockDeviceMappings`: EBS volume configuration (optional)
- `tags`: Additional instance tags (optional)
- `region`: AWS region, defaults to controller's region (optional)

#### Scenario: Create valid AWSNodeClass
- **WHEN** an AWSNodeClass is created with all required fields
- **THEN** the resource SHALL be accepted and stored

#### Scenario: AWSNodeClass missing required field
- **WHEN** an AWSNodeClass is created without `instanceType`
- **THEN** the resource SHALL be rejected with validation error "instanceType is required"

#### Scenario: AWSNodeClass with invalid subnetIds
- **WHEN** an AWSNodeClass is created with empty `subnetIds` array
- **THEN** the resource SHALL be rejected with validation error "subnetIds must have at least 1 item"

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
