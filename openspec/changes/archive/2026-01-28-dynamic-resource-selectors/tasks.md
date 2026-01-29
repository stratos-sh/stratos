## 1. CRD Type Changes

- [x] 1.1 Add selector structs to `api/v1alpha1/aws_nodeclass_types.go`: `AMISelector`, `SubnetSelector`, `SecurityGroupSelector`, `MetadataOptions`
- [x] 1.2 Add selector fields to `AWSNodeClassSpec`: `amiSelector`, `subnetSelector`, `securityGroupSelector`, `role`, `metadataOptions`
- [x] 1.3 Add resolved status types: `ResolvedSubnet` (ID + zone), `ResolvedSecurityGroup` (ID + name)
- [x] 1.4 Add resolved fields to `AWSNodeClassStatus`: `resolvedAMI`, `resolvedSubnets`, `resolvedSecurityGroups`, `resolvedInstanceProfile`
- [x] 1.5 Add new condition type constants: `AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`
- [x] 1.6 Change `ami`, `subnetIds`, `securityGroupIds`, `iamInstanceProfile` from `+kubebuilder:validation:Required` to optional with CEL validation rules enforcing mutual exclusivity (one of static or selector required per resource type)
- [x] 1.7 Run `make generate && make manifests` and verify generated CRD includes CEL rules

## 2. Resolver Interface and Implementation

- [x] 2.1 Create `internal/cloudprovider/aws/resolver.go` with `Resolver` interface: `ResolveAMI`, `ResolveSubnets`, `ResolveSecurityGroups`, `ResolveInstanceProfile`, `DeleteInstanceProfile`
- [x] 2.2 Implement `AWSResolver` struct with `ec2.Client` and `iam.Client` fields
- [x] 2.3 Implement `ResolveSubnets`: call `DescribeSubnets` with tag filters (AND semantics), return `[]ResolvedSubnet`
- [x] 2.4 Implement `ResolveSecurityGroups`: call `DescribeSecurityGroups` with tag filters and name filter (wildcard support), return `[]ResolvedSecurityGroup`
- [x] 2.5 Implement `ResolveAMI`: call `DescribeImages` with tag filters, name filter (wildcard), owner filter; sort by `CreationDate` descending, return newest AMI ID
- [x] 2.6 Implement `ResolveInstanceProfile`: call `GetInstanceProfile` or `CreateInstanceProfile` + `AddRoleToInstanceProfile`; profile named `stratos-<cluster>-<name>`, return ARN
- [x] 2.7 Implement `DeleteInstanceProfile`: call `RemoveRoleFromInstanceProfile` + `DeleteInstanceProfile`, handle not-found gracefully
- [x] 2.8 Implement role update handling: detect role change, remove old role, add new role
- [x] 2.9 Add `DescribeImages`, `DescribeSubnets`, `DescribeSecurityGroups`, and IAM operations as named entries in `internal/cloudprovider/aws/ratelimit.go`

## 3. AWSNodeClass Reconciler

- [x] 3.1 Create `internal/cloudprovider/aws/nodeclass_controller.go` with `AWSNodeClassReconciler` struct (holds `Resolver`, k8s client, cluster name)
- [x] 3.2 Implement `Reconcile` method: for each resource type, check if selector or static field is set, call resolver or copy static IDs to status
- [x] 3.3 Implement last-known-good semantics: on transient resolution failure, preserve previous status values, emit warning event; only set condition False if no cached values exist
- [x] 3.4 Set readiness conditions (`AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`) based on resolution results
- [x] 3.5 Add instance profile cleanup finalizer (`stratos.sh/instance-profile`), process after `stratos.sh/in-use` finalizer on deletion
- [x] 3.6 Register the AWSNodeClass reconciler with the manager in `cmd/stratos/main.go` (only when cloud provider is `aws`)

## 4. Update Launch Path

- [x] 4.1 Update `AWSProvider.LaunchInstance` in `internal/cloudprovider/aws/provider.go` to read AMI, subnets, security groups, and instance profile from `nodeClass.Status` resolved fields instead of `nodeClass.Spec`
- [x] 4.2 Add `metadataOptions` passthrough: read from `nodeClass.Spec.MetadataOptions`, map to `ec2types.InstanceMetadataOptionsRequest` in the `RunInstances` call
- [x] 4.3 Return error from `LaunchInstance` if resolved status fields are empty (AWSNodeClass not yet reconciled)

## 5. Update NodePool Controller

- [x] 5.1 Update `nodepool_controller.go` to check AWSNodeClass readiness conditions (`AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`) before launching. Set NodePool `Ready=False` with reason `NodeClassNotReady` if any condition is False
- [x] 5.2 Remove any direct resolution logic from the NodePool controller (it should only read from AWSNodeClass status)

## 6. Update Fake Provider

- [x] 6.1 Update `internal/cloudprovider/fake/provider.go` `LaunchInstance` to read from `nodeClass.Status` resolved fields instead of `nodeClass.Spec` fields
- [x] 6.2 Create `internal/cloudprovider/fake/resolver.go` with a `FakeResolver` implementation that returns configurable canned responses and supports hooks for error injection

## 7. LocalStack Integration Tests

- [x] 7.1 Add `testcontainers-go` and `testcontainers-go/modules/localstack` to `go.mod`
- [x] 7.2 Create `internal/cloudprovider/aws/resolver_integration_test.go` with `//go:build integration` tag
- [x] 7.3 Implement `TestMain` to start LocalStack container (EC2 + IAM services), construct `AWSResolver` pointing at LocalStack endpoint
- [x] 7.4 Implement seed helpers: `createSubnetWithTags`, `createSecurityGroupWithTags`, `registerImageWithTags`, `createRole`
- [x] 7.5 Test `ResolveSubnets`: tags match correct subnets, no match returns error, AND semantics across multiple tags
- [x] 7.6 Test `ResolveSecurityGroups`: tag match, name wildcard match, combined tag+name, no match returns error
- [x] 7.7 Test `ResolveAMI`: tag match, name wildcard, owner filter, newest-wins across multiple matches, no match returns error
- [x] 7.8 Test `ResolveInstanceProfile`: create from role, idempotent re-create, role update (remove old + add new), delete lifecycle
- [x] 7.9 Add `make test-localstack` target to Makefile

## 8. Controller Integration Tests (envtest)

- [x] 8.1 Add AWSNodeClass reconciler to envtest suite setup in `tests/integration/suite_test.go`, inject `FakeResolver`
- [x] 8.2 Test: AWSNodeClass with static IDs populates resolved status fields and sets all conditions True
- [x] 8.3 Test: AWSNodeClass with selectors resolves via FakeResolver, populates status, sets conditions True
- [x] 8.4 Test: AWSNodeClass with selector that resolves to nothing sets condition False, blocks NodePool launch
- [x] 8.5 Test: transient resolution failure preserves last-known-good values, condition stays True
- [x] 8.6 Test: NodePool checks AWSNodeClass readiness conditions before launching
- [x] 8.7 Test: instance profile cleanup finalizer runs on AWSNodeClass deletion (after in-use finalizer)
- [x] 8.8 Test: CEL validation rejects both `ami` and `amiSelector` set simultaneously
- [x] 8.9 Test: CEL validation rejects neither `ami` nor `amiSelector` set

## 9. Helm Chart and Samples

- [x] 9.1 Update CRD manifests in `deploy/charts/stratos/crds/` with regenerated CRDs
- [x] 9.2 Update Helm chart IAM policy templates to include new permissions: `ec2:DescribeImages`, `ec2:DescribeSubnets`, `ec2:DescribeSecurityGroups`, `iam:CreateInstanceProfile`, `iam:AddRoleToInstanceProfile`, `iam:RemoveRoleFromInstanceProfile`, `iam:DeleteInstanceProfile`, `iam:GetInstanceProfile`
- [x] 9.3 Add sample AWSNodeClass manifest using selectors (`deploy/samples/awsnodeclass_selectors.yaml`)
- [x] 9.4 Update existing sample manifests with comments showing the selector alternative
