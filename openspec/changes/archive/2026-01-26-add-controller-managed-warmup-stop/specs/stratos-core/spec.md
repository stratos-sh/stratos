# Spec Delta: Stratos Core

**Change**: add-controller-managed-warmup-stop
**Base**: openspec/specs/stratos-core/spec.md

## ADDED Requirements

### Requirement: FR-053 Configurable Warmup Completion Mode

NodePool MUST support configurable `preWarm.completionMode` to control how warmup completes:
- `SelfStop` (default): Instance self-stops via userdata script (current behavior)
- `ControllerStop`: Stratos stops instance when node is Ready

**Priority**: P1
**Status**: Implemented

```yaml
spec:
  preWarm:
    completionMode: ControllerStop  # or SelfStop (default)
```

#### Scenario: Default to SelfStop for backward compatibility
- **Given** a NodePool without `preWarm.completionMode` specified
- **When** an instance completes warmup
- **Then** Stratos waits for the instance to self-stop (existing behavior)

#### Scenario: ControllerStop mode configured
- **Given** a NodePool with `preWarm.completionMode: ControllerStop`
- **When** an instance is launched for warmup
- **Then** Stratos monitors the node and stops it when ready

---

### Requirement: FR-054 Controller Stop on Node Ready

In ControllerStop mode, Stratos MUST call `StopInstance` when:
1. The Kubernetes node exists (instance joined cluster)
2. The node has Ready condition = True
3. Network readiness conditions are met (if `startupTaintRemoval: WhenNetworkReady`)

**Priority**: P1
**Status**: Implemented

#### Scenario: Stop on node Ready
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** an instance in warmup state
- **When** the node becomes Ready
- **Then** Stratos calls StopInstance and transitions to standby

#### Scenario: Wait for network ready when configured
- **Given** a NodePool with `completionMode: ControllerStop` and `startupTaintRemoval: WhenNetworkReady`
- **And** a node that is Ready but NetworkingReady is False
- **When** Stratos evaluates the node
- **Then** it waits until NetworkingReady becomes True before stopping

#### Scenario: Node not yet Ready
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** an instance in warmup state
- **When** the node exists but Ready condition is not True
- **Then** Stratos continues monitoring (does not stop)

---

### Requirement: FR-055 Timeout Handling in ControllerStop Mode

In ControllerStop mode, if the node does not become Ready within `preWarm.timeout`, Stratos MUST apply the configured `timeoutAction` (stop or terminate).

**Priority**: P1
**Status**: Implemented

#### Scenario: Timeout in ControllerStop mode
- **Given** a NodePool with `completionMode: ControllerStop` and `timeout: 10m`
- **And** an instance that has been warming up for more than 10 minutes
- **When** the node is still not Ready
- **Then** Stratos applies the timeout action (stop or terminate)

---

### Requirement: FR-056 ControllerStop Without Userdata Shutdown Scripts

In ControllerStop mode, instances MUST be able to complete warmup without any shutdown commands in userdata. The userdata only needs to configure the node to join the cluster.

**Priority**: P1
**Status**: Implemented

#### Scenario: Bottlerocket with config-only userdata
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** Bottlerocket AMI with TOML-only userdata (no bootstrap containers)
- **When** the instance boots and joins the cluster
- **Then** Stratos detects the Ready node and stops it

#### Scenario: AL2023 without shutdown script
- **Given** a NodePool with `completionMode: ControllerStop`
- **And** AL2023 AMI with nodeadm config but no poweroff script
- **When** the instance boots and joins the cluster
- **Then** Stratos detects the Ready node and stops it

---

## MODIFIED Requirements

### Requirement: FR-007 Userdata Configuration for Cluster Join

Stratos MUST configure instances with userdata that joins the K8s cluster. In `SelfStop` mode (default), userdata MUST also include logic to stop the instance. In `ControllerStop` mode, no shutdown logic is required.

**Priority**: P1
**Status**: Implemented

#### Scenario: Node initialization (SelfStop mode)
- **Given** a launched instance with `completionMode: SelfStop`
- **When** userdata executes
- **Then** the node joins the cluster and the instance self-stops

#### Scenario: Node initialization (ControllerStop mode)
- **Given** a launched instance with `completionMode: ControllerStop`
- **When** userdata executes
- **Then** the node joins the cluster (no self-stop required)

---

### Requirement: FR-008 Warmup Instance Monitoring

Stratos MUST monitor launched instances in warmup state:
- In `SelfStop` mode: Wait for instance to self-stop (detect stopped state)
- In `ControllerStop` mode: Wait for node Ready, then call StopInstance

**Priority**: P1
**Status**: Implemented

#### Scenario: Detect self-stop (SelfStop mode)
- **Given** a launched instance in warmup state with `completionMode: SelfStop`
- **When** the instance stops
- **Then** Stratos marks the node as standby

#### Scenario: Stop on ready (ControllerStop mode)
- **Given** a launched instance in warmup state with `completionMode: ControllerStop`
- **When** the node becomes Ready
- **Then** Stratos stops the instance and marks it as standby
