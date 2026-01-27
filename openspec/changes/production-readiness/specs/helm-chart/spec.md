## ADDED Requirements

### Requirement: Helm chart structure
The Helm chart SHALL be located at `deploy/charts/stratos/` and follow standard Helm 3 conventions with `Chart.yaml`, `values.yaml`, `templates/`, and `crds/` directories.

#### Scenario: Chart directory layout
- **WHEN** a user inspects the chart directory
- **THEN** it SHALL contain Chart.yaml, values.yaml, templates/, and crds/ subdirectories

#### Scenario: Chart.yaml metadata
- **WHEN** the chart is packaged or installed
- **THEN** Chart.yaml SHALL specify `apiVersion: v2`, `type: application`, `name: stratos`, and both `version` and `appVersion` fields matching the release version

### Requirement: CRD installation
CRDs SHALL be placed in the `crds/` directory so Helm installs them on first install. The chart SHALL include both `stratos.sh_nodepools.yaml` and `stratos.sh_awsnodeclasses.yaml`.

#### Scenario: First install includes CRDs
- **WHEN** a user runs `helm install` for the first time
- **THEN** both NodePool and AWSNodeClass CRDs SHALL be created in the cluster

#### Scenario: CRDs not upgraded on helm upgrade
- **WHEN** a user runs `helm upgrade`
- **THEN** CRDs in `crds/` SHALL NOT be modified (standard Helm behavior)

### Requirement: Controller deployment template
The chart SHALL template a Deployment resource for the stratos controller with configurable image, replicas, resources, and security context matching the current `config/manager/manager.yaml` defaults.

#### Scenario: Default deployment
- **WHEN** installed with default values
- **THEN** the Deployment SHALL run 1 replica of `ghcr.io/stratos-sh/stratos` with the appVersion tag, using the security context from the existing manager.yaml (runAsNonRoot, readOnlyRootFilesystem, drop ALL capabilities, seccompProfile RuntimeDefault)

#### Scenario: Custom image override
- **WHEN** a user sets `image.repository` and/or `image.tag`
- **THEN** the Deployment SHALL use the specified image coordinates

### Requirement: Cluster name configuration
The chart SHALL require `clusterName` to be set, since the controller cannot operate without it. The controller SHALL receive the cluster name via the `CLUSTER_NAME` environment variable.

#### Scenario: Cluster name provided
- **WHEN** a user sets `clusterName: my-cluster`
- **THEN** the Deployment SHALL set `CLUSTER_NAME=my-cluster` as an environment variable

#### Scenario: Cluster name missing
- **WHEN** a user installs without setting `clusterName`
- **THEN** the chart SHALL fail with a validation error indicating clusterName is required

### Requirement: RBAC templates
The chart SHALL template ServiceAccount, ClusterRole, ClusterRoleBinding, and RoleBinding resources. The ClusterRole permissions SHALL match those generated in the current `config/rbac/role.yaml`.

#### Scenario: ServiceAccount creation
- **WHEN** `serviceAccount.create` is true (default)
- **THEN** a ServiceAccount SHALL be created with configurable annotations (for IRSA)

#### Scenario: ServiceAccount annotations for IRSA
- **WHEN** a user sets `serviceAccount.annotations` with an `eks.amazonaws.com/role-arn` key
- **THEN** the ServiceAccount SHALL include that annotation, enabling IAM Roles for Service Accounts

#### Scenario: Existing ServiceAccount
- **WHEN** `serviceAccount.create` is false and `serviceAccount.name` is set
- **THEN** the Deployment SHALL reference the specified existing ServiceAccount

### Requirement: Configurable controller arguments
The chart SHALL expose controller arguments as values: `leaderElect`, `cloudProvider`, `syncPeriod`, `metricsBindAddress`, and `healthProbeBindAddress`.

#### Scenario: Default arguments
- **WHEN** installed with default values
- **THEN** the controller SHALL run with `--leader-elect`, `--metrics-bind-address=:8080`, `--health-probe-bind-address=:8081`

#### Scenario: Custom cloud provider
- **WHEN** a user sets `cloudProvider: fake`
- **THEN** the controller SHALL include `--cloud-provider=fake` in its args

### Requirement: Resource configuration
The chart SHALL allow configuring CPU and memory requests/limits via `resources` in values.yaml.

#### Scenario: Default resources
- **WHEN** installed with default values
- **THEN** the Deployment SHALL use cpu 100m/500m request/limit and memory 128Mi/256Mi request/limit

#### Scenario: Custom resources
- **WHEN** a user overrides `resources.requests.memory: 512Mi`
- **THEN** the Deployment SHALL reflect the override

### Requirement: Health probes
The chart SHALL configure liveness and readiness probes matching the current manager.yaml defaults.

#### Scenario: Default probes
- **WHEN** installed with default values
- **THEN** the Deployment SHALL have a liveness probe on `/healthz:8081` (15s initial delay, 20s period) and readiness probe on `/readyz:8081` (5s initial delay, 10s period)

### Requirement: Namespace creation
The chart SHALL create the target namespace if `namespace.create` is true (default).

#### Scenario: Default namespace
- **WHEN** installed with default values
- **THEN** resources SHALL be deployed to `stratos-system` namespace

### Requirement: Helm helpers
The chart SHALL include a `_helpers.tpl` with standard helper templates for fullname, labels (app.kubernetes.io/name, app.kubernetes.io/instance, app.kubernetes.io/version, app.kubernetes.io/component, app.kubernetes.io/managed-by), and selector labels.

#### Scenario: Consistent labeling
- **WHEN** any chart resource is rendered
- **THEN** it SHALL include standard Kubernetes recommended labels generated by the helpers

### Requirement: NOTES.txt
The chart SHALL include a `NOTES.txt` that displays post-install instructions including how to check the controller status and a reminder about CRD upgrades.

#### Scenario: Post-install output
- **WHEN** a user installs the chart
- **THEN** Helm SHALL display instructions for verifying the deployment and a note about manual CRD upgrades
