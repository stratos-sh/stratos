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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestScaleCalculator_CalculateNodesNeeded(t *testing.T) {
	tests := []struct {
		name          string
		nodePool      *stratosv1alpha1.NodePool
		pods          []corev1.Pod
		existingNodes []corev1.Node
		want          int
	}{
		{
			name: "10 pods @ 500m CPU on m5.xlarge -> 2 nodes",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge", // 4 vCPU, 16 GiB
							},
						},
					},
				},
			},
			pods:          makePods(10, "500m", "2Gi"),
			existingNodes: nil,
			// Total: 5000m CPU, 20Gi memory
			// Usable (80%): 3200m CPU, 12.8Gi memory
			// nodesByCPU = ceil(5000/3200) = ceil(1.5625) = 2
			// nodesByMemory = ceil(20/12.8) = ceil(1.5625) = 2
			want: 2,
		},
		{
			name: "1 pod @ 100m CPU on m5.xlarge -> 1 node",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge",
							},
						},
					},
				},
			},
			pods:          makePods(1, "100m", "128Mi"),
			existingNodes: nil,
			want:          1,
		},
		{
			name: "pods without requests + defaults configured -> uses defaults",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					ScaleUp: &stratosv1alpha1.ScaleUpConfig{
						DefaultPodResources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge", // 4 vCPU, 16 GiB
							},
						},
					},
				},
			},
			pods:          makePodsWithoutRequests(10),
			existingNodes: nil,
			// Total: 5000m CPU (10 * 500m default), 10Gi memory (10 * 1Gi default)
			// Usable (80%): 3200m CPU, 12.8Gi memory
			// nodesByCPU = ceil(5000/3200) = 2
			// nodesByMemory = ceil(10/12.8) = 1
			want: 2,
		},
		{
			name: "unknown instance type -> falls back to 1:1",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "unknown.instance",
							},
						},
					},
				},
			},
			pods:          makePods(5, "500m", "1Gi"),
			existingNodes: nil,
			want:          5, // 1:1 fallback
		},
		{
			name: "mixed pods with and without requests",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					ScaleUp: &stratosv1alpha1.ScaleUpConfig{
						DefaultPodResources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge", // 4 vCPU, 16 GiB
							},
						},
					},
				},
			},
			pods: append(
				makePods(5, "500m", "1Gi"),    // 5 pods with explicit requests
				makePodsWithoutRequests(5)..., // 5 pods without requests (use defaults)
			),
			existingNodes: nil,
			// Pods with requests: 5 * 500m = 2500m CPU, 5 * 1Gi = 5Gi memory
			// Pods without requests: 5 * 250m = 1250m CPU, 5 * 512Mi = 2.5Gi memory
			// Total: 3750m CPU, 7.5Gi memory
			// Usable (80%): 3200m CPU, 12.8Gi memory
			// nodesByCPU = ceil(3750/3200) = ceil(1.17) = 2
			// nodesByMemory = ceil(7.5/12.8) = ceil(0.58) = 1
			want: 2,
		},
		{
			name: "no pods -> 0 nodes",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge",
							},
						},
					},
				},
			},
			pods:          nil,
			existingNodes: nil,
			want:          0,
		},
		{
			name: "memory bottleneck -> uses memory-based calculation",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "c5.xlarge", // 4 vCPU, 8 GiB (compute optimized)
							},
						},
					},
				},
			},
			pods:          makePods(5, "100m", "4Gi"),
			existingNodes: nil,
			// Total: 500m CPU, 20Gi memory
			// Usable (80%): 3200m CPU, 6.4Gi memory
			// nodesByCPU = ceil(500/3200) = 1
			// nodesByMemory = ceil(20/6.4) = ceil(3.125) = 4
			want: 4, // Memory is the bottleneck
		},
		{
			name: "uses existing node allocatable when available",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge",
							},
						},
					},
				},
			},
			pods: makePods(10, "500m", "2Gi"),
			existingNodes: []corev1.Node{
				{
					Status: corev1.NodeStatus{
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("3800m"), // Real allocatable (slightly less than 4 vCPU)
							corev1.ResourceMemory: resource.MustParse("15Gi"),  // Real allocatable
						},
					},
				},
			},
			// Total: 5000m CPU, 20Gi memory
			// Usable (80% of allocatable): 3040m CPU, 12Gi memory
			// nodesByCPU = ceil(5000/3040) = ceil(1.64) = 2
			// nodesByMemory = ceil(20/12) = ceil(1.67) = 2
			want: 2,
		},
		{
			name: "pods without requests and no defaults -> falls back to 1:1",
			nodePool: &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: "m5.xlarge",
							},
						},
					},
				},
			},
			pods:          makePodsWithoutRequests(5),
			existingNodes: nil,
			want:          5, // No requests, no defaults -> 1:1 fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := NewScaleCalculator(tt.nodePool)
			got := calc.CalculateNodesNeeded(tt.pods, tt.existingNodes)
			if got != tt.want {
				t.Errorf("CalculateNodesNeeded() = %d, want %d", got, tt.want)
			}
		})
	}
}

// makePods creates n pods with the given CPU and memory requests.
func makePods(n int, cpu, memory string) []corev1.Pod {
	pods := make([]corev1.Pod, n)
	for i := 0; i < n; i++ {
		pods[i] = corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "nginx",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpu),
								corev1.ResourceMemory: resource.MustParse(memory),
							},
						},
					},
				},
			},
		}
	}
	return pods
}

// makePodsWithoutRequests creates n pods without resource requests.
func makePodsWithoutRequests(n int) []corev1.Pod {
	pods := make([]corev1.Pod, n)
	for i := 0; i < n; i++ {
		pods[i] = corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod-no-requests",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "nginx",
						// No resource requests
					},
				},
			},
		}
	}
	return pods
}

func TestGetInstanceCapacity(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		wantCPU      int64 // in millicores
		wantMemory   int64 // in bytes (Gi)
		wantZero     bool
	}{
		{
			name:         "m5.xlarge",
			instanceType: "m5.xlarge",
			wantCPU:      4000,
			wantMemory:   16,
		},
		{
			name:         "c5.xlarge",
			instanceType: "c5.xlarge",
			wantCPU:      4000,
			wantMemory:   8,
		},
		{
			name:         "r5.xlarge",
			instanceType: "r5.xlarge",
			wantCPU:      4000,
			wantMemory:   32,
		},
		{
			name:         "unknown instance type",
			instanceType: "unknown.type",
			wantZero:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodePool := &stratosv1alpha1.NodePool{
				Spec: stratosv1alpha1.NodePoolSpec{
					Template: stratosv1alpha1.NodeTemplate{
						CloudProvider: stratosv1alpha1.CloudProviderConfig{
							Provider: "aws",
							AWS: &stratosv1alpha1.AWSConfig{
								InstanceType: tt.instanceType,
							},
						},
					},
				},
			}

			calc := NewScaleCalculator(nodePool)
			cap := calc.getNodeCapacity(nil)

			if tt.wantZero {
				if !cap.CPU.IsZero() || !cap.Memory.IsZero() {
					t.Errorf("expected zero capacity for unknown instance type, got CPU=%v, Memory=%v",
						cap.CPU.String(), cap.Memory.String())
				}
				return
			}

			if cap.CPU.MilliValue() != tt.wantCPU {
				t.Errorf("CPU = %d millicores, want %d", cap.CPU.MilliValue(), tt.wantCPU)
			}
			// Memory is in Gi, convert to bytes for comparison
			wantMemBytes := tt.wantMemory * 1024 * 1024 * 1024
			if cap.Memory.Value() != wantMemBytes {
				t.Errorf("Memory = %d bytes, want %d", cap.Memory.Value(), wantMemBytes)
			}
		})
	}
}
