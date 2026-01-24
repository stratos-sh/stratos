---
sidebar_position: 3
title: Monitoring
description: Monitor Stratos with Prometheus metrics and alerts
---

# Monitoring

Stratos exposes Prometheus metrics at `:8080/metrics`. This guide covers metrics, alerts, and dashboards.

## Metrics Reference

### Node Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `stratos_nodepool_nodes_total` | Gauge | `pool`, `state` | Nodes by pool and state |
| `stratos_nodepool_starting_nodes` | Gauge | `pool` | Nodes currently starting |
| `stratos_nodepool_desired_standby` | Gauge | `pool` | Desired standby count (minStandby) |
| `stratos_nodepool_pool_size` | Gauge | `pool` | Maximum pool size |

### Operation Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `stratos_nodepool_scaleup_total` | Counter | `pool` | Total scale-up operations |
| `stratos_nodepool_scaledown_total` | Counter | `pool` | Total scale-down operations |
| `stratos_nodepool_scaleup_duration_seconds` | Histogram | `pool` | Scale-up latency |
| `stratos_nodepool_warmup_duration_seconds` | Histogram | `pool` | Warmup latency |
| `stratos_nodepool_drain_duration_seconds` | Histogram | `pool` | Drain latency |
| `stratos_nodepool_warmup_failures_total` | Counter | `pool`, `reason` | Warmup failures |

### Startup Taint Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `stratos_nodepool_startup_taint_removal_total` | Counter | `pool`, `trigger`, `result` | Startup taint removals |
| `stratos_nodepool_startup_taint_duration_seconds` | Histogram | `pool` | Time to remove startup taints |

### Controller Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `stratos_nodepool_reconciliation_duration_seconds` | Histogram | `pool` | Reconciliation latency |
| `stratos_nodepool_reconciliation_errors_total` | Counter | `pool`, `type` | Reconciliation errors |

### Cloud Provider Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `stratos_cloud_provider_calls_total` | Counter | `provider`, `operation`, `status` | Cloud API calls |
| `stratos_cloud_provider_latency_seconds` | Histogram | `provider`, `operation` | Cloud API latency |

## Prometheus Queries

### Pool Health

```promql
# Nodes by state per pool
stratos_nodepool_nodes_total{pool="workers"}

# Available standby nodes
stratos_nodepool_nodes_total{pool="workers", state="standby"}

# Nodes currently starting
stratos_nodepool_starting_nodes{pool="workers"}

# Pool utilization (running / poolSize)
stratos_nodepool_nodes_total{state="running"} / stratos_nodepool_pool_size

# Standby shortage
stratos_nodepool_desired_standby - stratos_nodepool_nodes_total{state="standby"}
```

### Scale Operations

```promql
# Scale-up rate (per 5 minutes)
rate(stratos_nodepool_scaleup_total[5m])

# Scale-down rate
rate(stratos_nodepool_scaledown_total[5m])

# P95 scale-up latency
histogram_quantile(0.95, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))

# P99 scale-up latency
histogram_quantile(0.99, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))

# Warmup success rate
1 - (rate(stratos_nodepool_warmup_failures_total[1h]) / rate(stratos_nodepool_warmup_duration_seconds_count[1h]))
```

### Controller Health

```promql
# Reconciliation latency (P95)
histogram_quantile(0.95, rate(stratos_nodepool_reconciliation_duration_seconds_bucket[5m]))

# Reconciliation error rate
rate(stratos_nodepool_reconciliation_errors_total[5m])

# Cloud API latency (P95)
histogram_quantile(0.95, rate(stratos_cloud_provider_latency_seconds_bucket[5m]))

# Cloud API error rate
rate(stratos_cloud_provider_calls_total{status="error"}[5m])
```

## Recommended Alerts

```yaml title="stratos-alerts.yaml"
groups:
  - name: stratos
    rules:
      # Insufficient standby nodes
      - alert: StratosInsufficientStandby
        expr: |
          stratos_nodepool_nodes_total{state="standby"}
          < stratos_nodepool_desired_standby
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NodePool {{ $labels.pool }} has insufficient standby nodes"
          description: "Pool has {{ $value }} standby nodes but needs {{ $labels.desired }}"

      # No standby nodes
      - alert: StratosNoStandby
        expr: |
          stratos_nodepool_nodes_total{state="standby"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "NodePool {{ $labels.pool }} has no standby nodes"
          description: "Scale-up will be delayed until warmup completes"

      # High warmup failure rate
      - alert: StratosWarmupFailures
        expr: |
          rate(stratos_nodepool_warmup_failures_total[1h]) > 0.1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High warmup failure rate for pool {{ $labels.pool }}"
          description: "Failure rate is {{ $value | printf \"%.2f\" }} per second"

      # Scale-up taking too long
      - alert: StratosSlowScaleUp
        expr: |
          histogram_quantile(0.95, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m])) > 60
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Slow scale-up for pool {{ $labels.pool }}"
          description: "P95 scale-up latency is {{ $value | printf \"%.0f\" }} seconds"

      # Controller reconciliation errors
      - alert: StratosReconciliationErrors
        expr: |
          rate(stratos_nodepool_reconciliation_errors_total[5m]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Reconciliation errors for pool {{ $labels.pool }}"
          description: "Error type: {{ $labels.type }}"

      # Pool at capacity
      - alert: StratosPoolAtCapacity
        expr: |
          stratos_nodepool_nodes_total{state="running"} >= stratos_nodepool_pool_size
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NodePool {{ $labels.pool }} is at capacity"
          description: "Running {{ $value }} nodes, pool size is {{ $labels.pool_size }}"

      # Cloud API errors
      - alert: StratosCloudAPIErrors
        expr: |
          rate(stratos_cloud_provider_calls_total{status="error"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Cloud API errors for {{ $labels.provider }}"
          description: "Operation {{ $labels.operation }} failing at {{ $value | printf \"%.2f\" }} per second"
```

## Grafana Dashboard

### Panel 1: Pool Overview

**Query:** Node distribution by state

```promql
stratos_nodepool_nodes_total
```

**Visualization:** Stacked bar chart by state

### Panel 2: Standby Health

**Query:** Standby vs desired

```promql
# Actual standby
stratos_nodepool_nodes_total{state="standby"}

# Desired standby
stratos_nodepool_desired_standby
```

**Visualization:** Gauge with thresholds

### Panel 3: Scale Operations

**Query:** Scale events over time

```promql
increase(stratos_nodepool_scaleup_total[5m])
increase(stratos_nodepool_scaledown_total[5m])
```

**Visualization:** Time series

### Panel 4: Scale-Up Latency

**Query:** Latency percentiles

```promql
histogram_quantile(0.5, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))
```

**Visualization:** Time series with legend (P50, P95, P99)

### Panel 5: Controller Health

**Query:** Reconciliation metrics

```promql
rate(stratos_nodepool_reconciliation_errors_total[5m])
histogram_quantile(0.95, rate(stratos_nodepool_reconciliation_duration_seconds_bucket[5m]))
```

**Visualization:** Time series

### Panel 6: Cloud API

**Query:** API call rates and latency

```promql
rate(stratos_cloud_provider_calls_total[5m])
histogram_quantile(0.95, rate(stratos_cloud_provider_latency_seconds_bucket[5m]))
```

**Visualization:** Time series

## Kubernetes Events

Stratos emits Kubernetes events for significant operations:

```bash
kubectl get events --field-selector involvedObject.kind=NodePool
```

Event types:

| Event | Description |
|-------|-------------|
| `Created` | NodePool created |
| `ScaleUp` | Nodes started for scale-up |
| `ScaleDown` | Nodes stopped for scale-down |
| `Replenishing` | Launching new nodes for minStandby |
| `MaxRuntimeExceeded` | Node recycled due to max runtime |
| `CleanupFailed` | Error during NodePool deletion |
| `Deleted` | NodePool deleted |

## Health Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe |
| `/readyz` | 8081 | Readiness probe |
| `/metrics` | 8080 | Prometheus metrics |

## Troubleshooting with Metrics

### Diagnosing Scale-Up Issues

1. Check standby availability:
   ```promql
   stratos_nodepool_nodes_total{state="standby"}
   ```

2. Check in-flight scale-ups:
   ```promql
   stratos_nodepool_starting_nodes
   ```

3. Check scale-up latency:
   ```promql
   histogram_quantile(0.95, rate(stratos_nodepool_scaleup_duration_seconds_bucket[5m]))
   ```

### Diagnosing Warmup Issues

1. Check warmup duration:
   ```promql
   histogram_quantile(0.95, rate(stratos_nodepool_warmup_duration_seconds_bucket[5m]))
   ```

2. Check failure rate:
   ```promql
   rate(stratos_nodepool_warmup_failures_total[1h])
   ```

### Diagnosing Cloud API Issues

1. Check API error rate:
   ```promql
   rate(stratos_cloud_provider_calls_total{status="error"}[5m])
   ```

2. Check API latency:
   ```promql
   histogram_quantile(0.95, rate(stratos_cloud_provider_latency_seconds_bucket[5m]))
   ```

## Next Steps

- [NodePool API](../reference/api/nodepool.md) - Complete API reference
- [Labels and Annotations](../reference/labels-annotations.md) - Label reference
