# Capability: Node Pre-warming

## ADDED Requirements

### Requirement: Instance Launch

The system SHALL launch instances through the configured cloud provider.

#### Scenario: Launching a new instance
- **WHEN** Stratos needs to provision a new node for a NodePool
- **THEN** it launches an instance with the configured instance type, AMI, and network settings

---

### Requirement: Userdata Initialization

The system SHALL configure instances with userdata that joins the Kubernetes cluster and self-stops.

#### Scenario: Instance initialization via userdata
- **WHEN** Stratos launches a new instance
- **THEN** the instance runs userdata that joins the cluster and self-stops when ready

#### Scenario: Node appears in cluster
- **WHEN** a launched instance's Node object appears in the cluster
- **THEN** Stratos monitors it for completion of the warmup process

---

### Requirement: Warmup Monitoring

The system SHALL monitor launched instances waiting for them to self-stop.

#### Scenario: Instance self-stops successfully
- **WHEN** a launched instance joins the cluster and self-stops within the timeout
- **THEN** the node is marked as standby and ready for use

#### Scenario: Instance fails to join cluster
- **WHEN** a launched instance does NOT create a Node object within the timeout
- **THEN** Stratos terminates the instance and provisions a replacement

---

### Requirement: Warmup Timeout Configuration

The system SHALL support a configurable timeout for instances to self-stop.

#### Scenario: Configuring warmup timeout
- **WHEN** a NodePool specifies warmup.timeout of 10 minutes
- **THEN** Stratos waits up to 10 minutes for the instance to self-stop

---

### Requirement: Warmup Timeout Action

The system SHALL support a configurable timeout action: "stop" or "terminate".

#### Scenario: Timeout action is stop
- **WHEN** an instance does NOT self-stop within the timeout AND timeout action is "stop"
- **THEN** Stratos manually stops the instance and marks it as standby

#### Scenario: Timeout action is terminate
- **WHEN** an instance does NOT self-stop within the timeout AND timeout action is "terminate"
- **THEN** Stratos terminates the instance and provisions a replacement

---

### Requirement: Node Labeling

The system SHALL label pre-warmed Nodes with NodePool ownership and state.

#### Scenario: Standby node labels
- **WHEN** a pre-warmed standby node is viewed in kubectl
- **THEN** it shows labels indicating Stratos management, NodePool name, and standby state

#### Scenario: Running node labels
- **WHEN** a node is started from standby
- **THEN** its state label is updated to "running"
