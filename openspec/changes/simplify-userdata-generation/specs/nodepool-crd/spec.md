## REMOVED Requirements

### Requirement: Configurable Warmup Completion Mode (FR-053)

**Reason:** Simplified to always use ControllerStop behavior. This eliminates user confusion between modes and prevents misconfiguration (e.g., SelfStop mode with Bottlerocket which doesn't support shutdown scripts).

The controller SHALL always use ControllerStop behavior:
- Monitor node until it becomes Ready
- Stop the instance when Ready (and NetworkingReady if configured)
- Warmup scripts no longer call poweroff

**Migration:** Remove `preWarm.completionMode` from NodePool specs. The controller will automatically use ControllerStop behavior. Warmup scripts should be updated to NOT call poweroff.

### Requirement: SelfStop warmup completion

**Reason:** Removed in favor of ControllerStop-only behavior.

The instance self-stop via userdata script is no longer supported. All warmup completion is handled by the controller.

**Migration:** Remove poweroff/shutdown commands from userdata scripts. The controller handles instance stopping.

## MODIFIED Requirements

### Requirement: NodePool preWarm configuration

The NodePool `preWarm` block SHALL support the following fields:
- `timeout`: Duration to wait for warmup completion (default: 10m)
- `timeoutAction`: Action when warmup times out - "stop" or "terminate" (default: stop)
- `startupTaintRemoval`: When to remove startup taint - "WhenReady" or "WhenNetworkReady" (default: WhenReady)

**REMOVED field:**
- `completionMode`: No longer configurable, always uses ControllerStop behavior

#### Scenario: NodePool with preWarm configuration
- **WHEN** a NodePool is created with `preWarm: {timeout: 15m, timeoutAction: terminate}`
- **THEN** the resource SHALL be accepted
- **AND** the controller SHALL use ControllerStop behavior (stop instance when node becomes Ready)

#### Scenario: NodePool with legacy completionMode field
- **WHEN** a NodePool is created with `preWarm.completionMode: SelfStop`
- **THEN** the field SHALL be ignored
- **AND** the controller SHALL use ControllerStop behavior

### Requirement: Warmup completion via controller

The controller SHALL stop instances when warmup completes (ControllerStop behavior).

#### Scenario: Stop on node Ready
- **WHEN** an instance is in warmup state
- **AND** the corresponding node becomes Ready
- **THEN** the controller SHALL stop the instance
- **AND** transition the node to standby state

#### Scenario: Wait for network ready when configured
- **WHEN** a NodePool has `startupTaintRemoval: WhenNetworkReady`
- **AND** a node is Ready but NetworkingReady is False
- **THEN** the controller SHALL wait until NetworkingReady becomes True before stopping

#### Scenario: Warmup timeout
- **WHEN** an instance has been in warmup state longer than the configured timeout
- **AND** the node is not yet Ready
- **THEN** the controller SHALL apply the timeout action (stop or terminate)
