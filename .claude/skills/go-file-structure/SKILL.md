---
name: go-file-structure
description: |
  Guidelines for structuring Go packages and files for AI-assisted development.
  Use when: (1) Creating new Go packages or files, (2) Refactoring existing Go code,
  (3) Deciding whether to split or merge packages/files, (4) Organizing controller-runtime
  or kubebuilder projects. Triggers on: Go file creation/editing, package structure
  decisions, refactoring tasks, "should I split this file" questions.
---

# Go File Structure for AI-Assisted Development

Optimize Go code structure for LLM context efficiency while following Go idioms.

## Core Principle

**Smaller, focused files = less noise in context.** When working on scale-up logic, reading 200 lines of `scale_up.go` beats scanning 1300 lines to find relevant code.

## Package Structure

### When to Merge Packages

Merge when:
- Package A is the only consumer of Package B
- They're tightly coupled (frequent cross-package calls)
- The "separation" is artificial (no clear boundary)

```
# Before: artificial separation
internal/
├── controller/     # orchestrates nodemanager
└── nodemanager/    # only used by controller

# After: merged
internal/
└── controller/     # all node lifecycle + orchestration
```

### When to Keep Packages Separate

Keep separate when:
- Clean interface boundary exists
- Multiple consumers possible
- Different rate of change
- Truly reusable utility

```
# Good separation
internal/
├── controller/      # K8s reconciliation
├── cloudprovider/   # Cloud abstraction (interface-based)
└── metrics/         # Observability (cross-cutting)
```

## File Structure

### Target File Size

| Size | Action |
|------|--------|
| <300 lines | Fine as-is |
| 300-600 lines | Consider splitting if logical domains exist |
| >600 lines | Split into focused files |

### How to Split Files

Split by **logical domain**, not by function type:

```
# Good: split by domain
controller/
├── reconciler.go        # Core reconciler, setup
├── scale_up.go          # Scale-up logic
├── scale_down.go        # Scale-down logic
├── pool_maintenance.go  # Warmup, replenish, monitoring
└── state.go             # Constants, types

# Bad: split by function type
controller/
├── types.go      # All types
├── helpers.go    # All helpers
├── handlers.go   # All handlers
```

### File Naming

- Name files after their domain: `scale_up.go`, `drain.go`
- Keep related code together: `scale_calculator.go` + `scale_calculator_test.go`
- Avoid generic names: `helpers.go`, `utils.go`, `common.go`

## Simplification Patterns

### Struct with Single Method → Closure

```go
// Before: unnecessary struct
type Mapper struct {
    client client.Client
}

func NewMapper(c client.Client) *Mapper {
    return &Mapper{client: c}
}

func (m *Mapper) Map(ctx context.Context, obj client.Object) []Request {
    // uses m.client
}

// After: closure
func mapper(c client.Client) func(context.Context, client.Object) []Request {
    return func(ctx context.Context, obj client.Object) []Request {
        // uses c directly
    }
}
```

### Single-File Package → Merge Into Consumer

If a package has one file and one consumer, merge it:

```go
// Before: drain/ package with one file
internal/drain/drain.go  // only used by controller

// After: part of controller
internal/controller/drain.go
```

## Kubebuilder/Controller-Runtime Projects

Standard structure:

```
internal/
├── controller/           # Reconcilers + domain logic
│   ├── <resource>_controller.go  # Main reconciler
│   ├── <operation>.go            # Domain operations
│   └── helpers.go                # Shared helpers (if needed)
├── cloudprovider/        # Cloud abstraction (if applicable)
│   ├── interface.go
│   ├── aws/
│   └── fake/
└── metrics/              # Prometheus metrics
```

Keep domain logic in `controller/` unless it's truly reusable elsewhere.

## Decision Checklist

Before creating a new package:
- [ ] Is there a clear interface boundary?
- [ ] Will multiple packages consume it?
- [ ] Is it >200 lines of cohesive code?

Before splitting a file:
- [ ] Is it >300 lines?
- [ ] Are there distinct logical domains?
- [ ] Will splits be >100 lines each?

Before merging:
- [ ] Is there only one consumer?
- [ ] Is the separation causing friction?
- [ ] Will the merged result be <800 lines?
