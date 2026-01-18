# Capability: NodePool Management

## ADDED Requirements

### Requirement: NodePool CRD

The system SHALL provide a NodePool Custom Resource Definition for configuring pre-warmed node pools.

#### Scenario: NodePool creation with valid configuration
- **WHEN** a NodePool with poolSize=10 and minStandby=5 is created
- **THEN** Stratos accepts the resource and begins provisioning standby nodes

#### Scenario: NodePool creation with invalid minStandby
- **WHEN** a NodePool where minStandby exceeds poolSize is applied
- **THEN** the resource is rejected with a validation error

#### Scenario: NodePool creation with missing required fields
- **WHEN** a NodePool with invalid or missing required parameters is applied
- **THEN** it is rejected with a clear validation error

---

### Requirement: NodePool Configuration

The system SHALL support configuring poolSize (maximum total nodes) and minStandby (minimum standby count) for each NodePool.

#### Scenario: Configuring pool limits
- **WHEN** a NodePool with poolSize=10 and minStandby=5 is created
- **THEN** Stratos launches 5 instances that join the cluster, self-stop, and become standby nodes

#### Scenario: Updating minStandby
- **WHEN** a running NodePool with 5 standby nodes has minStandby increased to 8
- **THEN** Stratos provisions 3 additional pre-warmed nodes

---

### Requirement: Multiple NodePool Support

The system SHALL support multiple NodePool resources in a single cluster.

#### Scenario: Creating multiple NodePools
- **WHEN** two NodePools with different configurations are created
- **THEN** Stratos manages each pool independently

---

### Requirement: NodePool Deletion

The system SHALL terminate all associated nodes when a NodePool is deleted.

#### Scenario: Deleting a NodePool with running nodes
- **WHEN** a NodePool is deleted while nodes are running
- **THEN** Stratos drains and terminates all nodes associated with the NodePool

#### Scenario: Deleting a NodePool with standby nodes
- **WHEN** a NodePool is deleted while nodes are in standby
- **THEN** Stratos terminates all standby instances

---

### Requirement: NodePool Update Reconciliation

The system SHALL reconcile to the new desired state when NodePool spec is updated.

#### Scenario: Reducing poolSize
- **WHEN** poolSize is reduced below current total nodes
- **THEN** Stratos consolidates by stopping/terminating excess nodes

#### Scenario: Changing cloud configuration
- **WHEN** cloud-specific parameters (AMI, instance type) are changed
- **THEN** existing nodes are not affected; new nodes use updated configuration
