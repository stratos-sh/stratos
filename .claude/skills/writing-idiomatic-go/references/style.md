# Style Guide

## Package Structure

- Package in `internal/` for project-private code
- One package per directory
- Lowercase, single-word names: `instancepool`, `auth`, `cache`

File organization within a package:
1. Interfaces at top
2. Types/structs
3. Constructor (`New...`)
4. Methods

```go
// pkg/pool/manager.go

package pool

// 1. Interfaces
type Launcher interface {
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
}

// 2. Types
type Manager struct {
    launcher Launcher
    opts     Options
}

type Options struct {
    PoolSize int
    Cooldown time.Duration
}

// 3. Constructor
func NewManager(launcher Launcher, opts Options) *Manager {
    return &Manager{launcher: launcher, opts: opts}
}

// 4. Methods
func (m *Manager) Run(ctx context.Context) error {
    // ...
}
```

## Naming Conventions

### Packages
- Lowercase, single-word: `instancepool`, `auth`, `cache`
- No underscores or mixedCaps
- Short but descriptive

### Constructors
- `NewXxx` returns `*Xxx`

```go
func NewPoolManager(p CloudProvider, opts Options) *PoolManager
```

### Receivers
- Short (1-2 letters), consistent within type
- Use pointer receiver for mutation or large structs
- Use value receiver for small immutable types

```go
func (m *Manager) Run(ctx context.Context) error  // pointer: mutates or large
func (u User) String() string                      // value: no mutation, small
```

### Interfaces
- Single-method interfaces: method name + "er" suffix
- `Reader`, `Writer`, `Closer`, `Launcher`, `Stopper`

### Variables
- Short names in small scopes: `i`, `n`, `err`
- Descriptive names in larger scopes: `userCount`, `instanceID`
- Acronyms: all caps (`ID`, `URL`, `HTTP`)

```go
// Small scope: short names
for i, v := range items {
    // ...
}

// Larger scope: descriptive
func processInstances(instanceIDs []string, maxConcurrent int) {
    // ...
}
```

### Exported vs Unexported
- Exported (uppercase): part of public API
- Unexported (lowercase): internal implementation

```go
type Manager struct {       // Exported: users create this
    provider CloudProvider  // Unexported field: internal detail
}

func (m *Manager) Run() {}  // Exported: public API
func (m *Manager) reconcile() {}  // Unexported: internal
```

## Documentation Comments

Exported identifiers MUST have doc comments:

```go
// PoolManager manages a pool of cloud instances.
// It periodically checks pool size and launches new instances as needed.
type PoolManager struct {
    // ...
}

// NewPoolManager creates a PoolManager with the given cloud provider.
func NewPoolManager(p CloudProvider, opts Options) *PoolManager {
    // ...
}

// Run starts the pool management loop. It blocks until context is cancelled.
func (m *PoolManager) Run(ctx context.Context) error {
    // ...
}
```

Format:
- Start with the identifier name
- Complete sentence with period
- Explain what, not how

## Function Design

- Functions do one thing
- Keep functions small enough to understand at a glance
- Return early on errors
- Use named return values sparingly (only for documentation or defer)

```go
// Good: Early returns, clear flow
func (m *Manager) getNode(ctx context.Context, id string) (*Node, error) {
    if id == "" {
        return nil, errors.New("empty node ID")
    }

    node, err := m.store.Get(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("getting node %s: %w", id, err)
    }

    if node.State == StateTerminated {
        return nil, ErrNodeTerminated
    }

    return node, nil
}
```

```go
// Bad: Nested conditionals, hard to follow
func (m *Manager) getNode(ctx context.Context, id string) (*Node, error) {
    if id != "" {
        node, err := m.store.Get(ctx, id)
        if err == nil {
            if node.State != StateTerminated {
                return node, nil
            } else {
                return nil, ErrNodeTerminated
            }
        } else {
            return nil, fmt.Errorf("getting node: %w", err)
        }
    } else {
        return nil, errors.New("empty node ID")
    }
}
```
