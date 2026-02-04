---
sidebar_position: 1
title: Architecture
description: Understand the Stratos system architecture and components
---

# Architecture

Stratos is a Kubernetes operator built using the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) framework (kubebuilder pattern). This document describes its architecture and core components.

## Overview

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

The controller continuously reconciles the desired state (NodePool spec) with the actual state (cloud instances and Kubernetes nodes).

## Core Components

### NodePool Controller

The main reconciliation loop (`internal/controller/nodepool_controller.go`) handles:

- **NodePool lifecycle** - Create, update, delete NodePool resources
- **Pool maintenance** - Maintain minStandby nodes, replenish pools
- **Scale operations** - Scale up for pending pods, scale down empty nodes
- **State synchronization** - Sync Kubernetes node state with cloud instance state

The controller is event-driven, reacting to:

- NodePool spec changes
- Pending pod creation (triggers scale-up)
- Node state changes
- Periodic reconciliation (default: 30 seconds)

### Cloud Provider Interface

The cloud provider abstraction (`internal/cloudprovider/interface.go`) defines operations for managing cloud instances:

```go
type CloudProvider interface {
    LaunchInstance(ctx context.Context, cfg *LaunchConfig) (*Instance, error)
    StartInstance(ctx context.Context, instanceID string) error
    StopInstance(ctx context.Context, instanceID string, force bool) error
    TerminateInstance(ctx context.Context, instanceID string) error
    GetInstanceState(ctx context.Context, instanceID string) (InstanceState, error)
    GetInstance(ctx context.Context, instanceID string) (*Instance, error)
    ListInstances(ctx context.Context, tags map[string]string) ([]*Instance, error)
    UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error
}
```

**Implementations:**

| Provider | Location | Description |
|----------|----------|-------------|
| AWS | `internal/cloudprovider/aws/` | Production EC2 implementation with rate limiting |
| Fake | `internal/cloudprovider/fake/` | Mock provider for testing |

### Pod Watcher

The pod watcher (`internal/controller/pod_watcher.go`) monitors for unschedulable pods that could trigger scale-up. It filters for pods that:

- Have `PodScheduled` condition = `False`
- Have reason `Unschedulable`
- Could be satisfied by the NodePool's node template

### Scale Calculator

The scale calculator (`internal/controller/scale_calculator.go`) determines how many nodes to start based on:

- Pending pod resource requests (CPU, memory)
- Node capacity for the instance type
- Current in-flight scale-up operations

## Request Flow

### Scale-Up Flow

```
1. Pod created, cannot be scheduled
   |
2. PodWatcher detects unschedulable pod
   |
3. Controller reconcile triggered
   |
4. calculateScaleUpNeeded() runs:
   - Count unschedulable pods matching this pool
   - Calculate nodes needed based on resource requests
   - Subtract in-flight scale-ups (nodes already starting)
   - Cap at available standby nodes
   |
5. scaleUp() executes:
   - Select standby nodes
   - Start cloud instances
   - Transition state: standby -> running
   - Add scale-up-started annotation
   |
6. Node becomes Ready
   - CNI initializes
   - Network readiness taint removed
   - Pod scheduled
```

### Scale-Down Flow

```
1. Node has no scheduled pods (excluding DaemonSets)
   |
2. findScaleDownCandidates() runs:
   - Mark empty nodes with scale-down-candidate-since annotation
   - Check if emptyNodeTTL has elapsed
   |
3. scaleDown() executes (after TTL):
   - Transition state: running -> terminating
   - Cordon the node
   - Drain pods (respecting PDBs)
   |
4. After drain completes (or drainTimeout):
   - Stop cloud instance
   - terminating -> standby
   - Returns to standby pool
```

### Warmup Lifecycle

The warmup phase is where Stratos gains its speed advantage over on-demand provisioners like Karpenter. By completing all initialization work ahead of time, Stratos reduces scale-up from ~40-50 seconds (Karpenter) to ~20-25 seconds.

```
1. replenishStandby() launches new instance
   - State set to "warmup" in cloud tags
   - User data script starts executing
   |
2. User data runs:
   - Join Kubernetes cluster
   - Register with network readiness taint
   - Wait for kubelet healthy
   - Execute poweroff (self-stop)
   |
3. Stratos pre-pulls images:
   - Automatically pre-pulls all DaemonSet images
   - Pre-pulls any images configured in preWarm.imagesToPull
   |
4. monitorCloudWarmupInstances() detects stopped instance:
   - Verify cloud state is "stopped"
   - Create/update K8s Node object
   - Transition state: warmup -> standby
   |
5. On timeout (if instance doesn't self-stop):
   - timeoutAction=stop: Force stop, transition to standby
   - timeoutAction=terminate: Terminate instance
```

## In-Flight Tracking

To prevent duplicate scale-ups, Stratos tracks nodes that are "starting" using the `stratos.sh/scale-up-started` annotation.

Nodes are considered "starting" if:
- They have the annotation
- The annotation timestamp is within the TTL (60 seconds)
- The node is not yet Ready

## Security Model

### Controller Permissions

The controller requires cluster-admin level access for:
- Managing Node objects
- Watching/evicting Pods
- Reading PodDisruptionBudgets

See `config/rbac/` for the generated RBAC manifests.

### Cloud Permissions

The controller needs minimal EC2 permissions:
- `ec2:RunInstances`
- `ec2:TerminateInstances`
- `ec2:StartInstances`
- `ec2:StopInstances`
- `ec2:DescribeInstances`
- `ec2:CreateTags`

See [AWS Setup](../guides/aws-setup.md) for detailed IAM configuration.

## Directory Structure

```
cmd/stratos/main.go           # Entry point, manager setup, flags
api/v1alpha1/                 # NodePool CRD types
internal/
├── controller/               # Kubernetes reconcilers
│   ├── nodepool_controller.go    # Main reconciliation loop
│   ├── pod_watcher.go            # Detects pending pods
│   ├── state.go                  # Node state machine
│   ├── scale_up.go               # Scale-up logic
│   ├── scale_down.go             # Scale-down logic
│   ├── scale_calculator.go       # Pod-to-node calculation
│   ├── pool_maintenance.go       # Pool maintenance
│   └── network_readiness.go      # CNI readiness detection
├── cloudprovider/            # Cloud abstraction layer
│   ├── interface.go              # CloudProvider interface
│   ├── types.go                  # Cloud-agnostic types
│   ├── factory.go                # Provider factory
│   ├── aws/                      # AWS EC2 implementation
│   └── fake/                     # Mock provider for testing
└── metrics/                  # Prometheus metrics
config/
├── crd/bases/                # Generated CRD manifests
├── rbac/                     # Generated RBAC manifests
└── samples/                  # Example NodePool resources
```

## Next Steps

- [Node Lifecycle](./node-lifecycle.md) - Deep dive into node states
- [Cloud Providers](./cloud-providers.md) - Cloud provider abstraction
