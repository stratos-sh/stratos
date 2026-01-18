# Capability: Automatic Scale-Up

## ADDED Requirements

### Requirement: Pod Event Watching

The system SHALL watch for Kubernetes pod events and react immediately to unschedulable pods.

#### Scenario: Detecting pending pods
- **WHEN** pods become unschedulable due to insufficient capacity
- **THEN** Stratos evaluates them immediately via the event watcher

---

### Requirement: Pod Matching

The system SHALL evaluate pending pods against NodePool requirements (node selectors, taints/tolerations).

#### Scenario: Pods match a NodePool
- **WHEN** pending pods have requirements matching a NodePool's node template
- **THEN** Stratos considers those pods for scale-up using that pool's standby nodes

#### Scenario: Pods don't match any NodePool
- **WHEN** pending pods don't match any NodePool (wrong requirements)
- **THEN** Stratos ignores them and does not attempt to scale

---

### Requirement: Start Standby Nodes

The system SHALL start standby nodes when pending pods can be satisfied.

#### Scenario: Starting a standby node
- **WHEN** pending pods can be scheduled on a standby node
- **THEN** Stratos starts the standby node

#### Scenario: Node becomes ready after start
- **WHEN** a standby node is started
- **THEN** the node becomes Ready within seconds and pods are scheduled

---

### Requirement: Efficient Scale-Up

The system SHALL NOT start more nodes than needed to satisfy pending pods.

#### Scenario: Starting only necessary nodes
- **WHEN** pending pods require 2 nodes worth of capacity
- **THEN** Stratos starts exactly 2 standby nodes (not more)

---

### Requirement: Pool Size Limit

The system SHALL NOT exceed poolSize total nodes (standby + running).

#### Scenario: At pool size limit
- **WHEN** pending pods need capacity but pool is at poolSize limit
- **THEN** pods remain pending until capacity is released

---

### Requirement: Handling Scale-Up Failures

The system SHALL handle standby node start failures gracefully.

#### Scenario: Standby node fails to start
- **WHEN** a standby node fails to start
- **THEN** Stratos retries or terminates the node and tries another standby node

---

### Requirement: No Standby Available

The system SHALL leave pods pending when no standby nodes are available.

#### Scenario: Empty standby pool
- **WHEN** pending pods need capacity but no standby nodes are available
- **THEN** pods remain pending until standby nodes become available or are replenished
