## MODIFIED Requirements

### Requirement: AWSNodeClass is a cluster-scoped CRD

AWSNodeClass SHALL be a cluster-scoped custom resource containing AWS EC2 instance configuration.

The AWSNodeClass spec SHALL include:
- `instanceType`: EC2 instance type (required)
- `ami`: AMI ID (optional, mutually exclusive with `amiSelector`)
- `amiSelector`: AMI selector for dynamic resolution (optional, mutually exclusive with `ami`)
- `subnetIds`: List of subnet IDs (optional, mutually exclusive with `subnetSelector`)
- `subnetSelector`: Subnet selector for dynamic resolution (optional, mutually exclusive with `subnetIds`)
- `securityGroupIds`: List of security group IDs (optional, mutually exclusive with `securityGroupSelector`)
- `securityGroupSelector`: Security group selector for dynamic resolution (optional, mutually exclusive with `securityGroupIds`)
- `iamInstanceProfile`: IAM instance profile ARN or name (optional, mutually exclusive with `role`)
- `role`: IAM role name for automatic instance profile management (optional, mutually exclusive with `iamInstanceProfile`)
- `metadataOptions`: IMDS configuration (optional)
- `userData`: Base64-encoded user data script (optional)
- `blockDeviceMappings`: EBS volume configuration (optional)
- `tags`: Additional instance tags (optional)
- `region`: AWS region, defaults to controller's region (optional)

For each mutually exclusive pair, exactly one SHALL be specified. CRD validation SHALL enforce this at admission time using CEL rules.

The AWSNodeClass status SHALL include:
- `nodePoolCount`: Number of referencing NodePools
- `resolvedAMI`: Resolved AMI ID
- `resolvedSubnets`: List of resolved subnets (ID + availability zone)
- `resolvedSecurityGroups`: List of resolved security groups (ID + name)
- `resolvedInstanceProfile`: Resolved instance profile ARN
- `conditions`: Including `Valid`, `InUse`, `AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`

#### Scenario: Create valid AWSNodeClass with static IDs
- **WHEN** an AWSNodeClass is created with `ami`, `subnetIds`, `securityGroupIds`, and `iamInstanceProfile`
- **THEN** the resource SHALL be accepted and stored

#### Scenario: Create valid AWSNodeClass with selectors
- **WHEN** an AWSNodeClass is created with `amiSelector`, `subnetSelector`, `securityGroupSelector`, and `role`
- **THEN** the resource SHALL be accepted and stored

#### Scenario: Create valid AWSNodeClass with mixed static and selector
- **WHEN** an AWSNodeClass is created with `ami` (static), `subnetSelector` (dynamic), `securityGroupIds` (static), and `role` (dynamic)
- **THEN** the resource SHALL be accepted and stored

#### Scenario: AWSNodeClass with both static and selector for same resource
- **WHEN** an AWSNodeClass is created with both `ami` and `amiSelector`
- **THEN** the resource SHALL be rejected with validation error "ami and amiSelector are mutually exclusive"

#### Scenario: AWSNodeClass with neither static nor selector
- **WHEN** an AWSNodeClass is created without `ami` and without `amiSelector`
- **THEN** the resource SHALL be rejected with validation error "one of ami or amiSelector is required"

#### Scenario: AWSNodeClass missing required field
- **WHEN** an AWSNodeClass is created without `instanceType`
- **THEN** the resource SHALL be rejected with validation error "instanceType is required"

#### Scenario: AWSNodeClass with both role and iamInstanceProfile
- **WHEN** an AWSNodeClass is created with both `role` and `iamInstanceProfile`
- **THEN** the resource SHALL be rejected with validation error "role and iamInstanceProfile are mutually exclusive"
