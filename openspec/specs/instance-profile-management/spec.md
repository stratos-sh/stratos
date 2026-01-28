## Requirements

### Requirement: Controller creates instance profile from IAM role

When `spec.role` is set on an AWSNodeClass, the controller SHALL create an EC2 instance profile named `stratos-<cluster-name>-<awsnodeclass-name>` and attach the specified IAM role. The instance profile ARN SHALL be stored in `status.resolvedInstanceProfile`. The cluster name is sourced from the controller's `--cluster-name` flag.

#### Scenario: Instance profile created from role
- **WHEN** an AWSNodeClass is created with `spec.role: "my-node-role"`
- **AND** the AWSNodeClass is named "gpu-nodes"
- **AND** the controller's cluster name is "main"
- **THEN** the controller SHALL create instance profile "stratos-main-gpu-nodes"
- **AND** attach role "my-node-role" to the profile
- **AND** set `status.resolvedInstanceProfile` to the profile ARN

#### Scenario: Instance profile already exists
- **WHEN** an AWSNodeClass with `spec.role` is reconciled
- **AND** the instance profile "stratos-<cluster>-<name>" already exists with the correct role attached
- **THEN** the controller SHALL not create a duplicate
- **AND** SHALL set `status.resolvedInstanceProfile` to the existing profile ARN

#### Scenario: Role does not exist
- **WHEN** an AWSNodeClass has `spec.role: "nonexistent-role"`
- **AND** the IAM role does not exist
- **THEN** the controller SHALL set `InstanceProfileReady=False` with reason "RoleNotFound"

### Requirement: Controller handles role changes on existing AWSNodeClass

When `spec.role` is changed on an existing AWSNodeClass, the controller SHALL remove the old role from the instance profile and attach the new role. Instance profiles support only one role.

#### Scenario: Role updated from role-a to role-b
- **WHEN** an AWSNodeClass with `spec.role: "role-a"` is updated to `spec.role: "role-b"`
- **THEN** the controller SHALL remove "role-a" from the instance profile
- **AND** attach "role-b" to the instance profile
- **AND** update `status.resolvedInstanceProfile` with the profile ARN

#### Scenario: Role removal fails during update
- **WHEN** the controller attempts to remove the old role from the instance profile
- **AND** the removal fails (e.g., IAM API error)
- **THEN** the controller SHALL set `InstanceProfileReady=False` with reason "RoleUpdateFailed"
- **AND** retry on next reconcile

### Requirement: Instance profile is cleaned up on AWSNodeClass deletion

When an AWSNodeClass with `spec.role` is deleted, the controller SHALL remove the IAM role from the instance profile and delete the instance profile. This SHALL be enforced via a finalizer. The `stratos.sh/in-use` finalizer (blocking deletion while NodePools reference) SHALL be processed before the instance profile cleanup finalizer.

#### Scenario: Cleanup on deletion
- **WHEN** an AWSNodeClass with `spec.role: "my-role"` named "gpu-nodes" in cluster "main" is deleted
- **THEN** the controller SHALL remove role "my-role" from instance profile "stratos-main-gpu-nodes"
- **AND** delete the instance profile "stratos-main-gpu-nodes"
- **AND** remove the finalizer to allow the resource to be garbage collected

#### Scenario: Cleanup when profile already deleted externally
- **WHEN** an AWSNodeClass is deleted
- **AND** the instance profile has already been deleted outside the controller
- **THEN** the controller SHALL handle the "not found" error gracefully
- **AND** remove the finalizer to allow deletion to proceed

#### Scenario: Deletion blocked while NodePools reference AWSNodeClass
- **WHEN** an AWSNodeClass is deleted
- **AND** a NodePool still references it
- **THEN** the `stratos.sh/in-use` finalizer SHALL block deletion
- **AND** the instance profile SHALL NOT be deleted until all NodePool references are removed

### Requirement: Static iamInstanceProfile bypasses profile management

When `spec.iamInstanceProfile` is set (instead of `spec.role`), the controller SHALL use the provided value directly without creating or managing any instance profile. The value SHALL be stored in `status.resolvedInstanceProfile`.

#### Scenario: Static instance profile used directly
- **WHEN** an AWSNodeClass has `spec.iamInstanceProfile: "arn:aws:iam::123456789012:instance-profile/my-profile"`
- **THEN** `status.resolvedInstanceProfile` SHALL be set to that ARN
- **AND** the controller SHALL NOT create or manage any instance profile
- **AND** `InstanceProfileReady` SHALL be set to True

### Requirement: Instance profile management is integration tested with LocalStack

The instance profile create/attach/update/delete lifecycle SHALL be validated with LocalStack integration tests.

#### Scenario: LocalStack test validates full lifecycle
- **WHEN** the integration test creates an IAM role in LocalStack
- **AND** the resolver creates an instance profile from that role
- **THEN** the resolver SHALL return the instance profile ARN
- **AND** deletion SHALL successfully remove the role attachment and profile

#### Scenario: LocalStack test validates role update
- **WHEN** the integration test creates a profile with role-a
- **AND** then updates to role-b
- **THEN** the profile SHALL have only role-b attached
