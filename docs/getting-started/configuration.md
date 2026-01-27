---
sidebar_position: 2
title: Configuration
description: Configure the Stratos controller
---

# Controller Configuration

This guide covers the configuration options for the Stratos controller.

## Controller Flags

The Stratos controller accepts the following command-line flags:

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--cluster-name` | `CLUSTER_NAME` | `default` | Kubernetes cluster name. Used for cloud instance tagging. **Required for production.** |
| `--cloud-provider` | - | `aws` | Cloud provider to use: `aws` or `fake`. |
| `--sync-period` | - | `30s` | Minimum interval for reconciliation. |
| `--metrics-bind-address` | - | `:8080` | Address for the metrics endpoint. |
| `--health-probe-bind-address` | - | `:8081` | Address for health probe endpoints. |
| `--graceful-shutdown-timeout` | - | `30s` | Timeout for graceful shutdown. |

### Zap Logger Flags

The controller uses the Zap logger with these additional flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--zap-devel` | `true` | Development mode (human-readable output). |
| `--zap-log-level` | `info` | Log level: `debug`, `info`, `error`. |
| `--zap-encoder` | `console` | Log encoder: `console` or `json`. |
| `--zap-stacktrace-level` | `error` | Level at which to print stack traces. |

## Deployment Configuration

### Basic Deployment

```yaml title="deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stratos-controller
  namespace: stratos-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: stratos-controller
  template:
    metadata:
      labels:
        app: stratos-controller
    spec:
      serviceAccountName: stratos-controller
      containers:
        - name: controller
          image: stratos-controller:latest
          args:
            - --cluster-name=my-cluster
            - --cloud-provider=aws
            - --sync-period=30s
          ports:
            - containerPort: 8080
              name: metrics
            - containerPort: 8081
              name: health
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
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
```

### Production Logging

For production environments, use JSON logging:

```yaml
args:
  - --zap-encoder=json
  - --zap-log-level=info
  - --zap-devel=false
```

## Health Checks

The controller exposes health endpoints:

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe - is the controller running |
| `/readyz` | 8081 | Readiness probe - is the controller ready to serve |

## Metrics

Prometheus metrics are exposed at `:8080/metrics`. See [Monitoring](../guides/monitoring.md) for details.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CLUSTER_NAME` | Alternative to `--cluster-name` flag |
| `AWS_REGION` | Default AWS region (can be overridden per NodePool) |
| `AWS_ACCESS_KEY_ID` | AWS access key (prefer IRSA instead) |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key (prefer IRSA instead) |

## Next Steps

- [First NodePool](./first-nodepool.md) - Create your first NodePool
- [AWS Setup](../guides/aws-setup.md) - Configure AWS prerequisites
