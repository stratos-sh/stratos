## ADDED Requirements

### Requirement: NodeClass has finalizer when referenced

When a NodeClass is referenced by one or more NodePools, the controller SHALL add the `stratos.sh/in-use` finalizer to prevent accidental deletion.

#### Scenario: Finalizer added when NodePool references NodeClass
- **WHEN** a NodePool is created that references an AWSNodeClass
- **THEN** the controller SHALL add `stratos.sh/in-use` finalizer to the AWSNodeClass

#### Scenario: Finalizer prevents deletion
- **WHEN** a user attempts to delete an AWSNodeClass that has the `stratos.sh/in-use` finalizer
- **THEN** the deletion SHALL be blocked (resource enters Terminating state)
- **AND** the resource SHALL remain until all referencing NodePools are deleted

#### Scenario: Finalizer removed when no longer referenced
- **WHEN** the last NodePool referencing an AWSNodeClass is deleted
- **THEN** the controller SHALL remove the `stratos.sh/in-use` finalizer from the AWSNodeClass
- **AND** if the AWSNodeClass was pending deletion, it SHALL now be deleted

### Requirement: NodeClass has status subresource

AWSNodeClass SHALL have a status subresource tracking usage and validation state.

The status SHALL include:
- `nodePoolCount`: Number of NodePools currently referencing this NodeClass
- `conditions`: Standard Kubernetes conditions array

#### Scenario: Status reflects NodePool count
- **WHEN** two NodePools reference an AWSNodeClass
- **THEN** the AWSNodeClass status SHALL show `nodePoolCount: 2`

#### Scenario: Status updated when NodePool deleted
- **WHEN** one of two NodePools referencing an AWSNodeClass is deleted
- **THEN** the AWSNodeClass status SHALL update to `nodePoolCount: 1`

### Requirement: NodeClass has Valid condition

The controller SHALL maintain a `Valid` condition on NodeClass resources indicating whether the spec passes validation.

#### Scenario: Valid condition true for valid spec
- **WHEN** an AWSNodeClass has all required fields with valid values
- **THEN** the `Valid` condition SHALL be `True` with reason "SpecValid"

#### Scenario: Valid condition false for invalid spec
- **WHEN** an AWSNodeClass has an invalid AMI format (not matching `ami-*`)
- **THEN** the `Valid` condition SHALL be `False` with reason "InvalidAMI"

### Requirement: NodeClass has InUse condition

The controller SHALL maintain an `InUse` condition on NodeClass resources indicating whether any NodePools reference it.

#### Scenario: InUse condition true when referenced
- **WHEN** at least one NodePool references an AWSNodeClass
- **THEN** the `InUse` condition SHALL be `True` with reason "ReferencedByNodePools"

#### Scenario: InUse condition false when not referenced
- **WHEN** no NodePools reference an AWSNodeClass
- **THEN** the `InUse` condition SHALL be `False` with reason "NotReferenced"

### Requirement: Controller watches NodeClass changes

The NodePool controller SHALL watch for changes to NodeClass resources and reconcile affected NodePools.

#### Scenario: NodeClass update triggers NodePool reconcile
- **WHEN** an AWSNodeClass is updated
- **THEN** all NodePools referencing that AWSNodeClass SHALL be queued for reconciliation

#### Scenario: NodeClass deletion triggers NodePool reconcile
- **WHEN** an AWSNodeClass is deleted (after finalizer removed)
- **THEN** the controller SHALL handle any NodePools that may have stale references
