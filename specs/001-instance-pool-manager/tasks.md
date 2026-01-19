# Tasks: Stratos - Kubernetes Node Scaler

**Input**: Design documents from `/specs/001-instance-pool-manager/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are NOT explicitly requested in the spec. Test tasks are excluded from this task list.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Based on plan.md, this is a Kubernetes operator following kubebuilder conventions:
- `cmd/stratos/` - Entry point
- `api/v1alpha1/` - CRD types
- `internal/` - Implementation packages
- `config/` - Kubernetes manifests

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [ ] T001 Initialize Go module with `go mod init github.com/stratos-sh/stratos` at repository root
- [ ] T002 Create project directory structure per plan.md (cmd/, api/, internal/, config/)
- [ ] T003 [P] Configure golangci-lint with `.golangci.yml` per research.md
- [ ] T004 [P] Create Makefile with targets: build, test, lint, generate, manifests
- [ ] T005 [P] Create hack/boilerplate.go.txt for code generation headers
- [ ] T006 Add primary dependencies to go.mod: controller-runtime, client-go, aws-sdk-go-v2, prometheus/client_golang

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

### CRD Types and Code Generation

- [ ] T007 Create api/v1alpha1/groupversion_info.go with API group registration (stratos.sh/v1alpha1)
- [ ] T008 Create api/v1alpha1/nodepool_types.go with NodePoolSpec, NodePoolStatus, NodeTemplate structs per data-model.md
- [ ] T009 Create api/v1alpha1/cloudprovider_types.go with CloudProviderConfig, AWSConfig, BlockDeviceMapping structs
- [ ] T010 Create api/v1alpha1/config_types.go with ScaleDownConfig, PreWarmConfig, TimeoutAction
- [ ] T011 Run controller-gen to generate api/v1alpha1/zz_generated.deepcopy.go
- [ ] T012 Run controller-gen to generate config/crd/bases/stratos.sh_nodepools.yaml

### CloudProvider Interface

- [ ] T013 Create internal/cloudprovider/interface.go with CloudProvider interface per contracts/cloudprovider-interface.go
- [ ] T014 Create internal/cloudprovider/types.go with Instance, InstanceState, LaunchConfig, BlockDevice, error types
- [ ] T015 [P] Create internal/cloudprovider/fake/provider.go with FakeProvider for testing

### NodeState Management

- [ ] T016 Create internal/nodemanager/state.go with NodeState enum (warmup, standby, running, terminating) and transitions

### Metrics Infrastructure

- [ ] T017 Create internal/metrics/metrics.go with Prometheus metric definitions per research.md (gauges, counters, histograms)

### Controller Entry Point

- [ ] T018 Create cmd/stratos/main.go with manager setup, scheme registration, and controller startup
- [ ] T019 Add leader election and health checks to cmd/stratos/main.go

### RBAC Manifests

- [ ] T020 [P] Create config/rbac/service_account.yaml per contracts/rbac.yaml
- [ ] T021 [P] Create config/rbac/role.yaml with ClusterRole per contracts/rbac.yaml
- [ ] T022 [P] Create config/rbac/role_binding.yaml with ClusterRoleBinding per contracts/rbac.yaml
- [ ] T023 [P] Create config/manager/manager.yaml with controller Deployment

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Create and Manage NodePool (Priority: P1)

**Goal**: Kubernetes operators can create a NodePool resource that defines pre-warmed node capacity

**Independent Test**: Create a NodePool with poolSize=10 and minStandby=5, verify Stratos accepts it and begins pre-warming

### Implementation for User Story 1

- [ ] T024 [US1] Create internal/controller/nodepool_controller.go with NodePoolReconciler struct and SetupWithManager
- [ ] T025 [US1] Implement Reconcile method in internal/controller/nodepool_controller.go with finalizer pattern per research.md
- [ ] T026 [US1] Add CRD validation webhook logic in api/v1alpha1/nodepool_webhook.go (minStandby <= poolSize)
- [ ] T027 [US1] Implement status update logic to set conditions (Ready, Reconciling, Degraded) in internal/controller/nodepool_controller.go
- [ ] T028 [US1] Implement NodePool deletion cleanup (drain and terminate all nodes) in internal/controller/nodepool_controller.go
- [ ] T029 [US1] Add Kubernetes events emission for NodePool create/update/delete in internal/controller/nodepool_controller.go
- [ ] T030 [US1] Update metrics (stratos_nodepool_nodes_total, stratos_nodepool_desired_standby, stratos_nodepool_pool_size) in internal/controller/nodepool_controller.go

**Checkpoint**: User Story 1 complete - NodePool CRD can be created, validated, and deleted

---

## Phase 4: User Story 2 - Node Pre-warming Lifecycle (Priority: P1)

**Goal**: Nodes launch, join the cluster, pull images, and self-stop to become pre-warmed standby

**Independent Test**: Create a NodePool, verify nodes join cluster, execute initialization, and self-stop

### Implementation for User Story 2

- [ ] T031 [US2] Create internal/cloudprovider/aws/provider.go with AWSProvider struct implementing CloudProvider interface
- [ ] T032 [US2] Implement LaunchInstance in internal/cloudprovider/aws/provider.go with EC2 RunInstances API
- [ ] T033 [US2] Implement GetInstanceState, GetInstance, ListInstances in internal/cloudprovider/aws/provider.go
- [ ] T034 [US2] Implement UpdateInstanceTags in internal/cloudprovider/aws/provider.go
- [ ] T035 [US2] Create internal/cloudprovider/aws/ratelimit.go with client-side rate limiting and exponential backoff per research.md
- [ ] T036 [US2] Create internal/nodemanager/manager.go with NodeManager struct for node lifecycle operations
- [ ] T037 [US2] Implement LaunchNode in internal/nodemanager/manager.go (create instance, wait for Node object, apply labels)
- [ ] T038 [US2] Implement pre-warm timeout monitoring in internal/nodemanager/manager.go (detect self-stop or apply timeout action)
- [ ] T039 [US2] Implement node labeling with stratos.sh/pool, stratos.sh/state, stratos.sh/instance-id in internal/nodemanager/manager.go
- [ ] T040 [US2] Add warmup → standby state transition when instance self-stops in internal/nodemanager/manager.go
- [ ] T041 [US2] Implement timeout action (stop or terminate) when instance fails to self-stop in internal/nodemanager/manager.go
- [ ] T042 [US2] Update metrics (stratos_warmup_duration_seconds, stratos_warmup_failures_total) in internal/nodemanager/manager.go
- [ ] T043 [US2] Emit Kubernetes events for warmup started, warmup completed, warmup timeout in internal/nodemanager/manager.go

**Checkpoint**: User Story 2 complete - Nodes are pre-warmed and transition to standby

---

## Phase 5: User Story 3 - Automatic Scale-Up (Priority: P1)

**Goal**: Stratos automatically starts pre-warmed nodes when pods are pending

**Independent Test**: Deploy pods requiring more capacity, verify Stratos starts standby nodes and pods get scheduled

### Implementation for User Story 3

- [ ] T044 [US3] Create internal/controller/pod_watcher.go with pod event handler for unschedulable pods
- [ ] T045 [US3] Implement isPodUnschedulable predicate in internal/controller/pod_watcher.go
- [ ] T046 [US3] Implement findMatchingNodePools in internal/controller/pod_watcher.go (match pod requirements to NodePool)
- [ ] T047 [US3] Add Watches for Pods in SetupWithManager in internal/controller/nodepool_controller.go with EnqueueRequestsFromMapFunc
- [ ] T048 [US3] Implement StartInstance in internal/cloudprovider/aws/provider.go with EC2 StartInstances API
- [ ] T049 [US3] Implement scale-up decision logic in internal/controller/nodepool_controller.go (calculate nodes needed vs standby available)
- [ ] T050 [US3] Implement StartNode in internal/nodemanager/manager.go (start instance, wait for Node Ready, update labels)
- [ ] T051 [US3] Add standby → running state transition in internal/nodemanager/manager.go
- [ ] T052 [US3] Enforce poolSize limit (don't start if standby + running >= poolSize) in internal/controller/nodepool_controller.go
- [ ] T053 [US3] Update metrics (stratos_scaleup_total, stratos_scaleup_duration_seconds) in internal/controller/nodepool_controller.go
- [ ] T054 [US3] Emit Kubernetes events for scale-up triggered, node started in internal/controller/nodepool_controller.go

**Checkpoint**: User Story 3 complete - Pending pods trigger automatic scale-up

---

## Phase 6: User Story 4 - Automatic Scale-Down (Priority: P1)

**Goal**: Stratos automatically stops empty nodes and returns them to standby

**Independent Test**: Scale down a deployment, verify nodes become empty and Stratos drains/stops them

### Implementation for User Story 4

- [ ] T055 [US4] Create internal/nodemanager/drain.go with DrainHelper wrapper using k8s.io/kubectl/pkg/drain per research.md
- [ ] T056 [US4] Implement CordonNode in internal/nodemanager/drain.go
- [ ] T057 [US4] Implement DrainNode in internal/nodemanager/drain.go with PDB respect and configurable timeout
- [ ] T058 [US4] Implement StopInstance in internal/cloudprovider/aws/provider.go with EC2 StopInstances API
- [ ] T059 [US4] Implement empty node detection in internal/controller/nodepool_controller.go (no pods excluding DaemonSets)
- [ ] T060 [US4] Implement emptyNodeTTL tracking in internal/controller/nodepool_controller.go (annotate scale-down-candidate-since)
- [ ] T061 [US4] Implement StopNode in internal/nodemanager/manager.go (cordon, drain, stop instance)
- [ ] T062 [US4] Add running → terminating → standby state transition in internal/nodemanager/manager.go
- [ ] T063 [US4] Implement scale-down disabled check (skip if scaleDown.enabled=false) in internal/controller/nodepool_controller.go
- [ ] T064 [US4] Update metrics (stratos_scaledown_total, stratos_drain_duration_seconds) in internal/controller/nodepool_controller.go
- [ ] T065 [US4] Emit Kubernetes events for scale-down started, node drained, node stopped in internal/nodemanager/manager.go

**Checkpoint**: User Story 4 complete - Empty nodes are drained and returned to standby

---

## Phase 7: User Story 5 - NodePool Reconciliation (Priority: P1)

**Goal**: Stratos continuously reconciles actual node state with desired NodePool state

**Independent Test**: Manually terminate standby instances, verify Stratos detects and provisions replacements

### Implementation for User Story 5

- [ ] T066 [US5] Implement periodic reconciliation loop in internal/controller/nodepool_controller.go with configurable interval
- [ ] T067 [US5] Implement pool health check: count warmup, standby, running nodes per NodePool in internal/controller/nodepool_controller.go
- [ ] T068 [US5] Implement minStandby replenishment: provision nodes when standby < minStandby in internal/controller/nodepool_controller.go
- [ ] T069 [US5] Implement external termination detection: sync Node objects with cloud instance state in internal/controller/nodepool_controller.go
- [ ] T070 [US5] Implement stale Node cleanup: delete Node objects whose instances are terminated in internal/nodemanager/manager.go
- [ ] T071 [US5] Implement TerminateInstance in internal/cloudprovider/aws/provider.go with EC2 TerminateInstances API
- [ ] T072 [US5] Update NodePool status (warmup, standby, running, total counts) in internal/controller/nodepool_controller.go
- [ ] T073 [US5] Update lastReconcileTime in NodePool status in internal/controller/nodepool_controller.go

**Checkpoint**: User Story 5 complete - Pool self-heals and maintains desired state

---

## Phase 8: User Story 6 - Cloud Provider Support (Priority: P1)

**Goal**: Stratos works with cloud providers (AWS EC2 first)

**Independent Test**: Create a NodePool with AWS configuration, verify EC2 instances are created correctly

### Implementation for User Story 6

- [ ] T074 [US6] Implement cloud provider factory in internal/cloudprovider/factory.go (return provider based on config)
- [ ] T075 [US6] Add AWS credentials loading in internal/cloudprovider/aws/provider.go using config.LoadDefaultConfig
- [ ] T076 [US6] Implement instance tagging with managed-by, stratos.sh/pool, stratos.sh/cluster, stratos.sh/state in internal/cloudprovider/aws/provider.go
- [ ] T077 [US6] Implement subnet selection (round-robin across configured subnets) in internal/cloudprovider/aws/provider.go
- [ ] T078 [US6] Add cloud provider error handling with specific error types per contracts/cloudprovider-interface.go
- [ ] T079 [US6] Add retry logic with exponential backoff for throttling errors in internal/cloudprovider/aws/ratelimit.go

**Checkpoint**: User Story 6 complete - AWS EC2 cloud provider is fully functional

---

## Phase 9: User Story 7 - Graceful Controller Shutdown (Priority: P2)

**Goal**: Stratos shuts down gracefully without orphaned resources

**Independent Test**: Restart Stratos controller during active operations, verify all nodes are accounted for after restart

### Implementation for User Story 7

- [ ] T080 [US7] Implement graceful shutdown handler in cmd/stratos/main.go (SIGTERM handling)
- [ ] T081 [US7] Add context cancellation propagation to all operations in internal/controller/nodepool_controller.go
- [ ] T082 [US7] Implement in-progress operation timeout on shutdown in cmd/stratos/main.go
- [ ] T083 [US7] Implement state recovery on startup in internal/controller/nodepool_controller.go (reconcile all NodePools)

**Checkpoint**: User Story 7 complete - Controller handles restart gracefully

---

## Phase 10: User Story 8 - Maximum Node Runtime (Priority: P2)

**Goal**: Nodes running longer than maxNodeRuntime are automatically recycled

**Independent Test**: Configure maxNodeRuntime, run node beyond limit, verify Stratos recycles it

### Implementation for User Story 8

- [ ] T084 [US8] Implement maxNodeRuntime tracking in internal/controller/nodepool_controller.go (check last-started annotation)
- [ ] T085 [US8] Implement warning event when node approaches maxNodeRuntime threshold in internal/controller/nodepool_controller.go
- [ ] T086 [US8] Trigger scale-down for nodes exceeding maxNodeRuntime in internal/controller/nodepool_controller.go
- [ ] T087 [US8] Skip maxNodeRuntime check when disabled (maxNodeRuntime=0 or nil) in internal/controller/nodepool_controller.go

**Checkpoint**: User Story 8 complete - Long-running nodes are recycled automatically

---

## Phase 11: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T088 [P] Create config/samples/nodepool_sample.yaml with example NodePool per quickstart.md
- [ ] T089 [P] Create config/default/kustomization.yaml for full deployment
- [ ] T090 [P] Add Dockerfile for controller image at repository root
- [ ] T091 Code cleanup: ensure consistent error handling across all packages
- [ ] T092 Add comprehensive logging throughout controller and node manager
- [ ] T093 Run quickstart.md validation: verify all commands work

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-10)**: All depend on Foundational phase completion
  - US1 (NodePool CRUD) - Can start immediately after Foundational
  - US2 (Pre-warming) - Can start immediately after Foundational
  - US3 (Scale-Up) - Depends on US2 (needs cloud provider and node manager)
  - US4 (Scale-Down) - Can start immediately after Foundational
  - US5 (Reconciliation) - Depends on US1-US4 (integrates all operations)
  - US6 (Cloud Provider) - Can be built alongside US2-US5 (fills out implementation)
  - US7 (Graceful Shutdown) - Depends on US1-US5 (needs working controller)
  - US8 (Max Runtime) - Depends on US4 (uses scale-down mechanism)
- **Polish (Phase 11)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - No dependencies on other stories
- **User Story 2 (P1)**: Can start after Foundational - Creates cloud provider and node manager used by US3-US5
- **User Story 3 (P1)**: Depends on US2 (needs StartInstance, StartNode)
- **User Story 4 (P1)**: Can start after Foundational - Creates drain logic used by US8
- **User Story 5 (P1)**: Integrates US1-US4, should be implemented last among P1 stories
- **User Story 6 (P1)**: Fills out AWS provider, can be developed alongside US2-US5
- **User Story 7 (P2)**: Requires working controller from US1-US5
- **User Story 8 (P2)**: Requires drain logic from US4

### Within Each User Story

- Models/types before logic
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- US1 and US2 can be worked in parallel (different packages)
- US4 can be worked in parallel with US2/US3 (different focus areas)
- All Polish tasks marked [P] can run in parallel

---

## Parallel Example: Phase 2 Foundational

```bash
# Launch these RBAC tasks in parallel:
Task: "Create config/rbac/service_account.yaml"
Task: "Create config/rbac/role.yaml"
Task: "Create config/rbac/role_binding.yaml"
Task: "Create config/manager/manager.yaml"
```

## Parallel Example: User Story 2 + User Story 4

```bash
# These can be developed in parallel by different developers:
Developer A: User Story 2 (Pre-warming) - cloud provider, node launch
Developer B: User Story 4 (Scale-Down) - drain operations
```

---

## Implementation Strategy

### MVP First (P1 User Stories Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (NodePool CRUD)
4. Complete Phase 4: User Story 2 (Pre-warming)
5. Complete Phase 5: User Story 3 (Scale-Up)
6. Complete Phase 6: User Story 4 (Scale-Down)
7. Complete Phase 7: User Story 5 (Reconciliation)
8. Complete Phase 8: User Story 6 (Cloud Provider)
9. **STOP and VALIDATE**: Test complete P1 functionality
10. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add US1 (NodePool CRUD) → Can create/delete NodePools
3. Add US2 (Pre-warming) + US6 (Cloud Provider) → Nodes pre-warm on AWS
4. Add US3 (Scale-Up) → Pending pods trigger scale-up
5. Add US4 (Scale-Down) → Empty nodes return to standby
6. Add US5 (Reconciliation) → Pool self-heals
7. Add US7 (Graceful Shutdown) → Production-ready restarts
8. Add US8 (Max Runtime) → Node recycling

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (NodePool CRUD) + User Story 5 (Reconciliation)
   - Developer B: User Story 2 (Pre-warming) + User Story 3 (Scale-Up)
   - Developer C: User Story 4 (Scale-Down) + User Story 6 (Cloud Provider)
3. Stories complete and integrate incrementally

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- AWS is the only cloud provider in scope for v1
- Tests not included - add test tasks if TDD approach is requested
