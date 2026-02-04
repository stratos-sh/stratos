# Milestone v1: Stratos Codebase Restructure

**Status:** SHIPPED 2026-02-03
**Phases:** 1-6
**Total Plans:** 13

## Overview

Restructure the Stratos operator codebase from a monolithic controller package into focused, well-bounded Go packages following Karpenter patterns. The work proceeded in strict dependency order: mechanical cleanup first (naming, context, errors, dead code), then leaf package extraction (lifecycle), then strategy extraction (910-line kubernetes.go), then the core controller split (per-CRD packages), then linter enforcement to lock in boundaries, and finally documentation and test recovery. Every phase left the code compiling. Tests broke during phases but passed at the end.

## Phases

### Phase 1: Mechanical Cleanup

**Goal**: Every file has an unambiguous name, context flows correctly from reconciliation, errors are consistently wrapped, and internal functions are not exported
**Depends on**: Nothing (first phase)
**Requirements**: MECH-01, MECH-02, MECH-03, MECH-04, PKG-04
**Success Criteria** (what must be TRUE):
  1. No two files in the project share an ambiguous name -- reconcile.go/reconciler.go resolved, duplicate nodeclass.go files disambiguated
  2. Zero context.Background() calls in non-test production code -- all reconciliation paths propagate the reconciler context
  3. Every fmt.Errorf in the codebase uses %w for error wrapping -- no %v wrapping of errors that should be inspectable
  4. NodeEventHandler, NodeClassEventHandler, and query/maintenance helpers are unexported (package-private)
  5. The _extracted/ directory does not exist and no unused exports remain in queries.go or maintenance.go
**Plans**: 3 plans

Plans:
- [x] 01-01-PLAN.md -- Delete dead code, merge reconcile.go into reconciler.go, rename all controller files
- [x] 01-02-PLAN.md -- Unexport internal symbols across controller, strategy, nodestate, and aws packages
- [x] 01-03-PLAN.md -- Fix context propagation in reconciliation path, standardize error type checking

### Phase 2: Lifecycle Package Extraction

**Goal**: Node lifecycle operations (launch, start, stop, warmup monitoring) live in a clean leaf package with no upward imports to controller/
**Depends on**: Phase 1
**Requirements**: PKG-03
**Success Criteria** (what must be TRUE):
  1. lifecycle/ package has zero imports from internal/controller/ -- verified by go build and import graph
  2. warmup.go is split into files under 220 lines each with single responsibilities (warmup monitoring vs timeout handling vs state transitions) -- warmup_monitor.go accepted at ~215 lines per user decision to keep both monitors together
  3. operations.go is split into focused units -- each file handles one lifecycle concern (launch, start, stop, sync)
  4. nodestate/ remains a pure leaf package with no upward dependencies
**Plans**: 2 plans

Plans:
- [x] 02-01-PLAN.md -- Split warmup.go into warmup_monitor.go, warmup_handlers.go, warmup_adoption.go
- [x] 02-02-PLAN.md -- Split operations.go into node_launch.go, node_startstop.go, node_sync.go

### Phase 3: Strategy Package Extraction

**Goal**: Scaling strategies are a first-class top-level package, and kubernetes.go (910 lines) is decomposed into focused files organized by concern
**Depends on**: Phase 2
**Requirements**: PKG-02
**Success Criteria** (what must be TRUE):
  1. strategy/ lives at internal/strategy/ (not internal/controller/strategy/) as a top-level domain package
  2. kubernetes.go is replaced by a kubernetes/ sub-package with separate files -- no single file exceeds 300 lines
  3. strategy/ imports lifecycle/ and cloudprovider/ but never imports controller/ -- no circular dependencies
  4. Drain logic has a clear home (either strategy/kubernetes/drain.go or lifecycle/) with no ambiguity
**Plans**: 2 plans

Plans:
- [x] 03-01-PLAN.md -- Relocate strategy/ to internal/strategy/, split into kubernetes/ and githubactions/ sub-packages, update all import paths
- [x] 03-02-PLAN.md -- Decompose kubernetes.go into focused files by concern, split drain.go, refactor networkReadinessChecker

### Phase 4: Controller Split

**Goal**: Each CRD has its own controller package following Karpenter's package-per-controller pattern, with a central setup.go for registration
**Depends on**: Phase 3
**Requirements**: PKG-01
**Success Criteria** (what must be TRUE):
  1. internal/controller/nodepool/ package exists and contains all NodePool reconciliation logic (reconciler, scale-up, scale-down, maintenance, cloud sync, status)
  2. internal/controller/nodeclass/ package exists and consolidates all NodeClass lifecycle management (resolution, validation, conditions)
  3. internal/controller/setup.go registers all controllers with the manager -- single entry point for controller registration
  4. No reconciliation logic remains in internal/controller/ root except setup.go and shared utilities
  5. go build ./... compiles cleanly with no circular imports across the new package structure
**Plans**: 2 plans

Plans:
- [x] 04-01-PLAN.md -- Move cluster_config to internal/config/, create nodepool/ package, relocate lifecycle/ and nodestate/ under nodepool/
- [x] 04-02-PLAN.md -- Create nodeclass/ package with own reconciler, build aggregator setup.go, update main.go and integration tests

### Phase 5: Linter Enforcement

**Goal**: Structural linters enforce the new package boundaries and code quality standards so the restructuring cannot regress
**Depends on**: Phase 4
**Requirements**: QUAL-01
**Success Criteria** (what must be TRUE):
  1. depguard rules prevent strategy/ from importing aws/, prevent controller/ sub-packages from importing provider implementations directly
  2. funlen flags any function exceeding the configured threshold -- no new monolithic functions can be added
  3. cyclop enforces package-level complexity limits -- no package can accumulate unchecked complexity
  4. contextcheck catches any new context.Background() misuse in production code
  5. make lint passes with all new linters enabled and zero violations
**Plans**: 2 plans

Plans:
- [x] 05-01-PLAN.md -- Add 4 structural linters to golangci-lint config, fix all non-complexity violations (errcheck, gosec, govet, misspell, staticcheck, funlen)
- [x] 05-02-PLAN.md -- Refactor 5 high-complexity functions (reconcileNodePool, LaunchInstance, MonitorWarmup, MonitorCloudWarmup, FindScaleDownCandidates), achieve zero-violation make lint

### Phase 6: Documentation and Test Recovery

**Goal**: Every package has a doc.go explaining its purpose and relationships, and all integration tests compile and pass against the new structure
**Depends on**: Phase 5
**Requirements**: QUAL-02, QUAL-03
**Success Criteria** (what must be TRUE):
  1. Every package under internal/ has a doc.go file with a package comment explaining purpose, responsibilities, and key types
  2. make test passes -- all unit tests work with the new package structure
  3. make test-integration passes -- all integration tests updated for new imports and reconciler construction
  4. Integration tests run 3 consecutive times without flakes
**Plans**: 2 plans

Plans:
- [x] 06-01-PLAN.md -- Create doc.go files for all 15 packages under internal/ with expanded documentation
- [x] 06-02-PLAN.md -- Verify and certify all unit and integration tests pass (3 consecutive runs, no flakes)

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Mechanical Cleanup | 3/3 | Complete | 2026-02-02 |
| 2. Lifecycle Package Extraction | 2/2 | Complete | 2026-02-02 |
| 3. Strategy Package Extraction | 2/2 | Complete | 2026-02-03 |
| 4. Controller Split | 2/2 | Complete | 2026-02-03 |
| 5. Linter Enforcement | 2/2 | Complete | 2026-02-03 |
| 6. Documentation and Test Recovery | 2/2 | Complete | 2026-02-03 |

## Milestone Summary

**Key Decisions:**

- Merged reconcile.go into reconciler.go (single file for type + entry point + main loop)
- Controller file naming convention: subject_role.go (e.g., nodepool_validation.go, provider_cache.go)
- Context propagation: ensureCloudProvider takes ctx parameter and passes to all cloud operations
- warmup_monitor.go kept at 215 lines (both monitors together per user locked decision)
- Factory moved from strategy/ to controller to avoid Go import cycle (parent/child)
- networkReadinessChecker stored as struct field, instantiated once in New()
- NodeClass reconciler gets own Reconcile() with handleDeletion() and reconcileLifecycle()
- Aggregator setup.go at controller/ root calls nodepool.Setup() and nodeclass.Setup()
- safeInt32() overflow guard pattern used instead of nolint comments for gosec G115
- reconcileNodePool refactored into orchestrator calling 6 focused phase helpers

**Issues Resolved:**

- 910-line kubernetes.go monolith decomposed into 8 focused files under 300 lines each
- 455-line warmup.go split into 3 single-concern files
- 355-line operations.go split into 3 lifecycle files
- 39 context.Background() misuses fixed with proper context propagation
- All unexported symbols locked down before structural refactoring

**Technical Debt:**

- interface.go exists in two packages (cloudprovider/ and strategy/) — generic name doesn't indicate which interface
- warmup.go in aws/ could be more specific since lifecycle/ also had a warmup file

---

_Archived: 2026-02-03 as part of v1 milestone completion_
