# Architecture

**Analysis Date:** 2026-02-02

## Pattern Overview

**Overall:** Kubernetes operator pattern using controller-runtime (kubebuilder) with event-driven reconciliation and pluggable cloud providers.

**Key Characteristics:**
- Event-driven reconciliation triggered by NodePool, Node, and AWSNodeClass resource changes
- Periodic maintenance loop (default 30s) for state machine transitions and pool management
- Pluggable cloud provider abstraction (AWS EC2, fake for testing)
- Strategy pattern for workload-specific scaling logic (Kubernetes pods, GitHub Actions jobs)
- Node state machine with explicit transitions (warmup → standby → running → terminating)

## Layers

**API/Resource Definition:**
- Purpose: Define Kubernetes custom resources (CRDs) and validation
- Location: `api/v1alpha1/`
- Contains: NodePool, AWSNodeClass, GitHubActionsConfig, and strategy types
- Depends on: Kubernetes API types (corev1, metav1)
- Used by: Controller, cloud providers, strategies

**Controller Layer:**
- Purpose: Main reconciliation logic and orchestration
- Location: `internal/controller/reconciler.go` (NodePoolReconciler.Reconcile)
- Contains: Main reconciliation loop, node state counting, event routing
- Depends on: Cloud providers, strategies, node state management, metrics
- Used by: Kubernetes controller-runtime manager

**Strategy Layer:**
- Purpose: Workload-specific scaling decisions
- Location: `internal/controller/strategy/`
- Contains: ScalingStrategy interface, KubernetesStrategy, GitHubActionsStrategy
- Depends on: Kubernetes client, node state labels
- Used by: Controller for CheckDemand, OnScaleUp, FindScaleDownCandidates, RunMaintenance, DrainAndStop

**Cloud Provider Layer:**
- Purpose: Abstract cloud-specific instance operations
- Location: `internal/cloudprovider/`
- Contains: CloudProvider interface, AWS EC2 implementation, fake provider for testing
- Depends on: AWS SDK v2, Kubernetes client
- Used by: Controller, strategies, node state transitions

**Node State Management:**
- Purpose: Track and validate node state transitions
- Location: `internal/controller/nodestate/`
- Contains: NodeState enum, node labels/annotations, state validation rules
- Depends on: Kubernetes client
- Used by: Controller, cloud sync, strategies

**Node Lifecycle:**
- Purpose: Handle node preparation and readiness checking
- Location: `internal/controller/lifecycle/`
- Contains: Node launch, state transitions, warmup monitoring
- Depends on: Cloud providers, strategies, node state management
- Used by: Controller during node transitions

**Metrics:**
- Purpose: Prometheus metrics collection and recording
- Location: `internal/metrics/metrics.go`
- Contains: Node count metrics, pool configuration metrics, cloud provider call metrics
- Depends on: Prometheus client
- Used by: Controller and cloud providers

## Data Flow

**Reconciliation Cycle (triggered by NodePool, Node, or AWSNodeClass changes):**

1. **Entry:** NodePoolReconciler.Reconcile() receives request
2. **Fetch:** Load NodePool resource from Kubernetes API
3. **Get Providers:** Retrieve or create CloudProvider and ScalingStrategy instances (cached per pool)
4. **Demand Check (Fast Path):** Strategy.CheckDemand() evaluates if nodes need starting
   - KubernetesStrategy: Scans unschedulable pods and calculates resource requirements
   - GitHubActionsStrategy: Checks GitHub Actions job queue
   - Returns: NodesNeeded count and strategy-specific metadata
5. **Scale-Up (If Needed):** Start standby nodes via cloud provider
   - CloudProvider.StartInstance() for each standby node
   - Strategy.OnScaleUp() callback for post-startup tasks
   - Requeue quickly (5s) to handle remaining pods
6. **Cloud Sync (If No Scale-Up):** Detect externally terminated instances
   - CloudProvider.ListInstances() to sync cloud state with Kubernetes nodes
   - Remove stale K8s nodes if cloud instance is gone
7. **Warmup Monitoring:** Track nodes in warmup state (executing user data)
   - Detect when warmup completes (instance self-stops)
   - Transition to standby state
8. **Strategy Maintenance:** Strategy.RunMaintenance()
   - Kubernetes: Remove startup taints when CNI ready, clean pod assignments
   - GitHub Actions: Track job assignments
9. **Metrics:** Record node counts and pool configuration
10. **Scale-Down (If Applicable):** Strategy.FindScaleDownCandidates()
    - Return nodes eligible for shutdown
    - Strategy.DrainAndStop() respects PodDisruptionBudgets
    - Transition to terminating state
11. **Requeue:** Schedule next reconciliation (30s default)

**Node State Transitions:**

```
warmup → standby → running → terminating
```

- **warmup→standby:** Instance launches, runs user data, self-stops, controller detects stop
- **standby→running:** Controller.StartInstance() called for scale-up
- **running→terminating:** Strategy identifies candidate, drain pods, call StopInstance
- **terminating→deleted:** After drain completes, node is cleaned up from Kubernetes

## Key Abstractions

**CloudProvider Interface:**
- Purpose: Abstract all cloud instance operations
- Examples: `internal/cloudprovider/aws/provider.go`, `internal/cloudprovider/fake/provider.go`
- Pattern: Operations work on instance IDs (cloud-agnostic). LaunchInstance is provider-specific and takes NodeClass as parameter.
- Methods: StartInstance, StopInstance, TerminateInstance, GetInstanceState, GetInstance, ListInstances, UpdateInstanceTags

**ScalingStrategy Interface:**
- Purpose: Encapsulate workload-specific scaling logic
- Examples: `internal/controller/strategy/kubernetes.go`, `internal/controller/strategy/githubactions.go`
- Pattern: Strategy.CheckDemand() evaluates demand, Strategy.OnScaleUp() handles post-start tasks
- Methods: CheckDemand, OnScaleUp, FindScaleDownCandidates, DrainAndStop, RunMaintenance

**NodeClass Abstraction:**
- Purpose: Cloud-specific node configuration
- Examples: `api/v1alpha1/aws_nodeclass_types.go`, `api/v1alpha1/nodeclass.go`
- Pattern: NodeClass defines instance type, AMI, IAM role, subnet selection; AWSNodeClassReconciler resolves dynamic fields (subnets, IAM role ARN, instance types)
- Consumers: Cloud providers type-assert NodeClass to concrete type for LaunchInstance

**Node State Labels:**
- Purpose: Track node lifecycle via Kubernetes labels
- Labels: `stratos.sh/pool`, `stratos.sh/state`, `stratos.sh/instance-id`, `stratos.sh/state-since`
- Usage: Controller queries nodes by state, strategies identify candidates by labels

## Entry Points

**Manager Setup (`cmd/stratos/main.go`):**
- Location: `cmd/stratos/main.go`
- Triggers: Process startup with CLI flags or environment variables
- Responsibilities: Load configuration, initialize Kubernetes client, create cloud provider, register NodePoolReconciler and AWSNodeClassReconciler, start controller-runtime manager

**NodePool Reconciliation (`internal/controller/reconciler.go`):**
- Location: `internal/controller/reconciler.go:NodePoolReconciler.Reconcile()`
- Triggers: NodePool resource created/updated, Node changes (via NodeEventHandler), AWSNodeClass changes (via NodeClassEventHandler)
- Responsibilities: Orchestrate reconciliation cycle (demand check, scale-up, cloud sync, strategy maintenance, scale-down)

**Pod Watcher Strategy Handler (`internal/controller/strategy/kubernetes.go`):**
- Location: `internal/controller/strategy/kubernetes.go:KubernetesPodEventHandler()`
- Triggers: Unschedulable pod events
- Responsibilities: Detect pending pods and trigger NodePool reconciliation for scale-up

**AWSNodeClass Reconciler (`internal/cloudprovider/aws/nodeclass_controller.go`):**
- Location: `internal/cloudprovider/aws/nodeclass_controller.go:AWSNodeClassReconciler.Reconcile()`
- Triggers: AWSNodeClass created/updated
- Responsibilities: Resolve instance types, subnets, IAM roles; populate status fields for LaunchInstance

## Error Handling

**Strategy:** Non-critical errors log but don't fail reconciliation; critical errors (like cloud provider failures) requeue the request.

**Patterns:**
- Strategy errors (CheckDemand, FindScaleDownCandidates) log but continue (don't block other operations)
- Cloud sync errors log but continue to maintenance
- Scale-up/scale-down errors are logged with node names and requeue the request
- Validation errors in Reconcile (like invalid NodePool) return error to requeue
- Kubernetes client errors (Get, List) propagate as errors to requeue

## Cross-Cutting Concerns

**Logging:** Uses controller-runtime's structured logging with `log.FromContext(ctx)`. Logger includes pool name, node name, and operation details.

**Validation:** Happens at three levels:
1. CRD schema validation (kubebuilder markers in `api/v1alpha1/`)
2. NodePool spec validation (poolSize ≥ minStandby, reconciliationInterval checks)
3. AWSNodeClass resolution validation (required fields populated by reconciler)

**Authentication:** Kubernetes: Uses in-cluster config or kubeconfig from environment. AWS: Uses AWS SDK v2 default config chain (IAM role, credentials file, env vars).

**Rate Limiting:** AWS API calls go through RateLimiter in `internal/cloudprovider/aws/ratelimit.go` with configurable QPS/Burst multipliers (via `--aws-rate-limit-qps` and `--aws-rate-limit-burst` flags).

**Cloud State Sync:** syncNodesWithCloud() detects externally terminated instances by comparing Kubernetes nodes with cloud provider ListInstances() results. Removes K8s nodes if corresponding cloud instance is gone.

**Network Readiness:** When node starts, NetworkReadinessStrategy enum controls whether controller applies `stratos.sh/not-ready=true:NoSchedule` taint. Taint is removed by strategy when CNI is ready.

---

*Architecture analysis: 2026-02-02*
