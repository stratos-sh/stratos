# Requirements Archive: v1 Stratos Codebase Restructure

**Archived:** 2026-02-03
**Status:** SHIPPED

This is the archived requirements specification for v1.
For current requirements, see `.planning/REQUIREMENTS.md` (created for next milestone).

---

**Defined:** 2026-02-02
**Core Value:** Every file has a clear home, every name tells you what it does, and you can follow any code path without getting lost.

## v1 Requirements

Requirements for the restructure. Each maps to roadmap phases.

### Mechanical Cleanup

- [x] **MECH-01**: Resolve naming collisions — merge or rename reconcile.go/reconciler.go, disambiguate duplicate nodeclass.go files
- [x] **MECH-02**: Replace all context.Background() misuses with proper context propagation from reconciliation loop
- [x] **MECH-03**: Standardize error wrapping — use fmt.Errorf with %w consistently throughout all packages
- [x] **MECH-04**: Unexport internal functions — make NodeEventHandler, query helpers, maintenance helpers package-private

### Package Structure

- [x] **PKG-01**: Split controller into per-CRD packages — create controller/nodepool/ and controller/nodeclass/ following Karpenter pattern
- [x] **PKG-02**: Split monolithic strategy — break kubernetes.go (910 lines) into focused sub-files or sub-package by concern
- [x] **PKG-03**: Clean up lifecycle package — split warmup.go (455 lines) and operations.go (355 lines) into focused, single-responsibility units
- [x] **PKG-04**: Delete dead code — remove _extracted/ directory and all unused exports

### Quality Gates

- [x] **QUAL-01**: Add structural linters — enable funlen, cyclop, depguard, contextcheck in golangci-lint config to prevent regression
- [x] **QUAL-02**: Add package documentation — create doc.go for every package explaining purpose and relationships
- [x] **QUAL-03**: Fix integration tests — update all tests to compile and pass with new package structure

## v2 Requirements

Deferred to future work. Tracked but not in current roadmap.

### Further Decomposition

- **V2-01**: Split cloudprovider/aws/provider.go (522 lines) into sub-packages per AWS service (Karpenter's 12-package model)
- **V2-02**: Introduce internal/operator/ package to wrap manager setup
- **V2-03**: Evaluate reconciler.io/runtime SubReconciler framework for internal reconciler structure
- **V2-04**: Migrate integration tests from build tags to environment variable gating

### Performance

- **V2-05**: Add instance list caching to avoid repeated AWS API calls per reconcile
- **V2-06**: Cache node counts at start of reconcile instead of querying 3x

## Out of Scope

| Feature | Reason |
|---------|--------|
| New operator features (dry-run, quotas, multi-region) | Restructure only, not feature work |
| Kubernetes/dependency version upgrades | Separate concern, don't mix with restructure |
| Helm chart changes | Deployment stays the same |
| CRD schema changes | API surface preserved unless required for cleanup |
| Documentation site (docs/) rewrites | Focus is code structure, not user docs |
| Performance optimizations | Only incidental improvements from cleaner code |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| MECH-01 | Phase 1: Mechanical Cleanup | Complete |
| MECH-02 | Phase 1: Mechanical Cleanup | Complete |
| MECH-03 | Phase 1: Mechanical Cleanup | Complete |
| MECH-04 | Phase 1: Mechanical Cleanup | Complete |
| PKG-04 | Phase 1: Mechanical Cleanup | Complete |
| PKG-03 | Phase 2: Lifecycle Package Extraction | Complete |
| PKG-02 | Phase 3: Strategy Package Extraction | Complete |
| PKG-01 | Phase 4: Controller Split | Complete |
| QUAL-01 | Phase 5: Linter Enforcement | Complete |
| QUAL-02 | Phase 6: Documentation and Test Recovery | Complete |
| QUAL-03 | Phase 6: Documentation and Test Recovery | Complete |

**Coverage:**
- v1 requirements: 11 total
- Shipped: 11
- Adjusted: 0
- Dropped: 0

---

## Milestone Summary

**Shipped:** 11 of 11 v1 requirements
**Adjusted:** None — all requirements delivered as originally specified
**Dropped:** None

---
*Archived: 2026-02-03 as part of v1 milestone completion*
