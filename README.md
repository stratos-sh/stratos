<p align="center">
  <img src="https://stratos-sh.github.io/stratos/img/logo.png" alt="Stratos Logo" width="200">
</p>

<h1 align="center">Stratos</h1>

<p align="center">
  <strong>Eliminate cloud instance cold-start delays with pre-warmed, instantly-ready nodes.</strong>
</p>

<p align="center">
  <a href="https://stratos-sh.github.io/stratos/">Documentation</a> •
  <a href="https://github.com/stratos-sh/stratos/issues">Issues</a> •
  <a href="#quick-start">Quick Start</a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License"></a>
</p>

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
warmup --> standby --> running --> stopping
                ^                     |
                |_____________________|
```

1. **Warmup** - Stratos launches instances that run initialization scripts (join cluster, pull images, configure networking) and self-stop when ready
2. **Standby** - Stopped instances wait in the pool, costing only storage (no compute charges)
3. **Running** - When pods are pending, Stratos instantly starts standby nodes (seconds, not minutes)
4. **Stopping** - Empty nodes are drained and returned to standby for reuse

## Key Features

- **Sub-minute scale-up** - Start pre-warmed nodes in seconds instead of minutes
- **Cost efficient** - Stopped instances only incur storage costs, not compute
- **Kubernetes native** - Declarative NodePool and NodeClass CRDs, integrates with existing clusters
- **CNI-aware** - Properly handles startup taints for VPC CNI, Cilium, Calico
- **Automatic maintenance** - Pool replenishment, node recycling, state synchronization
- **Dynamic resource selectors** - Discover AMIs, subnets, and security groups by tags instead of hardcoding IDs
- **Observable** - Prometheus metrics for all operations

## Quick Start

### Prerequisites

- Kubernetes cluster (1.26+)
- [Helm](https://helm.sh/docs/intro/install/) 3.x
- AWS credentials configured (for EC2 operations)

### Installation

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster
```

### Create Resources

**AWSNodeClass** (cloud-specific configuration):

```yaml
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: workers
spec:
  instanceType: m5.large

  # Dynamic resource selectors - discover resources by tags instead of hardcoding IDs
  amiSelector:
    name: "my-eks-ami-*"
    owner: self
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  # Automatic instance profile management from IAM role name
  role: my-node-role

  userData: |
    #!/bin/bash
    /etc/eks/bootstrap.sh my-cluster \
      --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
    until curl -sf http://localhost:10248/healthz; do sleep 5; done
    sleep 30
    poweroff
```

> **Note:** You can also use static IDs (`ami`, `subnetIds`, `securityGroupIds`, `iamInstanceProfile`) instead of selectors. See the [Dynamic Resource Selectors guide](https://stratos-sh.github.io/stratos/guides/dynamic-resource-selectors) for details.

**NodePool** (references the AWSNodeClass):

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 3
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: workers
    labels:
      stratos.sh/pool: workers
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
```

**Verify:**

```bash
kubectl get awsnodeclasses,nodepools
```

## Architecture Overview

```
+------------------+     +-------------------+     +------------------+
|   NodePool CRD   | --> | Stratos Controller| --> |   Cloud Provider |
|  (Desired State) |     |   (Reconciler)    |     |   (AWS EC2)      |
+------------------+     +-------------------+     +------------------+
        |                        |
        v                        v
+------------------+       +-------------+
| AWSNodeClass CRD |       |  K8s Nodes  |
| (Cloud Config)   |       | (Managed)   |
+------------------+       +-------------+
```

NodePools reference a cloud-specific NodeClass (e.g., AWSNodeClass) that contains instance configuration. This separation allows multiple NodePools to share the same cloud configuration.

The controller watches for:
- **NodePool changes** - Create/update/delete pools
- **NodeClass changes** - Cloud configuration updates (e.g., AWSNodeClass)
- **Pending pods** - Trigger scale-up when pods can't be scheduled
- **Node state changes** - Track node lifecycle and health

## Use Cases

### CI/CD Pipelines

Traditional autoscalers don't just make you wait for a node to boot — they give you a completely cold environment. Every pipeline run pulls all DaemonSet images from scratch, then pulls the CI agent image, and every `docker build` or `npm install` starts with an empty cache. Stratos nodes come pre-warmed with all DaemonSet images already pulled, and since nodes are reused (stopped and restarted rather than terminated), build caches, Docker layer caches, and package manager caches persist across runs. Your second pipeline run is dramatically faster than the first.

### LLM / AI Model Serving

Large model images (often 10-50GB+) make cold starts painfully slow. Downloading a model, loading it into GPU memory, and running health checks can take 10+ minutes before the first request is served. With Stratos, the model image is pre-pulled during the warmup phase and persists on the node's EBS volume. When demand spikes, a standby node starts in seconds with the model image already on disk — cutting startup time from minutes to seconds.

### Scale-to-Zero Applications

Stratos's ~20-second pending-to-running time (when properly configured) makes true scale-to-zero viable for latency-sensitive services. Pair a simple ingress doorman with a 30-second timeout: when a request arrives at a scaled-down service, the doorman holds the connection while Stratos starts a standby node, and the request completes within the timeout window. No idle compute costs, no cold-start frustration.

## Documentation

Full documentation is available at **[stratos-sh.github.io/stratos](https://stratos-sh.github.io/stratos/)**

- [Getting Started](https://stratos-sh.github.io/stratos/getting-started/installation) - Installation and quickstart
- [Concepts](https://stratos-sh.github.io/stratos/concepts/architecture) - Architecture and node lifecycle
- [Guides](https://stratos-sh.github.io/stratos/guides/aws-setup) - AWS setup, dynamic resource selectors, scaling policies, monitoring
- [API Reference](https://stratos-sh.github.io/stratos/reference/api/nodepool) - NodePool and AWSNodeClass CRDs

### Running Docs Locally

```bash
cd docs
npm install
npm start
```

The documentation site will be available at `http://localhost:3000/stratos/`.

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
