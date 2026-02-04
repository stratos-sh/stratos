# Project Milestones: Stratos

## v1.1.1 Naming & Dead Code Cleanup (Shipped: 2026-02-04)

**Delivered:** Pure refactoring — renamed vestigial types (Strategy→Scaler), removed dead code (UncordonNode), aligned filenames to subject_role.go convention, and replaced unsafe interface{} with typed Pods field

**Phases completed:** 12-16 (5 plans total)

**Key accomplishments:**
- Removed dead UncordonNode method (20 lines, zero callers confirmed by deadcode tool)
- Renamed Strategy→Scaler, drainHelper→nodeDrainer, drainConfig→drainOptions via gopls atomic renames across 23 files
- Renamed kubernetes.go→scaler.go and events.go→pod_events.go with git blame preserved
- Replaced ScalingDemand.Metadata interface{} with typed Pods []corev1.Pod, eliminating runtime type assertion
- Committed network-readiness-strategy-enum baseline (15 files) as clean starting point

**Stats:**
- 46 files created/modified (1,063 insertions, 249 deletions)
- 21,734 lines of Go
- 5 phases, 5 plans, ~8 tasks
- 1 day (same day as v1.1)

**Git range:** `feat(12-01)` → `docs(12-16)`

**What's next:** TBD

---

## v1.1 Simplify Scaling (Shipped: 2026-02-04)

**Delivered:** Removed the ScalingStrategy abstraction and GitHub Actions support — Stratos is now Kubernetes-only with direct scaling logic via `internal/scaling/`

**Phases completed:** 7-11 (5 plans total)

**Key accomplishments:**
- Created `internal/scaling/` package with all Kubernetes scaling logic (18 Go files relocated from strategy/kubernetes/)
- Replaced strategy cache/factory/interface with single `*scaling.Strategy` field on reconciler (-81 lines of abstraction)
- Deleted ~4,500 lines of dead code: `strategy/`, `githubactions/`, `github/` packages
- Simplified NodePool CRD by removing `scalingStrategy` and `githubActions` fields
- Cleaned all residual references, RBAC markers, and dead code; full verification suite passes clean

**Stats:**
- 70 files created/modified (3,784 insertions, 1,483 deletions)
- 21,755 lines of Go (10,853 non-test)
- 5 phases, 5 plans, ~10 tasks
- 1 day from start to ship

**Git range:** `feat(07-01)` → `docs(v1.1)`

**What's next:** TBD

---

## v1 Codebase Restructure (Shipped: 2026-02-03)

**Delivered:** Restructured Stratos operator from monolithic controller package into focused, well-bounded Go packages following Karpenter patterns

**Phases completed:** 1-6 (13 plans total)

**Key accomplishments:**
- Cleaned naming collisions, context propagation, error wrapping, and unexported internal APIs
- Split lifecycle/ into focused single-concern files (warmup monitors, launch, start/stop, sync)
- Promoted strategy/ to top-level package, decomposed 910-line kubernetes.go into focused sub-files
- Split controller/ into per-CRD packages (nodepool/, nodeclass/) with aggregator setup.go
- Enforced structural linters (depguard, funlen, cyclop, contextcheck) with zero violations
- Added doc.go for all 14 internal packages, certified 163 tests pass flake-free

**Stats:**
- 125 files created/modified
- 18,093 lines of Go
- 6 phases, 13 plans
- 1 day from start to ship

**Git range:** `chore(01-01)` → `docs(06)`

**What's next:** TBD

---
