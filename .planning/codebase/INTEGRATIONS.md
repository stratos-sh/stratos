# External Integrations

**Analysis Date:** 2026-02-02

## APIs & External Services

**AWS EC2:**
- AWS EC2 API - Instance lifecycle operations (launch, start, stop, terminate, describe)
  - SDK/Client: `github.com/aws/aws-sdk-go-v2/service/ec2`
  - Location: `internal/cloudprovider/aws/provider.go`
  - Auth: IAM credentials (loaded via SDK default credential chain)
  - Operations: RunInstances, StartInstances, StopInstances, TerminateInstances, DescribeInstances, CreateTags
  - Rate Limiting: Custom implementation in `internal/cloudprovider/aws/ratelimit.go` with configurable QPS/burst multipliers

**AWS IAM:**
- AWS IAM API - Instance profile/role resolution for node bootstrap
  - SDK/Client: `github.com/aws/aws-sdk-go-v2/service/iam`
  - Location: `internal/cloudprovider/aws/provider.go` (AWSResolver)
  - Auth: Same as EC2 (AWS credential chain)

## Data Storage

**Databases:**
- Kubernetes etcd (via controller-runtime client)
  - Connection: In-cluster kubeconfig or explicit endpoint (STRATOS_CLUSTER_ENDPOINT)
  - Client: `k8s.io/client-go` (typed client via controller-runtime)
  - Stored Objects: NodePool (v1alpha1), AWSNodeClass (v1alpha1), Node (core/v1)

**File Storage:**
- Local filesystem only (no S3, GCS, etc.)
- Node userData scripts generated in-memory and passed to EC2 RunInstances API

**Caching:**
- None (all queries go through Kubernetes API server)

## Authentication & Identity

**Auth Provider:**
- Kubernetes RBAC (built-in)
  - Implementation: Service account tokens, IRSA (IAM Roles for Service Accounts) for AWS credentials
  - AWS SDK credential chain (default behavior: env vars → instance metadata → credential file)
  - Example IRSA setup in `deploy/charts/stratos/templates/iam-policy.yaml` and serviceaccount annotations

**Service Account:**
- Location: `deploy/charts/stratos/templates/serviceaccount.yaml`
- RBAC: ClusterRole in `deploy/charts/stratos/templates/clusterrole.yaml`
- IRSA: Configured via `serviceAccount.annotations` (e.g., `eks.amazonaws.com/role-arn`)

## Monitoring & Observability

**Error Tracking:**
- None detected (errors logged via structured logging only)

**Logs:**
- Structured logging via `github.com/go-logr/logr` interface
- Logger implementation: Zap (`go.uber.org/zap`) configured in `cmd/stratos/main.go`
- Development mode enabled (verbose output)
- Example: `setupLog.Info("starting stratos controller", "version", version, "commit", commit)`

**Metrics:**
- Prometheus metrics via `github.com/prometheus/client_golang`
- Namespace: `stratos`, subsystem: `nodepool`
- Registered in `internal/metrics/metrics.go`
- Metrics endpoint: `:8080` (configurable via STRATOS_METRICS_BIND_ADDRESS)
- Key metrics:
  - `stratos_nodepool_nodes_total` - Gauge by pool and state
  - `stratos_nodepool_starting_nodes` - In-flight scale-ups
  - `stratos_nodepool_desired_standby` - minStandby target
  - `stratos_nodepool_pool_size` - Pool size limit
  - `stratos_nodepool_scaleup_total` - Scale-up operations counter
  - `stratos_nodepool_scaledown_total` - Scale-down operations counter
  - `stratos_nodepool_warmup_failures_total` - Warmup failures counter
  - Cloud provider call latency metrics (by provider, operation, status)

**Health Probes:**
- Liveness and readiness probes via controller-runtime
- Probe endpoint: `:8081` (configurable via STRATOS_HEALTH_PROBE_BIND_ADDRESS)

## CI/CD & Deployment

**Hosting:**
- Kubernetes-native (as operator/controller)
- Deployment via Helm (chart at `deploy/charts/stratos`)
- Docker image: `ghcr.io/stratos-sh/stratos:${VERSION}`
- Build output: Multi-stage Dockerfile (Alpine builder → distroless runtime)

**CI Pipeline:**
- GitHub Actions (`.github/workflows/*`)
- Makefile targets: lint, test, test-integration, test-e2e, docker-build, docker-push

**Deployment Method:**
- Helm upgrade/install to Kubernetes cluster
- Example: `helm upgrade --install stratos deploy/charts/stratos --namespace stratos-system --create-namespace`

## Environment Configuration

**Required env vars:**
- `STRATOS_CLUSTER_NAME` - Cluster identifier (for AWS tagging and bootstrap)
- `STRATOS_CLUSTER_ENDPOINT` - Kubernetes API server URL (required if cluster-ca is set)
- `STRATOS_CLUSTER_CA` - Base64-encoded certificate authority
- `STRATOS_CLUSTER_CIDR` - Service CIDR for node bootstrap

**Optional env vars:**
- `STRATOS_CLOUD_PROVIDER` - "aws" (default) or "fake"
- `STRATOS_SYNC_PERIOD` - Reconciliation interval (default: 30s)
- `STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT` - Shutdown grace period (default: 30s)
- `STRATOS_METRICS_BIND_ADDRESS` - Prometheus metrics endpoint (default: :8080)
- `STRATOS_HEALTH_PROBE_BIND_ADDRESS` - Health probes (default: :8081)
- `STRATOS_LEADER_ELECT` - Enable leader election for HA (default: false)
- `STRATOS_AWS_RATE_LIMIT_QPS` - AWS API rate limit QPS multiplier (default: 1.0)
- `STRATOS_AWS_RATE_LIMIT_BURST` - AWS API rate limit burst multiplier (default: 1.0)

**Secrets location:**
- AWS credentials: IAM instance role (via IMDS) or explicit AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY env vars
- Kubernetes API CA: STRATOS_CLUSTER_CA (base64-encoded, typically from EKS CA certificate)
- Service account token: Mounted automatically by Kubernetes

**Legacy env vars (backward compatibility):**
- `CLUSTER_NAME` → `STRATOS_CLUSTER_NAME`
- `CLUSTER_ENDPOINT` → `STRATOS_CLUSTER_ENDPOINT`
- `CLUSTER_CA` → `STRATOS_CLUSTER_CA`
- `CLUSTER_CIDR` → `STRATOS_CLUSTER_CIDR`

## Webhooks & Callbacks

**Incoming:**
- None detected (controller is event-driven via Kubernetes watch)
- Reconciliation triggered by NodePool/Node/Pod changes in Kubernetes API

**Outgoing:**
- None (no external webhooks or callbacks)

## Testing Integrations

**LocalStack (AWS Mocking):**
- Framework: `github.com/testcontainers/testcontainers-go/modules/localstack`
- Used in: `tests/integration/*` and `internal/cloudprovider/aws/*`
- Docker image: localstack/localstack (pulled at test runtime)
- Purpose: Mock AWS EC2/IAM APIs without hitting real AWS

**Kubernetes envtest:**
- Framework: `sigs.k8s.io/controller-runtime/tools/setup-envtest`
- Provides: In-memory Kubernetes API server, etcd (no real cluster needed)
- Used in: All integration tests via `make test-integration`
- K8s version: 1.28.0 (set in Makefile as ENVTEST_K8S_VERSION)

**Fake Cloud Provider:**
- Implementation: `internal/cloudprovider/fake/provider.go`
- Purpose: Mock cloud provider for local testing without AWS/LocalStack
- Features: Hooks for intercepting operations, test data injection

---

*Integration audit: 2026-02-02*
