# Abstraction Rules

## Rule of Three

Do NOT create abstractions until code is duplicated 3 times:

```go
// First occurrence: write inline
func ProcessUser(u User) error {
    if u.Email == "" {
        return errors.New("email required")
    }
    // ... process
}

// Second occurrence: still inline, note the duplication
func ProcessAdmin(a Admin) error {
    if a.Email == "" {
        return errors.New("email required")
    }
    // ... process
}

// Third occurrence: NOW extract
func validateEmail(email string) error {
    if email == "" {
        return errors.New("email required")
    }
    return nil
}
```

## No Premature Abstraction

Write concrete code first. Extract abstractions only when:
1. Same code appears 3+ times
2. Clear pattern emerges from real usage
3. Abstraction simplifies, not complicates

```go
// Bad: Premature interface for single implementation
type Processor interface {
    Process(data []byte) error
}

type JSONProcessor struct{}
func (p *JSONProcessor) Process(data []byte) error { /* ... */ }

// Used exactly once - unnecessary abstraction
```

```go
// Good: Direct implementation
func ProcessJSON(data []byte) error {
    // Just write the code
}

// Extract interface later IF multiple implementations needed
```

## When Interfaces Are Justified

Create an interface when:
- Multiple implementations exist (or are clearly imminent)
- Testing requires mocking external dependencies
- Package boundary requires decoupling

```go
// Justified: CloudProvider will have AWS, GCP, Azure implementations
type Launcher interface {
    Launch(ctx context.Context, spec InstanceSpec) (string, error)
}

// Not justified: Only one validator exists
type Validator interface {  // Don't do this
    Validate(input string) error
}
```

## Delete Unused Code

Remove abstractions, interfaces, or helpers that become unused:

```go
// Bad: Keeping dead code
type OldProcessor interface {  // No longer used
    Process() error
}

var _unusedVar = "kept for backward compatibility"  // No

// removed: func oldHelper() {}  // No commented code
```

```go
// Good: Just delete it
// (nothing here - it's gone)
```

No backward-compatibility shims, no `// removed` comments, no renamed `_unused` variables. Git preserves history.

## Avoid Over-Engineering

Signs of over-engineering:
- Wrapper types that just forward calls
- Factories that always return the same type
- Configuration for things that never change
- Interfaces with one implementation and no tests mocking it

```go
// Over-engineered
type UserServiceFactory struct{}

func (f *UserServiceFactory) Create(cfg Config) UserServiceInterface {
    return &UserService{cfg: cfg}
}

// Simple
func NewUserService(cfg Config) *UserService {
    return &UserService{cfg: cfg}
}
```
