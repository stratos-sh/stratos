# Codebase Structure

**Analysis Date:** 2026-02-02

## Directory Layout

```
/home/roeeh/projects/presto/
├── api/v1alpha1/                 # Kubernetes CRD types and validation
├── cmd/stratos/                  # Binary entry point
├── internal/
│   ├── cloudprovider/            # Cloud abstraction layer
│   │   ├── aws/                  # AWS EC2 implementation
│   │   └── fake/                 # Mock provider for testing
│   ├── controller/               # Main reconciliation logic
│   │   ├── lifecycle/            # Node lifecycle operations
│   │   ├── nodestate/            # Node state constants and validation
│   │   └── strategy/             # Scaling strategy implementations
│   ├── config/                   # Configuration loading
│   ├── github/                   # GitHub API client for Actions strategy
│   └── metrics/                  # Prometheus metrics
├── deploy/charts/stratos/        # Helm chart for deployment
├── examples/                     # Example NodePool and AWSNodeClass manifests
├── tests/
│   ├── e2e/                      # End-to-end tests
│   └── integration/              # Integration tests with envtest
├── docs/                         # User documentation
├── hack/                         # Build and code generation scripts
└── openspec/                     # OpenSpec feature specifications
```

## Directory Purposes

**`api/v1alpha1/`:**
- Purpose: Kubernetes custom resource definitions (CRDs) and types
- Contains: NodePool, AWSNodeClass, strategy config types, validation rules
- Key files: `nodepool_types.go`, `aws_nodeclass_types.go`, `config_types.go`, `strategy_types.go`

**`cmd/stratos/`:**
- Purpose: Controller binary entry point
- Contains: main() function with manager setup, flag parsing, reconciler registration
- Key files: `main.go`

**`internal/cloudprovider/`:**
- Purpose: Cloud provider abstraction and implementations
- Contains: CloudProvider interface definition
- Key files: `interface.go`, `types.go`

**`internal/cloudprovider/aws/`:**
- Purpose: AWS EC2 implementation of CloudProvider
- Contains: AWSProvider struct, LaunchInstance implementation, rate limiting
- Key files: `provider.go`, `ratelimit.go`, `nodeclass_controller.go`, `resolver.go`, `instance_types.go`, `userdata.go`

**`internal/cloudprovider/fake/`:**
- Purpose: Mock cloud provider for unit and integration testing
- Contains: FakeProvider with hooks for intercepting operations
- Key files: `provider.go`, `resolver.go`

**`internal/config/`:**
- Purpose: Configuration loading from environment variables and .env files
- Contains: Config struct, env loading with fallback chain
- Key files: `config.go`

**`internal/controller/`:**
- Purpose: Main NodePool reconciliation logic and orchestration
- Contains: NodePoolReconciler, reconciliation loop, node querying, cloud sync
- Key files: `reconciler.go`, `reconcile.go`, `setup.go`, `cloud_sync.go`, `maintenance.go`, `queries.go`, `providers.go`

**`internal/controller/nodestate/`:**
- Purpose: Node state constants, labels, annotations, and transition validation
- Contains: NodeState enum (warmup, standby, running, terminating), label constants
- Key files: `nodestate.go`

**`internal/controller/strategy/`:**
- Purpose: Pluggable scaling strategy implementations
- Contains: ScalingStrategy interface, KubernetesStrategy, GitHubActionsStrategy
- Key files: `interface.go`, `factory.go`, `kubernetes.go`, `githubactions.go`, `kubernetes_capacity.go`, `kubernetes_drain.go`, `kubernetes_network.go`, `kubernetes_events.go`

**`internal/controller/lifecycle/`:**
- Purpose: Node lifecycle operations and warmup monitoring
- Contains: Manager for node transitions, LaunchInstance coordination, warmup completion detection
- Key files: `manager.go`, `operations.go`, `warmup.go`

**`internal/github/`:**
- Purpose: GitHub API integration for GitHubActions scaling strategy
- Contains: Client for querying job queues
- Key files: `client.go`

**`internal/metrics/`:**
- Purpose: Prometheus metrics collection
- Contains: Metrics definitions and recording functions
- Key files: `metrics.go`

**`deploy/charts/stratos/`:**
- Purpose: Helm chart for deploying Stratos controller
- Contains: Deployment manifests, RBAC, service account
- Key files: `Chart.yaml`, `values.yaml`, `templates/`, `crds/`

**`examples/`:**
- Purpose: Example NodePool and AWSNodeClass resources
- Contains: Different configurations for AL2, Bottlerocket, GitHub Actions
- Subdirectories: `al2023-basic/`, `bottlerocket-basic/`, `production/`, `selectors/`

**`tests/integration/`:**
- Purpose: Integration tests using Kubernetes envtest (in-memory API server)
- Contains: Full controller tests with fake cloud provider
- Key files: `*_test.go` files (suite_test.go, nodepool_test.go, scale_up_test.go, scale_down_test.go, etc.)

**`tests/e2e/`:**
- Purpose: End-to-end tests against real Kubernetes cluster
- Contains: Scenario-based tests
- Key files: `e2e_test.go`, `helpers_test.go`

**`docs/`:**
- Purpose: User and developer documentation
- Contains: Architecture, API reference, setup guides, troubleshooting
- Subdirectories: `concepts/`, `guides/`, `reference/`, `development/`

**`hack/`:**
- Purpose: Build scripts and code generation
- Contains: Makefile, code generation helpers
- Key files: `Makefile` (at project root)

**`openspec/`:**
- Purpose: Feature specification and change tracking
- Contains: Specs for features (nodeclass, scaling, strategies)
- Subdirectories: `specs/`, `changes/`

## Key File Locations

**Entry Points:**
- `cmd/stratos/main.go`: Controller binary entry point with manager initialization
- `internal/controller/reconciler.go`: NodePoolReconciler.Reconcile() main loop
- `internal/controller/strategy/kubernetes.go`: KubernetesPodEventHandler for pod events
- `internal/cloudprovider/aws/nodeclass_controller.go`: AWSNodeClassReconciler for NodeClass resolution

**Configuration:**
- `internal/config/config.go`: Configuration loading from env vars and .env files
- `cmd/stratos/main.go`: CLI flags definition (metrics address, health probe, sync period, etc.)
- `deploy/charts/stratos/values.yaml`: Helm chart default values

**Core Logic:**
- `internal/controller/reconcile.go`: reconcileNodePool() main orchestration
- `internal/controller/strategy/kubernetes.go`: KubernetesStrategy implementation (pod demand, drain)
- `internal/controller/strategy/githubactions.go`: GitHubActionsStrategy implementation
- `internal/cloudprovider/aws/provider.go`: AWSProvider.LaunchInstance, StartInstance, StopInstance

**Node State Management:**
- `internal/controller/nodestate/nodestate.go`: NodeState enum, labels, transition rules
- `internal/controller/lifecycle/manager.go`: Node lifecycle Manager and hooks
- `internal/controller/lifecycle/operations.go`: State transition operations
- `internal/controller/lifecycle/warmup.go`: Warmup completion monitoring

**Cloud Provider:**
- `internal/cloudprovider/interface.go`: CloudProvider interface definition
- `internal/cloudprovider/aws/provider.go`: AWS EC2 implementation
- `internal/cloudprovider/aws/resolver.go`: AWSNodeClass field resolution (subnets, IAM role)
- `internal/cloudprovider/aws/userdata.go`: User data script generation
- `internal/cloudprovider/aws/ratelimit.go`: AWS API rate limiting

**Testing:**
- `tests/integration/suite_test.go`: Integration test setup with envtest
- `tests/integration/nodepool_test.go`: NodePool lifecycle tests
- `tests/integration/scale_up_test.go`: Scale-up behavior tests
- `tests/integration/scale_down_test.go`: Scale-down behavior tests
- `internal/cloudprovider/fake/provider.go`: Fake cloud provider for testing

**API/CRDs:**
- `api/v1alpha1/nodepool_types.go`: NodePool CRD definition
- `api/v1alpha1/aws_nodeclass_types.go`: AWSNodeClass CRD definition
- `api/v1alpha1/strategy_types.go`: ScalingStrategy enum and config types
- `api/v1alpha1/zz_generated.deepcopy.go`: Generated deepcopy methods (DO NOT EDIT)

## Naming Conventions

**Files:**
- Controllers: `*_controller.go` or `reconciler.go` (e.g., `nodeclass_controller.go`)
- Tests: `*_test.go` (e.g., `nodepool_test.go`)
- Types: `*_types.go` (e.g., `nodepool_types.go`)
- Generated: `zz_generated.*.go` (never edit)
- Implementation files: descriptive names like `cloud_sync.go`, `maintenance.go`, `userdata.go`

**Directories:**
- Package names match directory names
- Internal packages under `internal/`
- API types under `api/v1alpha1/`
- Cloud providers grouped under `internal/cloudprovider/`
- Controllers grouped under `internal/controller/`

**Functions:**
- Public: PascalCase (e.g., `Reconcile`, `StartInstance`, `CheckDemand`)
- Private: camelCase (e.g., `reconcileNodePool`, `syncNodesWithCloud`, `countNodesByState`)
- Test helpers: camelCase with `test` prefix or suffix (e.g., `createTestNodePool`)

**Types:**
- PascalCase (e.g., `NodePoolReconciler`, `AWSProvider`, `ScalingStrategy`)
- Interfaces: Names end with "er" or "or" (e.g., `CloudProvider`, `ScalingStrategy`)

**Variables:**
- camelCase (e.g., `nodePool`, `standbyCount`, `provider`)
- Constants: ALL_CAPS_WITH_UNDERSCORES for unexported, PascalCase for exported (e.g., `LabelPool`, `defaultReconcileInterval`)

## Where to Add New Code

**New Feature (scaling behavior, pool lifecycle):**
- Primary code: Add methods to `internal/controller/` or create new strategy in `internal/controller/strategy/`
- Tests: `tests/integration/feature_name_test.go`
- API changes: Update `api/v1alpha1/nodepool_types.go` or `api/v1alpha1/config_types.go`
- Metrics: Add recording in `internal/metrics/metrics.go`

**New Cloud Provider (e.g., GCP):**
- Create: `internal/cloudprovider/gcp/` directory
- Implement: `provider.go` (CloudProvider interface), `types.go` (GCP-specific types)
- Register: Add to controller setup in `cmd/stratos/main.go` and `internal/controller/providers.go`
- CRD: Add `api/v1alpha1/gcp_nodeclass_types.go` and reconciler in `internal/cloudprovider/gcp/nodeclass_controller.go`

**New Scaling Strategy:**
- Create: `internal/controller/strategy/newstrategy.go`
- Implement: ScalingStrategy interface (CheckDemand, OnScaleUp, FindScaleDownCandidates, etc.)
- Register: Add to factory in `internal/controller/strategy/factory.go`
- CRD: Update `api/v1alpha1/nodepool_types.go` with new strategy type and config struct
- Tests: `internal/controller/strategy/newstrategy_test.go`

**New Kubernetes Node Hook (e.g., custom taint removal):**
- Implement: NodeHooks interface in strategy (PrepareForRunning, PrepareForStandby, IsReady)
- Location: `internal/controller/strategy/kubernetes.go` methods
- Tests: Add cases to `internal/controller/strategy/kubernetes_test.go`

**Utilities/Helpers:**
- Shared helpers: `internal/controller/queries.go` (for node queries), `internal/controller/validate.go` (for validation)
- Provider-specific: Under respective provider directory (e.g., `internal/cloudprovider/aws/instance_types.go`)

## Special Directories

**`api/v1alpha1/`:**
- Purpose: Kubernetes CRD types (required for controller-runtime)
- Generated: `zz_generated.deepcopy.go` (DO NOT EDIT - run `make generate` after changes)
- Committed: Yes (except generated files)

**`deploy/charts/stratos/crds/`:**
- Purpose: Generated Helm CRD manifests
- Generated: Yes (run `make manifests` after API changes)
- Committed: Yes
- Note: Sync with `api/v1alpha1/` via code generation

**`tests/`:**
- Purpose: Test code separate from source
- Integration tests: Use envtest (in-memory Kubernetes)
- E2E tests: Require real cluster
- Committed: Yes

**`openspec/`:**
- Purpose: Feature specification and change tracking
- Changed: Yes (new features documented here before implementation)
- Committed: Yes
- Note: Reference for understanding rationale behind features

**`examples/`:**
- Purpose: Reference configurations for users
- Committed: Yes
- Note: Keep updated when API changes

**`docs/`:**
- Purpose: User and developer documentation
- Built: Docusaurus (generates HTML from markdown)
- Committed: Yes (markdown source)
- Note: Update when feature/API changes

**`hack/`:**
- Purpose: Build and development scripts
- Committed: Yes
- Note: Makefile controls code generation, testing, linting

---

*Structure analysis: 2026-02-02*
