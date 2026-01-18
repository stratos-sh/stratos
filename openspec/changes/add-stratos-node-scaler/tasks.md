# Tasks: Add Stratos Node Scaler

## 1. Project Setup

- [ ] 1.1 Initialize Go project with go.mod (github.com/stratossh/stratos)
- [ ] 1.2 Set up controller-runtime dependency
- [ ] 1.3 Configure golangci-lint

## 2. NodePool CRD

- [ ] 2.1 Define NodePool API types (api/v1alpha1/nodepool_types.go)
- [ ] 2.2 Implement CRD validation (poolSize >= minStandby, required fields)
- [ ] 2.3 Write CRD manifest YAML (hand-written, can add controller-gen later)
- [ ] 2.4 Write unit tests for NodePool validation
- [ ] 2.5 Create sample NodePool manifests for testing

## 3. Cloud Provider Interface

- [ ] 3.1 Define CloudProvider interface (Launch, Start, Stop, GetState, Terminate)
- [ ] 3.2 Define InstanceSpec and InstanceState types
- [ ] 3.3 Implement AWS EC2 provider
- [ ] 3.4 Implement instance tagging (NodePool name, cluster ID, managed marker)
- [ ] 3.5 Implement retry logic with exponential backoff
- [ ] 3.6 Write integration tests with localstack or mocks

## 4. Node State Manager

- [ ] 4.1 Define NodeState enum (Warmup, Running, Standby)
- [ ] 4.2 Implement node label management (stratos.sh/managed, stratos.sh/nodepool, stratos.sh/state)
- [ ] 4.3 Implement node-to-instance mapping (via annotations or external store)
- [ ] 4.4 Implement state transition logic
- [ ] 4.5 Write unit tests for state transitions

## 5. NodePool Controller

- [ ] 5.1 Implement NodePool controller with controller-runtime
- [ ] 5.2 Implement Reconcile loop for NodePool resources
- [ ] 5.3 Implement NodePool deletion handling (drain and terminate all nodes)
- [ ] 5.4 Implement NodePool update handling (reconcile to new desired state)
- [ ] 5.5 Write controller unit tests

## 6. Node Pre-warming

- [ ] 6.1 Implement instance launch flow with userdata injection
- [ ] 6.2 Implement warmup monitoring (watch for Node creation and instance stop)
- [ ] 6.3 Implement warmup timeout handling (stop or terminate based on config)
- [ ] 6.4 Implement stuck instance detection (secondary timeout for stopping state)
- [ ] 6.5 Write integration tests for pre-warming lifecycle

## 7. Scale-Up (Pod Watcher)

- [ ] 7.1 Implement Pod informer watching for unschedulable pods
- [ ] 7.2 Implement pod matching against NodePool requirements (selectors, tolerations)
- [ ] 7.3 Implement capacity calculation (how many nodes needed)
- [ ] 7.4 Implement standby node selection and start
- [ ] 7.5 Implement poolSize limit enforcement
- [ ] 7.6 Write integration tests for scale-up scenarios

## 8. Scale-Down

- [ ] 8.1 Implement empty node detection (exclude DaemonSet pods)
- [ ] 8.2 Implement emptyNodeTTL tracking
- [ ] 8.3 Implement node cordon before drain
- [ ] 8.4 Implement PDB-respecting drain
- [ ] 8.5 Implement drain timeout handling
- [ ] 8.6 Implement instance stop after drain
- [ ] 8.7 Implement scale-down disable toggle
- [ ] 8.8 Write integration tests for scale-down scenarios

## 9. Pool Reconciliation

- [ ] 9.1 Implement periodic reconciliation loop (configurable interval)
- [ ] 9.2 Implement standby replenishment logic
- [ ] 9.3 Implement external termination detection
- [ ] 9.4 Implement stale Node cleanup
- [ ] 9.5 Write integration tests for reconciliation

## 10. Node Runtime Limits

- [ ] 10.1 Implement maxNodeRuntime tracking per node
- [ ] 10.2 Implement automatic recycling when limit exceeded
- [ ] 10.3 Implement warning event emission approaching limit
- [ ] 10.4 Write unit tests for runtime limit logic

## 11. Observability

- [ ] 11.1 Define Prometheus metrics (pool size, standby count, running count, warmup count)
- [ ] 11.2 Implement scale-up metrics (count, latency)
- [ ] 11.3 Implement scale-down metrics (count, drain duration)
- [ ] 11.4 Implement warmup failure metrics
- [ ] 11.5 Set up metrics HTTP endpoint
- [ ] 11.6 Implement Kubernetes event emission for operations
- [ ] 11.7 Create Grafana dashboard JSON

## 12. RBAC & Security

- [ ] 12.1 Define ClusterRole with least-privilege permissions
- [ ] 12.2 Create ServiceAccount and ClusterRoleBinding
- [ ] 12.3 Document required cloud provider IAM permissions

## 13. Deployment

- [ ] 13.1 Create Helm chart structure
- [ ] 13.2 Add values.yaml with configurable options
- [ ] 13.3 Add templates for Deployment, RBAC, CRD
- [ ] 13.4 Create kustomize base as alternative
- [ ] 13.5 Write installation documentation

## 14. Controller Lifecycle

- [ ] 14.1 Implement leader election for HA
- [ ] 14.2 Implement graceful shutdown (SIGTERM handling)
- [ ] 14.3 Implement in-flight operation completion on shutdown
- [ ] 14.4 Write tests for controller restart recovery

## 15. Documentation

- [ ] 15.1 Write README with quick start guide
- [ ] 15.2 Document NodePool configuration options
- [ ] 15.3 Document cloud provider setup (AWS IAM, credentials)
- [ ] 15.4 Write troubleshooting guide
- [ ] 15.5 Document metrics and alerting recommendations

## Dependencies

- Tasks 3.x (Cloud Provider) can run in parallel with 2.x (CRD)
- Tasks 5.x (Controller) depend on 2.x and 3.x
- Tasks 6.x (Pre-warming) depend on 5.x
- Tasks 7.x (Scale-Up) depend on 4.x and 6.x
- Tasks 8.x (Scale-Down) depend on 4.x and 5.x
- Tasks 9.x (Reconciliation) depend on 6.x, 7.x, 8.x
- Tasks 10.x-15.x can proceed after core functionality (9.x) is complete
