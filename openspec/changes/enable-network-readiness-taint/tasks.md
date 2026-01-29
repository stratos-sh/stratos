## 1. CRD Type Changes

- [ ] 1.1 Add `EnableNetworkReadinessTaint *bool` field to `NodeTemplate` in `api/v1alpha1/nodepool_types.go` with kubebuilder default `true` and JSON tag `enableNetworkReadinessTaint`
- [ ] 1.2 Remove `StartupTaintRemoval` field from `NodeTemplate` and delete the `StartupTaintRemovalMode` type and its constants
- [ ] 1.3 Update `startupTaints` field comment to clarify these are custom taints managed externally (Stratos applies but never removes)
- [ ] 1.4 Run `make generate` and `make manifests` to regenerate deepcopy and CRD YAML

## 2. Controller Logic

- [ ] 2.1 Add helper function `isNetworkReadinessTaintEnabled(pool) bool` that returns true when the field is nil or explicitly true
- [ ] 2.2 Define the built-in taint constant: key=`stratos.sh/not-ready`, value=`"true"`, effect=`NoSchedule`
- [ ] 2.3 Update `LaunchNode` in `manager.go` to include the built-in taint in launch config when enabled (alongside custom `startupTaints`)
- [ ] 2.4 Update `prepareNodeForStandby` to re-apply the built-in taint when enabled (alongside custom `startupTaints`)
- [ ] 2.5 Rework `ProcessStartupTaints` to handle built-in and custom taints separately: built-in taint uses `WhenNetworkReady` removal with timeout; custom taints are never removed by Stratos
- [ ] 2.6 Update `processRunningNodesStartupTaints` in `pool_maintenance.go` to process nodes when either built-in taint is enabled or custom taints exist (not just when custom taints are configured)
- [ ] 2.7 Remove all references to `StartupTaintRemovalMode`, `StartupTaintRemovalWhenNetworkReady`, `StartupTaintRemovalExternal`, and the mode-routing logic
- [ ] 2.8 Update warmup completion logic in the controller to wait for network readiness when `enableNetworkReadinessTaint` is true (replaces the old `startupTaintRemoval: WhenNetworkReady` check)

## 3. Unit Tests

- [ ] 3.1 Update `startup_taints_test.go` to test the new `enableNetworkReadinessTaint` field: default (nil = enabled), explicit true, explicit false
- [ ] 3.2 Add tests for built-in taint application at launch and re-application on standby
- [ ] 3.3 Add tests for built-in taint removal on network readiness and timeout
- [ ] 3.4 Add tests verifying custom `startupTaints` are never removed by Stratos
- [ ] 3.5 Remove tests for the old `startupTaintRemoval` mode field and `External` mode behavior

## 4. Integration Tests

- [ ] 4.1 Update `tests/integration/startup_taints_test.go` for the new field and behavior
- [ ] 4.2 Add integration test: default pool gets built-in taint, removed on network ready
- [ ] 4.3 Add integration test: `enableNetworkReadinessTaint: false` skips built-in taint
- [ ] 4.4 Add integration test: custom `startupTaints` are applied but not removed by Stratos

## 5. Samples and Documentation

- [ ] 5.1 Update sample NodePool manifests in `deploy/samples/` to reflect new field (remove `startupTaintRemoval` references)
- [ ] 5.2 Regenerate Helm CRD manifests (`deploy/charts/stratos/crds/`)
