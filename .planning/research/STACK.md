# Technology Stack: Warmup Image Pre-Pull

**Project:** Stratos Kubernetes Operator -- Warmup Image Pre-Pull Feature
**Researched:** 2026-02-04
**Overall confidence:** HIGH (verified with official AWS docs, EKS AMI GitHub issues, and community production patterns)

This document covers the exact binaries, paths, commands, and integration points needed to add container image pre-pulling during the Stratos warmup phase on EKS-optimized AL2023 and AL2 AMIs.

---

## Executive Assessment

Image pre-pulling during warmup requires **zero new Go module dependencies**. The entire feature is implemented by generating bash script fragments that call `ctr` (containerd's CLI, pre-installed on all EKS AMIs) to pull images into the `k8s.io` containerd namespace before the instance is stopped into standby. ECR authentication uses `aws ecr get-login-password` (AWS CLI is pre-installed) backed by the instance profile's IAM role.

The primary complexity is AMI-family-specific user data formats:
- **AL2**: Straightforward -- add a new MIME part (shell script) to the existing multipart structure.
- **AL2023**: Requires switching from plain NodeConfig YAML to MIME multipart format, combining the NodeConfig with a shell script part.

Both AMI families can share the same image-pull bash logic. The `ctr` binary and containerd socket are identical across both.

---

## Container Runtime Tooling on EKS AMIs

### Binary Availability Matrix

| Tool | AL2 (EKS-optimized) | AL2023 (EKS-optimized) | Path | Notes |
|------|---------------------|------------------------|------|-------|
| `ctr` | Pre-installed | Pre-installed | `/usr/bin/ctr` | Bundled with containerd; always available |
| `crictl` | Pre-installed (added ~2024) | NOT available | `/usr/bin/crictl` (AL2) | AL2023 distro repos do not include cri-tools (open issue #2163) |
| `nerdctl` | NOT available | NOT available by default | n/a | Was considered but not shipped in AMI; must be manually installed |
| `aws` CLI | Pre-installed (v1 or v2) | Pre-installed (v2) | `/usr/bin/aws` or `/usr/local/bin/aws` | Required for ECR auth |
| `containerd` | Pre-installed | Pre-installed | `/usr/bin/containerd` | Runtime; managed by systemd |

**Confidence:** HIGH -- verified via [amazon-eks-ami GitHub issues #797](https://github.com/awslabs/amazon-eks-ami/issues/797), [#1486](https://github.com/awslabs/amazon-eks-ami/issues/1486), [#2163](https://github.com/awslabs/amazon-eks-ami/issues/2163)

### Recommendation: Use `ctr` exclusively

**Use `ctr -n k8s.io images pull` for image pre-pulling.** Rationale:

1. `ctr` is the ONLY tool guaranteed to be present on BOTH AL2 and AL2023 EKS AMIs.
2. `crictl` is absent on AL2023 (the primary target going forward, since AL2 AMIs stopped publishing Nov 2025).
3. `ctr` operates directly on containerd, bypassing CRI -- this is fine for pre-pull because we do not need pod-level semantics.
4. The `k8s.io` namespace flag ensures kubelet sees the pre-pulled images.
5. This is the pattern used in production by Karpenter users, Terraform EKS modules, and the AWS prefetching blog post.

**Why NOT `crictl`:** Not available on AL2023 without manual installation. Adding a `crictl` installation step to user data is unnecessary complexity when `ctr` already works.

**Why NOT `nerdctl`:** Not pre-installed on either AMI family. Same issue as `crictl`.

---

## Containerd Configuration

### Socket and Config Paths (Both AL2 and AL2023)

| Item | Path | Notes |
|------|------|-------|
| Containerd socket | `/run/containerd/containerd.sock` | Consistent across all EKS AMI families |
| Containerd config | `/etc/containerd/config.toml` | Main config file |
| Config overrides | `/etc/containerd/config.d/` | Drop-in config directory |
| Containerd data | `/var/lib/containerd/` | Image and container storage |

**Confidence:** HIGH -- verified via [EKS AMI docs](https://awslabs.github.io/amazon-eks-ami/usage/al2/), [containerd issue #1917](https://github.com/awslabs/amazon-eks-ami/issues/1917)

### Containerd Namespace: `k8s.io`

Containerd uses namespaces for isolation. Kubelet uses the `k8s.io` namespace exclusively. Images pulled into any other namespace are invisible to kubelet.

**Every `ctr` command MUST include `-n k8s.io`.**

```bash
# CORRECT -- kubelet will see this image
ctr -n k8s.io images pull <image>

# WRONG -- kubelet will NOT see this image (defaults to "default" namespace)
ctr images pull <image>
```

**Confidence:** HIGH -- this is universally documented in [AWS containerd blog](https://aws.amazon.com/blogs/containers/all-you-need-to-know-about-moving-to-containerd-on-amazon-eks/), Kubernetes docs, and [containerd discussions](https://github.com/containerd/containerd/discussions/7902)

---

## ECR Authentication During User Data

### How It Works

ECR uses short-lived tokens (12-hour TTL) obtained via the `GetAuthorizationToken` API. During user data execution, the instance profile's IAM role provides credentials through IMDS (Instance Metadata Service). The AWS CLI automatically uses these credentials.

**Authentication command chain:**

```bash
# 1. Get ECR password (uses instance profile via IMDS automatically)
ECR_PASSWORD=$(aws ecr get-login-password --region <region>)

# 2. Pass to ctr as username:password
ctr -n k8s.io images pull -u "AWS:${ECR_PASSWORD}" <account-id>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>
```

**Confidence:** HIGH -- verified via [AWS re:Post](https://repost.aws/questions/QUo12Z0uqGRA2cp6K4UNyNnw/how-to-pull-ecr-image-using-ctr-tool-where-runtime-is-containerd), [ECR auth docs](https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html), community production patterns

### IMDS Credential Timing Issue

**Known problem:** `aws ecr get-login-password` can fail with `AccessDeniedException` during early user data execution because IMDS credentials are not yet available.

**Root cause:** The instance profile IAM role credentials are served via IMDS, and there is a brief window after instance launch where IMDS is not yet ready to serve credentials.

**Recommended mitigation:** Poll for IMDS availability before attempting ECR auth.

```bash
# Wait for IMDS credentials to become available
wait_for_imds() {
    local max_attempts=30
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if aws sts get-caller-identity >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    return 1
}
```

**Confidence:** HIGH -- documented in [AWS re:Post](https://repost.aws/questions/QUOT8lTHaITkqW0NGB74Pkeg/aws-ecr-get-login-password-not-working-when-executed-from-userdata-script-on-ec2-start-up) with known workaround

### Public ECR Images (No Auth Required)

For public ECR images (`public.ecr.aws/...`), no authentication is needed:

```bash
# Public ECR -- no credentials required
ctr -n k8s.io images pull public.ecr.aws/eks-distro/kubernetes/pause:3.9
```

### Required IAM Permissions

The instance profile must have these ECR permissions (included in `AmazonEC2ContainerRegistryReadOnly` managed policy, which EKS worker nodes have by default):

- `ecr:GetAuthorizationToken`
- `ecr:BatchCheckLayerAvailability`
- `ecr:BatchGetImage`
- `ecr:GetDownloadUrlForLayer`

**No additional IAM changes needed** -- standard EKS worker node IAM roles already include these permissions.

---

## Exact `ctr` Pull Command

### Full Command Syntax

```bash
# For private ECR images:
ctr -n k8s.io images pull \
    -u "AWS:${ECR_PASSWORD}" \
    "${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com/${REPO}:${TAG}"

# For public ECR images:
ctr -n k8s.io images pull \
    "public.ecr.aws/${ALIAS}/${IMAGE}:${TAG}"

# For non-ECR registries (Docker Hub, GHCR, etc.) -- no auth for public:
ctr -n k8s.io images pull \
    "docker.io/library/nginx:latest"
```

### Key `ctr` Flags

| Flag | Purpose | Required? |
|------|---------|-----------|
| `-n k8s.io` | Target the kubelet namespace | YES -- always |
| `-u "USER:PASS"` | Authentication credentials | YES for private ECR; NO for public |
| `--plain-http` | Use HTTP instead of HTTPS | NO -- never use; EKS always uses HTTPS |

### Exit Codes and Error Handling

`ctr` exit codes:

| Exit Code | Meaning | Action |
|-----------|---------|--------|
| 0 | Success | Continue |
| 1 | General failure (network, auth, not found) | Retry or skip based on policy |

**Error detection patterns in bash:**

```bash
# Pattern 1: BestEffort -- log and continue on failure
pull_image_best_effort() {
    local image="$1"
    if ! ctr -n k8s.io images pull -u "AWS:${ECR_PASSWORD}" "${image}" 2>&1; then
        log "WARNING: Failed to pull ${image}, continuing (best-effort mode)"
        return 0
    fi
    log "Successfully pulled ${image}"
}

# Pattern 2: Required -- fail the warmup if any image fails
pull_image_required() {
    local image="$1"
    local max_retries=3
    local attempt=0
    while [ $attempt -lt $max_retries ]; do
        if ctr -n k8s.io images pull -u "AWS:${ECR_PASSWORD}" "${image}" 2>&1; then
            log "Successfully pulled ${image}"
            return 0
        fi
        attempt=$((attempt + 1))
        log "Retry ${attempt}/${max_retries} for ${image}"
        sleep $((attempt * 5))  # Exponential-ish backoff: 5s, 10s, 15s
    done
    log "ERROR: Failed to pull ${image} after ${max_retries} attempts"
    return 1
}
```

---

## Timing: Containerd Availability During User Data

### AL2 Boot Sequence

```
cloud-init starts
  -> /etc/eks/bootstrap.sh runs (starts containerd, configures kubelet)
  -> MIME part scripts execute (in order of MIME parts)
  -> kubelet starts
  -> node joins cluster
```

On AL2, the bootstrap script (`/etc/eks/bootstrap.sh`) runs first as MIME part 1. It starts containerd and configures kubelet. Subsequent MIME parts execute after bootstrap.sh completes. **By the time the warmup script (MIME part 2+) runs, containerd is already running and the socket is available.**

**Implication for Stratos:** The image pre-pull script should be a MIME part that runs AFTER the bootstrap script but BEFORE or ALONGSIDE the warmup script. Since containerd is started by bootstrap.sh, the socket will be available.

**Current AL2 MIME part order in Stratos:**
1. `bootstrap.sh` -- calls `/etc/eks/bootstrap.sh`
2. `stratos-warmup.sh` -- waits for kubelet health
3. (optional) `custom-userdata.sh`

**New order with image pre-pull:**
1. `bootstrap.sh` -- calls `/etc/eks/bootstrap.sh` (starts containerd)
2. `stratos-image-pull.sh` -- pulls images via `ctr` (containerd is now running)
3. `stratos-warmup.sh` -- waits for kubelet health
4. (optional) `custom-userdata.sh`

**Confidence:** HIGH -- bootstrap.sh explicitly starts containerd, verified in [EKS AMI AL2 docs](https://awslabs.github.io/amazon-eks-ami/usage/al2/)

### AL2023 Boot Sequence

```
nodeadm-config.service  (reads user data from IMDS, writes config)
  -> cloud-init.service  (runs user data shell scripts)
  -> containerd.service   (starts containerd runtime)
  -> nodeadm-run.service  (runs nodeadm init, configures kubelet, starts kubelet)
```

**Critical difference from AL2:** On AL2023, user data shell scripts (in `text/x-shellscript` MIME parts) are executed by cloud-init BEFORE `nodeadm-run.service` starts. However, containerd may or may not be running during cloud-init execution. The systemd ordering is:

- `nodeadm-config.service` runs first (reads IMDS)
- `cloud-init.service` runs (executes user data scripts) -- **this is where our script runs**
- `containerd.service` may start in parallel or after cloud-init
- `nodeadm-run.service` waits for containerd AND cloud-final

**Implication for Stratos:** The image pre-pull script on AL2023 MUST wait for containerd to be ready before pulling images. It cannot assume containerd is running.

```bash
# Wait for containerd socket to be available
wait_for_containerd() {
    local max_wait=120
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if ctr -n k8s.io version >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done
    return 1
}
```

**Important:** The shell script in MIME multipart runs BEFORE nodeadm-run, which means it blocks node joining. The AWS community has noted this as a design issue ([Issue #2224](https://github.com/awslabs/amazon-eks-ami/issues/2224)). For Stratos warmup, this is actually **acceptable** because the node is going to be stopped after warmup anyway -- we WANT images pulled before the node is stopped. Blocking nodeadm-run briefly is fine for our use case.

**Confidence:** HIGH -- verified via [issue #1751](https://github.com/awslabs/amazon-eks-ami/issues/1751), [issue #2123](https://github.com/awslabs/amazon-eks-ami/issues/2123), [issue #1917](https://github.com/awslabs/amazon-eks-ami/issues/1917)

---

## AL2023 User Data Format Change

### Current AL2023 Format (Plain NodeConfig YAML)

The current `AL2023Generator` outputs plain NodeConfig YAML:

```yaml
apiVersion: node.eks.aws/v1alpha1
kind: NodeConfig
spec:
  cluster:
    name: my-cluster
    apiServerEndpoint: https://...
    certificateAuthority: Y2VydGlm...
    cidr: 10.100.0.0/16
  kubelet:
    flags:
      - "--node-labels=stratos.sh/pool=my-pool"
```

### Required AL2023 Format (MIME Multipart with Script)

To add a shell script alongside the NodeConfig, AL2023 must use MIME multipart:

```
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="BOUNDARY"

--BOUNDARY
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash
# Image pre-pull script runs here
# (Executes during cloud-init, before nodeadm-run)

--BOUNDARY
Content-Type: application/node.eks.aws

apiVersion: node.eks.aws/v1alpha1
kind: NodeConfig
spec:
  cluster:
    name: my-cluster
    ...

--BOUNDARY--
```

**Content type for NodeConfig:** `application/node.eks.aws` (NOT `text/x-shellscript`, NOT `text/yaml`)

**Execution order:** Shell script parts execute first during cloud-init. The `application/node.eks.aws` part is read by `nodeadm-config.service` (separate from cloud-init script execution).

**Confidence:** HIGH -- verified via [AWS EKS launch templates docs](https://docs.aws.amazon.com/eks/latest/userguide/launch-templates.html)

### Impact on AL2023Generator

The `AL2023Generator.Generate()` method currently returns plain YAML. When images are specified, it must return MIME multipart instead. When no images are specified, it can continue returning plain YAML (backward compatible).

**Approach:**
- If `config.WarmupImages` is empty: return plain NodeConfig YAML (no change)
- If `config.WarmupImages` is non-empty: return MIME multipart with shell script + NodeConfig

The existing `buildMIMEMultipart()` and `mimePartShellScript()` helpers in `al2023.go` can be reused. A new helper is needed for the `application/node.eks.aws` content type:

```go
func mimePartNodeConfig(content string) string {
    return fmt.Sprintf("Content-Type: application/node.eks.aws\n\n%s", content)
}
```

---

## AL2 MIME Multipart Ordering

### Current AL2 MIME Structure

```
MIME multipart:
  Part 1: bootstrap.sh           (text/x-shellscript)
  Part 2: stratos-warmup.sh      (text/x-shellscript)
  Part 3: custom-userdata.sh     (text/x-shellscript, optional)
```

### New AL2 MIME Structure (With Image Pre-Pull)

```
MIME multipart:
  Part 1: bootstrap.sh           (text/x-shellscript)  -- starts containerd
  Part 2: stratos-image-pull.sh  (text/x-shellscript)  -- pulls images via ctr
  Part 3: stratos-warmup.sh      (text/x-shellscript)  -- waits for kubelet
  Part 4: custom-userdata.sh     (text/x-shellscript, optional)
```

**Why Part 2 (after bootstrap, before warmup):**
- bootstrap.sh starts containerd, so the socket is available for Part 2
- Image pulling should complete before the warmup script signals readiness
- The warmup script waits for kubelet health, which is independent of image pulls
- Custom user data runs last (unchanged behavior)

**Impact on AL2Generator:** Insert the image-pull script as a new MIME part between bootstrap and warmup. The existing code structure (`var parts []string` with `append`) makes this straightforward.

---

## Shared Image Pull Script Template

Both AL2 and AL2023 can use the same core image-pull bash logic. The only difference is containerd readiness handling:

- **AL2:** Containerd is guaranteed running (bootstrap.sh started it)
- **AL2023:** Must wait for containerd socket

### Recommended Script Structure

```bash
#!/bin/bash
# Stratos image pre-pull script
set -euo pipefail

CONTAINERD_SOCK="/run/containerd/containerd.sock"
MAX_CONTAINERD_WAIT=120
PULL_TIMEOUT=300

log() {
    echo "[stratos-image-pull] [$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

# Wait for containerd (needed on AL2023; fast no-op on AL2 where it is already running)
wait_for_containerd() {
    local elapsed=0
    while [ $elapsed -lt $MAX_CONTAINERD_WAIT ]; do
        if ctr -n k8s.io version >/dev/null 2>&1; then
            log "containerd is ready"
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
        log "Waiting for containerd... (${elapsed}/${MAX_CONTAINERD_WAIT}s)"
    done
    log "ERROR: containerd did not become ready within ${MAX_CONTAINERD_WAIT}s"
    return 1
}

# Authenticate to ECR (only needed for private ECR images)
get_ecr_password() {
    local region="$1"
    local max_attempts=10
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        local password
        if password=$(aws ecr get-login-password --region "${region}" 2>/dev/null); then
            echo "${password}"
            return 0
        fi
        attempt=$((attempt + 1))
        log "Waiting for ECR auth (IMDS may not be ready)... attempt ${attempt}/${max_attempts}"
        sleep 3
    done
    log "ERROR: Failed to get ECR password after ${max_attempts} attempts"
    return 1
}

# Pull a single image
pull_image() {
    local image="$1"
    local creds_flag="$2"  # empty for public, "-u AWS:xxx" for private ECR
    local policy="$3"      # "Required" or "BestEffort"
    local max_retries=3
    local attempt=0

    while [ $attempt -lt $max_retries ]; do
        if ctr -n k8s.io images pull ${creds_flag} "${image}" >/dev/null 2>&1; then
            log "Pulled: ${image}"
            return 0
        fi
        attempt=$((attempt + 1))
        log "Retry ${attempt}/${max_retries} for ${image}"
        sleep $((attempt * 5))
    done

    if [ "${policy}" = "Required" ]; then
        log "ERROR: Failed to pull required image: ${image}"
        return 1
    else
        log "WARNING: Failed to pull image (best-effort): ${image}"
        return 0
    fi
}

# Main
wait_for_containerd || { log "Skipping image pulls (containerd not ready)"; exit 0; }

# ECR auth (templated by Go -- only present if ECR images are in the list)
# {{ECR_AUTH_BLOCK}}

# Pull images (templated by Go)
# {{IMAGE_PULL_BLOCK}}

log "Image pre-pull complete"
```

### Go Template Strategy

The Go code generates the script by:
1. Building the common shell functions (above)
2. Detecting which images are private ECR (by hostname pattern `*.dkr.ecr.*.amazonaws.com`)
3. Generating the ECR auth block only if needed
4. Generating `pull_image` calls for each image with appropriate auth

---

## Detecting ECR Images in Go

To determine whether an image needs ECR authentication:

```go
import "regexp"

var ecrPrivatePattern = regexp.MustCompile(`^\d+\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com/`)
var ecrPublicPattern = regexp.MustCompile(`^public\.ecr\.aws/`)

func isPrivateECR(image string) bool {
    return ecrPrivatePattern.MatchString(image)
}

func isPublicECR(image string) bool {
    return ecrPublicPattern.MatchString(image)
}
```

- Private ECR images need `aws ecr get-login-password` + `-u` flag
- Public ECR images do NOT need authentication
- Non-ECR images (Docker Hub, GHCR, etc.) do NOT need authentication for public images

### ECR Region Detection

For private ECR images, the region is embedded in the hostname: `<account>.dkr.ecr.<region>.amazonaws.com`. The Go code should parse this to pass the correct `--region` flag to `aws ecr get-login-password`.

```go
func extractECRRegion(image string) string {
    // Pattern: XXXX.dkr.ecr.REGION.amazonaws.com/...
    parts := strings.Split(image, ".")
    // parts[0]=account, [1]=dkr, [2]=ecr, [3]=REGION, [4]=amazonaws, [5]=com/repo
    if len(parts) >= 6 && parts[1] == "dkr" && parts[2] == "ecr" {
        return parts[3]
    }
    return ""
}
```

---

## No New Go Dependencies

| Existing Tool | Used For | Already in go.mod? |
|---------------|----------|-------------------|
| `fmt`, `strings`, `regexp` | Script template generation, ECR detection | YES (stdlib) |
| controller-runtime | Reconciliation, logging | YES |
| AWS SDK v2 | Provider operations | YES |

**No new Go modules need to be added.** The feature is implemented entirely through bash script generation in the existing user data framework.

---

## Integration Points with Existing Code

### Files That Need Changes

| File | Change Type | Description |
|------|------------|-------------|
| `internal/cloudprovider/aws/userdata.go` | Modify | Add `WarmupImages []string` and `ImagePullPolicy string` to `BootstrapConfig` |
| `internal/cloudprovider/aws/al2.go` | Modify | Insert image-pull MIME part between bootstrap and warmup parts |
| `internal/cloudprovider/aws/al2023.go` | Modify | Switch to MIME multipart when images are specified; add `application/node.eks.aws` MIME helper |
| `internal/cloudprovider/aws/warmup.go` | New content | Add image pull script generation function |
| `api/v1alpha1/nodepool_types.go` | Modify | Add `spec.warmup.images` and `spec.warmup.imagePullPolicy` to CRD |

### Files That Do NOT Need Changes

| File | Why Not |
|------|---------|
| `internal/cloudprovider/aws/bottlerocket.go` | Bottlerocket is out of scope for this milestone |
| `internal/cloudprovider/interface.go` | The CloudProvider interface does not change -- image pull is a user data concern |
| `internal/cloudprovider/aws/provider.go` | Launch config assembly already passes BootstrapConfig; adding fields is sufficient |
| `internal/controller/nodepool_controller.go` | No reconciliation logic changes -- images are pulled during instance launch via user data |

---

## AL2 End of Life Considerations

Amazon EKS stopped publishing AL2 AMIs on November 26, 2025. EKS 1.32 is the last version with AL2 support. From EKS 1.33 onward, only AL2023 and Bottlerocket AMIs are published.

**Implication:** AL2023 is the primary target. AL2 support should be maintained for existing clusters on EKS <= 1.32 but should not drive architectural decisions. The AL2023 MIME multipart format is the more important code path.

---

## Sources

### HIGH Confidence (Official Documentation)
- [AWS EKS Launch Templates docs](https://docs.aws.amazon.com/eks/latest/userguide/launch-templates.html) -- AL2023 MIME multipart format with `application/node.eks.aws` content type
- [AWS ECR Authentication docs](https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html) -- `get-login-password` token mechanism
- [EKS AMI AL2 Usage docs](https://awslabs.github.io/amazon-eks-ami/usage/al2/) -- containerd config paths, bootstrap.sh behavior
- [EKS AMI nodeadm docs](https://awslabs.github.io/amazon-eks-ami/nodeadm/) -- nodeadm configuration format
- [Kubernetes crictl docs](https://github.com/kubernetes-sigs/cri-tools/blob/master/docs/crictl.md) -- crictl config, runtime endpoint
- [ECR Credential Provider](https://deepwiki.com/awslabs/amazon-eks-ami/3.3-container-registry-integration) -- ecr-credential-provider binary path, kubelet integration

### HIGH Confidence (GitHub Issues with Verified Behavior)
- [amazon-eks-ami #797](https://github.com/awslabs/amazon-eks-ami/issues/797) -- crictl not shipped, nerdctl added instead (AL2)
- [amazon-eks-ami #1486](https://github.com/awslabs/amazon-eks-ami/issues/1486) -- crictl was added to AL2 AMIs, confirmed by maintainer
- [amazon-eks-ami #2163](https://github.com/awslabs/amazon-eks-ami/issues/2163) -- crictl NOT available on AL2023 (open issue, Dec 2025)
- [amazon-eks-ami #2123](https://github.com/awslabs/amazon-eks-ami/issues/2123) -- AL2023 pre/post nodeadm script execution bugs
- [amazon-eks-ami #2224](https://github.com/awslabs/amazon-eks-ami/issues/2224) -- post-nodeadm user data not natively supported; use systemd
- [amazon-eks-ami #1917](https://github.com/awslabs/amazon-eks-ami/issues/1917) -- nodeadm-run fails if containerd not ready
- [amazon-eks-ami #1751](https://github.com/awslabs/amazon-eks-ami/issues/1751) -- AL2023 boot sequence timing: nodeadm-config -> cloud-init -> containerd -> nodeadm-run

### MEDIUM Confidence (Community Production Patterns)
- [EKS image pre-pull from ECR (Medium)](https://medium.com/@andriikrymus/eks-image-pre-pull-from-ecr-2ec56d33ec82) -- `ctr -n k8s.io images pull -u "AWS:$PASSWORD"` pattern
- [AWS re:Post: Pull ECR image using ctr](https://repost.aws/questions/QUo12Z0uqGRA2cp6K4UNyNnw/how-to-pull-ecr-image-using-ctr-tool-where-runtime-is-containerd) -- ctr ECR auth syntax
- [AWS re:Post: ECR get-login-password fails in user data](https://repost.aws/questions/QUOT8lTHaITkqW0NGB74Pkeg/aws-ecr-get-login-password-not-working-when-executed-from-userdata-script-on-ec2-start-up) -- IMDS timing issue during early boot
- [AWS Blog: Start Pods faster by prefetching images](https://aws.amazon.com/blogs/containers/start-pods-faster-by-prefetching-images/) -- SSM-based prefetching approach
- [containerd namespace discussion](https://github.com/containerd/containerd/discussions/7902) -- k8s.io namespace requirement
