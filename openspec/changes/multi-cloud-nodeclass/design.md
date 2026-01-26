## Context

Stratos currently embeds AWS configuration directly in the NodePool CRD via `spec.template.cloudProvider.aws`. This tight coupling means:
- Adding GCP/Azure would bloat the NodePool schema with 30+ additional fields
- Each cloud has unique concepts (AMI vs Machine Image, Security Groups vs Firewall Rules) that don't map cleanly
- No reusability—identical AWS configs must be duplicated across NodePools
- Validation becomes complex with mutually exclusive cloud-specific fields

The Karpenter project solved this with separate `EC2NodeClass` CRDs referenced by `NodePool`. This pattern is proven at scale (Salesforce runs 1000+ EKS clusters with it).

**Current flow:**
```
NodePool.spec.template.cloudProvider.aws → manager.go converts → LaunchConfig → CloudProvider.LaunchInstance()
```

**Proposed flow:**
```
NodePool.spec.template.nodeClassRef → fetch AWSNodeClass → awsProvider.LaunchInstance(nodeClass)
```

## Goals / Non-Goals

**Goals:**
- Separate cloud-specific configuration into dedicated CRDs
- Enable NodeClass reuse across multiple NodePools
- Support future cloud providers without touching NodePool schema
- Provide deletion protection for in-use NodeClasses

**Non-Goals:**
- Supporting third-party/external NodeClass CRDs (future consideration)
- Automated migration from embedded config to NodeClass (v1alpha1 allows breaking changes)
- Multi-cloud within a single NodePool (one pool = one cloud)
- Namespaced NodeClasses (keeping cluster-scoped like NodePool)

## Decisions

### 1. Separate CRD per cloud provider (not discriminated union)

**Decision:** Create distinct CRD kinds: `AWSNodeClass`, `GCPNodeClass`, `AzureNodeClass`

**Alternatives considered:**
- Single `CloudNodeClass` with `provider: aws` discriminator and embedded configs
- Generic `NodeClass` with provider-specific annotations

**Rationale:** Discriminated union recreates the same bloat problem at a different level. Separate kinds enable:
- Clean per-provider validation (CEL rules, webhooks)
- Independent versioning and schema evolution
- Better IDE/kubectl completion
- Alignment with Karpenter pattern

### 2. Provider takes NodeClass directly (no intermediate LaunchConfig for launch)

**Decision:** Each cloud provider's `LaunchInstance` method takes its own NodeClass type directly. No intermediate `LaunchConfig` struct for launching.

**Alternatives considered:**
- Generic `LaunchConfig` struct that all providers use
- Converter functions to transform NodeClass → LaunchConfig

**Rationale:** The existing `LaunchConfig` is already AWS-specific (has `IAMInstanceProfile`, `SecurityGroupIDs`, `SubnetID`). Pretending it's generic creates false abstraction. Each cloud has different concepts—AWS has subnets, GCP has zones, Azure has availability sets. Let each provider own its launch logic entirely.

The `CloudProvider` interface remains for instance lifecycle operations (start, stop, terminate, get state) which only need instance IDs and are truly cloud-agnostic.

### 3. Simple NodeClassRef (kind + name only)

**Decision:** Reference structure with just `kind` and `name`, no `apiGroup`

```go
type NodeClassRef struct {
    Kind string `json:"kind"`  // e.g., "AWSNodeClass"
    Name string `json:"name"`  // e.g., "gpu-optimized"
}
```

**Alternatives considered:**
- Full ObjectReference with apiGroup, namespace, uid
- TypedLocalObjectReference

**Rationale:** External providers not supported initially, so apiGroup adds complexity without benefit. Cluster-scoped means no namespace. Can extend later if needed.

### 4. Finalizer for deletion protection

**Decision:** Add `stratos.sh/in-use` finalizer to NodeClass when referenced by any NodePool

**Alternatives considered:**
- Validating webhook to reject deletion
- Let deletion fail at reconcile time

**Rationale:** Finalizers are the Kubernetes-native pattern for preventing deletion of resources with dependents. Webhook approach requires additional infrastructure. Fail-at-reconcile gives poor UX.

### 5. Changes only affect new launches

**Decision:** Modifications to a NodeClass only apply to newly launched instances, not running nodes

**Alternatives considered:**
- Trigger re-launch of all nodes when NodeClass changes
- Version NodeClass and track which version each node uses

**Rationale:** Simplicity. Changing instance type or AMI mid-flight is dangerous. Users can drain and rebuild if they need to apply changes to existing nodes. This matches Karpenter behavior.

### 6. userData stays in NodeClass

**Decision:** Keep `userData` (and cloud equivalents like `startupScript`) entirely in the NodeClass

**Alternatives considered:**
- Split: common userData in NodePool, cloud-specific in NodeClass
- Extract cluster info to separate resource with templating
- Controller auto-injects cluster info

**Rationale:** userData is inherently tied to both cloud AND OS:
- Format differs by OS (MIME multipart for AL2023, TOML for Bottlerocket)
- Content includes cloud-specific operations (EBS volume initialization)
- Cluster info is the only portable piece, but even that's formatted differently per OS

Keeping it simple avoids templating complexity. Most users stick to one OS/cloud combination. Future features like image preloading can be designed with real requirements when needed.

**Future consideration:** Ship reference NodeClass templates for common configurations:
- `aws-al2023-nodeclass`
- `aws-bottlerocket-nodeclass`
- `aws-bottlerocket-cilium-nodeclass`

### 7. Minimal status subresource

**Decision:** Track only data we already have—no external API calls

```go
type AWSNodeClassStatus struct {
    NodePoolCount int32              `json:"nodePoolCount,omitempty"`
    Conditions    []metav1.Condition `json:"conditions,omitempty"`
}
```

**Conditions:**
- `Valid`: Spec passes validation
- `InUse`: At least one NodePool references this NodeClass

**Alternatives considered:**
- Rich status with AWS validation (AMI exists, subnets valid, etc.)
- No status subresource

**Rationale:** External API calls add latency and failure modes. Can add AWS validation later as opt-in. Status is still useful for showing usage and basic validation state.

## Risks / Trade-offs

**Risk: Controller must handle missing NodeClass gracefully**
→ Mitigation: Return clear error event on NodePool, requeue with backoff. Don't crash reconcile loop.

**Risk: Finalizer could orphan NodeClass if controller is down**
→ Mitigation: Standard Kubernetes behavior. Finalizer removal happens during normal reconcile. Not unique to this design.

**Risk: Breaking change requires user action**
→ Mitigation: Acceptable at v1alpha1. Document migration clearly. Provide sample manifests showing before/after.

**Trade-off: No hot-reload of NodeClass changes**
→ Accepted: Simplicity over flexibility. Users can drain pools to apply changes.

**Trade-off: Two resources to manage instead of one**
→ Accepted: Small overhead for significant schema clarity. Reusability offsets this for multi-pool deployments.

## Component Interaction

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         RECONCILIATION FLOW                             │
└─────────────────────────────────────────────────────────────────────────┘

  NodePoolReconciler                    NodeManager
        │                                    │
        │ 1. Get NodePool                    │
        │ 2. Get NodeClassRef                │
        │ 3. Fetch AWSNodeClass ─────────────┼──► K8s API
        │ 4. Validate exists                 │
        │                                    │
        ├────► LaunchNode(pool, nodeClass)───┤
        │                                    │
        │                                    │
        │      awsProvider.LaunchInstance()  │
        │        (takes AWSNodeClass directly│
        │         handles subnet selection   │
        │         internally)                │
        │                                    │


  Instance Lifecycle (generic - works with any cloud)
  ──────────────────────────────────────────────────

        │      CloudProvider.StartInstance(instanceID)
        │      CloudProvider.StopInstance(instanceID)
        │      CloudProvider.TerminateInstance(instanceID)
        │      CloudProvider.GetInstanceState(instanceID)
```

## Open Questions

1. **Should NodeClass validation be sync (webhook) or async (controller)?**
   - Webhook gives immediate feedback but adds operational complexity
   - Controller-based validation is simpler but errors surface later
   - Leaning toward: Start with controller-based, add webhook later if needed

2. **How to handle NodeClass updates while nodes are launching?**
   - Edge case: NodeClass changes mid-warmup
   - Options: Use resourceVersion to detect, or accept eventual consistency
   - Leaning toward: Accept eventual consistency (matches "changes only affect new launches")
