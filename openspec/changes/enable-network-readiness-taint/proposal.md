## Why

When a Stratos-managed node starts from standby, there is a race condition between CNI initialization and pod scheduling. The node becomes `Ready` before the CNI has finished setting up networking, causing the scheduler to place pods that then fail due to missing network paths. Currently, users must manually configure `startupTaints` and `startupTaintRemoval` to prevent this — most users don't, and hit the race condition.

## What Changes

- Add `enableNetworkReadinessTaint` boolean field to `NodeTemplate` (default: `true`). When enabled, Stratos automatically applies a `stratos.sh/not-ready:NoSchedule` taint at launch and removes it when the CNI reports ready.
- **BREAKING**: Remove the `startupTaintRemoval` field from `NodeTemplate`. The built-in network readiness taint is always auto-removed on network readiness. Custom `startupTaints` are now always externally managed (Stratos applies them at launch/standby but never removes them).
- Simplify the user mental model: safe-by-default with a single boolean toggle, custom taints as a separate power-user concern.

## Capabilities

### New Capabilities

_(none — this modifies existing capabilities)_

### Modified Capabilities

- `nodepool-crd`: Add `enableNetworkReadinessTaint` field, remove `startupTaintRemoval` field from NodeTemplate schema.
- `stratos-core`: Change startup taint processing — built-in taint auto-removed on network readiness, custom startupTaints always externally managed. Remove `startupTaintRemoval` mode routing.

## Impact

- **CRD types**: `api/v1alpha1/nodepool_types.go` — add field, remove field, remove `StartupTaintRemovalMode` type
- **Controller**: `internal/controller/manager.go`, `pool_maintenance.go` — rework `ProcessStartupTaints` to separate built-in vs custom taint handling
- **Cloud provider**: Launch config must include the built-in taint when enabled
- **Tests**: Unit and integration tests need updating for new field and removed field
- **Helm chart**: CRD manifests regenerated, sample manifests updated
- **Breaking change**: Users with `startupTaintRemoval: External` must migrate to `enableNetworkReadinessTaint: false` + `startupTaints`
