# Phase 10: CRD Simplification - Research

**Researched:** 2026-02-03
**Domain:** Kubernetes CRD types, kubebuilder code generation, controller-gen
**Confidence:** HIGH

## Summary

Phase 10 removes the strategy-related fields from the NodePool CRD and all consuming code. This is a pure deletion phase -- no new types or fields are introduced. The scope is precisely bounded: delete one file (`strategy_types.go`), remove two struct fields from `NodePoolSpec`, remove referencing code in 4 controller files plus 2 cloudprovider files, then regenerate deepcopy and CRD manifests.

An important finding: the CRD YAML at `deploy/charts/stratos/crds/stratos.sh_nodepools.yaml` already does NOT contain `scalingStrategy` or `githubActions` fields. This was verified by grep -- zero matches. The CRD manifest and Go types are currently out of sync (Go types have the fields, CRD YAML does not). This means `make manifests` will simply maintain the status quo. The regeneration step is still required for correctness, but it will not produce visible diffs in the CRD YAML.

The `SkipKubernetesBootstrap` field in `TemplateConfig` and its usage in both `node_launch.go` and `aws/provider.go` are directly tied to `ScalingStrategy` -- they exist solely to support non-Kubernetes strategies. These must be removed as part of this phase since they reference the deleted `ScalingStrategy` field and would cause compile errors.

**Primary recommendation:** Delete `strategy_types.go`, remove two fields from `NodePoolSpec`, remove 6 consuming code sites (validation, launch path, cloudprovider config, comments), then run `make generate && make manifests` and verify with `go build ./...`.

## Standard Stack

This phase uses only existing tools already in the project:

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| controller-gen | v0.16.5 | Regenerate deepcopy and CRD manifests | Already installed at `bin/controller-gen`, used by `make generate` and `make manifests` |

### Commands
| Command | Purpose |
|---------|---------|
| `make generate` | Runs `controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."` -- regenerates `zz_generated.deepcopy.go` |
| `make manifests` | Runs `controller-gen crd paths="./..." output:crd:artifacts:config=deploy/charts/stratos/crds` -- regenerates CRD YAML |
| `go build ./...` | Verifies entire project compiles after changes |

No new dependencies or tools are needed.

## Architecture Patterns

### Deletion Inventory

Complete list of all source code locations referencing strategy types (excluding `.planning/` docs):

**File: `api/v1alpha1/strategy_types.go` (DELETE ENTIRE FILE)**
- `ScalingStrategyType` type (line 25)
- `ScalingStrategyKubernetes` constant (line 31)
- `ScalingStrategyGitHubActions` constant (line 36)
- `GitHubActionsConfig` struct (lines 40-67)
- `SecretReference` struct (lines 70-78)
- `GetIdleTimeout()` method (lines 81-86)

**File: `api/v1alpha1/nodepool_types.go` (REMOVE 2 FIELDS)**
- Lines 60-66: `ScalingStrategy ScalingStrategyType` field with kubebuilder markers
- Lines 68-71: `GitHubActions *GitHubActionsConfig` field with kubebuilder markers

**File: `internal/controller/nodepool/nodepool_validation.go` (REMOVE VALIDATION BLOCK)**
- Lines 49-64: Entire `if nodePool.Spec.ScalingStrategy == stratosv1alpha1.ScalingStrategyGitHubActions` block
- References: `ScalingStrategyGitHubActions`, `nodePool.Spec.GitHubActions`, `SecretRef`

**File: `internal/controller/nodepool/lifecycle/node_launch.go` (SIMPLIFY LINE)**
- Line 45: `SkipKubernetesBootstrap: pool.Spec.ScalingStrategy != "" && pool.Spec.ScalingStrategy != stratosv1alpha1.ScalingStrategyKubernetes`
- After removal: set `SkipKubernetesBootstrap: false` or remove the field entirely

**File: `internal/cloudprovider/interface.go` (REMOVE FIELD)**
- Line 31: `SkipKubernetesBootstrap bool` field in `TemplateConfig` struct

**File: `internal/cloudprovider/aws/provider.go` (REMOVE BRANCH)**
- Lines 213-220: `if templateConfig != nil && templateConfig.SkipKubernetesBootstrap { ... }` branch in `setUserData()`

**File: `internal/controller/nodepool/lifecycle/manager.go` (UPDATE COMMENT)**
- Line 41: `// Both KubernetesStrategy and GitHubActionsStrategy implement this interface.`
- Change to: `// The scaling.Strategy implements this interface.` or similar

**File: `internal/controller/nodepool/doc.go` (UPDATE COMMENT)**
- Line 27: `// monitoring, pool maintenance, demand checking (via ScalingStrategy),`
- Change to: `// monitoring, pool maintenance, demand checking (via scaling.Strategy),`

**Auto-regenerated (DO NOT EDIT MANUALLY):**
- `api/v1alpha1/zz_generated.deepcopy.go` -- Lines 220-243 (`GitHubActionsConfig` deepcopy) and lines 409-413 (NodePoolSpec `GitHubActions` field deepcopy) and lines 616-628 (`SecretReference` deepcopy) will be removed by `make generate`
- `deploy/charts/stratos/crds/stratos.sh_nodepools.yaml` -- Already does not contain strategy fields; `make manifests` maintains status quo

### Critical Ordering

The correct execution order is:

1. Remove fields from `nodepool_types.go` (ScalingStrategy, GitHubActions)
2. Delete `strategy_types.go` (entire file)
3. Run `make generate` IMMEDIATELY (regenerate deepcopy to remove stale GitHubActionsConfig/SecretReference methods)
4. Fix consuming code (validation, launch, cloudprovider) -- these will fail to compile until fixed
5. Run `make manifests` (regenerate CRD YAML)
6. Run `go build ./...` to verify full compilation
7. Update comments (doc.go, manager.go)

Steps 1-3 must happen together because:
- Deleting strategy_types.go without removing fields from nodepool_types.go causes compile error (field type not found)
- Removing fields without regenerating deepcopy causes compile error (deepcopy references deleted types)
- Running `make generate` before deleting fields would regenerate with old fields still present

### Anti-Patterns to Avoid
- **Editing `zz_generated.deepcopy.go` manually:** Never do this. It has a build tag `!ignore_autogenerated` and a "DO NOT EDIT" header. Always use `make generate`.
- **Deprecating instead of removing:** The project decision is to remove. There are no external users setting `scalingStrategy: GitHubActions`. A deprecation period adds complexity with no benefit.
- **Leaving `SkipKubernetesBootstrap` as dead code:** The field is only ever set to true when strategy is not Kubernetes. With only Kubernetes, it is always false. Remove it entirely rather than hardcoding false.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Deepcopy regeneration | Manual edits to `zz_generated.deepcopy.go` | `make generate` | controller-gen handles all edge cases (slices, maps, pointers) |
| CRD schema regeneration | Manual edits to CRD YAML | `make manifests` | controller-gen generates OpenAPIv3 schema from kubebuilder markers |

## Common Pitfalls

### Pitfall 1: Stale Deepcopy After Type Deletion
**What goes wrong:** Deleting `strategy_types.go` without running `make generate` leaves `zz_generated.deepcopy.go` with `GitHubActionsConfig.DeepCopy()` and `SecretReference.DeepCopy()` methods that reference deleted types. Build fails.
**Why it happens:** `zz_generated.deepcopy.go` is a generated file that references types defined in the package. When types are deleted, the deepcopy code becomes orphaned.
**How to avoid:** Run `make generate` immediately after deleting `strategy_types.go` and removing fields from `nodepool_types.go`.
**Warning signs:** Compile errors like `undefined: GitHubActionsConfig`, `undefined: SecretReference` in `zz_generated.deepcopy.go`.

### Pitfall 2: Missing SkipKubernetesBootstrap Removal
**What goes wrong:** Removing `ScalingStrategy` from NodePoolSpec but leaving the `SkipKubernetesBootstrap` reference in `node_launch.go:45` causes compile error: `pool.Spec.ScalingStrategy` no longer exists.
**Why it happens:** `SkipKubernetesBootstrap` is in the lifecycle package, not the controller or API package. Easy to miss because it is not in the same file as the type changes.
**How to avoid:** Grep for ALL references to `ScalingStrategy` in the `internal/` tree. The grep result shows exactly 4 source files (excluding test files and planning docs).
**Warning signs:** Compile error in `lifecycle/node_launch.go` referencing `pool.Spec.ScalingStrategy`.

### Pitfall 3: Forgetting cloudprovider TemplateConfig
**What goes wrong:** Removing `SkipKubernetesBootstrap` from `node_launch.go` but leaving the field in `TemplateConfig` and `aws/provider.go` creates dead code. The field would always be false (since node_launch.go no longer sets it), but the branch in `setUserData()` would remain.
**Why it happens:** The change spans 3 files across 2 packages: `lifecycle/node_launch.go`, `cloudprovider/interface.go`, `cloudprovider/aws/provider.go`.
**How to avoid:** Remove the field from `TemplateConfig` struct (causes compile errors at all usage sites), then fix all compile errors.
**Warning signs:** `SkipKubernetesBootstrap` field unused but present in struct.

### Pitfall 4: Comment Staleness
**What goes wrong:** Code references to strategy types are removed, but comments in `doc.go` and `manager.go` still mention `ScalingStrategy` and `GitHubActionsStrategy`. Not a compile error but a misleading codebase.
**Why it happens:** Comments are not checked by the compiler.
**How to avoid:** Include comment updates as explicit tasks. Grep for `ScalingStrategy` and `GitHubActions` in all `.go` files after changes to catch stale comments.
**Warning signs:** `grep -r 'ScalingStrategy\|GitHubActions' internal/ api/` returns results in comments.

### Pitfall 5: Test Variable Names That Look Like References
**What goes wrong:** `internal/scaling/maintenance_test.go` uses a local variable named `scalingStrategy` (lines 52, 109). This is a variable name, NOT a reference to the CRD field `ScalingStrategy`. Do not change these -- they are legitimate variable names for a `*Strategy` instance.
**Why it happens:** The variable was renamed from `strat` to `scalingStrategy` in Phase 5 (linter enforcement) to avoid misspell false positives.
**How to avoid:** When grepping for stale references, distinguish between Go type/field references (`stratosv1alpha1.ScalingStrategy`, `nodePool.Spec.ScalingStrategy`) and local variable names (`scalingStrategy := &Strategy{...}`).
**Warning signs:** Unnecessary churn changing local variable names that have nothing to do with the CRD field.

## Code Examples

### Removing Fields from NodePoolSpec

Before (current `nodepool_types.go` lines 59-72):
```go
// ScalingStrategy selects which scaling strategy drives this pool.
// "Kubernetes" scales based on pod demand. "GitHubActions" scales based on
// queued GitHub Actions jobs.
// +kubebuilder:validation:Enum=Kubernetes;GitHubActions
// +kubebuilder:default=Kubernetes
// +optional
ScalingStrategy ScalingStrategyType `json:"scalingStrategy,omitempty"`

// GitHubActions configures the GitHub Actions scaling strategy.
// Required when scalingStrategy is "GitHubActions".
// +optional
GitHubActions *GitHubActionsConfig `json:"githubActions,omitempty"`
```

After: Both fields and their comments/markers are deleted entirely.

### Simplifying node_launch.go

Before (line 45):
```go
SkipKubernetesBootstrap: pool.Spec.ScalingStrategy != "" && pool.Spec.ScalingStrategy != stratosv1alpha1.ScalingStrategyKubernetes,
```

After: Line removed entirely (since `SkipKubernetesBootstrap` field will be removed from `TemplateConfig`).

### Removing SkipKubernetesBootstrap from TemplateConfig

Before (`cloudprovider/interface.go` lines 27-32):
```go
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint
    EnableNetworkReadinessTaint bool
    SkipKubernetesBootstrap     bool
}
```

After:
```go
type TemplateConfig struct {
    Labels                      map[string]string
    Taints                      []corev1.Taint
    EnableNetworkReadinessTaint bool
}
```

### Removing setUserData Branch in aws/provider.go

Before (lines 213-220):
```go
if templateConfig != nil && templateConfig.SkipKubernetesBootstrap {
    // Non-K8s strategies: only use customUserData, skip cluster bootstrap
    if nodeClass.Spec.CustomUserData != "" {
        encoded := base64.StdEncoding.EncodeToString([]byte(nodeClass.Spec.CustomUserData))
        input.UserData = aws.String(encoded)
    }
    return nil
}
```

After: Entire `if` block removed. The remaining code (cluster bootstrap path) always runs.

### Simplifying nodepool_validation.go

Before (lines 49-64):
```go
// Validate strategy-specific config
if nodePool.Spec.ScalingStrategy == stratosv1alpha1.ScalingStrategyGitHubActions {
    if nodePool.Spec.GitHubActions == nil {
        return fmt.Errorf("githubActions config is required when scalingStrategy is GitHubActions")
    }
    ghCfg := nodePool.Spec.GitHubActions
    if ghCfg.Organization == "" {
        return fmt.Errorf("githubActions.organization must be specified")
    }
    if len(ghCfg.RunnerLabels) == 0 {
        return fmt.Errorf("githubActions.runnerLabels must have at least one label")
    }
    if ghCfg.SecretRef.Name == "" || ghCfg.SecretRef.Namespace == "" {
        return fmt.Errorf("githubActions.secretRef.name and namespace must be specified")
    }
}
```

After: Entire block deleted. The `validateNodePool` function retains only pool size and NodeClassRef validation.

## State of the Art

| Aspect | Current State | After Phase 10 | Impact |
|--------|---------------|-----------------|--------|
| `NodePoolSpec` fields | 10 fields (including ScalingStrategy, GitHubActions) | 8 fields | Simpler API surface |
| `api/v1alpha1/` files | 10 files (including strategy_types.go) | 9 files | One fewer type definition file |
| `TemplateConfig` fields | 4 fields (including SkipKubernetesBootstrap) | 3 fields | Simpler launch config |
| CRD YAML | Already does not contain strategy fields | Same (regenerated for consistency) | No user-visible change |
| Deepcopy functions | Includes GitHubActionsConfig, SecretReference | Removed by make generate | Smaller generated code |

## Open Questions

1. **Should `SkipKubernetesBootstrap` removal be in Phase 10 or Phase 11?**
   - What we know: It directly references `pool.Spec.ScalingStrategy` (deleted in this phase), so it MUST be in Phase 10. Otherwise `go build` fails.
   - Recommendation: Include in Phase 10. It is part of the CRD field removal blast radius.

2. **Should the `NodeHooks` comment in `manager.go` be updated here or Phase 11?**
   - What we know: The comment "Both KubernetesStrategy and GitHubActionsStrategy implement this interface" is factually wrong after Phase 9 (GitHubActionsStrategy deleted). It is a comment-only change.
   - Recommendation: Include in Phase 10 since we are already touching the lifecycle package (node_launch.go). Minimal extra work.

## Sources

### Primary (HIGH confidence)
- Direct code analysis of all files in `api/v1alpha1/`, `internal/controller/nodepool/`, `internal/cloudprovider/` -- every grep result verified by reading source
- `make generate` and `make manifests` commands verified from Makefile (lines 99-105)
- controller-gen v0.16.5 confirmed installed at `bin/controller-gen`
- CRD YAML verified to NOT contain `scalingStrategy` or `githubActions` (grep returns zero matches)
- Build verified: `go build ./...` succeeds on current branch

### Secondary (MEDIUM confidence)
- Prior research in `.planning/research/PITFALLS.md` documented the deepcopy regeneration ordering requirement
- Prior research in `.planning/research/FEATURES.md` documented the SkipKubernetesBootstrap connection

## Metadata

**Confidence breakdown:**
- Deletion inventory: HIGH -- every reference found by exhaustive grep of the codebase
- Execution ordering: HIGH -- verified from prior phase experience with `make generate` / `make manifests`
- SkipKubernetesBootstrap scope: HIGH -- all 3 files verified by code reading
- CRD YAML state: HIGH -- verified by grep that it does not contain strategy fields

**Research date:** 2026-02-03
**Valid until:** indefinite (this is a codebase-specific analysis, not dependent on external libraries)
