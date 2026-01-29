---
sidebar_position: 1
title: Introduction
description: Stratos is a Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, reusable nodes.
slug: /
---

# Stratos

Stratos is a Kubernetes operator that maintains pools of pre-warmed, reusable Kubernetes nodes. Nodes are launched once, fully initialized, then stopped and restarted on demand — giving you instant capacity with warm caches, pre-pulled images, and zero cold-start overhead.

## The Problem

When Kubernetes needs more capacity, every existing autoscaler gives you a **brand new machine**. That means:

1. **Provisioning** — Wait for the cloud provider to allocate and launch an instance
2. **Booting** — OS initialization, kubelet startup, cluster join
3. **Networking** — CNI plugin initialization, IP allocation
4. **Image pulls** — Every DaemonSet image downloaded from scratch
5. **Application startup** — Your workload's images pulled, caches empty, no local state

Cluster Autoscaler takes 3-8 minutes. Karpenter brought this down to ~40-50 seconds. But even at Karpenter speed, you still get a **cold node every time** — empty caches, no pre-pulled images, no local state. For workloads like CI/CD pipelines, LLM inference, or bursty applications, the cold environment is just as painful as the wait.

## The Solution

Stratos takes a fundamentally different approach: **nodes are initialized once and reused**.

1. **Warmup** — Stratos launches instances that join the cluster, initialize CNI, pull all DaemonSet images, run any custom setup, then self-stop
2. **Standby** — Stopped instances sit in a pool, costing only EBS storage. The disk retains everything: images, caches, local state
3. **Scale-up** — When pods are pending, Stratos starts a standby instance. Since the node is already initialized, it's ready in ~20 seconds
4. **Scale-down** — Empty nodes are drained and stopped (not terminated), returning to standby with all their state intact

The key insight: **Stratos stops and starts nodes instead of terminating and recreating them.** This means every scale-up benefits from everything the node has accumulated — Docker layer caches, package manager caches, pre-pulled images, downloaded models, and any other local state.

### Not Just Faster Boot — Faster Everything

Traditional autoscalers measure success by "time to node ready." Stratos is faster there too (~20 seconds vs ~40-50 seconds). But the real advantage is what happens after the node is ready:

| | Traditional Autoscaler | Stratos |
|---|---|---|
| **Node provisioning** | Launch new instance every time | Start existing instance (~20s) |
| **DaemonSet images** | Pull from registry every time | Already on disk |
| **Application images** | Pull from registry every time | Already on disk (if previously run) |
| **Docker build cache** | Empty | Warm from previous runs |
| **Package manager cache** | Empty (`npm install` from scratch) | Warm (`node_modules` cached) |
| **Model weights** | Download every time (10+ min for LLMs) | Already on disk |
| **OS/system caches** | Cold | Warm |

A Karpenter node is ready in ~40 seconds, then your CI pipeline spends another 5 minutes pulling images and rebuilding dependencies. A Stratos node is ready in ~20 seconds, and your pipeline starts with warm caches from the last run.

## Use Cases

### CI/CD Pipelines
CI agents on Kubernetes typically get a fresh node with empty caches. Every `docker build`, `npm install`, or `go mod download` starts from scratch. Stratos nodes retain build caches across runs — your second pipeline is dramatically faster than the first, and every run after that benefits from the warm cache.

### LLM / AI Model Serving
Model images are often 10-50GB+. Downloading them on every scale-up makes autoscaling impractical. With Stratos, the model image is pre-pulled during warmup and persists on the node's EBS volume. Scaling out goes from 15+ minutes to under 2.

### Scale-to-Zero
Stratos's ~20-second startup makes true scale-to-zero viable. Pair it with an ingress doorman that holds requests for up to 30 seconds — when traffic hits a scaled-down service, a standby node starts and begins serving before the timeout. No idle compute, no cold-start frustration.

See the [Use Cases](./guides/use-cases.md) guide for detailed configurations.

## Key Features

- **Instant capacity**: Start pre-warmed nodes in ~20 seconds
- **Warm caches**: Nodes retain Docker layers, build caches, downloaded models, and local state across restarts
- **Pre-pulled images**: DaemonSet images pulled automatically during warmup
- **Simplified configuration**: Stratos generates node bootstrap scripts automatically based on your AMI family (AL2023, AL2, Bottlerocket)
- **Cost-efficient**: Stopped instances only incur EBS storage costs
- **CNI-aware**: Handles startup taints for VPC CNI, Cilium, and Calico
- **Kubernetes-native**: Declarative NodePool and AWSNodeClass CRDs
- **Cloud-agnostic design**: Built with a provider abstraction layer (AWS supported)

## Quick Start

### 1. Install with Helm

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster
```

### 2. Create an AWSNodeClass and NodePool

```yaml title="awsnodeclass.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: workers
spec:
  bootstrapTemplate: AL2023   # Stratos generates userData automatically
  instanceType: m5.large
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: my-eks-node-role      # Stratos manages instance profile
```

```yaml title="nodepool.yaml"
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
      workload-type: general    # Custom label for targeting
    startupTaints:
      - key: stratos.sh/not-ready
        value: "true"
        effect: NoSchedule
```

```bash
kubectl apply -f awsnodeclass.yaml
kubectl apply -f nodepool.yaml
```

### 3. Target Pods to Your NodePool

Use `nodeSelector` or `nodeAffinity` to schedule pods on Stratos-managed nodes:

```yaml title="deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      nodeSelector:
        stratos.sh/pool: workers      # Target the NodePool by label
      containers:
        - name: app
          image: my-app:latest
```

### 4. Watch Nodes Scale

```bash
kubectl get nodes -l stratos.sh/pool=workers -w
```

## How It Works

```
                    +---------+
                    | warmup  |  Launch, join cluster, pull images, run setup
                    +----+----+
                         |
           self-stop     |
                         v
                    +---------+
                    | standby |  Stopped — disk retains all state
                    +----+----+
                         |
         scale-up        |
         (start instance)|
                         v
                    +---------+
                    | running |  Serving pods, accumulating caches
                    +----+----+
                         |
         scale-down      |
         (drain & stop)  |
                         v
                    +---------+
                    | standby |  Back to pool — caches preserved
                    +---------+
```

## Next Steps

- [Installation](./getting-started/installation.md) - Install with Helm
- [Quickstart](./getting-started/quickstart.md) - Create your first NodePool
- [Use Cases](./guides/use-cases.md) - CI/CD, LLM serving, scale-to-zero
- [Architecture](./concepts/architecture.md) - Understand how Stratos works
- [AWS Setup](./guides/aws-setup.md) - Configure AWS prerequisites
