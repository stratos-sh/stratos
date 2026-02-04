# Feature Landscape: Warmup Image Pre-Pull

**Domain:** Kubernetes node image pre-pulling during warmup lifecycle
**Researched:** 2026-02-04
**Confidence:** HIGH (verified against Kubernetes upstream behavior, containerd/CRI documentation, AWS EKS patterns, and existing Stratos warmup architecture)

---

## Context

Stratos maintains pools of pre-warmed EC2 instances. During warmup, an instance launches,
runs user data (bootstrap + warmup script), and the controller stops it into standby when
the node becomes Ready. The image pre-pull feature adds container image pulling to the
warmup phase so that when a standby node starts for a pending pod, the required images are
already cached on disk -- eliminating the image pull delay from the critical path.

Design decisions already made:
- Images configured on `NodePool.spec.warmup.images` (not AWSNodeClass)
- `imagePullPolicy`: `Required` (default) or `BestEffort`
- Auto-detect `crictl` / `ctr` on the node
- AL2023 + AL2 only (Bottlerocket deferred)

---

## Table Stakes

Features users expect. Missing any of these makes image pre-pull incomplete or untrustworthy.

### TS-1: Image List Specification on NodePool CRD

| Aspect | Detail |
|--------|--------|
| **What** | `spec.warmup.images` field accepting a list of image references |
| **Why expected** | This is the fundamental input. Without it, nothing works. |
| **Complexity** | LOW |
| **Dependencies** | None (new CRD field) |

**Image reference format must support:**
- Tag references: `nginx:1.25`, `myapp:v2.1.0`
- Digest references: `nginx@sha256:abcdef...`
- Combined tag+digest: `nginx:1.25@sha256:abcdef...`
- Full registry paths: `123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp:v1`
- Default registry (docker.io): `nginx:latest`
- No tag (implies `:latest`): `nginx`

**Evidence:** Kubernetes upstream (v1.32 images documentation) supports all these formats
in pod specs. The CRI `PullImageRequest` accepts any of these as the image reference
string. Both `crictl pull` and `ctr -n k8s.io images pull` accept the same formats.
[Source: kubernetes.io/docs/concepts/containers/images/]

**CRD validation recommendation:** Use a regex pattern validator in the kubebuilder marker.
The image reference format is `[registry/][repository/]name[:tag][@sha256:digest]`. Do NOT
over-validate -- allow the container runtime to be the authority on whether a reference is
valid. Basic validation (non-empty string, no whitespace) is sufficient at the CRD level.

**Confidence:** HIGH -- standard Kubernetes image reference semantics.

### TS-2: Required vs BestEffort Image Pull Policy

| Aspect | Detail |
|--------|--------|
| **What** | Per-pool `imagePullPolicy` field: `Required` (default) or `BestEffort` |
| **Why expected** | Users need to control whether a failed image pull blocks node readiness |
| **Complexity** | LOW |
| **Dependencies** | TS-1 |

**Semantics:**

- **Required** (default): ALL images in the list must be pulled successfully. If any image
  fails after retries, the warmup is considered failed. The controller's existing
  `PreWarm.TimeoutAction` (`stop` or `terminate`) applies. This means a node with failed
  image pulls either sits in warmup until timeout (then gets stopped/terminated) or the
  warmup script exits non-zero and the controller handles the failure.

- **BestEffort**: Image pulls are attempted but failures do not block warmup completion.
  The node proceeds to Ready state regardless. This is for cases where pre-pulled images
  are a performance optimization, not a correctness requirement.

**Analogy to Kubernetes imagePullPolicy:** This is NOT the same as Kubernetes' `Always` /
`IfNotPresent` / `Never`. Stratos's policy controls whether image pull failure blocks node
warmup, not whether the runtime checks the registry. The naming `Required` / `BestEffort`
is more appropriate than reusing Kubernetes terminology, which would cause confusion.

**No ecosystem precedent for exact naming:** The specific "Required vs BestEffort" naming
for pre-pull policy does not appear in any existing Kubernetes tool (searched Karpenter,
Cluster Autoscaler, warm-image CRD, kubernetes-image-puller operator). This is a Stratos
innovation. The semantics are clear and the naming is intuitive.

**Confidence:** HIGH -- the semantics are straightforward; naming is a design choice.

### TS-3: Auto-Detection of Container Runtime CLI

| Aspect | Detail |
|--------|--------|
| **What** | Warmup script auto-detects whether to use `crictl` or `ctr` for image pulling |
| **Why expected** | Different AMIs and configurations may have different CLIs available |
| **Complexity** | LOW |
| **Dependencies** | None |

**Why this matters:**

- `crictl pull` goes through the CRI plugin, which respects CRI-level registry
  authentication configuration (including the kubelet credential provider for ECR).
  This is the preferred path because it matches how Kubernetes itself pulls images.

- `ctr -n k8s.io images pull` talks directly to the containerd API, bypasses CRI plugin
  auth config, and requires explicit `--user` flags or Docker config for authentication.
  It is a fallback for environments where `crictl` is not available.

**Detection order:**
1. Check for `crictl` in PATH (preferred -- CRI-aligned, uses kubelet's auth chain)
2. Fall back to `ctr` (direct containerd API, requires `-n k8s.io` namespace)
3. If neither found, log error and:
   - `Required` policy: exit non-zero (fail warmup)
   - `BestEffort` policy: skip image pull, log warning

**Key difference:** `crictl pull` uses the CRI plugin's registry auth (reads from containerd
config.toml's `[plugins."io.containerd.grpc.v1.cri".registry]` section and works with the
kubelet credential provider). `ctr pull` does NOT use CRI auth and requires separate
credential handling. [Source: GitHub containerd/containerd#5586, crictl vs ctr authentication
analysis]

**Confidence:** HIGH -- verified through containerd documentation and GitHub issues.

### TS-4: ECR Authentication During Image Pull

| Aspect | Detail |
|--------|--------|
| **What** | Pre-pulled images from ECR must authenticate using the node's IAM role |
| **Why expected** | Most Stratos users will pull from private ECR registries |
| **Complexity** | MEDIUM |
| **Dependencies** | TS-3 (runtime detection determines auth path) |

**How ECR auth works at the node level:**

1. **With `crictl` (preferred path):** On EKS-optimized AMIs (AL2, AL2023), the kubelet
   credential provider (`ecr-credential-provider`) is pre-configured. When `crictl pull`
   invokes the CRI, containerd delegates to the kubelet credential provider, which uses
   the EC2 instance's IAM role via IMDS to get ECR credentials. This happens
   automatically -- no additional configuration needed if the instance profile has
   `ecr:GetAuthorizationToken`, `ecr:BatchGetImage`, and `ecr:GetDownloadUrlForLayer`
   permissions.

2. **With `ctr` (fallback path):** Direct containerd API calls do NOT use the kubelet
   credential provider. The warmup script must explicitly obtain ECR credentials:
   ```bash
   ECR_TOKEN=$(aws ecr get-login-password --region $REGION)
   ctr -n k8s.io images pull --user "AWS:$ECR_TOKEN" $IMAGE
   ```
   This requires AWS CLI to be installed and the instance profile to have ECR permissions.

3. **Timing concern:** During warmup, the kubelet credential provider may not yet be
   initialized when the warmup script runs. The warmup script currently waits for kubelet
   health (`http://127.0.0.1:10248/healthz`). Image pulls should happen AFTER kubelet is
   healthy, ensuring the credential provider is ready. This is already the correct ordering
   in the existing warmup script architecture.

**IAM permissions:** The instance profile (configured via `AWSNodeClass.spec.iamInstanceProfile`
or `AWSNodeClass.spec.role`) must include ECR read permissions. This is typically already
present for EKS node groups. Document this as a prerequisite, not something Stratos manages.

**ECR token expiry:** ECR tokens expire after 12 hours. This is NOT a concern for warmup
because the pull happens once during a short warmup phase (minutes, not hours). However, if
a node sits in warmup for an extended period (timeout approaching), a stale token could
cause retries to fail. This is an edge case that the retry mechanism (TS-5) handles
naturally.

**Confidence:** HIGH -- ECR authentication via IAM instance profiles on EKS is well-documented
and standard. [Source: kubernetes.io/blog/2022/12/22/kubelet-credential-providers/,
cloud-provider-aws.sigs.k8s.io/credential_provider/]

### TS-5: Image Pull Retries with Backoff

| Aspect | Detail |
|--------|--------|
| **What** | Failed image pulls are retried with exponential backoff |
| **Why expected** | Transient failures (network, rate limits, ECR throttling) are common |
| **Complexity** | LOW |
| **Dependencies** | TS-3 |

**Retry pattern (recommended):**
- Initial wait: 5 seconds
- Backoff multiplier: 2x
- Max wait: 60 seconds
- Max attempts: 5 (configurable is a differentiator, hardcoded is table stakes)
- Sequence: 5s, 10s, 20s, 40s, 60s

**Why this specific pattern:** Kubernetes kubelet uses exponential backoff for image pulls
(5s, 10s, 20s, 40s, up to 5 minutes). For warmup, the total retry window should be shorter
because the warmup timeout (default 10 minutes) is the outer bound. With 5 retries at this
backoff, total retry time is ~135 seconds per image, leaving room for multiple images within
the timeout.

**What to retry:** Only retry on exit codes indicating transient failure. Non-zero exit from
`crictl pull` / `ctr pull` should be retried by default. The script cannot easily distinguish
"image not found" (permanent) from "network timeout" (transient) based on exit codes alone.
Retrying a non-existent image wastes time but does not cause harm -- the warmup timeout
provides the safety net.

**Behavior per policy:**
- `Required`: Retry all images. If any image exhausts retries, warmup fails.
- `BestEffort`: Retry all images. If any image exhausts retries, log warning and continue.

**Confidence:** HIGH -- exponential backoff is the universal pattern. The specific values
are recommendations that can be tuned.

### TS-6: Pull Execution in Warmup Script

| Aspect | Detail |
|--------|--------|
| **What** | Image pull commands injected into the warmup user data script |
| **Why expected** | This is the execution mechanism -- without it, the CRD field is decorative |
| **Complexity** | MEDIUM |
| **Dependencies** | TS-1, TS-3, TS-4, TS-5 |

**Integration with existing warmup architecture:**

The current warmup flow (from `warmup.go` and `al2.go`):
1. AL2: MIME multipart with bootstrap.sh + warmup script + optional custom user data
2. AL2023: nodeadm config (no MIME for bootstrap, but warmup could be added as MIME part)
3. Warmup script waits for kubelet health, then signals completion

Image pulls should be injected AFTER kubelet health check (to ensure credential provider
is ready) and BEFORE warmup completion signal:

```
1. Kubelet health wait (existing)
2. Image pre-pull loop (NEW)
3. Warmup completion signal (existing)
```

**Script generation:** The `BootstrapConfig` struct needs the image list and pull policy.
The warmup script generator (`getWarmupScript()`) currently returns a static string. It
must become parameterized to inject the image pull loop.

**Per-image execution:**
```bash
for IMAGE in "${IMAGES[@]}"; do
    pull_with_retry "$IMAGE" "$MAX_RETRIES"
    # Handle exit code based on policy
done
```

**Confidence:** HIGH -- direct integration point is clear from codebase analysis.

### TS-7: Status Reporting for Image Pulls

| Aspect | Detail |
|--------|--------|
| **What** | NodePool status reflects image pull state during warmup |
| **Why expected** | Users need to know if warmup is slow because of image pulls |
| **Complexity** | MEDIUM |
| **Dependencies** | TS-1, TS-2 |

**What the controller can observe:**
The controller does not have direct visibility into the warmup script's progress. The
controller sees:
- Node in `warmup` state (launched, not yet Ready)
- Time since warmup started (via `stratos.sh/state-since` label)
- Warmup timeout approaching

**Minimum viable status:**
- `ImagePullsConfigured` condition on NodePool: True when images are specified
- Log-level reporting: The warmup script logs image pull progress to instance console output
  (viewable via EC2 console or `aws ec2 get-console-output`)

**What the warmup script reports (on the node):**
- Per-image: pull started, pull succeeded, pull failed (with attempt number)
- Summary: X/Y images pulled successfully

**Kubernetes event reporting:**
- Event on NodePool when warmup completes with all images pulled
- Event on NodePool when warmup completes with failed images (BestEffort mode)
- Event on NodePool when warmup fails due to image pull failure (Required mode)

This does NOT require real-time streaming of image pull progress from the node to the
controller. The controller learns the outcome when the node either becomes Ready (success)
or times out (failure).

**Confidence:** MEDIUM -- the exact status reporting design has flexibility. The above is
the minimum that provides operational visibility without over-engineering.

---

## Differentiators

Features that set Stratos apart. Not expected by users but valuable when present.

### D-1: Image Pull Ordering (Largest First)

| Aspect | Detail |
|--------|--------|
| **What** | Pull larger images first to optimize total warmup time |
| **Value** | Reduces worst-case warmup latency by starting the heaviest pull earliest |
| **Complexity** | MEDIUM |
| **Dependencies** | TS-6 |

**Rationale:** If a user specifies [nginx:1.25 (50MB), pytorch:2.0 (5GB), redis:7 (30MB)],
pulling pytorch first means its download overlaps with subsequent smaller pulls. Pulling it
last means the total wall-clock time is dominated by the largest image.

**Implementation approach:** The warmup script could query image sizes before pulling
(using registry API or `crane manifest`), but this adds complexity and an extra API call.

**Simpler approach:** Allow users to specify pull order explicitly by defining the list
order as the pull order. Document that users should list their largest images first.
This is zero implementation cost and gives users full control.

**Ecosystem context:** Kubernetes upstream has discussed image pull ordering (kubernetes
issue #108405 -- prioritizing critical pods' images) but has not implemented it. No existing
tool offers automatic size-based ordering for pre-pulls.

**Recommendation:** Defer automatic ordering. Use list order as pull order and document the
recommendation to list largest images first. This is a convention, not code.

**Confidence:** MEDIUM -- the optimization is real but the simple solution (document
list ordering) may be sufficient.

### D-2: Parallel Image Pulls

| Aspect | Detail |
|--------|--------|
| **What** | Pull multiple images concurrently during warmup |
| **Value** | Reduces total warmup time when pulling multiple images |
| **Complexity** | MEDIUM |
| **Dependencies** | TS-6 |

**Ecosystem context:**
- Kubelet's `serializeImagePulls` defaults to true (sequential). When false, kubelet pulls
  in parallel, controlled by `maxParallelImagePulls`.
- Containerd has known issues with parallel pull performance when cold (GitHub
  containerd/containerd#4937). Multiple parallel pulls of different images can contend for
  disk I/O during layer extraction.
- Containerd's parallel layer unpacking (GitHub containerd/containerd#8881) achieved 3x
  faster pulls for large images, but this is per-image parallelism, not cross-image.

**Trade-offs:**
- Parallel pulls reduce wall-clock time for multiple small-medium images
- Parallel pulls can HURT performance for large images due to disk I/O contention
- Memory usage increases with parallelism (containerd PR #10177 found parallelism > 4
  yields diminishing returns and higher memory)

**Recommendation:** Start with sequential pulls (simpler, predictable). Add an optional
`maxParallelPulls` field later if users request it. Sequential is what kubelet defaults to,
and it is safer for the warmup phase where disk I/O is already being used for EBS
initialization.

**Confidence:** HIGH -- the trade-offs are well-documented in containerd issues.

### D-3: Image Pin for Garbage Collection Protection

| Aspect | Detail |
|--------|--------|
| **What** | Pin pre-pulled images so kubelet GC does not evict them |
| **Value** | Pre-pulled images survive kubelet garbage collection under disk pressure |
| **Complexity** | LOW-MEDIUM |
| **Dependencies** | TS-3 (requires `ctr` access for pinning) |

**The problem:** Kubelet periodically runs image garbage collection (every 5 minutes by
default). When `imageFs` usage exceeds `imageGCHighThresholdPercent` (default 85%), kubelet
removes unused images (not referenced by running containers) until usage drops below
`imageGCLowThresholdPercent` (default 80%). Pre-pulled images that are not yet used by any
running container are candidates for GC.

**The CRI pinned image mechanism:**
- CRI API v1 supports a `pinned` field on images (Kubernetes v1.23+, merged via PR #103299)
- Containerd supports the `io.cri-containerd.pinned=pinned` label
- Pinned images are excluded from kubelet GC
- Set via: `ctr -n k8s.io images label <image> io.cri-containerd.pinned=pinned`

**When this matters for Stratos:**
- Standby nodes sit stopped. When started, they have pre-pulled images but no running
  containers referencing them yet. The kubelet could theoretically GC these images before
  pods are scheduled.
- In practice, the window is very short (seconds between node start and pod scheduling),
  and GC only triggers under disk pressure. But for correctness, pinning is the right
  approach.

**Caveat:** `crictl` does not support setting the pinned label (it is a CRI-level attribute,
not a `crictl` command). Pinning requires `ctr -n k8s.io images label`. This means even if
`crictl` is used for pulling, `ctr` is needed for pinning. Both are available on EKS AMIs.

**Another caveat:** There is a known issue where `crictl rmi` (image prune) does not honor
the pinned flag (kubernetes-sigs/cri-tools#1356). This is for manual pruning only and does
not affect kubelet GC, which does honor the pinned flag.

**Recommendation:** Implement pinning as part of the pull loop. After each successful
`crictl pull`, run `ctr -n k8s.io images label <image> io.cri-containerd.pinned=pinned`.
The overhead is negligible and the protection is meaningful.

**Confidence:** HIGH -- pinned image support is GA since Kubernetes 1.23, well-documented.
[Source: hwchiu.medium.com, github.com/kubernetes/kubernetes/pull/103299,
github.com/containerd/containerd/issues/6352]

### D-4: Image Pull Size/Duration Logging

| Aspect | Detail |
|--------|--------|
| **What** | Log image size and pull duration for each image during warmup |
| **Value** | Operational visibility for tuning warmup timeout and image selection |
| **Complexity** | LOW |
| **Dependencies** | TS-6 |

**What to log:**
```
[2026-02-04 10:00:05] Pulling image 1/3: 123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp:v1
[2026-02-04 10:00:35] Pulled image 1/3: myapp:v1 (1.2 GB, 30s)
[2026-02-04 10:00:35] Pulling image 2/3: nginx:1.25
[2026-02-04 10:00:38] Pulled image 2/3: nginx:1.25 (50 MB, 3s)
```

**Implementation:** After `crictl pull` completes, query image size with
`crictl images --output json | jq` or `crictl inspecti`. Record wall-clock duration with
bash `$SECONDS` or `date` arithmetic.

**Recommendation:** Include in initial implementation. Trivial cost, high diagnostic value.

**Confidence:** HIGH -- straightforward logging enhancement.

### D-5: Warmup Timeout Auto-Adjustment

| Aspect | Detail |
|--------|--------|
| **What** | Warn users if configured images are likely to exceed warmup timeout |
| **Value** | Prevents frustrating timeout failures due to large image lists |
| **Complexity** | HIGH |
| **Dependencies** | TS-1, TS-7 |

**The problem:** Default warmup timeout is 10 minutes. If a user specifies 5 large images
(total 20GB), the pull alone could take 10+ minutes depending on network speed, causing
timeouts.

**Approaches:**
- Validation webhook that warns (not rejects) when image count exceeds a threshold
- Controller-side condition that suggests increasing timeout based on observed pull times
- Documentation-only: "For N large images, increase warmup timeout to X"

**Recommendation:** Defer to post-MVP. Documentation guidance is sufficient initially.
After observing real-world pull times, the controller can surface suggestions via
conditions.

**Confidence:** LOW -- auto-adjustment requires understanding of network bandwidth, image
sizes, and pull parallelism, which varies per environment.

### D-6: Image Digest Resolution and Pinning

| Aspect | Detail |
|--------|--------|
| **What** | Resolve tag references to digests and store resolved digest in status |
| **Value** | Guarantees the exact image version across all pool nodes |
| **Complexity** | MEDIUM |
| **Dependencies** | TS-1 |

**The problem:** If a user specifies `myapp:latest` and two nodes warm up at different
times, they could get different image versions if the tag was updated between warmups.

**Approach:** The controller (not the warmup script) could resolve tags to digests at
reconciliation time and pass the resolved digest to the warmup script. This requires
the controller to have registry access (ECR API) which it currently does not need.

**Recommendation:** Defer. Users who care about exact versions should use digests in their
image specifications. This is the standard Kubernetes guidance (Google Cloud documentation
explicitly recommends digests for deterministic deployments). Adding registry access to the
controller is a significant scope increase.

**Confidence:** MEDIUM -- the problem is real but the solution (use digests) already exists.

---

## Anti-Features

Features to deliberately NOT build. Common mistakes in this domain.

### AF-1: Do NOT Build a DaemonSet-Based Image Pre-Puller

**What it is:** Deploy a DaemonSet that runs on every node to pre-pull images (the
"warm-image CRD" pattern used by mattmoor/warm-image, Eclipse Che's kubernetes-image-puller,
and the Codefresh single-use DaemonSet pattern).

**Why it is tempting:** DaemonSets are a well-known Kubernetes pattern for running something
on every node. Several existing tools use this approach.

**Why avoid for Stratos:** Stratos nodes in standby are STOPPED EC2 instances. DaemonSets
cannot run on stopped instances. The entire point of warmup image pre-pull is that it
happens BEFORE the instance stops. A DaemonSet would only run when the node starts (defeating
the purpose) or would require keeping nodes running during warmup (which Stratos already
does, but DaemonSets add scheduling complexity). The user data script is the correct
execution mechanism because it runs during the one-time warmup phase.

**What to do instead:** Image pulls in the warmup user data script, as designed.

### AF-2: Do NOT Build Image Layer Caching or Snapshotting

**What it is:** Create EBS snapshots of container image layers to attach to new instances,
or maintain a shared image cache across the pool.

**Why it is tempting:** AWS documentation and Karpenter community suggest EBS snapshots as
a way to pre-cache images. Bottlerocket specifically supports a secondary data volume that
can be snapshotted.

**Why avoid for Stratos:** This is enormously complex (EBS snapshot lifecycle management,
cross-AZ snapshot copies, snapshot staleness). It solves a different problem -- making image
data available without any pull. Stratos already solves the pull-time problem by doing the
pull during warmup when time is not critical. The marginal benefit of snapshot-based caching
over warmup-time pulling is small compared to the implementation complexity.

**What to do instead:** Pull images during warmup with `crictl pull`. Simple, reliable,
uses standard tooling.

### AF-3: Do NOT Support Per-Image Pull Policy

**What it is:** Allow `Required` or `BestEffort` to be set per-image rather than per-pool.

**Why it is tempting:** Some images might be critical (app image) while others are optional
(monitoring sidecar).

**Why avoid:** This adds CRD complexity (each image entry becomes a struct with `name` and
`policy` fields instead of a simple string list). For MVP, a single pool-level policy
covers the common case. Users who need mixed policies can use `BestEffort` at the pool
level (all images are best-effort) or split into separate pools.

**What to do instead:** Pool-level `imagePullPolicy` that applies to all images in the list.
Revisit per-image policy only if users request it post-launch.

### AF-4: Do NOT Support Registry Authentication Configuration in the CRD

**What it is:** Add fields for registry credentials, pull secrets, or auth configuration
to the NodePool CRD.

**Why it is tempting:** Some tools (warm-image CRD) accept `imagePullSecrets` references
for private registries.

**Why avoid:** Stratos runs on AWS. ECR authentication happens automatically via the
instance profile's IAM role and the kubelet credential provider. Adding CRD-level auth
would be:
- Redundant with the standard AWS auth chain
- A security concern (credentials in CRDs)
- Scope creep (supporting arbitrary registries with arbitrary auth)

For non-ECR registries, the node's containerd configuration handles auth. This is
infrastructure-level config, not application-level config.

**What to do instead:** Document that ECR auth is automatic via IAM role. For non-ECR
registries, users configure containerd registry auth on the AMI or via custom user data.

### AF-5: Do NOT Build Real-Time Image Pull Progress Streaming

**What it is:** Stream image pull progress (layer download percentages) from the warmup
node to the controller in real time.

**Why it is tempting:** Kubernetes issue #19077 (from 2015) requests image pull progress
visibility. Users want to know if their 5GB image is 10% or 90% done.

**Why avoid:** This would require a communication channel from the warmup script to the
controller (SSM, CloudWatch, node annotation updates). The warmup phase runs BEFORE the
node is fully joined to the cluster, so Kubernetes API annotations are not available.
Building a side-channel for progress streaming is high complexity for modest value.

**What to do instead:** Log pull progress on the instance (viewable via EC2 console output).
Report success/failure via the existing warmup outcome mechanism (node becomes Ready or
times out). This provides adequate visibility for the v1 implementation.

### AF-6: Do NOT Make Retry Count Configurable in MVP

**What it is:** Add `spec.warmup.images.maxRetries` or similar CRD field.

**Why it is tempting:** Different environments have different failure characteristics.

**Why avoid for MVP:** The warmup timeout is the effective retry budget. With sensible
defaults (5 retries, exponential backoff), most transient failures resolve. Adding
configurability increases CRD surface area without clear user demand. The warmup timeout
is already configurable and serves as the outer bound.

**What to do instead:** Hardcode sensible defaults (5 retries, 5-60s backoff). Add CRD
configurability only if post-launch feedback demands it.

---

## Feature Dependencies

```
TS-1: Image List CRD Field
  |
  +--> TS-2: Required/BestEffort Policy (needs image list to apply to)
  |
  +--> TS-6: Pull Execution in Warmup Script (needs image list to iterate)
  |     |
  |     +--> TS-3: Runtime CLI Detection (crictl vs ctr)
  |     |
  |     +--> TS-4: ECR Authentication (depends on which CLI is used)
  |     |
  |     +--> TS-5: Retry with Backoff (wraps the pull command)
  |
  +--> TS-7: Status Reporting (needs image list for condition reporting)

Differentiators:
  D-1: Pull Ordering     --> depends on TS-6
  D-2: Parallel Pulls    --> depends on TS-6
  D-3: Image Pinning     --> depends on TS-3 (needs ctr for pinning)
  D-4: Size/Duration Log --> depends on TS-6
  D-5: Timeout Warning   --> depends on TS-1, TS-7
  D-6: Digest Resolution --> depends on TS-1
```

---

## MVP Recommendation

**For MVP, implement all table stakes (TS-1 through TS-7) plus D-3 (image pinning) and D-4 (size/duration logging).**

These nine items provide a complete, production-ready image pre-pull feature:

1. **TS-1**: Image list on CRD -- the input
2. **TS-2**: Required/BestEffort policy -- the failure mode control
3. **TS-3**: Runtime detection -- the execution prerequisite
4. **TS-4**: ECR authentication -- the auth path
5. **TS-5**: Retry with backoff -- the resilience layer
6. **TS-6**: Warmup script integration -- the execution mechanism
7. **TS-7**: Status reporting -- the observability layer
8. **D-3**: Image pinning -- GC protection (low cost, high correctness value)
9. **D-4**: Size/duration logging -- diagnostic value (trivial cost)

**Defer to post-MVP:**
- D-1 (Pull ordering): Document "list largest first" instead
- D-2 (Parallel pulls): Sequential is safer and simpler for v1
- D-5 (Timeout auto-adjustment): Documentation guidance is sufficient
- D-6 (Digest resolution): Users can specify digests manually

**Implementation ordering within MVP:**
1. CRD types (TS-1, TS-2) -- defines the API contract
2. Warmup script generation (TS-3, TS-4, TS-5, TS-6, D-3, D-4) -- the execution logic
3. Controller integration (TS-7) -- status reporting

---

## Comparison: How Other Tools Handle This

| Tool | Image Pre-Pull Method | Pre-Pull Timing | Failure Handling |
|------|----------------------|-----------------|-----------------|
| **Karpenter** | None (community uses custom AMIs or EBS snapshots) | N/A | N/A |
| **Cluster Autoscaler + Warm Pools** | DaemonSet-based cache image + warm pool | Node join time (DaemonSet runs on live nodes) | Pod restart backoff |
| **warm-image CRD (mattmoor)** | DaemonSet on every node | Continuous (DaemonSet always running) | DaemonSet restart |
| **kubernetes-image-puller (Eclipse Che)** | DaemonSet per image list | Continuous | CRD-configured |
| **Custom AMI ("fully baked")** | Images baked into AMI | AMI build time | Build fails |
| **AWS SSM-based** | SSM Run Command on nodes | On-demand via SSM | SSM status |
| **Stratos (proposed)** | User data script during warmup | During one-time warmup before stop | Required/BestEffort policy |

**Stratos differentiator:** The warmup user data approach is unique because it runs during
a dedicated warmup phase before the node enters standby. No other tool has this timing
advantage -- they either pull when the node is already serving traffic (DaemonSet) or
require pre-baking (custom AMI). Stratos pulls during "free time" when the node is
initializing but not yet needed.

---

## Sources

### HIGH Confidence
- Kubernetes v1.32 images documentation: https://v1-32.docs.kubernetes.io/docs/concepts/containers/images/
- Kubernetes kubelet credential providers (GA v1.26): https://kubernetes.io/blog/2022/12/22/kubelet-credential-providers/
- AWS ECR credential provider for kubelet: https://cloud-provider-aws.sigs.k8s.io/credential_provider/
- Containerd pinned image support: https://github.com/containerd/containerd/issues/6352
- Kubernetes PR #103299 (pinned image GC protection): https://github.com/kubernetes/kubernetes/pull/103299
- Containerd crictl vs ctr authentication: https://github.com/containerd/containerd/issues/5586
- CRI tools crictl documentation: https://github.com/kubernetes-sigs/cri-tools/blob/master/docs/crictl.md
- Kubelet image GC thresholds: https://kubernetes.io/docs/concepts/architecture/garbage-collection/

### MEDIUM Confidence
- Containerd parallel pull performance: https://github.com/containerd/containerd/issues/4937
- Containerd parallel unpacking (3x improvement): https://github.com/containerd/containerd/issues/8881
- Kubernetes parallel image pull promotion: https://github.com/kubernetes/kubernetes/issues/108405
- Pinned image GC protection guide: https://hwchiu.medium.com/preventing-container-image-deletion-by-kubelet-gc-df2fb8788602
- AWS containers blog (image prefetching): https://aws.amazon.com/blogs/containers/start-pods-faster-by-prefetching-images/
- EKS warm pool pattern: https://github.com/aws-samples/eks-node-group-with-warm-pool

### LOW Confidence
- Karpenter image pre-pull community request: https://github.com/aws/karpenter/issues/4153
- Karpenter warm node hibernation request: https://github.com/aws-samples/eks-node-group-with-warm-pool/issues/3798
- warm-image CRD (mattmoor): https://github.com/mattmoor/warm-image
- crictl pinned image prune issue: https://github.com/kubernetes-sigs/cri-tools/issues/1356

### Codebase Analysis (HIGH Confidence)
- Stratos NodePool CRD: `/home/roeeh/projects/presto/api/v1alpha1/nodepool_types.go`
- Stratos PreWarmConfig: `/home/roeeh/projects/presto/api/v1alpha1/config_types.go`
- Stratos warmup script: `/home/roeeh/projects/presto/internal/cloudprovider/aws/warmup.go`
- Stratos user data generation: `/home/roeeh/projects/presto/internal/cloudprovider/aws/userdata.go`
- Stratos AL2 bootstrap: `/home/roeeh/projects/presto/internal/cloudprovider/aws/al2.go`
- Stratos AL2023 bootstrap: `/home/roeeh/projects/presto/internal/cloudprovider/aws/al2023.go`

---
*Feature research for: Stratos operator warmup image pre-pull*
*Researched: 2026-02-04*
