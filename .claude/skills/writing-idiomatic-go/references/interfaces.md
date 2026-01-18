# Interface Design & Dependency Injection

## Single-Function Interfaces

Define interfaces with exactly ONE method:

```go
// Good: Single responsibility
type Counter interface {
    Count(ctx context.Context) (int, error)
}

type Launcher interface {
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
}

type Stopper interface {
    Stop(ctx context.Context, id string) error
}

type StateGetter interface {
    GetState(ctx context.Context, id string) (InstanceState, error)
}
```

```go
// Bad: God interface
type CloudManager interface {
    Count(ctx context.Context) (int, error)
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
    Stop(ctx context.Context, id string) error
    GetState(ctx context.Context, id string) (InstanceState, error)
    Terminate(ctx context.Context, id string) error
}
```

Benefits:
- Maximum composability
- Focused testing with minimal mocks
- Clear separation of concerns
- Functions accept only what they need

## Interface Composition

Compose single-function interfaces for full contracts:

```go
// Combined interface for implementers
type CloudProvider interface {
    Counter
    Launcher
    Stopper
    StateGetter
    Terminator
}

// AWS implements the full interface
type AWSProvider struct {
    ec2 *ec2.Client
}

func (a *AWSProvider) Launch(ctx context.Context, spec InstanceSpec) (string, error) { /* ... */ }
func (a *AWSProvider) Stop(ctx context.Context, id string) error { /* ... */ }
// ... implements all methods
```

## Dependency Injection

### Accept Interfaces, Return Structs

Functions and constructors accept interfaces (behavior) and return concrete types:

```go
// Good: Accept interface, return struct
func NewPoolManager(provider CloudProvider, opts Options) *PoolManager {
    return &PoolManager{
        provider: provider,
        opts:     opts,
    }
}

// Bad: Accept and return interface
func NewPoolManager(provider CloudProvider) PoolManagerInterface {
    return &PoolManager{provider: provider}
}
```

### Accept Minimal Interface

Each function accepts only the interface it needs:

```go
// Good: Function accepts minimal interface
func launchNodes(ctx context.Context, launcher Launcher, count int) ([]string, error) {
    ids := make([]string, 0, count)
    for i := 0; i < count; i++ {
        id, err := launcher.Launch(ctx, defaultSpec)
        if err != nil {
            return ids, err
        }
        ids = append(ids, id)
    }
    return ids, nil
}

// Bad: Function accepts full interface when it only needs Launch
func launchNodes(ctx context.Context, provider CloudProvider, count int) ([]string, error) {
    // Only uses provider.Launch() - over-specified dependency
}
```

### Struct Stores Individual Interfaces

Store the minimal interface each field needs:

```go
type PoolManager struct {
    counter  Counter
    launcher Launcher
    stopper  Stopper
    opts     Options
}

// Constructor accepts composed interface, stores decomposed
func NewPoolManager(p CloudProvider, opts Options) *PoolManager {
    return &PoolManager{
        counter:  p,
        launcher: p,
        stopper:  p,
        opts:     opts,
    }
}
```

Benefits:
- Clear which capabilities each component uses
- Easy to test: mock only what's needed
- Methods can document their actual dependencies

### Testing with Minimal Mocks

```go
// Test only needs to mock Launcher
type mockLauncher struct {
    launchFunc func(ctx context.Context, spec InstanceSpec) (string, error)
}

func (m *mockLauncher) Launch(ctx context.Context, spec InstanceSpec) (string, error) {
    return m.launchFunc(ctx, spec)
}

func TestLaunchNodes(t *testing.T) {
    mock := &mockLauncher{
        launchFunc: func(ctx context.Context, spec InstanceSpec) (string, error) {
            return "i-123", nil
        },
    }

    ids, err := launchNodes(context.Background(), mock, 3)
    // ...
}
```

## Interface Naming

Single-method interfaces use method name + "er" suffix:

| Method | Interface |
|--------|-----------|
| `Launch()` | `Launcher` |
| `Stop()` | `Stopper` |
| `Read()` | `Reader` |
| `Count()` | `Counter` |
| `GetState()` | `StateGetter` |

## Define Interfaces Where Used

Define interfaces in the package that uses them, not the package that implements them:

```go
// pkg/scaler/scaler.go
package scaler

// Define interface here, where it's used
type Launcher interface {
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
}

type Scaler struct {
    launcher Launcher
}
```

```go
// pkg/aws/provider.go
package aws

// Implements Launcher (and other interfaces) without importing scaler
type Provider struct { /* ... */ }

func (p *Provider) Launch(ctx context.Context, spec InstanceSpec) (string, error) {
    // ...
}
```

This inverts the dependency: `scaler` doesn't depend on `aws`.
