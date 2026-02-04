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

// Package scaling provides Kubernetes pod-demand scaling for Stratos
// NodePools. It computes node demand from unschedulable pods, orchestrates
// node drain (cordoning, pod eviction with PodDisruptionBudget respect),
// checks network/CNI readiness, and manages startup taint lifecycle.
//
// Key types:
//   - Scaler: core scaling logic -- CheckDemand, OnScaleUp,
//     FindScaleDownCandidates, DrainAndStop, RunMaintenance
//   - ScalingDemand: describes how many nodes should be started
//   - ScaleDownCandidate: wraps a node eligible for scale-down
package scaling
