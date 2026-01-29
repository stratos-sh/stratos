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

// Package nodemanager handles the lifecycle of Stratos-managed Kubernetes nodes.
package controller

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// NodeState represents the lifecycle state of a Stratos-managed node.
type NodeState string

const (
	// NodeStateWarmup - Instance launched, waiting for self-stop
	NodeStateWarmup NodeState = "warmup"

	// NodeStateStandby - Instance stopped, ready to start instantly
	NodeStateStandby NodeState = "standby"

	// NodeStateRunning - Instance running, serving pods
	NodeStateRunning NodeState = "running"

	// NodeStateTerminating - Node being drained/stopped
	NodeStateTerminating NodeState = "terminating"

	// NodeStateUnknown - Unknown state
	NodeStateUnknown NodeState = "unknown"
)

// Labels used for Stratos-managed nodes
const (
	// LabelPool identifies which NodePool manages this node
	LabelPool = "stratos.sh/pool"

	// LabelState indicates the current Stratos state
	LabelState = "stratos.sh/state"

	// LabelInstanceID maps to the cloud provider instance ID
	LabelInstanceID = "stratos.sh/instance-id"

	// LabelStateSince indicates when the node entered its current state
	LabelStateSince = "stratos.sh/state-since"
)

// Annotations used for Stratos-managed nodes
const (
	// AnnotationWarmupCompleted records when warmup finished
	AnnotationWarmupCompleted = "stratos.sh/warmup-completed"

	// AnnotationLastStarted records when the node was last started
	AnnotationLastStarted = "stratos.sh/last-started"

	// AnnotationScaleDownCandidateSince records when the node became a scale-down candidate
	AnnotationScaleDownCandidateSince = "stratos.sh/scale-down-candidate-since"

	// AnnotationScaleUpStarted marks when a node was started for scale-up.
	// Used for in-flight tracking to prevent duplicate scale-ups.
	AnnotationScaleUpStarted = "stratos.sh/scale-up-started"
)

// Scale-up tracking constants
const (
	// ScaleUpStartedTTL is how long to consider a node as "starting" for in-flight tracking.
	// After this duration, the annotation is considered stale and will be ignored/cleared.
	ScaleUpStartedTTL = 60 * time.Second

	// PodAssignmentTTL is the maximum duration a pod assignment remains valid.
	// After this period, the assignment is considered stale and will be cleaned up.
	PodAssignmentTTL = 10 * time.Minute
)

// Startup taint constants
const (
	// StartupTaintRemovalTimeout is how long to wait for CNI readiness before timing out.
	// After this duration, startup taints are forcibly removed (WhenNetworkReady mode)
	// or a warning is emitted (External mode).
	StartupTaintRemovalTimeout = 2 * time.Minute

	// NetworkingReadyCondition is the EKS-specific condition type set by eks-node-monitoring-agent
	// when the VPC CNI IPAMD is connected and networking is functional.
	NetworkingReadyCondition corev1.NodeConditionType = "NetworkingReady"
)

// Taints used for Stratos-managed nodes
const (
	// TaintKeyStandby is applied to standby nodes with NoExecute effect to evict stale pods
	TaintKeyStandby = "stratos.sh/standby"
)

// Tags used on cloud provider instances
const (
	// TagManagedBy identifies the instance as managed by Stratos
	TagManagedBy = "managed-by"

	// TagManagedByValue is the value for the managed-by tag
	TagManagedByValue = "stratos"

	// TagPool identifies which NodePool manages this instance
	TagPool = "stratos.sh/pool"

	// TagCluster identifies the Kubernetes cluster
	TagCluster = "stratos.sh/cluster"

	// TagState indicates the current Stratos state
	TagState = "stratos.sh/state"
)

// ValidTransitions defines valid state transitions for nodes.
// The map key is the current state, and the value is a slice of valid target states.
var ValidTransitions = map[NodeState][]NodeState{
	NodeStateWarmup: {
		NodeStateStandby,     // Instance self-stopped or timed out with stop action
		NodeStateTerminating, // Timeout with terminate action
	},
	NodeStateStandby: {
		NodeStateRunning,     // Scale-up triggered
		NodeStateTerminating, // Pool deleted or node needs recycling
	},
	NodeStateRunning: {
		NodeStateTerminating, // Scale-down or max runtime exceeded
		NodeStateStandby,     // Instance stopped externally (e.g., AWS console, spot interruption)
	},
	NodeStateTerminating: {
		NodeStateStandby, // Drain complete, instance stopped
	},
}

// IsValidTransition checks if transitioning from one state to another is valid.
func IsValidTransition(from, to NodeState) bool {
	validTargets, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, valid := range validTargets {
		if valid == to {
			return true
		}
	}
	return false
}

// InvalidTransitionError represents an invalid state transition.
type InvalidTransitionError struct {
	From NodeState
	To   NodeState
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("invalid state transition from %s to %s", e.From, e.To)
}

// ParseNodeState parses a string into a NodeState.
func ParseNodeState(s string) NodeState {
	switch NodeState(s) {
	case NodeStateWarmup, NodeStateStandby, NodeStateRunning, NodeStateTerminating:
		return NodeState(s)
	default:
		return NodeStateUnknown
	}
}

// IsActiveState returns true if the state represents an active node.
func (s NodeState) IsActiveState() bool {
	return s == NodeStateWarmup || s == NodeStateStandby || s == NodeStateRunning
}

// CanAcceptWorkload returns true if the node can accept pod scheduling.
func (s NodeState) CanAcceptWorkload() bool {
	return s == NodeStateRunning
}

// IsAvailableForScaleUp returns true if the node can be started for scale-up.
func (s NodeState) IsAvailableForScaleUp() bool {
	return s == NodeStateStandby
}

// IsNodeReady checks if a node has Ready condition = True.
func IsNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
