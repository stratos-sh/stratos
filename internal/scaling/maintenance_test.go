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

package scaling

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// TestEnsureTemplateLabels_FixesMissingLabels tests that ensureTemplateLabels patches
// nodes that are missing template labels.
func TestEnsureTemplateLabels_FixesMissingLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node without template labels
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				nodestate.LabelPool:  "test-pool",
				nodestate.LabelState: string(nodestate.NodeStateStandby),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	scalingStrategy := &Scaler{
		client: fakeClient,
	}

	nodePool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				Labels: map[string]string{
					"workload": "general",
					"env":      "staging",
				},
			},
		},
	}

	ctx := t.Context()
	err := scalingStrategy.ensureTemplateLabels(ctx, nodePool)
	if err != nil {
		t.Fatalf("ensureTemplateLabels() error = %v", err)
	}

	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}

	if updatedNode.Labels["workload"] != "general" {
		t.Errorf("Template label 'workload' = %q, want %q", updatedNode.Labels["workload"], "general")
	}
	if updatedNode.Labels["env"] != "staging" {
		t.Errorf("Template label 'env' = %q, want %q", updatedNode.Labels["env"], "staging")
	}
}

// TestEnsureTemplateLabels_NoOpWhenPresent tests that ensureTemplateLabels does nothing
// when all template labels are already correctly set.
func TestEnsureTemplateLabels_NoOpWhenPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node already has all template labels
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				nodestate.LabelPool:  "test-pool",
				nodestate.LabelState: string(nodestate.NodeStateRunning),
				"workload":           "general",
				"env":                "production",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	scalingStrategy := &Scaler{
		client: fakeClient,
	}

	nodePool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				Labels: map[string]string{
					"workload": "general",
					"env":      "production",
				},
			},
		},
	}

	ctx := t.Context()
	err := scalingStrategy.ensureTemplateLabels(ctx, nodePool)
	if err != nil {
		t.Fatalf("ensureTemplateLabels() error = %v", err)
	}

	// Verify labels remain unchanged
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}

	if updatedNode.Labels["workload"] != "general" {
		t.Errorf("Template label 'workload' = %q, want %q", updatedNode.Labels["workload"], "general")
	}
	if updatedNode.Labels["env"] != "production" {
		t.Errorf("Template label 'env' = %q, want %q", updatedNode.Labels["env"], "production")
	}
}
