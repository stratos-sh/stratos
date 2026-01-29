## Context

The controller currently uses Go's `flag` package for CLI configuration. Environment variable support exists for only 4 flags (CLUSTER_NAME, CLUSTER_ENDPOINT, CLUSTER_CA, CLUSTER_CIDR) via manual `os.Getenv` calls after flag parsing. This is inconsistent and doesn't support `.env` files for local development.

## Goals / Non-Goals

**Goals:**
- Support environment variables for ALL CLI flags
- Use consistent STRATOS_ prefix for all env vars (e.g., STRATOS_METRICS_BIND_ADDRESS)
- Automatically load `.env` file when present via godotenv
- Maintain CLI flag precedence over environment variables
- Zero behavioral change for existing users

**Non-Goals:**
- Configuration file support (YAML/TOML) - out of scope
- Dynamic configuration reloading - out of scope
- Validation beyond what flags already provide - out of scope

## Decisions

### 1. Environment Variable Naming Convention

**Decision**: Use `STRATOS_` prefix with uppercase snake_case flag names.

| Flag | Environment Variable |
|------|---------------------|
| `--metrics-bind-address` | `STRATOS_METRICS_BIND_ADDRESS` |
| `--health-probe-bind-address` | `STRATOS_HEALTH_PROBE_BIND_ADDRESS` |
| `--leader-elect` | `STRATOS_LEADER_ELECT` |
| `--sync-period` | `STRATOS_SYNC_PERIOD` |
| `--cluster-name` | `STRATOS_CLUSTER_NAME` |
| `--cluster-endpoint` | `STRATOS_CLUSTER_ENDPOINT` |
| `--cluster-ca` | `STRATOS_CLUSTER_CA` |
| `--cluster-cidr` | `STRATOS_CLUSTER_CIDR` |
| `--cloud-provider` | `STRATOS_CLOUD_PROVIDER` |
| `--graceful-shutdown-timeout` | `STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT` |

**Rationale**: Prefix prevents collisions with other tools. Snake_case is standard for env vars.

**Alternatives considered**:
- No prefix: Risk of collision with system/other tool env vars
- Short prefix (ST_): Less clear, harder to grep for in configs

### 2. Backward Compatibility for Existing Env Vars

**Decision**: Keep existing non-prefixed env vars (CLUSTER_NAME, CLUSTER_ENDPOINT, CLUSTER_CA, CLUSTER_CIDR) as aliases.

**Rationale**: Avoid breaking existing deployments. Prefixed versions take precedence if both are set.

### 3. godotenv Integration

**Decision**: Call `godotenv.Load()` at program start, ignore "file not found" errors.

**Rationale**:
- Automatic `.env` loading simplifies local development
- Silent failure when no `.env` exists is the standard godotenv behavior
- Users can place `.env` in the working directory

### 4. Precedence Order

**Decision**: CLI flags > Environment variables > Default values

**Rationale**: This is the standard precedence most users expect. CLI flags allow overriding any config.

## Risks / Trade-offs

**[Risk] Existing CLUSTER_* env vars may conflict with new STRATOS_CLUSTER_* vars**
→ Mitigation: Document that STRATOS_ prefixed vars take precedence. Log a warning if both are set.

**[Risk] .env file loading could expose secrets in logs**
→ Mitigation: godotenv doesn't log values. Don't log the loaded env vars.

**[Trade-off] Adding dependency on godotenv**
→ godotenv is a small, well-maintained library with no transitive dependencies. Worth the convenience.
