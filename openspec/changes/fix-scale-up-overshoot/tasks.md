# Tasks: Fix Scale-Up Overshoot

**Change ID**: fix-scale-up-overshoot
**Created**: 2026-01-21
**Updated**: 2026-01-21

## Implementation Tasks

### Phase 1: API Changes

- [ ] **T1: Add ScaleUpConfig to NodePool API**
  - File: `api/v1alpha1/nodepool_types.go`
  - Add `ScaleUp *ScaleUpConfig` field to `NodePoolSpec`
  - Add `ScaleUpConfig` struct with `DefaultPodResources`
  - Run `make generate && make manifests`
  - Validation: CRD includes new field

- [ ] **T2: Add annotation constant and TTL**
  - File: `internal/nodemanager/labels.go`
  - Add `AnnotationScaleUpStarted = "stratos.sh/scale-up-started"`
  - Add `ScaleUpStartedTTL = 60 * time.Second`
  - Validation: Constants compile

### Phase 2: AWS Instance Type Mapping

- [ ] **T3: Create AWS instance type capacity mapping**
  - File: `internal/cloudprovider/aws/instance_types.go`
  - Add `InstanceCapacity` struct with CPU/Memory
  - Add `awsInstanceCapacity` map with common instance types
  - Add `GetInstanceCapacity(instanceType string)` function
  - Include: m5, m6i, c5, c6i, r5, p3, g4dn, g5 families
  - Validation: Unit tests for known/unknown instance types

### Phase 3: Resource Calculator

- [ ] **T4: Create ScaleCalculator**
  - File: `internal/controller/scale_calculator.go`
  - Implement `ScaleCalculator` struct
  - Implement `CalculateNodesNeeded(pods []corev1.Pod, existingNodes []corev1.Node) int`
  - Implement `sumPodRequests()` - sum CPU/memory from all containers
  - Implement `getDefaultResources()` - read from NodePool config
  - Implement `getNodeCapacity()` - hybrid lookup (existing node → static mapping)
  - Apply 80% capacity factor (`NodeCapacityUsagePercent = 0.80`)
  - Validation: Unit tests for various scenarios

- [ ] **T5: Unit tests for ScaleCalculator**
  - File: `internal/controller/scale_calculator_test.go`
  - Test: 10 pods @ 500m CPU on m5.xlarge → 2 nodes
  - Test: 1 pod @ 100m CPU on m5.xlarge → 1 node
  - Test: Pods without requests + defaults → uses defaults
  - Test: Unknown instance type → falls back to 1:1
  - Test: Mixed pods (some with, some without requests)

### Phase 4: In-Flight Tracking

- [ ] **T6: Update StartNode to set annotation**
  - File: `internal/nodemanager/manager.go`
  - Modify `StartNode()` to set `stratos.sh/scale-up-started` annotation
  - Validation: Unit test confirms annotation is set

- [ ] **T7: Add countStartingNodes function**
  - File: `internal/controller/nodepool_controller.go`
  - Add `countStartingNodes(ctx, poolName)` function
  - Count nodes with annotation within TTL that are not Ready
  - Validation: Unit tests with various annotation ages

- [ ] **T8: Add isNodeReady helper**
  - File: `internal/controller/nodepool_controller.go`
  - Add `isNodeReady(node *corev1.Node) bool`
  - Check NodeReady condition status
  - Validation: Unit test

### Phase 5: Update Scale-Up Logic

- [ ] **T9: Update calculateScaleUpNeeded**
  - File: `internal/controller/nodepool_controller.go`
  - Use `ScaleCalculator` for resource-based calculation
  - Subtract `countStartingNodes()` from needed count
  - Add detailed logging for debugging
  - Validation: Unit tests confirm correct calculation

- [ ] **T10: Add clearStaleScaleUpAnnotations function**
  - File: `internal/controller/nodepool_controller.go`
  - Clear annotations when: node is Ready OR past TTL
  - Validation: Unit test confirms cleanup behavior

- [ ] **T11: Call cleanup in reconciliation loop**
  - File: `internal/controller/nodepool_controller.go`
  - Call `clearStaleScaleUpAnnotations()` during reconciliation
  - Validation: Integration test confirms annotations are cleared

### Phase 6: Metrics & Observability

- [ ] **T12: Add starting nodes metric**
  - File: `internal/metrics/metrics.go`
  - Add gauge for starting nodes count per pool
  - Validation: Metric exposed at /metrics endpoint

- [ ] **T13: Update logging**
  - File: `internal/controller/nodepool_controller.go`
  - Log: pending pods count, calculated need, starting nodes, final decision
  - Validation: Log output includes all relevant info

### Phase 7: Integration Testing

- [ ] **T14: Integration test for resource-based scale-up**
  - File: `internal/controller/integration_test.go`
  - Test: 10 small pods → creates ~2-3 nodes (not 10)
  - Test: Single large pod → creates 1 node
  - Requires: envtest setup

- [ ] **T15: Integration test for in-flight tracking**
  - File: `internal/controller/integration_test.go`
  - Test: Rapid reconciliation doesn't over-provision
  - Test: Annotation cleared when node becomes Ready

### Phase 8: Documentation

- [ ] **T16: Update sample NodePool**
  - File: `config/samples/stratos_v1alpha1_nodepool.yaml`
  - Add `scaleUp.defaultPodResources` example
  - Validation: Sample is valid YAML

- [ ] **T17: Update CHANGELOG**
  - Document: bug fix, resource-based calculation, new config options
  - Validation: CHANGELOG has entry

## Task Dependencies

```
T1 ──────────────────────────────────────┐
                                         │
T2 ─────► T6 ─────► T7 ─────► T9 ◄───────┤
                         │               │
T3 ─────► T4 ─────► T5 ──┴───► T9        │
                                │        │
T8 ─────────────────────────────┘        │
                                         │
T9 ─────► T10 ─────► T11 ◄───────────────┘
     │
     └────► T12 ─────► T13

T14, T15 depend on T11

T16, T17 can be done after T11
```

## Parallelizable Work

These can be done in parallel:
- T1 (API) + T2 (constants) + T3 (AWS instance types)
- T4 (calculator) + T6 (StartNode annotation) + T8 (isNodeReady)
- T14 (integration) + T16 (samples) + T17 (changelog)

## Verification

After implementation:

1. **Unit tests:**
   ```bash
   go test -v ./internal/controller/... -run ScaleCalculator
   go test -v ./internal/controller/... -run StartingNodes
   go test -v ./internal/cloudprovider/aws/... -run InstanceCapacity
   ```

2. **Integration tests:**
   ```bash
   make test-integration
   ```

3. **Manual test:**
   ```bash
   # Start controller
   go run ./cmd/stratos/main.go --cluster-name=test --cloud-provider=fake

   # Create NodePool with scaleUp config
   kubectl apply -f config/samples/stratos_v1alpha1_nodepool.yaml

   # Create 10 small pods
   for i in {1..10}; do
     kubectl run test-pod-$i --image=nginx --requests='cpu=100m,memory=128Mi'
   done

   # Verify: should start ~2 nodes, not 10
   kubectl get nodes -l stratos.sh/pool=test-pool
   ```

4. **Check metrics:**
   ```bash
   curl localhost:8080/metrics | grep stratos_nodepool_nodes
   ```

## Estimated Complexity

| Phase | Tasks | Complexity |
|-------|-------|------------|
| 1. API Changes | T1-T2 | Low |
| 2. Instance Types | T3 | Low |
| 3. Calculator | T4-T5 | Medium |
| 4. In-Flight Tracking | T6-T8 | Medium |
| 5. Scale-Up Logic | T9-T11 | Medium |
| 6. Metrics | T12-T13 | Low |
| 7. Integration | T14-T15 | Medium |
| 8. Documentation | T16-T17 | Low |
