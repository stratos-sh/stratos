# Data Model: Stratos - Kubernetes Node Scaler

**Date**: 2026-01-19
**Branch**: `001-instance-pool-manager`

This document defines the data model for Stratos, extracted from the feature specification.

---

## Entity Relationship Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                           Kubernetes Cluster                          │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│   ┌─────────────┐        manages        ┌─────────────┐              │
│   │  NodePool   │◄─────────────────────►│    Node     │              │
│   │   (CRD)     │        1:N            │  (K8s API)  │              │
│   └──────┬──────┘                       └──────┬──────┘              │
│          │                                     │                      │
│          │ owns                                │ backed by            │
│          │                                     │                      │
│          ▼                                     ▼                      │
│   ┌─────────────┐                       ┌─────────────┐              │
│   │  NodePool   │                       │  Instance   │              │
│   │   Status    │                       │  (EC2/GCP)  │              │
│   └─────────────┘                       └─────────────┘              │
│                                                                       │
│   ┌─────────────┐        triggers       ┌─────────────┐              │
│   │    Pod      │───────────────────────►│  Scale-Up   │              │
│   │(unscheduled)│                       │  Decision   │              │
│   └─────────────┘                       └─────────────┘              │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 1. NodePool (Custom Resource Definition)

The primary configuration resource for Stratos. Defines a pool of pre-warmed nodes.

### 1.1 NodePool Spec

```go
// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
    // PoolSize is the maximum total nodes (standby + running, excluding warmup)
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=1000
    PoolSize int32 `json:"poolSize"`

    // MinStandby is the minimum number of nodes to maintain in stopped/standby state
    // +kubebuilder:validation:Minimum=0
    MinStandby int32 `json:"minStandby"`

    // Template defines the node template for this pool
    Template NodeTemplate `json:"template"`

    // ScaleDown configures automatic scale-down behavior
    // +optional
    ScaleDown *ScaleDownConfig `json:"scaleDown,omitempty"`

    // PreWarm configures the pre-warming lifecycle
    // +optional
    PreWarm *PreWarmConfig `json:"preWarm,omitempty"`

    // MaxNodeRuntime configures automatic node recycling
    // Zero or nil means disabled
    // +optional
    MaxNodeRuntime *metav1.Duration `json:"maxNodeRuntime,omitempty"`

    // ReconciliationInterval is how often to run the maintenance loop
    // Default: 30 seconds
    // +optional
    ReconciliationInterval *metav1.Duration `json:"reconciliationInterval,omitempty"`
}
```

### 1.2 NodeTemplate

```go
// NodeTemplate defines the template for nodes in this pool
type NodeTemplate struct {
    // Labels to apply to nodes
    // +optional
    Labels map[string]string `json:"labels,omitempty"`

    // Taints to apply to nodes
    // +optional
    Taints []corev1.Taint `json:"taints,omitempty"`

    // CloudProvider specifies the cloud provider configuration
    CloudProvider CloudProviderConfig `json:"cloudProvider"`
}
```

### 1.3 CloudProviderConfig

```go
// CloudProviderConfig specifies cloud-specific instance configuration
type CloudProviderConfig struct {
    // Provider is the cloud provider type
    // +kubebuilder:validation:Enum=aws;gcp;azure
    Provider string `json:"provider"`

    // AWS-specific configuration
    // +optional
    AWS *AWSConfig `json:"aws,omitempty"`
}

// AWSConfig holds AWS EC2 configuration
type AWSConfig struct {
    // InstanceType is the EC2 instance type (e.g., "m5.large")
    InstanceType string `json:"instanceType"`

    // AMI is the Amazon Machine Image ID
    AMI string `json:"ami"`

    // SubnetIDs is the list of subnets to launch instances in
    // +kubebuilder:validation:MinItems=1
    SubnetIDs []string `json:"subnetIds"`

    // SecurityGroupIDs is the list of security groups
    // +kubebuilder:validation:MinItems=1
    SecurityGroupIDs []string `json:"securityGroupIds"`

    // IAMInstanceProfile is the IAM instance profile ARN
    IAMInstanceProfile string `json:"iamInstanceProfile"`

    // UserData is the base64-encoded user data script
    // This script should join the cluster and self-stop
    // +optional
    UserData string `json:"userData,omitempty"`

    // BlockDeviceMappings defines the EBS volumes
    // +optional
    BlockDeviceMappings []BlockDeviceMapping `json:"blockDeviceMappings,omitempty"`

    // Tags to apply to instances
    // +optional
    Tags map[string]string `json:"tags,omitempty"`
}

// BlockDeviceMapping defines an EBS volume
type BlockDeviceMapping struct {
    DeviceName string `json:"deviceName"`
    VolumeSize int32  `json:"volumeSize"`
    VolumeType string `json:"volumeType"`
    Encrypted  bool   `json:"encrypted"`
}
```

### 1.4 ScaleDownConfig

```go
// ScaleDownConfig configures automatic scale-down behavior
type ScaleDownConfig struct {
    // Enabled controls whether automatic scale-down is enabled
    // Default: true
    // +optional
    Enabled *bool `json:"enabled,omitempty"`

    // EmptyNodeTTL is how long a node must be empty before scale-down
    // Default: 5 minutes
    // +optional
    EmptyNodeTTL *metav1.Duration `json:"emptyNodeTTL,omitempty"`

    // DrainTimeout is the maximum time to wait for node drain
    // Default: 5 minutes
    // +optional
    DrainTimeout *metav1.Duration `json:"drainTimeout,omitempty"`
}
```

### 1.5 PreWarmConfig

```go
// PreWarmConfig configures the pre-warming lifecycle
type PreWarmConfig struct {
    // Timeout is how long to wait for an instance to self-stop
    // Default: 10 minutes
    // +optional
    Timeout *metav1.Duration `json:"timeout,omitempty"`

    // TimeoutAction is what to do if the instance doesn't self-stop
    // +kubebuilder:validation:Enum=stop;terminate
    // Default: "stop"
    // +optional
    TimeoutAction *TimeoutAction `json:"timeoutAction,omitempty"`
}

// TimeoutAction defines what happens when pre-warming times out
// +kubebuilder:validation:Enum=stop;terminate
type TimeoutAction string

const (
    TimeoutActionStop      TimeoutAction = "stop"
    TimeoutActionTerminate TimeoutAction = "terminate"
)
```

### 1.6 NodePoolStatus

```go
// NodePoolStatus defines the observed state of NodePool
type NodePoolStatus struct {
    // Conditions represent the latest available observations
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // ObservedGeneration is the last observed generation
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // Warmup is the count of nodes currently initializing
    Warmup int32 `json:"warmup,omitempty"`

    // Standby is the count of nodes in stopped/standby state
    Standby int32 `json:"standby,omitempty"`

    // Running is the count of nodes actively running pods
    Running int32 `json:"running,omitempty"`

    // Total is the total node count (warmup + standby + running)
    Total int32 `json:"total,omitempty"`

    // LastReconcileTime is when the pool was last reconciled
    LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}
```

### 1.7 Validation Rules

| Rule | Constraint |
|------|------------|
| minStandby <= poolSize | MinStandby cannot exceed PoolSize |
| poolSize >= 1 | Pool must have at least 1 node capacity |
| At least one subnet | SubnetIDs must have at least 1 entry |
| At least one security group | SecurityGroupIDs must have at least 1 entry |

---

## 2. NodeState (Internal Enum)

Stratos-managed nodes transition through these states:

```go
// NodeState represents the lifecycle state of a Stratos-managed node
type NodeState string

const (
    // NodeStateWarmup - Instance launched, waiting for self-stop
    NodeStateWarmup NodeState = "warmup"

    // NodeStateStandby - Instance stopped, ready to start instantly
    NodeStateStandby NodeState = "standby"

    // NodeStateRunning - Instance running, serving pods
    NodeStateRunning NodeState = "running"

    // NodeStateTerminating - Node being drained/stopped
    NodeStateTerminating NodeState = "terminating"
)
```

### State Transitions

```
[Launch] ──► warmup ──► standby ◄──► running ──► [Terminate]
                │                        │
                │ (timeout)              │ (scale-down)
                ▼                        ▼
           [stop/terminate]         terminating ──► standby
```

| From | To | Trigger |
|------|-----|---------|
| - | warmup | NodePool creation/replenishment |
| warmup | standby | Instance self-stops |
| warmup | terminated | Timeout (if action=terminate) |
| warmup | standby | Timeout (if action=stop) |
| standby | running | Pending pods detected |
| running | terminating | Empty node TTL exceeded |
| running | terminating | MaxNodeRuntime exceeded |
| terminating | standby | Drain complete, instance stopped |

---

## 3. Node Labels (Kubernetes Node Object)

Stratos uses labels on Kubernetes Node objects to track state:

```yaml
labels:
  # Pool membership
  stratos.sh/pool: "pool-name"

  # Current state
  stratos.sh/state: "warmup|standby|running|terminating"

  # Instance ID for cloud provider mapping
  stratos.sh/instance-id: "i-1234567890abcdef0"

  # Timestamp when node entered current state
  stratos.sh/state-since: "2026-01-19T12:00:00Z"
```

### Annotations

```yaml
annotations:
  # Pre-warming completion timestamp
  stratos.sh/warmup-completed: "2026-01-19T12:05:00Z"

  # Last start time (for maxNodeRuntime tracking)
  stratos.sh/last-started: "2026-01-19T12:10:00Z"

  # Scale-down candidate since
  stratos.sh/scale-down-candidate-since: "2026-01-19T14:00:00Z"
```

---

## 4. Instance Tags (Cloud Provider)

EC2 instances are tagged for management:

```
managed-by          = "stratos"
stratos.sh/pool     = "pool-name"
stratos.sh/cluster  = "cluster-name"
stratos.sh/state    = "warmup|standby|running"
Name                = "stratos-pool-name-abc123"
```

---

## 5. CloudProvider Interface

```go
// CloudProvider defines the interface for cloud instance operations
type CloudProvider interface {
    // LaunchInstance creates a new instance from the template
    LaunchInstance(ctx context.Context, cfg *LaunchConfig) (*Instance, error)

    // StartInstance starts a stopped instance
    StartInstance(ctx context.Context, instanceID string) error

    // StopInstance stops a running instance
    StopInstance(ctx context.Context, instanceID string, force bool) error

    // TerminateInstance terminates an instance
    TerminateInstance(ctx context.Context, instanceID string) error

    // GetInstanceState returns the current state of an instance
    GetInstanceState(ctx context.Context, instanceID string) (InstanceState, error)

    // ListInstances returns instances matching the given tags
    ListInstances(ctx context.Context, tags map[string]string) ([]*Instance, error)

    // UpdateInstanceTags updates tags on an instance
    UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error
}

// Instance represents a cloud compute instance
type Instance struct {
    ID            string
    State         InstanceState
    PrivateIP     string
    PublicIP      string
    LaunchTime    time.Time
    Tags          map[string]string
}

// InstanceState represents cloud instance state
type InstanceState string

const (
    InstanceStatePending      InstanceState = "pending"
    InstanceStateRunning      InstanceState = "running"
    InstanceStateStopping     InstanceState = "stopping"
    InstanceStateStopped      InstanceState = "stopped"
    InstanceStateShuttingDown InstanceState = "shutting-down"
    InstanceStateTerminated   InstanceState = "terminated"
)
```

---

## 6. Condition Types

NodePool status conditions:

| Type | Description |
|------|-------------|
| `Ready` | Pool has minStandby nodes available and is operating normally |
| `Reconciling` | Pool is actively being reconciled |
| `Degraded` | Pool cannot meet minStandby due to errors |
| `ScaleUpInProgress` | Scale-up operation in progress |
| `ScaleDownInProgress` | Scale-down operation in progress |

### Condition Structure

```go
metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    ObservedGeneration: nodePool.Generation,
    LastTransitionTime: metav1.Now(),
    Reason:             "PoolReady",
    Message:            "Pool has 5 standby nodes available",
}
```

---

## 7. Example NodePool Resource

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: worker-pool
  namespace: stratos-system
spec:
  poolSize: 20
  minStandby: 5

  template:
    labels:
      node-type: worker
      stratos.sh/pool: worker-pool
    taints:
      - key: "stratos.sh/pool"
        value: "worker-pool"
        effect: "NoSchedule"
    cloudProvider:
      provider: aws
      aws:
        instanceType: m5.xlarge
        ami: ami-0123456789abcdef0
        subnetIds:
          - subnet-abc123
          - subnet-def456
        securityGroupIds:
          - sg-abc123
        iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/KubernetesNode
        blockDeviceMappings:
          - deviceName: /dev/xvda
            volumeSize: 100
            volumeType: gp3
            encrypted: true
        tags:
          Environment: production
          Team: platform

  scaleDown:
    enabled: true
    emptyNodeTTL: 5m
    drainTimeout: 5m

  preWarm:
    timeout: 10m
    timeoutAction: stop

  maxNodeRuntime: 168h  # 7 days
  reconciliationInterval: 30s

status:
  conditions:
    - type: Ready
      status: "True"
      reason: PoolReady
      message: "Pool has 5 standby nodes available"
      lastTransitionTime: "2026-01-19T12:00:00Z"
      observedGeneration: 1
  observedGeneration: 1
  warmup: 0
  standby: 5
  running: 10
  total: 15
  lastReconcileTime: "2026-01-19T12:30:00Z"
```
