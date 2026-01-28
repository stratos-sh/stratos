## Requirements

### Requirement: Multi-arch Dockerfile
The Dockerfile SHALL support building for multiple architectures (linux/amd64 and linux/arm64) using Docker buildx platform arguments `TARGETOS` and `TARGETARCH`.

#### Scenario: Build for amd64
- **WHEN** building with `--platform linux/amd64`
- **THEN** the binary SHALL be compiled with `GOOS=linux GOARCH=amd64`

#### Scenario: Build for arm64
- **WHEN** building with `--platform linux/arm64`
- **THEN** the binary SHALL be compiled with `GOOS=linux GOARCH=arm64`

#### Scenario: Version injection
- **WHEN** the image is built in CI
- **THEN** the binary SHALL have version, commit, and build date injected via ldflags

### Requirement: Release workflow trigger
A GitHub Actions workflow at `.github/workflows/release.yml` SHALL trigger on pushes of tags matching `v*` (e.g., `v0.2.0`).

#### Scenario: Tag push triggers build
- **WHEN** a developer pushes tag `v0.2.0`
- **THEN** the release workflow SHALL execute

#### Scenario: Non-tag push does not trigger
- **WHEN** a developer pushes to the `main` branch without a tag
- **THEN** the release workflow SHALL NOT execute

### Requirement: Image tagging
The workflow SHALL push images to `ghcr.io/stratos-sh/stratos` with both the version tag and `latest`.

#### Scenario: Version tag
- **WHEN** the tag `v0.2.0` triggers the workflow
- **THEN** the image SHALL be pushed as `ghcr.io/stratos-sh/stratos:v0.2.0`

#### Scenario: Latest tag
- **WHEN** any version tag triggers the workflow
- **THEN** the image SHALL also be pushed as `ghcr.io/stratos-sh/stratos:latest`

### Requirement: Multi-arch image manifest
The workflow SHALL build and push a multi-platform manifest covering `linux/amd64` and `linux/arm64`.

#### Scenario: Multi-platform manifest
- **WHEN** the image is pushed
- **THEN** the manifest SHALL include entries for both `linux/amd64` and `linux/arm64` platforms

### Requirement: GHCR authentication
The workflow SHALL authenticate to GitHub Container Registry using the built-in `GITHUB_TOKEN`.

#### Scenario: Authentication
- **WHEN** the workflow runs
- **THEN** it SHALL log in to `ghcr.io` using `github.actor` and `secrets.GITHUB_TOKEN`

### Requirement: Helm chart OCI publishing
The workflow SHALL package the Helm chart and push it to `oci://ghcr.io/stratos-sh/charts` with the version derived from the git tag. Chart.yaml `version` and `appVersion` SHALL be updated to match the tag before packaging.

#### Scenario: Chart published on release
- **WHEN** the tag `v0.2.0` triggers the workflow
- **THEN** the chart SHALL be pushed as `oci://ghcr.io/stratos-sh/charts/stratos:0.2.0`

#### Scenario: Chart version matches tag
- **WHEN** the chart is packaged
- **THEN** Chart.yaml `version` SHALL be `0.2.0` (without `v` prefix) and `appVersion` SHALL be `v0.2.0`

#### Scenario: Users install from OCI
- **WHEN** a user runs `helm install stratos oci://ghcr.io/stratos-sh/charts/stratos --version 0.2.0`
- **THEN** the chart SHALL install successfully

### Requirement: Build reproducibility
The workflow SHALL checkout with `fetch-depth: 0` to ensure `git describe` works correctly for version injection.

#### Scenario: Full git history available
- **WHEN** the workflow checks out the repository
- **THEN** `git describe --tags` SHALL return the correct version string
