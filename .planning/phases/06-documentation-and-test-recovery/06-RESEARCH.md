# Phase 6: Documentation and Test Recovery - Research

**Researched:** 2026-02-03
**Domain:** Go package documentation (doc.go), test validation
**Confidence:** HIGH

## Summary

Phase 6 wraps up the 6-phase restructure with two requirements: QUAL-02 (doc.go for every package under internal/) and QUAL-03 (integration tests compile and pass). Research reveals that both requirements are nearly satisfied already, making this phase primarily mechanical.

All 15 packages under `internal/` already have a `// Package ...` comment on an existing source file, though 2 packages (`config` and `githubactions`) lack the comment entirely. No `doc.go` files exist anywhere in the project yet. The work is to create dedicated `doc.go` files with expanded descriptions of purpose, responsibilities, key types, and relationship to other packages in the dependency graph.

Tests are already green. `make test` passes (all unit tests). `make test-integration` passes 71/72 specs (1 skipped due to envtest CEL limitation). Three consecutive fresh runs of integration tests all pass without flakes. `make lint` reports zero issues.

**Primary recommendation:** This phase is pure mechanical documentation work plus a verification pass. Split into one plan for doc.go creation and one plan for test verification/certification. Both should be fast.

## Standard Stack

No new libraries needed. This phase uses only Go's built-in documentation conventions.

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| Go doc comments | Go 1.x | Package documentation via godoc | Built into the language, rendered by pkg.go.dev |
| go test | Go 1.x | Test execution | Standard Go toolchain |
| envtest | 0.17 | Integration test environment | Already configured in Makefile |
| Ginkgo/Gomega | v2 | BDD test framework | Already used by integration tests |

### Supporting
| Tool | Purpose | When to Use |
|------|---------|-------------|
| `go doc ./internal/...` | Verify doc.go renders correctly | After creating each doc.go |
| `make lint` | Verify no regressions | After all changes |

## Architecture Patterns

### doc.go File Structure

Every doc.go follows this exact structure:

```
[license header]

// Package [name] [first sentence: what the package does].
//
// [Expanded paragraph: responsibilities and scope]
//
// [Optional paragraph: key types and their roles]
//
// [Optional paragraph: relationship to other packages]
package [name]
```

**Rules from Go doc comment specification:**
- First sentence begins with "Package [name]" -- this is the synopsis shown in directory listings
- No blank line between the comment and the `package` declaration
- Subsequent paragraphs separated by blank comment lines (`//`)
- Use complete sentences
- Reference exported types/functions by name (rendered as links in godoc)
- The license header goes ABOVE the doc comment, separated by a blank line

### Packages Requiring doc.go Files (15 total)

```
internal/
  cloudprovider/              # Interface + types (CloudProvider, Instance, LaunchConfig)
    aws/                      # AWS EC2 implementation (AWSProvider, AWSNodeClassReconciler)
    fake/                     # Test double (FakeProvider, FakeResolver)
  config/                     # Controller + cluster config (Config, ClusterConfig)
  controller/                 # Aggregator setup.go only (Setup)
    nodeclass/                # AWSNodeClass reconciler (Reconciler)
    nodepool/                 # NodePool reconciler (Reconciler, plus all scaling/status)
      lifecycle/              # Node lifecycle operations (Manager)
      nodestate/              # State constants + transitions (NodeState, ValidTransition)
  github/                     # GitHub Actions REST client (Client, WorkflowJob)
  metrics/                    # Prometheus metrics (NodesTotal, ScaleUpTotal, etc.)
  strategy/                   # ScalingStrategy interface
    githubactions/            # GitHub Actions strategy implementation (Strategy)
    kubernetes/               # Kubernetes pod-demand strategy (Strategy)
```

### Existing Package Comments (Current State)

Every package already has a one-line `// Package ...` comment on some source file. These will be REPLACED by the doc.go (the comment must appear in exactly one file per Go convention).

| Package | Current Location | Has Comment | Needs Expansion |
|---------|-----------------|-------------|-----------------|
| cloudprovider | interface.go:17 | Yes | Yes - add key types, leaf package role |
| cloudprovider/aws | provider.go:17 | Yes | Yes - add key types, what it wraps |
| cloudprovider/fake | provider.go:17 | Yes | Yes - add hook mechanism description |
| config | (none) | NO | Yes - create from scratch |
| controller | setup.go:17 | Yes | Minimal - aggregator only, 1 file |
| controller/nodeclass | reconciler.go:17 | Yes | Yes - add responsibilities |
| controller/nodepool | reconciler.go:17 | Yes | Yes - add sub-package relationships |
| nodepool/lifecycle | manager.go:17 | Yes | Yes - add operations overview |
| nodepool/nodestate | nodestate.go:17 | Yes | Yes - add state machine description |
| github | client.go:17 | Yes | Minimal - already descriptive |
| metrics | metrics.go:17 | Yes | Yes - add metrics catalog |
| strategy | interface.go:17 | Yes | Yes - add strategy pattern description |
| strategy/githubactions | (none) | NO | Yes - create from scratch |
| strategy/kubernetes | capacity.go:17 | Yes | Yes - add components overview |

### Import Dependency Graph

Documented for each doc.go to reference:

```
nodestate (leaf) <-- lifecycle <-- nodepool <-- controller (root)
                                       |
cloudprovider (leaf) <-- aws            +-- nodeclass
                     <-- fake           |
                     <-- lifecycle      +-- strategy <-- kubernetes
                     <-- nodepool                    <-- githubactions
                     <-- strategy

config (leaf) <-- controller
              <-- nodepool

metrics (leaf) <-- aws
               <-- lifecycle
               <-- nodepool
               <-- kubernetes

github (leaf) <-- githubactions
```

### Anti-Patterns to Avoid
- **Duplicating doc comments:** When moving the comment to doc.go, REMOVE it from the original source file. Go concatenates all package comments in a package, so having two creates noise.
- **Over-documenting internals:** doc.go describes the package's public API and purpose, not implementation details of individual functions.
- **Forgetting the license header:** Every .go file in this project has the Apache 2.0 license header.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Test verification | Custom test runner scripts | `make test` + `make test-integration` | Already configured, Makefile handles KUBEBUILDER_ASSETS |
| Doc format validation | Custom linter | `go doc ./internal/...` | Standard tooling verifies doc comments render correctly |

## Common Pitfalls

### Pitfall 1: Leaving Duplicate Package Comments
**What goes wrong:** After creating doc.go, the old `// Package ...` line on the original file remains, causing godoc to concatenate both comments.
**Why it happens:** Easy to forget to remove the original when creating the new file.
**How to avoid:** For each package: (1) create doc.go with expanded comment, (2) remove the `// Package ...` line from the original source file, (3) verify with `go doc`.
**Warning signs:** `go doc ./internal/some/package` shows the description twice or concatenated.

### Pitfall 2: Missing License Header
**What goes wrong:** New doc.go files created without the project's Apache 2.0 license header.
**Why it happens:** doc.go is a new file, not copied from existing template.
**How to avoid:** Use the boilerplate from `hack/boilerplate.go.txt` for every new doc.go file.
**Warning signs:** Files without the copyright notice.

### Pitfall 3: Blank Line Between Comment and Package Declaration
**What goes wrong:** A blank line between `// Package ...` and `package name` causes Go to not recognize it as a doc comment.
**Why it happens:** Formatting mistake.
**How to avoid:** Ensure the comment is immediately followed by `package name` with no intervening blank line.
**Warning signs:** `go doc` shows no package documentation.

### Pitfall 4: Assuming Tests Need Fixing
**What goes wrong:** Spending time investigating or modifying tests that already pass.
**Why it happens:** The success criteria say "make test passes" and "make test-integration passes" -- which they already do.
**How to avoid:** Run tests FIRST to verify current state. Only fix what's actually broken.
**Warning signs:** Unnecessary test changes that could introduce regressions.

## Code Examples

### doc.go Template (Verified Pattern)

The exact template to use for every doc.go file in this project:

```go
/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package [name] [first sentence describing purpose].
//
// [Expanded description of responsibilities]
//
// [Key types: Type1, Type2, Type3]
//
// [Relationship to other packages]
package [name]
```

### Example: nodestate doc.go (leaf package)

```go
// Package nodestate defines the node state machine for Stratos-managed nodes.
//
// Every Stratos node transitions through a fixed set of states: warmup, standby,
// running, and terminating. This package defines the NodeState type, valid state
// transitions, and the Kubernetes label and taint constants used to track state
// on Node objects.
//
// Key types:
//   - NodeState: the lifecycle state enum (warmup, standby, running, terminating)
//   - ValidTransition: validates that a state change is legal
//
// This is a leaf package with no internal dependencies. It is imported by
// lifecycle, nodepool, strategy/kubernetes, and strategy/githubactions.
package nodestate
```

### Example: Removing Old Comment

Before (reconciler.go):
```go
// Package nodepool implements the NodePool controller.
package nodepool
```

After (reconciler.go, with doc.go now holding the package comment):
```go
package nodepool
```

## Current Test State (Verified)

### Unit Tests (`make test`)
- **Status:** ALL PASS
- **Coverage output:** All packages compile and run
- **Packages with tests:** api/v1alpha1, cloudprovider/aws, config, controller/nodeclass, controller/nodepool, lifecycle, nodestate, strategy/kubernetes

### Integration Tests (`make test-integration`)
- **Status:** 71/72 PASS, 1 SKIPPED
- **Skipped test:** CEL validation rule test -- envtest (v1.28) does not enforce CEL validation, test correctly skips with `Skip("envtest does not enforce CEL validation rules")`
- **3 consecutive fresh runs:** All pass (different random seeds: 1770136882, 1770137013, 1770137090)
- **No flakes detected**
- **Duration:** ~75 seconds per run

### Linter (`make lint`)
- **Status:** 0 issues
- **golangci-lint v2.8.0** with errcheck, gosimple, govet, ineffassign, staticcheck, unused, gosec, gocyclo, misspell, funlen, cyclop, depguard, contextcheck

### E2E Tests
- **Status:** Not part of success criteria (requires live EKS cluster)
- **Note:** Some e2e test files were deleted (spot_test.go, testdata/) as part of spot-replacement feature removal -- this is expected and not a test recovery item

### Deleted Test Files (Expected)
These files were deleted during the restructure because their code was reorganized into new packages:
- `internal/controller/config_test.go` -> tests now in `internal/config/config_test.go`
- `internal/controller/network_readiness_test.go` -> tests now in `strategy/kubernetes/network_test.go`
- `internal/controller/pod_watcher_test.go` -> functionality moved to strategy
- `internal/controller/scale_calculator_test.go` -> tests now in strategy/kubernetes
- `internal/controller/startup_taints_test.go` -> tests now in `strategy/kubernetes/startup_taints_test.go`
- `internal/controller/warmup_test.go` -> tests now in `lifecycle/warmup_test.go`
- `tests/integration/spot_replacement_test.go` -> spot replacement feature removed

## State of the Art

| Aspect | Current State | Required State | Gap |
|--------|--------------|----------------|-----|
| doc.go files | 0 exist | 15 needed (all internal/ packages) | 15 files to create |
| Package comments | 13/15 have inline comments | All in doc.go | Move 13, create 2 from scratch |
| Unit tests | All pass | All pass | None |
| Integration tests | 71/72 pass (1 correct skip) | All pass | None |
| Flake check | 3/3 clean runs | 3 consecutive passes | None |
| Linter | 0 issues | 0 issues | None |

## Open Questions

1. **api/v1alpha1 package:**
   - The success criteria say "every package under internal/" -- api/v1alpha1 is technically not under internal/
   - api/v1alpha1 already has a package comment in groupversion_info.go
   - Recommendation: Stick to the letter of the requirement (internal/ only). api/v1alpha1 is out of scope.

2. **cmd/stratos package:**
   - Also outside internal/, also not in scope per requirements
   - No doc.go needed

3. **tests/ packages:**
   - tests/integration and tests/e2e are test packages, not under internal/
   - No doc.go needed per requirements

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection: all package structures, imports, test results verified by running code
- `make test` output: verified 2026-02-03
- `make test-integration` output: 3 fresh runs verified 2026-02-03
- `make lint` output: verified 2026-02-03
- `hack/boilerplate.go.txt`: verified license header template

### Secondary (MEDIUM confidence)
- [Go Doc Comments specification](https://tip.golang.org/doc/comment) -- official Go documentation on doc comment format
- [Godoc: documenting Go code](https://go.dev/blog/godoc) -- Go blog post on documentation conventions
- [Google Go Style Best Practices](https://google.github.io/styleguide/go/best-practices.html) -- Google's Go style guide

### Tertiary (LOW confidence)
- Karpenter controller structure (observed via GitHub, but Karpenter does not actually have doc.go files in most packages -- the pattern is more of a convention than something Karpenter follows)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new libraries, just Go doc conventions
- Architecture: HIGH -- verified by direct codebase inspection and Go spec
- Pitfalls: HIGH -- based on known Go doc comment behavior and direct testing
- Test state: HIGH -- verified by running all tests 3+ times

**Research date:** 2026-02-03
**Valid until:** No expiry -- Go doc comment conventions are stable
