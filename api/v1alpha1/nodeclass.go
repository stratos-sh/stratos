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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeClass is the interface that all cloud-specific NodeClass types must implement.
// The controller works against this interface; cloud providers type-assert back to
// their concrete type inside LaunchInstance.
//
// +kubebuilder:object:generate=false
type NodeClass interface {
	client.Object

	// GetInstanceType returns the primary instance type (e.g., "m5.large").
	GetInstanceType() string

	// GetRegion returns the cloud region (e.g., "us-east-1").
	GetRegion() string

	// GetConditions returns the status conditions for this NodeClass.
	GetConditions() []metav1.Condition

	// SetConditions replaces the status conditions for this NodeClass.
	SetConditions(conditions []metav1.Condition)

	// GetNodePoolCount returns the number of NodePools referencing this NodeClass.
	GetNodePoolCount() int32

	// SetNodePoolCount sets the number of NodePools referencing this NodeClass.
	SetNodePoolCount(count int32)

	// GetReadinessConditionTypes returns the condition types that must all be True
	// for this NodeClass to be considered ready.
	GetReadinessConditionTypes() []string

	// GetFinalizerInUse returns the finalizer key used to track in-use status.
	GetFinalizerInUse() string
}
