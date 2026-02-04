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

// Package metrics provides Prometheus metric registration and helper
// variables for the Stratos controller.
//
// All metrics are registered with the controller-runtime metrics registry
// at init time. The package defines gauges, counters, and histograms that
// track node pool state (NodesTotal, NodesByState), scaling operations
// (ScaleUpTotal, ScaleDownTotal), cloud API performance
// (CloudOperationDuration), and reconciliation health
// (ReconciliationErrors).
//
// Key variables:
//   - NodesTotal, NodesByState: gauge vectors for pool sizing
//   - ScaleUpTotal, ScaleDownTotal: counters for scaling events
//   - CloudOperationDuration: histogram for cloud API latency
//   - ReconciliationErrors: counter for reconciliation failures
//
// This is a leaf package with no internal dependencies. It is imported by
// the aws, lifecycle, nodepool, and scaling packages.
package metrics
