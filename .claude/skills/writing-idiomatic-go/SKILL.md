---
name: writing-idiomatic-go
description: |
  Idiomatic Go code patterns and style guidelines. Use when writing or reviewing Go code,
  designing interfaces, structuring packages, or making architectural decisions. Triggers on:
  Go file creation/editing, interface design, dependency injection, package structure,
  code reviews, refactoring tasks, naming decisions.
---

# Writing Idiomatic Go

## Core Principles

1. **Single-function interfaces** - One method per interface, compose for larger contracts
2. **Rule of three** - No abstractions until code duplicates 3 times
3. **Accept interfaces, return structs** - Depend on behavior, provide concrete types
4. **Context first** - Pass `context.Context` as first param for cancellable operations
5. **Delete unused code** - No backward-compatibility shims or commented remnants

## Quick Reference

### Interface Design
```go
// Single-function interface
type Launcher interface {
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
}

// Compose for full contracts
type CloudProvider interface {
    Launcher
    Stopper
    StateGetter
}
```

### Dependency Injection
```go
// Accept minimal interface needed
func NewScaler(launcher Launcher, opts Options) *Scaler {
    return &Scaler{launcher: launcher, opts: opts}
}
```

### Error Handling
```go
result, err := doSomething(ctx)
if err != nil {
    return fmt.Errorf("doing something: %w", err)
}
```

## Detailed Guides

Load these references based on the task:

| Task | Reference |
|------|-----------|
| Designing interfaces, dependency injection | [references/interfaces.md](references/interfaces.md) |
| Deciding when to abstract, cleanup | [references/abstractions.md](references/abstractions.md) |
| Constructor, options, context, concurrency | [references/patterns.md](references/patterns.md) |
| Naming, documentation, package structure | [references/style.md](references/style.md) |
