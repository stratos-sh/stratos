# Stratos

**Eliminate cloud instance cold-start delays with pre-warmed, instantly-ready nodes.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## What is Stratos?

Stratos is a Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, stopped instances ready to start in seconds. Instead of waiting 3-5 minutes for new nodes to provision, boot, and initialize, Stratos enables sub-minute scale-up times by keeping instances in a "warm standby" state.

## The Problem

Spinning up a new cloud instance typically takes 3-5 minutes:

1. **Instance provisioning** - Cloud provider allocates resources
2. **OS boot** - Operating system initialization
3. **Kubernetes join** - Node registers with the cluster
4. **CNI setup** - Network plugin initialization
5. **Application initialization** - User data scripts, image pulls

For time-sensitive workloads like CI/CD pipelines, autoscaling events, or burst traffic handling, this delay is unacceptable.

## How Stratos Solves It

Stratos maintains a pool of pre-warmed, stopped instances using a four-phase lifecycle:

```
warmup --> standby --> running --> terminating
                ^                       |
                |_______________________|
```

1. **Warmup** - Stratos launches instances that run initialization scripts (join cluster, pull images, configure networking) and self-stop when ready
2. **Standby** - Stopped instances wait in the pool, costing only storage (no compute charges)
3. **Running** - When pods are pending, Stratos instantly starts standby nodes (seconds, not minutes)
4. **Terminating** - Empty nodes are drained and returned to standby for reuse

## Key Features

- **Sub-minute scale-up** - Start pre-warmed nodes in seconds instead of minutes
- **Cost efficient** - Stopped instances only incur storage costs, not compute
- **Kubernetes native** - Declarative NodePool CRD, integrates with existing clusters
- **CNI-aware** - Properly handles startup taints for VPC CNI, Cilium, Calico
- **Automatic maintenance** - Pool replenishment, node recycling, state synchronization
- **Observable** - Prometheus metrics for all operations

## Quick Start

### Prerequisites

- Kubernetes cluster (1.26+)
- AWS credentials configured (for EC2 operations)
- kubectl and kustomize installed

### Installation

1. **Install CRDs:**
   ```bash
   kubectl apply -k config/crd
   ```

2. **Deploy the controller:**
   ```bash
   kubectl apply -k config/default
   ```

3. **Create a NodePool:**
   ```yaml
   apiVersion: stratos.sh/v1alpha1
   kind: NodePool
   metadata:
     name: workers
   spec:
     poolSize: 10
     minStandby: 3
     template:
       labels:
         stratos.sh/pool: workers
       startupTaints:
         - key: node.eks.amazonaws.com/not-ready
           value: "true"
           effect: NoSchedule
       cloudProvider:
         provider: aws
         aws:
           region: us-east-1
           instanceType: m5.large
           ami: ami-0123456789abcdef0
           subnetIds:
             - subnet-12345678
           securityGroupIds:
             - sg-12345678
           iamInstanceProfile: arn:aws:iam::123456789:instance-profile/node-role
           userData: |
             #!/bin/bash
             /etc/eks/bootstrap.sh my-cluster \
               --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
             until curl -sf http://localhost:10248/healthz; do sleep 5; done
             sleep 30
             poweroff
   ```

4. **Verify the pool:**
   ```bash
   kubectl get nodepools
   ```

## Architecture Overview

```
+------------------+     +-------------------+     +------------------+
|   NodePool CRD   | --> | Stratos Controller| --> |   Cloud Provider |
|  (Desired State) |     |   (Reconciler)    |     |   (AWS EC2)      |
+------------------+     +-------------------+     +------------------+
                                |
                                v
                         +-------------+
                         |  K8s Nodes  |
                         | (Managed)   |
                         +-------------+
```

The controller watches for:
- **NodePool changes** - Create/update/delete pools
- **Pending pods** - Trigger scale-up when pods can't be scheduled
- **Node state changes** - Track node lifecycle and health

## Example Use Cases

| Use Case | Problem | Stratos Solution |
|----------|---------|------------------|
| **CI/CD Pipelines** | Runners queued waiting for capacity | Instant runners, zero queue time |
| **Kubernetes Autoscaling** | 3-5 min delay for new nodes | Sub-minute node availability |
| **Batch Processing** | Cold start delays compound | Pre-warmed workers ready instantly |
| **ML Inference** | Model loading takes minutes | Models pre-loaded, serve immediately |

## Documentation

- [Getting Started](docs/getting-started.md) - Installation and first NodePool
- [Architecture](docs/architecture.md) - System design and components
- [Configuration](docs/configuration.md) - NodePool CRD and controller options
- [Operations](docs/operations.md) - Monitoring, troubleshooting, best practices

### Running Docs Locally

```bash
cd docs
npm install
npm start
```

The documentation site will be available at `http://localhost:3000`.

## Development

### Build

```bash
make build
```

### Run Locally

```bash
# With fake cloud provider (for testing)
go run ./cmd/stratos/main.go --cluster-name=main --cloud-provider=fake

# With AWS
go run ./cmd/stratos/main.go --cluster-name=main --cloud-provider=aws
```

### Test

```bash
# Unit tests
make test

# Integration tests (requires envtest setup)
make test-integration

# Coverage report
make coverage
```

## Status

Stratos is currently in **alpha** development. The API may change between versions.

## License

Apache License 2.0
