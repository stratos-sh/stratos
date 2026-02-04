# Architecture Research: Warmup Image Pre-Pull Integration

**Domain:** Kubernetes operator feature -- adding container image pre-pull to warmup user data
**Researched:** 2026-02-04
**Confidence:** HIGH (every integration point verified by reading source code)

---

## Executive Summary

Adding warmup image pre-pull to Stratos requires threading a new field (`spec.preWarm.images` and `spec.preWarm.imagePullPolicy`) from the NodePool CRD through the controller to the user data generators. The architecture has a clean, well-defined data pipeline:

```
NodePool CRD spec --> controller --> TemplateConfig --> BootstrapConfig --> UserData generators --> shell script
```

The existing warmup script (`internal/cloudprovider/aws/warmup.go`) already runs during node warmup and waits for kubelet. Image pre-pull commands must be injected into this script after kubelet is healthy and before the warmup completion message. The change touches **7 files that need modification** and **0 new files** -- it integrates entirely into existing components.

Three AMI families handle user data differently:
- **AL2**: MIME multipart with separate warmup shell script -- image pulls go into the warmup script
- **AL2023**: Plain nodeadm YAML (no warmup script today) -- needs a warmup script added via MIME wrapper
- **Bottlerocket**: TOML config, no shell script -- image pre-pull is NOT possible via user data (Bottlerocket does not support arbitrary shell scripts in user data)

---

## 1. Complete Data Flow: CRD to User Data Script

### 1.1 Current Flow (No Image Pre-Pull)

This is the exact call chain verified by reading every file:

```
1. NodePool CR created/updated
   api/v1alpha1/nodepool_types.go: NodePoolSpec.PreWarm (*PreWarmConfig)

2. Reconciler detects deficit in standby count
   internal/controller/nodepool/pool_maintenance.go: replenishStandby()

3. replenishStandby() calls LaunchNode()
   internal/controller/nodepool/pool_maintenance.go:131
     --> nodeMgr.LaunchNode(ctx, nodePool, nodeClass, launcher)

4. LaunchNode builds TemplateConfig from NodePool spec
   internal/controller/nodepool/lifecycle/node_launch.go:41-45
     templateConfig := &cloudprovider.TemplateConfig{
       Labels:                      pool.Spec.Template.Labels,
       Taints:                      pool.Spec.Template.Taints,
       EnableNetworkReadinessTaint: pool.Spec.Template.IsNetworkReadinessTaintEnabled(),
     }

5. LaunchNode calls launcher.LaunchInstance()
   internal/controller/nodepool/lifecycle/node_launch.go:49
     --> launcher.LaunchInstance(ctx, nodeClass, pool.Name, m.clusterName, templateConfig)

6. AWSProvider.LaunchInstance() builds RunInstancesInput
   internal/cloudprovider/aws/provider.go:98
     input := p.buildRunInstancesInput(nodeClass, poolName, clusterName)

7. AWSProvider sets userData on the input
   internal/cloudprovider/aws/provider.go:100
     p.setUserData(input, nodeClass, poolName, templateConfig)

8. setUserData calls generateEncodedUserData()
   internal/cloudprovider/aws/provider.go:214
     --> p.generateEncodedUserData(nodeClass, poolName, templateConfig)

9. generateEncodedUserData builds BootstrapConfig
   internal/cloudprovider/aws/provider.go:242-257
     bootstrapConfig := &BootstrapConfig{
       ClusterName:                ...,
       ClusterEndpoint:            ...,
       ClusterCA:                  ...,
       ClusterCIDR:                ...,
       PoolName:                   poolName,
       BootstrapTemplate:          nodeClass.Spec.BootstrapTemplate,
       Kubelet:                    nodeClass.Spec.Kubelet,
       CustomUserData:             nodeClass.Spec.CustomUserData,
       TemplateLabels:             templateConfig.Labels,
       TemplateTaints:             templateConfig.Taints,
       EnableNetworkReadinessTaint: templateConfig.EnableNetworkReadinessTaint,
     }

10. GenerateUserData() dispatches to the right generator
    internal/cloudprovider/aws/userdata.go:86-92
      generator, _ := NewBootstrapGenerator(config.BootstrapTemplate)
      return generator.Generate(config)

11. AL2Generator.Generate() builds MIME multipart
    internal/cloudprovider/aws/al2.go:33-54
      Part 1: Bootstrap script (/etc/eks/bootstrap.sh)
      Part 2: Warmup script (getWarmupScript())     <-- IMAGE PULLS GO HERE
      Part 3: Optional custom userData

12. The warmup script is a constant string
    internal/cloudprovider/aws/warmup.go:24-60
      - Waits for kubelet health
      - Logs completion
      - Does NOT stop instance (controller handles that)
```

### 1.2 Key Observation: TemplateConfig is the Bridge

`TemplateConfig` (defined in `internal/cloudprovider/interface.go:27-31`) is the struct that carries NodePool template data from the controller layer to the cloud provider layer. Currently it has:

```go
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint
    EnableNetworkReadinessTaint bool
}
```

This is the struct that needs new fields to carry image pre-pull configuration through to the user data generators.

### 1.3 Key Observation: BootstrapConfig is the Final Destination

`BootstrapConfig` (defined in `internal/cloudprovider/aws/userdata.go:29-62`) is the struct that user data generators read to produce the actual script. Currently it has cluster config, pool config, kubelet config, labels, taints, and network readiness. It needs new fields for the image list and pull policy.

### 1.4 Key Observation: The Warmup Script is a Static Constant

`WarmupScript` in `internal/cloudprovider/aws/warmup.go` is a `const` string. To inject image pull commands, it must become dynamic -- either a function that takes parameters or a template.

---

## 2. Where Does Each File Need Changes?

### 2.1 Files That MUST Change (Modification Required)

| # | File | What Changes | Complexity |
|---|------|-------------|------------|
| 1 | `api/v1alpha1/config_types.go` | Add `Images []string` and `ImagePullPolicy` fields to `PreWarmConfig` | Low |
| 2 | `internal/cloudprovider/interface.go` | Add `WarmupImages []string` and `ImagePullPolicy string` to `TemplateConfig` | Low |
| 3 | `internal/controller/nodepool/lifecycle/node_launch.go` | Pass `pool.Spec.PreWarm` image fields into `TemplateConfig` | Low |
| 4 | `internal/cloudprovider/aws/provider.go` | Copy image fields from `templateConfig` to `BootstrapConfig` in `generateEncodedUserData()` | Low |
| 5 | `internal/cloudprovider/aws/userdata.go` | Add `WarmupImages []string` and `ImagePullPolicy string` to `BootstrapConfig` | Low |
| 6 | `internal/cloudprovider/aws/warmup.go` | Make warmup script dynamic -- generate `crictl pull` commands from image list | Medium |
| 7 | `internal/cloudprovider/aws/al2.go` | Pass images to warmup script generation (call `generateWarmupScript(config)` instead of `getWarmupScript()`) | Low |

### 2.2 Files That SHOULD Change (Test Updates)

| # | File | What Changes |
|---|------|-------------|
| 8 | `internal/cloudprovider/aws/userdata_test.go` | Add tests for image pre-pull in warmup script |
| 9 | `api/v1alpha1/config_types_test.go` | Add tests for new PreWarmConfig fields and defaults |
| 10 | `api/v1alpha1/zz_generated.deepcopy.go` | Auto-generated by `make generate` after CRD type changes |

### 2.3 Files That MAY Need Changes (AMI Family Considerations)

| # | File | What Changes | Condition |
|---|------|-------------|-----------|
| 11 | `internal/cloudprovider/aws/al2023.go` | Currently outputs plain nodeadm YAML with no warmup script. To support image pre-pull, AL2023 needs either a MIME multipart wrapper (like AL2) or a separate mechanism. | Only if AL2023 image pre-pull is in scope |
| 12 | `internal/cloudprovider/aws/bottlerocket.go` | Bottlerocket uses TOML config only. Image pre-pull via user data is NOT feasible for Bottlerocket. | Document as limitation, no code change |

### 2.4 Files That Do NOT Change

| File | Why No Change Needed |
|------|---------------------|
| `internal/cloudprovider/aws/provider.go` (buildRunInstancesInput) | Only setUserData needs changes; build input is unchanged |
| `internal/controller/nodepool/pool_maintenance.go` | Calls LaunchNode which already passes the full NodePool; no new data threading needed here |
| `internal/controller/nodepool/reconciler.go` | Does not directly participate in launch data flow |
| `internal/cloudprovider/fake/provider.go` | Fake provider does not generate real user data |
| `internal/controller/setup.go` | No new controller configuration needed |

---

## 3. Detailed Change Specification Per File

### 3.1 `api/v1alpha1/config_types.go` -- CRD Type Changes

**Current `PreWarmConfig` (lines 43-54):**
```go
type PreWarmConfig struct {
    Timeout       *metav1.Duration `json:"timeout,omitempty"`
    TimeoutAction *TimeoutAction   `json:"timeoutAction,omitempty"`
}
```

**Proposed additions:**
```go
type PreWarmConfig struct {
    Timeout        *metav1.Duration `json:"timeout,omitempty"`
    TimeoutAction  *TimeoutAction   `json:"timeoutAction,omitempty"`
    Images         []string         `json:"images,omitempty"`
    ImagePullPolicy *ImagePullPolicy `json:"imagePullPolicy,omitempty"`
}
```

New type needed in same file:
```go
type ImagePullPolicy string

const (
    ImagePullPolicyRequired   ImagePullPolicy = "Required"
    ImagePullPolicyBestEffort ImagePullPolicy = "BestEffort"
)
```

Also need a getter with default:
```go
func (c *PreWarmConfig) GetImagePullPolicy() ImagePullPolicy {
    if c == nil || c.ImagePullPolicy == nil {
        return ImagePullPolicyBestEffort
    }
    return *c.ImagePullPolicy
}

func (c *PreWarmConfig) GetImages() []string {
    if c == nil {
        return nil
    }
    return c.Images
}
```

**After changing types:** Run `make generate` to regenerate `zz_generated.deepcopy.go`.

### 3.2 `internal/cloudprovider/interface.go` -- TemplateConfig Extension

**Current `TemplateConfig` (lines 27-31):**
```go
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint
    EnableNetworkReadinessTaint bool
}
```

**Proposed additions:**
```go
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint
    EnableNetworkReadinessTaint bool
    WarmupImages                []string
    ImagePullPolicy             string
}
```

**Why `string` not the enum type:** `TemplateConfig` is in the cloud-agnostic `cloudprovider` package. It should not import `api/v1alpha1` types. The controller layer converts from the typed enum to a string.

### 3.3 `internal/controller/nodepool/lifecycle/node_launch.go` -- Thread Images Through

**Current TemplateConfig construction (lines 41-45):**
```go
templateConfig := &cloudprovider.TemplateConfig{
    Labels:                      pool.Spec.Template.Labels,
    Taints:                      pool.Spec.Template.Taints,
    EnableNetworkReadinessTaint: pool.Spec.Template.IsNetworkReadinessTaintEnabled(),
}
```

**Proposed change:**
```go
templateConfig := &cloudprovider.TemplateConfig{
    Labels:                      pool.Spec.Template.Labels,
    Taints:                      pool.Spec.Template.Taints,
    EnableNetworkReadinessTaint: pool.Spec.Template.IsNetworkReadinessTaintEnabled(),
    WarmupImages:                pool.Spec.PreWarm.GetImages(),
    ImagePullPolicy:             string(pool.Spec.PreWarm.GetImagePullPolicy()),
}
```

**Dependency note:** `pool.Spec.PreWarm` may be nil. The getter methods on `*PreWarmConfig` handle nil receiver (already the pattern used by `GetTimeout()` and `GetTimeoutAction()` in the same file).

### 3.4 `internal/cloudprovider/aws/provider.go` -- Pass to BootstrapConfig

**Current BootstrapConfig construction in `generateEncodedUserData()` (lines 242-257):**
```go
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
}
```

**Proposed addition inside the `if templateConfig != nil` block:**
```go
if templateConfig != nil {
    bootstrapConfig.TemplateLabels = templateConfig.Labels
    bootstrapConfig.TemplateTaints = templateConfig.Taints
    bootstrapConfig.EnableNetworkReadinessTaint = templateConfig.EnableNetworkReadinessTaint
    bootstrapConfig.WarmupImages = templateConfig.WarmupImages
    bootstrapConfig.ImagePullPolicy = templateConfig.ImagePullPolicy
}
```

### 3.5 `internal/cloudprovider/aws/userdata.go` -- BootstrapConfig Extension

**Current BootstrapConfig (lines 29-62):**
Add two new fields:

```go
type BootstrapConfig struct {
    // ... existing fields ...

    // WarmupImages is a list of container images to pre-pull during warmup
    WarmupImages []string

    // ImagePullPolicy controls behavior when image pulls fail ("Required" or "BestEffort")
    ImagePullPolicy string
}
```

### 3.6 `internal/cloudprovider/aws/warmup.go` -- Dynamic Warmup Script

**Current implementation:** A `const` string returned by `getWarmupScript()`.

**Required change:** Convert from static constant to a function that generates the script with image pull commands injected.

The current script structure is:
```bash
#!/bin/bash
set -euo pipefail
# Wait for kubelet health
# ...
log "Warmup script completed."
```

The new script must inject between "kubelet is healthy" and "warmup completed":
```bash
#!/bin/bash
set -euo pipefail
# Wait for kubelet health
# ...
log "Kubelet is healthy"

# Pre-pull container images
log "Pre-pulling container images..."
crictl pull docker.io/nginx:1.25 || log "WARNING: Failed to pull docker.io/nginx:1.25"
crictl pull myapp:latest || log "WARNING: Failed to pull myapp:latest"
log "Image pre-pull completed"

log "Warmup script completed."
```

**Design choices:**
- Use `crictl pull` (not `ctr` or `nerdctl`) because `crictl` is the standard CRI tool available on all EKS AMIs
- For `BestEffort` policy: use `|| log "WARNING: ..."` so failures do not abort the script
- For `Required` policy: use `||` with an exit code that signals failure
- The function signature changes from `getWarmupScript() string` to `generateWarmupScript(images []string, pullPolicy string) string`

### 3.7 `internal/cloudprovider/aws/al2.go` -- Call New Function

**Current (line 45):**
```go
warmupScript := getWarmupScript()
```

**Proposed change:**
```go
warmupScript := generateWarmupScript(config.WarmupImages, config.ImagePullPolicy)
```

---

## 4. AMI Family Analysis

### 4.1 AL2 (Amazon Linux 2) -- Full Support

**Current behavior:**
- Uses MIME multipart user data
- Part 1: Bootstrap script (`/etc/eks/bootstrap.sh`)
- Part 2: Warmup script (standalone shell script)
- Part 3: Optional custom user data

**Image pre-pull integration:** Straightforward. The warmup script already runs as a standalone shell script in MIME Part 2. Inject `crictl pull` commands into that script.

**Container runtime:** containerd (since EKS 1.24+). `crictl` is available on all EKS AL2 AMIs.

### 4.2 AL2023 (Amazon Linux 2023) -- Needs Architecture Decision

**Current behavior (lines 32-39 of al2023.go):**
```go
func (g *AL2023Generator) Generate(config *BootstrapConfig) (string, error) {
    // Just return the NodeConfig YAML - nodeadm will process it directly
    return g.generateNodeadmConfig(config), nil
}
```

AL2023 currently outputs ONLY a plain nodeadm `NodeConfig` YAML. There is NO warmup script included. There is NO MIME multipart wrapper.

**Problem:** To inject a shell script for image pre-pull, AL2023 must switch to MIME multipart format. The MIME helper functions (`buildMIMEMultipart`, `mimePartShellScript`) already exist in `al2023.go` (lines 131-152) but are currently unused by AL2023's `Generate()` method.

**Architecture decision options:**

1. **Wrap in MIME multipart (like AL2):** Change AL2023 to output MIME multipart when warmup images are specified:
   - Part 1: nodeadm NodeConfig YAML (as `Content-Type: application/node.eks.aws`)
   - Part 2: Warmup script with image pulls (as `text/x-shellscript`)
   - nodeadm supports MIME multipart with mixed content types

2. **Use nodeadm hooks:** nodeadm may support lifecycle hooks that can run scripts. This would need research into nodeadm's current capabilities.

3. **Defer AL2023 support:** Ship image pre-pull for AL2 first, add AL2023 in a follow-up.

**Recommendation:** Option 1 is the cleanest path. The MIME multipart helpers are already present in the codebase. nodeadm on AL2023 supports MIME multipart natively -- that is the standard approach for adding custom scripts alongside NodeConfig.

### 4.3 Bottlerocket -- NOT Supported

**Current behavior:** Outputs TOML configuration only. The comment in `bottlerocket.go:30` explicitly states: "Bottlerocket does NOT use a warmup script."

**Problem:** Bottlerocket does not support arbitrary shell scripts in user data. The user data is purely declarative TOML consumed by the Bottlerocket API server.

**Possible workaround:** Bottlerocket supports `bootstrap-containers` which can run OCI images at boot. However, this is a fundamentally different mechanism and should be a separate feature.

**Recommendation:** Document Bottlerocket as unsupported for image pre-pull in the initial implementation. This is not a gap -- Bottlerocket users can use `bootstrap-containers` manually via `customUserData`.

---

## 5. Data Flow Diagram (After Changes)

```
NodePool CR
  spec.preWarm.images: ["nginx:1.25", "myapp:latest"]
  spec.preWarm.imagePullPolicy: "BestEffort"
      |
      v
[1] api/v1alpha1/config_types.go
    PreWarmConfig.Images []string
    PreWarmConfig.ImagePullPolicy *ImagePullPolicy
      |
      v
[2] internal/controller/nodepool/lifecycle/node_launch.go
    LaunchNode() reads pool.Spec.PreWarm.GetImages()
    Sets templateConfig.WarmupImages and templateConfig.ImagePullPolicy
      |
      v
[3] internal/cloudprovider/interface.go
    TemplateConfig.WarmupImages []string
    TemplateConfig.ImagePullPolicy string
      |
      v
[4] internal/cloudprovider/aws/provider.go
    generateEncodedUserData() copies to bootstrapConfig.WarmupImages
      |
      v
[5] internal/cloudprovider/aws/userdata.go
    BootstrapConfig.WarmupImages []string
    BootstrapConfig.ImagePullPolicy string
      |
      v
[6] internal/cloudprovider/aws/al2.go (or al2023.go)
    Calls generateWarmupScript(config.WarmupImages, config.ImagePullPolicy)
      |
      v
[7] internal/cloudprovider/aws/warmup.go
    generateWarmupScript() returns shell script with crictl pull commands
      |
      v
    EC2 UserData (base64 encoded) --> Instance launch
```

---

## 6. Integration Points and Boundaries

### 6.1 CRD Layer (api/v1alpha1/)

The new fields go on `PreWarmConfig`, NOT on `NodeTemplate` or `AWSNodeClassSpec`. This is correct because:

- Image pre-pull is a warmup-phase concern, not a node template concern
- The image list is workload-specific (tied to the pool's purpose), not infrastructure-specific (tied to the node class)
- `PreWarmConfig` already owns warmup behavior (`Timeout`, `TimeoutAction`)
- Existing patterns: `PreWarmConfig` is on `NodePoolSpec` (line 44 of nodepool_types.go), and `NodePoolSpec` flows to `LaunchNode()` which has access to `pool.Spec.PreWarm`

### 6.2 Controller Layer (lifecycle/)

`LaunchNode()` already has access to the full `*NodePool` object (line 37 of node_launch.go). It already extracts template config from `pool.Spec.Template`. Adding extraction of `pool.Spec.PreWarm` fields is the same pattern.

The `NodeLauncher` interface signature does NOT need to change:
```go
LaunchInstance(ctx, nodeClass, poolName, clusterName, templateConfig)
```

All new data flows through `templateConfig`, which is already the bag of NodePool-derived config.

### 6.3 Cloud Provider Layer (cloudprovider/aws/)

The `BootstrapConfig` struct is the final staging area. All generators read from it. Adding fields here makes them available to all generators without changing generator interfaces.

The `BootstrapGenerator` interface does NOT need to change:
```go
Generate(config *BootstrapConfig) (string, error)
```

### 6.4 Fake Provider (cloudprovider/fake/)

The `FakeProvider.LaunchInstance()` receives `*cloudprovider.TemplateConfig` but does not use it for user data generation (it just creates a mock instance). No changes needed to the fake provider for this feature.

However, tests that verify image pre-pull behavior should use the real user data generators directly (unit tests on `al2.go`, `warmup.go`), not integration tests through the fake provider.

---

## 7. Build Order (Suggested Phase Structure)

Based on the dependency chain, here is the recommended implementation order:

### Step 1: CRD Types + Code Generation
**Files:** `api/v1alpha1/config_types.go`, then `make generate` and `make manifests`
**Why first:** Everything downstream depends on these types existing. They compile independently.
**Verify:** `go build ./api/...`

### Step 2: TemplateConfig Extension
**Files:** `internal/cloudprovider/interface.go`
**Why second:** The controller layer needs this struct updated before it can thread data through.
**Verify:** `go build ./internal/cloudprovider/...`

### Step 3: BootstrapConfig Extension + Warmup Script Generator
**Files:** `internal/cloudprovider/aws/userdata.go`, `internal/cloudprovider/aws/warmup.go`
**Why together:** These are co-dependent. BootstrapConfig gets the fields; warmup.go reads them.
**Verify:** `go build ./internal/cloudprovider/aws/...` and unit tests

### Step 4: AL2 Generator Update
**Files:** `internal/cloudprovider/aws/al2.go`
**Why after step 3:** Uses the new `generateWarmupScript()` function from warmup.go.
**Verify:** `go test ./internal/cloudprovider/aws/... -run TestAL2`

### Step 5: Thread Data Through Controller
**Files:** `internal/controller/nodepool/lifecycle/node_launch.go`, `internal/cloudprovider/aws/provider.go`
**Why last in the chain:** These are the "wiring" files that connect CRD fields to the generators. All upstream (types) and downstream (generators) must be ready first.
**Verify:** `go build ./...`

### Step 6: AL2023 Support (Optional, can be deferred)
**Files:** `internal/cloudprovider/aws/al2023.go`
**Why separate:** Requires wrapping in MIME multipart, which is a non-trivial change to al2023.go's output format.
**Verify:** `go test ./internal/cloudprovider/aws/... -run TestAL2023`

### Step 7: Tests
**Files:** `internal/cloudprovider/aws/userdata_test.go`, `api/v1alpha1/config_types_test.go`
**Verify:** `make test`

---

## 8. Patterns to Follow

### 8.1 Nil-Safe Getter Pattern

The codebase consistently uses nil-safe getters on optional config structs. Examples:

```go
// Existing pattern in config_types.go
func (c *PreWarmConfig) GetTimeout() metav1.Duration {
    if c == nil || c.Timeout == nil {
        return metav1.Duration{Duration: 10 * 60 * 1000000000}
    }
    return *c.Timeout
}
```

Follow this pattern for `GetImages()` and `GetImagePullPolicy()`.

### 8.2 TemplateConfig as the Bridge

The codebase uses `TemplateConfig` to carry NodePool-derived data through to the cloud provider without the cloud provider importing CRD types. The new fields should use primitive types (`[]string`, `string`) not CRD enum types.

### 8.3 MIME Multipart Assembly

AL2 already demonstrates the pattern for multi-part user data:
```go
var parts []string
parts = append(parts, mimePartShellScript(bootstrapScript, "bootstrap.sh"))
parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))
return buildMIMEMultipart(parts), nil
```

The helper functions `mimePartShellScript()` and `buildMIMEMultipart()` are defined in `al2023.go` but are package-level functions accessible from any generator.

### 8.4 Deterministic Output

The codebase sorts labels and taints for deterministic output (important for test assertions and for avoiding unnecessary user data changes that would cause instance replacement). Image lists should also be sorted or preserved in user-specified order.

---

## 9. Anti-Patterns to Avoid

### 9.1 Do NOT Put Images on AWSNodeClassSpec

Images are workload-specific, not infrastructure-specific. They belong on `NodePoolSpec.PreWarm`, not on `AWSNodeClassSpec`. Multiple NodePools can share one AWSNodeClass but need different image lists.

### 9.2 Do NOT Change the CloudProvider Interface

`LaunchInstance` is intentionally NOT on the `CloudProvider` interface (see comment on line 36-40 of interface.go). The `NodeLauncher` interface is the correct abstraction. Neither interface needs to change -- all new data flows through `TemplateConfig`.

### 9.3 Do NOT Use `ctr` or `nerdctl` Instead of `crictl`

`crictl` is the standard CRI tool available on all EKS AMIs. While `ctr` (containerd CLI) exists, it is a lower-level tool and not guaranteed to be in PATH on all AMI families. `crictl` is the correct choice.

### 9.4 Do NOT Make the Warmup Script a Go Template

Go's `text/template` package is overkill here. The warmup script generation is a simple string builder with a loop. Use `strings.Builder` and `fmt.Fprintf` like the rest of the user data generators (see AL2, AL2023, Bottlerocket generators).

---

## 10. Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| AL2023 output format change breaks nodeadm | MEDIUM | Test with real EKS AL2023 AMI; nodeadm docs confirm MIME multipart support |
| `crictl pull` not available during warmup | LOW | crictl is installed by EKS bootstrap; kubelet health check ensures containerd is running |
| Large image pulls exceed warmup timeout | MEDIUM | Document that users should set `preWarm.timeout` appropriately for their image sizes |
| Image pull fails silently in BestEffort mode | LOW | Log each pull result; consider adding a warmup condition to NodePool status |
| Bottlerocket users expect image pre-pull | LOW | Document limitation clearly; suggest bootstrap-containers as alternative |

---

## 11. Summary of All Files Touched

### Must Modify (7 files)

| File | Change Type | Lines Affected |
|------|------------|----------------|
| `api/v1alpha1/config_types.go` | Add types and fields | ~20 new lines |
| `internal/cloudprovider/interface.go` | Add struct fields | ~2 new lines |
| `internal/controller/nodepool/lifecycle/node_launch.go` | Thread fields | ~2 new lines |
| `internal/cloudprovider/aws/provider.go` | Thread fields | ~2 new lines |
| `internal/cloudprovider/aws/userdata.go` | Add struct fields | ~6 new lines |
| `internal/cloudprovider/aws/warmup.go` | Rewrite to dynamic generation | ~40 changed lines |
| `internal/cloudprovider/aws/al2.go` | Update function call | ~1 changed line |

### Auto-Generated (1 file)

| File | How |
|------|-----|
| `api/v1alpha1/zz_generated.deepcopy.go` | `make generate` |

### Tests to Add/Update (2+ files)

| File | Test Coverage |
|------|--------------|
| `internal/cloudprovider/aws/userdata_test.go` | Warmup script with images, pull policy, empty images |
| `api/v1alpha1/config_types_test.go` | GetImages(), GetImagePullPolicy() nil safety |

### Optional / Deferred (1 file)

| File | Condition |
|------|-----------|
| `internal/cloudprovider/aws/al2023.go` | Only if AL2023 image pre-pull is in scope for initial release |

---

## Sources

All findings verified by reading source code directly:

- `api/v1alpha1/nodepool_types.go` -- NodePoolSpec, PreWarmConfig location
- `api/v1alpha1/config_types.go` -- PreWarmConfig struct, getter patterns
- `api/v1alpha1/aws_nodeclass_types.go` -- AWSNodeClassSpec (verified images do NOT belong here)
- `internal/cloudprovider/interface.go` -- CloudProvider interface, TemplateConfig struct
- `internal/cloudprovider/types.go` -- Instance, InstanceState types
- `internal/cloudprovider/aws/provider.go` -- LaunchInstance, setUserData, generateEncodedUserData flow
- `internal/cloudprovider/aws/userdata.go` -- BootstrapConfig, BootstrapGenerator interface, GenerateUserData dispatch
- `internal/cloudprovider/aws/al2.go` -- AL2Generator.Generate(), MIME multipart assembly
- `internal/cloudprovider/aws/al2023.go` -- AL2023Generator.Generate(), plain YAML output (no warmup script)
- `internal/cloudprovider/aws/bottlerocket.go` -- BottlerocketGenerator.Generate(), TOML only, no shell scripts
- `internal/cloudprovider/aws/warmup.go` -- WarmupScript const, getWarmupScript() function
- `internal/controller/nodepool/lifecycle/node_launch.go` -- LaunchNode(), TemplateConfig construction
- `internal/controller/nodepool/lifecycle/manager.go` -- NodeLauncher interface, Manager struct
- `internal/controller/nodepool/pool_maintenance.go` -- replenishStandby(), full launch call chain
- `internal/controller/nodepool/provider_cache.go` -- ensureCloudProvider(), AWS ClusterConfig wiring
- `internal/cloudprovider/fake/provider.go` -- FakeProvider (confirmed no user data generation)
- `internal/cloudprovider/aws/userdata_test.go` -- Existing test patterns for all generators

No external sources needed -- this is a codebase-internal architectural analysis.
