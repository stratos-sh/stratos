# Phase 17: CRD Types and Code Generation - Research

**Researched:** 2026-02-04
**Domain:** Kubernetes CRD type definition with kubebuilder markers, controller-gen code generation
**Confidence:** HIGH

## Summary

Phase 17 adds two new fields (`Images` and `ImagePullPolicy`) plus a new string type (`ImagePullPolicy`) to the existing `PreWarmConfig` struct in `api/v1alpha1/config_types.go`. The implementation follows established patterns already present in the codebase (see `TimeoutAction` enum, `BootstrapTemplate` enum, nil-safe getters on `ScaleDownConfig` and `PreWarmConfig`).

The key technical decision is how to validate individual items in the `Images []string` slice. Controller-gen v0.16.5 (installed in this project) supports the `+kubebuilder:validation:items:MinLength=1` marker to apply `minLength` validation to each string element in a slice. This is the simplest approach and avoids needing a named type wrapper. The codebase already uses `+kubebuilder:validation:MinItems=1` on `SubnetIDs []string` for array-level validation, so this is a natural extension.

**Primary recommendation:** Add fields and type directly to `config_types.go` following the exact patterns of `TimeoutAction` (for `ImagePullPolicy` enum type) and `GetTimeoutAction()` (for nil-safe getters). Use `+kubebuilder:validation:items:MinLength=1` for per-item validation. Run `make generate && make manifests` to regenerate. Add tests in `config_types_test.go` following existing table-driven test patterns.

## Standard Stack

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| controller-gen | v0.16.5 | CRD manifest and deepcopy generation | Already installed at `bin/controller-gen`, pinned in Makefile |
| kubebuilder markers | N/A | Declarative CRD validation via Go comments | Standard approach for all CRD types in this project |

### Supporting
| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| `make generate` | N/A | Runs `controller-gen object` for deepcopy methods | After any type change in `api/v1alpha1/` |
| `make manifests` | N/A | Runs `controller-gen crd` for CRD YAML | After any marker or type change in `api/v1alpha1/` |
| `go test` | Go 1.25.5 | Unit tests | For nil-safe getter tests |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `+kubebuilder:validation:items:MinLength=1` on `[]string` | Named type `type ImageRef string` with `+kubebuilder:validation:MinLength=1` | Named type is more explicit but adds unnecessary indirection; the `items:` prefix is supported in v0.16.5 and simpler |
| CRD schema validation for empty strings | CEL validation rules (`+kubebuilder:validation:XValidation`) | CEL is more powerful but overkill for simple MinLength; schema validation is the standard approach |

## Architecture Patterns

### File Structure (no new files needed)
```
api/v1alpha1/
├── config_types.go           # MODIFY: Add ImagePullPolicy type, Images/ImagePullPolicy fields, GetImages()/GetImagePullPolicy() methods
├── config_types_test.go      # MODIFY: Add tests for GetImages() and GetImagePullPolicy()
├── zz_generated.deepcopy.go  # AUTO-GENERATED: Updated by make generate
├── nodepool_types.go         # NO CHANGE (PreWarm *PreWarmConfig already exists on NodePoolSpec)
└── groupversion_info.go      # NO CHANGE
deploy/charts/stratos/crds/
└── stratos.sh_nodepools.yaml # AUTO-GENERATED: Updated by make manifests
```

### Pattern 1: Enum String Type with Kubebuilder Validation
**What:** Define a named string type with `+kubebuilder:validation:Enum` marker and constants
**When to use:** For constrained string fields with known valid values
**Existing example in codebase (`config_types.go` lines 57-66):**
```go
// TimeoutAction defines what happens when pre-warming times out
// +kubebuilder:validation:Enum=stop;terminate
type TimeoutAction string

const (
	TimeoutActionStop      TimeoutAction = "stop"
	TimeoutActionTerminate TimeoutAction = "terminate"
)
```

**Apply to ImagePullPolicy:**
```go
// ImagePullPolicy defines how image pull failures affect warmup completion
// +kubebuilder:validation:Enum=Required;BestEffort
type ImagePullPolicy string

const (
	ImagePullPolicyRequired   ImagePullPolicy = "Required"
	ImagePullPolicyBestEffort ImagePullPolicy = "BestEffort"
)
```

### Pattern 2: Nil-Safe Getter Methods on Pointer Receiver
**What:** Methods on `*StructType` that handle nil receiver and nil fields, returning safe defaults
**When to use:** For all optional config struct fields accessed by controller logic
**Existing example in codebase (`config_types.go` lines 100-106):**
```go
func (c *PreWarmConfig) GetTimeoutAction() TimeoutAction {
	if c == nil || c.TimeoutAction == nil {
		return TimeoutActionStop
	}
	return *c.TimeoutAction
}
```

**Apply to new getters:**
```go
func (c *PreWarmConfig) GetImages() []string {
	if c == nil || c.Images == nil {
		return []string{}
	}
	return c.Images
}

func (c *PreWarmConfig) GetImagePullPolicy() ImagePullPolicy {
	if c == nil || c.ImagePullPolicy == nil {
		return ImagePullPolicyRequired
	}
	return *c.ImagePullPolicy
}
```

### Pattern 3: Kubebuilder Validation Markers on Slice Fields
**What:** Using `+kubebuilder:validation:items:` prefix to validate individual elements in a slice
**When to use:** When each element in a list must satisfy a constraint
**Controller-gen v0.16.5 support:** The `items:` prefix is supported. It generates `items: { minLength: 1 }` in the CRD OpenAPI schema.
```go
// Images is a list of container images to pre-pull during warmup.
// +optional
// +kubebuilder:validation:items:MinLength=1
Images []string `json:"images,omitempty"`
```

### Pattern 4: Table-Driven Tests for Getters
**What:** Standard Go table-driven tests covering nil receiver, empty struct, and explicit values
**When to use:** For all nil-safe getter methods
**Existing example in codebase (`config_types_test.go` lines 140-183):**
```go
func TestPreWarmConfig_GetTimeoutAction(t *testing.T) {
	stop := TimeoutActionStop
	terminate := TimeoutActionTerminate

	tests := []struct {
		name   string
		config *PreWarmConfig
		want   TimeoutAction
	}{
		{name: "nil config defaults to stop", config: nil, want: TimeoutActionStop},
		{name: "empty config defaults to stop", config: &PreWarmConfig{}, want: TimeoutActionStop},
		{name: "explicit stop returns stop", config: &PreWarmConfig{TimeoutAction: &stop}, want: TimeoutActionStop},
		{name: "explicit terminate returns terminate", config: &PreWarmConfig{TimeoutAction: &terminate}, want: TimeoutActionTerminate},
	}
	// ...
}
```

### Anti-Patterns to Avoid
- **Editing `zz_generated.deepcopy.go` manually:** Always use `make generate`. Manual edits will be overwritten.
- **Adding new files for a two-field addition:** The fields belong on `PreWarmConfig` in `config_types.go`. No new files needed.
- **Using `+kubebuilder:default=Required` on the `ImagePullPolicy` field:** The nil-safe getter handles the default. Using both kubebuilder default and getter default creates two sources of truth. Follow the existing pattern where `TimeoutAction` has `+kubebuilder:default=stop` -- this is acceptable since the existing codebase uses it. However, the milestone research says "Required is the default when nil/omitted" which the getter handles. Use `+kubebuilder:default=Required` to match the `TimeoutAction` pattern for consistency.
- **Putting `MinLength` on the slice field without `items:` prefix:** Without `items:`, the validation would be applied to the array itself (which is a string for arrays, not meaningful). Must use `+kubebuilder:validation:items:MinLength=1`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Deepcopy methods for new fields | Manual DeepCopyInto code | `make generate` (controller-gen) | Controller-gen handles `[]string` (copies slice) and `*ImagePullPolicy` (copies pointer) correctly. The current `PreWarmConfig.DeepCopyInto` will be automatically extended. |
| CRD YAML schema for new fields | Manual YAML editing | `make manifests` (controller-gen) | Controller-gen reads markers and generates the correct OpenAPI v3 schema including enum constraints and items validation. |
| Validation of empty strings in list | Custom webhook validation | `+kubebuilder:validation:items:MinLength=1` marker | Schema-level validation is simpler, declarative, and runs before any webhook. |
| Default value handling | Mutating webhook | Nil-safe getter + `+kubebuilder:default` | The existing codebase uses this pattern for all config defaults (ScaleDown, PreWarm). |

**Key insight:** This phase is purely declarative -- Go type definitions, kubebuilder markers, and generated code. No runtime logic changes.

## Common Pitfalls

### Pitfall 1: Forgetting to Run Both Generate and Manifests
**What goes wrong:** Running only `make generate` (deepcopy) but not `make manifests` (CRD YAML), or vice versa. The CRD YAML in `deploy/charts/stratos/crds/` won't match the Go types.
**Why it happens:** They are separate Makefile targets that do different things.
**How to avoid:** Always run both: `make generate && make manifests`. Verify `stratos.sh_nodepools.yaml` contains the new fields.
**Warning signs:** `make generate` succeeds but `kubectl apply -f deploy/charts/stratos/crds/` doesn't show new fields.

### Pitfall 2: Naming Discrepancy Between Requirements and Code
**What goes wrong:** The requirements docs use `spec.warmup.images` and `spec.warmup.imagePullPolicy` as shorthand, but the actual Go struct uses `spec.preWarm` (JSON tag is `preWarm`). Implementing a `warmup` field instead of adding to existing `preWarm` would create a separate field.
**Why it happens:** Requirements use informal naming.
**How to avoid:** The new fields go on `PreWarmConfig` which is already referenced as `spec.preWarm` in the CRD. The JSON paths are `spec.preWarm.images` and `spec.preWarm.imagePullPolicy`.
**Warning signs:** A field called `warmup` appearing in the CRD YAML instead of fields under `preWarm`.

### Pitfall 3: Wrong Enum Case for ImagePullPolicy Values
**What goes wrong:** Using lowercase `required`/`bestEffort` instead of PascalCase `Required`/`BestEffort`. The warmup script generator (Phase 18) will compare against these values.
**Why it happens:** The existing `TimeoutAction` uses lowercase (`stop`, `terminate`). The research summary specifies PascalCase for `ImagePullPolicy`.
**How to avoid:** Use `Required` and `BestEffort` as the enum values. This is a locked decision from STATE.md.
**Warning signs:** CRD YAML shows lowercase enum values.

### Pitfall 4: Slice Deepcopy Not Regenerated
**What goes wrong:** After adding `Images []string` to `PreWarmConfig`, forgetting `make generate` means the old `DeepCopyInto` doesn't copy the `Images` slice. Mutations to one object's images would affect the copy.
**Why it happens:** `zz_generated.deepcopy.go` is only updated by running `make generate`.
**How to avoid:** Always run `make generate` after type changes. Verify the generated file contains `if in.Images != nil` block.
**Warning signs:** Tests pass but images list is shared between copies.

### Pitfall 5: Not Testing Both nil Receiver AND nil Field
**What goes wrong:** Testing `GetImages()` on a nil `*PreWarmConfig` but not on a non-nil `PreWarmConfig` where `Images` field is nil. Or vice versa.
**Why it happens:** These are different code paths (`c == nil` vs `c.Images == nil`).
**How to avoid:** Test three cases: nil receiver, empty struct (fields are nil/zero), and explicit values. This matches the existing test pattern.
**Warning signs:** Panic on nil dereference in production when PreWarm is specified but Images is omitted.

## Code Examples

### Complete PreWarmConfig After Changes
```go
// Source: Based on existing config_types.go pattern, verified against codebase

// ImagePullPolicy defines how image pull failures affect warmup completion
// +kubebuilder:validation:Enum=Required;BestEffort
type ImagePullPolicy string

const (
	// ImagePullPolicyRequired fails warmup if any image pull fails
	ImagePullPolicyRequired ImagePullPolicy = "Required"

	// ImagePullPolicyBestEffort completes warmup regardless of pull failures
	ImagePullPolicyBestEffort ImagePullPolicy = "BestEffort"
)

// PreWarmConfig configures the pre-warming lifecycle
type PreWarmConfig struct {
	// Timeout is how long to wait for warmup to complete (for node to become Ready).
	// Default: 10 minutes
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// TimeoutAction is what to do if warmup doesn't complete in time.
	// +kubebuilder:validation:Enum=stop;terminate
	// +kubebuilder:default=stop
	// +optional
	TimeoutAction *TimeoutAction `json:"timeoutAction,omitempty"`

	// Images is a list of container images to pre-pull during warmup.
	// Each image is pulled using ctr and pinned to prevent kubelet GC eviction.
	// +optional
	// +kubebuilder:validation:items:MinLength=1
	Images []string `json:"images,omitempty"`

	// ImagePullPolicy controls whether image pull failures block warmup completion.
	// Required (default): warmup fails if any image cannot be pulled.
	// BestEffort: warmup completes regardless of pull failures.
	// +kubebuilder:validation:Enum=Required;BestEffort
	// +kubebuilder:default=Required
	// +optional
	ImagePullPolicy *ImagePullPolicy `json:"imagePullPolicy,omitempty"`
}
```

### Nil-Safe Getters
```go
// Source: Follows existing GetTimeoutAction() pattern in config_types.go

// GetImages returns the list of images to pre-pull (default: empty list)
func (c *PreWarmConfig) GetImages() []string {
	if c == nil || c.Images == nil {
		return []string{}
	}
	return c.Images
}

// GetImagePullPolicy returns the image pull policy (default: Required)
func (c *PreWarmConfig) GetImagePullPolicy() ImagePullPolicy {
	if c == nil || c.ImagePullPolicy == nil {
		return ImagePullPolicyRequired
	}
	return *c.ImagePullPolicy
}
```

### Test Pattern for GetImages
```go
// Source: Follows existing TestPreWarmConfig_GetTimeoutAction pattern in config_types_test.go

func TestPreWarmConfig_GetImages(t *testing.T) {
	tests := []struct {
		name   string
		config *PreWarmConfig
		want   []string
	}{
		{
			name:   "nil config returns empty slice",
			config: nil,
			want:   []string{},
		},
		{
			name:   "empty config returns empty slice",
			config: &PreWarmConfig{},
			want:   []string{},
		},
		{
			name: "explicit images returns those images",
			config: &PreWarmConfig{
				Images: []string{"nginx:1.25", "redis:7"},
			},
			want: []string{"nginx:1.25", "redis:7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetImages()
			if len(got) != len(tt.want) {
				t.Errorf("GetImages() returned %d items, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("GetImages()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}
```

### Test Pattern for GetImagePullPolicy
```go
func TestPreWarmConfig_GetImagePullPolicy(t *testing.T) {
	required := ImagePullPolicyRequired
	bestEffort := ImagePullPolicyBestEffort

	tests := []struct {
		name   string
		config *PreWarmConfig
		want   ImagePullPolicy
	}{
		{
			name:   "nil config defaults to Required",
			config: nil,
			want:   ImagePullPolicyRequired,
		},
		{
			name:   "empty config defaults to Required",
			config: &PreWarmConfig{},
			want:   ImagePullPolicyRequired,
		},
		{
			name:   "explicit Required returns Required",
			config: &PreWarmConfig{ImagePullPolicy: &required},
			want:   ImagePullPolicyRequired,
		},
		{
			name:   "explicit BestEffort returns BestEffort",
			config: &PreWarmConfig{ImagePullPolicy: &bestEffort},
			want:   ImagePullPolicyBestEffort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetImagePullPolicy()
			if got != tt.want {
				t.Errorf("GetImagePullPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

### Expected Generated Deepcopy (for reference only -- do not edit manually)
```go
// After make generate, PreWarmConfig.DeepCopyInto will include:
func (in *PreWarmConfig) DeepCopyInto(out *PreWarmConfig) {
	*out = *in
	if in.Timeout != nil {
		in, out := &in.Timeout, &out.Timeout
		*out = new(metav1.Duration)
		**out = **in
	}
	if in.TimeoutAction != nil {
		in, out := &in.TimeoutAction, &out.TimeoutAction
		*out = new(TimeoutAction)
		**out = **in
	}
	if in.Images != nil {
		in, out := &in.Images, &out.Images
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ImagePullPolicy != nil {
		in, out := &in.ImagePullPolicy, &out.ImagePullPolicy
		*out = new(ImagePullPolicy)
		**out = **in
	}
}
```

### Expected CRD YAML Fragment (for verification)
```yaml
# After make manifests, preWarm section in stratos.sh_nodepools.yaml will include:
preWarm:
  description: PreWarm configures the pre-warming lifecycle
  properties:
    timeout:
      description: ...
      type: string
    timeoutAction:
      ...
    images:
      description: |-
        Images is a list of container images to pre-pull during warmup.
        Each image is pulled using ctr and pinned to prevent kubelet GC eviction.
      items:
        minLength: 1
        type: string
      type: array
    imagePullPolicy:
      default: Required
      description: |-
        ImagePullPolicy controls whether image pull failures block warmup completion.
        ...
      enum:
      - Required
      - BestEffort
      type: string
  type: object
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Named type wrapper for slice item validation | `+kubebuilder:validation:items:` prefix on slice fields | controller-tools ~v0.15+ | Simpler, no extra type needed |
| Manual deepcopy | `make generate` with controller-gen | Always standard | Never hand-edit `zz_generated.deepcopy.go` |

**Deprecated/outdated:**
- Editing CRD YAML manually: Always use `make manifests` to generate from markers
- Using `+kubebuilder:validation:MinLength` directly on a `[]string` field: This applies to the array, not items. Use `items:MinLength` prefix.

## Open Questions

1. **`items:MinLength` support confirmation in v0.16.5**
   - What we know: The kubebuilder book documents `items:` prefix markers. Controller-gen v0.16.5 is installed and working. The release notes for v0.16.5 mention "fixed item validation for unhashable markers."
   - What's unclear: Whether `items:MinLength` specifically generates correct output in v0.16.5 has not been tested in this codebase.
   - Recommendation: Run `make manifests` immediately after adding the marker. Verify the generated YAML has `items: { minLength: 1 }`. If it does not work, fall back to defining `type ImageRef string` with `+kubebuilder:validation:MinLength=1` and using `[]ImageRef` instead of `[]string`. This is a LOW risk issue -- if it fails, the fallback is straightforward.

## Sources

### Primary (HIGH confidence)
- **Codebase files read directly:**
  - `/home/roeeh/projects/presto/api/v1alpha1/config_types.go` -- existing PreWarmConfig, TimeoutAction type, nil-safe getters
  - `/home/roeeh/projects/presto/api/v1alpha1/config_types_test.go` -- existing test patterns for getters
  - `/home/roeeh/projects/presto/api/v1alpha1/nodepool_types.go` -- NodePoolSpec.PreWarm field (JSON: preWarm)
  - `/home/roeeh/projects/presto/api/v1alpha1/aws_nodeclass_types.go` -- BootstrapTemplate enum pattern, SubnetIDs MinItems pattern
  - `/home/roeeh/projects/presto/api/v1alpha1/zz_generated.deepcopy.go` -- current deepcopy for PreWarmConfig
  - `/home/roeeh/projects/presto/Makefile` -- controller-gen v0.16.5, generate and manifests targets
  - `/home/roeeh/projects/presto/deploy/charts/stratos/crds/stratos.sh_nodepools.yaml` -- current CRD YAML
  - `/home/roeeh/projects/presto/.planning/research/SUMMARY.md` -- milestone research summary
  - `/home/roeeh/projects/presto/.planning/REQUIREMENTS.md` -- CRD-01, CRD-02, CRD-03, GEN-01, GEN-02, TEST-04
  - `/home/roeeh/projects/presto/.planning/STATE.md` -- locked decisions (Required default, PascalCase values)
- **Kubebuilder book (Context7: /kubernetes-sigs/kubebuilder)** -- CRD validation markers, enum type pattern, items prefix documentation
- **Kubebuilder book (WebFetch: https://book.kubebuilder.io/reference/markers/crd-validation)** -- confirmed `items:MinLength`, `items:Pattern`, `items:Enum` prefixes exist

### Secondary (MEDIUM confidence)
- **Controller-tools GitHub releases (WebFetch: https://github.com/kubernetes-sigs/controller-tools/releases/tag/v0.16.5)** -- v0.16.5 release notes mention "fixed item validation for unhashable markers"
- **Controller-tools GitHub issues** -- #342 (item validation support), #953 (format on array items)

### Tertiary (LOW confidence)
- None. All findings verified with primary sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- using exact same tools (controller-gen v0.16.5) and patterns (kubebuilder markers) already in the codebase
- Architecture: HIGH -- all code changes verified by reading every file that will be modified; patterns copied from existing codebase
- Pitfalls: HIGH -- all pitfalls identified from direct codebase analysis and verified kubebuilder documentation

**Research date:** 2026-02-04
**Valid until:** 2026-03-04 (stable domain, no moving parts)
