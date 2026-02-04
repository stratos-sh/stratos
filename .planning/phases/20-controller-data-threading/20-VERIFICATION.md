---
phase: 20-controller-data-threading
verified: 2026-02-04T23:15:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 20: Controller Data Threading Verification Report

**Phase Goal:** Image list and pull policy flow end-to-end from NodePool spec through the controller to the cloud provider's user data generation
**Verified:** 2026-02-04T23:15:00Z
**Status:** PASSED
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | NodePool with spec.preWarm.images launches instances whose user data contains ctr pull commands for those images | VERIFIED | `node_launch.go:45` sets `PreWarmConfig: pool.Spec.PreWarm` on TemplateConfig; `provider.go:257` passes it to BootstrapConfig; AL2023 generator at `al2023.go:42-54` produces MIME with `warmup.GenerateScript(config.PreWarmConfig.GetImages(), ...)` which emits `ctr` commands; test `TestGenerateEncodedUserData_WithPreWarmConfig` confirms MIME-Version, nginx:latest, and ctr in decoded output |
| 2 | NodePool without spec.preWarm launches instances with unchanged user data (no image pull script) | VERIFIED | `node_launch.go:45` assigns nil when pool.Spec.PreWarm is nil; `al2023.go:42-44` checks `config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0` and returns plain YAML when false; test `TestGenerateEncodedUserData_WithoutPreWarmConfig` confirms no MIME-Version and plain NodeConfig YAML; test `TestLaunchNode_NilPreWarmConfig` confirms nil passthrough |
| 3 | Changing images on a NodePool spec causes newly launched instances to use the updated list | VERIFIED | `node_launch.go:45` reads `pool.Spec.PreWarm` on every call (no caching); the reconciler fetches fresh NodePool from apiserver on each Reconcile (reconciler.go:94); new launches always use the current spec; test `TestLaunchNode_PreWarmConfigPassedToLauncher` verifies same-pointer passthrough from pool spec |
| 4 | Bottlerocket NodePool with images gets ImagePrePullSupported=False condition on status | VERIFIED | `nodepool_validation.go:82-115` implements `checkImagePrePullSupport`; when `BootstrapTemplate == BottlerocketNotSupported`, sets `ConditionTypeImagePrePullSupported` to False with reason `BottlerocketNotSupported`; `reconciler.go:164-166` calls this on every reconcile; 6 test cases in `nodepool_validation_test.go` cover this behavior |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/cloudprovider/interface.go` | TemplateConfig with PreWarmConfig field | VERIFIED | Line 33: `PreWarmConfig *stratosv1alpha1.PreWarmConfig` -- 78 lines, substantive, imported across cloudprovider and controller packages |
| `internal/controller/nodepool/lifecycle/node_launch.go` | PreWarmConfig populated from pool.Spec.PreWarm | VERIFIED | Line 45: `PreWarmConfig: pool.Spec.PreWarm` -- 114 lines, substantive, wired into reconciler via lifecycle.Manager |
| `internal/cloudprovider/aws/provider.go` | PreWarmConfig passthrough to BootstrapConfig | VERIFIED | Line 257: `bootstrapConfig.PreWarmConfig = templateConfig.PreWarmConfig` -- 541 lines, substantive, called via LaunchInstance in reconciler |
| `internal/controller/nodepool/reconciler.go` | checkImagePrePullSupport wired in reconcile loop | VERIFIED | Lines 163-166: `if nodeClass, ncErr := r.getNodeClass(...); ncErr == nil { r.checkImagePrePullSupport(nodePool, nodeClass) }` -- 341 lines, substantive, main reconcile entry point |
| `internal/controller/nodepool/lifecycle/node_launch_test.go` | Tests for PreWarmConfig passthrough | VERIFIED | 146 lines, 2 tests (with/without PreWarmConfig), spy launcher pattern, both pass |
| `internal/cloudprovider/aws/provider_test.go` | Tests for generateEncodedUserData with PreWarmConfig | VERIFIED | 196 lines, 4 tests (AL2023 with images, without images, nil templateConfig, AL2 with images), all pass |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `lifecycle/node_launch.go` | `cloudprovider/interface.go` | `TemplateConfig.PreWarmConfig = pool.Spec.PreWarm` | WIRED | Line 45 assigns pool.Spec.PreWarm to TemplateConfig.PreWarmConfig; test verifies same-pointer identity |
| `aws/provider.go` | `aws/userdata.go` | `bootstrapConfig.PreWarmConfig = templateConfig.PreWarmConfig` | WIRED | Line 257 inside `if templateConfig != nil` block; test `TestGenerateEncodedUserData_WithPreWarmConfig` confirms ctr commands in output |
| `nodepool/reconciler.go` | `nodepool/nodepool_validation.go` | `r.checkImagePrePullSupport(nodePool, nodeClass)` | WIRED | Line 165 calls checkImagePrePullSupport; uses `getNodeClass` (from nodeclass_fetch.go:38) to fetch NodeClass; 6 test cases in nodepool_validation_test.go |
| `aws/al2023.go` | `warmup/generator.go` | `warmup.GenerateScript(config.PreWarmConfig.GetImages(), ...)` | WIRED (prior phase) | Line 54 calls warmup.GenerateScript; generates ctr pull commands for each image |
| `aws/al2.go` | `warmup/generator.go` | `warmup.GenerateScript(config.PreWarmConfig.GetImages(), ...)` | WIRED (prior phase) | Line 48 calls warmup.GenerateScript conditionally when images configured |

### Requirements Coverage

| Requirement | Status | Blocking Issue |
|-------------|--------|----------------|
| FLOW-01: Image list and pull policy flow from NodePool spec through TemplateConfig to BootstrapConfig to warmup script | SATISFIED | None -- all 4 wiring points connected, tests verify end-to-end flow |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | - | - | No TODO, FIXME, placeholder, stub, or empty return patterns found in any modified file |

### Build and Test Verification

| Check | Result |
|-------|--------|
| `go build ./...` | PASS -- zero errors |
| `go vet ./...` | PASS -- zero warnings |
| `make test` (full suite) | PASS -- all 13 packages pass |
| LaunchNode PreWarmConfig tests | PASS -- 2/2 tests |
| generateEncodedUserData PreWarmConfig tests | PASS -- 4/4 tests |

### Human Verification Required

### 1. End-to-End Launch with Images

**Test:** Apply a NodePool with `spec.preWarm.images: ["nginx:latest"]` and an AL2023 AWSNodeClass. Wait for instance launch. Decode the user data from the EC2 console or via AWS CLI (`aws ec2 describe-instance-attribute --attribute userData`).
**Expected:** User data is MIME multipart containing an `image-pull.sh` part with `ctr` commands pulling `nginx:latest`.
**Why human:** Requires running cluster with AWS credentials and real EC2 instance launch.

### 2. Bottlerocket Condition Display

**Test:** Apply a NodePool with `spec.preWarm.images: ["nginx:latest"]` referencing a Bottlerocket AWSNodeClass. Run `kubectl describe nodepool`.
**Expected:** Status conditions include `ImagePrePullSupported=False` with message about Bottlerocket not supporting image pre-pull.
**Why human:** Requires running cluster with controller deployed.

### 3. Image List Update Propagation

**Test:** Apply a NodePool with images `["nginx:latest"]`. Wait for standby node. Update images to `["redis:7"]`. Trigger a new instance launch (increase poolSize or delete a node).
**Expected:** New instance has user data with `redis:7` (not `nginx:latest`). Existing standby node is unaffected.
**Why human:** Requires running cluster to verify live spec change propagation.

### Gaps Summary

No gaps found. All 4 must-have truths are verified through code inspection and passing tests. The 4 wiring points are connected:

1. `TemplateConfig.PreWarmConfig` field exists in the cloud-agnostic interface
2. `LaunchNode` populates it from `pool.Spec.PreWarm` on every call
3. `generateEncodedUserData` passes it through to `BootstrapConfig`
4. `reconcileNodePool` calls `checkImagePrePullSupport` on every reconcile

The downstream infrastructure (generators, MIME builder, warmup script) from Phases 17-19 correctly consumes `PreWarmConfig` when non-nil and produces user data with `ctr` pull commands. The Bottlerocket generator correctly ignores images. All tests pass including 6 new tests added in this phase.

---

_Verified: 2026-02-04T23:15:00Z_
_Verifier: Claude (gsd-verifier)_
