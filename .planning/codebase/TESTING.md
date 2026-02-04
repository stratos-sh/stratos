# Testing Patterns

**Analysis Date:** 2026-02-02

## Test Framework

**Runner:**
- `testing.T` (Go standard testing)
- `github.com/onsi/ginkgo/v2` for integration tests (BDD-style test suites)
- `github.com/onsi/gomega` for assertions (fluent matchers)

**Config:**
- No separate config file; tests use build tags: `//go:build integration` and `// +build integration`
- Integration tests require Kubernetes test environment setup via `envtest`

**Run Commands:**
```bash
make test                          # Run all unit tests with coverage
make test-integration              # Run all integration tests (requires envtest setup)
make test-integration TEST=NodePoolLifecycle  # Run specific integration test suite
make test-localstack               # Run AWS integration tests with LocalStack
make coverage                      # Generate HTML coverage report
go test -v -run TestSpecificName ./internal/controller/...  # Run single unit test
```

## Test File Organization

**Location:**
- Unit tests: Co-located with source code (same package)
  - Example: `internal/cloudprovider/aws/instance_types_test.go` next to `instance_types.go`
- Integration tests: Separate `tests/integration/` directory
  - All integration tests in one package: `package integration`
  - Share common test setup via `suite_test.go`
- E2E tests: `tests/e2e/` directory (requires live EKS cluster)

**Naming:**
- Unit test files: `<source>_test.go` (e.g., `instance_types_test.go`)
- Integration test files: `<feature>_test.go` (e.g., `nodepool_test.go`, `scale_up_test.go`)
- Test functions: `TestFeatureName(t *testing.T)` for unit tests
- Ginkgo test suites: `Describe("Feature Name", func() { ... })`

**Structure:**
```
tests/integration/
├── suite_test.go              # Shared test setup (BeforeSuite, AfterSuite)
├── helpers_test.go            # Helper functions (testAWSNodeClass, createTestNodePool, etc.)
├── nodepool_test.go           # Feature tests (Ginkgo suites)
├── scale_up_test.go
├── scale_down_test.go
├── error_handling_test.go
└── ...
```

## Test Structure

**Suite Organization:**

Integration tests use Ginkgo with suite-level setup/teardown:

```go
//go:build integration
// +build integration

package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	// Setup: bootstrap test environment, create clients, start manager
})

var _ = AfterSuite(func() {
	// Cleanup: stop environment, cancel context
})

var _ = BeforeEach(func() {
	// Reset state before each test (e.g., fakeProvider.Reset())
})

var _ = Describe("Feature Name", func() {
	Context("When condition", func() {
		It("should do something", func() {
			// Arrange: create test resources
			np := createTestNodePool("test-pool", 5, 2)

			// Act: trigger action
			triggerReconcile(np.Name)
			time.Sleep(200 * time.Millisecond)

			// Assert: verify outcome
			Eventually(func() bool {
				// Check state
				return true
			}, timeout, interval).Should(BeTrue())
		})
	})
})
```

**Patterns:**

1. **Setup:** `suite_test.go` uses `BeforeSuite` to:
   - Create in-memory K8s API server via `envtest.Environment`
   - Load CRD manifests from `deploy/charts/stratos/crds`
   - Create Kubernetes client and controller manager
   - Instantiate fake cloud provider and reconcilers
   - Start manager in background goroutine

2. **Teardown:** `AfterSuite` stops the test environment and cancels context

3. **Per-test Reset:** `BeforeEach` calls `fakeProvider.Reset()` to clear instance state between tests

4. **Assertions:** Gomega matchers with `Eventually()` for async operations:
   ```go
   Eventually(func() bool { return condition }, timeout, interval).Should(BeTrue())
   ```

**Unit Test Example from `instance_types_test.go`:**
```go
func TestGetInstanceCapacity(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		wantCPU      int64
		wantMemGi    int64
		wantZero     bool
	}{
		{name: "m5.xlarge", instanceType: "m5.xlarge", wantCPU: 4000, wantMemGi: 16},
		{name: "unknown instance type", instanceType: "unknown.type", wantZero: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := GetInstanceCapacity(tt.instanceType)
			if tt.wantZero {
				if !cap.IsZero() {
					t.Errorf("expected zero, got CPU=%v", cap.CPU.String())
				}
				return
			}
			if cap.CPU.MilliValue() != tt.wantCPU {
				t.Errorf("CPU = %d, want %d", cap.CPU.MilliValue(), tt.wantCPU)
			}
		})
	}
}
```

## Mocking

**Framework:**
- No external mocking library; interfaces used for dependency injection
- Fake provider with hooks pattern for testing

**Patterns:**

1. **Fake Cloud Provider:** `internal/cloudprovider/fake/provider.go`
   - Implements `CloudProvider` interface
   - Maintains in-memory instance map: `instances map[string]*cloudprovider.Instance`
   - Supports hook injection for error simulation:
     ```go
     type FakeProvider struct {
       mu         sync.RWMutex
       instances  map[string]*cloudprovider.Instance
       LaunchHook func(ctx context.Context, ...) error
       StartHook  func(ctx context.Context, instanceID string) error
       StopHook   func(ctx context.Context, instanceID string, force bool) error
       TerminateHook func(ctx context.Context, instanceID string) error
     }
     ```
   - Hooks called before operation execution, allowing error injection

2. **Hook Usage in Tests from `error_handling_test.go` lines 55-57:**
   ```go
   fakeProvider.LaunchHook = func(ctx context.Context, nodeClass stratosv1alpha1.NodeClass,
       poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) error {
     return fmt.Errorf("simulated EC2 launch failure")
   }
   ```

3. **Dependency Injection:** Reconciler receives cloud provider via:
   ```go
   reconciler.InjectCloudProvider(poolName, fakeProvider)
   ```
   See `tests/integration/suite_test.go` line 115

4. **Fake Resolver:** `internal/cloudprovider/fake/resolver.go` implements `Resolver` interface for AWSNodeClass resolution
   - Used in tests to provide resolved subnets/security groups without AWS calls
   - Injected into AWSNodeClassReconciler: See `suite_test.go` line 122

**What to Mock:**
- Cloud provider operations (launch, start, stop, terminate)
- AWS API calls (when not testing AWS-specific logic)
- External dependencies that are slow or unreliable

**What NOT to Mock:**
- Kubernetes API server (use `envtest` instead)
- Core business logic (state machines, reconciliation loops)
- Concrete implementations of public interfaces

## Fixtures and Factories

**Test Data:**
Helper functions in `tests/integration/helpers_test.go`:

```go
// testAWSNodeClass returns a minimal AWSNodeClass for testing
func testAWSNodeClass(name string) *stratosv1alpha1.AWSNodeClass {
	return &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			Region:       "us-east-1",
			InstanceType: "m5.large",
			SubnetIDs:    []string{"subnet-12345678"},
			SecurityGroupIDs: []string{"sg-12345678"},
		},
	}
}

// createTestNodePool creates a NodePool with dependencies
func createTestNodePool(name string, poolSize, minStandby int32) *stratosv1alpha1.NodePool {
	createTestAWSNodeClass(name + "-nodeclass")  // Dependency
	np := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   poolSize,
			MinStandby: minStandby,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: name + "-nodeclass",
				},
			},
		},
	}
	reconciler.InjectCloudProvider(name, fakeProvider)
	err := k8sClient.Create(ctx, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}

// simulateNodeJoin creates a K8s Node for a fake EC2 instance
func simulateNodeJoin(poolName, instanceID string, nodeState nodestate.NodeState) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				nodestate.LabelPool:       poolName,
				nodestate.LabelState:      string(nodeState),
				nodestate.LabelInstanceID: instanceID,
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: nodeState != nodestate.NodeStateRunning,
		},
	}
	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}
```

**Location:**
- `tests/integration/helpers_test.go` - All helper factories for integration tests
- No separate fixtures directory; fixtures created dynamically in tests

## Coverage

**Requirements:**
- No minimum coverage enforced
- Coverage report generated with `make coverage` → `coverage.html`

**View Coverage:**
```bash
make coverage                    # Generates coverage.html
go tool cover -html=coverage.out  # View existing coverage
```

**Coverage Gaps:**
- Integration tests not included in coverage report (measured separately)
- E2E tests require live cluster (no coverage tracking)
- See `Makefile` lines 60-61 for unit test coverage invocation

## Test Types

**Unit Tests:**
- Scope: Single function or small logical unit
- No external dependencies (use mocks/fakes)
- Location: `<package>/<file>_test.go` (co-located with source)
- Examples: `internal/cloudprovider/aws/instance_types_test.go` (capacity lookup)
- Run: `make test` or `go test ./...`

**Integration Tests:**
- Scope: Controller reconciliation logic with Kubernetes API server
- Dependencies: In-memory K8s API server (`envtest`), fake cloud provider
- Location: `tests/integration/*_test.go`
- Examples:
  - `nodepool_test.go` - NodePool lifecycle, finalizers, status updates
  - `scale_up_test.go` - Scale-up triggered by unschedulable pods
  - `scale_down_test.go` - Scale-down when nodes empty
  - `error_handling_test.go` - Error recovery and resilience
  - `state_transitions_test.go` - Valid/invalid node state transitions
- Run: `make test-integration` or `make test-integration TEST=TestName`
- Build tag requirement: `//go:build integration` at top of file

**E2E Tests:**
- Scope: Full system testing against live EKS cluster
- Location: `tests/e2e/e2e_test.go`
- Requirements: AWS credentials, EKS cluster, 20-minute timeout
- Run: `make test-e2e`
- Currently: Spot replacement tests removed (feature under development)

## Common Patterns

**Async Testing with Eventually:**
```go
timeout := 30 * time.Second
interval := 100 * time.Millisecond

Eventually(func() bool {
	updated := &stratosv1alpha1.NodePool{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: np.Name}, updated)
	if err != nil {
		return false
	}
	// Check condition on latest state
	return updated.Status.RunningCount == 3
}, timeout, interval).Should(BeTrue())
```

**Error Testing:**
From `error_handling_test.go` lines 51-67:
```go
It("should handle launch instance failure", func() {
	np := createTestNodePool("test-launch-error", 5, 2)

	// Inject error hook
	fakeProvider.LaunchHook = func(...) error {
		return fmt.Errorf("simulated EC2 launch failure")
	}

	// Trigger operation
	triggerReconcile(np.Name)
	time.Sleep(500 * time.Millisecond)

	// Verify pool still exists and reconciles (doesn't crash)
	updated := getNodePool(np.Name)
	Expect(updated).NotTo(BeNil())
})
```

**State Verification:**
```go
// Verify node has correct label
Expect(node.Labels[nodestate.LabelState]).To(Equal(string(nodestate.NodeStateRunning)))

// Verify instance state in fake provider
instance, err := fakeProvider.GetInstance(ctx, instanceID)
Expect(err).NotTo(HaveOccurred())
Expect(instance.State).To(Equal(cloudprovider.InstanceStateRunning))
```

**Test Variable Setup from `suite_test.go` lines 48-58:**
```go
var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	ctx         context.Context
	cancel      context.CancelFunc
	testScheme  *runtime.Scheme
	fakeProvider *fake.FakeProvider
	fakeResolver *fake.FakeResolver
	reconciler  *controller.NodePoolReconciler
	mgr         ctrl.Manager
)

const (
	testClusterName = "test-cluster"
	timeout         = 30 * time.Second
	interval        = 100 * time.Millisecond
)
```

---

*Testing analysis: 2026-02-02*
