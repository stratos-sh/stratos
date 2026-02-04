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

// Package nodepool implements the NodePool controller, the central
// reconciler in Stratos.
//
// The Reconciler watches NodePool custom resources and manages the full
// node lifecycle: scaling up from standby to running when demand is
// detected, scaling down when nodes are idle, maintaining pool sizes,
// and synchronizing Kubernetes node state with cloud instance state.
//
// Reconciliation is structured as a sequence of phases: validation,
// cloud provider initialization, cloud state synchronization, warmup
// monitoring, pool maintenance, demand checking (via scaling.Scaler),
// scale-up execution, and scale-down execution. Each phase is
// implemented as a focused helper method in reconciler_helpers.go.
//
// Key types:
//   - Reconciler: implements reconcile.Reconciler for NodePool resources
//
// The Reconciler delegates to lifecycle.Manager for node operations and
// to scaling.Scaler for demand calculation. It also imports
// cloudprovider, config, metrics, and nodestate.
package nodepool
