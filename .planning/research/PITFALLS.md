# Domain Pitfalls: Warmup Image Pre-Pull

**Domain:** Adding container image pre-pulling to a Kubernetes operator's warmup process
**Project:** Stratos
**Researched:** 2026-02-04
**Overall confidence:** HIGH (verified against codebase and official sources)

---

## Critical Pitfalls

Mistakes that cause warmup failures, broken nodes, or require architectural changes.

---

### Pitfall 1: AL2023 Shell Script Blocks nodeadm Cluster Join

**What goes wrong:** On AL2023, `text/x-shellscript` MIME parts in user data run *before* `nodeadm-run` completes. If the image pre-pull script is a shell script MIME part, `nodeadm-run` (cluster joining) is blocked until the script finishes. This means a 5-minute image pull delays cluster join by 5 minutes, defeating the purpose of pre-warming.

**Why it happens:** The current AL2023 generator (`internal/cloudprovider/aws/al2023.go`) outputs plain NodeConfig YAML -- no MIME wrapper, no shell script parts. Adding a shell script requires switching to MIME multipart format with `application/node.eks.aws` for NodeConfig and `text/x-shellscript` for the pull script. But cloud-init processes shell scripts before nodeadm-run finishes, creating a blocking dependency.

**Codebase evidence:** `AL2023Generator.Generate()` returns raw NodeConfig YAML (line 37: `return g.generateNodeadmConfig(config), nil`). The test explicitly asserts `if strings.Contains(userData, "MIME-Version")` should be false. Switching to MIME multipart is a breaking change to the AL2023 user data format.

**Consequences:**
- Cluster join delayed by total image pull time
- Warmup timeout (default 10 min) more likely to be hit
- Nodes stuck in warmup state while pulling large images

**Prevention:**
- Do NOT put image pulls in a `text/x-shellscript` MIME part for AL2023
- Use a systemd service approach: write a systemd unit file via the shell script that runs `After=nodeadm-run.service` and `After=containerd.service`
- The systemd service pulls images asynchronously after the node joins the cluster
- Alternative: use the nodeadm drop-in directory (`/etc/eks/nodeadm.d`) for dynamic config, and a separate systemd-based pull mechanism

**Detection:** Warmup duration metrics suddenly increase after enabling image pre-pull. Nodes take 2-5x longer to reach Ready state.

**Severity:** Blocks feature -- if not handled, image pre-pull makes warmup slower instead of faster.

**Which phase should address it:** Phase 1 (warmup script generation) -- this is an architectural decision that must be made upfront.

**Confidence:** HIGH -- verified via [awslabs/amazon-eks-ami#2224](https://github.com/awslabs/amazon-eks-ami/issues/2224), [awslabs/amazon-eks-ami#2123](https://github.com/awslabs/amazon-eks-ami/issues/2123), and codebase analysis.

---

### Pitfall 2: Containerd Socket Not Ready When Pull Script Runs

**What goes wrong:** The image pull script attempts to run `crictl pull` or `ctr pull` before containerd is fully initialized. The containerd socket (`/run/containerd/containerd.sock`) does not exist yet, or containerd's CRI plugin is not ready, causing `rpc error: code = Unknown desc = server is not initialized yet`.

**Why it happens:** The existing warmup script (`internal/cloudprovider/aws/warmup.go`) waits for kubelet health on port 10248, but kubelet health does not guarantee containerd is ready for image pulls. On AL2023, `nodeadm-run` itself can fail if containerd is not ready -- this is a known issue affecting roughly 1 in 4 custom AMI boots.

**Codebase evidence:** The current warmup script polls `http://127.0.0.1:10248/healthz` with a 300s timeout. It has no containerd readiness check.

**Consequences:**
- Image pull commands fail with socket/connection errors
- If using `set -euo pipefail` (as the current warmup script does), one failed pull aborts the entire script
- On AL2023, if the pull runs before containerd is ready, it can interfere with `nodeadm-run` itself

**Prevention:**
- Add an explicit containerd socket wait loop before any pull operations:
  ```bash
  while [ ! -S /run/containerd/containerd.sock ]; do
    sleep 2
  done
  # Also verify CRI is responding
  until crictl info >/dev/null 2>&1; do
    sleep 2
  done
  ```
- Set a timeout on the containerd wait (60s is reasonable -- containerd typically starts in under 10s)
- If using a systemd service, add `After=containerd.service` and `Requires=containerd.service`

**Detection:** Intermittent warmup failures where some nodes pre-pull successfully and others fail. Logs show "connection refused" or "server is not initialized" errors.

**Severity:** Blocks feature -- unreliable warmup if not handled.

**Which phase should address it:** Phase 1 (warmup script generation) -- containerd wait must be built into the pull script.

**Confidence:** HIGH -- verified via [awslabs/amazon-eks-ami#1917](https://github.com/awslabs/amazon-eks-ami/issues/1917) and containerd documentation.

---

### Pitfall 3: Kubelet Image Garbage Collection Removes Pre-Pulled Images

**What goes wrong:** Pre-pulled images are removed by kubelet's image garbage collector before pods are scheduled on the node. Since Stratos nodes sit in standby (stopped) for hours or days, kubelet GC runs when the node is started for scale-up and may evict pre-pulled images before they are used.

**Why it happens:** Kubelet runs image GC every 5 minutes. When disk usage exceeds `imageGCHighThresholdPercent` (default: 85%), it deletes images sorted by longest-unused-first. Pre-pulled images that have never been used by a running container are prime GC targets. When a standby node starts, kubelet resumes and may immediately trigger GC.

**Codebase evidence:** The controller stops instances when nodes reach Ready state (`handleControllerStopWarmup` in `warmup_handlers.go`). Between warmup completion and scale-up, the node is stopped -- images persist on EBS but kubelet GC runs when started.

**Consequences:**
- Images pulled during warmup get garbage collected when the node starts for actual use
- First pod scheduling still hits cold image pull, negating the entire pre-pull benefit
- Worse: users think images are cached but they are not, leading to unexpected latency

**Prevention:**
- **Pin images in containerd** using `ctr -n k8s.io images label <image> io.cri-containerd.pinned=pinned` during the pull script. This requires containerd 1.7+ and kubelet 1.29+ (both available on current EKS AMIs).
- Alternatively, pull with the pinned label: `ctr -n k8s.io images pull --label io.cri-containerd.pinned=pinned <image>`
- Do NOT rely on raising `imageGCHighThresholdPercent` -- this is a blunt tool that affects all images and does not prevent GC entirely
- Do NOT rely on keeping dummy pods running to "use" the images -- this is fragile and wastes resources

**Detection:** Monitor whether images exist on nodes after scale-up. Run `crictl images` after a standby node starts -- if pre-pulled images are missing, GC ate them.

**Severity:** Blocks feature -- without pinning, pre-pull is unreliable and may silently do nothing.

**Which phase should address it:** Phase 1 (warmup script) -- the pull script must pin images as part of the pull operation.

**Confidence:** HIGH -- verified via [containerd image pinning PR](https://github.com/kubernetes/kubernetes/pull/103299), [kubelet GC docs](https://kubernetes.io/docs/concepts/architecture/garbage-collection/), and [containerd pinning docs](https://thelinuxnotes.com/pinning-container-images-in-kubernetes-to-prevent-garbage-collection/).

**Known caveats with pinning:**
- `crictl rmi --prune` does NOT honor pinned labels ([containerd#9793](https://github.com/containerd/containerd/issues/9793))
- Some edge cases exist where pinned images can still be pruned ([containerd#12144](https://github.com/containerd/containerd/issues/12144))
- k3s has reported label loss after restart ([k3s#11363](https://github.com/k3s-io/k3s/issues/11363)) -- verify EKS AMIs do not have this issue

---

### Pitfall 4: Using `crictl pull` vs `ctr pull` in the Wrong Containerd Namespace

**What goes wrong:** Images pulled with `ctr pull` (without `-n k8s.io`) land in containerd's `default` namespace and are invisible to kubelet. Kubelet and crictl operate exclusively in the `k8s.io` namespace.

**Why it happens:** containerd uses namespaces to isolate images. The CRI plugin (used by kubelet) operates in `k8s.io`. The `ctr` command defaults to the `default` namespace. The design spec calls for "auto-detect crictl/ctr" but this detection must account for namespace behavior.

**Consequences:**
- Images appear to pull successfully but are not available when pods are scheduled
- `crictl images` shows no pre-pulled images
- Pod startup still requires a full image pull from the registry
- Silent failure -- no errors, just missing images

**Prevention:**
- **Prefer `crictl pull`** -- it uses the CRI API path identical to kubelet, ensuring images land in the correct namespace
- If using `ctr`, ALWAYS specify `-n k8s.io`: `ctr -n k8s.io images pull <image>`
- For image pinning, use `ctr -n k8s.io images label` (pinning labels must also be in the k8s.io namespace)
- Be aware that in containerd 2.0+, images pulled by `ctr` may have metadata incompatibilities with `crictl` even in the correct namespace
- Test with `crictl images | grep <image>` to verify visibility

**Detection:** After warmup, SSH to node and run `crictl images`. If pre-pulled images are missing but `ctr -n default images ls` shows them, this is the namespace issue.

**Severity:** Blocks feature -- pulled images are useless if not in the right namespace.

**Which phase should address it:** Phase 1 (warmup script) -- the tool selection and namespace handling must be correct from the start.

**Confidence:** HIGH -- verified via [containerd documentation](https://github.com/containerd/containerd/blob/main/docs/cri/crictl.md), [containerd#9773](https://github.com/containerd/containerd/issues/9773), and [crictl vs ctr comparison](https://mirilittleme.medium.com/ctr-vs-crictl-which-one-should-you-use-to-pull-or-push-images-1df4b4b5ad39).

---

### Pitfall 5: Warmup Timeout Exceeded by Large Image Pulls

**What goes wrong:** The existing warmup timeout (default 10 minutes, configurable via `PreWarmConfig.Timeout`) is exceeded because image pulls take longer than expected. The controller then terminates or stops the instance before images are cached.

**Why it happens:** The current timeout (`config_types.go`, `GetTimeout()`) applies to the entire warmup phase -- from instance launch to node Ready. Image pre-pulling adds significant time: a 2GB image over a 1Gbps link takes approximately 16 seconds, but ECR throttling, concurrent pulls, and network conditions can push this to minutes. Five images at 2GB each could easily need 5+ minutes, leaving only 5 minutes for the rest of warmup.

**Codebase evidence:** `handleWarmupRunning()` in `warmup_monitor.go` checks `time.Since(startTime) > timeout` and calls `handleWarmupTimeout()`. The timeout action defaults to "stop" (`TimeoutActionStop`), which wastes the entire warmup effort.

**Consequences:**
- Nodes are stopped/terminated mid-pull, wasting EC2 time and leaving partial images
- If `TimeoutAction=terminate`, the node is destroyed entirely
- Pool never reaches minStandby because warmup keeps failing
- `metrics.RecordWarmupFailure(pool.Name, "timeout")` fires repeatedly

**Prevention:**
- The image pre-pull script should NOT block the node from reaching Ready state. Pull images AFTER the node is Ready but BEFORE the controller stops it
- With the `imagePullPolicy: BestEffort` design, failed pulls should not prevent warmup completion
- For `imagePullPolicy: Required`, add a per-image timeout (e.g., 2 minutes) with overall image pull budget
- Consider increasing the default timeout recommendation in documentation when image pre-pull is enabled
- Log estimated pull time and warn if it approaches the timeout budget

**Detection:** `warmup_failure` metrics with reason "timeout" increase after enabling image pre-pull. Node pool stays in Degraded condition.

**Severity:** Blocks feature for Required policy; degrades quality for BestEffort policy.

**Which phase should address it:** Phase 1 (script design) and Phase 2 (controller integration with warmup readiness check).

**Confidence:** HIGH -- verified from codebase analysis of warmup_monitor.go and warmup_handlers.go.

---

## Moderate Pitfalls

Mistakes that cause delays, technical debt, or degraded functionality.

---

### Pitfall 6: ECR Token Expiry During Long-Running Standby Nodes

**What goes wrong:** For ECR registries, the authentication token obtained during warmup expires after 12 hours. If the node sits in standby for longer than 12 hours and then needs to pull additional images when started, the cached credentials are stale.

**Why it happens:** `aws ecr get-login-password` produces tokens valid for 12 hours. During warmup, the pull script authenticates with ECR. But EKS nodes use the kubelet credential provider to handle ECR auth for pod image pulls, which works independently.

**Prevention:**
- For the warmup pre-pull: this is a non-issue because the token is only needed during the warmup pull, not at pod scheduling time. The kubelet credential provider handles runtime pulls independently.
- For the warmup script itself: the token obtained at instance launch is valid for 12 hours, far longer than any warmup phase (10 min default). No special handling needed.
- Do NOT bake ECR tokens into the user data or store them on disk -- use the IAM instance profile directly during the pull.

**Detection:** Only an issue if the pull script caches tokens to files that are reused on node restart. Check that the script uses `aws ecr get-login-password` inline rather than storing credentials.

**Severity:** Degrades quality if implemented incorrectly; non-issue if IAM instance profile is used correctly.

**Which phase should address it:** Phase 1 (warmup script) -- use `aws ecr get-login-password` inline or rely on crictl's built-in ECR credential handling.

**Confidence:** HIGH -- verified via [AWS ECR auth docs](https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html).

---

### Pitfall 7: Mutable Image Tags Cause Stale Pre-Pulled Images

**What goes wrong:** If users specify images with mutable tags (`:latest`, `:stable`, `:v1`), the pre-pulled image version becomes stale when the tag is updated in the registry. Pods may use `imagePullPolicy: Always` which resolves the new digest and pulls anyway, or `imagePullPolicy: IfNotPresent` which silently uses the stale cached image.

**Why it happens:** Image tags are mutable references. The pre-pulled `:latest` at warmup time may point to digest `sha256:abc`. When a pod is scheduled days later, `:latest` may point to `sha256:def`. Kubelet behavior depends on the pod's `imagePullPolicy`, not the image pre-pull configuration.

**Consequences:**
- With pod `imagePullPolicy: Always`: kubelet resolves new digest, pulls new image anyway -- pre-pull was wasted
- With pod `imagePullPolicy: IfNotPresent`: stale cached image is used silently -- security and correctness risk
- With pod `imagePullPolicy: Never`: works correctly but only if exact image:tag was pre-pulled

**Prevention:**
- Recommend users specify images by digest (`image@sha256:...`) or immutable tags (`image:v1.2.3`) in the NodePool CRD
- Document the interaction between pre-pull and pod `imagePullPolicy`
- Consider adding a CRD validation warning (not error) for `:latest` tags
- Pull by the exact reference specified -- do not attempt to resolve digests in the controller

**Detection:** Users report that pre-pulled images are "not being used" because kubelet pulls a different digest.

**Severity:** Degrades quality -- users may not understand why pre-pull is not helping.

**Which phase should address it:** Phase 3 (CRD design) -- add documentation and optional validation.

**Confidence:** HIGH -- well-documented Kubernetes behavior per [Kubernetes image docs](https://v1-32.docs.kubernetes.io/docs/concepts/containers/images/).

---

### Pitfall 8: EC2 User Data 16KB Limit with Many Images

**What goes wrong:** EC2 user data has a hard 16KB limit (raw, pre-base64 encoding). Adding image pull logic with many image references can push the user data beyond this limit, causing `InvalidParameterValue` on `RunInstances`.

**Why it happens:** The current user data already contains: bootstrap script (AL2) or NodeConfig YAML (AL2023), warmup script, kubelet args, labels, taints, and custom user data. Each image reference adds approximately 80-150 bytes. With 20+ images, the pull script portion alone could be 3-4KB.

**Codebase evidence:** `generateEncodedUserData()` in `provider.go` base64-encodes the entire user data and passes it to `RunInstances`. There is no size validation before encoding.

**Consequences:**
- `LaunchInstance` fails with AWS `InvalidParameterValue` error
- No new nodes can be launched for this pool
- Pool enters Degraded state and cannot recover without user intervention
- Error is not immediately obvious -- it looks like a generic AWS API error

**Prevention:**
- Calculate user data size in the generator and reject if it exceeds 15KB (leaving 1KB margin)
- Use an S3 download pattern for large image lists: store the image list in S3, include only a small download+pull script in user data
- Compress user data with gzip (cloud-init supports `Content-Type: application/gzip` in MIME multipart)
- Add a CRD validation that limits the number of images (e.g., MaxItems=20) or total image reference string length
- Log a warning in the controller when user data approaches the limit

**Detection:** `LaunchInstance` errors with "User data is limited to 16384 bytes." Node pool stuck launching new nodes.

**Severity:** Degrades quality -- only affects pools with many images or large custom user data.

**Which phase should address it:** Phase 1 (script generation) -- size budget estimation. Phase 3 (CRD validation) -- MaxItems constraint.

**Confidence:** HIGH -- verified via [AWS RunInstances docs](https://docs.aws.amazon.com/autoscaling/ec2/APIReference/API_CreateLaunchConfiguration.html) and [user data limit issue](https://github.com/open-guides/og-aws/issues/221).

---

### Pitfall 9: Bottlerocket Has No Shell Script Support

**What goes wrong:** Bottlerocket uses TOML configuration and does not support arbitrary shell scripts in user data. The image pre-pull mechanism designed for AL2/AL2023 does not work on Bottlerocket.

**Why it happens:** Bottlerocket is a locked-down OS. The `BottlerocketGenerator` (`bottlerocket.go`) outputs TOML config and explicitly notes "Bottlerocket does NOT use a warmup script." There is no `text/x-shellscript` support.

**Codebase evidence:** `bottlerocket.go` line 30: `// Note: Bottlerocket does NOT use a warmup script - the controller waits for node Ready and stops the instance.` The test verifies `if strings.Contains(userData, "warmup")` is false.

**Consequences:**
- Image pre-pull cannot be implemented with the same script-based approach on Bottlerocket
- If the feature is advertised as working on all bootstrap templates, Bottlerocket users get silent no-op
- Requires a completely different mechanism (e.g., DaemonSet-based, or Bottlerocket's bootstrap containers)

**Prevention:**
- Phase 1 should explicitly scope image pre-pull to AL2023 and AL2 only
- Add CRD validation: if `bootstrapTemplate=Bottlerocket` and images are configured, emit a warning or error condition
- Document that Bottlerocket support requires a separate implementation approach (Bottlerocket bootstrap containers or in-cluster DaemonSet)
- Bottlerocket has a concept of "bootstrap containers" that could potentially be used for pre-pulling, but this requires separate research

**Detection:** Users on Bottlerocket configure image pre-pull and wonder why it has no effect.

**Severity:** Degrades quality -- silent failure for Bottlerocket users.

**Which phase should address it:** Phase 3 (CRD design) -- validation and clear error messaging. Bottlerocket support deferred to future milestone.

**Confidence:** HIGH -- verified from codebase analysis of bottlerocket.go.

---

### Pitfall 10: Private Registry Authentication Beyond ECR

**What goes wrong:** The pre-pull script uses ECR-specific authentication (`aws ecr get-login-password`). Images from Docker Hub, GCR, GHCR, or private registries require different credentials that are not available on the instance.

**Why it happens:** The design assumes ECR with IAM instance profile auth. For other registries, credentials would need to be passed to the instance. Kubernetes normally handles this via `imagePullSecrets` on pods, but the pre-pull script runs before any pods are scheduled.

**Consequences:**
- Pre-pull fails with 401 Unauthorized for non-ECR images
- With `imagePullPolicy: Required`, this blocks warmup completion
- With `imagePullPolicy: BestEffort`, images silently fail to cache
- Users must put registry credentials in user data (security risk) or find another mechanism

**Prevention:**
- Phase 1: Support ECR-only authentication (IAM instance profile). This covers the majority use case.
- `crictl pull` can authenticate using the kubelet credential provider if kubelet is running, but kubelet may not be fully configured during the pull phase
- For non-ECR: consider fetching credentials from AWS Secrets Manager or SSM Parameter Store in the pull script
- Document which registries are supported and what authentication mechanisms are available
- For `imagePullPolicy: BestEffort`, non-ECR pull failures are acceptable -- the pod will pull at scheduling time using `imagePullSecrets`

**Detection:** Pull errors with "unauthorized" or "authentication required" in warmup logs for non-ECR images.

**Severity:** Degrades quality -- limits feature to ECR initially, which is acceptable for most EKS users.

**Which phase should address it:** Phase 1 (ECR support only). Future: Phase N for generic registry credential support.

**Confidence:** MEDIUM -- ECR auth behavior verified via [AWS docs](https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html). Non-ECR credential handling requires further investigation for specific mechanisms.

---

### Pitfall 11: Exit Code Handling With set -euo pipefail

**What goes wrong:** The existing warmup script uses `set -euo pipefail`. If image pre-pull commands are added to this script and any pull fails, the entire script exits immediately. For `imagePullPolicy: BestEffort`, this is wrong -- the warmup should continue even if pulls fail.

**Why it happens:** `set -e` causes the script to exit on any non-zero exit code. `set -o pipefail` causes pipelines to fail if any command in the pipe fails. These are correct safety defaults for the bootstrap portion but wrong for best-effort image pulls.

**Codebase evidence:** `warmup.go` line 29: `set -euo pipefail`. The warmup script is a single bash script with strict error handling.

**Consequences:**
- One failed image pull aborts all subsequent pulls AND the rest of the warmup script
- A transient network error during one pull cancels the entire warmup
- The node may never reach Ready state because the warmup script exited early

**Prevention:**
- Wrap image pulls in a subshell or function that captures exit codes without triggering `set -e`:
  ```bash
  pull_image() {
    local image="$1"
    crictl pull "$image" 2>/dev/null && return 0
    log "WARNING: Failed to pull $image (continuing)"
    return 1
  }
  ```
- For `imagePullPolicy: Required`, collect failures and exit with error only after attempting all pulls
- For `imagePullPolicy: BestEffort`, log failures but always return 0
- Consider a dedicated function/section for image pulls that temporarily disables `set -e`

**Detection:** Warmup script exits before "Warmup script completed" log message. Only some images are cached.

**Severity:** Degrades quality -- especially bad for BestEffort where partial success is the correct outcome.

**Which phase should address it:** Phase 1 (warmup script generation) -- error handling must be built into the pull section.

**Confidence:** HIGH -- verified from codebase analysis of warmup.go.

---

## Minor Pitfalls

Mistakes that cause annoyance or minor issues but are fixable.

---

### Pitfall 12: CRD Validation Too Strict or Too Loose for Image Strings

**What goes wrong:** kubebuilder validation markers either reject valid image references or accept invalid ones. Image references have complex formats: `registry/repo:tag`, `registry/repo@sha256:...`, `repo:tag` (implicit docker.io), etc.

**Why it happens:** The OCI image reference format is complex. A simple regex like `.+:.+` (from kubebuilder docs) matches `http://foo` but not `nginx` (no tag). A strict regex may reject valid references with ports (`registry:5000/repo:tag`).

**Codebase evidence:** The existing CRD (`nodepool_types.go`, `aws_nodeclass_types.go`) uses kubebuilder markers for validation. The project already has `+kubebuilder:validation:Pattern` precedent but no image-specific patterns.

**Prevention:**
- Use a permissive regex that catches obvious errors without rejecting valid references: `// +kubebuilder:validation:MinLength=1`
- Perform strict validation in the controller webhook, not in CRD schema (where regex is limited and hard to debug)
- Validate that the image string is not empty and does not contain whitespace
- Avoid trying to validate the full OCI reference format in a kubebuilder Pattern marker -- the colon (`:`) in regex patterns can conflict with kubebuilder's marker parsing ([controller-tools#315](https://github.com/kubernetes-sigs/controller-tools/issues/315))
- Test with edge cases: digest references, port numbers in registry URLs, Docker Hub short names

**Detection:** Users report CRD validation errors for valid image references, or invalid references slip through and cause pull failures.

**Severity:** Cosmetic -- can be fixed by loosening/tightening validation later.

**Which phase should address it:** Phase 3 (CRD design).

**Confidence:** HIGH -- verified via [kubebuilder validation docs](https://book.kubebuilder.io/reference/markers/crd-validation) and [controller-tools#315](https://github.com/kubernetes-sigs/controller-tools/issues/315).

---

### Pitfall 13: Sequential vs Parallel Image Pulls Bandwidth Impact

**What goes wrong:** Pulling all images in parallel saturates the network interface, causing all pulls to be slow. Pulling sequentially is safe but slower. Neither extreme is optimal.

**Why it happens:** EC2 instances have finite network bandwidth (varies by instance type). containerd itself may limit concurrent pulls. ECR has per-registry throttling limits.

**Prevention:**
- Default to sequential pulls (simpler, more predictable, no bandwidth contention)
- For advanced users, consider a concurrency option (e.g., 2-3 parallel pulls)
- Use `crictl pull` which inherits containerd's own concurrency limits
- For the initial implementation, sequential is correct -- optimize later based on real-world data
- ECR rate limits: `GetAuthorizationToken` has a rate limit, but image pulls themselves are rate-limited per-IP, not per-API-call

**Detection:** Image pull times are unexpectedly slow on small instance types with many images.

**Severity:** Cosmetic -- sequential pulls are correct default; parallel is an optimization.

**Which phase should address it:** Phase 1 (sequential by default), future optimization.

**Confidence:** MEDIUM -- ECR-specific rate limits need verification for production workloads.

---

### Pitfall 14: AL2 End of Life Affects Long-Term Viability

**What goes wrong:** Amazon stopped publishing EKS-optimized AL2 AMIs as of November 26, 2025. Kubernetes 1.32 is the last version with AL2 support. Investment in AL2 image pre-pull may have limited shelf life.

**Why it happens:** AWS is migrating EKS users from AL2 to AL2023 and Bottlerocket. AL2 MIME multipart with `text/x-shellscript` is the easiest approach for shell scripts, but it is the deprecated platform.

**Codebase evidence:** AL2Generator (`al2.go`) already supports MIME multipart with shell scripts (warmup script as a MIME part). Adding image pre-pull to AL2 is straightforward. But AL2023 is the future.

**Prevention:**
- Implement for both AL2 and AL2023, but prioritize AL2023 as the primary target
- AL2 implementation is simpler (just add another MIME part) and useful for testing the pull logic
- Do not invest heavily in AL2-specific workarounds

**Detection:** N/A -- this is a strategic concern, not a runtime issue.

**Severity:** Cosmetic -- AL2 support is still useful for existing clusters on K8s <= 1.32.

**Which phase should address it:** Phase 1 -- implement both, but AL2023 drives the architecture.

**Confidence:** HIGH -- verified via [AWS EKS AL2 deprecation docs](https://docs.aws.amazon.com/eks/latest/userguide/al2023.html).

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation | Severity |
|-------------|---------------|------------|----------|
| Warmup script generation (Phase 1) | AL2023 shell script blocks nodeadm | Use systemd service approach | Critical |
| Warmup script generation (Phase 1) | Containerd not ready | Add socket/CRI wait loop before pulls | Critical |
| Warmup script generation (Phase 1) | Image GC removes pre-pulled images | Pin images with `io.cri-containerd.pinned` label | Critical |
| Warmup script generation (Phase 1) | Wrong containerd namespace | Use `crictl pull` or `ctr -n k8s.io` | Critical |
| Warmup script generation (Phase 1) | `set -euo pipefail` kills best-effort pulls | Wrap pulls in error-capturing function | Moderate |
| Warmup script generation (Phase 1) | User data exceeds 16KB | Size budget check, S3 fallback for large lists | Moderate |
| Controller integration (Phase 2) | Timeout exceeded by image pulls | Pull after Ready, before stop; budget-aware | Critical |
| CRD design (Phase 3) | Image tag validation too strict/loose | Minimal CRD validation, strict in webhook | Minor |
| CRD design (Phase 3) | Mutable tags cause stale images | Document, warn on `:latest` | Moderate |
| CRD design (Phase 3) | Bottlerocket silent no-op | Validation error for Bottlerocket + images | Moderate |

## Summary: Top 5 Things That Will Go Wrong

1. **AL2023 script ordering**: The shell script blocks nodeadm. Use a systemd service instead.
2. **Image GC eats cached images**: Kubelet garbage collects pre-pulled images during standby. Pin them.
3. **Wrong containerd namespace**: Images pulled with `ctr` default to wrong namespace. Use `crictl pull`.
4. **Warmup timeout budget**: Image pulls consume the 10-minute warmup budget. Pull after Ready.
5. **Containerd not ready**: The pull script runs before containerd socket exists. Wait for it.

## Sources

- [awslabs/amazon-eks-ami#2224 - Post-nodeadm user data support](https://github.com/awslabs/amazon-eks-ami/issues/2224)
- [awslabs/amazon-eks-ami#2123 - Pre-nodeadm script bug](https://github.com/awslabs/amazon-eks-ami/issues/2123)
- [awslabs/amazon-eks-ami#1917 - Nodeadm-run fails if containerd not ready](https://github.com/awslabs/amazon-eks-ami/issues/1917)
- [Kubernetes Garbage Collection docs](https://kubernetes.io/docs/concepts/architecture/garbage-collection/)
- [Preventing Container Image Deletion by Kubelet GC](https://hwchiu.medium.com/preventing-container-image-deletion-by-kubelet-gc-df2fb8788602)
- [Pinning Container Images in Kubernetes](https://thelinuxnotes.com/pinning-container-images-in-kubernetes-to-prevent-garbage-collection/)
- [kubernetes/kubernetes#103299 - Pinned image support PR](https://github.com/kubernetes/kubernetes/pull/103299)
- [containerd#9793 - crictl rmi --prune ignores pinned](https://github.com/containerd/containerd/issues/9793)
- [containerd crictl documentation](https://github.com/containerd/containerd/blob/main/docs/cri/crictl.md)
- [crictl vs ctr comparison](https://mirilittleme.medium.com/ctr-vs-crictl-which-one-should-you-use-to-pull-or-push-images-1df4b4b5ad39)
- [AWS ECR authentication docs](https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html)
- [AWS EKS AL2023 migration docs](https://docs.aws.amazon.com/eks/latest/userguide/al2023.html)
- [nodeadm official documentation](https://awslabs.github.io/amazon-eks-ami/nodeadm/)
- [Kubernetes container images docs](https://v1-32.docs.kubernetes.io/docs/concepts/containers/images/)
- [kubebuilder CRD validation markers](https://book.kubebuilder.io/reference/markers/crd-validation)
- [controller-tools#315 - Pattern regex colon issue](https://github.com/kubernetes-sigs/controller-tools/issues/315)
- [EC2 user data size limit](https://github.com/open-guides/og-aws/issues/221)
