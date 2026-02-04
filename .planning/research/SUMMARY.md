# Project Research Summary

**Project:** Stratos v1.2 Warmup Image Pre-Pull
**Domain:** Kubernetes operator feature — container image pre-caching during warmup lifecycle
**Researched:** 2026-02-04
**Confidence:** HIGH

## Executive Summary

Warmup image pre-pull is a surgical feature addition to Stratos that pre-caches container images on standby nodes during the warmup phase, eliminating cold-start image pull latency when pods are scheduled. Research confirms the implementation requires **zero new Go dependencies** — the entire feature is bash script generation that calls `ctr -n k8s.io images pull` (containerd CLI, pre-installed on all EKS AMIs) during the user data warmup phase. ECR authentication happens automatically via the instance profile's IAM role using `aws ecr get-login-password`.

The recommended implementation is straightforward for AL2 (add image pulls to existing warmup script) but AL2023 requires a critical architectural decision: switching from plain NodeConfig YAML to MIME multipart format to support a warmup shell script alongside the NodeConfig. The most significant risk is **image garbage collection** — kubelet will evict pre-pulled images unless they are explicitly pinned using the `io.cri-containerd.pinned=pinned` containerd label. Without pinning, the entire feature becomes unreliable.

Implementation spans 7 files with ~100 lines of changes. The data flow is clean: `NodePool.spec.preWarm.images` → `TemplateConfig` → `BootstrapConfig` → warmup script generator → bash MIME part. All table stakes features (TS-1 through TS-7) plus image pinning (D-3) should ship in MVP. The feature integrates into the existing warmup architecture without changing the controller reconciliation logic — images are pulled during warmup via user data, no runtime coordination needed.

## Key Findings

### Recommended Stack

**Use `ctr` exclusively for image pulling.** Research confirms `ctr` (containerd CLI) is the only tool guaranteed present on both AL2 and AL2023 EKS AMIs. While `crictl` is available on AL2, it is **not available** on AL2023 (amazon-eks-ami issue #2163), making `ctr` the only universal option. The command pattern is `ctr -n k8s.io images pull -u "AWS:${ECR_PASSWORD}" <image>` where the `-n k8s.io` namespace flag is critical (images in other namespaces are invisible to kubelet).

**Core technologies:**
- **ctr (containerd CLI)** — image pulling tool, pre-installed at `/usr/bin/ctr` on all EKS AMIs
- **AWS CLI** — ECR authentication via `aws ecr get-login-password`, pre-installed on all EKS AMIs
- **MIME multipart user data** — standard mechanism for combining shell scripts with bootstrap configs
- **containerd pinning labels** — `io.cri-containerd.pinned=pinned` prevents kubelet GC from removing cached images

**No new Go dependencies.** The feature is pure bash script generation using stdlib (`fmt`, `strings`, `regexp`). The existing user data framework handles all assembly and base64 encoding.

### Expected Features

Research identified 7 table stakes features and 6 differentiators. The MVP recommendation is all table stakes plus D-3 (image pinning) and D-4 (size/duration logging).

**Must have (table stakes):**
- **TS-1: Image list specification** — `spec.preWarm.images` field on NodePool CRD accepting standard image references (tag/digest/combined)
- **TS-2: Required vs BestEffort policy** — pool-level `imagePullPolicy` controlling whether failures block warmup
- **TS-3: Auto-detection of container runtime CLI** — warmup script detects `ctr` availability (no runtime selection needed — `ctr` is always present)
- **TS-4: ECR authentication** — automatic via `aws ecr get-login-password` using instance profile IAM role, with IMDS availability polling
- **TS-5: Retry with backoff** — exponential backoff (5s, 10s, 20s, 40s, 60s) for transient failures like ECR throttling
- **TS-6: Pull execution in warmup script** — inject image pulls after kubelet health check, before warmup completion signal
- **TS-7: Status reporting** — NodePool conditions/events for image pull outcomes, console logs for per-image progress

**Should have (differentiators, MVP):**
- **D-3: Image pinning** — set `io.cri-containerd.pinned=pinned` label after successful pull to prevent kubelet GC eviction (CRITICAL for correctness)
- **D-4: Size/duration logging** — log image size and pull duration for operational visibility (trivial cost, high diagnostic value)

**Defer (v2+):**
- **D-1: Pull ordering (largest first)** — document "list largest images first" instead of auto-detection
- **D-2: Parallel pulls** — sequential is safer and simpler for v1 (kubelet defaults to sequential)
- **D-5: Timeout auto-adjustment** — documentation guidance sufficient ("for N large images, increase warmup timeout")
- **D-6: Digest resolution** — users can specify digests manually in image references

### Architecture Approach

The feature threads new data through the existing user data generation pipeline. `NodePool.spec.preWarm.images` flows through `TemplateConfig` (controller → cloud provider bridge) to `BootstrapConfig` (cloud provider config staging) to the warmup script generator. The warmup script is currently a static constant (`internal/cloudprovider/aws/warmup.go`) and must become dynamic to inject `ctr pull` commands.

**Major components (in data flow order):**
1. **CRD types (`api/v1alpha1/config_types.go`)** — add `Images []string` and `ImagePullPolicy *ImagePullPolicy` to `PreWarmConfig`
2. **TemplateConfig bridge (`internal/cloudprovider/interface.go`)** — add `WarmupImages []string` and `ImagePullPolicy string` fields
3. **Controller threading (`lifecycle/node_launch.go`)** — copy `pool.Spec.PreWarm.GetImages()` into `TemplateConfig` at launch time
4. **BootstrapConfig staging (`aws/userdata.go`)** — add image fields, passed to all generators
5. **Warmup script generator (`aws/warmup.go`)** — convert from static constant to dynamic function that generates pull commands
6. **AL2 generator (`aws/al2.go`)** — insert image-pull script as MIME part 2 (after bootstrap.sh, before warmup completion)
7. **AL2023 generator (`aws/al2023.go`)** — switch to MIME multipart when images are specified (NodeConfig + shell script)

**Integration with existing warmup flow:**
- Current: `bootstrap.sh → warmup.go waits for kubelet → warmup complete → controller stops instance`
- New: `bootstrap.sh → wait for kubelet → pull images (NEW) → warmup complete → controller stops instance`
- Image pulls run in the warmup script AFTER kubelet health check (ensures containerd and credential provider are ready)

**AMI family differences:**
- **AL2**: MIME multipart already used → straightforward insertion of pull script as new MIME part
- **AL2023**: Currently plain NodeConfig YAML → must wrap in MIME multipart with `application/node.eks.aws` content type for NodeConfig + `text/x-shellscript` for pull script
- **Bottlerocket**: TOML only, no shell script support → document as unsupported for image pre-pull (defer to future milestone using bootstrap-containers)

### Critical Pitfalls

Research identified 5 critical pitfalls that block the feature if not handled upfront:

1. **Image garbage collection removes pre-pulled images (Pitfall 3)** — kubelet GC runs every 5 minutes and evicts unused images when disk usage exceeds 85%. Pre-pulled images on standby nodes (stopped for hours/days) are prime GC targets when the node starts. **Prevention:** Pin images with `ctr -n k8s.io images label <image> io.cri-containerd.pinned=pinned` immediately after successful pull. This is CRITICAL for feature correctness.

2. **Containerd socket not ready when pull script runs (Pitfall 2)** — warmup script may attempt `ctr pull` before containerd is fully initialized. On AL2023, containerd startup can race with cloud-init shell scripts. **Prevention:** Add explicit containerd socket wait loop (`while [ ! -S /run/containerd/containerd.sock ]; do sleep 2; done`) and CRI readiness check (`until ctr -n k8s.io version >/dev/null 2>&1; do sleep 2; done`) with 60s timeout before any pull operations.

3. **Wrong containerd namespace makes images invisible (Pitfall 4)** — containerd uses namespaces for isolation. Kubelet operates exclusively in the `k8s.io` namespace. Images pulled with `ctr` (without `-n k8s.io`) land in the `default` namespace and are invisible to kubelet. **Prevention:** ALWAYS use `-n k8s.io` flag with all `ctr` commands. Verify with `ctr -n k8s.io images ls` after pull.

4. **AL2023 shell script blocks nodeadm cluster join (Pitfall 1)** — on AL2023, `text/x-shellscript` MIME parts run BEFORE `nodeadm-run` completes. A 5-minute image pull delays cluster join by 5 minutes. **Mitigation:** The warmup script runs during the dedicated warmup phase (before node is stopped into standby), so blocking nodeadm-run briefly is acceptable — the node will be stopped anyway. However, the script MUST wait for containerd (Pitfall 2) to avoid interfering with nodeadm's own containerd dependency.

5. **Warmup timeout exceeded by large image pulls (Pitfall 5)** — default warmup timeout is 10 minutes. Five 2GB images could consume 5+ minutes of pull time, leaving only 5 minutes for the rest of warmup. **Prevention:** With `imagePullPolicy: BestEffort`, failed pulls do not block warmup completion. For `Required` policy, document that users should increase `preWarm.timeout` based on total image size. The pull script should enforce per-image timeouts to prevent a single stuck pull from consuming the entire budget.

## Implications for Roadmap

Based on research, the implementation naturally divides into 3 phases following dependency order:

### Phase 1: CRD Types and User Data Script Generation
**Rationale:** All downstream code depends on CRD types existing. The warmup script generator is the core implementation logic and can be developed and tested independently of controller integration.

**Delivers:**
- CRD field additions: `PreWarmConfig.Images []string` and `PreWarmConfig.ImagePullPolicy *ImagePullPolicy`
- Dynamic warmup script generator that produces bash with `ctr pull` commands, ECR auth, retry logic, and image pinning
- BootstrapConfig extension with image fields
- Unit tests for script generation with various image lists and policies

**Addresses features:**
- TS-1 (image list specification)
- TS-2 (Required/BestEffort policy)
- TS-3 (CLI detection — simplified to `ctr` only)
- TS-4 (ECR authentication)
- TS-5 (retry with backoff)
- D-3 (image pinning)
- D-4 (size/duration logging)

**Avoids pitfalls:**
- Pitfall 2 (containerd socket readiness) — wait loop in script
- Pitfall 3 (GC eviction) — pinning labels
- Pitfall 4 (wrong namespace) — `-n k8s.io` on all `ctr` commands
- Pitfall 11 (exit code handling) — error-capturing functions for BestEffort mode

**Files changed:** `api/v1alpha1/config_types.go`, `internal/cloudprovider/aws/userdata.go`, `internal/cloudprovider/aws/warmup.go` + tests

### Phase 2: AL2 and AL2023 Generator Integration
**Rationale:** With the warmup script generator ready, integrate it into the user data generators. AL2 is straightforward (add MIME part). AL2023 requires switching to MIME multipart format, which is a non-trivial change to its output.

**Delivers:**
- AL2Generator updated to insert image-pull script as MIME part 2 (after bootstrap, before warmup completion)
- AL2023Generator switched to MIME multipart when images are specified (NodeConfig as `application/node.eks.aws` + shell script)
- Integration tests for both AMI families with sample image lists

**Uses:**
- MIME multipart helpers (`buildMIMEMultipart`, `mimePartShellScript`) already present in codebase
- AL2023 `application/node.eks.aws` content type for NodeConfig part

**Implements:**
- TS-6 (pull execution in warmup script) — actual injection into user data

**Avoids pitfalls:**
- Pitfall 1 (AL2023 blocking) — addressed by accepting that warmup script blocks nodeadm-run (acceptable since node will be stopped)
- Pitfall 8 (user data size limit) — add size budget check, warn if approaching 16KB

**Files changed:** `internal/cloudprovider/aws/al2.go`, `internal/cloudprovider/aws/al2023.go` + tests

### Phase 3: Controller Data Threading and Status Reporting
**Rationale:** With generators ready, thread the image data from NodePool CRD through the controller to BootstrapConfig. Add observability via conditions and events.

**Delivers:**
- `TemplateConfig` extension with image fields (`internal/cloudprovider/interface.go`)
- Controller threading in `lifecycle/node_launch.go` to copy `pool.Spec.PreWarm.GetImages()` into `TemplateConfig`
- Provider threading in `aws/provider.go` to copy from `TemplateConfig` to `BootstrapConfig`
- NodePool conditions/events for image pull status reporting
- CRD validation (minimal — non-empty strings, no whitespace)
- Documentation updates

**Implements:**
- TS-7 (status reporting) — conditions and events

**Avoids pitfalls:**
- Pitfall 7 (mutable tags) — document recommendation to use digests for deterministic pulls
- Pitfall 9 (Bottlerocket silent no-op) — add validation warning if `bootstrapTemplate=Bottlerocket` with images configured
- Pitfall 12 (CRD validation) — use permissive regex, validate in webhook not CRD schema

**Files changed:** `internal/cloudprovider/interface.go`, `lifecycle/node_launch.go`, `aws/provider.go`, validation webhook (if exists) + tests

### Phase Ordering Rationale

- **Phase 1 first:** CRD types must exist before any code can reference them. The warmup script generator is the core implementation and can be fully tested in isolation using unit tests (no Kubernetes cluster needed).
- **Phase 2 second:** Generators depend on the warmup script generator function existing. AL2023's switch to MIME multipart is isolated to the generator and does not affect controller logic.
- **Phase 3 last:** Data threading is "wiring" — it connects components that must already exist. Status reporting requires the full stack to be functional for integration testing.

**Why this grouping avoids pitfalls:**
- Pitfall 2, 3, 4, 11 (script-level issues) are caught in Phase 1 unit tests
- Pitfall 1, 8 (user data format issues) are caught in Phase 2 integration tests with real AMI configs
- Pitfall 7, 9, 12 (API/UX issues) are addressed in Phase 3 with validation and documentation

### Research Flags

**Phases with standard patterns (skip phase-specific research):**
- **Phase 1:** Bash script generation and CRD field additions are well-understood. The exact `ctr` commands and ECR auth patterns are verified in STACK.md. No additional research needed.
- **Phase 2:** MIME multipart format for AL2023 is documented in AWS EKS launch template docs. Helper functions already exist in codebase. No additional research needed.
- **Phase 3:** Controller data threading follows existing patterns (labels, taints). No additional research needed.

**Phases unlikely to need `/gsd:research-phase`:**
- All phases — the project-level research (STACK, FEATURES, ARCHITECTURE, PITFALLS) is comprehensive and high-confidence. Implementation is straightforward.

**Areas to validate during implementation:**
- AL2023 MIME multipart behavior with real nodeadm (test with actual EKS AL2023 AMI to confirm shell script + NodeConfig coexist)
- Image pinning label persistence across stop/start cycles (verify pinned images survive on stopped-then-started instances)
- IMDS credential timing (confirm `aws ecr get-login-password` works reliably with the wait loop in STACK.md)

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Every binary path, command syntax, and AMI availability verified via official AWS docs and amazon-eks-ami GitHub issues. The `ctr` vs `crictl` decision is backed by evidence from issue #2163 (crictl missing on AL2023). |
| Features | HIGH | Table stakes derived from Kubernetes upstream behavior (image GC, pinning, pull policies) and community patterns. Differentiators validated against containerd documentation. MVP recommendation is conservative and achievable. |
| Architecture | HIGH | Every integration point verified by reading source code directly. The 7-file change list is complete and accurate. Data flow from CRD to user data is traced line-by-line. |
| Pitfalls | HIGH | All critical pitfalls verified with official sources (AWS docs, Kubernetes docs, containerd GitHub issues). Phase-specific warnings map directly to implementation risks. |

**Overall confidence:** HIGH

The feature is well-scoped and the implementation path is clear. No unknown unknowns — all risks are identified with known mitigations.

### Gaps to Address

1. **AL2023 shell script execution timing during nodeadm startup:** Research confirms shell scripts in MIME parts run before nodeadm-run completes (issue #2224). The mitigation is accepting this behavior since warmup nodes are stopped anyway. Validate with a real AL2023 AMI that the shell script + NodeConfig MIME combination works as expected (AWS docs confirm it should, but real-world testing recommended).

2. **Image pinning label persistence on stop/start cycles:** Research confirms pinned labels prevent kubelet GC (Kubernetes PR #103299, containerd issue #6352), but k3s reported label loss after restart (k3s issue #11363). Validate that EKS AMIs (which use upstream containerd, not k3s's fork) do not have this issue. Test: pull and pin an image during warmup, stop instance, start instance, verify `ctr -n k8s.io images ls` shows the pinned label.

3. **User data size budget with realistic image lists:** EC2 user data has a 16KB limit. With 10-15 images (typical for ML workloads), the generated script should stay well under the limit, but the exact size depends on image reference lengths and the verbosity of retry/logging logic. Add a size check in Phase 2 and test with a 20-image list to ensure the limit is not approached.

## Sources

### Primary (HIGH confidence)
- **AWS EKS Launch Templates docs** — AL2023 MIME multipart format, `application/node.eks.aws` content type
- **AWS ECR Authentication docs** — `get-login-password` token mechanism, IAM role-based auth
- **EKS AMI GitHub repository** — Issues #797 (crictl on AL2), #1486 (crictl added), #2163 (crictl NOT on AL2023), #2224 (AL2023 script ordering), #1917 (containerd readiness), #2123 (pre-nodeadm bugs), #1751 (AL2023 boot sequence)
- **Kubernetes v1.32 image docs** — Image reference formats, imagePullPolicy semantics
- **Kubernetes PR #103299** — Pinned image GC protection (GA in 1.23+)
- **containerd GitHub** — Issues #5586 (crictl vs ctr auth), #6352 (pinning support), #7902 (namespace discussion), #9793 (crictl rmi ignores pinned)
- **Stratos codebase** — All files read directly for architecture analysis

### Secondary (MEDIUM confidence)
- **AWS re:Post threads** — ECR auth with `ctr` syntax, IMDS timing issues during user data
- **AWS containers blog** — Image prefetching patterns, SSM-based approach (different mechanism, validates the problem exists)
- **Community production patterns** — Medium articles on EKS image pre-pull using `ctr -n k8s.io`
- **containerd issues** — Parallel pull performance (#4937), parallel unpacking improvements (#8881)

### Tertiary (LOW confidence, needs validation)
- **Karpenter community requests** — Issue #4153 (image pre-pull feature request), issue #3798 (warm node hibernation) — validates user demand but no implementation precedent
- **Third-party tools** — warm-image CRD (mattmoor), kubernetes-image-puller (Eclipse Che) — DaemonSet-based approaches, not applicable to stopped nodes

---
*Research completed: 2026-02-04*
*Ready for roadmap: yes*
