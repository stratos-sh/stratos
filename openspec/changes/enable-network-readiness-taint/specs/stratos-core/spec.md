## MODIFIED Requirements

### Requirement: FR-054 Controller Stop on Node Ready

In ControllerStop mode, Stratos MUST call `StopInstance` when:
1. The Kubernetes node exists (instance joined cluster)
2. The node has Ready condition = True
3. If `enableNetworkReadinessTaint` is true, network readiness conditions are also met

**Priority**: P1
**Status**: Implemented

#### Scenario: Stop on node Ready
- **WHEN** an instance is in warmup state
- **AND** the node becomes Ready
- **THEN** Stratos calls StopInstance and transitions to standby

#### Scenario: Wait for network ready when enableNetworkReadinessTaint is true
- **WHEN** a NodePool has `enableNetworkReadinessTaint: true` (or default)
- **AND** a node is Ready but NetworkingReady is False
- **THEN** Stratos SHALL wait until NetworkingReady becomes True before stopping

#### Scenario: No network wait when enableNetworkReadinessTaint is false
- **WHEN** a NodePool has `enableNetworkReadinessTaint: false`
- **AND** no custom `startupTaints` are configured
- **AND** a node becomes Ready
- **THEN** Stratos SHALL stop the instance immediately without waiting for network readiness

#### Scenario: Node not yet Ready
- **WHEN** an instance is in warmup state
- **AND** the node exists but Ready condition is not True
- **THEN** Stratos continues monitoring (does not stop)

## ADDED Requirements

### Requirement: Built-in network readiness taint lifecycle

When `enableNetworkReadinessTaint` is true, Stratos SHALL manage a `stratos.sh/not-ready:NoSchedule` taint through the full node lifecycle:

1. **Launch**: Include the taint in `--register-with-taints` alongside any custom `startupTaints`
2. **Standby transition**: Re-apply the taint to the node (kubelet does not re-apply register-with-taints on restart)
3. **Running**: Remove the taint when `NetworkReadinessChecker` reports the CNI is ready
4. **Timeout**: Force-remove the taint after `StartupTaintRemovalTimeout` (2 minutes) if CNI has not reported ready

#### Scenario: Built-in taint applied at launch
- **WHEN** a node is launched for a pool with `enableNetworkReadinessTaint: true`
- **THEN** the launch config SHALL include `stratos.sh/not-ready=true:NoSchedule` in register-with-taints

#### Scenario: Built-in taint re-applied on standby
- **WHEN** a running node transitions to standby
- **AND** `enableNetworkReadinessTaint` is true
- **THEN** the `stratos.sh/not-ready:NoSchedule` taint SHALL be re-applied to the node

#### Scenario: Built-in taint removed on network ready
- **WHEN** a node transitions from standby to running
- **AND** `NetworkReadinessChecker.IsReady()` returns true
- **THEN** the `stratos.sh/not-ready:NoSchedule` taint SHALL be removed

#### Scenario: Built-in taint timeout
- **WHEN** a running node has had the built-in taint for longer than `StartupTaintRemovalTimeout`
- **AND** the CNI has not reported ready
- **THEN** the taint SHALL be force-removed with a warning event

### Requirement: Custom startupTaints are externally managed

Custom `startupTaints` configured in `NodeTemplate` SHALL be applied by Stratos at launch and re-applied on standby transitions, but Stratos SHALL NOT remove them. Removal is the responsibility of an external controller or the user.

#### Scenario: Custom taints applied at launch
- **WHEN** a node is launched with custom `startupTaints` configured
- **THEN** those taints SHALL be included in `--register-with-taints`

#### Scenario: Custom taints re-applied on standby
- **WHEN** a running node with custom `startupTaints` transitions to standby
- **THEN** the custom taints SHALL be re-applied to the node

#### Scenario: Custom taints not removed by Stratos
- **WHEN** a node is running with custom `startupTaints`
- **AND** the CNI reports network ready
- **THEN** Stratos SHALL NOT remove the custom taints
- **AND** the custom taints SHALL remain until removed by an external controller
