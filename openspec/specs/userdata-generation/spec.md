## Requirements

### Requirement: Bootstrap generator produces userData based on bootstrapTemplate

The controller SHALL generate EC2 userData automatically based on the `bootstrapTemplate` field in AWSNodeClass.

Supported templates:
- `AL2023`: Generates MIME multipart with `application/node.eks.aws` nodeadm config
- `AL2`: Generates MIME multipart with shell script calling `/etc/eks/bootstrap.sh`
- `Bottlerocket`: Generates TOML configuration with `[settings.kubernetes]` block

#### Scenario: Generate AL2023 userData
- **WHEN** an AWSNodeClass has `bootstrapTemplate: AL2023`
- **THEN** the controller SHALL generate MIME multipart userData containing:
  - Part 1: `Content-Type: application/node.eks.aws` with nodeadm NodeConfig YAML
  - Part 2: `Content-Type: text/x-shellscript` with Stratos warmup script

#### Scenario: Generate Bottlerocket userData
- **WHEN** an AWSNodeClass has `bootstrapTemplate: Bottlerocket`
- **THEN** the controller SHALL generate TOML userData containing:
  - `[settings.kubernetes]` block with cluster configuration
  - `[settings.kubernetes.node-labels]` block with Stratos pool label
- **AND** warmup SHALL be handled by ControllerStop mode (not via userData script)

#### Scenario: Generate AL2 userData
- **WHEN** an AWSNodeClass has `bootstrapTemplate: AL2`
- **THEN** the controller SHALL generate MIME multipart userData containing:
  - Part 1: Shell script calling `/etc/eks/bootstrap.sh` with cluster arguments
  - Part 2: Stratos warmup script

### Requirement: Generated userData includes cluster configuration

The generated userData SHALL include cluster configuration from controller settings.

The cluster configuration SHALL include:
- `name`: Cluster name
- `apiServerEndpoint`: Kubernetes API server URL
- `certificateAuthority`: Base64-encoded CA certificate
- `cidr`: Cluster service CIDR

#### Scenario: Cluster config embedded in AL2023 nodeadm config
- **WHEN** generating userData for `bootstrapTemplate: AL2023`
- **THEN** the NodeConfig YAML SHALL contain `spec.cluster.name`, `spec.cluster.apiServerEndpoint`, `spec.cluster.certificateAuthority`, and `spec.cluster.cidr`

#### Scenario: Cluster config embedded in Bottlerocket TOML
- **WHEN** generating userData for `bootstrapTemplate: Bottlerocket`
- **THEN** the TOML SHALL contain `[settings.kubernetes]` with `cluster-name`, `api-server`, `cluster-certificate`, and `cluster-dns-ip`

### Requirement: Generated userData includes kubelet configuration

When `kubelet` is specified in AWSNodeClass, the generated userData SHALL include kubelet settings.

#### Scenario: Kubelet maxPods in AL2023
- **WHEN** an AWSNodeClass has `kubelet.maxPods: 110` and `bootstrapTemplate: AL2023`
- **THEN** the NodeConfig YAML SHALL contain `spec.kubelet.config.maxPods: 110`

#### Scenario: Kubelet nodeLabels in AL2023
- **WHEN** an AWSNodeClass has `kubelet.nodeLabels: {team: platform}` and `bootstrapTemplate: AL2023`
- **THEN** the NodeConfig YAML SHALL contain `spec.kubelet.flags` with `--node-labels=team=platform`

#### Scenario: Kubelet config in Bottlerocket
- **WHEN** an AWSNodeClass has `kubelet.maxPods: 110` and `bootstrapTemplate: Bottlerocket`
- **THEN** the TOML SHALL contain `[settings.kubernetes]` with `max-pods = 110`

### Requirement: Stratos warmup script is included for AL2023/AL2

The generated userData for AL2023 and AL2 SHALL include the Stratos warmup script that:
1. Waits for kubelet to become healthy
2. Initializes EBS volume (reads all blocks)

The script SHALL NOT call poweroff - the controller handles stopping via ControllerStop mode.

#### Scenario: Warmup script in AL2023
- **WHEN** generating userData for `bootstrapTemplate: AL2023`
- **THEN** the MIME multipart SHALL include a shell script part that waits for kubelet and initializes EBS
- **AND** the script SHALL NOT call poweroff

#### Scenario: Warmup script in AL2
- **WHEN** generating userData for `bootstrapTemplate: AL2`
- **THEN** the MIME multipart SHALL include a shell script part that waits for kubelet and initializes EBS
- **AND** the script SHALL NOT call poweroff

#### Scenario: Bottlerocket has no warmup script
- **WHEN** generating userData for `bootstrapTemplate: Bottlerocket`
- **THEN** the TOML SHALL NOT include warmup scripts
- **AND** the controller SHALL stop the instance when node becomes Ready

### Requirement: customUserData is merged with generated userData

When `customUserData` is specified in AWSNodeClass, it SHALL be merged with the generated userData.

For AL2023/AL2: Added as additional MIME part after warmup script.
For Bottlerocket: Added as additional bootstrap container.

#### Scenario: Custom script merged in AL2023
- **WHEN** an AWSNodeClass has `customUserData` with a shell script and `bootstrapTemplate: AL2023`
- **THEN** the MIME multipart SHALL include the custom script as the final part

#### Scenario: Custom config merged in Bottlerocket
- **WHEN** an AWSNodeClass has `customUserData` with TOML content and `bootstrapTemplate: Bottlerocket`
- **THEN** the custom TOML SHALL be merged with the generated TOML configuration

#### Scenario: No customUserData provided
- **WHEN** an AWSNodeClass has no `customUserData` field
- **THEN** the generated userData SHALL only contain bootstrap config and warmup script

### Requirement: Invalid bootstrapTemplate is rejected

The controller SHALL reject AWSNodeClass resources with invalid `bootstrapTemplate` values.

#### Scenario: Invalid bootstrapTemplate value
- **WHEN** an AWSNodeClass is created with `bootstrapTemplate: Windows`
- **THEN** the resource SHALL be rejected with validation error "bootstrapTemplate must be one of: AL2023, AL2, Bottlerocket"

#### Scenario: Missing bootstrapTemplate
- **WHEN** an AWSNodeClass is created without `bootstrapTemplate` field
- **THEN** the resource SHALL be rejected with validation error "bootstrapTemplate is required"
