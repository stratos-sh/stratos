# Coding Conventions

**Analysis Date:** 2026-02-02

## Naming Patterns

**Files:**
- Package files use lowercase with underscores for multi-word names (e.g., `launch_template.go`, `instance_types.go`)
- Test files append `_test.go` or `_integration_test.go` suffix
- Type implementations follow the concrete type name (e.g., `aws/provider.go` for `AWSProvider`)

**Functions:**
- Methods use receiver pattern: `func (r *ReceiverType) MethodName()`
- Constructor functions: `func New<TypeName>(...) *<TypeName>`
- Setter/getter methods: `func (r *Receiver) Get<Field>()` and `func (r *Receiver) Set<Field>()`
- Helper functions with action verbs: `calculateNodesNeeded()`, `ensureCloudProvider()`, `syncNodesWithCloud()`

**Variables:**
- Unexported (private) package globals use camelCase: `var ValidTransitions = map[...]`, `var supportedNodeClassKinds = map[...]`
- Loop variables use short names: `i`, `inst`, `node`, `pod`
- Context variables: `ctx` (always first parameter after receiver)
- Logger variables: `logger := log.FromContext(ctx)`

**Types:**
- Struct types use PascalCase: `NodePoolReconciler`, `AWSProvider`, `KubernetesStrategy`
- Interface types: `CloudProvider`, `NodeClass`
- Error types: `<Descriptor>Error` (e.g., `InstanceNotFoundError`, `InvalidStateError`)
- Constants for state/type values use UPPER_SNAKE_CASE: `InstanceStatePending`, `NodeStateWarmup`
- Map/slice types annotated with their element type (see `interface.go` line 71)

## Code Style

**Formatting:**
- Go standard formatting via `go fmt` (enforced by Makefile `make fmt`)
- Line length: No hard limit enforced (gocyclo limit: 15 for complexity)
- Indentation: Tab-aligned (Go standard)

**Linting:**
- Tool: `golangci-lint` (v2.8.0)
- Config: `.golangci.yml`
- Enabled linters: errcheck, govet (shadow, nilness), ineffassign, staticcheck, unused, gosec, gocyclo, misspell
- Test files excluded from: gocyclo, errcheck, gosec
- Pre-commit hook: Linting is checked before commit in CI

## Import Organization

**Order:**
1. Standard library imports (grouped together)
2. Third-party imports (grouped together, typically cloud SDKs, k8s, controller-runtime)
3. Local project imports (e.g., `github.com/stratos-sh/stratos/...`)

**Path Aliases:**
- None currently in use; explicit imports for clarity
- Full package paths always used: `corev1`, `apierrors`, `ctrl`, `client`

**Example from `reconciler.go`:**
```go
import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
)
```

## Error Handling

**Patterns:**
- Custom error types defined in `internal/cloudprovider/types.go`: `InstanceNotFoundError`, `InvalidStateError`, `RateLimitError`, `QuotaExceededError`, `InsufficientCapacityError`
- All custom errors implement `Error()` string method for error interface compliance
- Errors wrapped with context: `fmt.Errorf("failed to get unschedulable pods: %w", err)`
- Type assertions for cloud provider operations: Cast interface to concrete type with safety check
  ```go
  nodeClass, ok := nc.(*stratosv1alpha1.AWSNodeClass)
  if !ok {
    return nil, fmt.Errorf("expected *AWSNodeClass, got %T", nc)
  }
  ```
- Invalid state transitions validated with `nodestate.IsValidTransition()` before execution
- Kubernetes API errors checked with `apierrors.IsNotFound(err)` and similar predicates

## Logging

**Framework:**
- `sigs.k8s.io/controller-runtime/pkg/log` via `logr` interface
- Logger from context: `logger := log.FromContext(ctx)`

**Patterns:**
- Structured logging with key-value pairs (not format strings)
- Info level: Non-error operational status (e.g., "Adding finalizer to NodePool")
- Error level: Errors with context (e.g., `logger.Error(err, "Failed to update NodeClass lifecycle")`)
- Debug level (V(1)): Verbose information (e.g., `logger.V(1).Info("Could not fetch NodeClass", "error", err)`)
- All log calls in methods starting with context retrieve logger: `logger := log.FromContext(ctx)`

**Example from `strategy/kubernetes.go` line 80:**
```go
logger.Info("Found unschedulable pods", "count", len(pods))
```

## Comments

**When to Comment:**
- Package-level comment required for each package (first line of each `.go` file)
- Exported types/functions: One-line comment at package level explaining purpose
- Non-obvious logic: Comment "why" not "what"
- Complex algorithms: Multi-line explanation of approach

**JSDoc/TSDoc:**
- Not used (Go style comments instead)
- Godoc format for public API documentation:
  ```go
  // CloudProvider defines the interface for cloud instance lifecycle operations.
  // Implementations must be thread-safe for concurrent use.
  type CloudProvider interface {
  ```

**Example from `interface.go` lines 17-42:**
```go
// Package cloudprovider defines the interface for cloud instance operations.
package cloudprovider

// TemplateConfig holds NodePool template configuration for userData generation.
// This includes labels and taints that should be applied to nodes via kubelet flags.
type TemplateConfig struct {
```

## Function Design

**Size:**
- Prefer small, focused functions
- Methods typically 20-100 lines
- Longer methods break at 200 lines (gocyclo min: 15 to discourage overly complex functions)
- Reconcile methods are longest: `NodePoolReconciler.Reconcile()` in `reconciler.go` is ~150 lines (top-level orchestrator)

**Parameters:**
- Context always first after receiver: `func (r *Receiver) Method(ctx context.Context, ...)`
- Errors returned as last return value
- Maximum 3-4 parameters typical; complex configurations use struct types

**Return Values:**
- Errors always last return value
- Kubernetes reconcilers return `(ctrl.Result, error)` by convention
- Many methods return `error` only for side-effect operations
- Cloud provider methods return operation result + error: `(*Instance, error)` or `(InstanceState, error)`

## Module Design

**Exports:**
- Unexported (lowercase) types for internal implementation details
- Exported (uppercase) interfaces and types for public API
- Examples: `CloudProvider` interface (exported), `AWSProvider` struct (exported), `rateLimiter` field (unexported)

**Barrel Files:**
- Not used; each package has clear responsibilities
- Example structure in `internal/controller/`:
  - `reconciler.go` - Main reconciler type and main Reconcile() loop
  - `setup.go` - SetupWithManager() and watchers
  - `validate.go` - Validation functions
  - `cloud_sync.go` - Cloud instance synchronization
  - `queries.go` - Node queries by state/pool
  - Separate subdirectories: `nodestate/`, `strategy/`, `lifecycle/`

## Constants and Conventions

**Finalizer names:** `"stratos.sh/<resource>-finalizer"` (e.g., `"stratos.sh/nodepool-finalizer"`)

**Label/Annotation keys:** `stratos.sh/<name>` (e.g., `stratos.sh/pool`, `stratos.sh/state`)

**Taint keys:** `stratos.sh/<state>` or hardcoded (e.g., `stratos.sh/not-ready`, `stratos.sh/standby`)

**Instance state constants:** Defined in `internal/cloudprovider/types.go` as `InstanceState` string type:
- `InstanceStatePending`, `InstanceStateRunning`, `InstanceStateStopped`, `InstanceStateTerminated`, `InstanceStateUnknown`

**Node state constants:** Defined in `internal/controller/nodestate/nodestate.go` as `NodeState` string type:
- `NodeStateWarmup`, `NodeStateStandby`, `NodeStateRunning`, `NodeStateTerminating`, `NodeStateUnknown`

---

*Convention analysis: 2026-02-02*
