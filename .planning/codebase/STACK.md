# Technology Stack

**Analysis Date:** 2026-02-02

## Languages

**Primary:**
- Go 1.25.5 - Entire controller and cloud provider implementations

## Runtime

**Environment:**
- Kubernetes 1.28.0+ (tested with envtest; uses controller-runtime)
- Linux/Unix (Docker image: golang:1.25-alpine builder, distroless/static:nonroot runtime)

**Package Manager:**
- Go modules (go mod)
- Lockfile: go.sum (present)

## Frameworks

**Core:**
- `sigs.k8s.io/controller-runtime` v0.23.1 - Kubernetes controller framework (kubebuilder-based)
- `k8s.io/api` v0.35.0 - Kubernetes API types
- `k8s.io/client-go` v0.35.0 - Kubernetes client library
- `k8s.io/apimachinery` v0.35.0 - Kubernetes API machinery

**Testing:**
- `github.com/onsi/ginkgo/v2` v2.27.5 - BDD-style test framework
- `github.com/onsi/gomega` v1.39.0 - Assertion/matcher library
- `github.com/stretchr/testify` v1.11.1 - Additional testing utilities
- `github.com/testcontainers/testcontainers-go` v0.40.0 - Container-based testing
- `github.com/testcontainers/testcontainers-go/modules/localstack` v0.40.0 - LocalStack for AWS mocking
- `sigs.k8s.io/controller-runtime/tools/setup-envtest` - Kubernetes envtest for integration testing

**Build/Dev:**
- `sigs.k8s.io/controller-tools` v0.16.5 (controller-gen) - CRD/RBAC manifest generation
- `github.com/golangci/golangci-lint` v2.8.0 - Linting
- Makefile for task automation

## Key Dependencies

**Critical:**
- `github.com/aws/aws-sdk-go-v2` v1.41.1 - AWS SDK v2 base
  - `github.com/aws/aws-sdk-go-v2/service/ec2` v1.284.0 - EC2 instance operations (launch, start, stop, terminate)
  - `github.com/aws/aws-sdk-go-v2/service/iam` v1.53.2 - IAM for instance profiles/roles
  - `github.com/aws/aws-sdk-go-v2/config` v1.32.7 - AWS credential/region loading
  - `github.com/aws/aws-sdk-go-v2/credentials` v1.19.7 - AWS credential chain
- `github.com/aws/smithy-go` v1.24.0 - AWS smithy protocol library (error handling)

**Observability:**
- `github.com/prometheus/client_golang` v1.23.2 - Prometheus metrics (registered via `internal/metrics`)
- `github.com/go-logr/logr` v1.4.3 - Structured logging interface (transitive)
- `go.uber.org/zap` v1.27.0 - Zap logger (used via controller-runtime's zap integration)

**Configuration:**
- `github.com/joho/godotenv` v1.5.1 - .env file loading (`internal/config/config.go` calls godotenv.Load)

**Transitive/Supporting:**
- `k8s.io/klog/v2` v2.130.1 - Kubernetes logging
- `k8s.io/utils` v0.0.0-20251002143259-bc988d571ff4 - Kubernetes utilities
- `go.opentelemetry.io/otel` v1.36.0 - OpenTelemetry tracing (optional instrumentation)

## Configuration

**Environment:**
- Configuration loaded from `.env` file via `github.com/joho/godotenv`
- Environment variables with `STRATOS_` prefix (primary), legacy non-prefixed vars (fallback)
- CLI flags override environment variables
- Defaults defined in `internal/config/config.go` and applied in `cmd/stratos/main.go`

**Key configs required (from `.env.example`):**
- `STRATOS_CLUSTER_NAME` - Kubernetes cluster identifier (required for AWS provider)
- `STRATOS_CLUSTER_ENDPOINT` - Kubernetes API server URL
- `STRATOS_CLUSTER_CA` - Base64-encoded CA certificate
- `STRATOS_CLUSTER_CIDR` - Service CIDR (e.g., 172.20.0.0/16)
- `STRATOS_CLOUD_PROVIDER` - "aws" or "fake"
- `STRATOS_SYNC_PERIOD` - Reconciliation interval (default: 30s)
- `STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT` - Shutdown grace period (default: 30s)
- `STRATOS_METRICS_BIND_ADDRESS` - Prometheus metrics endpoint (default: :8080)
- `STRATOS_HEALTH_PROBE_BIND_ADDRESS` - Health probes (default: :8081)

**Build:**
- `Makefile` - Build targets: `make build`, `make test`, `make test-integration`, `make docker-build`
- Dockerfile (multi-stage) - Alpine builder, distroless runtime
- Build flags for version/commit/date injection (ldflags in Makefile)

## Platform Requirements

**Development:**
- Go 1.25.5+
- Docker (for LocalStack integration tests)
- kubectl (for cluster operations)
- make (for build targets)
- Kubernetes 1.28+ cluster (for E2E tests; integration tests use envtest)

**Production:**
- Kubernetes 1.28+ cluster (EKS, on-prem)
- AWS credentials (IAM role or explicit keys) for AWS provider
- IAM permissions for EC2 (RunInstances, StartInstances, StopInstances, TerminateInstances, DescribeInstances, CreateTags)
- Network access to EC2 API endpoint

**Container/Deployment:**
- Docker/containerd runtime
- Helm 3+ (for chart deployment)
- Linux kernel (distroless image is nonroot, runs as UID 65532)

---

*Stack analysis: 2026-02-02*
