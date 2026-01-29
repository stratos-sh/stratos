## ADDED Requirements

### Requirement: NodeTemplate enableNetworkReadinessTaint field

The `NodeTemplate` SHALL support an `enableNetworkReadinessTaint` boolean field (default: `true`).

When enabled, Stratos SHALL automatically apply a `stratos.sh/not-ready` taint with value `"true"` and effect `NoSchedule` to nodes at launch. This taint SHALL be removed when the CNI reports network readiness.

When disabled, no built-in network readiness taint is applied.

#### Scenario: Default behavior (field omitted)
- **WHEN** a NodePool is created without specifying `enableNetworkReadinessTaint`
- **THEN** the field SHALL default to `true`
- **AND** nodes SHALL receive the `stratos.sh/not-ready:NoSchedule` taint at launch

#### Scenario: Explicitly enabled
- **WHEN** a NodePool is created with `enableNetworkReadinessTaint: true`
- **THEN** nodes SHALL receive the `stratos.sh/not-ready:NoSchedule` taint at launch

#### Scenario: Explicitly disabled
- **WHEN** a NodePool is created with `enableNetworkReadinessTaint: false`
- **THEN** no built-in network readiness taint SHALL be applied to nodes

## MODIFIED Requirements

### Requirement: NodePool preWarm configuration

The NodePool `preWarm` block SHALL support the following fields:
- `timeout`: Duration to wait for warmup completion (default: 10m)
- `timeoutAction`: Action when warmup times out - "stop" or "terminate" (default: stop)

The `completionMode` field is no longer configurable - the controller always uses ControllerStop behavior.

#### Scenario: NodePool with preWarm configuration
- **WHEN** a NodePool is created with `preWarm: {timeout: 15m, timeoutAction: terminate}`
- **THEN** the resource SHALL be accepted
- **AND** the controller SHALL use ControllerStop behavior (stop instance when node becomes Ready)

#### Scenario: NodePool with legacy completionMode field
- **WHEN** a NodePool is created with `preWarm.completionMode: SelfStop`
- **THEN** the field SHALL be ignored
- **AND** the controller SHALL use ControllerStop behavior

## REMOVED Requirements

### Requirement: startupTaintRemoval field
**Reason**: Replaced by `enableNetworkReadinessTaint` boolean. The built-in taint always uses WhenNetworkReady removal. Custom `startupTaints` are always externally managed.
**Migration**: Users with `startupTaintRemoval: WhenNetworkReady` need no changes (this is now the default via `enableNetworkReadinessTaint: true`). Users with `startupTaintRemoval: External` should set `enableNetworkReadinessTaint: false` and use `startupTaints` with their external controller.
