## ADDED Requirements

### Requirement: AMI auto-discovery when amiSelector is omitted

When `amiSelector` is not specified in AWSNodeClass, the controller SHALL automatically discover the appropriate AMI based on `bootstrapTemplate` and cluster Kubernetes version.

#### Scenario: Auto-discover AL2023 AMI
- **WHEN** an AWSNodeClass has `bootstrapTemplate: AL2023` and no `amiSelector`
- **THEN** the controller SHALL resolve AMI using selector `name: amazon-eks-node-al2023-x86_64-standard-<k8s-version>-*` with `owner: amazon`

#### Scenario: Auto-discover Bottlerocket AMI
- **WHEN** an AWSNodeClass has `bootstrapTemplate: Bottlerocket` and no `amiSelector`
- **THEN** the controller SHALL resolve AMI using selector `name: bottlerocket-aws-k8s-<k8s-version>-x86_64-*` with `owner: amazon`

#### Scenario: Auto-discover AL2 AMI
- **WHEN** an AWSNodeClass has `bootstrapTemplate: AL2` and no `amiSelector`
- **THEN** the controller SHALL resolve AMI using selector `name: amazon-eks-node-<k8s-version>-x86_64-*` with `owner: amazon`

#### Scenario: Explicit amiSelector takes precedence
- **WHEN** an AWSNodeClass has both `bootstrapTemplate: AL2023` and `amiSelector: {name: my-custom-*}`
- **THEN** the controller SHALL use the explicit `amiSelector` instead of auto-discovery

### Requirement: Kubernetes version is detected at controller startup

The controller SHALL detect the cluster Kubernetes version at startup for AMI auto-discovery.

#### Scenario: Version detection from API server
- **WHEN** the controller starts
- **THEN** it SHALL query the Kubernetes API server version endpoint
- **AND** extract the major.minor version (e.g., "1.34" from "v1.34.2")

#### Scenario: Version detection failure
- **WHEN** the controller cannot detect the Kubernetes version
- **AND** an AWSNodeClass has no `amiSelector`
- **THEN** the controller SHALL set condition `AMIReady=False` with reason "VersionDetectionFailed"
- **AND** emit a Warning event "Cannot auto-discover AMI: Kubernetes version detection failed. Specify amiSelector explicitly."

### Requirement: Architecture-aware AMI discovery

The controller SHALL use the `architecture` field when auto-discovering AMIs.

#### Scenario: ARM64 architecture specified
- **WHEN** an AWSNodeClass has `architecture: arm64` and `bootstrapTemplate: AL2023` with no `amiSelector`
- **THEN** the controller SHALL resolve AMI using selector `name: amazon-eks-node-al2023-arm64-standard-<k8s-version>-*`

#### Scenario: x86_64 architecture specified
- **WHEN** an AWSNodeClass has `architecture: x86_64` and `bootstrapTemplate: AL2023` with no `amiSelector`
- **THEN** the controller SHALL resolve AMI using selector `name: amazon-eks-node-al2023-x86_64-standard-<k8s-version>-*`

#### Scenario: Architecture defaults to x86_64
- **WHEN** an AWSNodeClass has no `architecture` field
- **THEN** the controller SHALL default to `x86_64` for AMI auto-discovery

### Requirement: AMI auto-discovery selects latest matching AMI

When multiple AMIs match the auto-discovery selector, the controller SHALL select the most recent one.

#### Scenario: Multiple matching AMIs
- **WHEN** auto-discovery finds AMIs `amazon-eks-node-al2023-x86_64-standard-1.34-v20260115` and `amazon-eks-node-al2023-x86_64-standard-1.34-v20260120`
- **THEN** the controller SHALL select `v20260120` (the most recent)
- **AND** set `status.resolvedAMI` to the selected AMI ID
