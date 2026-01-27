## Why

Stratos has no production deployment story. There's no CI pipeline to build and publish container images, no Helm chart for installation, and the project layout uses kubebuilder's kustomize scaffolding which is uncommon in production Kubernetes operator deployments. Users can't `helm install` Stratos or pull a versioned image from a registry.

## What Changes

- **BREAKING**: Remove `config/` directory entirely (kustomize manifests, manager deployment, RBAC templates)
- Create a Helm chart at `deploy/charts/stratos/` with configurable values for image, cluster name, resources, IRSA annotations, etc.
- Move CRDs to `deploy/charts/stratos/crds/`
- Move sample manifests to `deploy/samples/`
- Update Dockerfile for multi-arch builds (amd64 + arm64) using Docker buildx `TARGETARCH`/`TARGETOS` args
- Add GitHub Actions workflow (`.github/workflows/release.yml`) triggered on `v*` tags to build and push images to `ghcr.io/stratos-sh/stratos`
- Update Makefile targets to reference new paths and remove kustomize dependencies
- Update CLAUDE.md to reflect new directory structure and commands

## Capabilities

### New Capabilities

- `helm-chart`: Helm chart for deploying the Stratos controller, including deployment, RBAC, service account, and CRDs
- `container-image-ci`: GitHub Actions workflow for building multi-arch container images and pushing versioned tags to GHCR

### Modified Capabilities

_None — this change is about packaging and deployment, not controller behavior._

## Impact

- **Directory structure**: `config/` removed, `deploy/` created
- **Dockerfile**: Modified for multi-arch support
- **Makefile**: Updated targets (`install`, `deploy`, `docker-build`, `docker-push`)
- **CI**: New workflow file at `.github/workflows/release.yml`
- **Documentation**: CLAUDE.md paths and commands updated
- **Users**: Must use `helm install` instead of `kustomize build | kubectl apply`
