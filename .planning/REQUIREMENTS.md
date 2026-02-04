# Requirements: Stratos v1.2 Warmup Image Pre-Pull

**Defined:** 2026-02-04
**Core Value:** Standby nodes have expected container images cached, eliminating image pull latency when pods are scheduled

## v1.2 Requirements

### CRD Configuration

- [ ] **CRD-01**: User can specify a list of container images to pre-pull on the NodePool CRD (`spec.warmup.images`)
- [ ] **CRD-02**: User can set image pull policy to Required (default) or BestEffort (`spec.warmup.imagePullPolicy`)
- [ ] **CRD-03**: CRD validation rejects empty image strings in the images list

### Warmup Script

- [ ] **WARM-01**: Warmup script waits for containerd socket readiness before pulling images
- [ ] **WARM-02**: Warmup script uses `ctr -n k8s.io images pull` to pull configured images into the kubelet-visible namespace
- [ ] **WARM-03**: For ECR images, warmup script authenticates via `aws ecr get-login-password` using the instance profile
- [ ] **WARM-04**: Image pulls retry with exponential backoff (up to 3 retries per image)
- [ ] **WARM-05**: With imagePullPolicy=Required, warmup fails if any image pull fails after retries
- [ ] **WARM-06**: With imagePullPolicy=BestEffort, warmup completes regardless of pull failures (failures logged)
- [ ] **WARM-07**: After successful pull, each image is pinned with `io.cri-containerd.pinned=pinned` label to prevent kubelet GC eviction
- [ ] **WARM-08**: Each image pull logs image name, duration, and success/failure status

### AMI Support

- [ ] **AMI-01**: Image pre-pull works on AL2 instances (MIME part injected between bootstrap and warmup scripts)
- [ ] **AMI-02**: Image pre-pull works on AL2023 instances (user data switches to MIME multipart when images specified, plain NodeConfig YAML when no images)
- [ ] **AMI-03**: Bottlerocket instances with configured images produce a validation warning (image pre-pull not supported on Bottlerocket)

### Data Flow

- [ ] **FLOW-01**: Image list and pull policy flow from NodePool spec through TemplateConfig to BootstrapConfig to warmup script
- [ ] **FLOW-02**: Controller generates user data with size check — logs warning if generated user data approaches 16KB EC2 limit

### Code Generation

- [ ] **GEN-01**: CRD types generate deepcopy methods via `make generate`
- [ ] **GEN-02**: CRD manifests updated via `make manifests`

### Tests

- [ ] **TEST-01**: Unit tests for warmup script generation with images, without images, Required policy, BestEffort policy
- [ ] **TEST-02**: Unit tests for AL2 MIME multipart with image pull MIME part
- [ ] **TEST-03**: Unit tests for AL2023 MIME multipart vs plain YAML conditional output
- [ ] **TEST-04**: Unit tests for nil-safe getter methods on PreWarmConfig (GetImages, GetImagePullPolicy)

## Future Requirements

### Enhanced Observability

- **OBS-01**: Warmup condition on NodePool status reports image pull outcomes
- **OBS-02**: Prometheus metrics for image pull duration and success rate

### Extended Support

- **EXT-01**: Bottlerocket image pre-pull via bootstrap containers
- **EXT-02**: Dynamic pending-pod image pre-pull at start time
- **EXT-03**: Parallel image pulls with configurable concurrency

## Out of Scope

| Feature | Reason |
|---------|--------|
| Dynamic pending-pod image pull | No clean mechanism to pass image list to node at start time without API server access |
| Bottlerocket image pre-pull | Immutable OS with no shell script support in user data; requires separate bootstrap-containers approach |
| Per-image pull policy | CRD complexity not justified; pool-level policy covers common case |
| Registry authentication configuration in CRD | ECR auth automatic via IAM; non-ECR registries use containerd config |
| Image digest resolution | Users specify digests if they need exact versions; controller doesn't need registry access |
| DaemonSet-based image puller | Standby nodes are stopped EC2 instances; DaemonSets can't run on stopped instances |
| EBS snapshot-based image caching | Enormous complexity for marginal benefit over warmup-time pulling |
| Configurable retry count | Hardcoded sensible defaults; warmup timeout is the effective retry budget |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CRD-01 | Phase 17 | Pending |
| CRD-02 | Phase 17 | Pending |
| CRD-03 | Phase 17 | Pending |
| WARM-01 | Phase 18 | Pending |
| WARM-02 | Phase 18 | Pending |
| WARM-03 | Phase 18 | Pending |
| WARM-04 | Phase 18 | Pending |
| WARM-05 | Phase 18 | Pending |
| WARM-06 | Phase 18 | Pending |
| WARM-07 | Phase 18 | Pending |
| WARM-08 | Phase 18 | Pending |
| AMI-01 | Phase 19 | Pending |
| AMI-02 | Phase 19 | Pending |
| AMI-03 | Phase 19 | Pending |
| FLOW-01 | Phase 20 | Pending |
| FLOW-02 | Phase 19 | Pending |
| GEN-01 | Phase 17 | Pending |
| GEN-02 | Phase 17 | Pending |
| TEST-01 | Phase 18 | Pending |
| TEST-02 | Phase 19 | Pending |
| TEST-03 | Phase 19 | Pending |
| TEST-04 | Phase 17 | Pending |

**Coverage:**
- v1.2 requirements: 22 total
- Mapped to phases: 22
- Unmapped: 0

---
*Requirements defined: 2026-02-04*
*Last updated: 2026-02-04 after roadmap creation*
