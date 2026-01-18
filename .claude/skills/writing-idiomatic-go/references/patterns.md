# Go Patterns

## Constructor Pattern

```go
func NewManager(provider CloudProvider, opts Options) *Manager {
    return &Manager{
        provider: provider,
        opts:     opts,
    }
}
```

Naming: `NewXxx` returns `*Xxx`.

## Options Pattern

Group configuration in a struct:

```go
type ManagerOptions struct {
    PoolSize      int
    MaxConcurrent int
    Cooldown      time.Duration
}

func NewManager(provider CloudProvider, opts ManagerOptions) *Manager {
    return &Manager{
        provider: provider,
        opts:     opts,
    }
}
```

For optional configuration with defaults, use functional options:

```go
type Option func(*Manager)

func WithPoolSize(size int) Option {
    return func(m *Manager) {
        m.poolSize = size
    }
}

func WithCooldown(d time.Duration) Option {
    return func(m *Manager) {
        m.cooldown = d
    }
}

func NewManager(provider CloudProvider, opts ...Option) *Manager {
    m := &Manager{
        provider: provider,
        poolSize: 10,           // default
        cooldown: time.Minute,  // default
    }
    for _, opt := range opts {
        opt(m)
    }
    return m
}

// Usage
m := NewManager(provider, WithPoolSize(20), WithCooldown(30*time.Second))
```

## Context Passing

Pass `context.Context` as first parameter when function:
- Makes network calls
- Does I/O operations
- Can be cancelled
- Runs for extended time

```go
// Good: Context first
func (m *Manager) Run(ctx context.Context) error {
    for {
        select {
        case <-time.After(m.cooldown):
            if err := m.reconcile(ctx); err != nil {
                return err
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func FetchUser(ctx context.Context, id string) (*User, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", "/users/"+id, nil)
    // ...
}
```

```go
// Bad: Missing context - no way to cancel
func (m *Manager) Run() error {
    for {
        time.Sleep(m.cooldown)
        m.reconcile()  // Can't cancel, can't timeout
    }
}
```

## Error Handling

Return errors as last value, check immediately:

```go
result, err := doSomething(ctx)
if err != nil {
    return fmt.Errorf("doing something: %w", err)
}
```

Wrap errors with context using `%w`:

```go
func (m *Manager) LaunchNode(ctx context.Context, spec NodeSpec) (string, error) {
    id, err := m.provider.Launch(ctx, spec)
    if err != nil {
        return "", fmt.Errorf("launching node with spec %v: %w", spec, err)
    }
    return id, nil
}
```

## Concurrency Patterns

### Goroutines with Context

Always ensure goroutines can exit:

```go
func (m *Manager) Start(ctx context.Context) {
    go func() {
        for {
            select {
            case <-ctx.Done():
                return  // Always have exit path
            case work := <-m.workChan:
                m.process(work)
            }
        }
    }()
}
```

### Worker Pool

```go
func (m *Manager) processAll(ctx context.Context, items []Item) error {
    g, ctx := errgroup.WithContext(ctx)
    sem := make(chan struct{}, m.maxConcurrent)

    for _, item := range items {
        item := item  // Capture for goroutine
        g.Go(func() error {
            select {
            case sem <- struct{}{}:
                defer func() { <-sem }()
            case <-ctx.Done():
                return ctx.Err()
            }
            return m.process(ctx, item)
        })
    }

    return g.Wait()
}
```

### Periodic Loop

```go
func (m *Manager) runLoop(ctx context.Context) error {
    ticker := time.NewTicker(m.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            if err := m.reconcile(ctx); err != nil {
                // Log error, continue or return based on severity
                log.Printf("reconcile error: %v", err)
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

## Graceful Shutdown

```go
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    mgr := NewManager(provider, opts)

    if err := mgr.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
        log.Fatalf("manager error: %v", err)
    }
}
```
