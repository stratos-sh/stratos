## Why

The controller currently supports environment variables for only a subset of CLI flags (CLUSTER_NAME, CLUSTER_ENDPOINT, CLUSTER_CA, CLUSTER_CIDR). Users expect consistent configuration options across all flags, and the ability to use `.env` files for local development and testing workflows.

## What Changes

- Add environment variable support for ALL CLI flags with a consistent naming convention (STRATOS_ prefix)
- Integrate godotenv to automatically load `.env` files when present
- Maintain CLI flag precedence over environment variables (existing behavior)
- Document all configuration options with their flag and env var equivalents

## Capabilities

### New Capabilities
- `env-config`: Environment variable configuration support for all CLI flags, including .env file loading via godotenv

### Modified Capabilities

## Impact

- **Code**: `cmd/stratos/main.go` - add godotenv integration, refactor flag/env handling
- **Dependencies**: Add `github.com/joho/godotenv` dependency
- **Helm chart**: Update `values.yaml` and deployment template to document env var options
- **Documentation**: Update CLAUDE.md with environment variable reference
