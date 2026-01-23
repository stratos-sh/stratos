# Design: Startup Taints

**Change ID**: add-startup-taints

## Overview

This document describes the technical design for implementing startup taints in Stratos, enabling nodes to block pod scheduling until the CNI is fully initialized.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Node Startup Flow                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Stratos starts stopped instance                                      │
│     └─> Instance state: stopped → running                                │
│                                                                          │
│  2. Kubelet starts with --register-with-taints                           │
│     └─> Node has startup taint: node.eks.amazonaws.com/not-ready         │
│                                                                          │
│  3. Stratos marks node as "running" but keeps startup taints             │
│     └─> Node state: standby → running                                    │
│     └─> Node still tainted (no pods scheduled)                           │
│                                                                          │
│  4. DaemonSets schedule (they tolerate all taints)                       │
│     └─> aws-node pod starts on the node                                  │
│                                                                          │
│  5. Stratos watches for CNI readiness                                    │
│     └─> Checks: aws-node pod Running + Ready                             │
│                                                                          │
│  6. Stratos removes startup taints                                       │
│     └─> Regular pods can now schedule                                    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## API Changes

### NodeTemplate (api/v1alpha1/nodepool_types.go)

```go
// StartupTaintRemovalMode determines how startup taints are removed
type StartupTaintRemovalMode string

const (
    // StartupTaintRemovalWhenNetworkReady - Stratos removes taints when network conditions indicate ready
    StartupTaintRemovalWhenNetworkReady StartupTaintRemovalMode = "WhenNetworkReady"

    // StartupTaintRemovalExternal - Stratos waits for external controller to remove taints
    StartupTaintRemovalExternal StartupTaintRemovalMode = "External"
)

// NodeTemplate defines the template for nodes in this pool
type NodeTemplate struct {
    // Labels to apply to nodes
    // +optional
    Labels map[string]string `json:"labels,omitempty"`

    // Taints to apply to nodes (permanent taints for workload isolation)
    // +optional
    Taints []corev1.Taint `json:"taints,omitempty"`

    // StartupTaints are taints applied during node startup that block pod scheduling
    // until the node is fully ready. These should match the taints configured in
    // kubelet --register-with-taints.
    // +optional
    StartupTaints []corev1.Taint `json:"startupTaints,omitempty"`

    // StartupTaintRemoval determines how startup taints are removed:
    // - WhenNetworkReady (default): Stratos removes taints when network conditions indicate CNI is ready
    // - External: Stratos waits for taints to be removed by CNI plugin or external controller
    // +kubebuilder:validation:Enum=WhenNetworkReady;External
    // +kubebuilder:default=WhenNetworkReady
    // +optional
    StartupTaintRemoval StartupTaintRemovalMode `json:"startupTaintRemoval,omitempty"`

    // CloudProvider specifies the cloud provider configuration
    CloudProvider CloudProviderConfig `json:"cloudProvider"`
}
```

## Implementation Components

### 1. Network Readiness Checker (CNI-Agnostic)

New file: `internal/controller/network_readiness.go`

```go
// NetworkReadinessChecker checks if the network/CNI is ready on a node
// using standard Kubernetes node conditions (CNI-agnostic).
type NetworkReadinessChecker struct{}

// IsReady returns true if network conditions indicate the CNI is ready.
// Supports multiple CNI plugins through standard node conditions:
// - EKS (VPC CNI): NetworkingReady=True (set by eks-node-monitoring-agent)
// - Cilium/Calico: NetworkUnavailable=False (set by CNI plugin)
func (c *NetworkReadinessChecker) IsReady(node *corev1.Node) bool {
    for _, cond := range node.Status.Conditions {
        // EKS: NetworkingReady condition set by eks-node-monitoring-agent
        // Indicates IPAMD is connected and networking is functional
        if cond.Type == "NetworkingReady" && cond.Status == corev1.ConditionTrue {
            return true
        }

        // Standard K8s: NetworkUnavailable condition set by CNI plugins
        // Cilium sets reason "CiliumIsUp", Calico sets "CalicoIsUp"
        if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionFalse {
            return true
        }
    }
    return false
}

// GetNetworkConditionReason returns the reason from the network condition for logging
func (c *NetworkReadinessChecker) GetNetworkConditionReason(node *corev1.Node) string {
    for _, cond := range node.Status.Conditions {
        if cond.Type == "NetworkingReady" && cond.Status == corev1.ConditionTrue {
            return cond.Reason // e.g., "NetworkingIsReady"
        }
        if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionFalse {
            return cond.Reason // e.g., "CiliumIsUp", "CalicoIsUp"
        }
    }
    return ""
}
```

### 2. Modified StartNode Flow

In `internal/controller/manager.go`:

```go
func (m *NodeManager) StartNode(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
    // ... existing code ...

    // Prepare node for running - but preserve startup taints
    if err := m.prepareNodeForRunning(ctx, pool, node); err != nil {
        return err
    }

    // ... rest of existing code ...
}

// Modified to preserve startup taints
func (m *NodeManager) prepareNodeForRunning(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
    // Uncordon the node
    node.Spec.Unschedulable = false

    // Remove standby taint
    node.Spec.Taints = removeTaint(node.Spec.Taints, TaintKeyStandby)

    // NOTE: Do NOT remove startup taints here
    // They will be removed by removeStartupTaintsIfReady() after CNI is ready
}
```

### 3. Startup Taint Removal

In `internal/controller/manager.go`:

```go
// ProcessStartupTaints handles startup taint removal based on the configured mode.
// Returns (taintsRemoved, error).
func (m *NodeManager) ProcessStartupTaints(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) (bool, error) {
    logger := log.FromContext(ctx)
    startupTaints := pool.Spec.Template.StartupTaints
    if len(startupTaints) == 0 {
        return true, nil // No startup taints configured
    }

    // Check if node still has any startup taints
    hasStartupTaints := m.nodeHasStartupTaints(node, startupTaints)
    if !hasStartupTaints {
        return true, nil // Already removed (by Stratos or external controller)
    }

    removalMode := pool.Spec.Template.StartupTaintRemoval
    if removalMode == "" {
        removalMode = stratosv1alpha1.StartupTaintRemovalWhenNetworkReady // default
    }

    switch removalMode {
    case stratosv1alpha1.StartupTaintRemovalWhenNetworkReady:
        return m.removeStartupTaintsWhenNetworkReady(ctx, pool, node, startupTaints)

    case stratosv1alpha1.StartupTaintRemovalExternal:
        // In External mode, just check if taints are still present
        // We don't remove them - external controller should
        logger.V(1).Info("Waiting for external controller to remove startup taints",
            "node", node.Name, "mode", "External")
        return false, nil
    }

    return false, nil
}

// removeStartupTaintsWhenNetworkReady removes taints when network conditions indicate ready.
func (m *NodeManager) removeStartupTaintsWhenNetworkReady(
    ctx context.Context,
    pool *stratosv1alpha1.NodePool,
    node *corev1.Node,
    startupTaints []corev1.Taint,
) (bool, error) {
    logger := log.FromContext(ctx)

    // Check network readiness using CNI-agnostic conditions
    checker := &NetworkReadinessChecker{}
    if !checker.IsReady(node) {
        logger.V(1).Info("Network not ready, waiting to remove startup taints", "node", node.Name)
        return false, nil
    }

    reason := checker.GetNetworkConditionReason(node)
    logger.Info("Network ready, removing startup taints",
        "node", node.Name, "reason", reason)

    // Remove startup taints
    patch := client.MergeFrom(node.DeepCopy())
    for _, st := range startupTaints {
        node.Spec.Taints = removeTaintByKeyAndEffect(node.Spec.Taints, st.Key, st.Effect)
    }
    if err := m.client.Patch(ctx, node, patch); err != nil {
        return false, fmt.Errorf("failed to remove startup taints: %w", err)
    }

    // Record metrics and event
    metrics.RecordStartupTaintRemoval(pool.Name, "network_ready", "success")
    if m.recorder != nil {
        m.recorder.Eventf(pool, corev1.EventTypeNormal, "StartupTaintsRemoved",
            "Removed startup taints from node %s after network ready (%s)", node.Name, reason)
    }

    return true, nil
}

// nodeHasStartupTaints checks if the node has any of the configured startup taints.
func (m *NodeManager) nodeHasStartupTaints(node *corev1.Node, startupTaints []corev1.Taint) bool {
    for _, st := range startupTaints {
        if hasTaintWithKeyAndEffect(node.Spec.Taints, st.Key, st.Effect) {
            return true
        }
    }
    return false
}
```

### 4. Reconciliation Loop Integration

In `internal/controller/nodepool_controller.go`:

```go
func (r *NodePoolReconciler) reconcileRunningNodes(ctx context.Context, pool *stratosv1alpha1.NodePool) error {
    runningNodes, err := r.getRunningNodes(ctx, pool.Name)
    if err != nil {
        return err
    }

    for _, node := range runningNodes {
        // Check and remove startup taints if CNI is ready
        if _, err := r.nodeManager.RemoveStartupTaintsIfReady(ctx, pool, &node); err != nil {
            logger.Error(err, "Failed to check/remove startup taints", "node", node.Name)
        }

        // ... existing running node reconciliation ...
    }

    return nil
}
```

## State Diagram

```
                    ┌──────────┐
                    │  Warmup  │
                    └────┬─────┘
                         │ instance self-stops
                         ▼
                    ┌──────────┐
                    │ Standby  │ (cordoned, standby taint)
                    └────┬─────┘
                         │ scale-up triggered
                         ▼
                    ┌──────────────────────────────────┐
                    │ Running (with startup taints)    │
                    │ - uncordoned                     │
                    │ - standby taint removed          │
                    │ - startup taints preserved       │
                    │ - DaemonSets can schedule        │
                    │ - regular pods blocked           │
                    └────┬─────────────────────────────┘
                         │ CNI ready (aws-node Running)
                         ▼
                    ┌──────────────────────────────────┐
                    │ Running (fully ready)            │
                    │ - startup taints removed         │
                    │ - all pods can schedule          │
                    └──────────────────────────────────┘
```

## Configuration Example

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 5
  template:
    labels:
      stratos.sh/pool: workers
    # Permanent taints for workload isolation
    taints:
      - key: dedicated
        value: workers
        effect: NoSchedule
    # Startup taints - removed by Stratos when CNI is ready
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
    cloudProvider:
      provider: aws
      aws:
        # ... other config ...
        userData: |
          #!/bin/bash
          /etc/eks/bootstrap.sh my-cluster \
            --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
          # ... warmup script ...
```

## Timeout Handling

If the CNI never becomes ready, we need a timeout mechanism:

```go
const StartupTaintTimeout = 2 * time.Minute

func (m *NodeManager) checkStartupTaintTimeout(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
    // Check how long since node started
    startedAt := node.Annotations[AnnotationLastStarted]
    if startedAt == "" {
        return nil
    }

    started, err := time.Parse(time.RFC3339, startedAt)
    if err != nil {
        return nil
    }

    if time.Since(started) > StartupTaintTimeout {
        // Log warning but remove taints anyway to avoid blocking forever
        logger.Info("Startup taint timeout, removing taints despite CNI not ready",
            "node", node.Name, "timeout", StartupTaintTimeout)

        // Emit warning event
        m.recorder.Eventf(pool, corev1.EventTypeWarning, "StartupTaintTimeout",
            "Node %s startup taints removed after timeout (CNI may not be ready)", node.Name)

        return m.forceRemoveStartupTaints(ctx, pool, node)
    }

    return nil
}
```

## Testing Strategy

1. **Unit Tests**:
   - CNI readiness checker with mock pod list
   - Startup taint removal logic
   - Timeout handling

2. **Integration Tests**:
   - Node startup with startup taints
   - Taint removal after mock CNI ready
   - Timeout scenario

3. **E2E Tests** (manual):
   - Deploy NodePool with startupTaints
   - Verify pods don't schedule until aws-node is ready
   - Verify no "connection refused" errors

## Metrics

New metrics to add:

- `stratos_startup_taint_removal_total{pool, result}` - Counter for taint removals (success/timeout)
- `stratos_startup_taint_duration_seconds{pool}` - Histogram of time from node start to taint removal
