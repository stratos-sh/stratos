# Roadmap: Stratos

## Milestones

- v1.0 Codebase Restructure - Phases 1-6 (shipped 2026-02-03)
- v1.1 Simplify Scaling - Phases 7-11 (shipped 2026-02-04)
- v1.1.1 Naming & Dead Code Cleanup - Phases 12-16 (shipped 2026-02-04)
- v1.2 Warmup Image Pre-Pull - Phases 17-20 (in progress)

## Phases

<details>
<summary>v1.0 Codebase Restructure (Phases 1-6) - SHIPPED 2026-02-03</summary>

See .planning/MILESTONES.md for v1.0 details (6 phases, 13 plans).

</details>

<details>
<summary>v1.1 Simplify Scaling (Phases 7-11) - SHIPPED 2026-02-04</summary>

See .planning/milestones/v1.1-ROADMAP.md for v1.1 details (5 phases, 5 plans).

</details>

<details>
<summary>v1.1.1 Naming & Dead Code Cleanup (Phases 12-16) - SHIPPED 2026-02-04</summary>

See .planning/milestones/v1.1.1-ROADMAP.md for v1.1.1 details (5 phases, 5 plans).

</details>

### v1.2 Warmup Image Pre-Pull (In Progress)

**Milestone Goal:** Standby nodes have expected container images cached, eliminating image pull latency when pods are scheduled

- [x] **Phase 17: CRD Types and Code Generation** - PreWarmConfig fields, validation, deepcopy, nil-safe getters
- [x] **Phase 18: Warmup Script Generator** - Dynamic bash script generation with ctr pull, ECR auth, retry, pinning
- [x] **Phase 19: AMI Generator Integration** - Inject image pull script into AL2 and AL2023 user data, Bottlerocket warning
- [ ] **Phase 20: Controller Data Threading** - Wire image config from NodePool spec through TemplateConfig to BootstrapConfig

## Phase Details

### Phase 17: CRD Types and Code Generation
**Goal**: NodePool CRD accepts warmup image configuration with validated fields and generated deepcopy methods
**Depends on**: Nothing (first phase of v1.2; builds on existing PreWarmConfig struct)
**Requirements**: CRD-01, CRD-02, CRD-03, GEN-01, GEN-02, TEST-04
**Success Criteria** (what must be TRUE):
  1. User can specify `spec.warmup.images` as a list of container image references on a NodePool manifest
  2. User can set `spec.warmup.imagePullPolicy` to Required or BestEffort, with Required as the default when omitted
  3. Applying a NodePool with an empty string in the images list is rejected by CRD validation
  4. `make generate` and `make manifests` succeed with the new fields, producing updated deepcopy methods and CRD YAML
  5. Calling GetImages() and GetImagePullPolicy() on a nil PreWarmConfig returns safe zero values without panicking
**Plans**: 1 plan
Plans:
- [x] 17-01-PLAN.md -- Add ImagePullPolicy type, PreWarmConfig fields, getters, tests, and code generation

### Phase 18: Warmup Script Generator
**Goal**: A warmup script generator produces correct bash that pulls configured images using ctr, with ECR auth, retry logic, pinning, and policy-aware failure handling
**Depends on**: Phase 17 (uses ImagePullPolicy type for policy-aware generation)
**Requirements**: WARM-01, WARM-02, WARM-03, WARM-04, WARM-05, WARM-06, WARM-07, WARM-08, TEST-01
**Success Criteria** (what must be TRUE):
  1. Generated script waits for containerd socket and CRI readiness before attempting any image pull
  2. Generated script pulls each image via `ctr -n k8s.io images pull` with ECR authentication when the image reference contains an ECR registry
  3. Generated script retries failed pulls with exponential backoff (up to 3 retries per image) and logs image name, duration, and outcome for each pull
  4. With imagePullPolicy=Required, generated script exits non-zero if any image fails after retries; with BestEffort, script completes regardless of failures
  5. After successful pull, generated script pins each image with `io.cri-containerd.pinned=pinned` label to prevent kubelet garbage collection
**Plans**: 1 plan
Plans:
- [x] 18-01-PLAN.md -- Warmup script generator package with template, ECR auth, retry, pinning, and tests

### Phase 19: AMI Generator Integration
**Goal**: Image pull script is correctly injected into user data for AL2 and AL2023 instances, with a validation warning for Bottlerocket
**Depends on**: Phase 18 (uses warmup script generator output)
**Requirements**: AMI-01, AMI-02, AMI-03, FLOW-02, TEST-02, TEST-03
**Success Criteria** (what must be TRUE):
  1. AL2 user data includes the image pull script as a MIME part between the bootstrap script and warmup completion script
  2. AL2023 user data switches to MIME multipart format (NodeConfig as application/node.eks.aws + shell script) when images are configured, and remains plain NodeConfig YAML when no images are specified
  3. Configuring images on a NodePool that uses a Bottlerocket AMI produces a validation warning (image pre-pull not supported)
  4. Generated user data with images logs a warning if it approaches the 16KB EC2 user data size limit
**Plans**: 2 plans
Plans:
- [x] 19-01-PLAN.md -- AL2/AL2023 image pull MIME integration with shared utilities and size warning
- [x] 19-02-PLAN.md -- Bottlerocket image pre-pull status condition warning

### Phase 20: Controller Data Threading
**Goal**: Image list and pull policy flow end-to-end from NodePool spec through the controller to the cloud provider's user data generation
**Depends on**: Phase 19 (generators must accept image config; Phase 17 CRD types must exist)
**Requirements**: FLOW-01
**Success Criteria** (what must be TRUE):
  1. When a NodePool with configured images launches a new instance, the generated user data contains the correct ctr pull commands for those images
  2. Changing the image list on a NodePool spec causes newly launched instances to use the updated image list (existing standby nodes are unaffected)
**Plans**: 1 plan
Plans:
- [ ] 20-01-PLAN.md -- Wire PreWarmConfig through TemplateConfig, provider, and reconcile loop with tests

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-6 | v1.0 | 13/13 | Complete | 2026-02-03 |
| 7. Type Relocation | v1.1 | 1/1 | Complete | 2026-02-03 |
| 8. Controller Rewiring | v1.1 | 1/1 | Complete | 2026-02-03 |
| 9. Strategy Deletion | v1.1 | 1/1 | Complete | 2026-02-03 |
| 10. CRD Simplification | v1.1 | 1/1 | Complete | 2026-02-03 |
| 11. Final Cleanup | v1.1 | 1/1 | Complete | 2026-02-03 |
| 12. Clean Working Tree | v1.1.1 | 1/1 | Complete | 2026-02-04 |
| 13. Dead Code Removal | v1.1.1 | 1/1 | Complete | 2026-02-04 |
| 14. Type Renames | v1.1.1 | 1/1 | Complete | 2026-02-04 |
| 15. File Renames | v1.1.1 | 1/1 | Complete | 2026-02-04 |
| 16. Struct Field Type Change | v1.1.1 | 1/1 | Complete | 2026-02-04 |
| 17. CRD Types & Code Gen | v1.2 | 1/1 | Complete | 2026-02-04 |
| 18. Warmup Script Generator | v1.2 | 1/1 | Complete | 2026-02-04 |
| 19. AMI Generator Integration | v1.2 | 2/2 | Complete | 2026-02-04 |
| 20. Controller Data Threading | v1.2 | 0/1 | Not started | - |
