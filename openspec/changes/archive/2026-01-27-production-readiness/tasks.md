## 1. Helm Chart Structure

- [x] 1.1 Create `deploy/charts/stratos/` directory with `Chart.yaml` (apiVersion v2, name stratos, version/appVersion from current tag)
- [x] 1.2 Create `values.yaml` with defaults: image (ghcr.io/stratos-sh/stratos), clusterName (""), cloudProvider (aws), resources, serviceAccount config, leaderElect, probe settings
- [x] 1.3 Create `templates/_helpers.tpl` with fullname, labels, selectorLabels helpers and clusterName required validation

## 2. Helm Templates

- [x] 2.1 Create `templates/deployment.yaml` — templatize the existing manager.yaml with values for image, resources, args, env, probes, securityContext
- [x] 2.2 Create `templates/serviceaccount.yaml` with conditional creation and annotations support (IRSA)
- [x] 2.3 Create `templates/clusterrole.yaml` with RBAC rules from existing role.yaml
- [x] 2.4 Create `templates/clusterrolebinding.yaml` and `templates/rolebinding.yaml`
- [x] 2.5 Create `templates/namespace.yaml` with conditional creation
- [x] 2.6 Create `templates/NOTES.txt` with post-install instructions and CRD upgrade reminder

## 3. CRDs and Samples

- [x] 3.1 Copy CRDs from `config/crd/bases/` to `deploy/charts/stratos/crds/`
- [x] 3.2 Move samples from `config/samples/` to `deploy/samples/`

## 4. Dockerfile Multi-arch

- [x] 4.1 Replace hardcoded `GOOS=linux GOARCH=amd64` with `ARG TARGETOS TARGETARCH` and `GOOS=${TARGETOS} GOARCH=${TARGETARCH}`

## 5. CI Release Workflow

- [x] 5.1 Create `.github/workflows/release.yml` — trigger on `v*` tags, checkout with fetch-depth 0, setup QEMU + buildx, login to ghcr.io, build and push linux/amd64,linux/arm64 with version tag + latest

## 6. Cleanup and Updates

- [x] 6.1 Delete `config/` directory entirely
- [x] 6.2 Update Makefile: change `install` target to kubectl apply CRDs, `deploy` to helm install, update `docker-build`/`docker-push` with new image path, remove kustomize tool targets
- [x] 6.3 Update CLAUDE.md with new directory structure, commands, and architecture paths
