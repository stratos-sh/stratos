---
sidebar_position: 3
title: Testing
description: Testing patterns and practices for Stratos
---

# Testing

This guide covers testing patterns and practices used in Stratos development.

## Test Categories

### Unit Tests

Unit tests verify individual functions and methods in isolation.

**Location:** `*_test.go` files alongside source code

**Run:**
```bash
make test
# or
go test ./...
```

### Integration Tests

Integration tests verify component interactions using envtest (in-memory Kubernetes API server).

**Location:** `tests/integration/`

**Run:**
```bash
make test-integration
# or
go test -tags=integration ./tests/integration/...
```

## Writing Unit Tests

### Basic Test Structure

```go
func TestCalculateScaleUp(t *testing.T) {
    tests := []struct {
        name           string
        pendingPods    int
        nodeCapacity   int
        currentStandby int
        expected       int
    }{
        {
            name:           "single pod needs one node",
            pendingPods:    1,
            nodeCapacity:   10,
            currentStandby: 5,
            expected:       1,
        },
        {
            name:           "multiple pods need multiple nodes",
            pendingPods:    25,
            nodeCapacity:   10,
            currentStandby: 5,
            expected:       3,
        },
        {
            name:           "capped by standby availability",
            pendingPods:    100,
            nodeCapacity:   10,
            currentStandby: 2,
            expected:       2,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := calculateScaleUp(tt.pendingPods, tt.nodeCapacity, tt.currentStandby)
            if result != tt.expected {
                t.Errorf("expected %d, got %d", tt.expected, result)
            }
        })
    }
}
```

### Testing with Mocks

Use interfaces for dependencies to enable mocking:

```go
// Production code
type CloudProvider interface {
    StartInstance(ctx context.Context, instanceID string) error
}

type NodeManager struct {
    cloud CloudProvider
}

// Test code
type mockCloudProvider struct {
    startInstanceFunc func(ctx context.Context, instanceID string) error
}

func (m *mockCloudProvider) StartInstance(ctx context.Context, instanceID string) error {
    if m.startInstanceFunc != nil {
        return m.startInstanceFunc(ctx, instanceID)
    }
    return nil
}

func TestNodeManager_ScaleUp(t *testing.T) {
    mock := &mockCloudProvider{
        startInstanceFunc: func(ctx context.Context, instanceID string) error {
            if instanceID != "i-expected" {
                t.Errorf("unexpected instance ID: %s", instanceID)
            }
            return nil
        },
    }

    nm := &NodeManager{cloud: mock}
    err := nm.ScaleUp(context.Background(), "i-expected")
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
}
```

## Using the Fake Provider

The fake provider (`internal/cloudprovider/fake/`) supports hooks for testing:

```go
import "github.com/stratos-sh/stratos/internal/cloudprovider/fake"

func TestWithFakeProvider(t *testing.T) {
    provider := fake.NewProvider()

    // Configure hooks
    provider.OnLaunchInstance = func(cfg *cloudprovider.LaunchConfig) (*cloudprovider.Instance, error) {
        // Custom behavior
        return &cloudprovider.Instance{
            ID:    "i-fake-123",
            State: cloudprovider.InstanceStateRunning,
        }, nil
    }

    // Use provider in tests
    instance, err := provider.LaunchInstance(context.Background(), &cloudprovider.LaunchConfig{
        InstanceType: "m5.large",
    })
    // ...
}
```

## Integration Tests with envtest

### Setup

```go
import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

var testEnv *envtest.Environment

func TestControllers(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "config", "crd", "bases"),
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    // ... setup client
})

var _ = AfterSuite(func() {
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### Writing Integration Tests

```go
var _ = Describe("NodePool Controller", func() {
    Context("when creating a NodePool", func() {
        It("should create standby nodes", func() {
            nodePool := &stratosv1alpha1.NodePool{
                ObjectMeta: metav1.ObjectMeta{
                    Name: "test-pool",
                },
                Spec: stratosv1alpha1.NodePoolSpec{
                    PoolSize:   5,
                    MinStandby: 3,
                    // ...
                },
            }

            Expect(k8sClient.Create(ctx, nodePool)).Should(Succeed())

            Eventually(func() int32 {
                np := &stratosv1alpha1.NodePool{}
                k8sClient.Get(ctx, types.NamespacedName{Name: "test-pool"}, np)
                return np.Status.Standby
            }, timeout, interval).Should(Equal(int32(3)))
        })
    })
})
```

## Test Patterns

### Testing State Transitions

```go
func TestValidTransitions(t *testing.T) {
    tests := []struct {
        from  NodeState
        to    NodeState
        valid bool
    }{
        {NodeStateWarmup, NodeStateStandby, true},
        {NodeStateWarmup, NodeStateRunning, false},
        {NodeStateStandby, NodeStateRunning, true},
        {NodeStateRunning, NodeStateTerminating, true},
        {NodeStateRunning, NodeStateWarmup, false},
    }

    for _, tt := range tests {
        t.Run(fmt.Sprintf("%s->%s", tt.from, tt.to), func(t *testing.T) {
            result := IsValidTransition(tt.from, tt.to)
            if result != tt.valid {
                t.Errorf("expected %v, got %v", tt.valid, result)
            }
        })
    }
}
```

### Testing Error Conditions

```go
func TestScaleUp_InsufficientStandby(t *testing.T) {
    provider := fake.NewProvider()
    // No standby nodes available

    manager := NewNodeManager(provider)
    err := manager.ScaleUp(context.Background(), 5)

    if err == nil {
        t.Error("expected error for insufficient standby")
    }
    if !errors.Is(err, ErrInsufficientStandby) {
        t.Errorf("expected ErrInsufficientStandby, got %v", err)
    }
}
```

### Testing Timeouts

```go
func TestWarmupTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    provider := fake.NewProvider()
    provider.OnGetInstanceState = func(instanceID string) (cloudprovider.InstanceState, error) {
        // Simulate instance that never stops
        return cloudprovider.InstanceStateRunning, nil
    }

    err := waitForWarmup(ctx, provider, "i-123")

    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected timeout error, got %v", err)
    }
}
```

## Test Coverage

### Generate Coverage Report

```bash
make coverage
```

This creates an HTML coverage report at `coverage.html`.

### View Coverage in Terminal

```bash
go test -cover ./...
```

### Coverage Targets

- **Target:** 70%+ overall coverage
- **Critical paths:** 90%+ coverage (reconciliation, state transitions)
- **Error handling:** All error conditions should be tested

## Continuous Integration

Tests run automatically on pull requests:

1. **Lint:** `make lint`
2. **Unit tests:** `make test`
3. **Integration tests:** `make test-integration`

### CI Configuration

```yaml
# .github/workflows/test.yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: make lint
      - run: make test
      - run: make test-integration
```

## Best Practices

### Test Independence

- Each test should be independent
- Don't rely on test execution order
- Clean up resources after tests

### Meaningful Names

```go
// Good
func TestScaleUp_WithInsufficientStandby_ReturnsError(t *testing.T)

// Bad
func TestScaleUp1(t *testing.T)
```

### Test One Thing

Each test should verify one specific behavior:

```go
// Good - single responsibility
func TestScaleUp_CalculatesCorrectNodeCount(t *testing.T) { ... }
func TestScaleUp_RespectsPoolSize(t *testing.T) { ... }
func TestScaleUp_HandlesCloudErrors(t *testing.T) { ... }

// Bad - tests multiple things
func TestScaleUp(t *testing.T) {
    // Tests calculation, pool size, and errors all in one
}
```

### Use Helpers

Extract common setup into helper functions:

```go
func newTestNodePool(name string, poolSize, minStandby int32) *stratosv1alpha1.NodePool {
    return &stratosv1alpha1.NodePool{
        ObjectMeta: metav1.ObjectMeta{
            Name: name,
        },
        Spec: stratosv1alpha1.NodePoolSpec{
            PoolSize:   poolSize,
            MinStandby: minStandby,
            // ... common defaults
        },
    }
}
```

## Next Steps

- [Contributing](./contributing.md) - Contribution guidelines
- [Local Development](./local-development.md) - Running locally
