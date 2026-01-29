---
sidebar_position: 2
title: Configuration
description: Configure the Stratos controller
---

# Controller Configuration

This guide covers the configuration options for the Stratos controller.

## Helm Values

When deploying with Helm, configuration is passed via `--set` flags or a values file:

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster \
  --set syncPeriod=60s \
  --set cloudProvider=aws
```

Or with a values file:

```yaml title="values.yaml"
clusterName: my-cluster
cloudProvider: aws
syncPeriod: "60s"
leaderElect: true

image:
  repository: ghcr.io/stratos-sh/stratos
  pullPolicy: IfNotPresent

replicaCount: 1

serviceAccount:
  create: true
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/stratos-controller-role

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

extraEnv: []
extraArgs: []
```

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  -f values.yaml
```

## Controller Flags

The Stratos controller accepts the following command-line flags. When using Helm, most of these are configured via Helm values (shown in parentheses):

| Flag | Helm Value | Default | Description |
|------|-----------|---------|-------------|
| `--cluster-name` | `clusterName` | `""` | Kubernetes cluster name. Used for cloud instance tagging. **Required.** |
| `--cloud-provider` | `cloudProvider` | `aws` | Cloud provider to use: `aws` or `fake`. |
| `--sync-period` | `syncPeriod` | `30s` | Minimum interval for reconciliation. |
| `--leader-elect` | `leaderElect` | `true` | Enable leader election for HA. |
| `--metrics-bind-address` | `metricsBindAddress` | `:8080` | Address for the metrics endpoint. |
| `--health-probe-bind-address` | `healthProbeBindAddress` | `:8081` | Address for health probe endpoints. |

Additional flags can be passed via the `extraArgs` Helm value.

### Zap Logger Flags

The controller uses the Zap logger with these additional flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--zap-devel` | `true` | Development mode (human-readable output). |
| `--zap-log-level` | `info` | Log level: `debug`, `info`, `error`. |
| `--zap-encoder` | `console` | Log encoder: `console` or `json`. |
| `--zap-stacktrace-level` | `error` | Level at which to print stack traces. |

To set logger flags via Helm:

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster \
  --set extraArgs[0]=--zap-encoder=json \
  --set extraArgs[1]=--zap-devel=false \
  --set extraArgs[2]=--zap-log-level=info
```

## Production Configuration

For production environments, use JSON logging and IRSA:

```yaml title="values-production.yaml"
clusterName: production
cloudProvider: aws
leaderElect: true

serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/stratos-controller-role

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

extraArgs:
  - --zap-encoder=json
  - --zap-devel=false
  - --zap-log-level=info
```

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  -f values-production.yaml
```

## Health Checks

The controller exposes health endpoints:

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe - is the controller running |
| `/readyz` | 8081 | Readiness probe - is the controller ready to serve |

These are configured automatically by the Helm chart.

## Metrics

Prometheus metrics are exposed at `:8080/metrics`. See [Monitoring](../guides/monitoring.md) for details.

## Environment Variables

All CLI flags can be configured via environment variables using the `STRATOS_` prefix. The controller supports loading environment variables from a `.env` file in the working directory.

**Precedence:** CLI flags > STRATOS_ env vars > legacy env vars > defaults

### Controller Variables

| Environment Variable | Corresponding Flag | Legacy Alias | Default |
|---------------------|-------------------|--------------|---------|
| `STRATOS_CLUSTER_NAME` | `--cluster-name` | `CLUSTER_NAME` | - |
| `STRATOS_CLUSTER_ENDPOINT` | `--cluster-endpoint` | `CLUSTER_ENDPOINT` | - |
| `STRATOS_CLUSTER_CA` | `--cluster-ca` | `CLUSTER_CA` | - |
| `STRATOS_CLUSTER_CIDR` | `--cluster-cidr` | `CLUSTER_CIDR` | - |
| `STRATOS_CLOUD_PROVIDER` | `--cloud-provider` | - | `aws` |
| `STRATOS_SYNC_PERIOD` | `--sync-period` | - | `30s` |
| `STRATOS_LEADER_ELECT` | `--leader-elect` | - | `false` |
| `STRATOS_METRICS_BIND_ADDRESS` | `--metrics-bind-address` | - | `:8080` |
| `STRATOS_HEALTH_PROBE_BIND_ADDRESS` | `--health-probe-bind-address` | - | `:8081` |
| `STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT` | `--graceful-shutdown-timeout` | - | `30s` |

### AWS Variables

| Variable | Description |
|----------|-------------|
| `AWS_REGION` | Default AWS region (can be overridden per NodePool) |
| `AWS_ACCESS_KEY_ID` | AWS access key (prefer IRSA instead) |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key (prefer IRSA instead) |

Environment variables can be passed via the `extraEnv` Helm value:

```yaml
extraEnv:
  - name: STRATOS_SYNC_PERIOD
    value: "60s"
  - name: STRATOS_CLOUD_PROVIDER
    value: "fake"
```

### Using .env Files

For local development, create a `.env` file in the working directory:

```bash title=".env"
STRATOS_CLUSTER_NAME=dev-cluster
STRATOS_CLOUD_PROVIDER=fake
STRATOS_SYNC_PERIOD=60s
```

The controller automatically loads this file at startup. See `.env.example` in the repository for a complete template.

## Next Steps

- [Quickstart](./quickstart.md) - Create your first NodePool
- [AWS Setup](../guides/aws-setup.md) - Configure AWS prerequisites
