# Capability: Automatic Scale-Down

## ADDED Requirements

### Requirement: Empty Node Detection

The system SHALL detect empty nodes (no pods excluding DaemonSets).

#### Scenario: Identifying empty nodes
- **WHEN** a running node has no pods (excluding DaemonSets)
- **THEN** Stratos identifies it as a candidate for scale-down

---

### Requirement: Empty Node TTL

The system SHALL support configurable emptyNodeTTL (duration before stopping empty nodes).

#### Scenario: Node empty for TTL duration
- **WHEN** a node has been empty for the configured emptyNodeTTL duration
- **THEN** Stratos initiates scale-down for that node

#### Scenario: Node becomes non-empty before TTL
- **WHEN** pods are scheduled on an empty node before TTL expires
- **THEN** the scale-down timer is reset and the node continues running

---

### Requirement: Node Cordoning

The system SHALL cordon nodes before draining.

#### Scenario: Cordoning before drain
- **WHEN** Stratos decides to scale down a node
- **THEN** it cordons the node first to prevent new pods from scheduling

---

### Requirement: PDB-Respecting Drain

The system SHALL drain nodes respecting PodDisruptionBudgets.

#### Scenario: Draining with PDBs
- **WHEN** a node being drained has pods with PodDisruptionBudgets
- **THEN** Stratos respects the PDBs and waits for allowed disruption

#### Scenario: DaemonSet pods with PDBs
- **WHEN** an empty node has DaemonSet pods with PDBs
- **THEN** Stratos respects PDBs and waits or skips the node

---

### Requirement: Stop Instead of Terminate

The system SHALL stop (not terminate) nodes on scale-down to preserve pre-warming.

#### Scenario: Node is stopped after drain
- **WHEN** a node is successfully drained during scale-down
- **THEN** Stratos stops (not terminates) the underlying instance

---

### Requirement: Return to Standby

Stopped nodes SHALL return to standby pool and be available for future scale-up.

#### Scenario: Node returns to standby
- **WHEN** a node is stopped during scale-down
- **THEN** it returns to the standby pool and is available for future scale-up

---

### Requirement: Scale-Down Toggle

The system SHALL support disabling automatic scale-down per NodePool.

#### Scenario: Scale-down disabled
- **WHEN** scale-down is disabled for a NodePool
- **THEN** empty nodes remain running until manually intervened

---

### Requirement: Drain Timeout Handling

The system SHALL handle drain timeouts gracefully.

#### Scenario: Drain takes too long
- **WHEN** drain exceeds the configured timeout
- **THEN** Stratos may force-drain or skip the node based on configuration
