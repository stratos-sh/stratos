## ADDED Requirements

### Requirement: Cluster configuration provided via Helm values

The Stratos Helm chart SHALL accept cluster configuration values used for userData generation.

The cluster configuration SHALL include:
- `cluster.name`: Kubernetes cluster name (existing, now used for userData)
- `cluster.apiServerEndpoint`: Kubernetes API server URL (required)
- `cluster.certificateAuthority`: Base64-encoded CA certificate (required)
- `cluster.cidr`: Cluster service CIDR (required)

#### Scenario: Valid cluster configuration
- **WHEN** Stratos is installed with all cluster configuration values
- **THEN** the controller SHALL start successfully and use the values for userData generation

#### Scenario: Missing apiServerEndpoint
- **WHEN** Stratos is installed without `cluster.apiServerEndpoint`
- **THEN** the controller SHALL fail to start with error "cluster.apiServerEndpoint is required"

#### Scenario: Missing certificateAuthority
- **WHEN** Stratos is installed without `cluster.certificateAuthority`
- **THEN** the controller SHALL fail to start with error "cluster.certificateAuthority is required"

#### Scenario: Missing cidr
- **WHEN** Stratos is installed without `cluster.cidr`
- **THEN** the controller SHALL fail to start with error "cluster.cidr is required"

### Requirement: Controller receives cluster config as flags or environment variables

The controller SHALL receive cluster configuration via command-line flags or environment variables.

Flags:
- `--cluster-name`: Cluster name
- `--cluster-endpoint`: API server endpoint
- `--cluster-ca`: Base64-encoded CA certificate
- `--cluster-cidr`: Service CIDR

Environment variables (alternative):
- `CLUSTER_NAME`
- `CLUSTER_ENDPOINT`
- `CLUSTER_CA`
- `CLUSTER_CIDR`

#### Scenario: Configuration via flags
- **WHEN** the controller is started with `--cluster-endpoint=https://... --cluster-ca=LS0t... --cluster-cidr=172.20.0.0/16`
- **THEN** the controller SHALL use these values for userData generation

#### Scenario: Configuration via environment variables
- **WHEN** the controller is started with `CLUSTER_ENDPOINT`, `CLUSTER_CA`, and `CLUSTER_CIDR` environment variables
- **THEN** the controller SHALL use these values for userData generation

#### Scenario: Flags take precedence over environment variables
- **WHEN** both `--cluster-endpoint` flag and `CLUSTER_ENDPOINT` env var are set
- **THEN** the controller SHALL use the flag value

### Requirement: Cluster configuration is validated at startup

The controller SHALL validate cluster configuration at startup and fail fast if invalid.

#### Scenario: Invalid apiServerEndpoint URL
- **WHEN** the controller is started with `--cluster-endpoint=not-a-url`
- **THEN** the controller SHALL fail to start with error "cluster-endpoint must be a valid HTTPS URL"

#### Scenario: Invalid certificateAuthority encoding
- **WHEN** the controller is started with `--cluster-ca=not-base64`
- **THEN** the controller SHALL fail to start with error "cluster-ca must be valid base64"

#### Scenario: Invalid CIDR format
- **WHEN** the controller is started with `--cluster-cidr=invalid`
- **THEN** the controller SHALL fail to start with error "cluster-cidr must be a valid CIDR notation"
