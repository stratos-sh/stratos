# Capability: Node Runtime Limits

## ADDED Requirements

### Requirement: Maximum Node Runtime Configuration

NodePool SHALL support configurable maxNodeRuntime (optional, 0 = disabled).

#### Scenario: Configuring maxNodeRuntime
- **WHEN** a NodePool specifies maxNodeRuntime of 24 hours
- **THEN** Stratos monitors running nodes for runtime duration

#### Scenario: Disabling maxNodeRuntime
- **WHEN** a NodePool does not specify maxNodeRuntime or sets it to 0
- **THEN** no automatic runtime-based recycling occurs

---

### Requirement: Automatic Node Recycling

The system SHALL automatically drain and stop nodes that exceed maxNodeRuntime.

#### Scenario: Node exceeds maxNodeRuntime
- **WHEN** a node has been running longer than the configured maxNodeRuntime
- **THEN** Stratos cordons, drains, and stops the node (returns to standby)

---

### Requirement: Runtime Warning Events

The system SHALL emit warning events when nodes approach maxNodeRuntime threshold.

#### Scenario: Approaching runtime limit
- **WHEN** a node approaches its maxNodeRuntime limit (e.g., 90% of limit)
- **THEN** Stratos emits a warning event on the NodePool

---

### Requirement: RBAC Least Privilege

The system SHALL operate with cluster-scoped least-privilege RBAC permissions.

#### Scenario: Node access
- **WHEN** Stratos operates
- **THEN** it has full access to Node objects (get, list, watch, create, update, patch, delete)

#### Scenario: Pod access
- **WHEN** Stratos operates
- **THEN** it has watch access to Pod objects cluster-wide (get, list, watch)

#### Scenario: NodePool CRD access
- **WHEN** Stratos operates
- **THEN** it has full access to NodePool CRD objects

#### Scenario: Event access
- **WHEN** Stratos operates
- **THEN** it has create access to Event objects for operational visibility

#### Scenario: Limited access
- **WHEN** Stratos operates
- **THEN** it does NOT require cluster-admin or access to secrets outside its own namespace
