# Design: Stratos Node Scaler

## Context

Stratos is a Kubernetes operator that manages pools of pre-warmed nodes for instant scaling. It replaces the traditional on-demand provisioning model (like Karpenter) with a pre-warming approach that eliminates cold-start delays.

**Stakeholders:**
- Kubernetes operators who need fast node scaling
- Platform teams managing CI/CD, ML inference, or bursty workloads
- Cloud cost optimization teams

**Constraints:**
- Must work with standard Kubernetes (no cluster modifications)
- Must support stop/start-capable cloud instances (AWS EC2, GCP, Azure)
- Must be deployable via Helm or kustomize
- Must follow Kubernetes operator patterns

## Goals / Non-Goals

### Goals
- Provide sub-30-second node scaling (from standby to Ready)
- Maintain a pool of pre-warmed nodes automatically
- Support multiple NodePools with different configurations
- Integrate with Kubernetes scheduling via pod watching
- Expose metrics for operational visibility

### Non-Goals
- Image preloading management (userdata handles this)
- Smart consolidation/bin-packing (v1 only scales down empty nodes)
- Multi-cluster management
- Cost optimization recommendations

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Stratos Controller                       │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  NodePool   │  │    Pod      │  │      Pool           │  │
│  │ Controller  │  │   Watcher   │  │   Reconciler        │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
│         │                │                     │            │
│         └────────────────┼─────────────────────┘            │
│                          │                                  │
│                  ┌───────▼───────┐                          │
│                  │  Node State   │                          │
│                  │   Manager     │                          │
│                  └───────┬───────┘                          │
│                          │                                  │
│                  ┌───────▼───────┐                          │
│                  │    Cloud      │                          │
│                  │   Provider    │                          │
│                  └───────────────┘                          │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
              ┌────────────────────────┐
              │   Cloud Provider API   │
              │   (EC2, GCP, Azure)    │
              └────────────────────────┘
```

### Node States

```
          ┌─────────┐
          │ Launch  │
          └────┬────┘
               │ instance launched
               ▼
          ┌─────────┐
          │ Warmup  │ joining cluster, pulling images
          └────┬────┘
               │ instance self-stops
               ▼
          ┌─────────┐
          │ Standby │ stopped, ready to start
          └────┬────┘
               │ pending pods need capacity
               ▼
          ┌─────────┐
          │ Running │ serving workloads
          └────┬────┘
               │ empty for TTL or maxRuntime
               ▼
          ┌─────────┐
          │ Standby │ (back to standby after drain)
          └─────────┘
```

## Decisions

### Decision 1: Event-Driven Scale-Up + Periodic Reconciliation

**What**: Scale-up is triggered by watching pod events (immediate). Pool maintenance runs on a periodic loop (configurable, default 30s).

**Why**:
- Event-driven ensures fastest possible response to pending pods
- Periodic reconciliation is simpler and sufficient for pool maintenance tasks
- Separation of concerns: urgent (pods) vs routine (pool health)

**Alternatives considered**:
- Pure periodic: Too slow for scale-up (pods wait up to 30s)
- Pure event-driven: Complex to handle all edge cases (instance failures, etc.)

### Decision 2: Userdata-Driven Initialization

**What**: The instance's userdata script handles cluster join and image pulling, then self-stops. Stratos only monitors the process.

**Why**:
- Keeps Stratos simple - no SSH, no agent communication
- Userdata is cloud-native and well-understood
- Users control their initialization logic
- Self-stop is a clear "ready" signal

**Alternatives considered**:
- Agent-based: Requires deploying/managing an agent
- SSH-based: Security complexity, credentials management

### Decision 3: Stop vs Terminate on Scale-Down

**What**: When nodes are empty, Stratos stops (not terminates) them to return to standby.

**Why**:
- Preserves pre-warming work (cluster membership, cached images)
- Stopped instances have minimal cost (EBS only)
- Enables instant restart without re-initialization

**Alternatives considered**:
- Terminate: Loses pre-warming, requires full re-initialization
- Hibernate: Not all instance types support it, higher complexity

### Decision 4: NodePool as Primary Abstraction

**What**: NodePool CRD defines a pool of nodes with shared configuration (instance type, subnets, AMI, etc.)

**Why**:
- Similar to Karpenter's NodePool - familiar to users
- Single configuration point for a class of nodes
- Supports multiple pools with different characteristics

### Decision 5: Cloud Provider Interface

**What**: Abstract interface with five operations: launch, start, stop, getState, terminate.

**Why**:
- Minimal interface - only what's needed
- Enables multi-cloud support
- Clean separation of concerns

```go
type CloudProvider interface {
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
    Start(ctx context.Context, instanceID string) error
    Stop(ctx context.Context, instanceID string) error
    GetState(ctx context.Context, instanceID string) (InstanceState, error)
    Terminate(ctx context.Context, instanceID string) error
}
```

## Risks / Trade-offs

### Risk 1: Stopped Instance Costs
- **Risk**: Stopped instances still incur EBS storage costs
- **Mitigation**: Clear documentation on cost model; poolSize limits maximum exposure; users can tune minStandby

### Risk 2: Stale Pre-Warming
- **Risk**: Stopped instances may have outdated packages/images after long standby
- **Mitigation**: maxNodeRuntime forces periodic recycling; users can set aggressive values for critical workloads

### Risk 3: Cloud Provider Rate Limits
- **Risk**: Rapid scale-up may hit cloud API rate limits
- **Mitigation**: Exponential backoff on retries; metrics for monitoring; pool pre-sizing reduces need for rapid provisioning

### Risk 4: Userdata Failures
- **Risk**: Buggy userdata prevents nodes from reaching standby
- **Mitigation**: Configurable timeout action (stop or terminate); clear timeout error events; metrics for warmup failures

## Data Model

### NodePool CRD

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: gpu-workers
spec:
  poolSize: 10           # max total nodes (standby + running)
  minStandby: 5          # min nodes to keep in standby

  # Pre-warming configuration
  warmup:
    timeout: 10m         # max time to wait for self-stop
    timeoutAction: terminate  # stop | terminate

  # Scale-down configuration
  scaleDown:
    enabled: true
    emptyNodeTTL: 5m     # time empty before stopping

  # Optional runtime limits
  maxNodeRuntime: 24h    # 0 = disabled

  # Cloud provider configuration
  provider: aws
  aws:
    instanceType: g4dn.xlarge
    amiID: ami-12345678
    subnets:
      - subnet-abc
      - subnet-def
    securityGroups:
      - sg-xyz
    instanceProfile: stratos-node-role

  # Kubernetes node configuration
  nodeTemplate:
    labels:
      pool: gpu-workers
    taints:
      - key: nvidia.com/gpu
        value: "true"
        effect: NoSchedule
```

### Node Labels

Stratos-managed nodes have these labels:

```yaml
labels:
  stratos.sh/managed: "true"
  stratos.sh/nodepool: gpu-workers
  stratos.sh/state: standby  # warmup | running | standby
```

## RBAC

Stratos requires cluster-scoped RBAC with least-privilege:

```yaml
rules:
  # Full access to Nodes
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Watch Pods for scale-up triggers
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]

  # Full access to NodePool CRD
  - apiGroups: ["stratos.sh"]
    resources: ["nodepools"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Create events for visibility
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
```

## Design Decisions (v1)

1. **Leader election**: Yes, use controller-runtime's built-in leader election for HA

2. **Disruption budget for standby**: No limit for v1 - consume all standby nodes if needed

3. **Node draining timeout**: 5 minutes default, then force-drain

4. **Instance type diversity**: Single instance type per NodePool for v1 (can add fallback later)
