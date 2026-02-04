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

// Package nodestate defines the node state machine for Stratos-managed
// nodes, including state constants, valid transitions, and shared label
// and taint definitions.
//
// Every Stratos-managed node passes through a well-defined lifecycle:
//
//	warmup -> standby -> running -> terminating
//
// The warmup state represents a freshly launched instance executing its
// user data script. Once ready, it self-stops into standby. Standby nodes
// are pre-warmed and stopped, ready to start in seconds. Running nodes
// are active with pods scheduled. Terminating nodes are draining before
// being stopped or terminated.
//
// Key types:
//   - NodeState: the state type (warmup, standby, running, terminating)
//   - ValidTransition: validates whether a state transition is allowed
//
// Label and taint constants (LabelPool, LabelState, LabelInstanceID,
// TaintNotReady, etc.) are defined here so that all packages reference a
// single source of truth.
//
// This is a pure leaf package with zero internal dependencies. It is
// imported by lifecycle, nodepool, and scaling.
package nodestate
