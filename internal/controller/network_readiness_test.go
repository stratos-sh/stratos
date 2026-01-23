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

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetworkReadinessChecker_IsReady(t *testing.T) {
	tests := []struct {
		name       string
		conditions []corev1.NodeCondition
		want       bool
	}{
		{
			name:       "no conditions -> not ready",
			conditions: nil,
			want:       false,
		},
		{
			name: "EKS node with NetworkingReady=True -> ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: "NetworkingIsReady",
				},
			},
			want: true,
		},
		{
			name: "EKS node with NetworkingReady=False -> not ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: "NetworkingNotReady",
				},
			},
			want: false,
		},
		{
			name: "Cilium node with NetworkUnavailable=False -> ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionFalse,
					Reason: "CiliumIsUp",
				},
			},
			want: true,
		},
		{
			name: "Calico node with NetworkUnavailable=False -> ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionFalse,
					Reason: "CalicoIsUp",
				},
			},
			want: true,
		},
		{
			name: "node with NetworkUnavailable=True -> not ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionTrue,
					Reason: "NetworkPluginNotReady",
				},
			},
			want: false,
		},
		{
			name: "node with only Ready condition -> not ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
					Reason: "KubeletReady",
				},
			},
			want: false,
		},
		{
			name: "EKS node with multiple conditions including NetworkingReady=True -> ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
					Reason: "KubeletReady",
				},
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: "NetworkingIsReady",
				},
				{
					Type:   corev1.NodeMemoryPressure,
					Status: corev1.ConditionFalse,
				},
			},
			want: true,
		},
		{
			name: "node with both EKS and standard conditions (EKS takes precedence) -> ready",
			conditions: []corev1.NodeCondition{
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: "NetworkingIsReady",
				},
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionTrue, // This is ignored because NetworkingReady is True
					Reason: "NetworkPluginNotReady",
				},
			},
			want: true,
		},
	}

	checker := NewNetworkReadinessChecker(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Conditions: tt.conditions,
				},
			}

			got := checker.IsReady(node)
			if got != tt.want {
				t.Errorf("IsReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkReadinessChecker_GetNetworkConditionReason(t *testing.T) {
	tests := []struct {
		name       string
		conditions []corev1.NodeCondition
		want       string
	}{
		{
			name:       "no conditions -> empty reason",
			conditions: nil,
			want:       "",
		},
		{
			name: "EKS node with NetworkingReady=True -> returns reason",
			conditions: []corev1.NodeCondition{
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: "NetworkingIsReady",
				},
			},
			want: "NetworkingIsReady",
		},
		{
			name: "EKS node with NetworkingReady=False -> empty reason",
			conditions: []corev1.NodeCondition{
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: "NetworkingNotReady",
				},
			},
			want: "",
		},
		{
			name: "Cilium node with NetworkUnavailable=False -> returns reason",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionFalse,
					Reason: "CiliumIsUp",
				},
			},
			want: "CiliumIsUp",
		},
		{
			name: "Calico node with NetworkUnavailable=False -> returns reason",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionFalse,
					Reason: "CalicoIsUp",
				},
			},
			want: "CalicoIsUp",
		},
		{
			name: "node with NetworkUnavailable=True -> empty reason",
			conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionTrue,
					Reason: "NetworkPluginNotReady",
				},
			},
			want: "",
		},
		{
			name: "EKS takes precedence over standard condition",
			conditions: []corev1.NodeCondition{
				{
					Type:   NetworkingReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: "NetworkingIsReady",
				},
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionFalse,
					Reason: "CiliumIsUp",
				},
			},
			want: "NetworkingIsReady", // EKS condition is checked first
		},
	}

	checker := NewNetworkReadinessChecker(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Conditions: tt.conditions,
				},
			}

			got := checker.GetNetworkConditionReason(node)
			if got != tt.want {
				t.Errorf("GetNetworkConditionReason() = %q, want %q", got, tt.want)
			}
		})
	}
}
