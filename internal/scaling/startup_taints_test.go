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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

func TestProcessStartupTaints_NetworkReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node started 45s ago, network became ready 40s ago (after node start)
	nodeStartTime := time.Now().Add(-45 * time.Second)

	nrt := nodestate.NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				nodestate.AnnotationLastStarted: nodeStartTime.Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: nodestate.NetworkingReadyCondition, Status: corev1.ConditionTrue, Reason: "NetworkingIsReady"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)

	testStrategy := &Scaler{
		client:         fakeClient,
		recorder:       recorder,
		networkChecker: newNetworkReadinessChecker(fakeClient, nil),
	}

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := t.Context()
	removed, err := testStrategy.processStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("processStartupTaints() error = %v", err)
	}
	if !removed {
		t.Error("processStartupTaints() should return true when network ready")
	}

	// Verify the taint was removed
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if nodestate.HasTaintWithKeyAndEffect(updatedNode.Spec.Taints, nodestate.TaintKeyNotReady, corev1.TaintEffectNoSchedule) {
		t.Error("Network readiness taint should have been removed from node")
	}
}

func TestProcessStartupTaints_Timeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node started more than StartupTaintRemovalTimeout ago
	nrt := nodestate.NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				nodestate.AnnotationLastStarted: time.Now().Add(-3 * time.Minute).Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				// No NetworkingReady condition - network not ready
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)

	testStrategy := &Scaler{
		client:         fakeClient,
		recorder:       recorder,
		networkChecker: newNetworkReadinessChecker(fakeClient, nil),
	}

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := t.Context()
	removed, err := testStrategy.processStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("processStartupTaints() error = %v", err)
	}
	if !removed {
		t.Error("processStartupTaints() should return true on timeout (force remove)")
	}

	// Verify the taint was removed despite network not ready
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if nodestate.HasTaintWithKeyAndEffect(updatedNode.Spec.Taints, nodestate.TaintKeyNotReady, corev1.TaintEffectNoSchedule) {
		t.Error("Network readiness taint should have been removed on timeout")
	}
}

func TestProcessStartupTaints_DefaultEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node started 45s ago, network became ready 40s ago (after node start)
	nodeStartTime := time.Now().Add(-45 * time.Second)

	nrt := nodestate.NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				nodestate.AnnotationLastStarted: nodeStartTime.Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: nodestate.NetworkingReadyCondition, Status: corev1.ConditionTrue, Reason: "NetworkingIsReady"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)

	testStrategy := &Scaler{
		client:         fakeClient,
		recorder:       recorder,
		networkChecker: newNetworkReadinessChecker(fakeClient, nil),
	}

	// NetworkReadinessStrategy is nil - should default to Taint (enabled)
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				// NetworkReadinessStrategy not set - defaults to Taint
			},
		},
	}

	ctx := t.Context()
	removed, err := testStrategy.processStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("processStartupTaints() error = %v", err)
	}
	if !removed {
		t.Error("processStartupTaints() should enable network readiness taint by default (nil = true)")
	}
}
