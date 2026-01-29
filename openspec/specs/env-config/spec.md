## Requirements

### Requirement: Environment variable configuration
The controller SHALL support configuration via environment variables for all CLI flags using the `STRATOS_` prefix with uppercase snake_case names (e.g., `--sync-period` → `STRATOS_SYNC_PERIOD`).

#### Scenario: Environment variable configures flag
- **WHEN** `STRATOS_SYNC_PERIOD=60s` is set and no `--sync-period` flag is provided
- **THEN** the controller uses 60s as the sync period

#### Scenario: CLI flag takes precedence over environment variable
- **WHEN** `STRATOS_SYNC_PERIOD=60s` is set AND `--sync-period=30s` is provided
- **THEN** the controller uses 30s as the sync period (CLI takes precedence)

### Requirement: Backward compatible environment variables
The controller SHALL support the existing non-prefixed environment variables (CLUSTER_NAME, CLUSTER_ENDPOINT, CLUSTER_CA, CLUSTER_CIDR) as aliases for backward compatibility.

#### Scenario: Legacy env var still works
- **WHEN** `CLUSTER_NAME=my-cluster` is set (without STRATOS_ prefix)
- **THEN** the controller uses "my-cluster" as the cluster name

#### Scenario: Prefixed env var takes precedence over legacy
- **WHEN** both `CLUSTER_NAME=old-name` and `STRATOS_CLUSTER_NAME=new-name` are set
- **THEN** the controller uses "new-name" as the cluster name (prefixed takes precedence)

### Requirement: Dotenv file loading
The controller SHALL automatically load environment variables from a `.env` file in the working directory at startup using godotenv, if the file exists.

#### Scenario: .env file loaded at startup
- **WHEN** a `.env` file exists in the working directory with `STRATOS_CLOUD_PROVIDER=fake`
- **THEN** the controller uses "fake" as the cloud provider (if no CLI flag overrides)

#### Scenario: Missing .env file is ignored
- **WHEN** no `.env` file exists in the working directory
- **THEN** the controller starts normally without error

### Requirement: Complete flag coverage
The controller SHALL support environment variables for all CLI flags including: metrics-bind-address, health-probe-bind-address, leader-elect, sync-period, cluster-name, cluster-endpoint, cluster-ca, cluster-cidr, cloud-provider, and graceful-shutdown-timeout.

#### Scenario: Boolean flag via environment variable
- **WHEN** `STRATOS_LEADER_ELECT=true` is set
- **THEN** the controller enables leader election

#### Scenario: Duration flag via environment variable
- **WHEN** `STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT=60s` is set
- **THEN** the controller uses 60s as the graceful shutdown timeout
