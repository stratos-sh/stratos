# Implementation Plan: Stratos - Kubernetes Node Scaler

**Branch**: `001-instance-pool-manager` | **Date**: 2026-01-19 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-instance-pool-manager/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Stratos is a Kubernetes operator that eliminates node provisioning delays by maintaining a pool of pre-warmed, stopped instances. Unlike Karpenter which provisions nodes on-demand (3-5 minutes), Stratos pre-warms nodes (join cluster, pull images, self-stop) and starts them in seconds when pods are pending.

Technical approach:
- Kubernetes operator built with controller-runtime (kubebuilder patterns)
- NodePool CRD (v1alpha1) for configuration
- Event-driven scale-up (watch unschedulable pods) + periodic reconciliation (pool maintenance)
- Pluggable cloud provider interface (AWS EC2 first)
- Prometheus metrics + Kubernetes events for observability

## Technical Context

**Language/Version**: Go 1.22+ (latest stable)
**Primary Dependencies**:
- `sigs.k8s.io/controller-runtime` - Kubernetes controller framework
- `k8s.io/client-go` - Kubernetes client
- `github.com/aws/aws-sdk-go-v2` - AWS SDK for EC2 operations
- `github.com/prometheus/client_golang` - Prometheus metrics

**Storage**: N/A (state stored in Kubernetes resources: NodePool CRD, Node objects with labels)
**Testing**: `go test` with standard library `testing` package, table-driven tests, envtest for controller tests
**Target Platform**: Linux (Kubernetes controller deployment), Kubernetes 1.27+
**Project Type**: Single project - Kubernetes operator
**Performance Goals**:
- Scale-up latency: <30 seconds from pending pod to node Ready
- Pre-warmed node start time: <10 seconds
- Reconciliation loop: 30 second default interval

**Constraints**:
- Cluster-scoped least-privilege RBAC
- Must respect PodDisruptionBudgets during drain
- Client-side rate limiting for cloud API calls

**Scale/Scope**:
- Multiple NodePools per cluster
- poolSize up to ~100 nodes per pool (cloud provider limits apply)
- Single controller deployment

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Principle I: Simplicity First

| Checkpoint | Status | Notes |
|------------|--------|-------|
| Start with simplest solution | ✅ PASS | Single controller pattern, standard CRD approach |
| Functions do one thing well | ✅ PASS | Clear separation: reconciler, cloud provider, node manager |
| Avoid premature abstraction | ✅ PASS | Cloud provider interface justified (AWS now, GCP/Azure later) |
| Dependencies justified | ✅ PASS | controller-runtime (required for K8s operators), AWS SDK (required for EC2) |
| No dead code | ✅ PASS | N/A - new project |

### Principle II: Idiomatic Go

| Checkpoint | Status | Notes |
|------------|--------|-------|
| Pass go fmt, go vet, linters | ✅ PASS | Will configure golangci-lint |
| Explicit error handling | ✅ PASS | Design will follow Go error patterns |
| Package naming conventions | ✅ PASS | See project structure below |
| Documentation comments | ✅ PASS | Required for exported identifiers |
| Composition over inheritance | ✅ PASS | Standard Go patterns |
| Justified concurrency | ✅ PASS | controller-runtime handles concurrency; custom goroutines only where needed |

### Principle III: Test Coverage

| Checkpoint | Status | Notes |
|------------|--------|-------|
| Public APIs have tests | ✅ PASS | Will use envtest for controller tests |
| Tests are deterministic | ✅ PASS | Mock cloud provider for unit tests |
| Table-driven tests | ✅ PASS | Standard approach for Go |
| Test files alongside code | ✅ PASS | `*_test.go` in same package |
| Integration tests marked | ✅ PASS | Build tags for integration tests |

**Gate Status**: ✅ PASS - No violations requiring justification.

## Project Structure

### Documentation (this feature)

```text
specs/001-instance-pool-manager/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/
└── stratos/
    └── main.go              # Entry point

internal/
├── controller/
│   ├── nodepool_controller.go    # NodePool reconciler
│   ├── nodepool_controller_test.go
│   ├── pod_watcher.go            # Unschedulable pod event handler
│   └── pod_watcher_test.go
├── nodemanager/
│   ├── manager.go                # Node lifecycle operations
│   ├── manager_test.go
│   ├── state.go                  # NodeState enum and transitions
│   └── state_test.go
├── cloudprovider/
│   ├── interface.go              # CloudProvider interface
│   ├── aws/
│   │   ├── provider.go           # AWS EC2 implementation
│   │   ├── provider_test.go
│   │   └── ratelimit.go          # Client-side rate limiting
│   └── fake/
│       └── provider.go           # Fake provider for testing
└── metrics/
    ├── metrics.go                # Prometheus metric definitions
    └── metrics_test.go

api/
└── v1alpha1/
    ├── nodepool_types.go         # NodePool CRD types
    ├── nodepool_types_test.go
    ├── groupversion_info.go      # API group registration
    └── zz_generated.deepcopy.go  # Generated deepcopy

config/
├── crd/
│   └── bases/
│       └── stratos.sh_nodepools.yaml  # Generated CRD manifest
├── rbac/
│   ├── role.yaml                 # ClusterRole
│   ├── role_binding.yaml         # ClusterRoleBinding
│   └── service_account.yaml      # ServiceAccount
├── manager/
│   └── manager.yaml              # Controller Deployment
└── samples/
    └── nodepool_sample.yaml      # Example NodePool CR

tests/
├── e2e/
│   └── nodepool_test.go          # End-to-end tests
└── integration/
    └── controller_test.go        # Integration tests with envtest
```

**Structure Decision**: Single project structure following standard Kubernetes operator layout (kubebuilder conventions). The `api/` directory contains CRD types, `internal/` contains implementation, and `config/` contains Kubernetes manifests.

## Complexity Tracking

> No violations requiring justification - Constitution Check passed.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | N/A | N/A |

---

## Post-Design Constitution Re-evaluation

*Completed after Phase 1 design artifacts generated.*

### Re-check: Principle I - Simplicity First

| Design Element | Status | Assessment |
|----------------|--------|------------|
| Single controller pattern | ✅ PASS | NodePoolReconciler handles all reconciliation |
| CloudProvider interface | ✅ PASS | Justified abstraction - AWS now, extensibility for GCP/Azure planned |
| NodeState enum | ✅ PASS | Simple enum for state machine, not over-engineered |
| Data model complexity | ✅ PASS | CRD spec mirrors user mental model, no unnecessary fields |

### Re-check: Principle II - Idiomatic Go

| Design Element | Status | Assessment |
|----------------|--------|------------|
| Package structure | ✅ PASS | `controller`, `nodemanager`, `cloudprovider`, `metrics` - single-word, descriptive |
| Interface design | ✅ PASS | CloudProvider interface follows Go idioms (small, focused) |
| Error types | ✅ PASS | Custom error types for specific failure modes (`InstanceNotFoundError`, etc.) |
| Context usage | ✅ PASS | All operations accept context for cancellation/timeout |

### Re-check: Principle III - Test Coverage

| Design Element | Status | Assessment |
|----------------|--------|------------|
| envtest for controllers | ✅ PASS | Standard K8s operator testing pattern |
| Fake cloud provider | ✅ PASS | Enables deterministic unit tests |
| Test file placement | ✅ PASS | `*_test.go` alongside source files |
| Build tag separation | ✅ PASS | Integration tests marked with `//go:build integration` |

### Final Gate Status

**✅ PASS** - Design adheres to all Constitution principles. No violations requiring justification.

---

## Generated Artifacts

| Artifact | Path | Description |
|----------|------|-------------|
| Research | [research.md](./research.md) | Technology decisions, patterns, best practices |
| Data Model | [data-model.md](./data-model.md) | Entity definitions, CRD spec, state machine |
| CRD Contract | [contracts/nodepool-crd.yaml](./contracts/nodepool-crd.yaml) | OpenAPI schema for NodePool CRD |
| RBAC Contract | [contracts/rbac.yaml](./contracts/rbac.yaml) | Cluster permissions definition |
| CloudProvider Interface | [contracts/cloudprovider-interface.go](./contracts/cloudprovider-interface.go) | Go interface for cloud operations |
| Quickstart | [quickstart.md](./quickstart.md) | Developer onboarding guide |

---

## Next Steps

Run `/speckit.tasks` to generate the implementation task list based on this plan.
