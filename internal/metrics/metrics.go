/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package metrics provides Prometheus metrics for Stratos.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	namespace = "stratos"
	subsystem = "nodepool"
)

var (
	// NodesTotal tracks the total number of nodes by pool and state.
	NodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "nodes_total",
			Help:      "Total nodes by state (warmup, standby, running, terminating)",
		},
		[]string{"pool", "state"},
	)

	// StartingNodes tracks the number of nodes currently in the process of starting.
	// These are nodes that have been triggered for scale-up but are not yet Ready.
	StartingNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "starting_nodes",
			Help:      "Number of nodes currently starting (in-flight scale-up)",
		},
		[]string{"pool"},
	)

	// DesiredStandby tracks the desired standby count per pool.
	DesiredStandby = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "desired_standby",
			Help:      "Desired number of standby nodes (minStandby)",
		},
		[]string{"pool"},
	)

	// PoolSize tracks the pool size limit per pool.
	PoolSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "pool_size",
			Help:      "Maximum pool size",
		},
		[]string{"pool"},
	)

	// ScaleUpTotal tracks the total number of scale-up operations.
	ScaleUpTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "scaleup_total",
			Help:      "Total scale-up operations",
		},
		[]string{"pool"},
	)

	// ScaleDownTotal tracks the total number of scale-down operations.
	ScaleDownTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "scaledown_total",
			Help:      "Total scale-down operations",
		},
		[]string{"pool"},
	)

	// WarmupFailuresTotal tracks the total number of warmup failures.
	WarmupFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "warmup_failures_total",
			Help:      "Total warmup failures by reason",
		},
		[]string{"pool", "reason"},
	)

	// ScaleUpDuration tracks the duration of scale-up operations.
	ScaleUpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "scaleup_duration_seconds",
			Help:      "Time from pending pod to node Ready",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
		},
		[]string{"pool"},
	)

	// WarmupDuration tracks the duration of warmup operations.
	WarmupDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "warmup_duration_seconds",
			Help:      "Time from launch to standby",
			Buckets:   []float64{30, 60, 120, 180, 300, 600, 900},
		},
		[]string{"pool"},
	)

	// DrainDuration tracks the duration of drain operations.
	DrainDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "drain_duration_seconds",
			Help:      "Time to drain node",
			Buckets:   []float64{5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"pool"},
	)

	// ReconciliationDuration tracks the duration of reconciliation loops.
	ReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_duration_seconds",
			Help:      "Time to complete reconciliation loop",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"pool"},
	)

	// ReconciliationErrors tracks reconciliation errors.
	ReconciliationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconciliation_errors_total",
			Help:      "Total reconciliation errors",
		},
		[]string{"pool", "type"},
	)

	// CloudProviderCalls tracks cloud provider API calls.
	CloudProviderCalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cloud_provider_calls_total",
			Help:      "Total cloud provider API calls",
		},
		[]string{"provider", "operation", "status"},
	)

	// CloudProviderLatency tracks cloud provider API latency.
	CloudProviderLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "cloud_provider_latency_seconds",
			Help:      "Cloud provider API call latency",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"provider", "operation"},
	)

	// StartupTaintRemovalTotal tracks the total number of startup taint removals.
	StartupTaintRemovalTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "startup_taint_removal_total",
			Help:      "Total startup taint removals by trigger (network_ready, timeout, external)",
		},
		[]string{"pool", "trigger", "result"},
	)

	// StartupTaintDuration tracks the duration from node start to startup taint removal.
	StartupTaintDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "startup_taint_duration_seconds",
			Help:      "Time from node start to startup taint removal",
			Buckets:   []float64{5, 10, 15, 30, 60, 120, 180},
		},
		[]string{"pool"},
	)
)

func init() {
	// Register all metrics with controller-runtime's metrics registry
	metrics.Registry.MustRegister(
		NodesTotal,
		StartingNodes,
		DesiredStandby,
		PoolSize,
		ScaleUpTotal,
		ScaleDownTotal,
		WarmupFailuresTotal,
		ScaleUpDuration,
		WarmupDuration,
		DrainDuration,
		ReconciliationDuration,
		ReconciliationErrors,
		CloudProviderCalls,
		CloudProviderLatency,
		StartupTaintRemovalTotal,
		StartupTaintDuration,
	)
}

// RecordNodeCounts updates the node count metrics for a pool.
func RecordNodeCounts(pool string, warmup, standby, running, terminating int) {
	NodesTotal.WithLabelValues(pool, "warmup").Set(float64(warmup))
	NodesTotal.WithLabelValues(pool, "standby").Set(float64(standby))
	NodesTotal.WithLabelValues(pool, "running").Set(float64(running))
	NodesTotal.WithLabelValues(pool, "terminating").Set(float64(terminating))
}

// RecordPoolConfig updates the pool configuration metrics.
func RecordPoolConfig(pool string, minStandby, poolSize int32) {
	DesiredStandby.WithLabelValues(pool).Set(float64(minStandby))
	PoolSize.WithLabelValues(pool).Set(float64(poolSize))
}

// RecordScaleUp records a scale-up operation.
func RecordScaleUp(pool string) {
	ScaleUpTotal.WithLabelValues(pool).Inc()
}

// RecordScaleDown records a scale-down operation.
func RecordScaleDown(pool string) {
	ScaleDownTotal.WithLabelValues(pool).Inc()
}

// RecordStartingNodes updates the starting nodes gauge for a pool.
func RecordStartingNodes(pool string, count int) {
	StartingNodes.WithLabelValues(pool).Set(float64(count))
}

// RecordWarmupFailure records a warmup failure.
func RecordWarmupFailure(pool, reason string) {
	WarmupFailuresTotal.WithLabelValues(pool, reason).Inc()
}

// RecordScaleUpDuration records the duration of a scale-up operation.
func RecordScaleUpDuration(pool string, durationSeconds float64) {
	ScaleUpDuration.WithLabelValues(pool).Observe(durationSeconds)
}

// RecordWarmupDuration records the duration of a warmup operation.
func RecordWarmupDuration(pool string, durationSeconds float64) {
	WarmupDuration.WithLabelValues(pool).Observe(durationSeconds)
}

// RecordDrainDuration records the duration of a drain operation.
func RecordDrainDuration(pool string, durationSeconds float64) {
	DrainDuration.WithLabelValues(pool).Observe(durationSeconds)
}

// RecordReconciliationDuration records the duration of a reconciliation loop.
func RecordReconciliationDuration(pool string, durationSeconds float64) {
	ReconciliationDuration.WithLabelValues(pool).Observe(durationSeconds)
}

// RecordReconciliationError records a reconciliation error.
func RecordReconciliationError(pool, errorType string) {
	ReconciliationErrors.WithLabelValues(pool, errorType).Inc()
}

// RecordCloudProviderCall records a cloud provider API call.
func RecordCloudProviderCall(provider, operation, status string, durationSeconds float64) {
	CloudProviderCalls.WithLabelValues(provider, operation, status).Inc()
	CloudProviderLatency.WithLabelValues(provider, operation).Observe(durationSeconds)
}

// CleanupPoolMetrics removes metrics for a deleted pool.
func CleanupPoolMetrics(pool string) {
	NodesTotal.DeleteLabelValues(pool, "warmup")
	NodesTotal.DeleteLabelValues(pool, "standby")
	NodesTotal.DeleteLabelValues(pool, "running")
	NodesTotal.DeleteLabelValues(pool, "terminating")
	StartingNodes.DeleteLabelValues(pool)
	DesiredStandby.DeleteLabelValues(pool)
	PoolSize.DeleteLabelValues(pool)
}

// RecordStartupTaintRemoval records a startup taint removal operation.
// trigger is the reason for removal: "network_ready", "timeout", or "external"
// result is "success" or "error"
func RecordStartupTaintRemoval(pool, trigger, result string) {
	StartupTaintRemovalTotal.WithLabelValues(pool, trigger, result).Inc()
}

// RecordStartupTaintDuration records the duration from node start to startup taint removal.
func RecordStartupTaintDuration(pool string, durationSeconds float64) {
	StartupTaintDuration.WithLabelValues(pool).Observe(durationSeconds)
}
