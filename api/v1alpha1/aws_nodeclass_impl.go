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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Compile-time check that AWSNodeClass implements NodeClass.
var _ NodeClass = (*AWSNodeClass)(nil)

func (a *AWSNodeClass) GetInstanceType() string {
	return a.Spec.InstanceType
}

func (a *AWSNodeClass) GetRegion() string {
	return a.Spec.Region
}

func (a *AWSNodeClass) GetConditions() []metav1.Condition {
	return a.Status.Conditions
}

func (a *AWSNodeClass) SetConditions(conditions []metav1.Condition) {
	a.Status.Conditions = conditions
}

func (a *AWSNodeClass) GetNodePoolCount() int32 {
	return a.Status.NodePoolCount
}

func (a *AWSNodeClass) SetNodePoolCount(count int32) {
	a.Status.NodePoolCount = count
}

func (a *AWSNodeClass) GetReadinessConditionTypes() []string {
	return []string{
		AWSNodeClassConditionTypeAMIReady,
		AWSNodeClassConditionTypeSubnetsReady,
		AWSNodeClassConditionTypeSecurityGroupsReady,
		AWSNodeClassConditionTypeInstanceProfileReady,
	}
}

func (a *AWSNodeClass) GetFinalizerInUse() string {
	return AWSNodeClassFinalizerInUse
}
