# Capability: Observability

## ADDED Requirements

### Requirement: Pool Metrics

The system SHALL expose Prometheus metrics for pool state.

#### Scenario: Pool size metrics
- **WHEN** querying Stratos metrics
- **THEN** metrics for pool size, standby count, running count, and warmup count are available per NodePool

---

### Requirement: Scale-Up Metrics

The system SHALL expose metrics for scale-up operations.

#### Scenario: Scale-up operation metrics
- **WHEN** scale-up operations occur
- **THEN** metrics for count and latency (time from pending to scheduled) are available

---

### Requirement: Scale-Down Metrics

The system SHALL expose metrics for scale-down operations.

#### Scenario: Scale-down operation metrics
- **WHEN** scale-down operations occur
- **THEN** metrics for count and drain duration are available

---

### Requirement: Kubernetes Events

The system SHALL emit Kubernetes events for significant operations.

#### Scenario: Node started event
- **WHEN** a standby node is started for scale-up
- **THEN** a Kubernetes event is emitted on the NodePool

#### Scenario: Node stopped event
- **WHEN** a node is stopped during scale-down
- **THEN** a Kubernetes event is emitted on the NodePool

#### Scenario: Error events
- **WHEN** an error occurs during any operation
- **THEN** a Kubernetes event with error details is emitted

---

### Requirement: Warmup Failure Metrics

The system SHALL expose metrics for warmup failures.

#### Scenario: Warmup timeout metrics
- **WHEN** instances fail to self-stop within timeout
- **THEN** warmup failure metrics are incremented
