---
name: run-tests
description: >
  Run project tests after code changes or when finishing implementation work.
  Triggers: (1) user says "run tests", "test this", "check tests", (2) after finishing
  openspec implementation (opsx:apply completes all tasks), (3) user asks to verify
  changes work, (4) any request to validate code changes with tests.
  When triggered after openspec implementation completion, ALWAYS run ALL tests
  (both unit and integration). When triggered after regular code changes, run the
  appropriate subset based on what changed.
---

# Run Tests

## Environment Setup

Integration tests require envtest binaries. Before running integration tests:

```bash
# Ensure setup-envtest is installed
test -s bin/setup-envtest || GOBIN=$(pwd)/bin go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Export KUBEBUILDER_ASSETS (required for envtest)
export KUBEBUILDER_ASSETS=$(bin/setup-envtest use -p path 2>/dev/null || $(go env GOPATH)/bin/setup-envtest use -p path)
```

If `setup-envtest` is not in `bin/` or `GOPATH/bin`, install it first.

## Test Commands

| Scope | Command |
|-------|---------|
| Unit tests | `go test ./...` |
| Integration tests (envtest) | `KUBEBUILDER_ASSETS="$(...)" go test -v -tags=integration ./tests/integration/...` |
| LocalStack tests | `go test -v -tags=integration -timeout 5m ./internal/cloudprovider/aws/...` |
| All tests | Run unit + integration sequentially |

## When to Run What

### After OpenSpec Implementation Completes (ALL tasks done)

Run ALL tests — unit AND integration. No exceptions.

1. Run unit tests: `go test ./...`
2. Run integration tests with envtest (set up `KUBEBUILDER_ASSETS` first)
3. Report results for both

### After Regular Code Changes

Select tests based on what changed:

| Changed path | Tests to run |
|---|---|
| `api/v1alpha1/` | Unit: `go test ./api/...` + Integration: full suite |
| `internal/controller/` | Unit: `go test ./internal/controller/...` + Integration: full suite |
| `internal/cloudprovider/aws/` | Unit: `go test ./internal/cloudprovider/aws/...` |
| `internal/cloudprovider/fake/` | Integration: full suite (fake is used by integration tests) |
| `internal/cloudprovider/aws/resolver*.go` | Unit + LocalStack if available |
| `tests/integration/` | Integration: full suite |
| `deploy/` or `Makefile` | No code tests needed (but verify `go vet ./...`) |
| Multiple packages or unclear | Run all: unit + integration |

### Focused Test Run

If the user specifies a test name or pattern:

```bash
# Single test
go test -v -run TestSpecificName ./internal/controller/...

# Ginkgo focus (integration)
go test -v -tags=integration ./tests/integration/ -ginkgo.focus="pattern"
```

## Procedure

1. Determine scope (openspec completion = all, otherwise select based on changes)
2. Set up envtest if integration tests are needed
3. Run unit tests first (faster feedback)
4. Run integration tests if needed
5. Report pass/fail summary with counts

If any test fails, show the failure output and stop. Do not mark openspec tasks as complete if tests fail.
