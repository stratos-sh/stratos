---
sidebar_position: 2
title: Local Development
description: Running Stratos locally for development
---

# Local Development

This guide covers how to run Stratos locally for development and testing.

## Prerequisites

- Go 1.21 or later
- kubectl configured with a Kubernetes cluster
- AWS credentials (for AWS provider) or use fake provider

## Running Locally

### Using the Fake Provider

The fake provider allows development without cloud costs:

```bash
go run ./cmd/stratos/main.go \
  --cluster-name=dev \
  --cloud-provider=fake \
  --zap-log-level=debug
```

The fake provider:
- Simulates instance lifecycle in memory
- Supports hooks for testing
- Instant operations (no cloud latency)

### Using the AWS Provider

To test with real AWS resources:

```bash
# Ensure AWS credentials are configured
aws sts get-caller-identity

# Run controller
go run ./cmd/stratos/main.go \
  --cluster-name=dev \
  --cloud-provider=aws \
  --zap-log-level=debug
```

:::warning
Running with the AWS provider will create real EC2 instances. Ensure you have appropriate permissions and monitor costs.
:::

## Process Management

### Starting the Controller

```bash
# Standard way to run locally
go run ./cmd/stratos/main.go --cluster-name=dev --cloud-provider=fake
```

### Checking if Running

The controller process appears as `main` in the process list:

```bash
ps aux | grep -E "main.*--cluster-name" | grep -v grep
```

### Stopping the Controller

```bash
# Kill any existing controller
pkill -f "cmd/stratos/main.go"
```

:::tip
Always check for and kill existing controller processes before starting a new one to avoid conflicts.
:::

## Development Cycle

### 1. Make Code Changes

Edit files in:
- `cmd/stratos/` - Entry point and flags
- `api/v1alpha1/` - CRD types
- `internal/controller/` - Reconciliation logic
- `internal/cloudprovider/` - Cloud provider implementations

### 2. Regenerate Code (if needed)

If you modified `api/v1alpha1/*.go`:

```bash
# Regenerate deepcopy methods
make generate

# Regenerate CRD manifests
make manifests
```

### 3. Apply CRDs

```bash
# Install/update CRDs in cluster
make install
```

### 4. Run the Controller

```bash
go run ./cmd/stratos/main.go --cluster-name=dev --cloud-provider=fake
```

### 5. Test with a NodePool

```bash
kubectl apply -f config/samples/nodepool_sample.yaml
```

### 6. Observe Behavior

```bash
# Watch NodePool status
kubectl get nodepools -w

# Watch nodes
kubectl get nodes -l stratos.sh/pool=workers -w

# Check controller logs (in the terminal running the controller)
```

## Using Telepresence

For faster iteration with a remote cluster, use Telepresence:

```bash
# Connect to cluster
telepresence connect

# Run controller locally, connected to remote cluster
go run ./cmd/stratos/main.go --cluster-name=dev --cloud-provider=fake
```

## Debugging

### VS Code Launch Configuration

```json title=".vscode/launch.json"
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Stratos",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/cmd/stratos",
      "args": [
        "--cluster-name=dev",
        "--cloud-provider=fake",
        "--zap-log-level=debug"
      ]
    }
  ]
}
```

### GoLand Run Configuration

1. Create a new "Go Build" configuration
2. Set:
   - Package path: `github.com/stratos-sh/stratos/cmd/stratos`
   - Program arguments: `--cluster-name=dev --cloud-provider=fake --zap-log-level=debug`

### Delve Debugging

```bash
# Build with debug symbols
go build -gcflags="all=-N -l" -o /tmp/stratos ./cmd/stratos

# Run with delve
dlv exec /tmp/stratos -- --cluster-name=dev --cloud-provider=fake
```

## Testing Changes

### Unit Tests

```bash
# Run all unit tests
make test

# Run specific package tests
go test -v ./internal/controller/...

# Run specific test
go test -v -run TestScaleUp ./internal/controller/...
```

### Integration Tests

```bash
# Run integration tests (requires envtest)
make test-integration

# Run specific integration test
go test -v -tags=integration -run TestNodePoolLifecycle ./tests/integration/...
```

### Manual Testing

1. Create a test NodePool:
   ```yaml title="test-pool.yaml"
   apiVersion: stratos.sh/v1alpha1
   kind: NodePool
   metadata:
     name: test
   spec:
     poolSize: 3
     minStandby: 1
     template:
       labels:
         stratos.sh/pool: test
       cloudProvider:
         provider: aws  # or fake
         aws:
           instanceType: t3.small
           ami: ami-0123456789abcdef0
           subnetIds: ["subnet-12345678"]
           securityGroupIds: ["sg-12345678"]
           iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/test
           userData: |
             #!/bin/bash
             echo "test"
             poweroff
   ```

2. Apply and observe:
   ```bash
   kubectl apply -f test-pool.yaml
   kubectl get nodepools -w
   ```

3. Clean up:
   ```bash
   kubectl delete nodepool test
   ```

## Environment Setup

### Required Environment Variables

```bash
# For AWS provider
export AWS_REGION=us-east-1
# AWS credentials are typically loaded from ~/.aws/credentials or IRSA

# For testing
export KUBECONFIG=~/.kube/config
```

### Optional Environment Variables

```bash
# Override cluster name
export CLUSTER_NAME=dev

# Enable verbose logging
export LOG_LEVEL=debug
```

## Troubleshooting

### Controller Won't Start

Check for existing processes:

```bash
ps aux | grep -E "main.*--cluster-name" | grep -v grep
pkill -f "cmd/stratos/main.go"
```

### CRD Not Found

Regenerate and install CRDs:

```bash
make generate
make manifests
make install
```

### Permission Denied

Ensure kubectl has cluster-admin access:

```bash
kubectl auth can-i '*' '*'
```

### AWS Errors

Verify AWS credentials:

```bash
aws sts get-caller-identity
aws ec2 describe-instances --max-items 1
```

## Next Steps

- [Testing](./testing.md) - Testing patterns and practices
- [Contributing](./contributing.md) - Contribution guidelines
