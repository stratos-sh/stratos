# Stratos

## What This Is

A Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, stopped instances ready to start in seconds. Built on controller-runtime (kubebuilder pattern) with a clean, well-bounded package structure following Karpenter conventions. Now supports pre-pulling container images during warmup for instant workload scheduling.

## Core Value

Kubernetes pod-driven scaling with no unnecessary abstraction layers — the code does one thing and does it directly.

## Requirements

### Validated

- ✓ NodePool CRD reconciliation with event-driven + periodic maintenance — existing
- ✓ Node state machine (warmup → standby → running → terminating) — existing
- ✓ CloudProvider abstraction with AWS EC2 and fake implementations — existing
- ✓ ScalingStrategy abstraction removed — v1.1 (replaced with direct scaling package)
- ✓ Remove ScalingStrategy interface — v1.1
- ✓ Remove GitHub Actions strategy and GitHub API client packages — v1.1
- ✓ Remove scalingStrategy and githubActions fields from NodePool CRD — v1.1
- ✓ Rename strategy/kubernetes/ to internal/scaling/ — v1.1
- ✓ Controller uses scaling package directly (no factory/cache) — v1.1
- ✓ All existing tests pass after simplification — v1.1
- ✓ AWSNodeClass resolution (subnets, IAM, instance types, AMI) — existing
- ✓ Pod-driven scale-up (detect unschedulable pods, start standby nodes) — existing
- ✓ Scale-down with drain (PDB-respecting, graceful) — existing
- ✓ Cloud sync (detect externally terminated instances) — existing
- ✓ Warmup monitoring (detect instance self-stop after user data) — existing
- ✓ Network readiness detection with startup taints — existing
- ✓ Prometheus metrics collection — existing
- ✓ Helm chart deployment — existing
- ✓ Integration tests with envtest — existing
- ✓ E2E test framework — existing
- ✓ Clean controller package — per-CRD packages with clear responsibilities — v1.0
- ✓ Consistent naming — subject_role.go convention, no ambiguous duplicates — v1.0
- ✓ Clear package boundaries — controller/, strategy/, lifecycle/, cloudprovider/ well-defined — v1.0
- ✓ Focused files — no file exceeds 300 lines, single responsibility per file — v1.0
- ✓ Proper context propagation — zero context.Background() in production code — v1.0
- ✓ Standardized error handling — all fmt.Errorf uses %w consistently — v1.0
- ✓ No dead code — _extracted/ removed, all exports used — v1.0
- ✓ Package documentation — doc.go for every package — v1.0
- ✓ Structural linters — depguard, funlen, cyclop, contextcheck enforced — v1.0
- ✓ Node state management — nodestate/ pure leaf package with valid transitions — v1.0
- ✓ Remove dead code (UncordonNode) — v1.1.1
- ✓ Rename Strategy → Scaler across codebase — v1.1.1
- ✓ Rename drainHelper → nodeDrainer, drainConfig → drainOptions — v1.1.1
- ✓ Rename generic files to follow subject_role.go convention — v1.1.1
- ✓ Replace ScalingDemand.Metadata interface{} with Pods []corev1.Pod — v1.1.1
- ✓ Warmup image pre-pull — user-configured images pulled during instance warmup — v1.2
- ✓ Configurable image pull policy — Required (default) or BestEffort — v1.2
- ✓ AL2023 + AL2 support — inject pull commands into user data — v1.2
- ✓ CRD fields — spec.preWarm.images and spec.preWarm.imagePullPolicy on NodePool — v1.2
- ✓ ECR authentication — automatic via aws ecr get-login-password — v1.2
- ✓ Image pinning — prevent kubelet GC eviction with io.cri-containerd.pinned — v1.2
- ✓ Bottlerocket warning — ImagePrePullSupported=False condition — v1.2

### Active

(None — milestone complete, ready for next milestone)

### Out of Scope

- Performance optimizations beyond what falls out of cleaner code — not the goal
- Documentation site (docs/) rewrites — focus is on code, not user docs
- GitHub Actions scaling — will be a separate project
- Adding new scaling strategies — Stratos is Kubernetes-only going forward
- Dynamic pending-pod image pre-pull at start time — complexity of passing image list to starting node without API server access; considered and rejected
- Bottlerocket image pre-pull — immutable OS has different container runtime story; defer to future milestone

## Current State

**Last shipped:** v1.2 Warmup Image Pre-Pull (2026-02-05)
**Next milestone:** TBD

## Context

Shipped v1.2 warmup image pre-pull with 23,417 lines of Go across 12 internal packages. Four milestones completed in 3 days (v1.0, v1.1, v1.1.1, v1.2) — codebase restructure, scaling simplification, naming cleanup, and warmup image pre-pull.

**Architecture after v1.2:**
```
internal/
├── config/                   # ClusterConfig + controller config
├── controller/
│   ├── setup.go              # Aggregator — registers all controllers
│   ├── nodepool/             # NodePool CRD reconciler
│   │   ├── lifecycle/        # Node lifecycle operations
│   │   └── nodestate/        # Node state machine (pure leaf, no upward deps)
│   └── nodeclass/            # NodeClass CRD reconciler
├── scaling/                  # Kubernetes pod-based scaling (direct, no interface)
├── warmup/                   # Warmup script generator for image pre-pull
├── cloudprovider/            # CloudProvider interface + implementations
│   ├── aws/                  # AWS EC2 implementation + user data generators
│   └── fake/                 # Mock provider for testing
└── metrics/                  # Prometheus metrics
```

**Packages added in v1.2:** internal/warmup/ (image pre-pull script generator)

**Tech debt (minor):**
- ReasonImagePrePullSupported constant defined but unused — defined for symmetry, harmless

## Constraints

- **K8s behavior preservation**: All Kubernetes pod-driven scaling must work identically after changes
- **Go idioms**: Follow standard Go package design — small, focused packages with clear interfaces
- **controller-runtime patterns**: Stay within kubebuilder/controller-runtime conventions
- **CRD migration**: Removing CRD fields is a breaking change — acceptable for major versions
- **Test recovery**: Tests will break during restructure but must all pass at the end

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Full refactor scope — change interfaces, split/merge packages | User wants whatever it takes to make code readable | ✓ Good |
| Break-and-fix test approach | Restructuring first, fix tests after — faster iteration | ✓ Good |
| Delete _extracted/ directory | Dead code from previous spot replacement extraction | ✓ Good |
| Nothing off limits | api/, controller/, cloudprovider/ all fair game for restructure | ✓ Good |
| Merged reconcile.go into reconciler.go | Single file for type + entry point + main loop | ✓ Good |
| subject_role.go naming convention | Descriptive file names (nodepool_validation.go, provider_cache.go) | ✓ Good |
| Factory in consumer, not strategy/ | Avoid Go import cycle between parent and child packages | ✓ Good |
| Aggregator setup.go at controller/ root | Single entry point for all controller registration | ✓ Good |
| safeInt32() overflow guard | Avoids nolint comments for gosec G115 | ✓ Good |
| Orchestrator pattern for reconcileNodePool | 6 focused phase helpers instead of one monolithic function | ✓ Good |
| Remove ScalingStrategy interface | Only K8s supported, GHA becomes separate project. No need for abstraction. | ✓ Good |
| Direct package import over merge | Keep scaling logic in its own package (internal/scaling/) rather than merging 1,814 LOC into controller | ✓ Good |
| Remove CRD fields (breaking change) | Clean API surface, no deprecated fields to maintain | ✓ Good |
| Type aliases for migration | Zero-overhead backward compatibility during package relocation | ✓ Good |
| Single scaler field replaces per-pool cache | All pools share one stateless Strategy — caching per pool was unnecessary | ✓ Good |
| +kubebuilder:object:generate=false for interfaces | Prevents controller-gen from generating invalid deepcopy methods | ✓ Good |
| Rename Strategy → Scaler | Vestige of ScalingStrategy interface removed in v1.1. "Scaler" describes what it does. | ✓ Good |
| Concrete Metadata type | interface{} was for multi-strategy support. Only pods now. | ✓ Good |
| Image pre-pull on NodePool, not AWSNodeClass | Images are a pool concern (what workloads run), not a cloud concern (how instances are configured) | ✓ Good |
| Required as default imagePullPolicy | Stricter default ensures standby nodes have expected images; users opt into BestEffort explicitly | ✓ Good |
| Defer Bottlerocket support | Immutable OS has different container runtime; AL2023 + AL2 cover primary use cases | ✓ Good |
| Defer dynamic pending-pod image pull | No clean mechanism to pass image list to node at start time without API server access | ✓ Good |
| Use ctr exclusively (not crictl) | crictl missing on AL2023, ctr universal on all EKS AMIs | ✓ Good |
| Image pinning with io.cri-containerd.pinned | Prevents kubelet GC eviction of pre-pulled images | ✓ Good |
| Flat sequential bash script for warmup | Simpler to generate and debug than bash functions | ✓ Good |
| AL2023 conditional MIME format switch | Plain YAML when no images, MIME multipart when images — backward compatible | ✓ Good |
| PreWarmConfig as pointer field | nil clearly means "no pre-warm configured"; distinguishes from empty struct | ✓ Good |
| checkImagePrePullSupport is non-blocking | Sets condition but doesn't prevent launch — informational only | ✓ Good |

---
*Last updated: 2026-02-05 after v1.2 milestone*
