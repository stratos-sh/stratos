# Research: Stratos - Kubernetes Node Scaler

**Date**: 2026-01-19
**Branch**: `001-instance-pool-manager`
**Status**: Complete

This document consolidates research findings for implementing Stratos, resolving all technical decisions and unknowns identified during planning.

---

## Table of Contents

1. [Controller-Runtime Patterns](#1-controller-runtime-patterns)
2. [AWS EC2 Lifecycle Management](#2-aws-ec2-lifecycle-management)
3. [Kubernetes Node Drain Operations](#3-kubernetes-node-drain-operations)
4. [Prometheus Metrics](#4-prometheus-metrics)
5. [Linting Configuration](#5-linting-configuration)
6. [Technology Decisions Summary](#6-technology-decisions-summary)

---

## 1. Controller-Runtime Patterns

### 1.1 Multi-Resource Watching (NodePool + Pods + Nodes)

**Decision**: Single NodePoolReconciler watching multiple resource types using `handler.EnqueueRequestsFromMapFunc`.

**Rationale**: controller-runtime pattern is to have a single primary reconciler that watches secondary resources and maps changes back to the primary resource.

**Implementation Pattern**:

```go
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&stratosv1alpha1.NodePool{}).              // Primary resource
        Watches(
            &corev1.Pod{},                              // Watch Pods
            handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
                pod := obj.(*corev1.Pod)
                // Find which NodePool(s) this pod could match
                return findMatchingNodePools(ctx, pod)
            }),
            builder.WithPredicates(isPodUnschedulable()),
        ).
        Watches(
            &corev1.Node{},                             // Watch Nodes
            handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
                // Find which NodePool this node belongs to
                return findNodePoolForNode(ctx, obj)
            }),
        ).
        Complete(r)
}
```

**Alternatives Considered**:
- Separate controllers for Pods and Nodes: Rejected - adds complexity without benefit
- Informers without controller-runtime: Rejected - controller-runtime handles edge cases

### 1.2 Event-Driven vs Periodic Reconciliation

**Decision**: Hybrid approach - event-driven for scale-up, periodic for pool maintenance.

**Rationale**:
- Event-driven provides immediate response to pending pods (latency-critical)
- Periodic reconciliation detects external changes (instance termination, failures)

**Implementation**:

```go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    Scheme:     scheme,
    SyncPeriod: &syncPeriod30s,  // 30 seconds for pool maintenance
})
```

**Key Pattern**: Reconcile must be idempotent - read all state, compute delta, write corrections.

### 1.3 Finalizer Pattern for NodePool Deletion

**Decision**: Use `controllerutil` finalizer pattern for clean resource cleanup.

**Implementation**:

```go
const finalizerName = "stratos.sh/nodepool-finalizer"

func (r *NodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    np := &stratosv1alpha1.NodePool{}
    if err := r.Get(ctx, req.NamespacedName, np); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if np.ObjectMeta.DeletionTimestamp != nil {
        if controllerutil.ContainsFinalizer(np, finalizerName) {
            if err := r.cleanupNodePoolResources(ctx, np); err != nil {
                return ctrl.Result{}, err
            }
            controllerutil.RemoveFinalizer(np, finalizerName)
            if err := r.Update(ctx, np); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer on creation
    if !controllerutil.ContainsFinalizer(np, finalizerName) {
        controllerutil.AddFinalizer(np, finalizerName)
        if err := r.Update(ctx, np); err != nil {
            return ctrl.Result{}, err
        }
    }

    return r.reconcileNodePool(ctx, np)
}
```

### 1.4 Status Subresource and Conditions

**Decision**: Use standard Kubernetes conditions pattern with `meta.SetStatusCondition`.

**NodePool Status Structure**:

```go
type NodePoolStatus struct {
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
    Ready              int32              `json:"ready,omitempty"`
    Running            int32              `json:"running,omitempty"`
    Standby            int32              `json:"standby,omitempty"`
    Warmup             int32              `json:"warmup,omitempty"`
}
```

**Condition Types**:
- `Ready`: Pool has minimum standby nodes available
- `Reconciling`: Pool is actively being reconciled
- `Degraded`: Pool cannot meet minStandby due to errors

### 1.5 Controller Testing with envtest

**Decision**: Use envtest for integration tests, mock cloud provider for unit tests.

**Test Setup Pattern**:

```go
testEnv = &envtest.Environment{
    CRDDirectoryPaths: []string{"../../config/crd/bases"},
}
cfg, err = testEnv.Start()
```

**Key Testing Patterns**:
- Use `Eventually()` with Gomega for eventual consistency
- Create fake cloud provider implementing CloudProvider interface
- Test reconciliation behavior, not implementation details

---

## 2. AWS EC2 Lifecycle Management

### 2.1 Stop/Start State Preservation

**What is Preserved on Stop**:
- EBS volumes (root and data) - fully persistent
- Private IPv4 addresses (primary and secondary)
- IPv6 addresses
- Elastic IP addresses (if associated)
- Security group associations
- VPC and subnet associations
- Instance tags

**What is Lost on Stop**:
- Public IPv4 addresses (reassigned on start)
- Data in RAM/memory
- Instance store volume data

**Critical for Stratos**: Private IPs persist, which is essential for maintaining Kubernetes cluster membership. Nodes can rejoin cluster after start without re-initialization.

### 2.2 Instance State Transitions

**State Codes**:
- 0: pending
- 16: running
- 32: shutting-down
- 48: terminated
- 64: stopping
- 80: stopped

**State Flow**:
```
pending → running → stopping → stopped → pending (on start)
              ↓
        shutting-down → terminated
```

**Detection Method**: EventBridge (preferred over polling)
- EC2 Instance State-change Notification events
- Real-time, doesn't consume API tokens
- Scales efficiently

**Fallback**: Polling `DescribeInstances` with pagination (`MaxResults=10`) for larger token bucket.

### 2.3 API Rate Limiting

**Stop/Start Specific Limits**:

| Operation | Bucket Max | Refill Rate |
|-----------|-----------|------------|
| StartInstances | 1000 tokens | 2/sec |
| StopInstances | 1000 tokens | 20/sec |
| DescribeInstances (paginated) | 100 tokens | 10/sec |

**Best Practices**:
1. Use pagination in DescribeInstances queries
2. Implement exponential backoff with jitter for retries
3. Consider EventBridge instead of polling for state changes
4. Monitor RequestLimitExceeded CloudWatch metrics

### 2.4 AWS SDK Go v2 Patterns

**Error Handling**:

```go
import (
    "github.com/aws/smithy-go"
    awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
)

if err != nil {
    var ae smithy.APIError
    if errors.As(err, &ae) {
        switch ae.ErrorCode() {
        case "InvalidInstanceID.NotFound":
            // Instance doesn't exist - don't retry
        case "Throttling", "RequestLimitExceeded":
            // Rate limited - SDK will retry automatically
        }
    }
}
```

**Retry Configuration**:

```go
cfg, err := config.LoadDefaultConfig(ctx,
    config.WithRetryer(func() aws.Retryer {
        return retry.AddWithMaxAttempts(retry.NewStandard(), 5)
    }),
)
```

### 2.5 Tagging Strategy

**Required Tags**:

```go
tags := []types.Tag{
    {Key: aws.String("managed-by"), Value: aws.String("stratos")},
    {Key: aws.String("stratos.sh/pool"), Value: aws.String(poolName)},
    {Key: aws.String("stratos.sh/cluster"), Value: aws.String(clusterName)},
    {Key: aws.String("stratos.sh/state"), Value: aws.String("warmup|standby|running")},
}
```

**Benefits**:
- Filter instances by pool: `--filters "Name=tag:stratos.sh/pool,Values=pool-name"`
- Cost allocation tracking
- Automation via AWS Config rules

---

## 3. Kubernetes Node Drain Operations

### 3.1 Drain Implementation

**Decision**: Use `k8s.io/kubectl/pkg/drain` package - well-tested, handles edge cases.

**API Calls Made by Drain**:
1. GET /api/v1/nodes/{nodeName} - Fetch node
2. PATCH /api/v1/nodes/{nodeName} - Set unschedulable (cordon)
3. GET /api/v1/namespaces/*/pods - List pods on node
4. POST /api/v1/namespaces/{ns}/pods/{pod}/eviction - Evict pod (respects PDB)
5. WATCH /api/v1/namespaces/*/pods - Wait for termination

**Implementation**:

```go
drainer := &drain.Helper{
    Ctx:                  ctx,
    Client:               clientset,
    GracePeriodSeconds:   -1,              // Use pod default
    IgnoreAllDaemonSets:  true,            // Skip DaemonSet pods
    DeleteEmptyDirData:   false,           // Skip pods with emptyDir
    Force:                false,           // Don't delete orphaned pods
    Timeout:              5 * time.Minute,
    DisableEviction:      false,           // Use Eviction API (respects PDB)
}

if err := drain.RunCordonOrUncordon(drainer, node, true); err != nil {
    return err
}
if err := drain.RunNodeDrain(drainer, nodeName); err != nil {
    return err
}
```

### 3.2 PodDisruptionBudget Handling

**How PDB Works with Eviction API**:
- API server validates PDB constraints before allowing eviction
- 200 OK: Eviction allowed
- 429 Too Many Requests: PDB would be violated, retry later
- 500 Internal Server Error: PDB misconfiguration

**Key Pattern**: Always use Eviction API (not direct DELETE) to respect PDBs automatically.

### 3.3 DaemonSet Pod Handling

**Decision**: Ignore DaemonSet pods during drain (`IgnoreAllDaemonSets: true`).

**Rationale**:
- DaemonSet pods will be recreated immediately after deletion
- They ignore unschedulable markings
- Not relevant to scale-down decisions

### 3.4 Timeout Configuration

**Recommended Values**:

| Scenario | Grace Period | Total Timeout | Force Delete |
|----------|-------------|---------------|--------------|
| Scale-down | pod default | 5 min | No |
| Node recycle | 30s | 2 min | Yes |
| Emergency | 5s | 30s | Yes |

### 3.5 Karpenter Patterns

**Learnings from Karpenter**:
1. Taint-based cordon: `Karpenter.sh/disrupted:NoSchedule`
2. Priority-based pod eviction (non-critical first)
3. Volume-aware draining (wait for volume detachment)
4. Respect both pod and node termination grace periods

---

## 4. Prometheus Metrics

### 4.1 Metric Types for Stratos

**Gauges** (current state):
- `stratos_nodepool_nodes_total{pool, state}` - Nodes by state (warmup/standby/running)
- `stratos_nodepool_desired_standby{pool}` - Desired standby count
- `stratos_nodepool_pool_size{pool}` - Pool size limit

**Counters** (cumulative):
- `stratos_scaleup_total{pool}` - Total scale-up operations
- `stratos_scaledown_total{pool}` - Total scale-down operations
- `stratos_warmup_failures_total{pool, reason}` - Pre-warming failures

**Histograms** (latency):
- `stratos_scaleup_duration_seconds{pool}` - Time from pending pod to node Ready
- `stratos_warmup_duration_seconds{pool}` - Time from launch to standby
- `stratos_drain_duration_seconds{pool}` - Time to drain node

### 4.2 Implementation Pattern

```go
var (
    nodesTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "stratos_nodepool_nodes_total",
            Help: "Total nodes by state",
        },
        []string{"pool", "state"},
    )

    scaleupDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "stratos_scaleup_duration_seconds",
            Help:    "Scale-up latency in seconds",
            Buckets: []float64{1, 5, 10, 30, 60, 120},
        },
        []string{"pool"},
    )
)

func init() {
    prometheus.MustRegister(nodesTotal, scaleupDuration)
}
```

---

## 5. Linting Configuration

### 5.1 golangci-lint Configuration

**Decision**: Use golangci-lint v2 with project-specific configuration.

**`.golangci.yml`**:

```yaml
version: "2"

linters:
  default: none
  enable:
    - errcheck      # Check for unchecked errors
    - gosimple      # Simplify code
    - govet         # Reports suspicious constructs
    - ineffassign   # Detects ineffectual assignments
    - staticcheck   # Go static analysis
    - unused        # Check for unused code
    - gosec         # Security issues
    - gocyclo       # Cyclomatic complexity
    - misspell      # Spelling mistakes

  settings:
    errcheck:
      check-type-assertions: true
      check-blank: true
    gocyclo:
      min-complexity: 15
    govet:
      enable:
        - shadow
        - nilness

run:
  timeout: 5m
  skip-dirs:
    - vendor
    - testdata

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gocyclo
        - errcheck
        - gosec
```

---

## 6. Technology Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Controller framework | controller-runtime | Industry standard for K8s operators |
| CRD API version | v1alpha1 | Standard graduation path |
| Cloud provider | AWS EC2 first | Primary target, pluggable interface |
| State detection | EventBridge + polling fallback | Real-time without API token cost |
| Drain implementation | kubectl/pkg/drain | Well-tested, handles PDB |
| Metrics | Prometheus client_golang | Standard for K8s ecosystem |
| Linting | golangci-lint | Fast, comprehensive |
| Testing | envtest + fake cloud provider | Integration + unit coverage |
| Rate limiting | SDK built-in + custom backoff | Handles throttling gracefully |

---

## Sources

### controller-runtime
- [controller-runtime README](https://github.com/kubernetes-sigs/controller-runtime/blob/main/README.md)
- [controller-runtime FAQ](https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md)
- [kubebuilder watching resources](https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/reference/watching-resources/)

### AWS EC2
- [EC2 instance state changes](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-lifecycle.html)
- [How EC2 stop/start works](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/how-ec2-instance-stop-start-works.html)
- [EC2 API throttling](https://docs.aws.amazon.com/ec2/latest/devguide/ec2-api-throttling.html)
- [AWS SDK Go v2 retries](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-retries-timeouts.html)

### Kubernetes Node Drain
- [kubectl drain package](https://pkg.go.dev/k8s.io/kubectl/pkg/drain)
- [PodDisruptionBudgets](https://kubernetes.io/docs/tasks/run-application/configure-pdb/)
- [API-initiated eviction](https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/)

### Karpenter
- [Karpenter disruption](https://karpenter.sh/docs/concepts/disruption/)
- [Karpenter termination](https://deepwiki.com/kubernetes-sigs/karpenter/4.3-termination-and-deprovisioning)

### Prometheus
- [Prometheus Go client](https://github.com/prometheus/client_golang)

### golangci-lint
- [golangci-lint configuration](https://golangci-lint.run/usage/configuration/)
