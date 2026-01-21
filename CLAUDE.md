# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

## Project Overview

Stratos is a Kubernetes operator that eliminates cloud instance cold-start delays by maintaining pools of pre-warmed, stopped instances ready to start in seconds. Built on controller-runtime (kubebuilder pattern).

## Common Commands

```bash
# Build & Run
make build                    # Build binary (includes fmt, vet)
make run                      # Run controller locally against kubeconfig
make docker-build             # Build container image

# Testing
make test                     # Unit tests with race detection & coverage
make test-integration         # Integration tests with envtest
make coverage                 # Generate HTML coverage report

# Code Quality
make lint                     # Run golangci-lint
make fmt                      # Format code
make vet                      # Run go vet

# Code Generation (run after modifying api/v1alpha1/*.go)
make generate                 # Generate deepcopy methods
make manifests                # Generate CRD and RBAC manifests

# Deployment
make install                  # Install CRDs to cluster
make deploy                   # Deploy controller to cluster
```

Run a single test:
```bash
go test -v -run TestSpecificName ./internal/controller/...
```

## Architecture

```
cmd/stratos/main.go           # Entry point, manager setup, flags
api/v1alpha1/                 # NodePool CRD types (kubebuilder markers for RBAC/CRD gen)
internal/
├── controller/               # Kubernetes reconcilers
│   ├── nodepool_controller.go    # Main reconciliation loop
│   └── pod_watcher.go            # Detects pending pods for scale-up
├── cloudprovider/            # Cloud abstraction layer
│   ├── interface.go              # CloudProvider interface (all cloud ops go through this)
│   ├── aws/provider.go           # AWS EC2 implementation with rate limiting
│   └── fake/provider.go          # Mock provider for testing
├── nodemanager/              # Node lifecycle (state transitions, labels)
├── drain/                    # Graceful node eviction (respects PDBs)
└── metrics/                  # Prometheus metrics
config/
├── crd/bases/                # Generated CRD manifests
├── rbac/                     # Generated RBAC manifests
└── samples/                  # Example NodePool resources
specs/001-instance-pool-manager/  # Feature specification and design docs
```

**Key patterns:**
- Event-driven reconciliation with 30s periodic maintenance loop
- CloudProvider interface abstracts all instance operations (launch, start, stop, terminate)
- Use fake provider for local development: `--cloud-provider=fake`
- Node state tracked via labels: `stratos.sh/pool`, `stratos.sh/state`

## Key Controller Flags

```bash
--cluster-name=<name>         # Required (or CLUSTER_NAME env var)
--cloud-provider=aws|fake     # Default: aws
--sync-period=30s             # Reconciliation interval
--metrics-bind-address=:8080
--health-probe-bind-address=:8081
```

## Running the Controller Locally

**IMPORTANT:** Always use `go run` to run the controller locally. Never build a separate binary (`/tmp/stratos`, etc.) as it makes it harder to track running processes.

```bash
# Run controller locally (standard way)
go run ./cmd/stratos/main.go --cluster-name=main --cloud-provider=aws

# Before starting, always check for and kill any existing controller
pkill -f "cmd/stratos/main.go" 2>/dev/null
ps aux | grep -E "main.*--cluster-name" | grep -v grep

# Check if controller is running (go run shows as 'main' in process list)
ps aux | grep -E "main.*--cluster-name" | grep -v grep
```

The controller process appears as `main --cluster-name=...` in the process list, not as `stratos`.

## Linting

golangci-lint configured with: errcheck, gosimple, govet (shadow, nilness), staticcheck, unused, gosec, gocyclo (min: 15), misspell. Test files excluded from gocyclo, errcheck, gosec.

## Context7 MCP

Always use Context7 MCP proactively for third-party library documentation (controller-runtime, AWS SDK, client-go, etc.) without waiting for explicit instruction.
