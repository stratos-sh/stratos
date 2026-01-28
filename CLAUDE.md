# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Stratos is a Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, stopped instances ready to start in seconds. Built on controller-runtime (kubebuilder pattern).

Run a single test:
```bash
go test -v -run TestSpecificName ./internal/controller/...
```

## Architecture

```
cmd/stratos/main.go           # Entry point, manager setup, flags
api/v1alpha1/                 # NodePool CRD types (kubebuilder markers for RBAC/CRD gen)
internal/
├── controller/               # Kubernetes reconcilers
│   ├── nodepool_controller.go    # Main reconciliation loop
│   ├── pod_watcher.go            # Detects pending pods for scale-up
│   ├── state.go                  # Node state machine with valid transitions
│   ├── scale_up.go               # Scale-up logic (standby → running)
│   ├── scale_down.go             # Scale-down logic (drain, stop)
│   ├── scale_calculator.go       # Pod-to-node capacity calculation
│   ├── pool_maintenance.go       # Pool maintenance operations
│   └── network_readiness.go      # CNI readiness detection
├── cloudprovider/            # Cloud abstraction layer
│   ├── interface.go              # CloudProvider interface (all cloud ops go through this)
│   ├── types.go                  # Cloud-agnostic types (LaunchConfig, Instance, errors)
│   ├── factory.go                # Provider factory (creates aws/fake based on config)
│   ├── aws/provider.go           # AWS EC2 implementation
│   ├── aws/ratelimit.go          # AWS API rate limiting
│   ├── aws/instance_types.go     # Instance type → capacity mapping
│   └── fake/provider.go          # Mock provider for testing (with hooks)
└── metrics/                  # Prometheus metrics
deploy/
├── charts/stratos/           # Helm chart
│   ├── Chart.yaml                # Chart metadata and version
│   ├── values.yaml               # Default configuration values
│   ├── crds/                     # Generated CRD manifests
│   └── templates/                # Kubernetes resource templates
└── samples/                  # Example NodePool/AWSNodeClass resources
```

**Key patterns:**
- Event-driven reconciliation with 30s periodic maintenance loop
- CloudProvider interface abstracts all instance operations (launch, start, stop, terminate)
- Use fake provider for local development: `--cloud-provider=fake`
- Node state tracked via labels: `stratos.sh/pool`, `stratos.sh/state`

### Node State Machine

```
warmup → standby → running → terminating
```

- **warmup**: Instance launched, running user data script, will self-stop when ready (10min timeout)
- **standby**: Instance stopped, ready for instant start (seconds vs minutes)
- **running**: Active node with pods scheduled
- **terminating**: Node draining (respects PodDisruptionBudgets), then stops

State transitions are defined in `internal/controller/state.go` with explicit validation.

### Labels & Tags

**Kubernetes Node Labels** (set by controller):
- `stratos.sh/pool`: Pool name managing this node
- `stratos.sh/state`: Current state (warmup/standby/running/terminating)
- `stratos.sh/instance-id`: Cloud instance ID
- `stratos.sh/state-since`: Timestamp when state changed

**Cloud Instance Tags** (for discovery and sync):
- `managed-by`: "stratos"
- `stratos.sh/pool`: Pool name
- `stratos.sh/cluster`: Cluster identifier
- `stratos.sh/state`: Current state

## Key Controller Flags

```bash
--cluster-name=<name>         # Required (or CLUSTER_NAME env var)
--cloud-provider=aws|fake     # Default: aws
--sync-period=30s             # Reconciliation interval
--metrics-bind-address=:8080
--health-probe-bind-address=:8081
```

## Environment Variables

All CLI flags can be configured via environment variables with the `STRATOS_` prefix. The controller also loads `.env` files automatically via godotenv.

**Precedence:** CLI flags > STRATOS_ env vars > legacy env vars > defaults

| Flag | Environment Variable | Legacy Alias | Default |
|------|---------------------|--------------|---------|
| `--metrics-bind-address` | `STRATOS_METRICS_BIND_ADDRESS` | - | `:8080` |
| `--health-probe-bind-address` | `STRATOS_HEALTH_PROBE_BIND_ADDRESS` | - | `:8081` |
| `--leader-elect` | `STRATOS_LEADER_ELECT` | - | `false` |
| `--sync-period` | `STRATOS_SYNC_PERIOD` | - | `30s` |
| `--cluster-name` | `STRATOS_CLUSTER_NAME` | `CLUSTER_NAME` | - |
| `--cluster-endpoint` | `STRATOS_CLUSTER_ENDPOINT` | `CLUSTER_ENDPOINT` | - |
| `--cluster-ca` | `STRATOS_CLUSTER_CA` | `CLUSTER_CA` | - |
| `--cluster-cidr` | `STRATOS_CLUSTER_CIDR` | `CLUSTER_CIDR` | - |
| `--cloud-provider` | `STRATOS_CLOUD_PROVIDER` | - | `aws` |
| `--graceful-shutdown-timeout` | `STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT` | - | `30s` |

## Running the Controller Locally

**IMPORTANT:** Always use `go run` to run the controller locally. Never build a separate binary (`/tmp/stratos`, etc.) as it makes it harder to track running processes.

```bash
# Run controller locally (standard way)
go run ./cmd/stratos/main.go --cluster-name=main --cloud-provider=aws

# Before starting, always check for and kill any existing controller
pkill -f "cmd/stratos/main.go" 2>/dev/null
ps aux | grep -E "main.*--cluster-name" | grep -v grep

# Check if controller is running (go run shows as 'main' in process list)
ps aux | grep -E "main.*--cluster-name" | grep -v grep
```

The controller process appears as `main --cluster-name=...` in the process list, not as `stratos`.

## Linting

golangci-lint configured with: errcheck, gosimple, govet (shadow, nilness), ineffassign, staticcheck, unused, gosec, gocyclo (min: 15), misspell. Test files excluded from gocyclo, errcheck, gosec.

## Testing

### Running Tests

```bash
# Unit tests
make test                     # Run all unit tests with coverage

# Integration tests (uses envtest with in-memory K8s API server)
make test-integration         # Run all integration tests

# Run a specific unit test
go test -v -run TestSpecificName ./internal/controller/...

# Run a specific integration test
make test-integration TEST=TestNodePoolLifecycle
```

**Important:** Always use `make test-integration` for integration tests. Running `go test` directly will fail because integration tests require kubebuilder binaries (etcd, kube-apiserver). The Makefile sets up `KUBEBUILDER_ASSETS` automatically.

### Test Patterns

- **Unit tests**: Standard Go tests in `*_test.go` files alongside source
- **Integration tests**: Use `envtest` (in-memory K8s API server) in `tests/integration/`
- **Fake provider**: `internal/cloudprovider/fake/provider.go` supports hooks for intercepting cloud operations
- **BDD style**: Integration tests use Ginkgo/Gomega for readable assertions

## OpenSpec Workflow

After completing an OpenSpec implementation (`/opsx:apply` finishes all tasks), always run the full test suite (unit + integration) before marking the work as complete. Use `/run-tests` to execute all tests.

## Context7 MCP

Always use Context7 MCP proactively for third-party library documentation (controller-runtime, AWS SDK, client-go, etc.) without waiting for explicit instruction.
