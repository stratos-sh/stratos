## Context

Currently, AWSNodeClass requires users to provide complete `userData` as a raw string containing:
- MIME multipart structure (for AL2023) or TOML (for Bottlerocket) or shell script (for AL2)
- Embedded cluster credentials (apiServerEndpoint, certificateAuthority, CIDR)
- Kubelet configuration
- Stratos warmup script (wait for kubelet, init EBS, poweroff)

This is passed directly to EC2 RunInstances. The complexity is entirely on the user, leading to:
- Configuration errors (e.g., trailing spaces in base64 certificates)
- Silent failures (nodes fail to join, no validation until runtime)
- Duplication of cluster config across every AWSNodeClass

The controller already has `clusterName` from Helm values. Extending this to include full cluster config enables automatic userData generation.

## Goals / Non-Goals

**Goals:**
- Stratos generates correct userData based on `bootstrapTemplate` (AL2023/AL2/Bottlerocket)
- Cluster configuration provided once via Helm values
- Optional `kubelet` block for common customizations
- Optional `customUserData` for user scripts (merged, not replaced)
- Auto-discover AMI when `amiSelector` is omitted
- Validate configuration before launch (fail fast)

**Non-Goals:**
- Windows support (future consideration)
- Custom bootstrap templates (users who need full control can fork)
- Multi-cluster management from single Stratos instance (cluster config is global)

## Decisions

### Decision 1: Cluster config location - Helm values

**Choice:** Add cluster config to Helm values, passed to controller as flags/env vars.

```yaml
# values.yaml
cluster:
  name: main                    # existing, now required for userData
  apiServerEndpoint: https://...
  certificateAuthority: LS0t...
  cidr: 172.20.0.0/16
```

**Alternatives considered:**
- ConfigMap: More dynamic but adds complexity, rarely changes
- Per-NodeClass: Duplicates config, current pain point
- Auto-discover via EKS API: Rate limits during scale-up, not portable

**Rationale:** Cluster config is static and set once at install time. Helm values are the simplest approach and match Karpenter's pattern.

### Decision 2: Bootstrap generator architecture

**Choice:** Interface-based generator with per-template implementations, flat files in AWS provider package.

```
internal/cloudprovider/aws/
├── userdata.go       # BootstrapGenerator interface, BootstrapConfig types, factory
├── al2023.go         # AL2023Generator (nodeadm MIME)
├── al2.go            # AL2Generator (bootstrap.sh)
├── bottlerocket.go   # BottlerocketGenerator (TOML)
├── warmup.go         # Shared warmup script logic
├── ami.go            # DefaultAMISelector for auto-discovery
├── provider.go       # (existing)
├── resolver.go       # (existing)
└── ...
```

```go
type BootstrapGenerator interface {
    Generate(config BootstrapConfig) (string, error)
}

type BootstrapConfig struct {
    Cluster      ClusterConfig
    Kubelet      *KubeletConfig
    CustomScript string
    PoolName     string
}
```

**Alternatives considered:**
- Template files: Less type-safe, harder to test
- Single generator with switch: Works but less extensible
- Shared `internal/bootstrap/` package: Premature abstraction - all current templates are AWS/EKS-specific
- Subdirectory `aws/bootstrap/`: Unnecessary - generator is internal to AWS provider

**Rationale:** Flat files in AWS package since the generator is only used by `provider.go`. File names match `bootstrapTemplate` enum values (AL2023, AL2, Bottlerocket). Keeps it simple - one package, no extra imports.

### Decision 3: Warmup script injection

**Choice:** Warmup script is always injected by the generator, not user-configurable.

For AL2023/AL2: Added as separate MIME part after bootstrap config.
For Bottlerocket: Uses `ControllerStop` completion mode - no userData script needed, controller stops instance when node becomes Ready.

**Rationale:** Warmup is core to Stratos's standby model. AL2023/AL2 use self-stop scripts, Bottlerocket uses controller-managed stopping (already implemented).

### Decision 4: customUserData merging strategy

**Choice:** User scripts run AFTER Stratos warmup completes (for AL2023/AL2). For Bottlerocket, custom TOML is merged with generated config.

For AL2023/AL2:
```
MIME Part 1: Bootstrap config (nodeadm/bootstrap.sh)
MIME Part 2: Stratos warmup script
MIME Part 3: User's customUserData (if provided)
```

For Bottlerocket:
```toml
[settings.kubernetes]
cluster-name = "..."
api-server = "..."
# ... generated config

# User's customUserData TOML is merged here
```

**Rationale:** AL2023/AL2 warmup must complete (node stops) before user scripts. Bottlerocket uses ControllerStop mode, so custom TOML is simply merged with the generated kubernetes settings.

### Decision 5: AMI auto-discovery

**Choice:** When `amiSelector` is omitted, derive selector from `bootstrapTemplate` + detected cluster version.

```go
func DefaultAMISelector(template, arch, k8sVersion string) AMISelector {
    switch template {
    case "AL2023":
        return AMISelector{
            Name:  fmt.Sprintf("amazon-eks-node-al2023-%s-standard-%s-*", arch, k8sVersion),
            Owner: "amazon",
        }
    case "Bottlerocket":
        return AMISelector{
            Name:  fmt.Sprintf("bottlerocket-aws-k8s-%s-%s-*", k8sVersion, arch),
            Owner: "amazon",
        }
    case "AL2":
        return AMISelector{
            Name:  fmt.Sprintf("amazon-eks-node-%s-%s-*", k8sVersion, arch),
            Owner: "amazon",
        }
    }
}
```

Kubernetes version is detected at controller startup via API server discovery.

**Rationale:** Reduces config burden for common case. Users who need specific AMIs can still use `amiSelector`.

### Decision 6: AWSNodeClass API changes

**New fields:**
```go
type AWSNodeClassSpec struct {
    // BootstrapTemplate determines the bootstrap format
    // +kubebuilder:validation:Enum=AL2023;AL2;Bottlerocket
    // +kubebuilder:validation:Required
    BootstrapTemplate string `json:"bootstrapTemplate"`

    // Architecture for AMI auto-discovery
    // +kubebuilder:validation:Enum=x86_64;arm64
    // +kubebuilder:default=x86_64
    Architecture string `json:"architecture,omitempty"`

    // Kubelet configuration (optional)
    Kubelet *KubeletConfig `json:"kubelet,omitempty"`

    // CustomUserData - scripts merged with generated bootstrap (optional)
    CustomUserData string `json:"customUserData,omitempty"`

    // AMISelector - optional, auto-discovers if omitted
    AMISelector *AMISelector `json:"amiSelector,omitempty"`

    // ... existing fields (instanceType, subnetSelector, etc.)
}

type KubeletConfig struct {
    MaxPods    *int32            `json:"maxPods,omitempty"`
    NodeLabels map[string]string `json:"nodeLabels,omitempty"`
    NodeTaints []corev1.Taint    `json:"nodeTaints,omitempty"`
    ExtraArgs  map[string]string `json:"extraArgs,omitempty"`
}
```

**Removed fields:**
- `userData` - replaced by generated userData
- `ami` - use `amiSelector` instead (simpler API, one way to specify)

## Risks / Trade-offs

**Risk: Cluster version detection fails**
→ Mitigation: Fall back to requiring `amiSelector` if version cannot be detected. Log warning.

**Risk: Generated userData incompatible with future EKS changes**
→ Mitigation: Follow EKS AMI release notes. `bootstrapTemplate` versioning could be added later if needed (e.g., `AL2023-v2`).

**Trade-off: Less flexibility than raw userData**
→ Acceptable: Users needing full control can fork or use a different tool. Simplicity for 90% case is worth it.

**Trade-off: Cluster config in Helm means re-deploy to change**
→ Acceptable: Cluster config rarely changes. If it does, nodes would need re-warmup anyway.
