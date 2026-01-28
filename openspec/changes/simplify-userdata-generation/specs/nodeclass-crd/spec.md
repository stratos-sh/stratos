## MODIFIED Requirements

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

**REMOVED fields:**
- `userData`: Replaced by automatic generation based on `bootstrapTemplate`
- `ami`: Use `amiSelector` instead for explicit AMI selection

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

## REMOVED Requirements

### Requirement: userData field in AWSNodeClass

**Reason:** Replaced by automatic userData generation based on `bootstrapTemplate`. Users no longer need to construct complex MIME multipart or TOML configurations manually.

**Migration:** Remove `userData` field and add `bootstrapTemplate` field. Use `kubelet` for kubelet customizations and `customUserData` for additional scripts.

### Requirement: ami field in AWSNodeClass

**Reason:** Consolidated into `amiSelector` for consistency. Auto-discovery is now the default when `amiSelector` is omitted.

**Migration:** Replace `ami: ami-12345` with `amiSelector: {name: ami-12345}` or omit for auto-discovery.
