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

// Package lifecycle handles the lifecycle of Stratos-managed Kubernetes
// nodes, encapsulating all cloud instance interactions behind the Manager
// type.
//
// Manager provides operations that span the full node lifecycle: Launch
// creates new instances from a LaunchConfig, Start transitions stopped
// instances to running, Stop transitions running instances to stopped,
// and Sync reconciles Kubernetes node objects with cloud instance state.
// It also provides MonitorWarmup and MonitorCloudWarmup for tracking
// instances through the warmup phase.
//
// Key types:
//   - Manager: central type providing Launch, Start, Stop, Sync,
//     MonitorWarmup, and MonitorCloudWarmup operations
//
// Manager imports cloudprovider for instance operations, metrics for
// recording cloud API latency, and nodestate for state transition
// validation and label constants.
package lifecycle
