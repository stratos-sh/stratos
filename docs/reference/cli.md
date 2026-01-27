---
sidebar_position: 2
title: CLI Reference
description: Stratos controller command-line flags reference
---

# CLI Reference

This document provides a complete reference for Stratos controller command-line flags.

## Controller Flags

### Core Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cluster-name` | string | `default` | Kubernetes cluster name. Used for cloud instance tagging. Can also be set via `CLUSTER_NAME` environment variable. **Required for production.** |
| `--cloud-provider` | string | `aws` | Cloud provider to use. Options: `aws`, `fake`. |
| `--sync-period` | duration | `30s` | Minimum interval at which watched resources are reconciled. |
| `--graceful-shutdown-timeout` | duration | `30s` | Timeout for graceful shutdown of the manager. |

### Network Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--metrics-bind-address` | string | `:8080` | The address the metric endpoint binds to. |
| `--health-probe-bind-address` | string | `:8081` | The address the probe endpoint binds to. |

### Zap Logger Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--zap-devel` | bool | `true` | Development mode. Sets encoder to `console`, time encoding to ISO8601. |
| `--zap-log-level` | string | `info` | Log level. Options: `debug`, `info`, `error`. |
| `--zap-encoder` | string | `console` | Log encoder. Options: `console`, `json`. |
| `--zap-stacktrace-level` | string | `error` | Level at which to print stack traces. Options: `debug`, `info`, `error`. |
| `--zap-time-encoding` | string | `iso8601` | Time encoding format. Options: `epoch`, `millis`, `nano`, `iso8601`, `rfc3339`, `rfc3339nano`. |

## Environment Variables

| Variable | Corresponding Flag | Description |
|----------|-------------------|-------------|
| `CLUSTER_NAME` | `--cluster-name` | Kubernetes cluster name |
| `AWS_REGION` | - | Default AWS region (can be overridden per NodePool) |
| `AWS_ACCESS_KEY_ID` | - | AWS access key (prefer IRSA instead) |
| `AWS_SECRET_ACCESS_KEY` | - | AWS secret key (prefer IRSA instead) |
| `AWS_SESSION_TOKEN` | - | AWS session token for temporary credentials |

## Usage Examples

### Basic Usage

```bash
stratos \
  --cluster-name=production \
  --cloud-provider=aws
```

### Local Development

```bash
go run ./cmd/stratos/main.go \
  --cluster-name=dev \
  --cloud-provider=fake \
  --zap-log-level=debug
```

### Production with HA

```bash
stratos \
  --cluster-name=production \
  --cloud-provider=aws \
  --leader-elect=true \
  --zap-encoder=json \
  --zap-devel=false \
  --zap-log-level=info
```

### Custom Reconciliation Interval

```bash
stratos \
  --cluster-name=production \
  --sync-period=60s
```

### Custom Ports

```bash
stratos \
  --cluster-name=production \
  --metrics-bind-address=:9090 \
  --health-probe-bind-address=:9091
```

## Kubernetes Deployment

Stratos is deployed via Helm. See [Installation](../getting-started/installation.md) for full details.

### Basic Deployment

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=production
```

### High Availability Deployment

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=production \
  --set replicaCount=2
```

Leader election is enabled by default (`leaderElect: true`).

### Production Logging

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=production \
  --set extraArgs[0]=--zap-encoder=json \
  --set extraArgs[1]=--zap-devel=false \
  --set extraArgs[2]=--zap-log-level=info
```

## Health Endpoints

The controller exposes health endpoints for Kubernetes probes:

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe - controller is running |
| `/readyz` | 8081 | Readiness probe - controller is ready |

### Probe Configuration

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 15
  periodSeconds: 20

readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Metrics Endpoint

Prometheus metrics are exposed at the metrics address:

| Endpoint | Port | Description |
|----------|------|-------------|
| `/metrics` | 8080 | Prometheus metrics |

### ServiceMonitor

```yaml title="servicemonitor.yaml"
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: stratos
  namespace: stratos-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: stratos
  endpoints:
    - port: metrics
      interval: 30s
```

## Next Steps

- [Labels and Annotations](./labels-annotations.md) - Labels and tags reference
- [NodePool API](./api/nodepool.md) - CRD API reference
