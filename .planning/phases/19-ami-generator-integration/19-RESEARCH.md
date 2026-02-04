# Phase 19: AMI Generator Integration - Research

**Researched:** 2026-02-04
**Domain:** EC2 user data generation with MIME multipart integration and Kubernetes status conditions
**Confidence:** HIGH

## Summary

This phase integrates the warmup script generator (Phase 18) into existing AMI bootstrap generators by injecting image pull scripts as MIME parts in EC2 user data. The task spans three AMI families with different integration strategies: AL2 uses MIME multipart (already implemented), AL2023 switches from plain NodeConfig YAML to MIME multipart when images are configured, and Bottlerocket surfaces a validation warning via Kubernetes status condition.

**Key findings:**
- Existing codebase has AL2Generator using MIME multipart with `buildMIMEMultipart()` utility and fixed boundary `==STRATOS_MIME_BOUNDARY==`
- AL2023Generator currently outputs plain NodeConfig YAML; must switch to MIME multipart when images exist
- MIME multipart uses `text/x-shellscript` for shell scripts and `application/node.eks.aws` for AL2023 NodeConfig
- EC2 user data limit is 16,384 bytes (16 KiB) raw, before base64 encoding; multipart format counts toward this limit
- Kubernetes status conditions use `metav1.Condition` with `meta.SetStatusCondition()` from apimachinery/meta
- Bottlerocket uses TOML format with no shell script support; bootstrap containers exist but image persistence unclear
- AMI family detection available via `config.BootstrapTemplate` field (AL2023 | AL2 | Bottlerocket enum)

**Primary recommendation:** Extend existing MIME builder utilities, conditionally invoke warmup generator based on `PreWarm.Images` presence, add status condition for Bottlerocket unsupported scenario, and implement size warning at 14 KiB threshold (85% of limit).

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (fmt, strings) | Go 1.25+ | String building and MIME assembly | No dependencies, built-in |
| internal/warmup.GenerateScript | Phase 18 | Image pull script generation | Just implemented, designed for this integration |
| k8s.io/apimachinery/pkg/apis/meta/v1 | controller-runtime | Condition types and constants | Kubernetes standard for status conditions |
| k8s.io/apimachinery/pkg/api/meta | controller-runtime | meta.SetStatusCondition() | Standard helper for condition management |

**Why existing MIME utilities:**
- Codebase already has `buildMIMEMultipart()` and `mimePartShellScript()` in `al2023.go`
- Fixed boundary `==STRATOS_MIME_BOUNDARY==` provides deterministic output
- Functions are tested and working in AL2Generator

**Why Kubernetes status conditions:**
- Standard way to communicate resource state in custom resources
- Supported by kubectl describe and K8s API conventions
- Existing codebase uses `meta.SetStatusCondition()` in nodeclass and nodepool reconcilers
- Type-safe with `metav1.Condition` struct

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Go regexp | stdlib | ECR pattern detection (already in warmup generator) | Inherited from Phase 18 |
| encoding/base64 | stdlib | Calculate base64 size for warning threshold | When checking user data size |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Conditional MIME for AL2023 | Always use MIME multipart | Always-MIME simpler but breaks existing plain YAML format; conditional preserves backward compat |
| Status condition | Log warning only | Condition is discoverable via kubectl, persists in cluster state |
| Fixed 14 KiB threshold | Calculate exact script sizes | Simple threshold catches 85% of issues; exact calc would be complex and brittle |
| Separate condition types | Single "Warning" type | Specific type (ImagePrePullNotSupported) is more semantic and filterable |

## Architecture Patterns

### Recommended Code Structure

```
internal/cloudprovider/aws/
├── userdata.go            # Existing: BootstrapConfig, generator interface
├── al2.go                 # Existing: AL2Generator - add image pull MIME part
├── al2023.go              # Existing: AL2023Generator - add MIME conditional + image pull
├── bottlerocket.go        # Existing: BottlerocketGenerator - no changes
├── mime.go                # NEW: Extracted MIME utilities (buildMIMEMultipart, etc.)
└── userdata_test.go       # Existing: Add tests for image integration

internal/controller/nodepool/
└── reconciler.go          # Add Bottlerocket+images validation with status condition
```

**Rationale:**
- Extract MIME utilities to `mime.go` so both AL2 and AL2023 generators can import
- No changes to `bottlerocket.go` (TOML format incompatible with shell scripts)
- Validation logic lives in controller reconciler where NodePool spec is evaluated
- Tests co-located with generators in `userdata_test.go`

### Pattern 1: Conditional MIME Multipart for AL2023

**What:** AL2023 generator switches output format based on whether images are configured. Plain NodeConfig YAML when no images; MIME multipart with NodeConfig + image pull script when images exist.

**When to use:** When a feature requires shell script injection but backward compatibility with plain config format is important.

**Example:**
```go
// Source: Codebase analysis + AWS EKS AL2023 documentation
func (g *AL2023Generator) Generate(config *BootstrapConfig) (string, error) {
	nodeConfig := g.generateNodeadmConfig(config)

	// Check if images are configured
	hasImages := config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0

	if !hasImages {
		// Plain NodeConfig YAML (original behavior)
		return nodeConfig, nil
	}

	// MIME multipart with images
	var parts []string

	// Part 1: NodeConfig as application/node.eks.aws
	parts = append(parts, mimePartNodeConfig(nodeConfig))

	// Part 2: Image pull script as text/x-shellscript
	imagePullScript := warmup.GenerateScript(
		config.PreWarmConfig.Images,
		config.PreWarmConfig.ImagePullPolicy,
	)
	parts = append(parts, mimePartShellScript(imagePullScript, "image-pull.sh"))

	// Part 3: Warmup completion script
	warmupScript := getWarmupScript()
	parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))

	return buildMIMEMultipart(parts), nil
}
```

**Key decisions (from CONTEXT.md):**
- Use MIME only when images present (not always)
- Three parts: NodeConfig, image pull, warmup completion
- Image pull script is separate MIME part (not appended to bootstrap)
- NodeConfig uses `application/node.eks.aws` content type
- Shell scripts use `text/x-shellscript` content type

### Pattern 2: MIME Part Builders

**What:** Utility functions for creating MIME parts with correct headers and content types.

**When to use:** Any time you need to build MIME multipart user data with multiple content types.

**Example:**
```go
// Source: Existing codebase (al2023.go) + cloud-init documentation
// Move to mime.go for shared use

// mimePartShellScript creates a MIME part for a shell script
func mimePartShellScript(content, filename string) string {
	return fmt.Sprintf(`Content-Type: text/x-shellscript; charset="us-ascii"
Content-Disposition: attachment; filename="%s"

%s`, filename, content)
}

// mimePartNodeConfig creates a MIME part for AL2023 NodeConfig
func mimePartNodeConfig(content string) string {
	return fmt.Sprintf(`Content-Type: application/node.eks.aws; charset="us-ascii"
Content-Disposition: attachment; filename="nodeadm-config.yaml"

%s`, content)
}

// buildMIMEMultipart assembles parts into a MIME multipart message
func buildMIMEMultipart(parts []string) string {
	boundary := "==STRATOS_MIME_BOUNDARY=="
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"%s\"\n\n", boundary))

	for _, part := range parts {
		sb.WriteString(fmt.Sprintf("--%s\n%s\n", boundary, part))
	}

	sb.WriteString(fmt.Sprintf("--%s--\n", boundary))

	return sb.String()
}
```

**Why fixed boundary:** Deterministic output makes testing easier. Cloud-init doesn't require unique boundaries.

### Pattern 3: AL2 Image Pull Integration

**What:** AL2 already uses MIME multipart. Add image pull script as a middle part between bootstrap and warmup.

**When to use:** When extending existing MIME multipart user data with additional scripts.

**Example:**
```go
// Source: Existing codebase (al2.go) + Phase 18 integration
func (g *AL2Generator) Generate(config *BootstrapConfig) (string, error) {
	var parts []string

	// Part 1: Bootstrap script (unchanged)
	bootstrapScript := g.generateBootstrapScript(config)
	parts = append(parts, mimePartShellScript(bootstrapScript, "bootstrap.sh"))

	// Part 2: Image pull script (NEW - if images configured)
	if config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0 {
		imagePullScript := warmup.GenerateScript(
			config.PreWarmConfig.Images,
			config.PreWarmConfig.ImagePullPolicy,
		)
		parts = append(parts, mimePartShellScript(imagePullScript, "image-pull.sh"))
	}

	// Part 3: Warmup script (unchanged)
	warmupScript := getWarmupScript()
	parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))

	// Part 4: Optional custom userData (unchanged)
	if config.CustomUserData != "" {
		parts = append(parts, mimePartShellScript(config.CustomUserData, "custom-userdata.sh"))
	}

	return buildMIMEMultipart(parts), nil
}
```

**Order matters:** Bootstrap must run first (joins cluster), then image pull (requires containerd from bootstrap), then warmup completion.

### Pattern 4: Kubernetes Status Condition for Validation Warnings

**What:** Surface configuration warnings (like unsupported features) as status conditions on the NodePool resource.

**When to use:** When a configuration is valid but has limited functionality, and users need to be notified via K8s API.

**Example:**
```go
// Source: Codebase patterns (nodepool_status.go, nodeclass/reconciler.go) + Kubernetes conventions
import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// In nodepool reconciler, after fetching NodeClass
func (r *NodePoolReconciler) validateImagePrePull(nodePool *stratosv1alpha1.NodePool, nodeClass *stratosv1alpha1.AWSNodeClass) {
	// Check if images configured with Bottlerocket
	hasImages := nodePool.Spec.PreWarm != nil && len(nodePool.Spec.PreWarm.Images) > 0
	isBottlerocket := nodeClass.Spec.BootstrapTemplate == stratosv1alpha1.BootstrapTemplateBottlerocket

	if hasImages && isBottlerocket {
		condition := metav1.Condition{
			Type:    "ImagePrePullSupported",
			Status:  metav1.ConditionFalse,
			Reason:  "BottlerocketNotSupported",
			Message: "Image pre-pull is not supported on Bottlerocket AMIs. Instances will launch without cached images.",
			ObservedGeneration: nodePool.Generation,
		}
		meta.SetStatusCondition(&nodePool.Status.Conditions, condition)
	} else {
		// Remove condition if previously set
		meta.RemoveStatusCondition(&nodePool.Status.Conditions, "ImagePrePullSupported")
	}
}
```

**Condition semantics:**
- Type: `ImagePrePullSupported` (positive capability statement)
- Status: `False` when unsupported, `True` when supported
- Reason: Short machine-readable identifier
- Message: Human-readable explanation with impact

**Alternative condition type:** Could use `ImagePrePullNotSupported` with `Status: True` when unsupported. Chose positive form (`Supported = False`) to align with K8s conventions for capability conditions.

### Pattern 5: User Data Size Warning

**What:** Log warning when generated user data approaches the 16 KiB EC2 limit.

**When to use:** Any time you generate EC2 user data, especially with user-controlled content like image lists.

**Example:**
```go
// Source: AWS EC2 documentation + codebase logging patterns
func (g *AL2Generator) Generate(config *BootstrapConfig) (string, error) {
	// ... generate user data ...
	userData := buildMIMEMultipart(parts)

	// Check size (before base64 encoding)
	const warnThreshold = 14 * 1024 // 14 KiB = 85% of 16 KiB limit
	const hardLimit = 16 * 1024     // 16 KiB

	size := len(userData)
	if size > hardLimit {
		return "", fmt.Errorf("user data size %d bytes exceeds EC2 limit of %d bytes", size, hardLimit)
	}
	if size > warnThreshold {
		// Use structured logging from controller-runtime
		log := logr.FromContext(ctx)
		log.Info("User data size approaching limit",
			"size", size,
			"limit", hardLimit,
			"percentUsed", (size*100)/hardLimit,
			"nodeClass", config.NodeClassName,
		)
	}

	return userData, nil
}
```

**Threshold rationale:**
- 14 KiB = 85% of limit, gives headroom for minor changes
- Hard error at 16 KiB prevents EC2 API failures
- Log includes percentage and context for troubleshooting

**Note:** Size check should happen in generator, but logging context needs to flow from controller. Consider passing logger or adding size check in controller wrapper.

### Pattern 6: PreWarm Config Access in Generator

**What:** Pass PreWarmConfig to bootstrap generators via BootstrapConfig struct.

**When to use:** When generators need access to NodePool-level configuration.

**Example:**
```go
// Source: Existing BootstrapConfig pattern
// In userdata.go, extend BootstrapConfig struct:

type BootstrapConfig struct {
	ClusterName       string
	ClusterEndpoint   string
	ClusterCA         string
	ClusterCIDR       string
	PoolName          string
	BootstrapTemplate stratosv1alpha1.BootstrapTemplate
	Kubelet           *stratosv1alpha1.KubeletConfig
	TemplateLabels    map[string]string
	TemplateTaints    []corev1.Taint
	EnableNetworkReadinessTaint bool
	CustomUserData    string

	// NEW: PreWarm configuration for image pre-pull
	PreWarmConfig *stratosv1alpha1.PreWarmConfig
}

// In controller code that calls GenerateUserData:
config := &aws.BootstrapConfig{
	// ... existing fields ...
	PreWarmConfig: nodePool.Spec.PreWarm, // Pass through from NodePool spec
}
```

**Why pass full config:** Generator needs both `.Images` slice and `.ImagePullPolicy` enum. Passing full config object is cleaner than individual fields.

### Anti-Patterns to Avoid

- **Always using MIME for AL2023:** Breaks backward compatibility. Use MIME only when images configured.
- **Appending image pull to bootstrap script:** Bootstrap script is complex and AMI-specific. Separate MIME part is cleaner and more maintainable.
- **Ignoring user data size:** EC2 API will reject oversized user data with cryptic errors. Check size and warn proactively.
- **Implementing Bottlerocket image pull:** Bootstrap containers may not persist images for kubelet. Defer to separate phase after research.
- **Using dynamic MIME boundaries:** Deterministic boundaries (fixed string) make testing and debugging easier.
- **Blocking launch on Bottlerocket+images:** Degraded mode (launch without images) is better than blocking. Use status condition to inform user.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| MIME multipart formatting | Custom boundary/header logic | Existing `buildMIMEMultipart()` utility | Already tested, handles edge cases, deterministic boundaries |
| Status condition management | Direct slice manipulation | `meta.SetStatusCondition()` from apimachinery | Handles upserts, timestamp management, comparison logic |
| Image pull script generation | Template strings in generators | `warmup.GenerateScript()` from Phase 18 | Complete solution with retry, ECR auth, pinning, policy handling |
| User data size calculation | String length checks | `len(userData)` with constants | Size is in bytes; EC2 measures raw bytes before base64 |

**Key insight:** Phase 18 already solved the hard problem (script generation with retry, auth, pinning). This phase is just integration—wire existing components together with conditional logic.

## Common Pitfalls

### Pitfall 1: AL2023 MIME Breaking Existing Deployments

**What goes wrong:** Switching AL2023 generator to always use MIME multipart breaks existing NodePools that expect plain NodeConfig YAML. Cluster join may fail if nodeadm doesn't parse MIME correctly.

**Why it happens:** Original AL2023 implementation outputs plain YAML for simplicity. MIME multipart is required only when adding shell scripts (like image pull).

**How to avoid:**
- Use conditional logic: `if hasImages` then MIME multipart, else plain YAML
- Test both paths: with images (MIME) and without (plain YAML)
- Verify nodeadm parses both formats correctly

**Warning signs:** Existing AL2023 nodes fail to join cluster after controller upgrade. New nodes without images configured fail warmup.

**Detection:** Integration test with AL2023 NodePool without images configured. Should produce plain YAML, not MIME.

### Pitfall 2: MIME Part Ordering Breaks Bootstrap

**What goes wrong:** Image pull script runs before bootstrap completes, tries to use containerd before it's ready. Script fails, warmup times out.

**Why it happens:** MIME parts are processed sequentially by cloud-init. If image pull comes before bootstrap, containerd socket doesn't exist yet.

**How to avoid:**
- AL2: Bootstrap → Image Pull → Warmup (in that order)
- AL2023: NodeConfig part first (processed by nodeadm-config.service), then shell scripts run afterward
- Phase 18's containerd readiness check provides defense-in-depth

**Warning signs:** Intermittent warmup failures with "connection refused" errors from ctr command.

**Detection:** Check generated user data order. Bootstrap/NodeConfig must be first MIME part.

### Pitfall 3: User Data Exceeds 16 KiB Limit Silently

**What goes wrong:** User configures 20+ large images. Generated user data exceeds 16,384 bytes. EC2 API rejects launch with vague error. Instances never appear, no clear failure message.

**Why it happens:** EC2 has hard 16 KiB limit (before base64 encoding). Large image lists with long registry URLs consume space quickly. MIME multipart adds headers (~200 bytes per part).

**How to avoid:**
- Calculate size: `len(userData)` in bytes
- Warn at 14 KiB (85% threshold): `if size > 14*1024`
- Error at 16 KiB: `if size > 16*1024 { return error }`
- Log helpful message: "User data size: 15360 bytes (93% of limit). Consider reducing image list."

**Warning signs:** EC2 launch failures with "user data" in error message. kubectl describe shows instance launch failures.

**Detection:** Unit test with large image list (e.g., 30 images with long URLs). Assert size < 16384.

**Calculation example:**
```
NodeConfig YAML: ~500 bytes
Image pull script base: ~800 bytes
Per-image overhead: ~100 bytes (retry loop, logging)
Warmup script: ~400 bytes
MIME headers: ~300 bytes
Custom user data: varies

With 50 images: ~500 + ~800 + (50*100) + ~400 + ~300 = ~7000 bytes (safe)
With 150 images: ~500 + ~800 + (150*100) + ~400 + ~300 = ~17000 bytes (exceeds limit)
```

### Pitfall 4: Bottlerocket Launches Fail Instead of Degrading

**What goes wrong:** User configures images on Bottlerocket NodePool. Code returns error, blocks instance launch entirely. Pool stays at zero nodes.

**Why it happens:** Defensive validation that rejects unsupported configuration. But Bottlerocket can launch successfully—just without image pre-pull.

**How to avoid:**
- Surface warning via status condition (non-blocking)
- Allow instance launch to proceed (degraded mode)
- Log warning but don't return error from generator
- Document that Bottlerocket image pre-pull support comes in future phase

**Warning signs:** NodePool with Bottlerocket + images never launches instances. Error in controller logs about unsupported configuration.

**Detection:** Create NodePool with Bottlerocket NodeClass and `preWarm.images` configured. Should launch instances successfully with status condition warning.

**Correct behavior:**
```
Status: Degraded = True
Reason: ImagePrePullNotSupported
Message: Image pre-pull not supported on Bottlerocket. Instances will launch without cached images.
```

### Pitfall 5: Missing PreWarmConfig Nil Check Causes Panic

**What goes wrong:** Code assumes `config.PreWarmConfig` is non-nil. User doesn't configure `preWarm` section. Generator panics with nil pointer dereference.

**Why it happens:** `PreWarmConfig` is optional in NodePool spec (`+optional` marker). Nil when not specified.

**How to avoid:**
- Always check nil before accessing: `if config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0`
- Phase 18 generator handles empty image list gracefully (returns no-op script)
- Don't call warmup.GenerateScript if PreWarmConfig is nil

**Warning signs:** Controller crashes on NodePool without `preWarm` section. Stack trace shows nil pointer in generator.

**Detection:** Unit test with BootstrapConfig where PreWarmConfig is nil. Should generate user data successfully without image pull script.

### Pitfall 6: Status Condition Overwritten by Other Reconciler Paths

**What goes wrong:** Set `ImagePrePullSupported = False` condition. Next reconciliation loop overwrites it or removes it. User never sees the warning.

**Why it happens:** Multiple code paths update status. Without coordination, conditions get stomped.

**How to avoid:**
- Use `meta.SetStatusCondition()` which upserts (updates or inserts)
- Set condition in main reconciliation path, not error branches
- Use `meta.RemoveStatusCondition()` explicitly when condition no longer applies
- Update status at end of reconciliation, not multiple times

**Warning signs:** Condition appears briefly in `kubectl describe` then disappears. Flapping conditions.

**Detection:** Watch NodePool status over time: `kubectl get nodepool -w -o jsonpath='{.status.conditions}'`

## Code Examples

Verified patterns from codebase analysis:

### Extending BootstrapConfig for PreWarm

```go
// Source: internal/cloudprovider/aws/userdata.go (existing + new field)
type BootstrapConfig struct {
	ClusterName       string
	ClusterEndpoint   string
	ClusterCA         string
	ClusterCIDR       string
	PoolName          string
	BootstrapTemplate stratosv1alpha1.BootstrapTemplate
	Kubelet           *stratosv1alpha1.KubeletConfig
	TemplateLabels    map[string]string
	TemplateTaints    []corev1.Taint
	EnableNetworkReadinessTaint bool
	CustomUserData    string

	// NEW: PreWarm configuration from NodePool.Spec.PreWarm
	PreWarmConfig *stratosv1alpha1.PreWarmConfig
}
```

### AL2023 Conditional MIME Generation

```go
// Source: internal/cloudprovider/aws/al2023.go (modified Generate method)
func (g *AL2023Generator) Generate(config *BootstrapConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	nodeConfig := g.generateNodeadmConfig(config)

	// Check if images are configured
	hasImages := config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0

	if !hasImages {
		// Plain NodeConfig YAML (original behavior, no images)
		return nodeConfig, nil
	}

	// MIME multipart with images
	var parts []string

	// Part 1: NodeConfig as application/node.eks.aws
	parts = append(parts, mimePartNodeConfig(nodeConfig))

	// Part 2: Image pull script
	imagePullScript := warmup.GenerateScript(
		config.PreWarmConfig.Images,
		config.PreWarmConfig.GetImagePullPolicy(),
	)
	parts = append(parts, mimePartShellScript(imagePullScript, "image-pull.sh"))

	// Part 3: Warmup completion script
	warmupScript := getWarmupScript()
	parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))

	return buildMIMEMultipart(parts), nil
}

// mimePartNodeConfig creates MIME part for AL2023 NodeConfig
func mimePartNodeConfig(content string) string {
	return fmt.Sprintf(`Content-Type: application/node.eks.aws; charset="us-ascii"
Content-Disposition: attachment; filename="nodeadm-config.yaml"

%s`, content)
}
```

### AL2 Image Pull Integration

```go
// Source: internal/cloudprovider/aws/al2.go (modified Generate method)
func (g *AL2Generator) Generate(config *BootstrapConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	var parts []string

	// Part 1: Bootstrap script (unchanged)
	bootstrapScript := g.generateBootstrapScript(config)
	parts = append(parts, mimePartShellScript(bootstrapScript, "bootstrap.sh"))

	// Part 2: Image pull script (NEW - conditional)
	if config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0 {
		imagePullScript := warmup.GenerateScript(
			config.PreWarmConfig.Images,
			config.PreWarmConfig.GetImagePullPolicy(),
		)
		parts = append(parts, mimePartShellScript(imagePullScript, "image-pull.sh"))
	}

	// Part 3: Warmup script (unchanged)
	warmupScript := getWarmupScript()
	parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))

	// Part 4: Optional custom userData (unchanged)
	if config.CustomUserData != "" {
		parts = append(parts, mimePartShellScript(config.CustomUserData, "custom-userdata.sh"))
	}

	return buildMIMEMultipart(parts), nil
}
```

### Bottlerocket Status Condition

```go
// Source: Kubernetes status condition patterns + codebase nodepool_status.go
import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// In NodePool reconciler, after fetching NodeClass
func (r *NodePoolReconciler) checkImagePrePullSupport(
	ctx context.Context,
	nodePool *stratosv1alpha1.NodePool,
	nodeClass *stratosv1alpha1.AWSNodeClass,
) {
	hasImages := nodePool.Spec.PreWarm != nil && len(nodePool.Spec.PreWarm.Images) > 0
	isBottlerocket := nodeClass.Spec.BootstrapTemplate == stratosv1alpha1.BootstrapTemplateBottlerocket

	if hasImages && isBottlerocket {
		// Surface warning condition
		condition := metav1.Condition{
			Type:    "ImagePrePullSupported",
			Status:  metav1.ConditionFalse,
			Reason:  "BottlerocketNotSupported",
			Message: "Image pre-pull is not supported on Bottlerocket AMIs. Instances will launch without cached images. Pods may experience cold-start image pull delays.",
			ObservedGeneration: nodePool.Generation,
		}
		meta.SetStatusCondition(&nodePool.Status.Conditions, condition)
	} else {
		// Remove condition if no longer applicable
		meta.RemoveStatusCondition(&nodePool.Status.Conditions, "ImagePrePullSupported")
	}
}
```

### User Data Size Warning

```go
// Source: AWS EC2 limits + controller-runtime logging
import "sigs.k8s.io/controller-runtime/pkg/log"

func (g *AL2023Generator) Generate(config *BootstrapConfig) (string, error) {
	// ... generate user data ...
	userData := buildMIMEMultipart(parts)

	// Check size against EC2 limits
	const warnThreshold = 14 * 1024 // 14 KiB = 85%
	const hardLimit = 16 * 1024     // 16 KiB

	size := len(userData)
	percentUsed := (size * 100) / hardLimit

	if size > hardLimit {
		return "", fmt.Errorf(
			"user data size %d bytes exceeds EC2 limit of %d bytes (%.0f%%); reduce image list or disable image pre-pull",
			size, hardLimit, float64(percentUsed),
		)
	}

	if size > warnThreshold {
		// Note: This requires passing context from controller
		// Alternative: Return size info in error/result struct for controller to log
		log.FromContextOrDiscard(ctx).Info(
			"User data size approaching EC2 limit",
			"size", size,
			"limit", hardLimit,
			"percentUsed", percentUsed,
			"poolName", config.PoolName,
			"recommendation", "Consider reducing image list or splitting across pools",
		)
	}

	return userData, nil
}
```

### Shared MIME Utilities (Extract to mime.go)

```go
// Source: Existing al2023.go functions, extracted for reuse
// internal/cloudprovider/aws/mime.go (NEW FILE)
package aws

import (
	"fmt"
	"strings"
)

// mimePartShellScript creates a MIME part for a shell script
func mimePartShellScript(content, filename string) string {
	return fmt.Sprintf(`Content-Type: text/x-shellscript; charset="us-ascii"
Content-Disposition: attachment; filename="%s"

%s`, filename, content)
}

// mimePartNodeConfig creates a MIME part for AL2023 NodeConfig
func mimePartNodeConfig(content string) string {
	return fmt.Sprintf(`Content-Type: application/node.eks.aws; charset="us-ascii"
Content-Disposition: attachment; filename="nodeadm-config.yaml"

%s`, content)
}

// buildMIMEMultipart assembles parts into a MIME multipart message
// Uses fixed boundary for deterministic output and easier testing
func buildMIMEMultipart(parts []string) string {
	boundary := "==STRATOS_MIME_BOUNDARY=="
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"%s\"\n\n", boundary))

	for _, part := range parts {
		sb.WriteString(fmt.Sprintf("--%s\n%s\n", boundary, part))
	}

	sb.WriteString(fmt.Sprintf("--%s--\n", boundary))

	return sb.String()
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| AL2023 plain YAML always | Conditional MIME multipart when images present | Phase 19 (this phase) | Enables shell script injection while maintaining backward compat |
| Image pull in bootstrap script | Separate MIME part | Phase 19 | Cleaner separation, reusable script from Phase 18 |
| Log-only validation warnings | Kubernetes status conditions | Industry standard (2020+) | Machine-readable, discoverable via kubectl, persists in cluster |
| Manual string concatenation | Utility functions for MIME parts | Existing codebase | Consistent formatting, easier testing |
| Dynamic MIME boundaries | Fixed boundary string | Existing codebase (al2.go) | Deterministic output, simpler tests |

**Deprecated/outdated:**
- None - this is new functionality building on existing patterns

**Current standards:**
- MIME multipart for multi-format user data (AWS + cloud-init standard since 2010+)
- `application/node.eks.aws` for AL2023 NodeConfig (AWS EKS convention, 2023+)
- `metav1.Condition` for status (Kubernetes API conventions, 2019+)
- Fixed 16 KiB user data limit (AWS EC2 constant since service launch)

## Open Questions

1. **AL2023 MIME ordering with nodeadm**
   - What we know: nodeadm-config.service runs first, processes NodeConfig YAML. Shell scripts in MIME run after via cloud-init.
   - What's unclear: Exact timing of shell script execution relative to nodeadm cluster join. Does image pull block join or run in parallel?
   - Recommendation: Put NodeConfig as first MIME part. Image pull script has built-in containerd readiness check (from Phase 18). Test on actual AL2023 instance to verify timing. If blocking becomes issue, consider systemd service with `After=nodeadm-run.service`.

2. **User data size in practice**
   - What we know: Limit is 16,384 bytes raw. MIME adds ~300 bytes overhead. Image pull script ~800 bytes base + ~100 bytes per image.
   - What's unclear: At what image count does limit become problem? Do real-world image URLs vary significantly in length?
   - Recommendation: Set warning at 14 KiB (85%). Include formula in warning message: "N images consume ~X bytes. Consider splitting images across pools or using shorter registry URLs." Test with 50, 100, 150 images to find practical limit.

3. **Status condition visibility**
   - What we know: `kubectl describe nodepool` shows conditions. Controller logs can reference them.
   - What's unclear: Do users regularly check status conditions? Should we also log warning at launch time?
   - Recommendation: Set condition AND log warning when Bottlerocket+images detected. Condition is authoritative source, log is discoverable. Consider metrics (future phase): `stratos_nodepool_warnings_total{type="ImagePrePullNotSupported"}`.

4. **Bottlerocket bootstrap containers**
   - What we know: Bottlerocket has bootstrap containers feature for init-time workloads. Container images can be pre-configured in TOML.
   - What's unclear: Do bootstrap container images persist for kubelet, or are they GC'd after bootstrap? Can they be pinned?
   - Recommendation: Defer Bottlerocket image pre-pull to separate phase (v1.3 or later). Research bootstrap containers: test if images pulled there are visible to kubelet with `crictl images`. This phase only surfaces "not supported" warning.

## Sources

### Primary (HIGH confidence)
- Codebase: `/internal/cloudprovider/aws/userdata.go`, `al2.go`, `al2023.go`, `bottlerocket.go` - Existing generator patterns
- Codebase: `/internal/warmup/generator.go` - Phase 18 output, GenerateScript API
- Codebase: `/api/v1alpha1/nodepool_types.go`, `config_types.go` - NodePool spec structure, PreWarmConfig fields
- Codebase: `/internal/controller/nodepool/nodepool_status.go` - Existing condition management with `meta.SetStatusCondition`
- [AWS EC2 User Data Documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html) - 16 KiB limit, base64 encoding
- [Cloud-init User Data Formats](https://cloudinit.readthedocs.io/en/latest/explanation/format.html) - MIME multipart standard, content types
- [AWS EKS AL2023 Custom User Data](https://repost.aws/knowledge-center/custom-user-eks-2023) - MIME format with application/node.eks.aws

### Secondary (MEDIUM confidence)
- [AWS CDK MultipartUserData](https://docs.aws.amazon.com/cdk/api/v2/python/aws_cdk.aws_ec2/MultipartUserData.html) - MIME patterns
- [Karpenter NodePool Status](https://karpenter.sh/docs/concepts/nodepools/) - Condition patterns for NodePool CRDs
- [Kubernetes Status Conditions Best Practices](https://superorbital.io/blog/status-and-conditions/) - Condition semantics

### Tertiary (LOW confidence)
- GitHub issues on EC2 user data size limits - Confirmed 16 KiB is hard limit, workarounds involve S3

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All components exist in codebase or stdlib, patterns verified
- Architecture: HIGH - CONTEXT.md decisions locked, codebase patterns established, AWS/K8s standards clear
- Pitfalls: HIGH - Size limits documented by AWS, nil checks standard Go practice, MIME ordering testable

**Research date:** 2026-02-04
**Valid until:** 90 days (stable domain: AWS EC2 user data limits unchanged for years, K8s status conditions stable API)
