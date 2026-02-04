# Phase 20: Controller Data Threading - Research

**Researched:** 2026-02-04
**Domain:** Kubernetes controller data flow, Go interface design patterns
**Confidence:** HIGH

## Summary

This phase threads image pre-pull configuration from the NodePool CRD spec through the controller reconciliation loop to the cloud provider's user data generation. The research reveals a **single missing connection**: the controller's `AWSProvider.generateEncodedUserData()` function receives the NodePool via `LaunchInstance()` but does not populate `BootstrapConfig.PreWarmConfig` from `pool.Spec.PreWarm`.

The infrastructure from Phases 17-19 is complete and ready:
- Phase 17: CRD types with `spec.preWarm.images` and getters exist
- Phase 18: Warmup script generator (`internal/warmup`) produces bash from image config
- Phase 19: AMI generators (AL2, AL2023) consume `BootstrapConfig.PreWarmConfig` and generate MIME multipart with image pull scripts; Bottlerocket generator ignores images (no warmup script support)

**Primary recommendation:** Extend `cloudprovider.TemplateConfig` with a `PreWarmConfig` field to carry image configuration from controller to provider, then populate it in the controller's `LaunchNode()` function and consume it in `AWSProvider.generateEncodedUserData()`.

## Standard Stack

### Core Components (Already Built)

| Component | Location | Purpose | Status |
|-----------|----------|---------|--------|
| CRD Types | `api/v1alpha1/config_types.go` | PreWarmConfig struct with Images, ImagePullPolicy, getters | Phase 17 ✅ |
| Warmup Generator | `internal/warmup/generator.go` | GenerateScript(images, policy) produces bash | Phase 18 ✅ |
| AMI Generators | `internal/cloudprovider/aws/al2.go`, `al2023.go`, `bottlerocket.go` | Consume BootstrapConfig.PreWarmConfig | Phase 19 ✅ |
| MIME Utilities | `internal/cloudprovider/aws/mime.go` | buildMIMEMultipart, checkUserDataSize with 14 KiB warning, 16 KiB error | Phase 19 ✅ |
| Validation | `internal/controller/nodepool/nodepool_validation.go` | checkImagePrePullSupport() sets ImagePrePullSupported condition | Phase 19 ✅ |

### Data Flow Pattern (Kubernetes Controllers)

The standard Kubernetes controller pattern for data flow:

```
CRD Spec → Reconciler → Provider Interface → Cloud-Specific Implementation → User Data
```

**Pattern used by Karpenter** (source: [Karpenter NodePools](https://karpenter.sh/docs/concepts/nodepools/), [How to Create Kubernetes Karpenter Node Pools](https://oneuptime.com/blog/post/2026-01-30-kubernetes-karpenter-node-pools/view)):
- NodePool references NodeClass via `spec.template.spec.nodeClassRef`
- Reconciler fetches both resources
- Passes NodePool template config + NodeClass to provider
- Provider generates cloud-specific configuration (AWS user data, etc.)

**Pattern used by Cluster API** (source: [Azure Integration](https://deepwiki.com/kubernetes-sigs/cluster-api-provider-azure/7-azure-service-operator-integration), [Nutanix + Cluster API](https://medium.com/@deepak.muley/nutanix-cluster-api-infrastructure-provider-deep-dive-bcffa6083428)):
- Separates "what to create" (specifications) from "how to create it" (service implementations)
- Each scope object implements interface methods that return specification getters
- Provider implementations consume these specifications

**Stratos current pattern** (verified in codebase):
- `Manager.LaunchNode()` receives full `pool *stratosv1alpha1.NodePool`
- Builds `cloudprovider.TemplateConfig` with labels, taints, network readiness setting
- Passes TemplateConfig to `launcher.LaunchInstance(ctx, nodeClass, poolName, clusterName, templateConfig)`
- `AWSProvider.LaunchInstance()` calls `generateEncodedUserData(nodeClass, poolName, templateConfig)`
- `generateEncodedUserData()` creates `BootstrapConfig` and populates fields from templateConfig
- **Missing**: PreWarmConfig population from `pool.Spec.PreWarm`

## Architecture Patterns

### Current Data Flow (Verified)

```
NodePool CRD
  └─> controller/nodepool/lifecycle/node_launch.go: LaunchNode(pool, nodeClass, launcher)
        └─> Builds cloudprovider.TemplateConfig from pool.Spec.Template
        └─> launcher.LaunchInstance(ctx, nodeClass, poolName, clusterName, templateConfig)
              └─> internal/cloudprovider/aws/provider.go: AWSProvider.LaunchInstance()
                    └─> generateEncodedUserData(nodeClass, poolName, templateConfig)
                          └─> Creates BootstrapConfig
                          └─> Populates from templateConfig (labels, taints, network readiness)
                          └─> [MISSING] Does NOT populate PreWarmConfig
                          └─> GenerateUserData(bootstrapConfig)
                                └─> NewBootstrapGenerator(bootstrapConfig.BootstrapTemplate)
                                └─> generator.Generate(bootstrapConfig)
                                      └─> AL2/AL2023: Checks bootstrapConfig.PreWarmConfig
                                      └─> If images configured, calls warmup.GenerateScript()
                                      └─> Builds MIME multipart with image pull script
                                      └─> checkUserDataSize() validates against 16 KiB limit
```

### Missing Connection Point

**Location**: `internal/cloudprovider/aws/provider.go:241-257`

```go
// CURRENT (Phase 19 state)
func (p *AWSProvider) generateEncodedUserData(
    nodeClass *stratosv1alpha1.AWSNodeClass,
    poolName string,
    templateConfig *cloudprovider.TemplateConfig) (string, error) {

    bootstrapConfig := &BootstrapConfig{
        ClusterName:       p.clusterConfig.Name,
        ClusterEndpoint:   p.clusterConfig.APIServerEndpoint,
        ClusterCA:         p.clusterConfig.CertificateAuthority,
        ClusterCIDR:       p.clusterConfig.CIDR,
        PoolName:          poolName,
        BootstrapTemplate: nodeClass.Spec.BootstrapTemplate,
        Kubelet:           nodeClass.Spec.Kubelet,
        CustomUserData:    nodeClass.Spec.CustomUserData,
    }

    if templateConfig != nil {
        bootstrapConfig.TemplateLabels = templateConfig.Labels
        bootstrapConfig.TemplateTaints = templateConfig.Taints
        bootstrapConfig.EnableNetworkReadinessTaint = templateConfig.EnableNetworkReadinessTaint
        // MISSING: bootstrapConfig.PreWarmConfig = ???
    }

    // ... rest of function
}
```

**Problem**: The function has no access to `pool.Spec.PreWarm`. The `templateConfig` parameter doesn't carry it.

### Recommended Pattern: Extend TemplateConfig

Following Go interface design best practices (source: [A clean way to pass configs in a Go application](https://dev.to/ilyakaznacheev/a-clean-way-to-pass-configs-in-a-go-application-1g64), [Mastering Interface-based Configuration in Go](https://www.codingexplorations.com/blog/interface-based-configuration-go)):

**Option 1: Add PreWarmConfig to TemplateConfig (RECOMMENDED)**

```go
// internal/cloudprovider/interface.go
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint
    EnableNetworkReadinessTaint bool
    PreWarmConfig               *stratosv1alpha1.PreWarmConfig  // NEW FIELD
}
```

**Rationale**:
- TemplateConfig already carries NodePool template data (labels, taints)
- PreWarm is part of NodePool spec, not NodeClass
- Minimal change: one field addition, populated in one location
- Type safety: uses existing PreWarmConfig type with getters
- No interface changes: CloudProvider interface unchanged

**Option 2: Pass NodePool to LaunchInstance (NOT RECOMMENDED)**

Would require changing CloudProvider interface signature, impacting fake provider and tests. Violates separation of concerns (provider shouldn't need full NodePool).

**Option 3: Create PreWarmTemplateConfig wrapper (OVERCOMPLICATED)**

Adds unnecessary abstraction for a single field.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Passing configuration through layers | Custom parameter passing, global state | Struct fields in existing config objects | Go best practice: configuration structs are the standard pattern for carrying related data through function calls |
| Interface versioning | Breaking changes to interface signatures | Extend configuration structs passed as parameters | Allows adding fields without changing interface methods |
| Data validation | Ad-hoc nil checks scattered in code | Getter methods with defaults (already exist in PreWarmConfig) | Phase 17 built GetImages(), GetImagePullPolicy() with safe defaults |
| Size validation | Manual byte counting | checkUserDataSize() function (already exists) | Phase 19 built this with proper thresholds (14 KiB warn, 16 KiB error) |
| Image pull script generation | Inline bash in user data generators | warmup.GenerateScript() (already exists) | Phase 18 built comprehensive generator with ECR auth, retries, pinning |

## Common Pitfalls

### Pitfall 1: Forgetting to Wire checkImagePrePullSupport

**What goes wrong**: Controller launches instances with image config but doesn't set the `ImagePrePullSupported` condition, leaving users uninformed that Bottlerocket will ignore images.

**Why it happens**: Phase 19 implemented `checkImagePrePullSupport()` but intentionally did not wire it into the reconcile loop (deferred to Phase 20).

**How to avoid**: Call `r.checkImagePrePullSupport(pool, nodeClass)` in the reconcile loop before launching instances. The function is non-blocking (void return), so it won't prevent launches.

**Where to call**: Early in reconciliation, after fetching NodeClass but before any launch decisions. Suggested location: after `nodeClass, err := r.fetchNodeClass(...)` in the main reconcile function.

**Warning signs**: ImagePrePullSupported condition never appears on NodePool status.

### Pitfall 2: nil PreWarmConfig Pointer Dereferencing

**What goes wrong**: Panic when accessing `templateConfig.PreWarmConfig.Images` without nil check.

**Why it happens**: `pool.Spec.PreWarm` is an optional pointer field (`*PreWarmConfig`). Many NodePools won't have pre-warm configured.

**How to avoid**:
- In controller: `templateConfig.PreWarmConfig = pool.Spec.PreWarm` (direct assignment, nil is valid)
- In provider: `bootstrapConfig.PreWarmConfig = templateConfig.PreWarmConfig` (direct assignment)
- In generators: `if config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0` (already implemented in Phase 19)
- Use getters: `config.PreWarmConfig.GetImages()` returns empty slice when nil

**Warning signs**: Test failures with "invalid memory address" panics when testing NodePools without PreWarm.

### Pitfall 3: User Data Size Limit Exceeded

**What goes wrong**: LaunchInstance fails with "user data size exceeds EC2 limit" when too many images configured.

**Why it happens**: EC2 has a hard 16 KiB limit on user data. MIME multipart format adds overhead (~500 bytes). Each image reference adds ~50-200 bytes depending on URL length.

**How to avoid**: The generators already call `checkUserDataSize()` which:
- Returns error at 16 KiB (prevents launch with invalid data)
- Prints warning to stderr at 14 KiB (~85%, leaves buffer)
- Includes pool name in error for debugging

**Capacity estimation**:
- AL2023 MIME with 10 short images (~1000 chars total): ~3-4 KiB
- AL2023 MIME with 50 ECR images (long URLs): ~12-14 KiB (warning threshold)
- AL2 MIME is ~200 bytes larger (bash bootstrap script vs YAML)

**Warning signs**: Reconcile loop shows "user data size approaching EC2 limit" warnings in logs.

### Pitfall 4: Testing Without Checking Generated User Data

**What goes wrong**: Code change breaks user data format but tests pass because they only check function return errors.

**Why it happens**: User data generation has many layers (TemplateConfig → BootstrapConfig → Generator → MIME → base64), easy to verify wrong layer.

**How to avoid**: Phase 19 established testing pattern:
- Test generators directly with `generator.Generate(config)` and inspect output
- Verify MIME parts present (AL2/AL2023 when images configured)
- Verify image pull script contains expected image references
- Verify size checks trigger at correct thresholds
- See `internal/cloudprovider/aws/userdata_test.go` for examples

**Warning signs**: Integration tests pass but instances fail to join cluster with "invalid user data" in cloud-init logs.

### Pitfall 5: Assuming PreWarmConfig Changes Require CRD Regeneration

**What goes wrong**: Developer runs `make manifests` unnecessarily, or worse, doesn't run it when actually needed.

**Why it happens**: Confusion about when code changes require CRD regeneration.

**How to avoid**:
- **Adding PreWarmConfig to TemplateConfig**: NO CRD changes (TemplateConfig is internal Go struct, not part of CRD)
- **Changing PreWarmConfig fields in config_types.go**: YES, requires `make manifests` (it's part of NodePool CRD spec)
- **Changing BootstrapConfig fields in userdata.go**: NO (internal struct, not exposed to CRD)

**Rule of thumb**: If you edit `api/v1alpha1/*.go` with kubebuilder markers (`// +kubebuilder:...`), run `make manifests`. Otherwise, don't.

**Warning signs**: `kubectl describe nodepool` shows old schema after you changed CRD types.

## Code Examples

### Example 1: Extending TemplateConfig (Recommended Approach)

**Location**: `internal/cloudprovider/interface.go`

```go
// Source: Analysis of existing codebase pattern
// TemplateConfig holds NodePool template configuration for userData generation.
// This includes labels and taints that should be applied to nodes via kubelet flags.
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint // Permanent taints
    EnableNetworkReadinessTaint bool           // Whether to add stratos.sh/not-ready taint
    PreWarmConfig               *stratosv1alpha1.PreWarmConfig  // Image pre-pull configuration
}
```

**Rationale**: Mirrors existing fields. PreWarmConfig is nil-safe like other pointer fields in the codebase.

### Example 2: Populating TemplateConfig in Controller

**Location**: `internal/controller/nodepool/lifecycle/node_launch.go:37-49`

```go
// Source: Existing pattern in node_launch.go, extended
func (m *Manager) LaunchNode(ctx context.Context, pool *stratosv1alpha1.NodePool, nodeClass stratosv1alpha1.NodeClass, launcher NodeLauncher) (*corev1.Node, error) {
    logger := log.FromContext(ctx)

    // Build template config from NodePool spec
    templateConfig := &cloudprovider.TemplateConfig{
        Labels:                      pool.Spec.Template.Labels,
        Taints:                      pool.Spec.Template.Taints,
        EnableNetworkReadinessTaint: pool.Spec.Template.IsNetworkReadinessTaintEnabled(),
        PreWarmConfig:               pool.Spec.PreWarm,  // NEW: Add image config
    }

    // Launch the instance using the cloud-specific provider
    logger.Info("Launching instance", "pool", pool.Name, "nodeClass", nodeClass.GetName(), "instanceType", nodeClass.GetInstanceType())
    instance, err := launcher.LaunchInstance(ctx, nodeClass, pool.Name, m.clusterName, templateConfig)
    // ... rest of function unchanged
}
```

**Note**: `pool.Spec.PreWarm` is `*PreWarmConfig`, can be nil. Direct assignment is safe.

### Example 3: Consuming PreWarmConfig in Provider

**Location**: `internal/cloudprovider/aws/provider.go:241-257`

```go
// Source: Existing pattern in generateEncodedUserData, extended
func (p *AWSProvider) generateEncodedUserData(nodeClass *stratosv1alpha1.AWSNodeClass, poolName string, templateConfig *cloudprovider.TemplateConfig) (string, error) {
    bootstrapConfig := &BootstrapConfig{
        ClusterName:       p.clusterConfig.Name,
        ClusterEndpoint:   p.clusterConfig.APIServerEndpoint,
        ClusterCA:         p.clusterConfig.CertificateAuthority,
        ClusterCIDR:       p.clusterConfig.CIDR,
        PoolName:          poolName,
        BootstrapTemplate: nodeClass.Spec.BootstrapTemplate,
        Kubelet:           nodeClass.Spec.Kubelet,
        CustomUserData:    nodeClass.Spec.CustomUserData,
    }

    if templateConfig != nil {
        bootstrapConfig.TemplateLabels = templateConfig.Labels
        bootstrapConfig.TemplateTaints = templateConfig.Taints
        bootstrapConfig.EnableNetworkReadinessTaint = templateConfig.EnableNetworkReadinessTaint
        bootstrapConfig.PreWarmConfig = templateConfig.PreWarmConfig  // NEW: Pass through
    }

    userData, err := GenerateUserData(bootstrapConfig)
    // ... rest unchanged
}
```

**Note**: Generators already handle nil PreWarmConfig (Phase 19). No changes needed in al2.go, al2023.go, bottlerocket.go.

### Example 4: Wiring checkImagePrePullSupport

**Location**: `internal/controller/nodepool/nodepool_controller.go` (exact location TBD in planning)

```go
// Source: Phase 19 research and validation.go implementation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // ... fetch NodePool ...

    // Fetch NodeClass
    nodeClass, err := r.fetchNodeClass(ctx, pool.Spec.Template.NodeClassRef)
    if err != nil {
        // ... error handling ...
    }

    // Check image pre-pull support (non-blocking, sets condition only)
    r.checkImagePrePullSupport(&pool, nodeClass)

    // ... rest of reconciliation logic ...
}
```

**Note**: `checkImagePrePullSupport()` has void return, is non-blocking. Always safe to call early.

### Example 5: Testing Generated User Data with Images

**Source**: `internal/cloudprovider/aws/userdata_test.go:585-650` (Phase 19 pattern)

```go
func TestAL2023GeneratorWithImages(t *testing.T) {
    config := &BootstrapConfig{
        ClusterName:       "test-cluster",
        ClusterEndpoint:   "https://test.example.com",
        ClusterCA:         "dGVzdC1jYQ==",
        ClusterCIDR:       "172.20.0.0/16",
        PoolName:          "test-pool",
        BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
        PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
            Images: []string{
                "public.ecr.aws/eks/pause:3.5",
                "nginx:latest",
            },
        },
    }

    generator := &AL2023Generator{}
    userData, err := generator.Generate(config)

    // Verify no error
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Verify MIME multipart format
    if !strings.Contains(userData, "MIME-Version: 1.0") {
        t.Error("expected MIME multipart format")
    }

    // Verify NodeConfig part present
    if !strings.Contains(userData, "Content-Type: application/node.eks.aws") {
        t.Error("expected NodeConfig YAML part")
    }

    // Verify image pull script part present
    if !strings.Contains(userData, "Content-Disposition: attachment; filename=\"image-pull.sh\"") {
        t.Error("expected image pull script part")
    }

    // Verify images present in script
    if !strings.Contains(userData, "public.ecr.aws/eks/pause:3.5") {
        t.Error("expected pause image in script")
    }
    if !strings.Contains(userData, "nginx:latest") {
        t.Error("expected nginx image in script")
    }
}
```

## State of the Art

| Aspect | Phase 17-19 (Current) | Phase 20 Goal | Change Required |
|--------|----------------------|---------------|-----------------|
| CRD Schema | PreWarmConfig with images, imagePullPolicy, getters | No change | None |
| Warmup Script Generator | GenerateScript(images, policy) complete | No change | None |
| AMI Generators | Consume BootstrapConfig.PreWarmConfig | No change | None |
| TemplateConfig | Labels, Taints, NetworkReadinessTaint | Add PreWarmConfig field | 1 field addition |
| Controller LaunchNode | Builds TemplateConfig from pool.Spec.Template | Add PreWarmConfig population | 1 line addition |
| Provider generateUserData | Creates BootstrapConfig from templateConfig | Add PreWarmConfig passthrough | 1 line addition |
| Validation | checkImagePrePullSupport() exists, not called | Wire into reconcile loop | 1 function call |
| End-to-End Flow | Broken (PreWarmConfig never reaches generators) | Complete data threading | 4 small changes total |

**Current State**: Infrastructure complete but not wired. Generators can produce correct output when BootstrapConfig.PreWarmConfig is populated, but controller never populates it.

**Target State**: Image config flows from NodePool spec → controller → TemplateConfig → provider → BootstrapConfig → generator → user data with image pull script.

## Open Questions

### 1. Should TemplateConfig Validation Exist?

**What we know**: TemplateConfig is created in controller, passed to provider, used to populate BootstrapConfig. Currently no validation.

**What's unclear**: Should we validate PreWarmConfig fields (e.g., image name format, list length) at TemplateConfig creation time, or rely on generator-level validation?

**Recommendation**: No validation at TemplateConfig level. Reasons:
- Validation exists at CRD level (kubebuilder markers on PreWarmConfig)
- Generators already handle nil PreWarmConfig safely
- Size validation happens in generators (checkUserDataSize)
- TemplateConfig is just a data carrier, not a validation point
- Keep controller layer thin, push logic to generators

### 2. Should Fake Provider Support Image Pre-Pull?

**What we know**: Fake provider is used for testing. It implements CloudProvider interface and has `LaunchInstance()` method.

**What's unclear**: Should fake provider verify that PreWarmConfig was passed through correctly, or just ignore it?

**Recommendation**: Add verification hook in fake provider's LaunchInstance. Reasons:
- Tests should verify full data flow
- Hook pattern already exists in fake provider (see `internal/cloudprovider/fake/provider.go`)
- Allows integration tests to assert image config reached provider
- Example: `OnLaunchInstance func(nodeClass, poolName, clusterName, templateConfig) error`

### 3. When Should checkImagePrePullSupport Be Called?

**What we know**: Function exists, sets ImagePrePullSupported condition, doesn't block operations.

**What's unclear**: Should it be called:
- On every reconcile? (updates condition frequently)
- Only when pool.Spec.PreWarm changes? (requires tracking previous state)
- Only when nodeClassRef changes? (might miss AMI family updates)

**Recommendation**: Call on every reconcile, early in loop. Reasons:
- Simple: no state tracking needed
- Fast: just type assertion and condition set
- Idempotent: controller-runtime handles condition deduplication
- Ensures condition always reflects current NodeClass
- Matches controller-runtime pattern (reconcile establishes full desired state)

## Sources

### Primary (HIGH confidence)

**Codebase analysis** (verified directly):
- `api/v1alpha1/config_types.go` - PreWarmConfig type and getters
- `api/v1alpha1/nodepool_types.go` - NodePool CRD schema
- `internal/cloudprovider/interface.go` - CloudProvider interface and TemplateConfig
- `internal/cloudprovider/aws/provider.go` - AWSProvider.LaunchInstance and generateEncodedUserData
- `internal/cloudprovider/aws/userdata.go` - BootstrapConfig and GenerateUserData
- `internal/cloudprovider/aws/al2.go`, `al2023.go`, `bottlerocket.go` - Generator implementations
- `internal/cloudprovider/aws/mime.go` - MIME utilities and size validation
- `internal/controller/nodepool/lifecycle/node_launch.go` - Controller LaunchNode function
- `internal/warmup/generator.go` - Image pull script generator
- `.planning/phases/19-ami-generator-integration/19-VERIFICATION.md` - Phase 19 completion status

### Secondary (MEDIUM confidence)

- [Karpenter NodePools](https://karpenter.sh/docs/concepts/nodepools/) - NodePool/NodeClass reference pattern
- [How to Create Kubernetes Karpenter Node Pools](https://oneuptime.com/blog/post/2026-01-30-kubernetes-karpenter-node-pools/view) - Published Jan 2026, NodePool to NodeClass data flow
- [Azure Integration - Cluster API](https://deepwiki.com/kubernetes-sigs/cluster-api-provider-azure/7-azure-service-operator-integration) - CAPI specification pattern
- [Nutanix + Cluster API: Infrastructure Provider Deep Dive](https://medium.com/@deepak.muley/nutanix-cluster-api-infrastructure-provider-deep-dive-bcffa6083428) - Published Jan 2026, provider architecture
- [A clean way to pass configs in a Go application](https://dev.to/ilyakaznacheev/a-clean-way-to-pass-configs-in-a-go-application-1g64) - Go configuration struct pattern
- [Mastering Interface-based Configuration in Go](https://www.codingexplorations.com/blog/interface-based-configuration-go) - Interface-based config pattern

### Tertiary (LOW confidence)

- [What's in a controller? - The Kubebuilder Book](https://book.kubebuilder.io/cronjob-tutorial/controller-overview.html) - General controller concepts
- [Controllers and Reconciliation - Cluster API Book](https://release-1-7.cluster-api.sigs.k8s.io/developer/providers/implementers-guide/controllers_and_reconciliation) - High-level reconciliation patterns

## Metadata

**Confidence breakdown**:
- Data flow gap identification: HIGH - Verified by reading actual code, found exact missing line
- Recommended solution (TemplateConfig extension): HIGH - Matches existing codebase patterns, minimal change
- Common pitfalls: HIGH - Based on Phase 19 implementation and testing patterns
- Alternative approaches: MEDIUM - Not all alternatives fully prototyped

**Research date**: 2026-02-04
**Valid until**: 60 days (stable codebase, no fast-moving dependencies)

**Key finding**: Only 4 small code changes needed to complete end-to-end flow. All infrastructure from prior phases is production-ready and waiting to be connected.
