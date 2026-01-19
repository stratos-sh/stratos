# Quickstart: Stratos Development

**Date**: 2026-01-19
**Branch**: `001-instance-pool-manager`

This guide helps developers get started with Stratos development quickly.

---

## Prerequisites

- Go 1.22+
- kubectl configured with cluster access
- Docker (for building images)
- Kind or Minikube (for local testing)
- AWS credentials (for EC2 testing)

## Project Setup

### 1. Initialize Go Module

```bash
# From repository root
go mod init github.com/stratos-sh/stratos
go mod tidy
```

### 2. Install Development Tools

```bash
# Install kubebuilder (for code generation)
curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x kubebuilder && mv kubebuilder /usr/local/bin/

# Install controller-gen
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# Install golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.55.2

# Install envtest binaries
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use
```

### 3. Generate CRD Types

After defining types in `api/v1alpha1/nodepool_types.go`:

```bash
# Generate deepcopy methods
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Generate CRD manifests
controller-gen crd paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate RBAC manifests (from kubebuilder markers)
controller-gen rbac:roleName=stratos-controller paths="./..." output:rbac:artifacts:config=config/rbac
```

## Development Workflow

### Run Locally (Against Cluster)

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Run controller locally
go run cmd/stratos/main.go --kubeconfig=$HOME/.kube/config
```

### Run Tests

```bash
# Unit tests
go test ./... -v

# Integration tests (requires envtest)
go test ./tests/integration/... -v -tags=integration

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Lint

```bash
golangci-lint run
```

## Key Commands

### Create a NodePool

```yaml
# config/samples/nodepool_sample.yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: worker-pool
spec:
  poolSize: 10
  minStandby: 3
  template:
    labels:
      node-type: worker
    cloudProvider:
      provider: aws
      aws:
        instanceType: m5.large
        ami: ami-0123456789abcdef0
        subnetIds:
          - subnet-abc123
        securityGroupIds:
          - sg-abc123
        iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/KubernetesNode
```

```bash
kubectl apply -f config/samples/nodepool_sample.yaml
```

### Check NodePool Status

```bash
# List pools
kubectl get nodepools

# Describe pool
kubectl describe nodepool worker-pool

# Watch pool status
kubectl get nodepool worker-pool -w
```

### Check Stratos-Managed Nodes

```bash
# List nodes managed by a pool
kubectl get nodes -l stratos.sh/pool=worker-pool

# Check node state
kubectl get nodes -l stratos.sh/pool=worker-pool -o custom-columns=NAME:.metadata.name,STATE:.metadata.labels.stratos\\.sh/state
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Stratos Controller                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌───────────────────┐    ┌───────────────────┐            │
│  │ NodePoolReconciler│    │   PodWatcher      │            │
│  │                   │    │                   │            │
│  │ - Pool maintenance│    │ - Detect pending  │            │
│  │ - Replenish standby    │   pods            │            │
│  │ - Detect failures │    │ - Trigger scale-up│            │
│  └─────────┬─────────┘    └─────────┬─────────┘            │
│            │                        │                       │
│            └────────────┬───────────┘                       │
│                         │                                   │
│            ┌────────────▼────────────┐                      │
│            │    NodeManager          │                      │
│            │                         │                      │
│            │ - State transitions     │                      │
│            │ - Drain operations      │                      │
│            │ - Label management      │                      │
│            └────────────┬────────────┘                      │
│                         │                                   │
│            ┌────────────▼────────────┐                      │
│            │   CloudProvider         │                      │
│            │   (Interface)           │                      │
│            │                         │                      │
│            │ - Launch/Start/Stop     │                      │
│            │ - Get state             │                      │
│            │ - Terminate             │                      │
│            └─────────────────────────┘                      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Local Development with Kind

```bash
# Create kind cluster
kind create cluster --name stratos-dev

# Build and load image
docker build -t stratos:dev .
kind load docker-image stratos:dev --name stratos-dev

# Deploy
kubectl apply -k config/default
```

## Fake Cloud Provider for Testing

For local development without AWS access, use the fake cloud provider:

```go
// internal/cloudprovider/fake/provider.go
type FakeProvider struct {
    instances map[string]*cloudprovider.Instance
    mu        sync.RWMutex
}

func (f *FakeProvider) StartInstance(ctx context.Context, id string) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    if inst, ok := f.instances[id]; ok {
        inst.State = cloudprovider.InstanceStateRunning
        return nil
    }
    return &cloudprovider.InstanceNotFoundError{InstanceID: id}
}
```

Enable with flag:

```bash
go run cmd/stratos/main.go --cloud-provider=fake
```

## Debugging

### Enable Verbose Logging

```bash
go run cmd/stratos/main.go --zap-log-level=debug
```

### View Controller Metrics

```bash
# Port-forward to metrics endpoint
kubectl port-forward -n stratos-system deploy/stratos-controller 8080:8080

# View metrics
curl http://localhost:8080/metrics | grep stratos_
```

### Inspect Kubernetes Events

```bash
kubectl get events --field-selector involvedObject.kind=NodePool -w
```

## Common Issues

### "CRD not found"

```bash
# Regenerate and apply CRDs
controller-gen crd paths="./..." output:crd:artifacts:config=config/crd/bases
kubectl apply -f config/crd/bases/
```

### "Permission denied" on node operations

Check RBAC:

```bash
kubectl auth can-i create nodes --as=system:serviceaccount:stratos-system:stratos-controller
```

### envtest fails to start

```bash
# Reinstall envtest binaries
setup-envtest use --bin-dir=/usr/local/kubebuilder/bin
export KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin
```

## Next Steps

1. Read [data-model.md](./data-model.md) for entity definitions
2. Read [research.md](./research.md) for implementation patterns
3. Review contracts in [contracts/](./contracts/) for API schemas
