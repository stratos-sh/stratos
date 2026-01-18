# Capability: Pool Reconciliation

## ADDED Requirements

### Requirement: Periodic Reconciliation Loop

The system SHALL run a periodic reconciliation loop to maintain pool health.

#### Scenario: Reconciliation interval
- **WHEN** Stratos is running
- **THEN** it executes pool reconciliation at the configured interval (default 30 seconds)

---

### Requirement: Standby Replenishment

The system SHALL provision new nodes when standby count is below minStandby.

#### Scenario: Standby below minimum
- **WHEN** poolSize=10, minStandby=5, and current standby count is 3
- **THEN** Stratos provisions 2 new nodes to reach minStandby

#### Scenario: Standby at minimum
- **WHEN** poolSize=10, minStandby=5, and current standby count is 5
- **THEN** no new nodes are provisioned

#### Scenario: At poolSize limit
- **WHEN** poolSize=10, minStandby=5, with 7 running and 3 standby (10 total at limit)
- **THEN** no new nodes are provisioned even though standby < minStandby

---

### Requirement: External Termination Detection

The system SHALL detect and handle externally terminated instances.

#### Scenario: Standby instance externally terminated
- **WHEN** a standby node's underlying instance was externally terminated
- **THEN** Stratos removes the stale Node object and provisions a replacement

#### Scenario: Running instance externally terminated
- **WHEN** a running node's underlying instance was externally terminated
- **THEN** Stratos removes the stale Node object and replenishes standby if needed

---

### Requirement: Controller Restart Recovery

The system SHALL recover state on controller restart.

#### Scenario: Controller restarts
- **WHEN** the Stratos controller restarts
- **THEN** it reconciles all NodePools and recovers any inconsistent state from the cluster

---

### Requirement: Stuck Instance Handling

The system SHALL handle instances stuck in transitional states.

#### Scenario: Instance stuck stopping
- **WHEN** an instance is stuck in "stopping" state beyond a secondary timeout
- **THEN** Stratos terminates the instance and provisions a replacement
