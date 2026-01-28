## Context

Stratos currently uses kubebuilder's default kustomize layout under `config/`. There is no Helm chart, no CI for container images, and the Dockerfile only supports amd64. The project has one git tag (`v0.0.1`) and images have never been published to a registry. The repository has moved to `github.com/stratos-sh/stratos`.

## Goals / Non-Goals

**Goals:**
- Users can install Stratos with `helm install` and configure it via values
- Tagged releases produce versioned multi-arch images at `ghcr.io/stratos-sh/stratos`
- Tagged releases publish the Helm chart to `oci://ghcr.io/stratos-sh/charts/stratos`
- Clean directory layout that reflects Helm as the deployment method
- CRDs, RBAC, deployment all managed through the chart

**Non-Goals:**
- Automated chart version bumping outside of CI (Chart.yaml tracks dev version, CI stamps the release version)
- Helm chart tests (`helm test`)
- ServiceMonitor/PodMonitor for Prometheus Operator (future)
- Horizontal Pod Autoscaler for the controller

## Decisions

### Directory structure: `deploy/` over `charts/` at root

Using `deploy/charts/stratos/` follows the Go project layout convention. Samples move to `deploy/samples/`. The entire `config/` directory is removed.

**Alternatives considered:**
- `charts/` at root — common for chart-only repos, but this is a Go project with a chart, not a chart repo
- `helm/` — non-standard, less recognizable

### CRDs in `crds/` directory (not `templates/`)

Placing CRDs in `crds/` means Helm installs them on first install but won't upgrade them. Users must run `kubectl apply -f deploy/charts/stratos/crds/` before `helm upgrade` when CRDs change.

**Alternatives considered:**
- CRDs in `templates/` with conditional — always applied on upgrade, but risks accidental deletion if chart is uninstalled. cert-manager and Karpenter use this, but it adds complexity (install-crd flag, hooks)
- Separate CRD chart — over-engineered for this stage

The `crds/` approach is simpler and appropriate for the project's maturity. Can revisit when CRD churn increases.

### Chart version equals app version

Both `version` and `appVersion` in Chart.yaml track the git tag. When tagging `v0.3.0`, both become `0.3.0` / `v0.3.0`. This avoids maintaining two version numbers for a single-artifact project.

### Namespace handling

The chart creates the namespace by default (`namespace.create: true`). The release namespace (`{{ .Release.Namespace }}`) is used for all namespaced resources. Defaults to `stratos-system`.

### Cluster name as required value

`clusterName` is validated in `_helpers.tpl` with a `required` function call. The controller cannot operate without it, so failing at install time is better than a crashlooping pod.

### Dockerfile multi-arch via buildx args

Replace hardcoded `GOARCH=amd64` with `ARG TARGETOS TARGETARCH` which Docker buildx sets automatically per platform. No changes to the build stage base image or runtime image needed — both `golang:1.22-alpine` and `gcr.io/distroless/static:nonroot` support multi-arch.

### GitHub Actions with docker/build-push-action

Using the standard `docker/setup-qemu-action`, `docker/setup-buildx-action`, `docker/login-action`, and `docker/build-push-action` pattern. This is the most common CI setup for multi-arch container builds on GitHub.

### Helm chart publishing via OCI to GHCR

The release workflow packages the chart and pushes it to `oci://ghcr.io/stratos-sh/charts`. Before packaging, Chart.yaml `version` and `appVersion` are stamped from the git tag (stripping the `v` prefix for `version`). This happens in CI only — the repo's Chart.yaml keeps a dev version.

**Alternatives considered:**
- GitHub Pages chart repo — requires a separate `gh-pages` branch and index.yaml management
- Separate chart release workflow — unnecessary complexity, chart and image should release together

Users install with:
```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos --version 0.2.0 --set clusterName=my-cluster
```

### Makefile updates

- `make docker-build` updated to use buildx for local builds (single platform)
- `make docker-push` updated with new image path
- `make install` becomes `kubectl apply` of CRDs from chart
- `make deploy` becomes `helm install/upgrade` invocation
- Kustomize tool download targets removed

## Risks / Trade-offs

- **CRD upgrade gap**: Users must manually apply CRDs before `helm upgrade`. Mitigated by clear NOTES.txt and documentation.
- **Single-platform local builds**: `make docker-build` builds for the host platform only (not multi-arch) for speed. Multi-arch only happens in CI.
- **GITHUB_TOKEN permissions**: GHCR push requires the repository's Actions settings to allow write access to packages. This is a one-time setup step.

## Migration Plan

1. Create `deploy/` directory structure with Helm chart
2. Move CRDs from `config/crd/bases/` to `deploy/charts/stratos/crds/`
3. Move samples from `config/samples/` to `deploy/samples/`
4. Delete `config/` entirely
5. Update Dockerfile for multi-arch
6. Add `.github/workflows/release.yml`
7. Update Makefile targets
8. Update CLAUDE.md
