# Spec Delta: Startup Taints

**Change**: add-startup-taints
**Base**: openspec/specs/stratos-core/spec.md

---

## ADDED Requirements

### Requirement: FR-053 Startup Taints Configuration

**Priority**: P1
**Status**: Draft

NodePool MUST support configurable `startupTaints` in the node template. These taints are applied during node registration (via kubelet `--register-with-taints`) and removed either by Stratos or an external controller depending on configuration.

```yaml
spec:
  template:
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
```

#### Scenario: NodePool with startup taints
- **Given** a NodePool with `startupTaints` configured
- **When** the NodePool is created
- **Then** the configuration is accepted and stored

#### Scenario: NodePool without startup taints
- **Given** a NodePool without `startupTaints` configured
- **When** a node is started
- **Then** the existing behavior is unchanged (backward compatible)

---

### Requirement: FR-054 Startup Taint Removal Mode

**Priority**: P1
**Status**: Draft

NodePool MUST support configurable `startupTaintRemoval` mode that determines how startup taints are removed:

- `WhenNetworkReady` (default): Stratos removes taints when node network conditions indicate CNI is ready
- `External`: Stratos waits for taints to be removed by an external controller (CNI plugin or custom DaemonSet)

```yaml
spec:
  template:
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        effect: NoSchedule
    startupTaintRemoval: WhenNetworkReady  # or External
```

#### Scenario: WhenNetworkReady mode (default)
- **Given** a NodePool with `startupTaintRemoval: WhenNetworkReady` (or not specified)
- **When** a node is started and network conditions indicate ready
- **Then** Stratos removes the startup taints

#### Scenario: External mode
- **Given** a NodePool with `startupTaintRemoval: External`
- **When** a node is started
- **Then** Stratos does NOT remove startup taints
- **And** Stratos waits for taints to be removed by external controller before considering node fully ready

---

### Requirement: FR-055 Preserve Startup Taints During Scale-Up

**Priority**: P1
**Status**: Draft

When Stratos starts a standby node for scale-up, it MUST NOT remove startup taints. The standby taint is removed (allowing DaemonSets), but startup taints are preserved to block regular pod scheduling.

#### Scenario: Node started with startup taints
- **Given** a NodePool with startup taints configured
- **When** Stratos starts a standby node
- **Then** the node is uncordoned
- **And** the standby taint (`stratos.sh/standby`) is removed
- **And** the startup taints are preserved
- **And** DaemonSet pods can schedule (they tolerate all taints)
- **And** regular pods cannot schedule (blocked by startup taint)

#### Scenario: Node started without startup taints
- **Given** a NodePool without `startupTaints` configured
- **When** Stratos starts a standby node
- **Then** the node is uncordoned
- **And** the standby taint is removed
- **And** regular pods can schedule immediately (existing behavior)

---

### Requirement: FR-056 CNI-Agnostic Network Readiness Detection

**Priority**: P1
**Status**: Draft

When `startupTaintRemoval: WhenNetworkReady`, Stratos MUST detect network readiness using standard Kubernetes node conditions, supporting multiple CNI plugins:

1. **EKS (VPC CNI)**: Check for `NetworkingReady: True` condition (set by EKS Node Monitoring Agent)
2. **Cilium/Calico**: Check for `NetworkUnavailable: False` condition (set by CNI plugin)

```go
func isCNIReady(node *corev1.Node) bool {
    for _, cond := range node.Status.Conditions {
        // EKS: NetworkingReady set by eks-node-monitoring-agent
        if cond.Type == "NetworkingReady" && cond.Status == ConditionTrue {
            return true
        }
        // Standard K8s: NetworkUnavailable set by CNI (Cilium, Calico)
        if cond.Type == "NetworkUnavailable" && cond.Status == ConditionFalse {
            return true
        }
    }
    return false
}
```

#### Scenario: EKS with VPC CNI
- **Given** a running node on EKS with VPC CNI
- **When** Stratos checks network readiness
- **Then** it checks for `NetworkingReady: True` condition
- **And** CNI is considered ready when condition is True with reason `NetworkingIsReady`

#### Scenario: Cluster with Cilium CNI
- **Given** a running node with Cilium CNI
- **When** Stratos checks network readiness
- **Then** it checks for `NetworkUnavailable: False` condition
- **And** CNI is considered ready when condition is False with reason `CiliumIsUp`

#### Scenario: Cluster with Calico CNI
- **Given** a running node with Calico CNI
- **When** Stratos checks network readiness
- **Then** it checks for `NetworkUnavailable: False` condition
- **And** CNI is considered ready when condition is False with reason `CalicoIsUp`

#### Scenario: No network condition found
- **Given** a running node without recognized network conditions
- **When** Stratos checks network readiness
- **Then** CNI is considered not ready
- **And** Stratos continues checking until timeout

---

### Requirement: FR-057 Startup Taint Removal After Network Ready

**Priority**: P1
**Status**: Draft

When `startupTaintRemoval: WhenNetworkReady`, Stratos MUST remove startup taints from a node once network readiness is detected.

#### Scenario: Taints removed when network ready
- **Given** a running node with startup taints and network not ready
- **When** network conditions indicate ready (NetworkingReady=True or NetworkUnavailable=False)
- **And** Stratos reconciles the node
- **Then** Stratos removes all startup taints configured in the NodePool
- **And** regular pods can now schedule to the node

#### Scenario: Multiple startup taints
- **Given** a NodePool with multiple startup taints configured
- **When** network becomes ready
- **Then** all configured startup taints are removed in a single operation

#### Scenario: Taint already removed externally
- **Given** a running node where startup taints were removed externally
- **When** Stratos reconciles the node
- **Then** Stratos does not error (no-op)

---

### Requirement: FR-058 External Mode Behavior

**Priority**: P1
**Status**: Draft

When `startupTaintRemoval: External`, Stratos MUST wait for startup taints to be removed by an external controller before considering the node fully initialized.

#### Scenario: Taints removed by Cilium
- **Given** a NodePool with `startupTaintRemoval: External`
- **And** startup taint `node.cilium.io/agent-not-ready`
- **When** Cilium agent becomes ready on the node
- **Then** Cilium removes the taint
- **And** Stratos detects taint removal
- **And** Stratos considers node fully ready

#### Scenario: Taints removed by custom DaemonSet
- **Given** a NodePool with `startupTaintRemoval: External`
- **And** a custom DaemonSet that removes taints when ready
- **When** the DaemonSet removes the startup taints
- **Then** Stratos detects taint removal
- **And** Stratos considers node fully ready

#### Scenario: External taints not removed (timeout)
- **Given** a NodePool with `startupTaintRemoval: External`
- **And** no controller removes the startup taints
- **When** the startup taint timeout is reached
- **Then** Stratos emits a Warning event
- **And** node remains tainted (Stratos does not force remove in External mode)

---

### Requirement: FR-059 Startup Taint Timeout

**Priority**: P1
**Status**: Draft

Stratos MUST implement a timeout for startup taint removal. Behavior on timeout depends on the removal mode:

- `WhenNetworkReady`: Force remove taints after timeout (with warning)
- `External`: Emit warning but do NOT remove taints

Default timeout: 2 minutes from node start.

#### Scenario: Timeout in WhenNetworkReady mode
- **Given** a running node with startup taints and `startupTaintRemoval: WhenNetworkReady`
- **And** 2 minutes have passed since the node was started
- **And** network conditions still not ready
- **When** Stratos reconciles the node
- **Then** Stratos removes the startup taints anyway
- **And** Stratos emits a Warning event indicating the timeout

#### Scenario: Timeout in External mode
- **Given** a running node with startup taints and `startupTaintRemoval: External`
- **And** 2 minutes have passed since the node was started
- **And** taints still present
- **When** Stratos reconciles the node
- **Then** Stratos does NOT remove the startup taints
- **And** Stratos emits a Warning event indicating external controller has not removed taints

#### Scenario: Network ready before timeout
- **Given** a running node with startup taints
- **And** network becomes ready within 30 seconds
- **When** Stratos reconciles the node
- **Then** startup taints are removed promptly
- **And** no timeout warning is emitted

---

### Requirement: FR-060 Startup Taint Metrics

**Priority**: P2
**Status**: Draft

Stratos MUST expose metrics for startup taint operations.

#### Scenario: Taint removal metric
- **Given** startup taints are removed from a node
- **When** the operation completes
- **Then** `stratos_startup_taint_removal_total{pool, mode, result}` is incremented
- **And** `mode` is "network_ready" or "external"
- **And** `result` is "success", "timeout", or "external"

#### Scenario: Taint removal duration
- **Given** startup taints are removed from a node
- **When** the operation completes
- **Then** `stratos_startup_taint_duration_seconds{pool}` records the time from node start to taint removal

---

### Requirement: FR-061 Startup Taint Events

**Priority**: P2
**Status**: Draft

Stratos MUST emit Kubernetes events for significant startup taint operations.

#### Scenario: Taints removed after network ready
- **Given** startup taints are removed after network conditions indicate ready
- **When** the operation completes
- **Then** a Normal event "StartupTaintsRemoved" is emitted with message indicating network ready

#### Scenario: Taints removed due to timeout
- **Given** startup taints are removed due to timeout (WhenNetworkReady mode)
- **When** the operation completes
- **Then** a Warning event "StartupTaintTimeout" is emitted on the NodePool

#### Scenario: External removal timeout warning
- **Given** startup taints not removed by external controller within timeout
- **When** timeout is reached
- **Then** a Warning event "StartupTaintExternalTimeout" is emitted indicating external controller has not removed taints

---

## MODIFIED Requirements

#### FR-018: Fast Node Ready

**Priority**: P1
**Status**: Implemented

Started nodes MUST become Ready within seconds (not minutes). With startup taints, "ready for pods" includes startup taint removal.

#### Scenario: Quick startup
- **Given** a standby node is started
- **When** it transitions to running
- **Then** kubelet becomes Ready within 30 seconds

#### Scenario: Quick startup with startup taints (WhenNetworkReady)
- **Given** a standby node is started with startup taints and `startupTaintRemoval: WhenNetworkReady`
- **When** it transitions to running
- **Then** kubelet becomes Ready within 30 seconds
- **And** startup taints are removed within 60 seconds (after network ready)
- **And** total time from start to pods-schedulable is under 90 seconds

#### Scenario: Quick startup with startup taints (External)
- **Given** a standby node is started with startup taints and `startupTaintRemoval: External`
- **When** it transitions to running
- **Then** kubelet becomes Ready within 30 seconds
- **And** startup taints are removed by external controller
- **And** timing depends on external controller performance

---

## Success Criteria Additions

| ID | Criteria | Target |
|----|----------|--------|
| SC-016 | Startup taints block pod scheduling | Until network ready or external removal |
| SC-017 | WhenNetworkReady mode removes taints | Within 30 seconds of network ready condition |
| SC-018 | No CNI connection refused errors | 0 errors during scale-up with startup taints |
| SC-019 | Startup taint timeout works | Warning emitted after 2 minutes |
| SC-020 | External mode respects external controller | Stratos does not remove taints in External mode |
| SC-021 | CNI-agnostic detection works | Works with VPC CNI, Cilium, Calico |
