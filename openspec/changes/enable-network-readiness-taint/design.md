## Context

Stratos manages pre-warmed node pools. When a stopped node starts from standby, there's a race condition: the node becomes `Ready` before the CNI plugin has finished initializing networking. Pods scheduled during this window experience network failures.

Currently, users must manually configure `startupTaints` and `startupTaintRemoval` to prevent this. The `startupTaintRemoval` field has two modes (`WhenNetworkReady`, `External`) that control how Stratos removes taints. This design conflates two concerns: the built-in network readiness gate and custom user-managed taints.

## Goals / Non-Goals

**Goals:**
- Make network readiness gating the default behavior (safe by default)
- Separate built-in network readiness taint from user-managed custom taints
- Simplify the API surface by removing `startupTaintRemoval` mode field
- Maintain backward-compatible network readiness detection (EKS, Cilium, Calico)

**Non-Goals:**
- Changing how network readiness is detected (the `NetworkReadinessChecker` stays as-is)
- Changing the timeout mechanism (2 minute timeout with force-remove stays)
- Adding new CNI support
- Changing custom `startupTaints` application at launch/standby (still applied by Stratos)

## Decisions

### D1: New boolean field `enableNetworkReadinessTaint` on NodeTemplate

**Decision:** Add `EnableNetworkReadinessTaint *bool` field to `NodeTemplate` with a kubebuilder default of `true`.

**Rationale:** A boolean is the simplest opt-in/opt-out mechanism. Using a pointer allows distinguishing "not set" (use default=true) from "explicitly false". The name describes exactly what it does: enables a taint that gates on network readiness.

**Alternative considered:** Making `startupTaints` default to include the network taint — rejected because it conflates the built-in safe default with user-controlled custom taints, and opt-out via null-vs-empty-list is fragile in Kubernetes.

### D2: Built-in taint is `stratos.sh/not-ready:NoSchedule`

**Decision:** When `enableNetworkReadinessTaint` is true, Stratos automatically applies `stratos.sh/not-ready` with value `"true"` and effect `NoSchedule`.

**Rationale:** Consistent with Kubernetes conventions (similar to `node.kubernetes.io/not-ready`). Uses Stratos's own domain. `NoSchedule` prevents new pods but doesn't evict existing ones (relevant during node transitions).

### D3: Remove `startupTaintRemoval` field and `StartupTaintRemovalMode` type

**Decision:** Remove the `StartupTaintRemoval` field from `NodeTemplate` and the `StartupTaintRemovalMode` type entirely.

**Rationale:** With the built-in taint always using `WhenNetworkReady` removal, and custom `startupTaints` always being externally managed, there's no need for a mode selector. The two concerns are fully decoupled:
- Built-in taint → always auto-removed on network readiness (hardcoded)
- Custom `startupTaints` → always externally managed (Stratos applies but never removes)

**Migration for `External` mode users:** Set `enableNetworkReadinessTaint: false` and use `startupTaints` with their external controller.

### D4: Custom `startupTaints` become always externally managed

**Decision:** Stratos continues to apply custom `startupTaints` at launch and re-apply them on standby transitions, but never removes them. No timeout logic for custom taints.

**Rationale:** If a user specifies custom taints, they have a custom readiness mechanism. Stratos shouldn't guess when those taints should be removed. This eliminates the ambiguity of the old `External` vs `WhenNetworkReady` mode for custom taints.

### D5: Built-in taint included in launch config and standby re-application

**Decision:** The built-in network readiness taint is:
1. Included in `--register-with-taints` at launch (alongside any custom `startupTaints`)
2. Re-applied when a node transitions to standby (so it's present on next start)
3. Removed by `ProcessStartupTaints` when network is ready (same `NetworkReadinessChecker` logic)

**Rationale:** Follows the same lifecycle as existing startup taints, just managed automatically.

## Risks / Trade-offs

**[Breaking change for `startupTaintRemoval: External` users]** → Documented migration path: set `enableNetworkReadinessTaint: false` and manage taints via `startupTaints`. This is a v1alpha1 API, so breaking changes are expected.

**[Existing users get new default taint on upgrade]** → Nodes that were previously immediately schedulable will now wait for CNI readiness. This is strictly safer behavior, but could briefly delay pod scheduling on nodes with fast CNI init. The 2-minute timeout ensures nodes aren't stuck indefinitely.

**[Pointer bool complexity]** → Using `*bool` adds nil-checking in code, but is the standard Kubernetes pattern for "default true" booleans. A helper function `isNetworkReadinessTaintEnabled(pool)` will encapsulate the nil-check.
