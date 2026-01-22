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

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestIsWellKnownLabel(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "kubernetes.io prefix - hostname",
			key:  "kubernetes.io/hostname",
			want: true,
		},
		{
			name: "kubernetes.io prefix - os",
			key:  "kubernetes.io/os",
			want: true,
		},
		{
			name: "kubernetes.io prefix - arch",
			key:  "kubernetes.io/arch",
			want: true,
		},
		{
			name: "node.kubernetes.io prefix - instance-type",
			key:  "node.kubernetes.io/instance-type",
			want: true,
		},
		{
			name: "topology.kubernetes.io prefix - region",
			key:  "topology.kubernetes.io/region",
			want: true,
		},
		{
			name: "topology.kubernetes.io prefix - zone",
			key:  "topology.kubernetes.io/zone",
			want: true,
		},
		{
			name: "node-role.kubernetes.io prefix - control-plane",
			key:  "node-role.kubernetes.io/control-plane",
			want: true,
		},
		{
			name: "node-role.kubernetes.io prefix - worker",
			key:  "node-role.kubernetes.io/worker",
			want: true,
		},
		{
			name: "custom label - workload-type",
			key:  "workload-type",
			want: false,
		},
		{
			name: "custom label - team",
			key:  "team",
			want: false,
		},
		{
			name: "custom label with domain - app.example.com/tier",
			key:  "app.example.com/tier",
			want: false,
		},
		{
			name: "custom label starting with kubernetes (without dot-io)",
			key:  "kubernetes-custom/label",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWellKnownLabel(tt.key)
			if got != tt.want {
				t.Errorf("isWellKnownLabel(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestCouldSatisfyPod(t *testing.T) {
	tests := []struct {
		name string
		pool *stratosv1alpha1.NodePool
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod with matching custom nodeSelector",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{
							"workload-type": "ml",
						},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"workload-type": "ml",
					},
				},
			},
			want: true,
		},
		{
			name: "pod with mismatched custom nodeSelector value",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{
							"workload-type": "ml",
						},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"workload-type": "web",
					},
				},
			},
			want: false,
		},
		{
			name: "pod with custom label not in pool template",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{}, // Empty labels
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"workload-type": "ml",
					},
				},
			},
			want: false,
		},
		{
			name: "pod with custom label and pool has nil labels",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: nil, // nil labels
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"workload-type": "ml",
					},
				},
			},
			want: false,
		},
		{
			name: "pod with well-known label not in pool template",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{}, // Empty labels
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux",
					},
				},
			},
			want: true,
		},
		{
			name: "pod with topology zone selector not in pool template",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"topology.kubernetes.io/zone": "us-east-1a",
					},
				},
			},
			want: true,
		},
		{
			name: "pod with no nodeSelector",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: nil,
				},
			},
			want: true,
		},
		{
			name: "pod without tolerations for pool taints",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{},
						Taints: []corev1.Taint{
							{
								Key:    "dedicated",
								Value:  "gpu",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Tolerations: nil, // No tolerations
				},
			},
			want: false,
		},
		{
			name: "pod with matching tolerations for pool taints",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{},
						Taints: []corev1.Taint{
							{
								Key:    "dedicated",
								Value:  "gpu",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Value:    "gpu",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pool with no standby capacity",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 0,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{},
			},
			want: false,
		},
		{
			name: "pod with mixed well-known and custom labels - custom missing",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{}, // No labels
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux", // Well-known, OK
						"workload-type":    "ml",    // Custom, NOT in pool
					},
				},
			},
			want: false,
		},
		{
			name: "pod with mixed well-known and custom labels - all present",
			pool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						Labels: map[string]string{
							"workload-type": "ml",
						},
					},
				},
				Status: stratosv1alpha1.NodePoolStatus{
					Standby: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux", // Well-known, OK
						"workload-type":    "ml",    // Custom, matches pool
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := couldSatisfyPod(tt.pool, tt.pod)
			if got != tt.want {
				t.Errorf("couldSatisfyPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPodUnschedulable(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod in pending phase with unschedulable condition",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionFalse,
							Reason: corev1.PodReasonUnschedulable,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod in running phase",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			want: false,
		},
		{
			name: "pod in pending phase without conditions",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase:      corev1.PodPending,
					Conditions: nil,
				},
			},
			want: false,
		},
		{
			name: "pod in pending phase with scheduled condition true",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPodUnschedulable(tt.pod)
			if got != tt.want {
				t.Errorf("isPodUnschedulable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTolerationMatches(t *testing.T) {
	tests := []struct {
		name       string
		toleration corev1.Toleration
		taint      corev1.Taint
		want       bool
	}{
		{
			name: "exact match with Equal operator",
			toleration: corev1.Toleration{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "gpu",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			taint: corev1.Taint{
				Key:    "dedicated",
				Value:  "gpu",
				Effect: corev1.TaintEffectNoSchedule,
			},
			want: true,
		},
		{
			name: "key match with Exists operator",
			toleration: corev1.Toleration{
				Key:      "dedicated",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
			taint: corev1.Taint{
				Key:    "dedicated",
				Value:  "any-value",
				Effect: corev1.TaintEffectNoSchedule,
			},
			want: true,
		},
		{
			name: "empty key with Exists matches all",
			toleration: corev1.Toleration{
				Key:      "",
				Operator: corev1.TolerationOpExists,
			},
			taint: corev1.Taint{
				Key:    "any-key",
				Value:  "any-value",
				Effect: corev1.TaintEffectNoSchedule,
			},
			want: true,
		},
		{
			name: "key mismatch",
			toleration: corev1.Toleration{
				Key:      "different-key",
				Operator: corev1.TolerationOpEqual,
				Value:    "gpu",
			},
			taint: corev1.Taint{
				Key:   "dedicated",
				Value: "gpu",
			},
			want: false,
		},
		{
			name: "value mismatch with Equal operator",
			toleration: corev1.Toleration{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "cpu",
			},
			taint: corev1.Taint{
				Key:   "dedicated",
				Value: "gpu",
			},
			want: false,
		},
		{
			name: "effect mismatch",
			toleration: corev1.Toleration{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "gpu",
				Effect:   corev1.TaintEffectNoExecute,
			},
			taint: corev1.Taint{
				Key:    "dedicated",
				Value:  "gpu",
				Effect: corev1.TaintEffectNoSchedule,
			},
			want: false,
		},
		{
			name: "empty effect in toleration matches any",
			toleration: corev1.Toleration{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "gpu",
				Effect:   "", // Empty effect matches any
			},
			taint: corev1.Taint{
				Key:    "dedicated",
				Value:  "gpu",
				Effect: corev1.TaintEffectNoSchedule,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tolerationMatches(tt.toleration, tt.taint)
			if got != tt.want {
				t.Errorf("tolerationMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
