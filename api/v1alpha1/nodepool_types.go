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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
	// PoolSize is the maximum total nodes (standby + running, excluding warmup)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	PoolSize int32 `json:"poolSize"`

	// MinStandby is the minimum number of nodes to maintain in stopped/standby state
	// +kubebuilder:validation:Minimum=0
	MinStandby int32 `json:"minStandby"`

	// Template defines the node template for this pool
	Template NodeTemplate `json:"template"`

	// ScaleDown configures automatic scale-down behavior
	// +optional
	ScaleDown *ScaleDownConfig `json:"scaleDown,omitempty"`

	// PreWarm configures the pre-warming lifecycle
	// +optional
	PreWarm *PreWarmConfig `json:"preWarm,omitempty"`

	// MaxNodeRuntime configures automatic node recycling.
	// Zero or nil means disabled.
	// +optional
	MaxNodeRuntime *metav1.Duration `json:"maxNodeRuntime,omitempty"`

	// ReconciliationInterval is how often to run the maintenance loop.
	// Default: 30 seconds
	// +optional
	ReconciliationInterval *metav1.Duration `json:"reconciliationInterval,omitempty"`

	// ScaleUp configures scale-up behavior including resource-based calculation
	// +optional
	ScaleUp *ScaleUpConfig `json:"scaleUp,omitempty"`
}

// ScaleUpConfig configures scale-up behavior
type ScaleUpConfig struct {
	// DefaultPodResources specifies default resource requests for pods
	// that don't have explicit requests. Used in scale-up calculations
	// to estimate how many nodes are needed.
	// +optional
	DefaultPodResources *corev1.ResourceRequirements `json:"defaultPodResources,omitempty"`
}

// NodeClassRef references a cloud-specific NodeClass resource
type NodeClassRef struct {
	// Kind is the NodeClass kind (e.g., "AWSNodeClass", "GCPNodeClass")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=AWSNodeClass
	Kind string `json:"kind"`

	// Name is the name of the NodeClass resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// NetworkReadinessStrategy defines how Stratos handles network readiness for nodes.
// +kubebuilder:validation:Enum=Taint;None
type NetworkReadinessStrategy string

const (
	// NetworkReadinessStrategyTaint adds a stratos.sh/not-ready=true:NoSchedule taint
	// to kubelet and removes it when the CNI is ready.
	NetworkReadinessStrategyTaint NetworkReadinessStrategy = "Taint"

	// NetworkReadinessStrategyNone disables network readiness management.
	NetworkReadinessStrategyNone NetworkReadinessStrategy = "None"
)

// NodeTemplate defines the template for nodes in this pool
type NodeTemplate struct {
	// Labels to apply to nodes
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Taints to apply to nodes (permanent taints for workload isolation)
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// NetworkReadinessStrategy controls how Stratos handles network readiness.
	// "Taint" adds a stratos.sh/not-ready=true:NoSchedule taint to kubelet and
	// removes it when the CNI is ready. "None" disables network readiness management.
	// Default: Taint.
	// +kubebuilder:default=Taint
	// +optional
	NetworkReadinessStrategy *NetworkReadinessStrategy `json:"networkReadinessStrategy,omitempty"`

	// NodeClassRef references the cloud-specific NodeClass that defines
	// instance configuration (e.g., AWSNodeClass for AWS).
	// +kubebuilder:validation:Required
	NodeClassRef NodeClassRef `json:"nodeClassRef"`
}

// IsNetworkReadinessTaintEnabled returns true if the network readiness taint is enabled.
// Defaults to true when the field is nil (Taint strategy).
func (t *NodeTemplate) IsNetworkReadinessTaintEnabled() bool {
	if t.NetworkReadinessStrategy == nil {
		return true
	}
	return *t.NetworkReadinessStrategy != NetworkReadinessStrategyNone
}

// PodAssignment tracks the mapping of an unschedulable pod to a node being started for it.
// Used to prevent duplicate scale-ups when a node is starting but the pod hasn't been scheduled yet.
type PodAssignment struct {
	// PodName is the name of the pending pod
	PodName string `json:"podName"`

	// PodNamespace is the namespace of the pending pod
	PodNamespace string `json:"podNamespace"`

	// NodeName is the name of the node started for this pod
	NodeName string `json:"nodeName"`

	// AssignedAt is when this assignment was created
	AssignedAt metav1.Time `json:"assignedAt"`
}

// NodePoolStatus defines the observed state of NodePool
type NodePoolStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Warmup is the count of nodes currently initializing
	// +optional
	Warmup int32 `json:"warmup,omitempty"`

	// Standby is the count of nodes in stopped/standby state
	// +optional
	Standby int32 `json:"standby,omitempty"`

	// Running is the count of nodes actively running pods
	// +optional
	Running int32 `json:"running,omitempty"`

	// Total is the total node count (warmup + standby + running)
	// +optional
	Total int32 `json:"total,omitempty"`

	// PodAssignments tracks pending pods assigned to starting nodes.
	// Prevents duplicate scale-ups during the window between node start and pod scheduling.
	// +optional
	PodAssignments []PodAssignment `json:"podAssignments,omitempty"`

	// LastReconcileTime is when the pool was last reconciled
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=np;npool
// +kubebuilder:printcolumn:name="PoolSize",type=integer,JSONPath=`.spec.poolSize`
// +kubebuilder:printcolumn:name="MinStandby",type=integer,JSONPath=`.spec.minStandby`
// +kubebuilder:printcolumn:name="Standby",type=integer,JSONPath=`.status.standby`
// +kubebuilder:printcolumn:name="Running",type=integer,JSONPath=`.status.running`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodePool is the Schema for the nodepools API
type NodePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolSpec   `json:"spec,omitempty"`
	Status NodePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePoolList contains a list of NodePool
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePool{}, &NodePoolList{})
}

// Condition types for NodePool
const (
	// ConditionTypeReady indicates the pool has minStandby nodes available
	ConditionTypeReady = "Ready"

	// ConditionTypeReconciling indicates the pool is being reconciled
	ConditionTypeReconciling = "Reconciling"

	// ConditionTypeDegraded indicates the pool cannot meet minStandby
	ConditionTypeDegraded = "Degraded"

	// ConditionTypeScaleUpInProgress indicates scale-up is in progress
	ConditionTypeScaleUpInProgress = "ScaleUpInProgress"

	// ConditionTypeScaleDownInProgress indicates scale-down is in progress
	ConditionTypeScaleDownInProgress = "ScaleDownInProgress"
)

// Condition reasons
const (
	ReasonPoolReady           = "PoolReady"
	ReasonPoolNotReady        = "PoolNotReady"
	ReasonReconciling         = "Reconciling"
	ReasonDegraded            = "Degraded"
	ReasonScaleUpInProgress   = "ScaleUpInProgress"
	ReasonScaleDownInProgress = "ScaleDownInProgress"
	ReasonNodeClassNotFound   = "NodeClassNotFound"
)
