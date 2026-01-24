---
sidebar_position: 1
title: Contributing
description: Guidelines for contributing to Stratos
---

# Contributing to Stratos

Thank you for your interest in contributing to Stratos! This guide covers how to get started with development.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Docker (for building images)
- kubectl configured with a Kubernetes cluster
- make

### Clone the Repository

```bash
git clone https://github.com/stratos-sh/stratos.git
cd stratos
```

### Build

```bash
# Build the binary
make build

# Build the container image
make docker-build
```

### Run Tests

```bash
# Unit tests
make test

# Integration tests (requires envtest)
make test-integration

# Generate coverage report
make coverage
```

## Development Workflow

### 1. Fork and Branch

1. Fork the repository on GitHub
2. Create a feature branch:
   ```bash
   git checkout -b feature/my-feature
   ```

### 2. Make Changes

Follow the coding guidelines below when making changes.

### 3. Run Quality Checks

```bash
# Format code
make fmt

# Run linter
make lint

# Run vet
make vet

# Run all tests
make test
```

### 4. Regenerate Code (if needed)

If you modify CRD types in `api/v1alpha1/`:

```bash
# Regenerate deepcopy methods
make generate

# Regenerate CRD and RBAC manifests
make manifests
```

### 5. Submit a Pull Request

1. Push your branch to your fork
2. Open a pull request against `main`
3. Fill out the PR template
4. Wait for CI checks to pass

## Code Organization

```
cmd/stratos/main.go           # Entry point
api/v1alpha1/                 # CRD types (kubebuilder markers)
internal/
├── controller/               # Kubernetes reconcilers
├── cloudprovider/            # Cloud abstraction layer
└── metrics/                  # Prometheus metrics
config/
├── crd/                      # Generated CRD manifests
├── rbac/                     # Generated RBAC manifests
└── samples/                  # Example NodePool resources
tests/
└── integration/              # Integration tests
```

## Coding Guidelines

### Go Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Follow [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Use `gofmt` and `goimports` for formatting

### Error Handling

- Always handle errors explicitly
- Use wrapped errors with context:
  ```go
  return fmt.Errorf("failed to start instance %s: %w", instanceID, err)
  ```
- Define sentinel errors for expected conditions:
  ```go
  var ErrInstanceNotFound = errors.New("instance not found")
  ```

### Logging

- Use structured logging with key-value pairs:
  ```go
  log.Info("scaling up", "pool", poolName, "count", count)
  ```
- Log at appropriate levels:
  - `Error`: Unexpected failures
  - `Info`: Significant operations
  - `Debug`: Detailed debugging information

### Testing

- Write unit tests for all exported functions
- Use table-driven tests where appropriate
- Mock external dependencies (cloud providers, Kubernetes API)

### Comments

- Document exported types and functions
- Explain *why*, not *what* the code does
- Keep comments up to date with code changes

## Pull Request Guidelines

### PR Title

Use conventional commit format:

- `feat: Add support for GCP instances`
- `fix: Handle rate limit errors in AWS provider`
- `docs: Update installation guide`
- `refactor: Simplify scale calculator logic`
- `test: Add integration tests for scale-down`

### PR Description

Include:

1. **What**: Brief description of changes
2. **Why**: Motivation for the changes
3. **How**: Implementation approach
4. **Testing**: How you tested the changes

### Review Process

1. All PRs require at least one approval
2. CI checks must pass
3. Address review feedback promptly
4. Squash commits before merge (or use squash merge)

## Adding a New Cloud Provider

To add support for a new cloud provider:

1. **Implement the interface:**
   ```go
   // internal/cloudprovider/newprovider/provider.go
   type Provider struct {
       // ...
   }

   func (p *Provider) LaunchInstance(ctx context.Context, cfg *cloudprovider.LaunchConfig) (*cloudprovider.Instance, error) {
       // Implementation
   }
   // ... implement all interface methods
   ```

2. **Register in the factory:**
   ```go
   // internal/cloudprovider/factory.go
   func NewProvider(name string) (cloudprovider.CloudProvider, error) {
       switch name {
       case "aws":
           return aws.NewProvider()
       case "newprovider":
           return newprovider.NewProvider()
       // ...
       }
   }
   ```

3. **Add CRD types:**
   ```go
   // api/v1alpha1/cloudprovider_types.go
   type NewProviderConfig struct {
       // Configuration fields
   }
   ```

4. **Add documentation:**
   - Update `docs/concepts/cloud-providers.md`
   - Add setup guide in `docs/guides/`

5. **Add tests:**
   - Unit tests for provider methods
   - Integration tests with fake/mock services

## Reporting Issues

### Bug Reports

Include:

1. Stratos version
2. Kubernetes version
3. Cloud provider and region
4. Steps to reproduce
5. Expected vs actual behavior
6. Controller logs (if applicable)

### Feature Requests

Include:

1. Use case description
2. Proposed solution
3. Alternatives considered
4. Potential impact on existing features

## Code of Conduct

Be respectful and constructive in all interactions. We are committed to providing a welcoming and inclusive environment for all contributors.

## License

By contributing to Stratos, you agree that your contributions will be licensed under the Apache License 2.0.

## Next Steps

- [Local Development](./local-development.md) - Running Stratos locally
- [Testing](./testing.md) - Testing patterns and practices
