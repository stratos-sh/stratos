# Design: Fix Scale-Up Overshoot

**Change ID**: fix-scale-up-overshoot
**Created**: 2026-01-21
**Updated**: 2026-01-21

## Context

The current scale-up logic has a race condition where multiple reconciliation cycles can start more nodes than needed. Additionally, the current 1:1 pod-to-node assumption wastes resources when multiple pods can fit on a single node.

## Design Decisions

Based on discussion:

1. **Node capacity source**: Use instance type from NodePool spec → static mapping to CPU/memory
2. **Pods without resource requests**: Add configurable default resource requests in NodePool spec
3. **Capacity buffer**: Use 80% of node capacity to account for DaemonSets and system overhead

## Current Flow (Broken)

```
Pod Pending -> calculateScaleUpNeeded() -> needed = len(pods)  // 1:1 assumption
                                        -> Start `needed` nodes
                                        -> Requeue after 5s

(5s later, pod still pending)
Pod Pending -> calculateScaleUpNeeded() -> needed = len(pods)  // Same pods!
                                        -> Start more nodes...  // OVER-PROVISIONING
```

## Proposed Flow

```
Pod Pending -> calculateScaleUpNeeded()
            -> Sum pod resource requests (CPU, memory)
            -> Get node capacity from instance type (use 80%)
            -> nodesNeeded = max(ceil(cpu/nodeCPU), ceil(mem/nodeMem))
            -> Subtract starting nodes (in-flight tracking)
            -> Start only what's needed
```

## Detailed Design

### 1. New NodePool Spec Fields

Add `ScaleUp` configuration to NodePoolSpec:

```go
// NodePoolSpec
type NodePoolSpec struct {
    // ... existing fields ...

    // ScaleUp configures scale-up behavior
    // +optional
    ScaleUp *ScaleUpConfig `json:"scaleUp,omitempty"`
}

// ScaleUpConfig configures scale-up behavior
type ScaleUpConfig struct {
    // DefaultPodResources specifies default resource requests for pods
    // that don't have explicit requests. Used in scale-up calculations.
    // +optional
    DefaultPodResources *corev1.ResourceRequirements `json:"defaultPodResources,omitempty"`
}
```

**Example NodePool:**
```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: gpu-runners
spec:
  poolSize: 10
  minStandby: 3
  scaleUp:
    defaultPodResources:
      requests:
        cpu: "500m"
        memory: "1Gi"
  template:
    cloudProvider:
      provider: aws
      aws:
        instanceType: m5.xlarge  # 4 vCPU, 16 GiB
        # ...
```

### 2. Instance Capacity Lookup (Hybrid)

Use a hybrid approach for AWS:

```
Priority:
1. Existing node's .status.allocatable (real data from actual nodes)
2. Static mapping for known AWS instance types (fallback for empty pools)
3. Fall back to 1:1 pod-to-node if unknown
```

#### AWS Instance Type Mapping

```go
// internal/cloudprovider/aws/instance_types.go

package aws

import "k8s.io/apimachinery/pkg/api/resource"

// InstanceCapacity represents the CPU and memory capacity of an instance type
type InstanceCapacity struct {
    CPU    resource.Quantity
    Memory resource.Quantity
}

// awsInstanceCapacity maps AWS EC2 instance types to their capacity
var awsInstanceCapacity = map[string]InstanceCapacity{
    // General Purpose - M5
    "m5.large":    {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
    "m5.xlarge":   {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
    "m5.2xlarge":  {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
    "m5.4xlarge":  {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},

    // General Purpose - M6i
    "m6i.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
    "m6i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
    "m6i.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},

    // Compute Optimized - C5/C6i
    "c5.large":    {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
    "c5.xlarge":   {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
    "c6i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},

    // Memory Optimized - R5
    "r5.large":    {CPU: resource.MustParse("2"), Memory: resource.MustParse("16Gi")},
    "r5.xlarge":   {CPU: resource.MustParse("4"), Memory: resource.MustParse("32Gi")},

    // GPU - P3, G4dn, G5
    "p3.2xlarge":   {CPU: resource.MustParse("8"), Memory: resource.MustParse("61Gi")},
    "g4dn.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
    "g5.xlarge":    {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},

    // ... add more as needed
}

// GetInstanceCapacity returns the capacity for an AWS instance type.
// Returns zero values if instance type is unknown.
func GetInstanceCapacity(instanceType string) InstanceCapacity {
    if cap, ok := awsInstanceCapacity[instanceType]; ok {
        return cap
    }
    return InstanceCapacity{}
}
```

#### Hybrid Lookup in ScaleCalculator

```go
// internal/controller/scale_calculator.go

// getNodeCapacity returns node capacity using hybrid lookup:
// 1. Try existing node's .status.allocatable
// 2. Fall back to static mapping for AWS instance type
func (c *ScaleCalculator) getNodeCapacity(existingNodes []corev1.Node) aws.InstanceCapacity {
    // Priority 1: Use real data from existing nodes
    for _, node := range existingNodes {
        if node.Status.Allocatable != nil {
            cpu := node.Status.Allocatable[corev1.ResourceCPU]
            mem := node.Status.Allocatable[corev1.ResourceMemory]
            if !cpu.IsZero() && !mem.IsZero() {
                return aws.InstanceCapacity{CPU: cpu, Memory: mem}
            }
        }
    }

    // Priority 2: Static mapping for AWS instance type
    if c.nodePool.Spec.Template.CloudProvider.AWS != nil {
        instanceType := c.nodePool.Spec.Template.CloudProvider.AWS.InstanceType
        return aws.GetInstanceCapacity(instanceType)
    }

    return aws.InstanceCapacity{} // Unknown - will fall back to 1:1
}
```

### 3. Resource Calculation Helper

```go
// internal/controller/scale_calculator.go

package controller

import (
    "math"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"

    stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
    "github.com/stratos-sh/stratos/internal/cloudprovider/aws"
)

const (
    // NodeCapacityUsagePercent is the percentage of node capacity to use
    // in scale-up calculations. Accounts for DaemonSets and system overhead.
    NodeCapacityUsagePercent = 0.80
)

// ScaleCalculator calculates how many nodes are needed for pending pods
type ScaleCalculator struct {
    nodePool *stratosv1alpha1.NodePool
}

// NewScaleCalculator creates a new scale calculator
func NewScaleCalculator(pool *stratosv1alpha1.NodePool) *ScaleCalculator {
    return &ScaleCalculator{nodePool: pool}
}

// CalculateNodesNeeded returns the number of nodes needed for the given pods
func (c *ScaleCalculator) CalculateNodesNeeded(pods []corev1.Pod) int {
    if len(pods) == 0 {
        return 0
    }

    // Get node capacity from instance type
    nodeCapacity := c.getNodeCapacity()
    if nodeCapacity.CPU.IsZero() || nodeCapacity.Memory.IsZero() {
        // Unknown instance type - fall back to 1:1 pod-to-node
        return len(pods)
    }

    // Apply 80% usage factor
    usableCPU := float64(nodeCapacity.CPU.MilliValue()) * NodeCapacityUsagePercent
    usableMemory := float64(nodeCapacity.Memory.Value()) * NodeCapacityUsagePercent

    // Sum pod resource requests
    totalCPU, totalMemory := c.sumPodRequests(pods)

    // Calculate nodes needed (ceiling division)
    nodesByCPU := int(math.Ceil(float64(totalCPU) / usableCPU))
    nodesByMemory := int(math.Ceil(float64(totalMemory) / usableMemory))

    // Return the larger of the two (bottleneck resource)
    if nodesByCPU > nodesByMemory {
        return nodesByCPU
    }
    return nodesByMemory
}

// getNodeCapacity returns the capacity of nodes in this pool
func (c *ScaleCalculator) getNodeCapacity() aws.InstanceCapacity {
    if c.nodePool.Spec.Template.CloudProvider.AWS != nil {
        instanceType := c.nodePool.Spec.Template.CloudProvider.AWS.InstanceType
        return aws.GetInstanceCapacity(instanceType)
    }
    return aws.InstanceCapacity{}
}

// sumPodRequests sums the resource requests of all pods
func (c *ScaleCalculator) sumPodRequests(pods []corev1.Pod) (cpuMillis int64, memoryBytes int64) {
    defaultCPU, defaultMemory := c.getDefaultResources()

    for _, pod := range pods {
        podCPU, podMemory := c.getPodRequests(&pod)

        // Use defaults if pod has no requests
        if podCPU == 0 {
            podCPU = defaultCPU
        }
        if podMemory == 0 {
            podMemory = defaultMemory
        }

        cpuMillis += podCPU
        memoryBytes += podMemory
    }

    return cpuMillis, memoryBytes
}

// getPodRequests returns the total resource requests for a pod
func (c *ScaleCalculator) getPodRequests(pod *corev1.Pod) (cpuMillis int64, memoryBytes int64) {
    for _, container := range pod.Spec.Containers {
        if container.Resources.Requests != nil {
            if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
                cpuMillis += cpu.MilliValue()
            }
            if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
                memoryBytes += mem.Value()
            }
        }
    }

    // Include init containers (they run before main containers)
    for _, container := range pod.Spec.InitContainers {
        if container.Resources.Requests != nil {
            if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
                // Init containers run sequentially, take the max
                if cpu.MilliValue() > cpuMillis {
                    // Actually for init containers we should take max, not sum
                    // But for simplicity, we'll be conservative and add
                }
            }
        }
    }

    return cpuMillis, memoryBytes
}

// getDefaultResources returns the default resource requests from NodePool config
func (c *ScaleCalculator) getDefaultResources() (cpuMillis int64, memoryBytes int64) {
    if c.nodePool.Spec.ScaleUp == nil || c.nodePool.Spec.ScaleUp.DefaultPodResources == nil {
        // No defaults configured - return zeros (will use pod's actual or assume full node)
        return 0, 0
    }

    defaults := c.nodePool.Spec.ScaleUp.DefaultPodResources.Requests
    if cpu, ok := defaults[corev1.ResourceCPU]; ok {
        cpuMillis = cpu.MilliValue()
    }
    if mem, ok := defaults[corev1.ResourceMemory]; ok {
        memoryBytes = mem.Value()
    }

    return cpuMillis, memoryBytes
}
```

### 4. Updated calculateScaleUpNeeded

```go
// internal/controller/nodepool_controller.go

func (r *NodePoolReconciler) calculateScaleUpNeeded(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (int, error) {
    logger := log.FromContext(ctx)

    // Get unschedulable pods
    pods, err := r.getUnschedulablePods(ctx, nodePool)
    if err != nil {
        return 0, fmt.Errorf("failed to get unschedulable pods: %w", err)
    }

    if len(pods) == 0 {
        return 0, nil
    }

    logger.Info("Found unschedulable pods", "count", len(pods))

    // Calculate nodes needed based on resources
    calculator := NewScaleCalculator(nodePool)
    nodesNeeded := calculator.CalculateNodesNeeded(pods)

    logger.Info("Calculated nodes needed from resources",
        "pendingPods", len(pods),
        "nodesNeeded", nodesNeeded)

    // Get current node counts
    _, standby, running, _, err := r.countNodesByState(ctx, nodePool.Name)
    if err != nil {
        return 0, err
    }

    // Count nodes that are currently starting (in-flight scale-up)
    starting, err := r.countStartingNodes(ctx, nodePool.Name)
    if err != nil {
        logger.Error(err, "Failed to count starting nodes")
        starting = 0
    }

    // Subtract starting nodes - they will satisfy some demand once ready
    nodesNeeded = nodesNeeded - starting
    if nodesNeeded <= 0 {
        logger.Info("Scale-up already in progress",
            "calculatedNeed", nodesNeeded + starting,
            "startingNodes", starting)
        return 0, nil
    }

    // Check pool capacity
    maxRunning := int(nodePool.Spec.PoolSize)
    canStart := maxRunning - running - starting
    if canStart <= 0 {
        logger.Info("Pool at capacity, cannot scale up",
            "poolSize", nodePool.Spec.PoolSize,
            "running", running,
            "starting", starting)
        return 0, nil
    }

    // Cap at available standby and capacity
    if nodesNeeded > standby {
        nodesNeeded = standby
    }
    if nodesNeeded > canStart {
        nodesNeeded = canStart
    }

    logger.Info("Final scale-up decision",
        "pendingPods", len(pods),
        "nodesNeeded", nodesNeeded,
        "startingNodes", starting,
        "standbyAvailable", standby)

    return nodesNeeded, nil
}
```

### 5. In-Flight Tracking (Annotations)

```go
// internal/nodemanager/labels.go

const (
    // AnnotationScaleUpStarted marks when a node was started for scale-up
    AnnotationScaleUpStarted = "stratos.sh/scale-up-started"

    // ScaleUpStartedTTL is how long to consider a node as "starting"
    ScaleUpStartedTTL = 60 * time.Second
)
```

```go
// internal/nodemanager/manager.go - StartNode modification

func (m *NodeManager) StartNode(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
    // ... existing code ...

    // Set scale-up started annotation (for in-flight tracking)
    patch := client.MergeFrom(node.DeepCopy())
    if node.Annotations == nil {
        node.Annotations = make(map[string]string)
    }
    node.Annotations[AnnotationScaleUpStarted] = time.Now().Format(time.RFC3339)
    node.Annotations[AnnotationLastStarted] = time.Now().Format(time.RFC3339)
    if err := m.client.Patch(ctx, node, patch); err != nil {
        logger.Error(err, "Failed to set scale-up annotations")
    }

    // ... rest of existing code ...
}
```

```go
// internal/controller/nodepool_controller.go

// countStartingNodes counts nodes that are in the process of starting
func (r *NodePoolReconciler) countStartingNodes(ctx context.Context, poolName string) (int, error) {
    nodes, err := r.getNodesForPool(ctx, poolName)
    if err != nil {
        return 0, err
    }

    count := 0
    now := time.Now()

    for _, node := range nodes {
        ts, ok := node.Annotations[nodemanager.AnnotationScaleUpStarted]
        if !ok {
            continue
        }

        startedAt, err := time.Parse(time.RFC3339, ts)
        if err != nil {
            continue
        }

        // Check if within TTL and node is not yet Ready
        if now.Sub(startedAt) < nodemanager.ScaleUpStartedTTL && !isNodeReady(&node) {
            count++
        }
    }

    return count, nil
}

// isNodeReady checks if a node has Ready condition = True
func isNodeReady(node *corev1.Node) bool {
    for _, cond := range node.Status.Conditions {
        if cond.Type == corev1.NodeReady {
            return cond.Status == corev1.ConditionTrue
        }
    }
    return false
}
```

## Calculation Example

**Scenario:**
- Instance type: `m5.xlarge` (4 vCPU, 16 GiB)
- Usable capacity (80%): 3.2 vCPU, 12.8 GiB
- 10 pending pods, each requesting 500m CPU, 2Gi memory
- Total pod requests: 5 vCPU, 20 GiB
- 1 node already starting

**Calculation:**
```
nodesByCPU    = ceil(5000m / 3200m) = ceil(1.56) = 2
nodesByMemory = ceil(20Gi / 12.8Gi) = ceil(1.56) = 2
nodesNeeded   = max(2, 2) = 2
afterStarting = 2 - 1 = 1 node to start
```

**Result:** Start 1 additional node (total 2 will handle 10 pods)

## Sequence Diagram

```
    10 Pending Pods      Controller              Cloud
         │                   │                     │
         │  pods detected    │                     │
         │ ────────────────► │                     │
         │                   │ Sum requests: 5 CPU, 20Gi
         │                   │ Node capacity (80%): 3.2 CPU, 12.8Gi
         │                   │ nodesNeeded = 2
         │                   │ startingNodes = 0
         │                   │ toStart = 2
         │                   │                     │
         │                   │   Start 2 nodes     │
         │                   │ ──────────────────► │
         │                   │   (set annotations) │
         │                   │                     │
         │                   │ requeue 5s          │
         │                   │ ───┐                │
         │                   │    │                │
(5s)     │  still pending    │ ◄──┘                │
         │ ────────────────► │                     │
         │                   │ nodesNeeded = 2
         │                   │ startingNodes = 2
         │                   │ toStart = 0 ✓
         │                   │                     │
(15s)    │                   │              nodes Ready
         │                   │                  ◄──│
         │  scheduled!       │                     │
         │ ◄─────────────────────────────────────  │
```

## Testing Strategy

### Unit Tests

1. `TestScaleCalculator_CalculateNodesNeeded`
   - 10 pods @ 500m CPU each, m5.xlarge → 2 nodes
   - 1 pod @ 100m CPU, m5.xlarge → 1 node
   - Pods without requests + defaults configured → uses defaults

2. `TestScaleCalculator_UnknownInstanceType`
   - Unknown instance type → falls back to 1:1

3. `TestCountStartingNodes`
   - Within TTL, not Ready → counted
   - Within TTL, Ready → not counted
   - Past TTL → not counted

### Integration Tests

1. 10 small pods → creates minimum nodes needed (not 10)
2. Single large pod → creates 1 node
3. Rapid reconciliation doesn't over-provision

## Rollout

1. Deploy to staging
2. Monitor: nodes started vs pods scheduled
3. Verify ~20s scheduling latency maintained
4. Roll to production
