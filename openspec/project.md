# Stratos Project Overview

**Repository**: stratos-sh/stratos
**Domain**: stratos.sh
**Type**: Kubernetes Operator
**Language**: Go 1.22+

## What is Stratos?

Stratos is a Kubernetes operator that eliminates node provisioning delays by maintaining pools of pre-warmed, stopped instances ready to start in seconds.

### How It Works

1. **Pre-warm**: Stratos launches instances that run initialization (join cluster, pull images) then self-stop
2. **Standby**: Stopped instances wait in the pool, costing minimal resources
3. **Instant Scale**: When pods are pending, Stratos starts a pre-warmed instance (seconds, not minutes)
4. **Scale Down**: When nodes are idle, Stratos drains and stops them, returning to standby

### Stratos vs Karpenter

| Aspect | Karpenter | Stratos |
|--------|-----------|---------|
| Provisioning | On-demand (3-5 min) | Pre-warmed (seconds) |
| Node readiness | Cold start every time | Already configured, images pulled |
| Cost model | Pay only when running | Small cost for stopped instances |
| Best for | General workloads | Time-sensitive scaling (CI/CD, ML inference, bursty traffic) |

## Architecture

```
cmd/stratos/main.go           # Entry point, manager setup
api/v1alpha1/                 # NodePool CRD types
internal/
├── controller/               # Kubernetes reconcilers
│   ├── nodepool_controller.go    # Main reconciliation loop
│   └── pod_watcher.go            # Pending pod detection
├── cloudprovider/            # Cloud abstraction layer
│   ├── interface.go              # CloudProvider interface
│   ├── aws/provider.go           # AWS EC2 implementation
│   └── fake/provider.go          # Mock provider for testing
├── nodemanager/              # Node lifecycle management
├── drain/                    # Graceful node eviction
└── metrics/                  # Prometheus metrics
config/
├── crd/bases/                # Generated CRD manifests
├── rbac/                     # RBAC manifests
└── samples/                  # Example NodePool resources
```

## Key Concepts

- **NodePool**: CRD defining a pool of pre-warmed nodes with `poolSize` and `minStandby` settings
- **NodeState**: warmup (initializing), standby (stopped, ready), running (active), terminating (draining)
- **CloudProvider**: Abstraction for cloud operations (launch, start, stop, terminate)
- **Pre-warming**: Node initialization (cluster join, image pull) followed by self-stop

## Technical Stack

- **Framework**: controller-runtime (kubebuilder patterns)
- **Cloud**: AWS EC2 (pluggable interface for future providers)
- **Kubernetes**: 1.27+ required
- **Observability**: Prometheus metrics, Kubernetes events

## Existing Specs

- `openspec/specs/stratos-core/` - Core NodePool management and node lifecycle
