## Context

AWSNodeClass currently requires users to hardcode AWS resource IDs for AMIs, subnets, security groups, and instance profiles. The CRD fields `ami`, `subnetIds`, `securityGroupIds`, and `iamInstanceProfile` are all required. The AWS provider in `internal/cloudprovider/aws/provider.go` reads these directly from `nodeClass.Spec` during launch.

The controller currently has no resolution step — it passes static IDs straight to the EC2 RunInstances API. We need to add a resolution layer that translates selector-based lookups into concrete IDs, caches the results in AWSNodeClass status, and feeds them to the launch path.

The AWS provider currently only uses the `ec2` SDK client. Resolution will require additional AWS API calls (DescribeImages, DescribeSubnets, DescribeSecurityGroups) and optionally IAM API calls for instance profile management.

## Goals / Non-Goals

**Goals:**
- Users can specify tag-based selectors instead of hardcoded IDs for subnets, security groups, and AMIs
- Users can specify an IAM role name and have the controller manage the instance profile
- Users can configure IMDS metadata options
- Resolved resources are visible in AWSNodeClass status for debugging
- Existing static-ID manifests continue to work with zero changes
- Resolution happens at reconciliation time, not at launch time (fail early)

**Non-Goals:**
- Full Karpenter-style `SelectorTerms[]` with AND/OR composition — we use simple tag maps
- AMI family abstraction or automatic UserData generation based on AMI type
- Kubelet configuration in AWSNodeClass (belongs in UserData for Stratos)
- Instance type flexibility/selection within a single pool (Stratos pools are homogeneous)
- Automatic AMI rotation of existing standby instances when selector resolves to a new AMI
- Spot instance support

## Decisions

### 1. Selector fields are simple tag maps, not SelectorTerms arrays

**Decision:** Each selector is a `map[string]string` of tag key-value pairs (AND semantics). AMI selector adds extra fields (`name`, `owner`) as struct fields rather than overloading the tag map.

**Rationale:** Karpenter's `SelectorTerms` with OR-composition across terms and AND within terms is powerful but complex. Stratos pools are homogeneous — you don't need "AMIs matching tag A OR tag B." A single tag map with AND semantics covers the common case: "subnets tagged for this cluster in this environment."

**Alternative considered:** Full `[]SelectorTerm` array — rejected as over-engineering for the warm pool use case.

**Structs:**

```go
type AMISelector struct {
    // Tags to match (AND semantics)
    Tags map[string]string `json:"tags,omitempty"`
    // Name with wildcard support (e.g., "my-eks-ami-*")
    Name string `json:"name,omitempty"`
    // Owner account ID or alias ("self", "amazon")
    Owner string `json:"owner,omitempty"`
}

type SubnetSelector struct {
    // Tags to match (AND semantics)
    Tags map[string]string `json:"tags,omitempty"`
}

type SecurityGroupSelector struct {
    // Tags to match (AND semantics)
    Tags map[string]string `json:"tags,omitempty"`
    // Name with wildcard support
    Name string `json:"name,omitempty"`
}
```

### 2. Mutual exclusivity enforced via CEL validation

**Decision:** For each resource type, exactly one of the static field or the selector field must be set. Enforced with kubebuilder CEL validation markers on the CRD, not in controller code.

**Rationale:** CEL runs at admission time, giving immediate feedback. The existing `+kubebuilder:validation:Required` markers on `ami`, `subnetIds`, `securityGroupIds`, `iamInstanceProfile` must be removed and replaced with CEL rules that check "one of X or Y is set."

**Example:** `+kubebuilder:validation:XValidation:rule="has(self.ami) || has(self.amiSelector)",message="one of ami or amiSelector is required"`

### 3. Resolution happens in a dedicated resolver, results cached in status

**Decision:** Add a new file `internal/cloudprovider/aws/resolver.go` containing a `ResourceResolver` struct with methods: `ResolveAMI`, `ResolveSubnets`, `ResolveSecurityGroups`, `ResolveInstanceProfile`. Results are written to `AWSNodeClassStatus` fields. The launch path reads from status (resolved IDs) rather than spec when selectors are used.

**Rationale:** Separating resolution from launch means:
- Resolution errors surface early (at reconcile time, not launch time)
- Resolved values are visible to users via `kubectl get awsnc -o yaml`
- Resolution can be rate-limited independently of launch operations
- The launch path stays simple — it always reads IDs, either from spec (static) or status (resolved)

**Resolution flow:**
```
AWSNodeClass reconcile
  ├── spec.amiSelector set?
  │     ├── yes → resolver.ResolveAMI() → status.resolvedAMI
  │     └── no  → status.resolvedAMI = spec.ami
  ├── spec.subnetSelector set?
  │     ├── yes → resolver.ResolveSubnets() → status.resolvedSubnets
  │     └── no  → status.resolvedSubnets = spec.subnetIds (mapped to ResolvedSubnet structs)
  └── ... same for SGs, instance profile

LaunchInstance reads:
  - status.resolvedAMI (always populated)
  - status.resolvedSubnets[round-robin].ID
  - status.resolvedSecurityGroups[*].ID
  - status.resolvedInstanceProfile
```

### 4. Resolution frequency: every AWSNodeClass reconcile

**Decision:** Re-resolve selectors on every reconcile of the AWSNodeClass (triggered by changes to the resource or periodic resync). Do not add a separate timer or watch.

**Rationale:** The controller already periodically reconciles. Re-resolving on each pass catches infrastructure changes (new subnets, updated AMIs) without adding complexity. AWS API costs are negligible — these are read-only Describe calls. If resolution returns different results, status is updated and a condition change signals the difference.

**Considered alternative:** Resolve only on AWSNodeClass create/update — rejected because it misses infrastructure changes (e.g., new subnet added with matching tags).

### 5. AMI resolution picks the newest matching AMI

**Decision:** When `amiSelector` matches multiple AMIs, select the most recently created one (sorted by `CreationDate`). Store the selected AMI ID in `status.resolvedAMI`.

**Rationale:** Users tagging AMIs for Stratos expect "latest matching AMI" behavior, consistent with Karpenter. The resolved AMI is pinned in status — existing standby instances are NOT rotated. AMI changes only affect new launches (consistent with existing spec: "NodeClass changes only affect new launches").

### 6. Instance profile management uses a finalizer for cleanup

**Decision:** When `spec.role` is set, the controller creates an instance profile named `stratos-<cluster-name>-<awsnodeclass-name>`, adds the specified role, and stores the profile ARN in `status.resolvedInstanceProfile`. A finalizer ensures cleanup (remove role, delete profile) on AWSNodeClass deletion.

**Rationale:** Instance profiles are IAM resources (global within an AWS account), while AWSNodeClass is a Kubernetes resource (scoped to a cluster). Including the cluster name in the profile name prevents collisions when multiple clusters run Stratos in the same AWS account. The controller already has `--cluster-name` available. The finalizer pattern is already used for the `stratos.sh/in-use` finalizer, so the pattern is established.

**Role update handling:** If `spec.role` is changed from "role-a" to "role-b", the controller SHALL remove "role-a" from the instance profile and add "role-b". Instance profiles support only one role, so this is a remove-then-add operation. If removal fails, set `InstanceProfileReady=False` and retry on next reconcile.

**Finalizer ordering:** The `stratos.sh/in-use` finalizer (blocks deletion while NodePools reference) SHALL be processed before the instance profile cleanup finalizer. This ensures no NodePools are launching with the profile while it's being deleted.

**IAM API calls:**
- Create: `iam:CreateInstanceProfile`, `iam:AddRoleToInstanceProfile`
- Delete: `iam:RemoveRoleFromInstanceProfile`, `iam:DeleteInstanceProfile`
- Check: `iam:GetInstanceProfile` (to verify existence before launch)

### 7. MetadataOptions is a simple struct, not a selector

**Decision:** Add `metadataOptions` as a direct configuration field (not a selector). Maps directly to EC2 `InstanceMetadataOptionsRequest`.

```go
type MetadataOptions struct {
    HTTPTokens              string `json:"httpTokens,omitempty"`              // "required" or "optional"
    HTTPPutResponseHopLimit int32  `json:"httpPutResponseHopLimit,omitempty"` // 1-64
    HTTPEndpoint            string `json:"httpEndpoint,omitempty"`            // "enabled" or "disabled"
}
```

**Rationale:** This is pure configuration passthrough, no resolution needed. Defaults to EC2 defaults if not specified.

### 8. New file for resolver, IAM client added to provider

**Decision:** Create `internal/cloudprovider/aws/resolver.go` for resolution logic. The `AWSProvider` struct gains an optional `iam.Client` field, initialized only when needed (lazy or at construction time when role-based profiles are detected). The resolver is a separate struct that takes both ec2 and iam clients.

**Rationale:** Keeps `provider.go` focused on instance lifecycle. Resolution is a distinct concern with its own AWS API calls and error handling.

### 9. Dedicated AWSNodeClass reconciler in the AWS provider package

**Decision:** Add a dedicated AWSNodeClass reconciler in `internal/cloudprovider/aws/` (e.g., `internal/cloudprovider/aws/nodeclass_controller.go`). This reconciler handles resource resolution independently of the NodePool controller. It is registered with the manager in `cmd/stratos/main.go`.

**Rationale:** The AWSNodeClass reconciler is inherently AWS-specific — it reconciles an AWS resource, calls AWS APIs (Describe, IAM), and uses the resolver. Placing it in the AWS provider package keeps all AWS-specific code in one place and avoids the `internal/controller/` package importing `internal/cloudprovider/aws/`. The resolver interface, its implementation, and the reconciler that uses it all colocate in the same package. The generic NodePool controller in `internal/controller/` never imports cloud-specific packages for resolution — it only reads from AWSNodeClass status fields.

**Responsibilities:**
- Run selector resolution (AMI, subnets, SGs) and write results to status
- Manage instance profile lifecycle (create/update/delete) when `spec.role` is set
- Set readiness conditions (`AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`)
- Handle finalizer for instance profile cleanup on deletion

**The NodePool controller** no longer handles resolution. It reads from AWSNodeClass status (resolved fields) and checks conditions before launching. It still manages the `stratos.sh/in-use` finalizer and `nodePoolCount` status field.

### 10. LocalStack integration tests for resolver validation

**Decision:** Use `testcontainers-go` with LocalStack to integration test the resolver against real AWS SDK calls. Tests live in `internal/cloudprovider/aws/resolver_integration_test.go` with a `//go:build integration` tag.

**Rationale:** The resolver translates user-provided selectors into AWS API calls (DescribeImages filters, DescribeSubnets tag filters, DescribeSecurityGroups name filters, IAM instance profile lifecycle). Unit tests with mocked EC2/IAM clients only verify "did I call the mock correctly" — they don't catch wrong filter syntax, incorrect parameter names, or response parsing bugs. LocalStack lets us test the actual SDK call chain against a real (emulated) API.

**Test architecture:**

```
┌──────────────────────────────────────────────────────┐
│  Test Layer            What it validates             │
├──────────────────────────────────────────────────────┤
│  Unit tests            Controller logic: selector    │
│  (mocked resolver)     in spec → resolved IDs in    │
│                        status, conditions, launch    │
│                        reads from status             │
│                                                      │
│  LocalStack tests      AWS SDK calls: filter syntax, │
│  (testcontainers)      tag matching, name wildcards, │
│                        newest AMI selection,          │
│                        IAM profile create/delete      │
│                        round-trip                     │
│                                                      │
│  envtest tests         Full controller reconcile     │
│  (existing)            with injected fake resolver   │
└──────────────────────────────────────────────────────┘
```

**LocalStack test setup:**
1. `testcontainers-go` starts a LocalStack container (EC2 + IAM services)
2. Test seeds AWS resources: `CreateSubnet` with tags, `CreateSecurityGroup` with tags/name, `RegisterImage` with name/tags, `CreateRole`
3. `ResourceResolver` is constructed pointing at LocalStack endpoint (`aws.EndpointResolverWithOptionsFunc`)
4. Tests call resolver methods and assert resolved IDs match seeded resources
5. Container is shared across all tests in the suite (started once in `TestMain`)

**Test cases covered by LocalStack:**
- Subnet resolution: tags match correct subnets, no match returns error
- Security group resolution: tag match, name wildcard match, no match returns error
- AMI resolution: tag match, name wildcard, owner filter, newest-wins selection across multiple matches
- Instance profile: create from role, add role, verify existence, delete on cleanup
- Edge cases: empty tag values, special characters in names, pagination (if results exceed single page)

**Dependencies added:** `github.com/testcontainers/testcontainers-go` and `github.com/testcontainers/testcontainers-go/modules/localstack`. Docker required in CI.

**Alternative considered:** Mocked EC2/IAM client interfaces only — rejected because filter syntax bugs and parameter formatting issues are the highest-risk bugs in resolver code, and mocks don't catch them.

### 10. Resolver is injectable for controller-level tests

**Decision:** Define a `Resolver` interface in `internal/cloudprovider/aws/resolver.go` that the controller depends on. The real implementation calls AWS APIs (or LocalStack in tests). For envtest integration tests, inject a fake resolver that returns canned responses.

```go
type Resolver interface {
    ResolveAMI(ctx context.Context, selector *AMISelector) (string, error)
    ResolveSubnets(ctx context.Context, selector *SubnetSelector) ([]ResolvedSubnet, error)
    ResolveSecurityGroups(ctx context.Context, selector *SecurityGroupSelector) ([]ResolvedSecurityGroup, error)
    ResolveInstanceProfile(ctx context.Context, role string, profileName string) (string, error)
    DeleteInstanceProfile(ctx context.Context, profileName string) error
}
```

**Rationale:** This gives us two clean testing paths:
- LocalStack tests validate the real `AWSResolver` implementation (SDK calls are correct)
- envtest tests validate the controller flow (resolution results flow to status and launch) using a fake resolver

## Risks / Trade-offs

**[Risk] Transient resolution failure blocks launches** → If a Describe API call fails transiently (throttle, network error), the design must NOT wipe previously resolved values. The controller SHALL keep the last successfully resolved values in status and set a warning condition (e.g., `SubnetsReady=True` with a warning event "resolution failed, using cached values"). Only set the condition to False if resolution has NEVER succeeded (no cached values). This "last known good" semantics prevents a transient AWS blip from blocking all launches across all NodePools referencing that AWSNodeClass.

**[Risk] AWS API rate limits from Describe calls** → Resolution calls are already behind the existing `RateLimiter`. Add `DescribeImages`, `DescribeSubnets`, `DescribeSecurityGroups` as rate-limited operations. These are read-only and have generous limits.

**[Risk] Selector resolves to zero results** → Set condition `AMIReady=False` (or equivalent) with reason `NoMatchingResources`. Block launches until resolved. This is better than failing at launch time.

**[Risk] Selector resolves to different AMI over time** → Existing standby nodes keep their original AMI. Only new launches use the newly resolved AMI. This is consistent with the existing "changes only affect new launches" principle. Users who want to rotate must drain and re-warm. Documenting this behavior is important.

**[Risk] IAM instance profile creation fails due to permissions** → The controller's own IAM role needs expanded permissions. If `spec.role` is used but the controller lacks IAM permissions, set `InstanceProfileReady=False` with a clear message. Fall back guidance: use `spec.iamInstanceProfile` instead.

**[Risk] IAM eventual consistency** → After creating an instance profile, it may not be immediately usable. Add a brief wait or retry in the launch path when using a newly created profile. AWS IAM propagation typically takes a few seconds.

**[Trade-off] Simple tag maps vs. SelectorTerms** → We lose OR-composition across selectors. A user who needs "subnets tagged env=prod OR env=staging" would need two AWSNodeClass resources. This is acceptable for the warm pool use case where pools are intentionally homogeneous.

**[Trade-off] Resolution on every reconcile** → Slightly more AWS API calls than "resolve once." But it catches infrastructure drift and the calls are cheap read-only operations.

**[Trade-off] LocalStack in CI** → Adds Docker dependency to CI pipeline and ~5-10s container startup per test suite. Worth it because resolver bugs (wrong filter syntax, parameter names) are the highest-risk class of bugs and are invisible to mocked tests. LocalStack tests are scoped to `resolver_integration_test.go` only — not the full controller suite.

**[Risk] LocalStack fidelity gaps** → LocalStack doesn't perfectly match AWS behavior in all edge cases (pagination, throttling, some filter semantics). Mitigated by keeping LocalStack tests focused on common filter patterns, not AWS quirks. Real AWS behavior is ultimately validated in staging/prod.

## Open Questions

1. **Subnet selection strategy with resolved subnets** — When selectors resolve multiple subnets, should we enrich the selection beyond round-robin? Karpenter picks the subnet with the most available IPs. For now, keeping round-robin is simpler and sufficient for warm pools.
