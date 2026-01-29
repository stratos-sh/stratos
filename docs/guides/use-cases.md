---
sidebar_position: 0
title: Use Cases
description: Common use cases where Stratos eliminates cold-start delays
---

# Use Cases

Stratos is designed for workloads where cold-start latency is a bottleneck. Below are three scenarios where pre-warmed nodes make a significant difference.

## CI/CD Pipelines

### The Problem

CI/CD agents running on Kubernetes (Jenkins agents, GitHub Actions runners, GitLab runners, etc.) are typically ephemeral pods that scale based on pipeline demand. When a pipeline triggers and no capacity is available, traditional autoscalers spin up a fresh node. This creates a cascade of delays:

1. **Node provisioning** — The autoscaler requests a new instance from the cloud provider (2-5 minutes)
2. **DaemonSet image pulls** — Every DaemonSet (logging, monitoring, CNI, etc.) pulls its images from scratch on the new node
3. **CI agent image pull** — The runner/agent container image is pulled
4. **Pipeline cold cache** — Since the node is brand new, there are no Docker layer caches, no `node_modules` caches, no Go module caches — every `docker build`, `npm install`, or `go mod download` starts from zero

With Karpenter this is faster (~40-50 seconds to node ready), but you still get a completely cold environment every time.

### How Stratos Helps

Stratos fundamentally changes this in two ways:

**Pre-pulled images.** During the warmup phase, Stratos automatically pulls all DaemonSet images. You can also configure additional images to pre-pull (like your CI agent image) via `preWarm.imagesToPull`. When a pipeline triggers, the node starts in ~20 seconds with images already on disk.

**Persistent caches.** Unlike traditional autoscalers that terminate instances after use, Stratos stops instances and returns them to standby. The EBS volume — with all its caches — survives across runs. Your Docker layer cache, package manager caches, and build artifacts persist. The second pipeline run on a Stratos node is dramatically faster than the first, and every subsequent run benefits from the warm cache.

### Example Configuration

```yaml title="awsnodeclass-ci.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: ci-runners
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.2xlarge
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: ci-node-role
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 200  # Large disk for Docker/build caches
      volumeType: gp3
      encrypted: true
---
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: ci-runners
spec:
  poolSize: 20
  minStandby: 5
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: ci-runners
    labels:
      stratos.sh/pool: ci-runners
      node-role.kubernetes.io/ci: ""
    startupTaints:
      - key: stratos.sh/not-ready
        value: "true"
        effect: NoSchedule
    startupTaintRemoval: WhenNetworkReady
  preWarm:
    timeout: 15m
    timeoutAction: terminate
  scaleDown:
    enabled: true
    emptyNodeTTL: 10m
    drainTimeout: 5m
```

Target CI pods to this pool:

```yaml title="ci-agent-deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jenkins-agent
spec:
  template:
    spec:
      nodeSelector:
        stratos.sh/pool: ci-runners
      containers:
        - name: agent
          image: jenkins/inbound-agent:latest
```

## LLM / AI Model Serving

### The Problem

Large language models and AI inference workloads have notoriously long startup times. A typical cold start involves:

1. **Node provisioning** — Standard cloud instance launch (2-5 minutes)
2. **Model image pull** — Model container images are often 10-50GB+, taking 5-15 minutes to download
3. **Model loading** — Loading model weights into memory (or GPU VRAM) takes additional minutes
4. **Health check warm-up** — The model needs to process a few warm-up requests before serving at full speed

Total cold-start time can exceed 15-20 minutes. When demand spikes, users wait in queues or requests time out. Scaling becomes impractical if every new replica takes that long to become ready.

### How Stratos Helps

Stratos addresses the most time-consuming part of this process: the image pull. During the warmup phase, the model container image is pre-pulled onto the node's EBS volume. Since Stratos reuses nodes (stop/start rather than terminate/launch), the image persists on disk across scale events.

When demand spikes and a new replica is needed:
- Stratos starts a standby node in ~20 seconds
- The model image is already on disk — no download needed
- Only model loading into memory remains, cutting total startup from 15+ minutes to under 2 minutes

You can also use the user data script during warmup to pre-download model weights to a persistent volume, further reducing startup time.

### Example Configuration

```yaml title="awsnodeclass-inference.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: inference
spec:
  bootstrapTemplate: AL2023
  instanceType: g5.2xlarge  # GPU instance
  architecture: x86_64
  amiSelector:
    name: "amazon-eks-gpu-node-*"
    owner: amazon
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: inference-node-role
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 500  # Large disk for model images
      volumeType: gp3
      iops: 16000
      throughput: 1000
      encrypted: true
---
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: inference
spec:
  poolSize: 10
  minStandby: 2
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: inference
    labels:
      stratos.sh/pool: inference
      node-role.kubernetes.io/inference: ""
      nvidia.com/gpu: "present"
    startupTaints:
      - key: stratos.sh/not-ready
        value: "true"
        effect: NoSchedule
    startupTaintRemoval: WhenNetworkReady
  preWarm:
    timeout: 20m
    timeoutAction: terminate
  scaleDown:
    enabled: true
    emptyNodeTTL: 30m  # Keep nodes longer to avoid re-pulling models
    drainTimeout: 5m
```

Target inference workloads to this pool:

```yaml title="llm-deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: llm-server
spec:
  template:
    spec:
      nodeSelector:
        stratos.sh/pool: inference
      containers:
        - name: model
          image: my-registry.com/llama-3:70b
          resources:
            limits:
              nvidia.com/gpu: 1
```

## Scale-to-Zero Applications

### The Problem

Scale-to-zero is the holy grail of cost optimization — pay nothing when there's no traffic. But traditional Kubernetes autoscaling makes this impractical for latency-sensitive services:

- **Karpenter/Cluster Autoscaler**: 40 seconds to 5+ minutes to provision a new node
- **KEDA + HPA**: Can scale pods to zero, but if no node is available, you're back to waiting for node provisioning
- **Knative/Serverless**: Designed for scale-to-zero but still bound by underlying node availability

Most teams settle for "scale-to-one" instead, keeping at least one replica running 24/7 to avoid cold starts. This wastes resources for services with low or sporadic traffic.

### How Stratos Helps

Stratos's ~20-second pending-to-running time (when properly configured) makes true scale-to-zero viable. The pattern is straightforward:

1. **Scale pods to zero** when idle using KEDA, CronJobs, or custom logic
2. **Use an ingress doorman** (a lightweight proxy that holds incoming requests)
3. When a request arrives at a scaled-down service, the doorman triggers pod creation
4. The pod becomes Pending, Stratos starts a standby node in ~20 seconds
5. The pod schedules and starts serving — all within a 30-second timeout window

This works because Stratos has already done all the heavy lifting during warmup. The node is fully initialized — kubelet running, CNI configured, DaemonSet images pulled. Starting it from standby is just a cloud API call to resume a stopped instance.

### Example Configuration

```yaml title="awsnodeclass-ondemand.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: on-demand
spec:
  bootstrapTemplate: AL2023
  instanceType: m5.large
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster
  role: node-role
---
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: on-demand
spec:
  poolSize: 10
  minStandby: 2  # Keep 2 nodes ready for instant scale-up
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: on-demand
    labels:
      stratos.sh/pool: on-demand
      scale-to-zero: enabled
    startupTaints:
      - key: stratos.sh/not-ready
        value: "true"
        effect: NoSchedule
    startupTaintRemoval: WhenNetworkReady
  scaleDown:
    enabled: true
    emptyNodeTTL: 2m   # Return nodes to standby quickly
    drainTimeout: 30s
```

Target scale-to-zero services to this pool:

```yaml title="service-deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
spec:
  replicas: 0  # Scaled to zero by KEDA or manually
  template:
    spec:
      nodeSelector:
        stratos.sh/pool: on-demand
      containers:
        - name: app
          image: my-app:latest
```

With this setup, you can confidently scale services to zero knowing that when traffic arrives, a node will be available in seconds rather than minutes.

## Next Steps

- [Quickstart](../getting-started/quickstart.md) - Set up your first NodePool
- [Scaling Policies](./scaling-policies.md) - Fine-tune scale-up and scale-down behavior
- [Monitoring](./monitoring.md) - Track pool health and scale-up latency
