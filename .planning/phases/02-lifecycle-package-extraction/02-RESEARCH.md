# Phase 2: Lifecycle Package Extraction - Research

**Researched:** 2026-02-02
**Domain:** Go package file splitting within an existing leaf package
**Confidence:** HIGH

## Summary

This phase is a pure file-organization refactor within the existing `internal/controller/lifecycle/` package. No new packages, no interface changes, no dependency changes. The work is splitting two large files -- `warmup.go` (455 lines, 7 methods) and `operations.go` (355 lines, 9 methods) -- into focused files under 200 lines each.

Two of four success criteria are already met: lifecycle/ imports zero packages from controller/ (verified), and nodestate/ is a pure leaf importing only `fmt`, `time`, and `k8s.io/api/core/v1` (verified). The remaining work is mechanical: move method definitions between files in the same package, ensuring each new file has the correct imports.

**Primary recommendation:** Split warmup.go into 3 files (monitors, handlers, adoption) and operations.go into 3 files (launch, startstop, sync). Use `warmup_` prefix for warmup files and `node_` prefix for operation files. Keep manager.go unchanged. Keep warmup_test.go unchanged (it tests functions across multiple source files, which is valid in Go).

## Standard Stack

Not applicable -- this phase uses no new libraries. All existing imports remain unchanged:

### Existing Dependencies (unchanged)
| Library | Purpose | Used By |
|---------|---------|---------|
| `sigs.k8s.io/controller-runtime/pkg/client` | K8s API patching | operations, warmup |
| `sigs.k8s.io/controller-runtime/pkg/log` | Structured logging | operations, warmup |
| `k8s.io/api/core/v1` | Node type definitions | all files |
| `k8s.io/apimachinery/pkg/api/errors` | API error checking | deleteNode only |
| `k8s.io/client-go/tools/events` | Event recording | manager.go only |
| `internal/cloudprovider` | Cloud operations | operations, warmup |
| `internal/controller/nodestate` | State constants/validation | operations, warmup |
| `internal/metrics` | Prometheus metrics | operations, warmup |

## Architecture Patterns

### Current Package Structure
```
internal/controller/lifecycle/
  manager.go        (108 lines) - Manager struct, interfaces, constructor, helpers
  warmup.go         (455 lines) - 7 methods: warmup monitoring + timeout + adoption
  operations.go     (355 lines) - 9 methods: launch, label, transition, start, stop, sync, find, helpers
  warmup_test.go    (969 lines) - 13 tests covering warmup + operations
```

### Recommended Target Structure
```
internal/controller/lifecycle/
  manager.go           (108 lines) - UNCHANGED
  warmup_monitor.go    (~197 lines) - MonitorWarmup, MonitorCloudWarmup
  warmup_handlers.go   (~194 lines) - handleWarmupTimeout, handleControllerStopWarmup, handleWarmupFailure, handleCloudWarmupTimeout
  warmup_adoption.go   (~92 lines) - adoptAndTransitionToStandby
  node_launch.go       (~102 lines) - LaunchNode, LabelNode
  node_startstop.go    (~159 lines) - StartNode, StopNode, TransitionState
  node_sync.go         (~125 lines) - SyncNodeState, FindNodeByInstanceID, deleteNode, setLastStartedAnnotation
  warmup_test.go       (969 lines) - UNCHANGED
```

### Pattern: Same-Package File Splitting in Go

**What:** Moving method definitions between .go files within the same package. All files share the same `package lifecycle` declaration, so methods on `*Manager` can reference each other across files freely.

**Key property:** In Go, all files in a package share a single namespace. There is no concept of file-level scope. A function defined in `warmup_monitor.go` can call a function in `node_sync.go` without any import, as long as both are in `package lifecycle`.

**Implications for this refactor:**
- No function signatures change
- No new exports needed (private methods stay private)
- No import changes between files (imports are per-file, but all types are package-scoped)
- Test files continue to access all functions regardless of which source file defines them
- `go build` treats all .go files in the directory equally

### Decision Rationale: Warmup Split

The user decision locks: "Both warmup monitoring flows stay in the same file." The 178-line estimate in the decision refers to the two public Monitor* methods only (MonitorWarmup: 92 lines + MonitorCloudWarmup: 85 lines = 177 lines). With file overhead (~20 lines for license, package, imports), this yields ~197 lines -- within the 200-line target.

The remaining warmup methods split naturally into handlers and adoption:

**warmup_handlers.go (~194 lines):** Groups all timeout/completion/failure handling:
- `handleWarmupTimeout` (37 lines) -- called by both MonitorWarmup and MonitorCloudWarmup
- `handleControllerStopWarmup` (68 lines) -- called only by MonitorWarmup
- `handleWarmupFailure` (20 lines) -- called only by MonitorCloudWarmup
- `handleCloudWarmupTimeout` (39 lines) -- called only by MonitorCloudWarmup
- Total: 164 code lines + ~30 overhead = ~194 lines

**warmup_adoption.go (~92 lines):** The adoption flow is self-contained:
- `adoptAndTransitionToStandby` (62 lines) -- called only by MonitorCloudWarmup
- Total: 62 code lines + ~30 overhead = ~92 lines

Rationale for grouping handleControllerStopWarmup with handlers rather than monitors:
- It is a private helper, not a public entry point
- Its concern is "handling warmup completion" (same as timeout/failure handlers)
- Keeping it with MonitorWarmup would push warmup_monitor.go to ~267 lines (over limit)
- Both timeout handlers and the completion handler share the same import profile

### Decision Rationale: Operations Split

The 9 operations methods group naturally into 3 concerns:

**node_launch.go (~102 lines):** Instance provisioning concern:
- `LaunchNode` (30 lines) -- launches cloud instance
- `LabelNode` (42 lines) -- applies Stratos labels to K8s node
- Both relate to bringing a new node into the system

**node_startstop.go (~159 lines):** Instance lifecycle transitions:
- `StartNode` (56 lines) -- standby to running
- `StopNode` (50 lines) -- running to standby
- `TransitionState` (23 lines) -- validates and patches state label
- StartNode and StopNode both call TransitionState, creating strong cohesion

**node_sync.go (~125 lines):** Discovery, sync, and cleanup:
- `SyncNodeState` (51 lines) -- reconciles K8s state with cloud state
- `FindNodeByInstanceID` (22 lines) -- discovers node by instance ID
- `deleteNode` (10 lines) -- deletes K8s node object
- `setLastStartedAnnotation` (12 lines) -- annotation helper used by SyncNodeState
- SyncNodeState calls deleteNode, TransitionState, and setLastStartedAnnotation
- FindNodeByInstanceID calls containsInstanceID (defined in manager.go)

### Call Graph (Cross-File Calls After Split)

```
warmup_monitor.go:
  MonitorWarmup -> TransitionState (node_startstop.go)
                -> deleteNode (node_sync.go)
                -> handleWarmupTimeout (warmup_handlers.go)
                -> handleControllerStopWarmup (warmup_handlers.go)
  MonitorCloudWarmup -> FindNodeByInstanceID (node_sync.go)
                     -> LabelNode (node_launch.go)
                     -> deleteNode (node_sync.go)
                     -> adoptAndTransitionToStandby (warmup_adoption.go)
                     -> handleWarmupFailure (warmup_handlers.go)
                     -> handleWarmupTimeout (warmup_handlers.go)
                     -> handleCloudWarmupTimeout (warmup_handlers.go)

warmup_handlers.go:
  handleWarmupTimeout -> TransitionState (node_startstop.go)
                      -> deleteNode (node_sync.go)
  handleControllerStopWarmup -> TransitionState (node_startstop.go)

node_startstop.go:
  StartNode -> TransitionState (same file)
  StopNode -> TransitionState (same file)

node_sync.go:
  SyncNodeState -> deleteNode (same file)
                -> TransitionState (node_startstop.go)
                -> setLastStartedAnnotation (same file)
  FindNodeByInstanceID -> containsInstanceID (manager.go)
```

All cross-file calls work transparently because all files share the same package namespace.

### File Naming Convention

Use `warmup_` prefix for warmup-domain files and `node_` prefix for operations-domain files:
- `warmup_monitor.go`, `warmup_handlers.go`, `warmup_adoption.go`
- `node_launch.go`, `node_startstop.go`, `node_sync.go`

This provides clear visual grouping in the directory listing and distinguishes the two domains (warmup lifecycle vs general node operations).

### Anti-Patterns to Avoid
- **Splitting test files unnecessarily:** The 969-line warmup_test.go tests functions from both warmup.go and operations.go. In Go, test files can test any function in the same package regardless of source file. Do not split the test file -- it would create churn with no benefit.
- **Creating sub-packages for file organization:** The decision locks flat structure. Sub-packages would add import complexity for no gain.
- **Moving helper functions to callers when they have multiple callers:** `containsInstanceID` and `parseUnixTimestamp` are called from both warmup.go and operations.go -- they must stay in manager.go (or another shared file). Moving them to a caller's file would create a misleading ownership signal.

## Don't Hand-Roll

Not applicable -- this phase is pure code movement. No new functionality is being built.

## Common Pitfalls

### Pitfall 1: Import Duplication After Split
**What goes wrong:** When splitting a file, forgetting to add the correct imports to each new file. The original file's import block covers all methods; each new file needs only its subset.
**Why it happens:** Copy-paste of the full import block, or missing imports for functions that were previously covered by the original file's imports.
**How to avoid:** After creating each new file, run `goimports` or `go build ./internal/controller/lifecycle/` to verify. Better: use `goimports -w` on each new file to auto-fix imports.
**Warning signs:** `go build` fails with "undefined" or "imported and not used" errors.

### Pitfall 2: Accidentally Breaking the 200-Line Limit with Overhead
**What goes wrong:** Counting only code lines and forgetting that each file needs ~20-30 lines of boilerplate (license header, package declaration, import block, blank lines).
**Why it happens:** The method sizes sum to under 200, but with per-file overhead the total exceeds 200.
**How to avoid:** The analysis above includes overhead in all estimates. The tightest files are warmup_monitor.go (~197) and warmup_handlers.go (~194). Both are within limit but monitor is tight -- verify after implementation.
**Warning signs:** `wc -l` on a new file shows over 200.

### Pitfall 3: Shared Helper Functions Breaking Across Files
**What goes wrong:** Moving a private helper to a specific file when it is called from multiple files.
**Why it happens:** Not checking the full call graph before deciding where a helper lives.
**How to avoid:** The two shared helpers (`containsInstanceID`, `parseUnixTimestamp`) stay in manager.go where they are today. `deleteNode` stays in node_sync.go and is called from warmup methods -- which works fine cross-file.
**Warning signs:** Method X calls Y, but Y was moved to the same file as Z based on a different grouping logic.

### Pitfall 4: Git History Loss
**What goes wrong:** Creating new files and deleting old files instead of using git-trackable renames or moves.
**Why it happens:** Go has no refactoring tool that preserves git blame across file splits.
**How to avoid:** Accept this limitation. File splits inherently break per-line blame. The git log on the commit will document what moved where. Use a clear commit message listing all file movements.
**Warning signs:** N/A -- this is an inherent trade-off of file splitting.

### Pitfall 5: Forgetting the License Header
**What goes wrong:** New files missing the Apache 2.0 license header that all existing files have.
**Why it happens:** Copying only the code, not the boilerplate.
**How to avoid:** Copy the exact license header from an existing file (15 lines). All files in this project use the identical header.

## Code Examples

### Example: New File Structure (warmup_monitor.go)

```go
/*
Copyright 2026 Stratos Authors.
[...license header...]
*/

package lifecycle

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
    "github.com/stratos-sh/stratos/internal/cloudprovider"
    "github.com/stratos-sh/stratos/internal/controller/nodestate"
    "github.com/stratos-sh/stratos/internal/metrics"
)

// MonitorWarmup monitors a node in warmup state [...]
func (m *Manager) MonitorWarmup(...) error {
    // [exact code from warmup.go L36-L127]
}

// MonitorCloudWarmup monitors a cloud instance in warmup state [...]
func (m *Manager) MonitorCloudWarmup(...) error {
    // [exact code from warmup.go L242-L326]
}
```

### Example: New File Structure (node_startstop.go)

```go
/*
Copyright 2026 Stratos Authors.
[...license header...]
*/

package lifecycle

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
    "github.com/stratos-sh/stratos/internal/cloudprovider"
    "github.com/stratos-sh/stratos/internal/controller/nodestate"
    "github.com/stratos-sh/stratos/internal/metrics"
)

// TransitionState transitions a node to a new state.
func (m *Manager) TransitionState(...) error {
    // [exact code from operations.go L118-L140]
}

// StartNode starts a standby node for scale-up.
func (m *Manager) StartNode(...) error {
    // [exact code from operations.go L143-L198]
}

// StopNode stops a running node for scale-down.
func (m *Manager) StopNode(...) error {
    // [exact code from operations.go L201-L250]
}
```

### Verification Command Sequence

```bash
# After all file moves, verify:
# 1. Package compiles
go build ./internal/controller/lifecycle/

# 2. No upward imports to controller/
go list -f '{{.Imports}}' ./internal/controller/lifecycle/ | grep -v nodestate | grep controller && echo "FAIL: upward import" || echo "PASS"

# 3. All tests pass
go test -v -count=1 ./internal/controller/lifecycle/

# 4. All files under 200 lines
wc -l internal/controller/lifecycle/*.go

# 5. nodestate still clean
go list -f '{{.Imports}}' ./internal/controller/nodestate/ | grep controller && echo "FAIL" || echo "PASS"
```

## State of the Art

Not applicable -- this is internal code organization, not a technology choice.

## Open Questions

None. This phase is fully scoped:
- All method sizes are measured
- All call graphs are mapped
- All file groupings are determined
- All line counts (with overhead) are within the 200-line target
- The test file requires no changes

## Detailed Method Inventory

### warmup.go (455 lines) -- 7 methods

| Method | Lines | Visibility | Callers | Calls (within lifecycle) |
|--------|-------|-----------|---------|--------------------------|
| `MonitorWarmup` | 92 (L36-L127) | exported | cloud_sync.go | TransitionState, deleteNode, handleWarmupTimeout, handleControllerStopWarmup |
| `handleWarmupTimeout` | 37 (L130-L166) | private | MonitorWarmup, MonitorCloudWarmup | TransitionState, deleteNode |
| `handleControllerStopWarmup` | 68 (L170-L237) | private | MonitorWarmup | TransitionState |
| `MonitorCloudWarmup` | 85 (L242-L326) | exported | cloud_sync.go | FindNodeByInstanceID, adoptAndTransitionToStandby, handleWarmupFailure, deleteNode, LabelNode, handleWarmupTimeout, handleCloudWarmupTimeout |
| `handleWarmupFailure` | 20 (L330-L349) | private | MonitorCloudWarmup | (none) |
| `handleCloudWarmupTimeout` | 39 (L352-L390) | private | MonitorCloudWarmup | (none) |
| `adoptAndTransitionToStandby` | 62 (L394-L455) | private | MonitorCloudWarmup | (none) |

### operations.go (355 lines) -- 9 methods

| Method | Lines | Visibility | Callers | Calls (within lifecycle) |
|--------|-------|-----------|---------|--------------------------|
| `LaunchNode` | 30 (L39-L68) | exported | pool_maintenance.go | (none) |
| `LabelNode` | 42 (L74-L115) | exported | MonitorCloudWarmup | (none) |
| `TransitionState` | 23 (L118-L140) | exported | StartNode, StopNode, SyncNodeState, MonitorWarmup, handleWarmupTimeout, handleControllerStopWarmup | (none) |
| `StartNode` | 56 (L143-L198) | exported | external callers | TransitionState |
| `StopNode` | 50 (L201-L250) | exported | external callers | TransitionState |
| `deleteNode` | 10 (L253-L262) | private | SyncNodeState, MonitorWarmup, handleWarmupTimeout, MonitorCloudWarmup | (none) |
| `SyncNodeState` | 51 (L265-L315) | exported | cloud_sync.go | deleteNode, TransitionState, setLastStartedAnnotation |
| `FindNodeByInstanceID` | 22 (L319-L340) | exported | MonitorCloudWarmup | containsInstanceID (manager.go) |
| `setLastStartedAnnotation` | 12 (L344-L355) | private | SyncNodeState | (none) |

### manager.go (108 lines) -- UNCHANGED

| Item | Lines | Type |
|------|-------|------|
| License header | 15 | boilerplate |
| Imports | 10 | imports |
| `NodeLauncher` interface | 3 | type |
| `NodeHooks` interface | 5 | type |
| `Manager` struct | 7 | type |
| `NewManager` constructor | 8 | function |
| `WithNodeHooks` | 4 | method |
| `PrepareForStandby` | 6 | method |
| `PrepareForRunning` | 6 | method |
| `containsInstanceID` | 5 | helper function |
| `parseUnixTimestamp` | 9 | helper function |

## Sources

### Primary (HIGH confidence)
- Direct code inspection of all files in `internal/controller/lifecycle/`
- Direct code inspection of all callers in `internal/controller/` (cloud_sync.go, pool_maintenance.go, provider_cache.go)
- `go build` and `go test` verification on current codebase
- `go list` import graph verification

### Confidence Breakdown
- Standard stack: HIGH -- no new dependencies, verified existing imports
- Architecture: HIGH -- all method sizes, call graphs, and line counts measured from source
- Pitfalls: HIGH -- based on direct analysis of the specific code being split

**Research date:** 2026-02-02
**Valid until:** No expiry (internal code analysis, not external dependency research)
