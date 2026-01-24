---
sidebar_position: 1
title: Introduction
description: Stratos is a Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, stopped instances.
slug: /
---

# Stratos

Stratos is a Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, stopped instances ready to start in seconds.

## The Problem

When Kubernetes needs more capacity, even modern autoscalers face fundamental latency constraints:

### Traditional Cluster Autoscaler

1. Pod becomes pending (unschedulable)
2. Cluster Autoscaler detects pending pod
3. Cloud provider launches new instance (2-5 minutes)
4. Instance boots and joins cluster (1-2 minutes)
5. CNI initializes networking (30-60 seconds)
6. Pod finally schedules

**Total time: 3-8 minutes**

### Karpenter

Karpenter improves on Cluster Autoscaler with direct cloud API integration and faster decision-making:

1. Pod becomes pending
2. Karpenter provisions instance directly via cloud API
3. Instance launches, boots, joins cluster
4. CNI initializes
5. Pod schedules

**Total time: ~40-50 seconds** - a major improvement, but still limited by cold-start fundamentals.

### The Root Cause

Both approaches share the same bottleneck: they provision instances on-demand. Every scale-up must wait for:

- Instance launch and boot
- OS initialization
- Kubelet startup and cluster join
- CNI initialization
- DaemonSet pod startup

No matter how fast the autoscaler's decision-making, these cold-start steps take time.

## The Solution

Stratos eliminates cold-start delays by doing the work ahead of time. Instead of provisioning on-demand, Stratos maintains a pool of pre-warmed instances in a stopped state:

1. **Pre-warming**: Instances launch, join the cluster, initialize CNI, pre-pull DaemonSet images, then self-stop
2. **Standby**: Stopped instances cost only EBS storage, ready for instant start
3. **Scale-up**: When pods are pending, Stratos starts standby instances in ~20-25 seconds
4. **Scale-down**: Empty nodes are drained and returned to standby

**Result: Scale-up in ~20-25 seconds - roughly half the time of Karpenter.**

### Why Stratos is Faster

| Step | Karpenter | Stratos |
|------|-----------|---------|
| Instance launch | Must wait | Already done |
| OS boot | Must wait | Already done |
| Kubelet startup | Must wait | Already done |
| CNI initialization | Must wait | Already done |
| DaemonSet pods | Must pull images | Images pre-pulled |
| **Instance start** | N/A | ~15-20 seconds |
| **Node ready** | N/A | ~5 seconds |

The only work remaining at scale-up time is starting an already-initialized instance.

## Key Features

- **Fastest scale-up**: Start pre-warmed instances in ~20-25 seconds (half of Karpenter's ~40-50 seconds)
- **Pre-pulled images**: DaemonSet images are automatically pre-pulled during warmup
- **Custom image pre-pulling**: Configure additional images to pre-pull for even faster pod startup
- **Cost-efficient**: Stopped instances only incur storage costs
- **CNI-aware**: Handles startup taints for VPC CNI, Cilium, and Calico
- **Kubernetes-native**: Manages nodes through standard Kubernetes APIs
- **Cloud-agnostic design**: Built with a provider abstraction layer (AWS supported)

## Quick Start

### 1. Install CRDs and Controller

```bash
kubectl apply -k config/crd
kubectl apply -k config/default
```

### 2. Create a NodePool

```yaml title="nodepool.yaml"
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
        instanceType: m5.large
        ami: ami-0123456789abcdef0
        subnetIds: ["subnet-12345678"]
        securityGroupIds: ["sg-12345678"]
        iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/node-role
        userData: |
          #!/bin/bash
          /etc/eks/bootstrap.sh my-cluster \
            --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
          until curl -sf http://localhost:10248/healthz; do sleep 5; done
          sleep 30
          poweroff
```

```bash
kubectl apply -f nodepool.yaml
```

### 3. Watch Nodes Scale

```bash
kubectl get nodes -l stratos.sh/pool=workers -w
```

## How It Works

```
                    +---------+
                    | warmup  |  Instance launching, running user data
                    +----+----+
                         |
           self-stop     |
                         v
                    +---------+
                    | standby |  Instance stopped, ready for instant start
                    +----+----+
                         |
         scale-up        |
         (start instance)|
                         v
                    +---------+
                    | running |  Active node, serving pods
                    +----+----+
                         |
         scale-down      |
         (drain & stop)  |
                         v
                    +---------+
                    | standby |  Back to pool
                    +---------+
```

## Next Steps

- [Installation](./getting-started/installation.md) - Detailed installation instructions
- [Architecture](./concepts/architecture.md) - Understand how Stratos works
- [AWS Setup](./guides/aws-setup.md) - Configure AWS prerequisites
